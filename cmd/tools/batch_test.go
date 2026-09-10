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

package tools

import "testing"

func TestBatchSidecarFlagIsPersistent(t *testing.T) {
	if flag := batchCmd.PersistentFlags().Lookup("sidecar"); flag == nil {
		t.Fatal("batch --sidecar persistent flag is not registered")
	}
	if flag := batchDiffCmd.LocalFlags().Lookup("sidecar"); flag != nil {
		t.Fatal("batch diff must inherit --sidecar from batch")
	}
	if flag := batchUpgradeCmd.LocalFlags().Lookup("sidecar"); flag != nil {
		t.Fatal("batch upgrade must inherit --sidecar from batch")
	}
}
