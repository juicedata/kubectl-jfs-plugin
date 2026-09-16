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

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDescribeShowsSidecarTypeAndTargetNamespace(t *testing.T) {
	analyzer := &DiffAnalyzer{
		crtJob: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "sidecar-job", Namespace: "kube-system"},
		},
		podOfJob: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "sidecar-job-pod"},
		},
		conf: &jConfig.BatchConfig{
			Kind:      jConfig.UpgradeKindSidecar,
			Namespace: "application",
		},
	}

	out, err := analyzer.describe()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Type:", "sidecar", "Target Namespace:", "application"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail output missing %q: %s", want, out)
		}
	}
}
