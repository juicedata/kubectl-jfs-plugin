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
	"bytes"
	"context"
	"fmt"

	"github.com/juicedata/juicefs-csi-driver/pkg/common"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (d *DiffAnalyzer) DoAction(jobName, action string) error {
	job, err := util.GetJob(d.clientSet, jobName)
	if err != nil {
		return err
	}
	d.crtJob = job
	conf, err := d.LoadUpgradeConfig(context.TODO(), job.Labels[common.JfsUpgradeConfig])
	if err != nil {
		return err
	}
	d.conf = conf
	if !dashboard.CanDoAction(conf.Status, action) {
		return fmt.Errorf("cannot [%s] job when status is %s", action, conf.Status)
	}
	if action == "delete" {
		if err := d.clientSet.BatchV1().Jobs(job.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
		fmt.Printf("delete job %s successfully\n", jobName)
		return nil
	}

	pod, err := util.GetPodOfUpgradeJob(d.clientSet, job)
	if err != nil {
		return err
	}
	if err := d.doActionInUpgradeJob(context.TODO(), pod, action); err != nil {
		return err
	}
	fmt.Printf("%s job %s successfully\n", action, jobName)
	return nil
}

func (d *DiffAnalyzer) doActionInUpgradeJob(ctx context.Context, pod *corev1.Pod, action string) error {
	sig := "-SIGUSR1"
	if action == "stop" {
		sig = "-SIGTERM"
	}
	req := d.clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   []string{"kill", sig, "1"},
		Container: "juicefs-upgrade",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(d.kubeConf, "POST", req.URL())
	if err != nil {
		return err
	}
	var sout, serr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &sout,
		Stderr: &serr,
		Tty:    false,
	})
	if err != nil {
		return err
	}

	return err
}
