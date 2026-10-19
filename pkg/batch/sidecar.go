/*
 * Copyright 2026 Juicedata Inc
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
	"encoding/json"
	"fmt"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/sidecar"
	jUtil "github.com/juicedata/juicefs-csi-driver/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func listSidecarPods(ctx context.Context, client kubernetes.Interface, namespace, nodeName string) ([]corev1.Pod, error) {
	if namespace == "" {
		return nil, fmt.Errorf("--namespace is required with --sidecar")
	}
	options := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{common.InjectSidecarDone: common.True}).String(),
	}
	if nodeName != "" {
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func loadSidecarTargetResources(ctx context.Context, client kubernetes.Interface, namespace string) (sidecar.TargetResources, error) {
	if namespace == "" {
		return sidecar.TargetResources{}, fmt.Errorf("--namespace is required with --sidecar")
	}
	pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return sidecar.TargetResources{}, err
	}
	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return sidecar.TargetResources{}, err
	}
	secrets, err := client.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return sidecar.TargetResources{}, err
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return sidecar.TargetResources{}, err
	}

	resources := sidecar.TargetResources{
		PVCs:    make(map[string]corev1.PersistentVolumeClaim, len(pvcs.Items)),
		PVs:     make(map[string]corev1.PersistentVolume, len(pvs.Items)),
		Secrets: make(map[types.NamespacedName]corev1.Secret, len(secrets.Items)),
		Nodes:   make(map[string]corev1.Node, len(nodes.Items)),
	}
	for _, pvc := range pvcs.Items {
		resources.PVCs[pvc.Name] = pvc
	}
	for _, pv := range pvs.Items {
		resources.PVs[pv.Name] = pv
	}
	for _, secret := range secrets.Items {
		resources.Secrets[types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}] = secret
	}
	for _, node := range nodes.Items {
		resources.Nodes[node.Name] = node
	}
	return resources, nil
}

func sidecarContainerNames(pod *corev1.Pod) []string {
	if pod == nil || pod.Labels[common.InjectSidecarDone] != common.True {
		return nil
	}
	names := make([]string, 0)
	for _, container := range pod.Spec.Containers {
		if jConfig.IsSidecarContainerName(container.Name) {
			names = append(names, container.Name)
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways && jConfig.IsSidecarContainerName(container.Name) {
			names = append(names, container.Name)
		}
	}
	return names
}

func resolveSidecarUpgradeTargets(pods []corev1.Pod, resources sidecar.TargetResources) (eligible, skipped []jConfig.UpgradeTarget, err error) {
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil || !sidecarPodReady(pod) {
			continue
		}
		podEligible, podSkipped, err := sidecar.ResolveUpgradeTargets(pod, sidecarContainerNames(pod), resources)
		if err != nil {
			return nil, nil, err
		}
		eligible = append(eligible, podEligible...)
		skipped = append(skipped, podSkipped...)
	}
	return eligible, skipped, nil
}

func sidecarPodReady(pod *corev1.Pod) bool {
	ready, containersReady := false, false
	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case corev1.PodReady:
			ready = true
		case corev1.ContainersReady:
			containersReady = true
		}
	}
	return ready && containersReady
}

func collectSidecarUpgradeDiffs(pod *corev1.Pod, resources sidecar.TargetResources) (map[string]jConfig.SidecarUpgradeDiff, error) {
	if pod == nil {
		return nil, fmt.Errorf("pod is not an injected JuiceFS sidecar pod")
	}
	if pod.Labels[common.InjectSidecarDone] != common.True {
		return nil, fmt.Errorf("pod %s/%s is not an injected JuiceFS sidecar pod", pod.Namespace, pod.Name)
	}
	diffs := make(map[string]jConfig.SidecarUpgradeDiff)
	for _, containerName := range sidecarContainerNames(pod) {
		container := sidecarContainer(pod, containerName)
		if container == nil {
			continue
		}
		currentImage, currentCmd, cmdKnown, err := sidecar.EffectiveSidecarState(pod, *container)
		if err != nil {
			return nil, err
		}
		target, err := sidecar.ResolveTargetStateFromObjects(pod, container.Name, resources)
		if err != nil {
			return nil, err
		}
		updateBinary, updateArgs := sidecar.RequiredUpdates(currentImage, currentCmd, cmdKnown, target)
		if !updateBinary && !updateArgs {
			continue
		}
		if updateArgs && !updateBinary && !jUtil.ImageSupportSidecarRestartArgs(container.Image) {
			continue
		}
		key := (&jConfig.UpgradeTarget{Namespace: pod.Namespace, Name: pod.Name, ContainerName: container.Name}).Key()
		diffs[key] = jConfig.SidecarUpgradeDiff{
			CurrentImage:     currentImage,
			TargetImage:      target.Image,
			CurrentMountArgs: mountArgs(currentCmd),
			TargetMountArgs:  mountArgs(target.Cmd),
		}
	}
	return diffs, nil
}

func sidecarContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

func mountArgs(cmd []string) []string {
	if len(cmd) <= 1 {
		return []string{}
	}
	return cmd[1:]
}

func formatMountArgs(args []string) string {
	if args == nil {
		args = []string{}
	}
	data, _ := json.Marshal(args)
	return string(data)
}
