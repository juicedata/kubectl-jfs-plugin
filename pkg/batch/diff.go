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
	"io"
	"os"
	"sort"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	"github.com/juicedata/juicefs-csi-driver/pkg/k8sclient"
	"github.com/juicedata/juicefs-csi-driver/pkg/util/resource"
	"github.com/sergi/go-diff/diffmatchpatch"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kdescribe "k8s.io/kubectl/pkg/describe"
	"sigs.k8s.io/yaml"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

type DiffAnalyzer struct {
	kubeConf  *rest.Config
	clientSet *kubernetes.Clientset
	k8sClient *k8sclient.K8sClient

	// used in list job
	jobs     []batchv1.Job
	confList map[string]*jConfig.BatchConfig

	// used in detail/upgrade/diff
	podsNeedToUpdate []corev1.Pod
	allPods          []corev1.Pod
	podDiffs         []dashboard.PodDiff
	pvcMap           map[string]*corev1.PersistentVolumeClaim

	// used in detail
	crtJob   *batchv1.Job
	podOfJob *corev1.Pod
	conf     *jConfig.BatchConfig
	pvc      *corev1.PersistentVolumeClaim
	total    int
	success  int
}

type PodDiffList []dashboard.PodDiff

func (p PodDiffList) Len() int {
	return len(p)
}

func (p PodDiffList) Less(i, j int) bool {
	return p[i].Pod.CreationTimestamp.Time.After(p[j].Pod.CreationTimestamp.Time)
}

func (p PodDiffList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

func NewDiffAnalyzer(clientSet *kubernetes.Clientset, conf *rest.Config) (*DiffAnalyzer, error) {
	// set global config in jConfig
	jConfig.Namespace = config.MountNamespace

	k8sClient, err := k8sclient.NewClientWithConfig(*conf)
	if err != nil {
		return nil, err
	}
	d := &DiffAnalyzer{
		kubeConf:  conf,
		clientSet: clientSet,
		k8sClient: k8sClient,
	}
	if err := d.loadGlobalConfig(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DiffAnalyzer) loadGlobalConfig() error {
	// load config
	csiNodes, err := util.GetCSINodeList(d.clientSet, "")
	if err != nil {
		return err
	}
	os.Setenv("JUICEFS_CONFIG_NAME", getEnvFromPod(&csiNodes[0], "JUICEFS_CONFIG_NAME", "juicefs-csi-driver-config"))

	return jConfig.LoadFromConfigMap(context.TODO(), d.k8sClient)
}

func (d *DiffAnalyzer) generatePodsDiff(nodeName, uniqueId string) error {
	// only get pods in batch job conf
	pods, err := util.ListMoundPods(d.clientSet, nodeName, uniqueId)
	if err != nil {
		return err
	}
	d.allPods = resource.FilterPodsToUpgrade(corev1.PodList{Items: pods}, true)

	return d._generatePodsDiff(true)
}

func (d *DiffAnalyzer) generatePodsDiffOfConf(conf *jConfig.BatchConfig) error {
	// only get pods in batch job conf
	pods, err := util.ListBatchPods(d.clientSet, conf)
	if err != nil {
		return err
	}
	d.allPods = pods
	return d._generatePodsDiff(false)
}

func (d *DiffAnalyzer) _generatePodsDiff(shouldDiff bool) error {
	// get pvc、pv、secret
	pvs, err := util.GetPVList(d.clientSet)
	if err != nil {
		return err
	}
	pvcs, err := util.GetPVCList(d.clientSet, "")
	if err != nil {
		return err
	}
	pvcMap := make(map[string]*corev1.PersistentVolumeClaim)
	for _, pvc := range pvcs {
		pvc2 := pvc
		pvcMap[pvc.Spec.VolumeName] = &pvc2
	}
	d.pvcMap = pvcMap

	secrets, err := util.GetSecretList(d.clientSet, "")
	if err != nil {
		return err
	}

	d.podsNeedToUpdate, d.podDiffs, err = dashboard.GenPodDiffs(d.allPods, shouldDiff, false, pvs, pvcs, secrets)
	sort.Sort(PodDiffList(d.podDiffs))
	return err
}

func (d *DiffAnalyzer) generatePodDiff(podName string) (*dashboard.PodDiff, error) {

	pod, err := d.clientSet.CoreV1().Pods(config.MountNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var (
		pvc        *corev1.PersistentVolumeClaim
		pv         *corev1.PersistentVolume
		custSecret *corev1.Secret
		pvcSecret  *corev1.Secret
	)
	pvc, err = d.getPVCByUniqueId(pod.Annotations[common.UniqueId])
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

	oldConfig, _, newConfig, _, err := jConfig.GetDiff(pod, pvc, pv, pvcSecret, custSecret)
	if err != nil {
		return nil, err
	}
	pd := &dashboard.PodDiff{
		Pod:       *pod,
		OldConfig: *oldConfig,
		NewConfig: *newConfig,
	}
	return pd, nil
}

func (d *DiffAnalyzer) ListDiffPods() error {
	if err := d.generatePodsDiff("", ""); err != nil {
		return err
	}
	out, err := d.printDiff()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (d *DiffAnalyzer) DiffPod(podName string) error {
	pd, err := d.generatePodDiff(podName)
	if err != nil {
		return err
	}

	oldText, err := yaml.Marshal(pd.OldConfig)
	if err != nil {
		return err
	}
	newText, err := yaml.Marshal(pd.NewConfig)
	if err != nil {
		return err
	}

	dmp := diffmatchpatch.New()

	diffs := dmp.DiffMain(string(oldText), string(newText), false)

	fmt.Printf("Config diff of pod [%s]:\n", podName)

	fmt.Println(dmp.DiffPrettyText(diffs))

	return nil
}

func (d *DiffAnalyzer) printDiff() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAME\tNAMESPACE\tPVC\tNODE\tSTATUS\tAGE\n")
		for _, diff := range d.podDiffs {
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\t%s\t%s\n", util.IfNil(diff.Pod.Name), util.IfNil(diff.Pod.Namespace), util.IfNil(d.pvcMap[diff.Pod.Labels[common.PodUniqueIdLabelKey]].Name), util.IfNil(diff.Pod.Spec.NodeName), util.IfNil(util.GetPodStatus(diff.Pod)), util.TranslateTimestampSince(diff.Pod.CreationTimestamp))
		}
		return nil
	})
}
