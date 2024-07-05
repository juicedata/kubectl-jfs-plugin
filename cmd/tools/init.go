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
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/juicedata/kubectl-jfs-plugin/pkg"
	"github.com/juicedata/kubectl-jfs-plugin/pkg/config"
)

var (
	KubernetesConfigFlags *genericclioptions.ConfigFlags
)

func init() {
	KubernetesConfigFlags = genericclioptions.NewConfigFlags(true)
	RootCmd.PersistentFlags().StringVarP(&config.MountNamespace, "mount-namespace", "m", "kube-system", "namespace of juicefs csi driver")
	KubernetesConfigFlags.AddFlags(RootCmd.PersistentFlags())
}

var RootCmd = &cobra.Command{
	Use:   "kubectl-jfs",
	Short: "tool for juicefs debug in kubernetes",
	Version: func() string {
		return pkg.Version()
	}(),
}
