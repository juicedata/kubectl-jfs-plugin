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
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/k8sclient"
	"github.com/juicedata/juicefs-csi-driver/pkg/sidecar"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestSidecarContainerNames(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{common.InjectSidecarDone: common.True}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: common.MountContainerName},
			},
			InitContainers: []corev1.Container{
				{Name: "jfs-mount-1", RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways)},
				{Name: "jfs-mount-2"},
			},
		},
	}

	want := []string{"jfs-mount", "jfs-mount-1"}
	if got := sidecarContainerNames(pod); !reflect.DeepEqual(got, want) {
		t.Fatalf("sidecarContainerNames() = %#v, want %#v", got, want)
	}
}

func TestFormatMountArgs(t *testing.T) {
	args := []string{"--cache-dir", "/path with space", "", "quote\"value", "tab\tvalue", `back\slash`}
	want := `["--cache-dir","/path with space","","quote\"value","tab\tvalue","back\\slash"]`
	if got := formatMountArgs(args); got != want {
		t.Fatalf("formatMountArgs() = %q, want %q", got, want)
	}
	if got := formatMountArgs(nil); got != `[]` {
		t.Fatalf("formatMountArgs(nil) = %q, want []", got)
	}
	if got := formatMountArgs([]string{}); got != `[]` {
		t.Fatalf("formatMountArgs(empty) = %q, want []", got)
	}
}

func TestSidecarResourceAdapters(t *testing.T) {
	injectedA := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "injected-a", Namespace: "app", Labels: map[string]string{common.InjectSidecarDone: common.True}}, Spec: corev1.PodSpec{NodeName: "node-a"}}
	injectedB := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "injected-b", Namespace: "app", Labels: map[string]string{common.InjectSidecarDone: common.True}}, Spec: corev1.PodSpec{NodeName: "node-b"}}
	notInjected := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "app"}, Spec: corev1.PodSpec{NodeName: "node-a"}}
	client := fake.NewSimpleClientset(
		injectedA,
		injectedB,
		notInjected,
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "app"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "customer", Namespace: "juicefs"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
	)
	var fieldSelector string
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		fieldSelector = action.(k8stesting.ListAction).GetListRestrictions().Fields.String()
		return true, &corev1.PodList{Items: []corev1.Pod{*injectedA}}, nil
	})

	pods, err := listSidecarPods(context.Background(), client, "app", "node-a")
	if err != nil {
		t.Fatalf("listSidecarPods() error = %v", err)
	}
	if len(pods) != 1 || pods[0].Name != injectedA.Name {
		t.Fatalf("listSidecarPods() = %#v, want only %s", pods, injectedA.Name)
	}
	if fieldSelector != "spec.nodeName=node-a" {
		t.Fatalf("field selector = %q, want %q", fieldSelector, "spec.nodeName=node-a")
	}

	resources, err := loadSidecarTargetResources(context.Background(), client, "app")
	if err != nil {
		t.Fatalf("loadSidecarTargetResources() error = %v", err)
	}
	if _, ok := resources.PVCs["claim"]; !ok {
		t.Fatal("PVC claim missing from target resources")
	}
	if _, ok := resources.PVs["pv"]; !ok {
		t.Fatal("PV pv missing from target resources")
	}
	if _, ok := resources.Secrets[types.NamespacedName{Namespace: "juicefs", Name: "customer"}]; !ok {
		t.Fatal("Secret juicefs/customer missing from target resources")
	}
	if _, ok := resources.Nodes["node-a"]; !ok {
		t.Fatal("Node node-a missing from target resources")
	}
}

func TestLoadSidecarTargetResourcesPreservesForbidden(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("denied"))
	})

	resources, err := loadSidecarTargetResources(context.Background(), client, "app")
	if !apierrors.IsForbidden(err) {
		t.Fatalf("loadSidecarTargetResources() error = %v, want Forbidden", err)
	}
	if resources.PVCs != nil || resources.PVs != nil || resources.Secrets != nil || resources.Nodes != nil {
		t.Fatalf("loadSidecarTargetResources() returned usable partial snapshot: %#v", resources)
	}
}

func TestResolveSidecarUpgradeTargetsArgumentsOnlyEligibilityAndIdentity(t *testing.T) {
	oldConfig := jConfig.GlobalConfig
	jConfig.GlobalConfig = &jConfig.Config{MountPodPatch: []jConfig.MountPodPatch{{EEMountImage: "juicedata/mount:ee-5.4.3"}}}
	t.Cleanup(func() { jConfig.GlobalConfig = oldConfig })

	tests := []struct {
		name           string
		containerImage string
		effectiveImage string
		wantEligible   int
		wantSkipped    int
	}{
		{name: "supported args only", containerImage: "juicedata/mount:ee-5.4.3", effectiveImage: "juicedata/mount:ee-5.4.3", wantEligible: 1},
		{name: "unsupported args only", containerImage: "juicedata/mount:ee-5.3.13", effectiveImage: "juicedata/mount:ee-5.4.3", wantSkipped: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, resources := newSidecarFixture(t, tt.containerImage, tt.effectiveImage, []string{"/sbin/mount.juicefs", "stale"}, false)
			eligible, skipped, err := resolveSidecarUpgradeTargets([]corev1.Pod{*pod}, resources)
			if err != nil {
				t.Fatalf("resolveSidecarUpgradeTargets() error = %v", err)
			}
			if len(eligible) != tt.wantEligible || len(skipped) != tt.wantSkipped {
				t.Fatalf("eligible/skipped = %d/%d, want %d/%d", len(eligible), len(skipped), tt.wantEligible, tt.wantSkipped)
			}
			if len(eligible) == 0 {
				return
			}
			target := eligible[0]
			if target.PodUID != types.UID("uid-1") || target.ContainerID != "containerd://current" {
				t.Fatalf("target identity = %#v, want PodUID and ContainerID preserved", target)
			}

			data, err := json.Marshal(jConfig.NewBatchConfigForSidecars(eligible, 1, false, "app"))
			if err != nil {
				t.Fatalf("marshal batch config: %v", err)
			}
			var persisted jConfig.BatchConfig
			if err := json.Unmarshal(data, &persisted); err != nil {
				t.Fatalf("unmarshal batch config: %v", err)
			}
			got := persisted.Batches[0][0]
			if got.PodUID != types.UID("uid-1") || got.ContainerID != "containerd://current" {
				t.Fatalf("persisted target identity = %#v", got)
			}
		})
	}
}

func TestCollectSidecarUpgradeDiffs(t *testing.T) {
	oldConfig := jConfig.GlobalConfig
	jConfig.GlobalConfig = &jConfig.Config{MountPodPatch: []jConfig.MountPodPatch{{CEMountImage: "juicedata/mount:ce-1.3.0", EEMountImage: "juicedata/mount:ee-5.4.3"}}}
	t.Cleanup(func() { jConfig.GlobalConfig = oldConfig })

	basePod, resources := newSidecarFixture(t, "juicedata/mount:ee-5.4.3", "juicedata/mount:ee-5.4.3", []string{"stale"}, false)
	target, err := sidecar.ResolveTargetStateFromObjects(basePod, common.MountContainerName, resources)
	if err != nil {
		t.Fatalf("resolve target state: %v", err)
	}
	staleCmd := []string{"/sbin/mount.juicefs", "stale"}

	tests := []struct {
		name             string
		containerImage   string
		effectiveImage   string
		effectiveCommand []string
		isCE             bool
		wantDiff         bool
		wantCurrentImage string
	}{
		{name: "image only", containerImage: "juicedata/mount:ee-5.4.3", effectiveImage: "juicedata/mount:ee-5.3.13", effectiveCommand: target.Cmd, wantDiff: true, wantCurrentImage: "juicedata/mount:ee-5.3.13"},
		{name: "CE image only", containerImage: "juicedata/mount:ce-1.2.0", effectiveImage: "juicedata/mount:ce-1.2.0", effectiveCommand: staleCmd, isCE: true, wantDiff: true, wantCurrentImage: "juicedata/mount:ce-1.2.0"},
		{name: "supported arguments only", containerImage: "juicedata/mount:ee-5.4.3", effectiveImage: target.Image, effectiveCommand: staleCmd, wantDiff: true, wantCurrentImage: target.Image},
		{name: "combined", containerImage: "juicedata/mount:ee-5.4.3", effectiveImage: "juicedata/mount:ee-5.3.13", effectiveCommand: staleCmd, wantDiff: true, wantCurrentImage: "juicedata/mount:ee-5.3.13"},
		{name: "unsupported arguments only", containerImage: "juicedata/mount:ee-5.3.13", effectiveImage: target.Image, effectiveCommand: staleCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, resources := newSidecarFixture(t, tt.containerImage, tt.effectiveImage, tt.effectiveCommand, tt.isCE)
			diffs, err := collectSidecarUpgradeDiffs(pod, resources)
			if err != nil {
				t.Fatalf("collectSidecarUpgradeDiffs() error = %v", err)
			}
			if !tt.wantDiff {
				if len(diffs) != 0 {
					t.Fatalf("collectSidecarUpgradeDiffs() = %#v, want empty", diffs)
				}
				return
			}
			key := (&jConfig.UpgradeTarget{Namespace: "app", Name: "workload", ContainerName: common.MountContainerName}).Key()
			diff, ok := diffs[key]
			if !ok {
				t.Fatalf("diff key %q missing from %#v", key, diffs)
			}
			wantTargetImage := target.Image
			wantTargetArgs := target.Cmd[1:]
			if tt.isCE {
				wantTargetImage = "juicedata/mount:ce-1.3.0"
				wantTargetArgs = tt.effectiveCommand[1:]
			}
			if diff.CurrentImage != tt.wantCurrentImage || diff.TargetImage != wantTargetImage {
				t.Fatalf("image diff = %#v", diff)
			}
			if !reflect.DeepEqual(diff.CurrentMountArgs, tt.effectiveCommand[1:]) || !reflect.DeepEqual(diff.TargetMountArgs, wantTargetArgs) {
				t.Fatalf("mount args diff = %#v, want current %#v target %#v", diff, tt.effectiveCommand[1:], wantTargetArgs)
			}
		})
	}

	nonInjected := basePod.DeepCopy()
	nonInjected.Labels = nil
	if _, err := collectSidecarUpgradeDiffs(nonInjected, resources); err == nil || !strings.Contains(err.Error(), "not an injected JuiceFS sidecar pod") {
		t.Fatalf("non-injected Pod error = %v", err)
	}
}

func TestFilterSidecarTargetsInOngoingUpgrade(t *testing.T) {
	oldNamespace := jConfig.Namespace
	jConfig.Namespace = "kube-system"
	t.Cleanup(func() { jConfig.Namespace = oldNamespace })

	conflicting := jConfig.UpgradeTarget{Namespace: "app", Name: "workload-a", ContainerName: common.MountContainerName}
	eligibleTarget := jConfig.UpgradeTarget{Namespace: "app", Name: "workload-b", ContainerName: common.MountContainerName}
	batchConfig := jConfig.NewBatchConfigForSidecars([]jConfig.UpgradeTarget{conflicting}, 1, false, "app")
	data, err := json.Marshal(batchConfig)
	if err != nil {
		t.Fatalf("marshal ongoing sidecar batch config: %v", err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ongoing-sidecar-upgrade-config",
			Namespace: jConfig.Namespace,
			Labels:    map[string]string{common.PodTypeKey: common.ConfigTypeValue},
		},
		Data: map[string]string{"upgrade": string(data)},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ongoing-sidecar-upgrade",
			Namespace: jConfig.Namespace,
			Labels: map[string]string{
				common.JfsJobKind:       common.KindOfUpgrade,
				common.JfsUpgradeConfig: configMap.Name,
			},
		},
	}
	client := &k8sclient.K8sClient{Interface: fake.NewSimpleClientset(configMap, job)}

	eligible, skipped, err := jConfig.FilterTargetsNotInOngoingUpgrade(
		context.Background(), client, []jConfig.UpgradeTarget{conflicting, eligibleTarget},
	)
	if err != nil {
		t.Fatalf("FilterTargetsNotInOngoingUpgrade() error = %v", err)
	}
	if !reflect.DeepEqual(eligible, []jConfig.UpgradeTarget{eligibleTarget}) {
		t.Fatalf("eligible = %#v, want %#v", eligible, []jConfig.UpgradeTarget{eligibleTarget})
	}
	if !reflect.DeepEqual(skipped, []jConfig.UpgradeTarget{conflicting}) {
		t.Fatalf("skipped = %#v, want %#v", skipped, []jConfig.UpgradeTarget{conflicting})
	}
}

func newSidecarFixture(t *testing.T, containerImage, effectiveImage string, effectiveCommand []string, isCE bool) (*corev1.Pod, sidecar.TargetResources) {
	t.Helper()
	setting := jConfig.JfsSetting{
		Name: "volume", Source: "volume", VolumeId: "volume-id", UniqueId: "unique-id", UUID: "uuid", IsCe: isCE,
		Envs: map[string]string{}, Configs: map[string]string{},
		Attr: &jConfig.PodAttr{Namespace: "app", Image: containerImage, Labels: map[string]string{}, Annotations: map[string]string{}},
	}
	raw, err := json.Marshal(setting)
	if err != nil {
		t.Fatalf("marshal setting: %v", err)
	}
	annotation, err := json.Marshal(map[string]jConfig.SidecarBinaryUpgradeInfo{
		common.MountContainerName: {Image: effectiveImage, Cmd: effectiveCommand, ContainerID: "containerd://current"},
	})
	if err != nil {
		t.Fatalf("marshal upgrade annotation: %v", err)
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workload", Namespace: "app", UID: types.UID("uid-1"),
			Labels:      map[string]string{common.InjectSidecarDone: common.True},
			Annotations: map[string]string{common.SidecarBinaryUpgradeAnnotationKey: string(annotation)},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Volumes:  []corev1.Volume{{Name: "jfs-scripts", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "settings"}}}},
			Containers: []corev1.Container{{
				Name: common.MountContainerName, Image: containerImage,
				Command:      []string{"sh", "-c", "exec /sbin/mount.juicefs volume /jfs/actual -o foreground,no-update"},
				VolumeMounts: []corev1.VolumeMount{{Name: "jfs-scripts", MountPath: "/jfs-scripts"}},
			}},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{{Name: common.MountContainerName, ContainerID: "containerd://current"}},
		},
	}
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Namespace: "app"},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pv-1",
			Resources:  corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
		},
	}
	pv := corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}}
	settings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "settings", Namespace: "app",
			OwnerReferences: []metav1.OwnerReference{{Kind: "PersistentVolumeClaim", Name: "pvc-1"}},
		},
		Data: map[string][]byte{"jfsSettings": raw},
	}
	resources := sidecar.TargetResources{
		PVCs:    map[string]corev1.PersistentVolumeClaim{"pvc-1": pvc},
		PVs:     map[string]corev1.PersistentVolume{"pv-1": pv},
		Secrets: map[types.NamespacedName]corev1.Secret{{Namespace: "app", Name: "settings"}: settings},
		Nodes:   map[string]corev1.Node{"node-a": {ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}},
	}
	return pod, resources
}
