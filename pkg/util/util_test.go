/*
 * Copyright 2024 Juicedata Inc
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

package util

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetShareMountModes(t *testing.T) {
	pods := []corev1.Pod{
		{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Env: []corev1.EnvVar{{Name: "STORAGE_CLASS_SHARE_MOUNT", Value: "TRUE"}},
				}},
			},
		},
		{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Env: []corev1.EnvVar{{Name: "FS_SHARE_MOUNT", Value: "true"}},
				}},
			},
		},
		{
			Spec: corev1.PodSpec{},
		},
	}

	storageClassShareMount, fsShareMount := GetShareMountModes(pods)
	if !storageClassShareMount {
		t.Fatalf("expected storageClassShareMount to be true")
	}
	if !fsShareMount {
		t.Fatalf("expected fsShareMount to be true")
	}
}

func TestGetShareMountModes_NoMatch(t *testing.T) {
	pods := []corev1.Pod{
		{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Env: []corev1.EnvVar{{Name: "STORAGE_CLASS_SHARE_MOUNT", Value: "false"}},
				}},
			},
		},
		{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Env: []corev1.EnvVar{{Name: "FS_SHARE_MOUNT", Value: "false"}},
				}},
			},
		},
	}

	storageClassShareMount, fsShareMount := GetShareMountModes(pods)
	if storageClassShareMount || fsShareMount {
		t.Fatalf("expected both modes to be false, got storageClassShareMount=%v fsShareMount=%v", storageClassShareMount, fsShareMount)
	}
}

func TestIsShareMount(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Env: []corev1.EnvVar{{Name: "STORAGE_CLASS_SHARE_MOUNT", Value: "true"}},
			}},
		},
	}
	if !IsShareMount(pod) {
		t.Fatalf("expected IsShareMount to return true")
	}
	if IsShareMount(nil) {
		t.Fatalf("expected IsShareMount(nil) to return false")
	}
}

func TestListNodePodsByUID(t *testing.T) {
	clientSet := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-1",
				Namespace: "ns-a",
				UID:       types.UID("uid-a"),
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-2",
				Namespace: "ns-b",
				UID:       types.UID("uid-b"),
			},
			Spec: corev1.PodSpec{NodeName: "node-2"},
		},
	)

	podsByUID, err := ListNodePodsByUID(clientSet, "node-1")
	if err != nil {
		t.Fatalf("ListNodePodsByUID returned error: %v", err)
	}
	if len(podsByUID) != 1 {
		t.Fatalf("expected 1 pod on node-1, got %d", len(podsByUID))
	}
	pod := podsByUID["uid-a"]
	if pod == nil {
		t.Fatalf("expected uid-a in result")
	}
	if pod.Name != "app-1" {
		t.Fatalf("expected app-1, got %s", pod.Name)
	}
	if podsByUID["uid-b"] != nil {
		t.Fatalf("did not expect uid-b in result")
	}
}
