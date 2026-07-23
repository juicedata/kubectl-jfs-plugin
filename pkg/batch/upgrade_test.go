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

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterPodsByOngoingJobs(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1Obj("pod-a")},
		{ObjectMeta: metav1Obj("pod-b")},
		{ObjectMeta: metav1Obj("pod-c")},
		{ObjectMeta: metav1Obj("pod-d")},
		{ObjectMeta: metav1Obj("pod-e")},
	}
	diffs := []dashboard.PodDiff{
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-a")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-b")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-c")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-d")}},
		{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-e")}},
	}

	jobs := []batchv1.Job{
		{ObjectMeta: metav1ObjWithLabels("job-pending", map[string]string{common.JfsUpgradeConfig: "cfg-pending"})},
		{ObjectMeta: metav1ObjWithLabels("job-pause", map[string]string{common.JfsUpgradeConfig: "cfg-pause"})},
		{ObjectMeta: metav1ObjWithLabels("job-success", map[string]string{common.JfsUpgradeConfig: "cfg-success"})},
	}

	confList := map[string]*jConfig.BatchConfig{
		"cfg-pending": {
			Status: jConfig.Pending,
			Batches: [][]jConfig.MountPodUpgrade{{
				{Name: "pod-b"},
			}},
		},
		"cfg-pause": {
			Status: jConfig.Pause,
			Batches: [][]jConfig.MountPodUpgrade{{
				{Name: "pod-a"},
			}},
		},
		"cfg-success": {
			Status: jConfig.Success,
			Batches: [][]jConfig.MountPodUpgrade{{
				{Name: "pod-c"},
			}},
		},
	}

	filteredPods, filteredDiffs, skipped := filterPodsByOngoingJobs(pods, diffs, jobs, confList)

	if got, want := podNames(filteredPods), []string{"pod-c", "pod-d", "pod-e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered pods mismatch, got %v want %v", got, want)
	}
	if got, want := diffNames(filteredDiffs), []string{"pod-c", "pod-d", "pod-e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered diffs mismatch, got %v want %v", got, want)
	}
	if want := []string{"pod-a", "pod-b"}; !reflect.DeepEqual(skipped, want) {
		t.Fatalf("skipped pods mismatch, got %v want %v", skipped, want)
	}
}

func TestFilterPodsByOngoingJobs_NoOngoingJobs(t *testing.T) {
	pods := []corev1.Pod{{ObjectMeta: metav1Obj("pod-a")}, {ObjectMeta: metav1Obj("pod-b")}}
	diffs := []dashboard.PodDiff{{Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-a")}}, {Pod: corev1.Pod{ObjectMeta: metav1Obj("pod-b")}}}

	jobs := []batchv1.Job{
		{ObjectMeta: metav1ObjWithLabels("job-success", map[string]string{common.JfsUpgradeConfig: "cfg-success"})},
		{ObjectMeta: metav1ObjWithLabels("job-stop", map[string]string{common.JfsUpgradeConfig: "cfg-stop"})},
	}
	confList := map[string]*jConfig.BatchConfig{
		"cfg-success": {Status: jConfig.Success, Batches: [][]jConfig.MountPodUpgrade{{{Name: "pod-a"}}}},
		"cfg-stop":    {Status: jConfig.Stop, Batches: [][]jConfig.MountPodUpgrade{{{Name: "pod-b"}}}},
	}

	filteredPods, filteredDiffs, skipped := filterPodsByOngoingJobs(pods, diffs, jobs, confList)
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped pods, got %v", skipped)
	}
	if got, want := podNames(filteredPods), []string{"pod-a", "pod-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered pods mismatch, got %v want %v", got, want)
	}
	if got, want := diffNames(filteredDiffs), []string{"pod-a", "pod-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered diffs mismatch, got %v want %v", got, want)
	}
}

func TestIsUpgradeJobOngoing(t *testing.T) {
	cases := []struct {
		status jConfig.UpgradeStatus
		want   bool
	}{
		{status: jConfig.Pending, want: true},
		{status: jConfig.Running, want: true},
		{status: jConfig.Pause, want: true},
		{status: jConfig.Success, want: false},
		{status: jConfig.Fail, want: false},
		{status: jConfig.Stop, want: false},
	}

	for _, tc := range cases {
		if got := isUpgradeJobOngoing(tc.status); got != tc.want {
			t.Fatalf("status %q ongoing mismatch: got %v want %v", tc.status, got, tc.want)
		}
	}
}

func podNames(pods []corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
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

func metav1ObjWithLabels(name string, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Labels: labels}
}
