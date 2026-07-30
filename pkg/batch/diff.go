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
	"strings"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	"github.com/juicedata/juicefs-csi-driver/pkg/k8sclient"
	"github.com/juicedata/juicefs-csi-driver/pkg/util/resource"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kdescribe "k8s.io/kubectl/pkg/describe"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

type DiffAnalyzer struct {
	kubeConf  *rest.Config
	clientSet *kubernetes.Clientset
	k8sClient *k8sclient.K8sClient

	// used in list job
	jobs            []batchv1.Job
	confList        map[string]*jConfig.BatchConfig
	currentNodeName string

	// used in detail/upgrade/diff
	podsNeedToUpdate []corev1.Pod
	allPods          []corev1.Pod
	podDiffs         []dashboard.PodDiff

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
	d.currentNodeName = nodeName
	// only get pods in batch job conf
	pods, err := util.ListMoundPods(d.clientSet, nodeName, uniqueId)
	if err != nil {
		return err
	}
	d.allPods = resource.FilterPodsToUpgrade(corev1.PodList{Items: pods}, true)

	return d._generatePodsDiff(true)
}

func (d *DiffAnalyzer) generatePodsDiffOfConf(conf *jConfig.BatchConfig) error {
	d.currentNodeName = ""
	// only get pods in batch job conf
	pods, err := util.ListBatchPods(d.clientSet, conf)
	if err != nil {
		return err
	}
	d.allPods = pods
	return d._generatePodsDiff(false)
}

func (d *DiffAnalyzer) _generatePodsDiff(shouldDiff bool) error {
	// get pvc、pv
	pvs, err := util.GetPVList(d.clientSet)
	if err != nil {
		return err
	}
	pvcs, err := util.GetPVCList(d.clientSet, "")
	if err != nil {
		return err
	}
	var secrets []corev1.Secret
	secrets, err = util.GetSecretList(d.clientSet, "")
	if err != nil {
		return err
	}

	nodeMap, err := d.buildNodeMap(d.allPods)
	if err != nil {
		return err
	}

	d.podsNeedToUpdate, d.podDiffs, err = dashboard.GenPodDiffs(d.allPods, shouldDiff, pvs, pvcs, secrets, nodeMap)
	sort.Sort(PodDiffList(d.podDiffs))
	return err
}

func (d *DiffAnalyzer) buildNodeMap(pods []corev1.Pod) (map[string]*corev1.Node, error) {
	if d.currentNodeName != "" {
		nodeMap := make(map[string]*corev1.Node, 1)
		node, err := d.clientSet.CoreV1().Nodes().Get(context.Background(), d.currentNodeName, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				nodeMap[d.currentNodeName] = nil
				return nodeMap, nil
			}
			return nil, err
		}
		nodeMap[d.currentNodeName] = node
		return nodeMap, nil
	}

	nodes, err := d.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	allNodes := make(map[string]*corev1.Node, len(nodes.Items))
	for i := range nodes.Items {
		node := &nodes.Items[i]
		allNodes[node.Name] = node
	}

	nodeMap := make(map[string]*corev1.Node)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		if _, ok := nodeMap[pod.Spec.NodeName]; ok {
			continue
		}
		nodeMap[pod.Spec.NodeName] = allNodes[pod.Spec.NodeName]
	}
	return nodeMap, nil
}

func (d *DiffAnalyzer) ListDiffPods(nodeName string) error {
	if err := d.generatePodsDiff(nodeName, ""); err != nil {
		return err
	}
	skippedPods, err := d.filterPodsInOngoingUpgradeJobs()
	if err != nil {
		return err
	}
	if len(skippedPods) > 0 {
		fmt.Printf("Skip %d pods already in ongoing upgrade jobs: %s\n", len(skippedPods), strings.Join(skippedPods, ", "))
	}
	out, err := d.printDiff()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (d *DiffAnalyzer) printDiff() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAME\tNAMESPACE\tNODE\tSTATUS\tAGE\n")
		for _, diff := range d.podDiffs {
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\t%s\n", util.IfNil(diff.Pod.Name), util.IfNil(diff.Pod.Namespace), util.IfNil(diff.Pod.Spec.NodeName), util.IfNil(util.GetPodStatus(diff.Pod)), util.TranslateTimestampSince(diff.Pod.CreationTimestamp))
		}
		return nil
	})
}
