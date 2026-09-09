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
	"strings"
	"testing"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestEnsureMountPodRejectsApplicationPod guards against the panic reported
// when `batch diff <pod-name>` (without --sidecar) is mistakenly pointed at
// an application pod (e.g. one with an injected sidecar) instead of a real
// juicefs-mount pod. Application pods don't carry a mount command, so the
// vendored parser used to index out of range; we now fail fast with a clear
// error instead.
func TestEnsureMountPodRejectsApplicationPod(t *testing.T) {
	appPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamic-ee-6744675bfb-v68j7",
			Namespace: "kube-system",
			// no app.kubernetes.io/name=juicefs-mount label: this is an
			// application pod, not a mount pod.
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "jfs-mount"}},
		},
	}

	err := ensureMountPod(appPod)
	if err == nil {
		t.Fatal("expected error for non-mount-pod, got nil")
	}
	if !strings.Contains(err.Error(), "not a juicefs mount pod") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEnsureMountPodAcceptsMountPod(t *testing.T) {
	mountPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juicefs-abc-pv-xyz",
			Namespace: "kube-system",
			Labels:    map[string]string{common.PodTypeKey: common.PodTypeValue},
		},
	}

	if err := ensureMountPod(mountPod); err != nil {
		t.Fatalf("expected no error for mount pod, got: %v", err)
	}
}
