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

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
)

func LoadFromConfigMap(clientSet *kubernetes.Clientset, csiNodes []corev1.Pod) error {
	cmName := "juicefs-csi-driver-config"
	if len(csiNodes) > 0 {
		for _, env := range csiNodes[0].Spec.Containers[0].Env {
			if env.Name == "JUICEFS_CONFIG_NAME" {
				cmName = env.Value
			}
		}
	}
	cm, err := clientSet.CoreV1().ConfigMaps(config.MountNamespace).Get(context.Background(), cmName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	cfg := &jConfig.Config{}

	err = cfg.Unmarshal([]byte(cm.Data["config.yaml"]))
	if err != nil {
		return err
	}

	jConfig.GlobalConfig = cfg
	return nil
}
