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
	"context"
	"fmt"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	dashboardutils "github.com/juicedata/juicefs-csi-driver/pkg/dashboard/utils"
	"github.com/sergi/go-diff/diffmatchpatch"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

const (
	storageClassShareMountMode = "storageClassShareMount"
	fsShareMountMode           = "fsShareMount"
)

func (d *DiffAnalyzer) DiffPod(podName string) error {
	pd, err := d.generatePodDiff(podName)
	if err != nil {
		return err
	}

	oldText, err := yaml.Marshal(pd.OldSetting)
	if err != nil {
		return err
	}
	newText, err := yaml.Marshal(pd.NewSetting)
	if err != nil {
		return err
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(oldText), string(newText), false)

	fmt.Printf("Config diff of pod [%s]:\n", podName)
	fmt.Println(dmp.DiffPrettyText(diffs))
	return nil
}

// ensureMountPod guards against the panic reported when `batch diff <pod-name>`
// (without --sidecar) is mistakenly pointed at an application pod (e.g. one
// with an injected sidecar) instead of a real juicefs-mount pod. Application
// pods don't carry a mount command, so the vendored parser used to index out
// of range; we now fail fast with a clear error instead.
func ensureMountPod(pod *corev1.Pod) error {
	if pod.Labels[common.PodTypeKey] != common.PodTypeValue {
		return fmt.Errorf("pod %s/%s is not a juicefs mount pod, use --sidecar if it is an application pod with an injected sidecar", pod.Namespace, pod.Name)
	}
	return nil
}

func (d *DiffAnalyzer) generatePodDiff(podName string) (*dashboard.PodDiff, error) {
	pod, err := d.clientSet.CoreV1().Pods(config.MountNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if err := ensureMountPod(pod); err != nil {
		return nil, err
	}

	var (
		pvc        *corev1.PersistentVolumeClaim
		pv         *corev1.PersistentVolume
		custSecret *corev1.Secret
		pvcSecret  *corev1.Secret
	)
	pvc, err = d.getPVCOfMountPod(context.Background(), pod)
	if err != nil {
		return nil, err
	}

	if pvc != nil {
		pv, err = util.GetPVOfPVC(d.clientSet, pvc)
		if err != nil {
			return nil, err
		}
	}
	if pv != nil && pv.Spec.CSI != nil && pv.Spec.CSI.NodePublishSecretRef != nil {
		secretName := pv.Spec.CSI.NodePublishSecretRef.Name
		secretNamespace := pv.Spec.CSI.NodePublishSecretRef.Namespace
		custSecret, err = d.clientSet.CoreV1().Secrets(secretNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		pvcSecretName := fmt.Sprintf("juicefs-%s-secret", pod.Annotations[common.UniqueId])
		pvcSecret, err = d.clientSet.CoreV1().Secrets(config.MountNamespace).Get(context.Background(), pvcSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	oldSetting, newSetting, err := jConfig.GetDiff(pod, pvc, pv, pvcSecret, custSecret)
	if err != nil {
		return nil, err
	}
	pd := &dashboard.PodDiff{
		Pod:        *pod,
		OldSetting: oldSetting,
		NewSetting: newSetting,
	}
	return pd, nil
}

func (d *DiffAnalyzer) getPVCOfMountPod(ctx context.Context, mountPod *corev1.Pod) (*corev1.PersistentVolumeClaim, error) {
	if mountPod == nil || mountPod.Spec.NodeName == "" {
		return nil, nil
	}

	mountPodUniqueID := mountPod.Labels[common.PodUniqueIdLabelKey]
	storageClassShareMount, fsShareMount := getMountPodShareModes(mountPod)

	podsByUID, err := util.ListNodePodsByUID(d.clientSet, mountPod.Spec.NodeName)
	if err != nil {
		return nil, err
	}

	for _, annotation := range mountPod.Annotations {
		targetUID := dashboardutils.GetTargetUID(annotation)
		if targetUID == "" {
			continue
		}
		appPod, ok := podsByUID[targetUID]
		if !ok {
			continue
		}
		for _, volume := range appPod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			pvc, err := d.clientSet.CoreV1().PersistentVolumeClaims(appPod.Namespace).Get(ctx, volume.PersistentVolumeClaim.ClaimName, metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, err
			}

			uniqueID, err := d.getUniqueIdOfPVCWithModes(pvc, storageClassShareMount, fsShareMount)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, err
			}
			if uniqueID == "" {
				continue
			}
			if uniqueID == mountPodUniqueID {
				return pvc, nil
			}
		}
	}

	return nil, nil
}

func (d *DiffAnalyzer) getUniqueIdOfPVCWithModes(pvc *corev1.PersistentVolumeClaim, storageClassShareMount, fsShareMount bool) (string, error) {
	pv, err := util.GetPVOfPVC(d.clientSet, pvc)
	if err != nil {
		return "", err
	}

	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != config.DriverName {
		return "", nil
	}

	var secret *corev1.Secret
	if fsShareMount && pv.Spec.CSI != nil && pv.Spec.CSI.NodePublishSecretRef != nil {
		secretName := pv.Spec.CSI.NodePublishSecretRef.Name
		secretNamespace := pv.Spec.CSI.NodePublishSecretRef.Namespace
		secret, err = d.clientSet.CoreV1().Secrets(secretNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
	}

	return uniqueIdFromPV(pv, storageClassShareMount, fsShareMount, secret), nil
}

func getMountPodShareModes(mountPod *corev1.Pod) (bool, bool) {
	if mountPod == nil {
		return false, false
	}
	switch mountPod.Annotations[common.JuicefsMountShareMode] {
	case storageClassShareMountMode:
		return true, false
	case fsShareMountMode:
		return false, true
	default:
		return false, false
	}
}
