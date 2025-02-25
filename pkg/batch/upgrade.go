/*
 * Copyright 2025 Juicedata Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package batch

import (
	"context"
	"fmt"
	"os"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (d *DiffAnalyzer) NewUpgradeJob(pvcName, nodeName string, worker int, ignoreErr bool, quiet bool) error {
	jobName := dashboard.GenUpgradeJobName()
	cmName := dashboard.GenUpgradeConfig(jobName)
	csiNodes, err := util.GetCSINodeList(d.clientSet, nodeName)
	if err != nil {
		return err
	}
	if nodeName != "" && len(csiNodes) == 0 {
		return fmt.Errorf("there is no csi node on node: %s", nodeName)
	}

	uniqueId := ""
	if pvcName != "" {
		pvc, err := util.FindPVC(d.clientSet, pvcName)
		if err != nil {
			return err
		}
		if pvc != nil {
			uniqueId, err = d.getUniqueIdOfPVC(pvc, csiNodes)
			if err != nil {
				return err
			}
		}
	}
	if err := d.generatePodsDiff(nodeName, uniqueId); err != nil {
		return err
	}

	batchjConfig := jConfig.NewBatchConfig(d.podsNeedToUpdate, worker, ignoreErr, true, nodeName, uniqueId, csiNodes)

	if len(batchjConfig.Batches) == 0 {
		return fmt.Errorf("no pod needs to upgrade")
	}

	fmt.Println("The following pods will be upgraded:")
	out, err := d.printDiff()
	if err != nil {
		return err
	}
	fmt.Println(out)

	if !quiet {
		fmt.Print("Please confirm (y/n): ")
		if confirm := util.WaitForConfirm(); !confirm {
			fmt.Println("Upgrade job canceled")
			return nil
		}
	}

	// create configMap of upgrade job
	cfg, err := jConfig.CreateUpgradeConfig(context.TODO(), d.k8sClient, cmName, batchjConfig)
	if err != nil {
		return err
	}
	// set dashboard sa and image in env
	dashboardPod, err := util.GetCSIDashboardPod(d.clientSet)
	if err != nil {
		return err
	}
	os.Setenv("JUICEFS_CSI_DASHBOARD_SA", getEnvFromPod(dashboardPod, "JUICEFS_CSI_DASHBOARD_SA", "juicefs-csi-dashboard-sa"))
	os.Setenv("DASHBOARD_IMAGE", getEnvFromPod(dashboardPod, "DASHBOARD_IMAGE", getImageFromPod(dashboardPod)))

	// create job
	newJob := dashboard.NewUpgradeJob(jobName)
	job, err := d.clientSet.BatchV1().Jobs(newJob.Namespace).Create(context.TODO(), newJob, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if cfg, err = d.clientSet.CoreV1().ConfigMaps(cfg.Namespace).Get(context.TODO(), cfg.Name, metav1.GetOptions{}); err != nil {
		return err
	}
	dashboard.SetJobAsConfigMapOwner(cfg, job)
	if _, err := d.clientSet.CoreV1().ConfigMaps(cfg.Namespace).Update(context.TODO(), cfg, metav1.UpdateOptions{}); err != nil {
		return err
	}

	// print describe cmd
	detailCmd := fmt.Sprintf("kubectl jfs batch describe %s", job.Name)
	if config.MountNamespace != "kube-system" {
		detailCmd = fmt.Sprintf("%s -m %s", detailCmd, config.MountNamespace)
	}
	fmt.Printf("Job for batch upgrade created: %s, please execute the following command to see details:\n%s\n", job.Name, detailCmd)
	return nil
}

func (d *DiffAnalyzer) getUniqueIdOfPVC(pvc *corev1.PersistentVolumeClaim, csiNodes []corev1.Pod) (string, error) {
	pv, err := util.GetPVOfPVC(d.clientSet, pvc)
	if err != nil {
		return "", err
	}

	uniqueId := pv.Spec.CSI.VolumeHandle
	if len(csiNodes) != 0 && util.IsShareMount(&csiNodes[0]) {
		uniqueId = pv.Spec.StorageClassName
	}
	return uniqueId, nil
}

func (d *DiffAnalyzer) getPVCByUniqueId(uniqueId string) (*corev1.PersistentVolumeClaim, error) {
	csiNodes, err := util.GetCSINodeList(d.clientSet, "")
	if err != nil {
		return nil, err
	}
	if len(csiNodes) != 0 && util.IsShareMount(&csiNodes[0]) {
		pvcs, err := d.clientSet.CoreV1().PersistentVolumeClaims("").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, pvc := range pvcs.Items {
			if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == uniqueId {
				return &pvc, nil
			}
		}
		return nil, fmt.Errorf("pvc not found by uniqueId: %s", uniqueId)
	}
	pv := &corev1.PersistentVolume{}
	pvs, err := d.clientSet.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, p := range pvs.Items {
		if p.Spec.CSI != nil && p.Spec.CSI.VolumeHandle == uniqueId {
			pv = &p
			break
		}
	}

	if pv.Spec.ClaimRef == nil {
		return nil, fmt.Errorf("pvc not found by uniqueId: %s", uniqueId)
	}
	pvc, err := d.clientSet.CoreV1().PersistentVolumeClaims(pv.Spec.ClaimRef.Namespace).Get(context.Background(), pv.Spec.ClaimRef.Name, metav1.GetOptions{})
	return pvc, err
}

func getEnvFromPod(pod *corev1.Pod, key string, defaultVal string) string {
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == key {
			return env.Value
		}
	}
	return defaultVal
}

func getImageFromPod(pod *corev1.Pod) string {
	return pod.Spec.Containers[0].Image
}
