CLUSTER_NAME ?= karpenter-oci-test
CLUSTER_ENDPOINT ?= https://10.0.0.10:6443
COMPARTMENT_ID ?= ocid1.compartment.oc1..aaaaaaaa
CLUSTER_DNS ?= 10.96.5.5
REGION ?= us-ashburn-1
TENANCY_NAMESPACE ?= tenantnamespace
## Inject the app version into operator.Version
LDFLAGS ?= -ldflags=-X=sigs.k8s.io/karpenter/pkg/operator.Version=$(shell git describe --tags --always | cut -d"v" -f2)

GOFLAGS ?= $(LDFLAGS)
WITH_GOFLAGS = GOFLAGS="$(GOFLAGS)"

## Extra helm options
HELM_OPTS ?= --set settings.clusterName=${CLUSTER_NAME} \
            --set settings.clusterEndpoint=${CLUSTER_ENDPOINT} \
            --set settings.compartmentId=${COMPARTMENT_ID} \
            --set settings.clusterDns=${CLUSTER_DNS} \
            --set settings.ociResourcePrincipalRegion=${REGION} \
			--set controller.resources.requests.cpu=1 \
			--set controller.resources.requests.memory=1Gi \
			--set controller.resources.limits.cpu=1 \
			--set controller.resources.limits.memory=1Gi \
			--create-namespace

# CR for local builds of Karpenter
KARPENTER_NAMESPACE ?= karpenter
KARPENTER_VERSION ?= $(shell git tag --sort=committerdate | tail -1 | cut -d"v" -f2)
KO_DOCKER_REPO ?= ocir.us-ashburn-1.oci.oraclecloud.com/${TENANCY_NAMESPACE}/karpenter/karpenter-oci
KOCACHE ?= ~/.ko

# Common Directories
MOD_DIRS = $(shell find . -name go.mod -type f -print | xargs dirname)
KARPENTER_CORE_DIR = $(shell go list -m -mod=mod -f '{{ .Dir }}' sigs.k8s.io/karpenter)

# TEST_SUITE enables you to select a specific test suite directory to run "make e2etests" against
TEST_SUITE ?= "..."

help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

presubmit: verify test ## Run all steps in the developer loop

ci-test: test coverage ## Runs tests and submits coverage

ci-non-test: verify licenses vulncheck ## Runs checks other than tests

run: ## Run Karpenter controller binary against your local cluster
	SYSTEM_NAMESPACE=${KARPENTER_NAMESPACE} \
		KUBERNETES_MIN_VERSION="1.19.0-0" \
		DISABLE_LEADER_ELECTION=true \
		DISABLE_WEBHOOK=true \
		CLUSTER_NAME=${CLUSTER_NAME} \
		CLUSTER_ENDPOINT=${CLUSTER_ENDPOINT} \
		COMPARTMENT_ID=${COMPARTMENT_ID} \
		CLUSTER_DNS=${CLUSTER_DNS} \
		OCI_AUTH_METHODS=SESSION \
		go run ./cmd/controller/main.go

test: ## Run tests
	go test ./pkg/... \
		-cover -coverprofile=coverage.out -outputdir=. -coverpkg=./... \
		--ginkgo.focus="${FOCUS}" \
		--ginkgo.randomize-all \
		--ginkgo.vv

deflake: ## Run randomized, racing tests until the test fails to catch flakes
	ginkgo \
		--race \
		--focus="${FOCUS}" \
		--randomize-all \
		--until-it-fails \
		-v \
		./pkg/...


coverage:
	go tool cover -html coverage.out -o coverage.html

verify-licence:
	hack/make-rules/check_license.sh

verify: tidy download
	go generate ./...
	hack/boilerplate.sh
	cp  $(KARPENTER_CORE_DIR)/pkg/apis/crds/* pkg/apis/crds
	hack/validation/kubelet.sh
	hack/validation/requirements.sh
	hack/validation/labels.sh
	cp pkg/apis/crds/* charts/karpenter-crd/templates
	#hack/github/dependabot.sh
	$(foreach dir,$(MOD_DIRS),cd $(dir) && golangci-lint run --timeout 10m $(newline))
	@git diff --quiet ||\
		{ echo "New file modification detected in the Git working tree. Please check in before commit."; git --no-pager diff --name-only | uniq | awk '{print "  - " $$0}'; \
		if [ "${CI}" = true ]; then\
			exit 1;\
		fi;}

vulncheck: ## Verify code vulnerabilities
	@govulncheck ./pkg/...

licenses: download ## Verifies dependency licenses
	# TODO: remove nodeadm check once license is updated
	! go-licenses csv ./... | grep -v -e 'MIT' -e 'Apache-2.0' -e 'BSD-3-Clause' -e 'BSD-2-Clause' -e 'ISC' -e 'MPL-2.0' -e 'github.com/awslabs/amazon-eks-ami/nodeadm'

image: ## Build the Karpenter controller images using ko build

	$(eval CONTROLLER_IMG=$(shell $(WITH_GOFLAGS) KOCACHE=$(KOCACHE) KO_DOCKER_REPO="$(KO_DOCKER_REPO)" ko build --bare -t `git tag --sort=committerdate | tail -1 | cut -d"v" -f2`  github.com/zoom/karpenter-oci/cmd/controller))
	$(eval IMG_REPOSITORY=$(shell echo $(CONTROLLER_IMG) | cut -d "@" -f 1 | cut -d ":" -f 1))
	$(eval IMG_TAG=$(shell echo $(CONTROLLER_IMG) | cut -d "@" -f 1 | cut -d ":" -f 2 -s))
	$(eval IMG_DIGEST=$(shell echo $(CONTROLLER_IMG) | cut -d "@" -f 2))

apply: verify image ## Deploy the controller from the current state of your git repository into your ~/.kube/config cluster
	kubectl apply -f ./pkg/apis/crds/
	helm upgrade --install karpenter charts/karpenter --namespace ${KARPENTER_NAMESPACE} \
        $(HELM_OPTS) \
        --set logLevel=debug \
        --set controller.image.repository=$(IMG_REPOSITORY) \
        --set controller.image.tag=$(IMG_TAG) \
        --set controller.image.digest=$(IMG_DIGEST)

#TODO impl me
install:  ## Deploy the latest released version into your ~/.kube/config cluster
	@echo Upgrading to ${KARPENTER_VERSION}
	helm upgrade --install karpenter oci://iad.ocir.io/${TENANCY_NAMESPACE}/karpenter/karpenter-oci --version ${KARPENTER_VERSION} --namespace ${KARPENTER_NAMESPACE} \
		$(HELM_OPTS)

delete: ## Delete the controller from your ~/.kube/config cluster
	helm uninstall karpenter --namespace ${KARPENTER_NAMESPACE}

snapshot: ## Builds and publishes snapshot release
	$(WITH_GOFLAGS) ./hack/release/snapshot.sh "ocir.us-ashburn-1.oci.oraclecloud.com/${TENANCY_NAMESPACE}"

release: ## Builds and publishes stable release
	$(WITH_GOFLAGS) ./hack/release/release.sh "ocir.us-ashburn-1.oci.oraclecloud.com/${TENANCY_NAMESPACE}"

tidy: ## Recursively "go mod tidy" on all directories where go.mod exists
	$(foreach dir,$(MOD_DIRS),cd $(dir) && go mod tidy $(newline))

download: ## Recursively "go mod download" on all directories where go.mod exists
	$(foreach dir,$(MOD_DIRS),cd $(dir) && go mod download $(newline))

.PHONY: help presubmit ci-test ci-non-test run test deflake coverage verify-licence verify vulncheck licenses image apply install delete snapshot release tidy download

define newline


endef
