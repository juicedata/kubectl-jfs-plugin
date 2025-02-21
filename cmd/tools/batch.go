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

package tools

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/batch"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

var (
	nodeName    string
	ignoreError bool
	pvcName     string
	workerNum   int
	quiet       bool
)

var batchCmd = &cobra.Command{
	Use:                   "batch",
	Short:                 "create a job for batch upgrade",
	DisableFlagsInUseLine: true,
}

var batchDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "show all mount pods that can be upgraded, or show the config diff of a specific mount pod",
	Example: `  # show all mount pods that can be upgraded
  kubectl jfs batch diff

  # show the config diff of a specific mount pod
  kubectl jfs batch diff <pod-name>
`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)

		if len(args) < 1 {
			cobra.CheckErr(da.ListDiffPods())
		} else {
			cobra.CheckErr(da.DiffPod(args[0]))
		}
	},
}

var batchListCmd = &cobra.Command{
	Use:   "list",
	Short: "list all batch upgrade jobs",
	Example: `  # list all batch upgrade jobs
  kubectl jfs batch list`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.ListJobs())
	},
}

var batchUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "upgrade mount pods smoothly by batch",
	Example: `  # upgrade mount pods smoothly by batch
  kubectl jfs batch upgrade

  # upgrade mount pods only on specific node
  kubectl jfs batch upgrade --node <node-name>

  # upgrade mount pods only on specific pvc
  kubectl jfs batch upgrade --pvc <pvc-name>

  # concurrency of upgrade mount pods 
  kubectl jfs batch upgrade --worker <worker-number>

  # ignore error or not during upgrade
  kubectl jfs batch upgrade --ignore-error
`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.NewUpgradeJob(pvcName, nodeName, workerNum, ignoreError, quiet))
	},
}

var batchDetailCmd = &cobra.Command{
	Use:   "describe",
	Short: "show the detail of a batch upgrade job",
	Example: `  # show the detail of a batch upgrade job
  kubectl jfs batch describe <job-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify job name")
			os.Exit(1)
		}

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.GetDetailOfJob(args[0]))
	},
}

var batchPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "pause a batch upgrade job",
	Example: `  # pause a batch upgrade job
  kubectl jfs batch pause <job-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify job name")
			os.Exit(1)
		}

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.DoAction(args[0], "pause"))
	},
}

var batchResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "resume a paused batch upgrade job",
	Example: `  # resume a paused batch upgrade job
  kubectl jfs batch resume <job-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify job name")
			os.Exit(1)
		}

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.DoAction(args[0], "resume"))
	},
}

var batchStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "stop a batch upgrade job",
	Example: `  # stop a batch upgrade job
  kubectl jfs batch stop <job-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify job name")
			os.Exit(1)
		}

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.DoAction(args[0], "stop"))
	},
}

var batchDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "delete a batch upgrade job",
	Example: `  # delete a batch upgrade job
  kubectl jfs batch delete <job-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify job name")
			os.Exit(1)
		}

		da, err := batch.NewDiffAnalyzer(clientSet, conf)
		cobra.CheckErr(err)
		cobra.CheckErr(da.DoAction(args[0], "stop"))
	},
}

func init() {
	batchUpgradeCmd.Flags().StringVarP(&nodeName, "node", "", "", "node name")
	batchUpgradeCmd.Flags().BoolVarP(&ignoreError, "ignore-error", "", false, "ignore error")
	batchUpgradeCmd.Flags().StringVarP(&pvcName, "pvc", "", "", "pvc name")
	batchUpgradeCmd.Flags().IntVarP(&workerNum, "worker", "", 1, "worker number")
	batchUpgradeCmd.Flags().BoolVarP(&quiet, "quiet", "y", false, "quiet mode")
	batchCmd.AddCommand(batchDiffCmd)
	batchCmd.AddCommand(batchListCmd)
	batchCmd.AddCommand(batchUpgradeCmd)
	batchCmd.AddCommand(batchDetailCmd)
	batchCmd.AddCommand(batchPauseCmd)
	batchCmd.AddCommand(batchResumeCmd)
	batchCmd.AddCommand(batchStopCmd)
	batchCmd.AddCommand(batchDeleteCmd)
	RootCmd.AddCommand(batchCmd)
}
