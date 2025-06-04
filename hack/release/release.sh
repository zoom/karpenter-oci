#!/usr/bin/env bash

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
# shellcheck source=hack/release/common.sh
source "${SCRIPT_DIR}/common.sh"

git_tag="$(git describe --exact-match --tags || echo "no tag")"
if [[ "${git_tag}" == "no tag" ]]; then
  echo "Failed to release: commit is untagged"
  exit 1
fi
commit_sha="$(git rev-parse HEAD)"

# Don't release with a dirty commit!
if [[ "$(git status --porcelain)" != "" ]]; then
  exit 1
fi
repo="${1}"
release "${commit_sha}" "${git_tag#v}" "${repo}"
