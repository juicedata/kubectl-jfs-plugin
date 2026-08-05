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
	"strings"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

	skippedPods, err := d.filterPodsInOngoingUpgradeJobs()
	if err != nil {
		return err
	}
	if len(skippedPods) > 0 {
		fmt.Printf("Skip %d pods already in ongoing upgrade jobs: %s\n", len(skippedPods), strings.Join(skippedPods, ", "))
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
	dashboardImage := os.Getenv("DASHBOARD_IMAGE")
	dashboardSA := os.Getenv("JUICEFS_CSI_DASHBOARD_SA")

	if dashboardImage == "" || dashboardSA == "" {
		dashboardDeployment, err := util.GetCSIDashboardDeployment(d.clientSet)
		if err != nil {
			return err
		}
		os.Setenv("JUICEFS_CSI_DASHBOARD_SA", getEnvFromDeployment(dashboardDeployment, "JUICEFS_CSI_DASHBOARD_SA", "juicefs-csi-dashboard-sa"))
		os.Setenv("DASHBOARD_IMAGE", getEnvFromDeployment(dashboardDeployment, "DASHBOARD_IMAGE", getImageFromDeployment(dashboardDeployment)))
	}

	// create job
	newJob := dashboard.NewUpgradeJob(jobName)
	addBatchUpgradeTimeoutEnv(newJob)
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

func (d *DiffAnalyzer) filterPodsInOngoingUpgradeJobs() ([]string, error) {
	filteredPods, skippedPods, err := jConfig.FilterPodsNotInOngoingUpgrade(context.TODO(), d.k8sClient, d.podsNeedToUpdate)
	if err != nil {
		return nil, err
	}
	d.podsNeedToUpdate = filteredPods
	d.podDiffs = filterPodDiffsByPodNames(d.podDiffs, skippedPods)
	return skippedPods, nil
}

func filterPodDiffsByPodNames(podDiffs []dashboard.PodDiff, skippedPodNames []string) []dashboard.PodDiff {
	if len(skippedPodNames) == 0 || len(podDiffs) == 0 {
		return podDiffs
	}
	skippedSet := make(map[string]struct{}, len(skippedPodNames))
	for _, name := range skippedPodNames {
		skippedSet[name] = struct{}{}
	}
	filteredDiffs := make([]dashboard.PodDiff, 0, len(podDiffs))
	for _, diff := range podDiffs {
		if _, exists := skippedSet[diff.Pod.Name]; exists {
			continue
		}
		filteredDiffs = append(filteredDiffs, diff)
	}
	return filteredDiffs
}

func (d *DiffAnalyzer) getUniqueIdOfPVC(pvc *corev1.PersistentVolumeClaim, csiNodes []corev1.Pod) (string, error) {
	pv, err := util.GetPVOfPVC(d.clientSet, pvc)
	if err != nil {
		return "", err
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != config.DriverName {
		return "", fmt.Errorf("pvc %s is not a juicefs csi pvc", pvc.Name)
	}
	storageClassShareMount, fsShareMount := util.GetShareMountModes(csiNodes)

	var secret *corev1.Secret
	if fsShareMount && pv.Spec.CSI != nil && pv.Spec.CSI.NodePublishSecretRef != nil {
		secretName := pv.Spec.CSI.NodePublishSecretRef.Name
		secretNamespace := pv.Spec.CSI.NodePublishSecretRef.Namespace
		secret, err = d.clientSet.CoreV1().Secrets(secretNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
	}

	return uniqueIdFromPV(pv, storageClassShareMount, fsShareMount, secret), nil
}

func uniqueIdFromPV(pv *corev1.PersistentVolume, storageClassShareMount, fsShareMount bool, secret *corev1.Secret) string {
	if pv == nil || pv.Spec.CSI == nil {
		return ""
	}
	if storageClassShareMount && pv.Spec.StorageClassName != "" {
		return pv.Spec.StorageClassName
	}
	if fsShareMount && secret != nil {
		if fsname, ok := secret.Data["name"]; ok && string(fsname) != "" {
			return string(fsname)
		}
	}
	return pv.Spec.CSI.VolumeHandle
}

func getEnvFromPod(pod *corev1.Pod, key string, defaultVal string) string {
	if len(pod.Spec.Containers) == 0 {
		return defaultVal
	}
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == key {
			return env.Value
		}
	}
	return defaultVal
}

func getEnvFromDeployment(deployment *appsv1.Deployment, key string, defaultVal string) string {
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return defaultVal
	}
	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == key {
			return env.Value
		}
	}
	return defaultVal
}

func getImageFromDeployment(deployment *appsv1.Deployment) string {
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return ""
	}
	return deployment.Spec.Template.Spec.Containers[0].Image
}

func addBatchUpgradeTimeoutEnv(job *batchv1.Job) {
	timeout := os.Getenv("BATCH_UPGRADE_TIMEOUT")
	if timeout == "" || job == nil || len(job.Spec.Template.Spec.Containers) == 0 {
		return
	}
	envs := job.Spec.Template.Spec.Containers[0].Env
	for i := range envs {
		if envs[i].Name == "BATCH_UPGRADE_TIMEOUT" {
			envs[i].Value = timeout
			job.Spec.Template.Spec.Containers[0].Env = envs
			return
		}
	}
	job.Spec.Template.Spec.Containers[0].Env = append(envs, corev1.EnvVar{Name: "BATCH_UPGRADE_TIMEOUT", Value: timeout})
}
