#!/bin/bash

# Copyright 2019 The Kubeflow Authors.
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

set -o errexit
set -o nounset
set -o pipefail

if [[ -z "${GOPATH:-}" ]]; then
    export GOPATH=$(go env GOPATH)
fi

VERSION_LIST=(v1beta1)
PROJECT_ROOT=${GOPATH}/src/github.com/kubeflow/katib

modules=(experiments suggestions trials common)
versions=(v1beta1)
versionStr=$(printf ",%s" "${versions[@]}")
GROUP_VERSIONS=$(printf "%s:${versionStr:1} " "${modules[@]}")

echo "Generating clients for ${GROUP_VERSIONS} ..."
${PROJECT_ROOT}/vendor/k8s.io/code-generator/generate-groups.sh \
    all \
    github.com/kubeflow/katib/pkg/client/controller \
    github.com/kubeflow/katib/pkg/apis/controller \
    "${GROUP_VERSIONS}" \
    --go-header-file ${PROJECT_ROOT}/hack/boilerplate.go.txt
