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

	"github.com/juicedata/kubectl-jfs-plugin/pkg/debug"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

var debugCmd = &cobra.Command{
	Use:                   "debug <resource> <name>",
	Short:                 "Debug the pod/pv/pvc which is using juicefs",
	DisableFlagsInUseLine: true,
	Example: `  # debug the pod which is using juicefs pvc
  kubectl jfs debug po <pod-name> -n <namespace>

  # when juicefs csi driver is not in kube-system
  kubectl jfs debug po <pod-name> -n <namespace> -m <mount-namespace>

  # debug pvc using juicefs pv
  kubectl jfs debug pvc <pvc-name> -n <namespace>

  # debug pv which is juicefs pv
  kubectl jfs debug pv <pv-name>`,
	Run: func(cmd *cobra.Command, args []string) {
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error:", "please specify the resource")
			os.Exit(1)
		}
		ns, _ := RootCmd.Flags().GetString("namespace")
		if ns == "" {
			ns = "default"
		}
		resourceType := args[0]
		resourceName := args[1]
		cobra.CheckErr(debug.Debug(clientSet, ns, resourceType, resourceName))
	},
}

func init() {
	RootCmd.AddCommand(debugCmd)
}
