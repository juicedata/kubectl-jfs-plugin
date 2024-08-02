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
	"github.com/spf13/cobra"

	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/list"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/util"
)

var pvcCmd = &cobra.Command{
	Use:   "pvc",
	Short: "Show juicefs pvcs",
	Run: func(cmd *cobra.Command, args []string) {
		ns, _ := RootCmd.Flags().GetString("namespace")
		if ns == "" {
			ns = "default"
		}
		if config.AllNamespaces {
			ns = ""
		}
		clientSet, err := util.ClientSet(KubernetesConfigFlags)
		cobra.CheckErr(err)

		pa, err := list.NewPVCAnalyzer(clientSet, ns)
		cobra.CheckErr(err)
		cobra.CheckErr(pa.ListPVC())
	},
}

func init() {
	RootCmd.AddCommand(pvcCmd)
}
