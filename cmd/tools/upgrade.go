/*
 * Copyright 2024 Juicedata Inc
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

	"github.com/juicedata/kubectl-jfs-plugin/pkg/exec"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

var recreate bool

var upgradeCmd = &cobra.Command{
	Use:                   "upgrade <name>",
	Short:                 "upgrade mount pod smoothly",
	DisableFlagsInUseLine: true,
	Example: `  # upgrade juicefs mount pod binary 
  kubectl jfs upgrade <pod-name>
  
  # upgrade juicefs mount pod with it recreated
  kubectl jfs upgrade <pod-name> --recreate`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)

		eCli := exec.NewExecCli(clientSet, conf)
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify the mount pod name")
			os.Exit(1)
		}

		var (
			podName string
		)
		podName = args[0]
		cobra.CheckErr(eCli.Upgrade(podName, recreate))
	},
}

func init() {
	upgradeCmd.Flags().BoolVarP(&recreate, "recreate", "r", recreate, "recreate mount pod when upgrade")
	RootCmd.AddCommand(upgradeCmd)
}
