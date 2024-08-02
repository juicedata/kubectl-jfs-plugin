/*
 Copyright 2024 Juicedata Inc

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package list

import (
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kdescribe "k8s.io/kubectl/pkg/describe"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

type PVCAnalyzer struct {
	clientSet *kubernetes.Clientset
	ns        string
	pvcs      []corev1.PersistentVolumeClaim
	pvs       map[string]corev1.PersistentVolume
	scs       map[string]storagev1.StorageClass

	pvcShows []pvcShow
}

type pvcShow struct {
	name      string
	namespace string
	status    string
	pv        string
	sc        string
	createAt  metav1.Time
}

func NewPVCAnalyzer(clientSet *kubernetes.Clientset, ns string) (pa *PVCAnalyzer, err error) {
	pa = &PVCAnalyzer{
		clientSet: clientSet,
		pvs:       map[string]corev1.PersistentVolume{},
		scs:       map[string]storagev1.StorageClass{},
	}
	var (
		pvList = make([]corev1.PersistentVolume, 0)
		scList = make([]storagev1.StorageClass, 0)
	)
	if pa.pvcs, err = util.GetPVCList(clientSet, ns); err != nil {
		return
	}
	if scList, err = util.GetStorageClassList(clientSet); err != nil {
		return
	}
	for _, sc := range scList {
		pa.scs[sc.Name] = sc
	}
	if pvList, err = util.GetPVList(clientSet); err != nil {
		return
	}
	for _, pv := range pvList {
		pa.pvs[pv.Name] = pv
	}
	return
}

func (pa *PVCAnalyzer) ListPVC() error {
	pvcs := make([]pvcShow, 0)
	for _, pvc := range pa.pvcs {
		var (
			appending bool
			pv        corev1.PersistentVolume
			scName    string
		)

		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
			if sc, ok := pa.scs[scName]; ok {
				if sc.Provisioner != config.DriverName {
					continue
				}
				appending = true
			}
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			appending = true
		}
		if pvc.Spec.VolumeName != "" {
			pv = pa.pvs[pvc.Spec.VolumeName]
			if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == config.DriverName {
				appending = true
			}
		}
		if appending {
			ps := pvcShow{
				name:      pvc.Name,
				namespace: pvc.Namespace,
				status:    string(pvc.Status.Phase),
				pv:        pv.Name,
				sc:        scName,
				createAt:  pvc.CreationTimestamp,
			}
			pvcs = append(pvcs, ps)
		}
	}
	if len(pvcs) == 0 {
		fmt.Printf("No juicefs pvc found in namespace %s\n", pa.ns)
		return nil
	}
	pa.pvcShows = pvcs

	out, err := pa.printPVCs()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (pa *PVCAnalyzer) printPVCs() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAME\tNAMESPACE\tVOLUME\tSTORAGECLASS\tSTATUS\tAGE\n")
		for _, pvc := range pa.pvcShows {
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\t%s\t%s\n", pvc.name, pvc.namespace, pvc.pv, pvc.sc, pvc.status, util.TranslateTimestampSince(pvc.createAt))
		}
		return nil
	})
}
