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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPrintJobDisplaysTypeAndFiltersSidecars(t *testing.T) {
	analyzer := &DiffAnalyzer{
		jobs: []batchv1.Job{
			{ObjectMeta: metav1.ObjectMeta{Name: "mount-job", Namespace: "kube-system"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "sidecar-job", Namespace: "kube-system"}},
		},
		confList: map[string]*jConfig.BatchConfig{
			"mount-job-config":   {Kind: jConfig.UpgradeKindMountPod},
			"sidecar-job-config": {Kind: jConfig.UpgradeKindSidecar},
		},
	}

	out, err := analyzer.printJob(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TYPE", "sidecar-job", "sidecar"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sidecar list missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "mount-job") {
		t.Fatalf("sidecar list contains mount pod job: %s", out)
	}

	out, err = analyzer.printJob(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mount-job", "mountPod", "sidecar-job", "sidecar"} {
		if !strings.Contains(out, want) {
			t.Fatalf("all job list missing %q: %s", want, out)
		}
	}
}
