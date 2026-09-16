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
	"strings"
	"testing"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
)

func TestPrintSidecarDiff(t *testing.T) {
	analyzer := &DiffAnalyzer{
		sidecarTargets: []jConfig.UpgradeTarget{{
			Namespace:     "app",
			Name:          "workload",
			ContainerName: "jfs-mount",
			Node:          "node-a",
		}},
	}

	out, err := analyzer.printSidecarDiff()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAMESPACE", "POD", "CONTAINER", "NODE", "app", "workload", "jfs-mount", "node-a"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestPrintSidecarPodImageDiff(t *testing.T) {
	imageDiffs := map[string]jConfig.SidecarImageDiff{
		"workload/jfs-mount": {
			CurrentImage: "registry.example/mount:v1",
			TargetImage:  "registry.example/mount:v2",
		},
	}

	out, err := printSidecarPodImageDiff("app", "workload", imageDiffs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Image diff of sidecars in pod [app/workload]:",
		"CONTAINER",
		"CURRENT IMAGE",
		"TARGET IMAGE",
		"jfs-mount",
		"registry.example/mount:v1",
		"registry.example/mount:v2",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestGetGlobalConfigNameFallsBackToDaemonSet(t *testing.T) {
	oldNamespace := config.MountNamespace
	config.MountNamespace = "kube-system"
	t.Cleanup(func() {
		config.MountNamespace = oldNamespace
	})

	clientSet := fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "juicefs-csi-node",
				Namespace: "kube-system",
			},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "juicefs-plugin",
							Env: []corev1.EnvVar{
								{Name: "JUICEFS_CONFIG_NAME", Value: "config-from-ds"},
							},
						}},
					},
				},
			},
		},
	)

	got, err := getGlobalConfigName(clientSet)
	if err != nil {
		t.Fatalf("getGlobalConfigName returned error: %v", err)
	}
	if got != "config-from-ds" {
		t.Fatalf("getGlobalConfigName mismatch, got %q want %q", got, "config-from-ds")
	}
}
