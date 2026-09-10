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
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
)

func TestUpgradeOptionsValidateSidecar(t *testing.T) {
	tests := []struct {
		name string
		opts UpgradeOptions
		want string
	}{
		{
			name: "requires namespace",
			opts: UpgradeOptions{Sidecar: true},
			want: "--namespace is required with --sidecar",
		},
		{
			name: "rejects PVC filter",
			opts: UpgradeOptions{Sidecar: true, Namespace: "app", PVCName: "data"},
			want: "--pvc cannot be used with --sidecar",
		},
		{
			name: "requires positive worker count",
			opts: UpgradeOptions{Sidecar: true, Namespace: "app", Worker: 0},
			want: "--worker must be greater than zero with --sidecar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestUpgradeOptionsValidateMountPod(t *testing.T) {
	if err := (UpgradeOptions{PVCName: "data"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestSidecarTargetDisplayName(t *testing.T) {
	target := jConfig.UpgradeTarget{
		Namespace:     "app",
		Name:          "workload",
		ContainerName: "jfs-mount",
	}
	if got, want := sidecarTargetDisplayName(target), "app/workload/jfs-mount"; got != want {
		t.Fatalf("sidecarTargetDisplayName() = %q, want %q", got, want)
	}
}

func TestNewBatchConfigForSidecars(t *testing.T) {
	targets := []jConfig.UpgradeTarget{
		{Namespace: "app", Name: "workload", ContainerName: "jfs-mount-1", Node: "node-a"},
		{Namespace: "app", Name: "workload", ContainerName: "jfs-mount", Node: "node-a"},
	}
	got := jConfig.NewBatchConfigForSidecars(targets, 2, true, "app")

	if got.Kind != jConfig.UpgradeKindSidecar || got.Namespace != "app" || got.NoRecreate || !got.IgnoreError {
		t.Fatalf("batch config = %#v", got)
	}
	want := [][]jConfig.UpgradeTarget{{
		{Namespace: "app", Name: "workload", ContainerName: "jfs-mount", Node: "node-a"},
		{Namespace: "app", Name: "workload", ContainerName: "jfs-mount-1", Node: "node-a"},
	}}
	if !reflect.DeepEqual(got.Batches, want) {
		t.Fatalf("batches = %#v, want %#v", got.Batches, want)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal batch config: %v", err)
	}
	var persisted struct {
		Kind      jConfig.UpgradeKind       `json:"kind"`
		Namespace string                    `json:"namespace"`
		Batches   [][]jConfig.UpgradeTarget `json:"batches"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal batch config: %v", err)
	}
	if persisted.Kind != jConfig.UpgradeKindSidecar || persisted.Namespace != "app" {
		t.Fatalf("persisted config = %#v, want sidecar config in namespace app", persisted)
	}
	if got := persisted.Batches[0][0].ContainerName; got != "jfs-mount" {
		t.Fatalf("persisted target container = %q, want %q", got, "jfs-mount")
	}
}

func TestParseUpgradeStatusesKeepsSidecarContainerKeys(t *testing.T) {
	logs := `POD-SUCCESS [workload/jfs-mount]
POD-FAIL [workload/jfs-mount-1]`

	got, success := parseUpgradeStatuses(logs)
	want := map[string]jConfig.UpgradeStatus{
		"workload/jfs-mount":   jConfig.Success,
		"workload/jfs-mount-1": jConfig.Fail,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %#v, want %#v", got, want)
	}
	if success != 1 {
		t.Fatalf("success = %d, want 1", success)
	}
}

func TestResolveDashboardJobEnvironmentKeepsExplicitImage(t *testing.T) {
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "dashboard-sa",
					Containers: []corev1.Container{{
						Name:  "dashboard",
						Image: "registry.example/dashboard:old",
						Env: []corev1.EnvVar{{
							Name:  config.EnvJuicefsDashboardSA,
							Value: "dashboard-sa",
						}},
					}},
				},
			},
		},
	}

	image, serviceAccount := resolveDashboardJobEnvironment(
		deployment,
		"registry.example/dashboard:sidecar-supported",
		"",
	)

	if image != "registry.example/dashboard:sidecar-supported" {
		t.Fatalf("image = %q, want explicitly configured image", image)
	}
	if serviceAccount != "dashboard-sa" {
		t.Fatalf("service account = %q, want %q", serviceAccount, "dashboard-sa")
	}
}
