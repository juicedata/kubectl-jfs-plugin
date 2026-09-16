/*
 * Copyright 2026 Juicedata Inc
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

	jConfig "github.com/juicedata/juicefs-csi-driver/pkg/config"
)

type UpgradeOptions struct {
	PVCName     string
	Namespace   string
	NodeName    string
	Worker      int
	IgnoreError bool
	Quiet       bool
	Sidecar     bool
}

func (o UpgradeOptions) Validate() error {
	if !o.Sidecar {
		return nil
	}
	if o.Namespace == "" {
		return fmt.Errorf("--namespace is required with --sidecar")
	}
	if o.PVCName != "" {
		return fmt.Errorf("--pvc cannot be used with --sidecar")
	}
	if o.Worker <= 0 {
		return fmt.Errorf("--worker must be greater than zero with --sidecar")
	}
	return nil
}

func sidecarTargetDisplayName(target jConfig.UpgradeTarget) string {
	return fmt.Sprintf("%s/%s/%s", target.Namespace, target.Name, target.ContainerName)
}
