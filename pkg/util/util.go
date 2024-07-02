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

func GetCSINodeList(clientSet *kubernetes.Clientset) ([]corev1.Pod, error) {
	nodeLabelMap, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{config.PodTypeKey: "juicefs-csi-driver", "app": "juicefs-csi-node"},
	})
	csiNodeList, err := clientSet.CoreV1().Pods(config.MountNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: nodeLabelMap.String()})
	if err != nil {
		return nil, err
	}
	return csiNodeList.Items, nil
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
