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

K8S_VERSION="${K8S_VERSION:="1.31.1"}"
KUBEBUILDER_ASSETS="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"


main() {
    tools
    kubebuilder
}

tools() {
    go install github.com/google/go-licenses@v1.6.0
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2
    go install github.com/google/ko@v0.17.1
    go install github.com/mikefarah/yq/v4@v4.44.5
    go install github.com/norwoodj/helm-docs/cmd/helm-docs@v1.14.2
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
    go install github.com/sigstore/cosign/v2/cmd/cosign@v2.4.1
    go install golang.org/x/vuln/cmd/govulncheck@v1.1.3
    go install github.com/onsi/ginkgo/v2/ginkgo@latest
    go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.4
    go install github.com/mattn/goveralls@v0.0.12
    go install github.com/google/go-containerregistry/cmd/crane@v0.20.2

    if ! echo "$PATH" | grep -q "${GOPATH:-undefined}/bin\|$HOME/go/bin"; then
        echo "Go workspace's \"bin\" directory is not in PATH. Run 'export PATH=\"\$PATH:\${GOPATH:-\$HOME/go}/bin\"'."
    fi
}

kubebuilder() {
    if ! mkdir -p ${KUBEBUILDER_ASSETS}; then
      sudo mkdir -p ${KUBEBUILDER_ASSETS}
      sudo chown $(whoami) ${KUBEBUILDER_ASSETS}
    fi
    arch=$(go env GOARCH)
    ln -sf $(setup-envtest use -p path "${K8S_VERSION}" --arch="${arch}" --bin-dir="${KUBEBUILDER_ASSETS}")/* ${KUBEBUILDER_ASSETS}
    find $KUBEBUILDER_ASSETS

    # Install latest binaries for 1.25.x (contains CEL fix)
    if [[ "${K8S_VERSION}" = "1.25.x" ]] && [[ "$OSTYPE" == "linux"* ]]; then
        for binary in 'kube-apiserver' 'kubectl'; do
            rm $KUBEBUILDER_ASSETS/$binary
            wget -P $KUBEBUILDER_ASSETS dl.k8s.io/v1.25.16/bin/linux/${arch}/${binary}
            chmod +x $KUBEBUILDER_ASSETS/$binary
        done
    fi
}

main "$@"
