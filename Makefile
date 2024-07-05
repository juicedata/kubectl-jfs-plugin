# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

VERSION=$(shell git describe --tags --match 'v*' | grep -oE 'v[0-9]+\.[0-9][0-9]*(\.[0-9]+)?')
GIT_BRANCH?=$(shell git rev-parse --abbrev-ref HEAD)
GIT_COMMIT?=$(shell git rev-parse HEAD)
DEV_TAG=dev-$(shell git describe --always --dirty)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
REVISIONDATE:=$(shell git log -1 --pretty=format:'%cd' --date short 2>/dev/null)
GO111MODULE=on

.PHONY: build
build:
	docker run -it --rm -v $(shell pwd):/go/src/github.com/juicedata/kubectl-jfs-plugin \
	-e VERSION=${VERSION} \
	-e GIT_COMMIT=${GIT_COMMIT} \
	-e BUILD_DATE=${BUILD_DATE} \
	-v $(shell pwd)/bin:/bin/jfs \
	-w /go/src/github.com/juicedata/kubectl-jfs-plugin \
	golang:1.22 sh ./hack/multibuild.sh ./cmd/ /bin/jfs
