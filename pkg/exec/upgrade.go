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

package exec

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (e *ExecCli) Upgrade(podName string, recreate bool) (err error) {
	if !strings.HasPrefix(podName, "juicefs-") {
		return fmt.Errorf("pod %s is not juicefs mount pod\n", podName)
	}
	var pod *corev1.Pod
	if pod, err = e.clientSet.CoreV1().Pods(config.MountNamespace).Get(context.Background(), podName, metav1.GetOptions{}); err != nil {
		return err
	}

	if pod.Labels[config.PodTypeKey] != config.PodTypeValue {
		return fmt.Errorf("pod %s is not juicefs mount pod", podName)
	}

	var supported bool
	v := util.ParseClientVersion(pod.Spec.Containers[0].Image)
	if v.IsCe {
		supported = !v.LessThan(util.ClientVersion{Major: 1, Minor: 2, Patch: 0})
		if recreate {
			supported = !v.LessThan(util.ClientVersion{Major: 1, Minor: 2, Patch: 1})
		}
	} else {
		supported = !v.LessThan(util.ClientVersion{Major: 5, Minor: 0, Patch: 0})
		if recreate {
			supported = !v.LessThan(util.ClientVersion{Major: 5, Minor: 1, Patch: 0})
		}
	}
	if !supported {
		return fmt.Errorf("juicefs mount pod %s is not supported to upgrade: %s", podName, pod.Spec.Containers[0].Image)
	}

	var csiNode *corev1.Pod
	if csiNode, err = util.GetCSINode(e.clientSet, pod.Spec.NodeName); err != nil {
		return err
	}

	var cmds []string
	cmds = []string{"juicefs-csi-driver", "upgrade", pod.Name}
	if recreate {
		cmds = append(cmds, "--restart")
	}

	return e.Completion().
		SetNamespace(config.MountNamespace).
		SetPod(csiNode.Name).
		Container("juicefs-plugin").
		Commands(cmds).
		Run()
}
