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

package list

import (
	"testing"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestJfsPodIncludesSidecarInjectedPod(t *testing.T) {
	analyzer := &AppAnalyzer{
		pods: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-sidecar",
					Namespace: "app",
					Labels:    map[string]string{common.InjectSidecarDone: common.True},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "app"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
		},
	}
	analyzer.collectJfsPods()

	if len(analyzer.apps) != 1 {
		t.Fatalf("listed %d Pods, want 1: %#v", len(analyzer.apps), analyzer.apps)
	}
	if got := analyzer.apps[0].name; got != "with-sidecar" {
		t.Fatalf("listed Pod = %q, want %q", got, "with-sidecar")
	}
	if got := len(analyzer.apps[0].mountPods); got != 0 {
		t.Fatalf("mount Pods = %d, want 0", got)
	}
}
