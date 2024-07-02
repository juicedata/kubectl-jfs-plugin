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

package tools

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/exec"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

var accesslogCmd = &cobra.Command{
	Use:                   "accesslog <name>",
	Short:                 "collect access log from mount pod",
	DisableFlagsInUseLine: true,
	Example: `  # collect access log from mount pod
  kubectl jfs accesslog <pod-name>

  # when juicefs csi driver is not in kube-system
  kubectl jfs accesslog <pod-name> -m <mount-namespace>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		conf, err := KubernetesConfigFlags.ToRESTConfig()
		cobra.CheckErr(err)
		eCli := exec.NewExecCli(clientSet, conf)

		cmd.Flags().BoolVarP(&eCli.Stdin, "stdin", "i", eCli.Stdin, "Pass stdin to the container")
		cmd.Flags().BoolVarP(&eCli.TTY, "tty", "t", eCli.TTY, "Stdin is a TTY")
		cmd.Flags().BoolVarP(&eCli.Quiet, "quiet", "q", eCli.Quiet, "Only print output from the remote session")
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify the mount pod name")
			os.Exit(1)
		}

		podName := args[0]
		cobra.CheckErr(eCli.AccessLog(podName))
	},
}

func init() {
	RootCmd.AddCommand(accesslogCmd)
}
