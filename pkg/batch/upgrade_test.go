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
	"reflect"
	"testing"

	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterPodDiffsByPodNames(t *testing.T) {
	diffs := []dashboard.PodDiff{
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-a")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-b")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-c")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-d")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-e")}},
	}
	filteredDiffs := filterPodDiffsByPodNames(diffs, []string{"pod-a", "pod-b"})
	if got, want := diffNames(filteredDiffs), []string{"pod-c", "pod-d", "pod-e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered diffs mismatch, got %v want %v", got, want)
	}
}

func TestFilterPodDiffsByPodNames_NoSkip(t *testing.T) {
	diffs := []dashboard.PodDiff{{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-a")}}, {Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-b")}}}
	filteredDiffs := filterPodDiffsByPodNames(diffs, nil)
	if got, want := diffNames(filteredDiffs), []string{"pod-a", "pod-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered diffs mismatch, got %v want %v", got, want)
	}
}

func TestUniqueIdFromPV(t *testing.T) {
	pv := &corev1.PersistentVolume{
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{VolumeHandle: "volume-handle"},
			},
			StorageClassName: "sc-name",
		},
	}
	secret := &corev1.Secret{Data: map[string][]byte{"name": []byte("fs-name")}}

	if got := uniqueIdFromPV(pv, true, false, nil); got != "sc-name" {
		t.Fatalf("storage class share unique id mismatch, got %q", got)
	}
	if got := uniqueIdFromPV(pv, false, true, secret); got != "fs-name" {
		t.Fatalf("fs share unique id mismatch, got %q", got)
	}
	if got := uniqueIdFromPV(pv, false, false, nil); got != "volume-handle" {
		t.Fatalf("default unique id mismatch, got %q", got)
	}
}

func diffNames(diffs []dashboard.PodDiff) []string {
	names := make([]string, 0, len(diffs))
	for _, diff := range diffs {
		names = append(names, diff.Pod.Name)
	}
	return names
}

func metav1Obj(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}
