#!/usr/bin/env bash

# Copyright 2022 Jijie Chen.
# Copyright 2021 The Clusternet Authors.
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

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(
  cd "${SCRIPT_ROOT}"
  if [ ! -d ./vendor/k8s.io/code-generator ]; then
    go mod vendor
  fi
  ls -d -1 ./vendor/k8s.io/code-generator
)}


bash "${CODEGEN_PKG}/generate-groups.sh" all \
  github.com/jijiechen/external-crd/pkg/generated \
  github.com/jijiechen/external-crd/pkg/apis \
  "kcrd:v1alpha1 overlay:v1alpha1" \
  --output-base "$(dirname "${BASH_SOURCE[0]}")/.." \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" 
  # -v 10 \

  rm -rf "$(dirname "${BASH_SOURCE[0]}")/../pkg/generated"
  mv github.com/jijiechen/external-crd/pkg/generated "$(dirname "${BASH_SOURCE[0]}")/../pkg/"
  rm -rf github.com

# debug read -r line
