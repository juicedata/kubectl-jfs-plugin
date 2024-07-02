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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kdescribe "k8s.io/kubectl/pkg/describe"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

type PVAnalyzer struct {
	clientSet *kubernetes.Clientset
	pvs       []corev1.PersistentVolume
	pvcs      map[string]string
	pvShows   []pvShow
}

type pvShow struct {
	name     string
	status   string
	pvc      string
	sc       string
	createAt metav1.Time
}

func NewPVAnalyzer(clientSet *kubernetes.Clientset) (pa *PVAnalyzer, err error) {
	pa = &PVAnalyzer{
		clientSet: clientSet,
		pvs:       make([]corev1.PersistentVolume, 0),
		pvcs:      make(map[string]string),
	}
	pa.pvs, err = util.GetPVList(pa.clientSet)
	if err != nil {
		return nil, err
	}
	for _, pv := range pa.pvs {
		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver != config.DriverName {
			continue
		}
		if pv.Spec.ClaimRef != nil {
			pa.pvcs[pv.Name] = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
	}

	return pa, nil
}

func (pa *PVAnalyzer) ListPV() error {
	pvs := make([]pvShow, 0)
	for _, pv := range pa.pvs {
		pvs = append(pvs, pvShow{
			name:     pv.Name,
			status:   util.GetPVStatus(pv),
			pvc:      pa.pvcs[pv.Name],
			sc:       pv.Spec.StorageClassName,
			createAt: pv.CreationTimestamp,
		})
	}

	if len(pvs) == 0 {
		fmt.Println("No juicefs pv found")
		return nil
	}
	pa.pvShows = pvs

	out, err := pa.printPVs()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (pa *PVAnalyzer) printPVs() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAME\tCLAIM\tSTORAGECLASS\tSTATUS\tAGE\n")
		for _, pvc := range pa.pvShows {
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\t%s\n", pvc.name, pvc.pvc, pvc.sc, pvc.status, util.TranslateTimestampSince(pvc.createAt))
		}
		return nil
	})
}
