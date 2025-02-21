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

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (d *DiffAnalyzer) NewUpgradeJob(pvcName, nodeName string, worker int, ignoreErr bool, quiet bool) error {
	jobName := dashboard.GenUpgradeJobName()

	cmName := dashboard.GenUpgradeConfig(jobName)
	if err := d.generatePodsDiff(nil); err != nil {
		return err
	}
	csiNodes, err := util.GetCSINodeList(d.clientSet, nodeName)
	if err != nil {
		return err
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

	batchjConfig := jConfig.NewBatchConfig(d.podsNeedToUpdate, worker, ignoreErr, true, nodeName, uniqueId, csiNodes)

	if len(batchjConfig.Batches) == 0 {
		return fmt.Errorf("no pod needs to upgrade")
	}

	out, err := d.printDiff()
	if err != nil {
		return err
	}
	fmt.Println(out)

	fmt.Print("The above pods will be upgraded, please confirm (y/n): ")

	if !quiet {
		if confirm := util.WaitForConfirm(); !confirm {
			fmt.Println("Upgrade job canceled")
			return nil
		}
	}

	// set global config in jConfig
	jConfig.Namespace = config.MountNamespace
	cfg, err := jConfig.CreateUpgradeConfig(context.TODO(), d.k8sClient, cmName, batchjConfig)
	if err != nil {
		return err
	}
	newJob, err := d.newUpgradeJob(jobName)
	if err != nil {
		return err
	}
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
	detailCmd := fmt.Sprintf("kubectl jfs batch describe %s", job.Name)
	if config.MountNamespace != "kube-system" {
		detailCmd = fmt.Sprintf("%s -m %s", detailCmd, config.MountNamespace)
	}
	fmt.Printf("Job for batch upgrade created: %s, please execute the following command to see details:\n%s\n", job.Name, detailCmd)
	return nil
}

func (d *DiffAnalyzer) newUpgradeJob(jobName string) (*batchv1.Job, error) {
	dashboardPod, err := util.GetCSIDashboardPod(d.clientSet)
	if err != nil {
		return nil, err
	}
	sysNamespace := config.MountNamespace
	cmds := []string{"juicefs-csi-dashboard", "upgrade"}
	sa := getEnvFromPod(dashboardPod, "JUICEFS_CSI_DASHBOARD_SA")
	if sa == "" {
		sa = "juicefs-csi-dashboard-sa"
	}
	jConfigName := dashboard.GenUpgradeConfig(jobName)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: sysNamespace,
			Labels: map[string]string{
				common.PodTypeKey:       common.JobTypeValue,
				common.JfsJobKind:       common.KindOfUpgrade,
				common.JfsUpgradeConfig: jConfigName,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism:             util.ToPtr(int32(1)),
			Completions:             util.ToPtr(int32(1)),
			BackoffLimit:            util.ToPtr(int32(0)),
			TTLSecondsAfterFinished: util.ToPtr(int32(3600 * 24 * 7)), // automatically deleted after 7 day
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						common.PodTypeKey:        common.JobTypeValue,
						common.JfsJobKind:        common.KindOfUpgrade,
						common.JfsUpgradeJobName: jobName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "juicefs-upgrade",
						Image:   getEnvFromPod(dashboardPod, "DASHBOARD_IMAGE"),
						Command: cmds,
						Env: []corev1.EnvVar{
							{Name: "SYS_NAMESPACE", Value: sysNamespace},
							{Name: common.JfsUpgradeConfig, Value: jConfigName},
						},
					}},
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: sa,
				},
			},
		},
	}, nil
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

func getEnvFromPod(pod *corev1.Pod, key string) string {
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == key {
			return env.Value
		}
	}
	return ""
}
