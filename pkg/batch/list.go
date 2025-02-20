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
	"fmt"
	"io"

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
	"github.com/juicedata/juicefs-csi-driver/pkg/dashboard"
	kdescribe "k8s.io/kubectl/pkg/describe"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

func (d *DiffAnalyzer) ListJobs() error {
	jobs, err := util.ListBatchJobs(d.clientSet)
	if err != nil {
		return err
	}
	d.jobs = jobs

	confList, err := util.ListUpgradeConfigs(d.clientSet)
	if err != nil {
		return err
	}
	d.confList = confList

	out, err := d.printJob()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)
	return nil
}

func (d *DiffAnalyzer) printJob() (string, error) {
	return util.TabbedString(func(out io.Writer) error {
		w := kdescribe.NewPrefixWriter(out)
		w.Write(kdescribe.LEVEL_0, "NAME\tNAMESPACE\tSTATUS\tDURATION\tAGE\n")
		for _, job := range d.jobs {
			status := jConfig.Pending
			if conf, ok := d.confList[dashboard.GenUpgradeConfig(job.Name)]; ok {
				status = conf.Status
			}
			w.Write(kdescribe.LEVEL_0, "%s\t%s\t%s\t%s\t%s\n", util.IfNil(job.Name), util.IfNil(job.Namespace), util.IfNil(string(status)), util.GetJobDuration(job), util.TranslateTimestampSince(job.CreationTimestamp))
		}
		return nil
	})
}
