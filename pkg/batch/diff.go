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

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	"github.com/juicedata/juicefs-csi-driver/pkg/k8sclient"
	"github.com/juicedata/juicefs-csi-driver/pkg/util/resource"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	sidecarTargets   []jConfig.UpgradeTarget

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
	configName, err := getGlobalConfigName(d.clientSet)
	if err != nil {
		return err
	}
	os.Setenv(config.EnvJuicefsConfigName, configName)

	return jConfig.LoadFromConfigMap(context.TODO(), d.k8sClient)
}

func getGlobalConfigName(clientSet kubernetes.Interface) (string, error) {
	daemonSet, err := clientSet.AppsV1().DaemonSets(config.MountNamespace).Get(context.Background(), "juicefs-csi-node", metav1.GetOptions{})
	if err != nil {
		return config.DefaultJuicefsConfigName, nil
	}
	return getEnvFromDaemonSet(daemonSet, config.EnvJuicefsConfigName, config.DefaultJuicefsConfigName), nil
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

func (d *DiffAnalyzer) ListSidecarDiffPods(namespace, nodeName string) error {
	if namespace == "" {
		return fmt.Errorf("--namespace is required with --sidecar")
	}

	targets, err := d.listSidecarUpgradeTargets(context.TODO(), namespace, nodeName)
	if err != nil {
		return err
	}
	targets, skippedTargets, err := jConfig.FilterTargetsNotInOngoingUpgrade(context.TODO(), d.k8sClient, targets)
	if err != nil {
		return err
	}
	if len(skippedTargets) > 0 {
		names := make([]string, 0, len(skippedTargets))
		for _, target := range skippedTargets {
			names = append(names, sidecarTargetDisplayName(target))
		}
		fmt.Printf("Skip %d sidecars already in ongoing upgrade jobs: %s\n", len(names), strings.Join(names, ", "))
	}

	d.sidecarTargets = targets
	out, err := d.printSidecarDiff()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (d *DiffAnalyzer) DiffSidecarPod(namespace, podName string) error {
	if namespace == "" {
		return fmt.Errorf("--namespace is required with --sidecar")
	}

	pod, err := d.clientSet.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	pvcs, err := d.clientSet.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	pvcMap := make(map[string]corev1.PersistentVolumeClaim, len(pvcs.Items))
	for _, pvc := range pvcs.Items {
		pvcMap[pvc.Name] = pvc
	}
	secretSelector := labels.SelectorFromSet(map[string]string{common.JuicefsSecretLabelKey: common.True})
	secrets, err := d.clientSet.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: secretSelector.String()})
	if err != nil {
		return err
	}
	secretMap := make(map[types.NamespacedName]corev1.Secret, len(secrets.Items))
	for _, secret := range secrets.Items {
		secretMap[types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}] = secret
	}

	out, err := printSidecarPodImageDiff(namespace, podName, jConfig.CollectSidecarImageDiffs([]corev1.Pod{*pod}, pvcMap, secretMap))
	if err != nil {
		return err
	}
	fmt.Print(out)
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

func (d *DiffAnalyzer) printSidecarDiff() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAMESPACE\tPOD\tCONTAINER\tNODE\n")
		for _, target := range d.sidecarTargets {
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\n", target.Namespace, target.Name, target.ContainerName, target.Node)
		}
		return nil
	})
}

func printSidecarPodImageDiff(namespace, podName string, imageDiffs map[string]jConfig.SidecarImageDiff) (string, error) {
	keys := make([]string, 0, len(imageDiffs))
	for key, imageDiff := range imageDiffs {
		if imageDiff.TargetImage != "" && imageDiff.CurrentImage != imageDiff.TargetImage {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no sidecar image differences found for pod %s/%s", namespace, podName)
	}
	sort.Strings(keys)

	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "Image diff of sidecars in pod [%s/%s]:\n", namespace, podName)
		w.Write(kdescribe.LEVEL_0, "CONTAINER\tCURRENT IMAGE\tTARGET IMAGE\n")
		for _, key := range keys {
			imageDiff := imageDiffs[key]
			containerName := strings.TrimPrefix(key, podName+"/")
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\n", containerName, imageDiff.CurrentImage, imageDiff.TargetImage)
		}
		return nil
	})
}

func getEnvFromDaemonSet(daemonSet *appsv1.DaemonSet, key string, defaultVal string) string {
	if daemonSet == nil || len(daemonSet.Spec.Template.Spec.Containers) == 0 {
		return defaultVal
	}
	for _, env := range daemonSet.Spec.Template.Spec.Containers[0].Env {
		if env.Name == key {
			return env.Value
		}
	}
	return defaultVal
}
