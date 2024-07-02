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

func (e *ExecCli) AccessLog(podName string) (err error) {
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

	mountPath, _, err := util.GetMountPathOfPod(*pod)
	if err != nil {
		return fmt.Errorf("get mount path of pod %s error: %s\n", podName, err.Error())
	}
	return e.Completion().
		SetNamespace(config.MountNamespace).
		SetPod(podName).
		Container(config.MountContainerName).
		Commands([]string{"cat", fmt.Sprintf("%s/.accesslog", mountPath)}).
		Run()
}
