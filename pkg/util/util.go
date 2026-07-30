/*
 * Copyright 2024 Juicedata Inc
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

package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
)

func ClientSet(configFlags *genericclioptions.ConfigFlags) (*kubernetes.Clientset, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return clientSet, nil
}

func GetMountPodList(clientSet *kubernetes.Clientset, volumeId string) ([]corev1.Pod, error) {
	labelSelector := labels.Set{config.PodTypeKey: config.PodTypeValue}
	if volumeId != "" {
		labelSelector[config.PodUniqueIdLabelKey] = volumeId
	}
	mountLabelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: labelSelector,
	})
	mountList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(),
		metav1.ListOptions{LabelSelector: mountLabelMap.String()},
	)
	if err != nil {
		return nil, err
	}
	return mountList.Items, nil
}

func GetMountPodOnNode(clientSet *kubernetes.Clientset, nodeName string) ([]corev1.Pod, error) {
	fieldSelector := fields.Set{"spec.nodeName": nodeName}
	mountLabelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{config.PodTypeKey: config.PodTypeValue},
	})
	mountList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: mountLabelMap.String(),
		FieldSelector: fieldSelector.String(),
	})
	if err != nil {
		return nil, err
	}
	return mountList.Items, nil
}

func GetPodList(clientSet *kubernetes.Clientset, ns string) ([]corev1.Pod, error) {
	podList, err := clientSet.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func ListNodePodsByUID(clientSet kubernetes.Interface, nodeName string) (map[string]*corev1.Pod, error) {
	podList, err := clientSet.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
	})
	if err != nil {
		return nil, err
	}

	podsByUID := make(map[string]*corev1.Pod, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if nodeName != "" && pod.Spec.NodeName != nodeName {
			continue
		}
		podsByUID[string(pod.UID)] = pod
	}
	return podsByUID, nil
}

func GetAppPodList(clientSet *kubernetes.Clientset, ns string) ([]corev1.Pod, error) {
	labelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      config.UniqueId,
			Operator: metav1.LabelSelectorOpExists,
		}},
	})
	podList, err := clientSet.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: labelMap.String()})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func GetCSINodeList(clientSet *kubernetes.Clientset, nodeName string) ([]corev1.Pod, error) {
	nodeLabelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{config.PodTypeKey: "juicefs-csi-driver", "app": "juicefs-csi-node"},
	})
	listOptions := metav1.ListOptions{LabelSelector: nodeLabelMap.String()}
	if nodeName != "" {
		listOptions.FieldSelector = fields.Set{"spec.nodeName": nodeName}.String()
	}
	csiNodeList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(), listOptions)
	if err != nil {
		return nil, err
	}
	return csiNodeList.Items, nil
}

func GetCSIDashboardPod(clientSet *kubernetes.Clientset) (*corev1.Pod, error) {
	labelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{config.PodTypeKey: "juicefs-csi-driver", "app": "juicefs-csi-dashboard"},
	})
	podList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: labelMap.String()})
	if err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no juicefs-csi-dashboard pod found")
	}
	return &podList.Items[0], nil

}

func GetCSIDashboardDeployment(clientSet *kubernetes.Clientset) (*appsv1.Deployment, error) {
	deployment, err := clientSet.AppsV1().Deployments(config.MountNamespace).Get(context.Background(), "juicefs-csi-dashboard", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment juicefs-csi-dashboard failed: %w", err)
	}
	return deployment, nil
}

func ListBatchJobs(clientSet *kubernetes.Clientset) ([]batchv1.Job, error) {
	labelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.PodTypeKey: common.JobTypeValue,
			common.JfsJobKind: common.KindOfUpgrade,
		},
	})
	jobList, err := clientSet.BatchV1().Jobs(config.MountNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: labelMap.String()})
	if err != nil {
		return nil, err
	}
	return jobList.Items, nil
}

func GetJob(clientSet *kubernetes.Clientset, jobName string) (*batchv1.Job, error) {
	job, err := clientSet.BatchV1().Jobs(config.MountNamespace).Get(context.Background(), jobName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return job, nil
}

func ListMoundPods(clientSet *kubernetes.Clientset, nodeName, uniqueId string) ([]corev1.Pod, error) {
	ls := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name": "juicefs-mount",
		},
	}
	if uniqueId != "" {
		ls.MatchLabels[common.PodUniqueIdLabelKey] = uniqueId
	}
	sls, _ := metav1.LabelSelectorAsSelector(ls)
	listOptions := metav1.ListOptions{
		LabelSelector: sls.String(),
	}
	if nodeName != "" {
		fieldSelector := fields.Set{"spec.nodeName": nodeName}.AsSelector()
		listOptions.FieldSelector = fieldSelector.String()
	}
	pods, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(), listOptions)
	return pods.Items, err
}

func ListBatchPods(clientSet *kubernetes.Clientset, conf *jConfig.BatchConfig) ([]corev1.Pod, error) {
	pods, err := ListMoundPods(clientSet, conf.Node, conf.UniqueId)
	if err != nil {
		return nil, err
	}
	podsMap := make(map[string]corev1.Pod)
	for _, pod := range pods {
		podsMap[pod.Name] = pod
	}

	results := make([]corev1.Pod, 0)
	for _, batch := range conf.Batches {
		for _, p := range batch {
			if po, ok := podsMap[p.Name]; ok {
				results = append(results, po)
			}
		}
	}

	return results, nil
}

func ListUpgradeConfigs(clientSet *kubernetes.Clientset) (map[string]*jConfig.BatchConfig, error) {
	var (
		cmList  *corev1.ConfigMapList
		configs = make(map[string]*jConfig.BatchConfig)
		err     error
	)
	s, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.PodTypeKey: common.ConfigTypeValue,
		},
	})
	cmList, err = clientSet.CoreV1().ConfigMaps(config.MountNamespace).List(context.TODO(), metav1.ListOptions{LabelSelector: s.String()})
	if err != nil {
		return nil, err
	}
	for _, cm := range cmList.Items {
		cfg, err := jConfig.LoadBatchConfig(&cm)
		if err != nil {
			return nil, err
		}
		configs[cm.Name] = cfg
	}
	return configs, nil
}

func GetPodOfUpgradeJob(clientSet *kubernetes.Clientset, job *batchv1.Job) (*corev1.Pod, error) {
	if job == nil {
		return nil, nil
	}
	s, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.PodTypeKey:        common.JobTypeValue,
			common.JfsJobKind:        common.KindOfUpgrade,
			common.JfsUpgradeJobName: job.Name,
		},
	})
	podList, err := clientSet.CoreV1().Pods(job.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: s.String()})
	if err == nil && len(podList.Items) != 0 {
		return &podList.Items[0], nil
	}
	return nil, fmt.Errorf("no pod found for job %s", job.Name)
}

func GetPVCList(clientSet *kubernetes.Clientset, ns string) ([]corev1.PersistentVolumeClaim, error) {
	pvcList, err := clientSet.CoreV1().PersistentVolumeClaims(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcList.Items, nil
}

func GetPVList(clientSet *kubernetes.Clientset) ([]corev1.PersistentVolume, error) {
	pvList, err := clientSet.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvList.Items, nil
}

func GetStorageClassList(clientSet *kubernetes.Clientset) ([]storagev1.StorageClass, error) {
	scList, err := clientSet.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return scList.Items, nil
}

func GetSecretList(clientSet *kubernetes.Clientset, namespace string) ([]corev1.Secret, error) {
	secretList, err := clientSet.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return secretList.Items, nil
}

func FindPVC(clientSet *kubernetes.Clientset, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	pvcs, err := clientSet.CoreV1().PersistentVolumeClaims("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pvc := range pvcs.Items {
		if pvc.Name == pvcName {
			return &pvc, nil
		}
	}
	return nil, fmt.Errorf("pvc %s not found", pvcName)
}

func GetPVOfPVC(clientSet *kubernetes.Clientset, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolume, error) {
	if pvc.Spec.VolumeName == "" {
		return nil, fmt.Errorf("pvc %s has no volumeName", pvc.Name)
	}
	pv, err := clientSet.CoreV1().PersistentVolumes().Get(context.Background(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func GetCSINode(clientSet *kubernetes.Clientset, nodeName string) (*corev1.Pod, error) {
	fieldSelector := fields.Set{"spec.nodeName": nodeName}
	nodeLabelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{config.PodTypeKey: "juicefs-csi-driver", "app": "juicefs-csi-node"},
	})
	csiNodeList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(),
		metav1.ListOptions{
			LabelSelector: nodeLabelMap.String(),
			FieldSelector: fieldSelector.String(),
		})
	if err != nil {
		return nil, err
	}
	if csiNodeList == nil || len(csiNodeList.Items) == 0 {
		return nil, nil
	}
	return &csiNodeList.Items[0], nil
}

func GetNamespaceList(clientSet *kubernetes.Clientset) ([]corev1.Namespace, error) {
	namespaces, err := clientSet.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return namespaces.Items, nil
}

func TabbedString(f func(io.Writer) error) (string, error) {
	out := new(tabwriter.Writer)
	buf := &bytes.Buffer{}
	out.Init(buf, 0, 8, 2, ' ', 0)

	err := f(out)
	if err != nil {
		return "", err
	}

	out.Flush()
	return buf.String(), nil
}

// getPodStatus: copy from kubernetes/pkg/printers/internalversion/printers.go, which `kubectl get po` used.
func GetPodStatus(pod corev1.Pod) string {
	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	initializing := false
	for i := range pod.Status.InitContainerStatuses {
		container := pod.Status.InitContainerStatuses[i]
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(pod.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		hasRunning := false
		for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := pod.Status.ContainerStatuses[i]

			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			if hasPodReadyCondition(pod.Status.Conditions) {
				reason = "Running"
			} else {
				reason = "NotReady"
			}
		}
	}

	if pod.DeletionTimestamp != nil && pod.Status.Reason == "NodeLost" {
		reason = "Unknown"
	} else if pod.DeletionTimestamp != nil {
		reason = "Terminating"
	}
	return reason
}

func GetPVStatus(pv corev1.PersistentVolume) string {
	if pv.DeletionTimestamp != nil {
		return "Terminating"
	}
	return string(pv.Status.Phase)
}

func GetPVCStatus(pvc corev1.PersistentVolumeClaim) string {
	if pvc.DeletionTimestamp != nil {
		return "Terminating"
	}
	return string(pvc.Status.Phase)
}

func GetJobStatus(job batchv1.Job) string {
	var status string
	if hasJobCondition(job.Status.Conditions, batchv1.JobComplete) {
		status = "Complete"
	} else if hasJobCondition(job.Status.Conditions, batchv1.JobFailed) {
		status = "Failed"
	} else if job.ObjectMeta.DeletionTimestamp != nil {
		status = "Terminating"
	} else if hasJobCondition(job.Status.Conditions, batchv1.JobSuspended) {
		status = "Suspended"
	} else if hasJobCondition(job.Status.Conditions, batchv1.JobFailureTarget) {
		status = "FailureTarget"
	} else {
		status = "Running"
	}
	return status
}

func GetJobDuration(job batchv1.Job) string {
	var jobDuration string
	switch {
	case job.Status.StartTime == nil:
	case job.Status.CompletionTime == nil:
		jobDuration = duration.HumanDuration(time.Since(job.Status.StartTime.Time))
	default:
		jobDuration = duration.HumanDuration(job.Status.CompletionTime.Sub(job.Status.StartTime.Time))
	}
	return jobDuration
}

func hasJobCondition(conditions []batchv1.JobCondition, conditionType batchv1.JobConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func GetContainerErrorMessage(pod corev1.Pod) string {
	for _, cn := range pod.Status.InitContainerStatuses {
		if cn.State.Waiting != nil && cn.State.Waiting.Message != "" {
			return cn.State.Waiting.Message
		}
		if cn.State.Terminated != nil && cn.State.Terminated.Message != "" {
			return cn.State.Terminated.Message
		}
	}
	for _, cn := range pod.Status.ContainerStatuses {
		if cn.State.Waiting != nil && cn.State.Waiting.Message != "" {
			return cn.State.Waiting.Message
		}
		if cn.State.Terminated != nil && cn.State.Terminated.Message != "" {
			return cn.State.Terminated.Message
		}
	}
	return ""
}

func hasPodReadyCondition(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func IfNil(field string) string {
	if field == "" {
		return "<none>"
	}
	return field
}

func TranslateTimestampSince(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

func IsPodReady(pod *corev1.Pod) bool {
	conditionsTrue := 0
	for _, cond := range pod.Status.Conditions {
		if cond.Status == corev1.ConditionTrue && (cond.Type == corev1.ContainersReady || cond.Type == corev1.PodReady) {
			conditionsTrue++
		}
	}
	return conditionsTrue == 2
}

func GetMountPathOfPod(pod corev1.Pod) (string, string, error) {
	if len(pod.Spec.Containers) == 0 {
		return "", "", fmt.Errorf("pod %v has no container", pod.Name)
	}
	cmd := pod.Spec.Containers[0].Command
	if cmd == nil || len(cmd) < 3 {
		return "", "", fmt.Errorf("get error pod command:%v", cmd)
	}
	sourcePath, volumeId, err := parseMntPath(cmd[2])
	if err != nil {
		return "", "", err
	}
	return sourcePath, volumeId, nil
}

// ParseMntPath return mntPath, volumeId (/jfs/volumeId, volumeId err)
func parseMntPath(cmd string) (string, string, error) {
	cmds := strings.Split(cmd, "\n")
	mc := cmds[len(cmds)-1]
	args := strings.Fields(mc)
	if len(args) < 3 || !strings.HasPrefix(args[2], config.PodMountBase) {
		return "", "", fmt.Errorf("err cmd:%s", cmd)
	}
	argSlice := strings.Split(args[2], "/")
	if len(argSlice) < 3 {
		return "", "", fmt.Errorf("err mntPath:%s", args[2])
	}
	return args[2], argSlice[2], nil
}

func ToPtr[T any](v T) *T {
	return &v
}
func IsShareMount(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	storageClassShareMount, _ := GetShareMountModes([]corev1.Pod{*pod})
	return storageClassShareMount
}

func GetShareMountModes(csiNodes []corev1.Pod) (storageClassShareMount bool, fsShareMount bool) {
	for _, pod := range csiNodes {
		if len(pod.Spec.Containers) == 0 {
			continue
		}
		for _, env := range pod.Spec.Containers[0].Env {
			if !strings.EqualFold(env.Value, "true") {
				continue
			}
			switch env.Name {
			case "STORAGE_CLASS_SHARE_MOUNT":
				storageClassShareMount = true
			case "FS_SHARE_MOUNT":
				fsShareMount = true
			}
			if storageClassShareMount && fsShareMount {
				return true, true
			}
		}
	}
	return storageClassShareMount, fsShareMount
}

func WaitForConfirm() bool {
	var input string
	for {
		_, err := fmt.Scanln(&input)
		if err != nil {
			fmt.Println("An error occurred while reading input. Please try again.")
			var discard string
			fmt.Scanln(&discard)
			continue
		}

		input = strings.ToLower(input)

		if input == "y" {
			return true
		} else if input == "n" {
			return false
		} else {
			fmt.Printf("Invalid input. Please enter 'y' or 'n':")
		}
	}
}
