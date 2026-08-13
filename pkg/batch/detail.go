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
	"regexp"
	"time"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kdescribe "k8s.io/kubectl/pkg/describe"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (d *DiffAnalyzer) GetDetailOfJob(jobName string) error {
	job, err := util.GetJob(d.clientSet, jobName)
	if err != nil {
		return err
	}
	d.crtJob = job
	pod, err := util.GetPodOfUpgradeJob(d.clientSet, job)
	if err != nil {
		return err
	}
	d.podOfJob = pod
	conf, err := d.LoadUpgradeConfig(context.TODO(), job.Labels[common.JfsUpgradeConfig])
	if err != nil {
		return err
	}
	d.conf = conf
	d.pvc, err = d.getPVCOfUpgradeJob(conf)
	if err != nil {
		return err
	}
	total := 0
	for _, batch := range conf.Batches {
		total += len(batch)
	}
	d.total = total

	if err := d.record(); err != nil {
		return err
	}

	if err := d.generatePodsDiffOfConf(conf); err != nil {
		return err
	}

	output, err := d.describe()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", output)
	return nil
}

func (d *DiffAnalyzer) LoadUpgradeConfig(ctx context.Context, configName string) (*jConfig.BatchConfig, error) {
	if configName == "" {
		return nil, fmt.Errorf("config name is empty")
	}

	cm, err := d.clientSet.CoreV1().ConfigMaps(config.MountNamespace).Get(ctx, configName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return jConfig.LoadBatchConfig(cm)
}

func (d *DiffAnalyzer) getPVCOfUpgradeJob(conf *jConfig.BatchConfig) (*corev1.PersistentVolumeClaim, error) {
	if conf == nil {
		return nil, nil
	}
	for _, batch := range conf.Batches {
		for _, mountPod := range batch {
			if mountPod.Name == "" {
				continue
			}
			pod, err := d.clientSet.CoreV1().Pods(config.MountNamespace).Get(context.Background(), mountPod.Name, metav1.GetOptions{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, err
			}
			pvc, err := d.getPVCOfMountPod(context.Background(), pod)
			if err != nil {
				return nil, err
			}
			if pvc != nil {
				return pvc, nil
			}
		}
	}
	return nil, nil
}

func (d *DiffAnalyzer) record() error {
	d.clientSet.CoreV1().Pods(d.podOfJob.Namespace).GetLogs(d.podOfJob.Name, &corev1.PodLogOptions{})
	logs, err := d.clientSet.CoreV1().Pods(d.podOfJob.Namespace).GetLogs(d.podOfJob.Name, &corev1.PodLogOptions{
		Container: d.podOfJob.Spec.Containers[0].Name,
	}).DoRaw(context.TODO())
	if err != nil {
		return err
	}
	msg := string(logs)

	podsStatus := make(map[string]jConfig.UpgradeStatus)

	runningRegex := `POD-START \[([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)\]`
	runningRe := regexp.MustCompile(runningRegex)

	runningMatches := runningRe.FindAllStringSubmatch(msg, -1)
	for _, match := range runningMatches {
		podName := match[1]
		podsStatus[podName] = jConfig.Running
	}

	successRegex := `POD-SUCCESS \[([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)\]`
	successRe := regexp.MustCompile(successRegex)

	successMatches := successRe.FindAllStringSubmatch(msg, -1)
	for _, match := range successMatches {
		podName := match[1]
		podsStatus[podName] = jConfig.Success
		d.success++
	}

	failRegex := `POD-FAIL \[([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)\]`
	failRe := regexp.MustCompile(failRegex)

	failMatches := failRe.FindAllStringSubmatch(msg, -1)
	for _, match := range failMatches {
		podName := match[1]
		podsStatus[podName] = jConfig.Fail
	}

	for i := range d.conf.Batches {
		batch := d.conf.Batches[i]
		for j := range batch {
			po := batch[j]
			if status, ok := podsStatus[po.Name]; ok {
				d.conf.Batches[i][j].Status = status
			}
		}
	}
	return nil
}

func (d *DiffAnalyzer) describe() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "Name:\t%s\n", d.crtJob.Name)
		w.Write(kdescribe.LEVEL_0, "Namespace:\t%s\n", d.crtJob.Namespace)
		w.Write(kdescribe.LEVEL_0, "Start Time:\t%s\n", d.crtJob.CreationTimestamp.Time.Format(time.RFC1123Z))
		if d.crtJob.Status.CompletionTime != nil {
			w.Write(kdescribe.LEVEL_0, "Completed At:\t%s\n", d.crtJob.Status.CompletionTime.Time.Format(time.RFC1123Z))
		}
		w.Write(kdescribe.LEVEL_0, "Duration:\t%s\n", util.GetJobDuration(*d.crtJob))

		w.Write(kdescribe.LEVEL_0, "Status:\t%s\n", d.conf.Status)
		node := d.conf.Node
		if node == "" {
			node = "All Nodes"
		}
		w.Write(kdescribe.LEVEL_0, "Node:\t%s\n", node)
		w.Write(kdescribe.LEVEL_0, "Worker:\t%d\n", d.conf.Parallel)
		w.Write(kdescribe.LEVEL_0, "Ignore Error:\t%t\n", d.conf.IgnoreError)
		if d.pvc != nil {
			w.Write(kdescribe.LEVEL_0, "PVC:\t%s\n", d.pvc.Name)
		}
		w.Write(kdescribe.LEVEL_0, "Success:\t%s\n", fmt.Sprintf("%d/%d", d.success, d.total))

		cmd := fmt.Sprintf("kubectl logs %s -n %s", d.podOfJob.Name, config.MountNamespace)
		w.Write(kdescribe.LEVEL_0, "Get logs of job:\t%s\n", cmd)

		if d.total > 0 {
			w.Write(kdescribe.LEVEL_0, "Mount Pods Updated:\n")
			w.Write(kdescribe.LEVEL_1, "Name\tStatus\n")
			w.Write(kdescribe.LEVEL_1, "----\t------\n")
			for _, batch := range d.conf.Batches {
				for _, pod := range batch {
					w.Write(kdescribe.LEVEL_1, "%s\t%s\n", pod.Name, pod.Status)
				}
			}
		}

		return nil
	})
}
