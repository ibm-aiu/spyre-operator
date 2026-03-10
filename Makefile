# Copyright (c) 2025, 2026 IBM Corp.
# SPDX-License-Identifier: Apache-2.0

GOLANG_VERSION		?= $(shell cd $(REPO_ROOT) && go list -f {{.GoVersion}} -m)
BUILDER_IMAGE		?= registry.access.redhat.com/ubi9/go-toolset:1.24.6-1758501173
MAKEFILE_PATH		:= $(abspath $(lastword $(MAKEFILE_LIST)))
REPO_ROOT 			:= $(abspath $(patsubst %/,%,$(dir $(MAKEFILE_PATH))))
CURRENT_DIR			:= $(shell pwd)
OCP_VERSIONS		?= v4.16-v4.20
VERSION				?= $(shell cat $(REPO_ROOT)/VERSION)
REGISTRY			?= docker.io/spyre-operator
DOCKER				?= $(shell command -v podman 2> /dev/null || echo docker)
DOCKERFILE			= $(REPO_ROOT)/Dockerfile
DOCKER_BUILD_OPTS	?= --progress=plain

IMAGE_NAME 			:= $(REGISTRY)/spyre-operator
IMAGE_TAG 			?= $(VERSION)
IMAGE 				?= $(IMAGE_NAME):$(IMAGE_TAG)
TEST_IMG			?= $(IMAGE_NAME):dev
CODECOV_PERCENT		?= 57

# Read any custom variables overrides from a local.mk file.  This will only be read if it exists in the
# same directory as this Makefile.  Variables can be specified in the standard format supported by
# GNU Make since `include` processes any valid Makefile
# Standard variables override would include anything you would pass at runtime that is different
# from the defaults specified in this file
OPERATOR_MAKE_ENV_FILE = $(REPO_ROOT)/local.mk
-include $(OPERATOR_MAKE_ENV_FILE)

# Define local and dockerized golang targets
KUBECTL             ?= $(shell command -v oc 2> /dev/null || echo kubectl)
OC                  ?= $(shell command -v oc)
OPERATOR_NAMESPACE  ?= spyre-operator
DEFAULT_CHANNEL		?=fast-v1.3
CHANNELS            ?= $(DEFAULT_CHANNEL)

# Operating system
OS					?= $(shell go env GOOS)
ARCH				?= $(shell go env GOARCH)

# End to end test configuration variables
E2E_KUBECONFIG		?= ${HOME}/.kube/config
TEST_CONFIG          ?= $(REPO_ROOT)/test/config.yaml
export E2E_KUBECONFIG
export TEST_CONFIG

# Integration test configuration variables
# This LABEL only runs operator related tests
INTEGRATION_TEST_LABEL ?= "integration && !cardmgmt"

# detect-secrets
DETECT_SECRETS_GIT ?= "https://github.com/ibm/detect-secrets.git@master\#egg=detect-secrets"

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
IMAGE_TAG_BASE ?= $(IMAGE_NAME)


# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite=true $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

define go-mod-version
$(shell go mod graph | grep $(1) 2>/dev/null | head -n 1 | cut -d'@' -f 2)
endef

# Using controller-gen to fetch external CRDs and put them in config/crd/external folder
# They're used in tests, as they have to be created for controller to work
define fetch-external-crds
GOFLAGS="-mod=readonly" $(CONTROLLER_GEN) crd \
paths=$(shell go env GOPATH)/pkg/mod/$(1)@$(call go-mod-version,$(1))/$(2)/... \
output:crd:artifacts:config=test/crd/external
endef

## Tool Binaries
CONTROLLER_GEN	?= $(LOCALBIN)/controller-gen
CRDOC 			?= $(LOCALBIN)/crdoc
ENVTEST			?= $(LOCALBIN)/setup-envtest
GINKGO			?= $(LOCALBIN)/ginkgo
GOLANGCI_LINT	?= $(LOCALBIN)/golangci-lint
GOVULCHECK		?= $(LOCALBIN)/govulncheck
JQ				?= $(LOCALBIN)/jq
KIND			?= $(LOCALBIN)/kind
KIND			?= $(LOCALBIN)/kind
KUSTOMIZE 		?= $(LOCALBIN)/kustomize
YQ				?= $(LOCALBIN)/yq
YAMLFMT			?= $(LOCALBIN)/yamlfmt

## Tool Versions
CONTROLLER_TOOLS_VERSION 	?= v0.17.3
CRDOC_VERSION 				?= v0.6.4
ENVTEST_K8S_VERSION			?= 1.31
GINKGO_VERSION				?= v2.28.1
GOLANGCI_LINT_VERSION		?= 1.64.8
JQ_VERSION 					?= jq-1.7.1
KIND_VERSION				?= 0.20.0
KUSTOMIZE_VERSION 			?= v5.4.1
YQ_VERSION 					?= v4.29.2
KUSTOMIZE_INSTALL_SCRIPT 	?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
YAMLFMT_VERSION				?= v0.17.0

# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make fbc-build).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(VERSION)
DOCKER_BASENAME = $(shell basename $(DOCKER))
DOCKER_GO_BUILD_FLAGS ?= -race

##@ General

.PHONY: all
all: build ## Build all defined targets

.PHONY: all-build
# "bundle-push" is required for "fbc-build".
all-build: bundle bundle-validate bundle-build bundle-push fbc-bundle-add fbc-build docker-build ## Build all images (and push bundle image).

.PHONY: all-push
all-push: bundle-push fbc-push docker-push ## Push all images

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


.PHONY: version
version: ## Display image version
	@echo "Image version: $(VERSION)"

.PHONY: echo-version
echo-version: ## Print (echo) the current version
	$(info $(VERSION))
	@echo > /dev/null

##@ Development tools

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download and install ginkgo
$(GINKGO):$(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download and install setup-envtest
$(ENVTEST):$(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240624150636-162a113134de

GOLANGCI_LINT_INSTALL_SCRIPT ?= 'https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh'
.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ### Download golangci-lint locally if necessary.
$(GOLANGCI_LINT):$(LOCALBIN)
	test -s $(GOLANGCI_LINT) || { curl --retry 30 -sSfL $(GOLANGCI_LINT_INSTALL_SCRIPT) | sh -s -- -b $(LOCALBIN)  v$(GOLANGCI_LINT_VERSION); }

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary
$(KIND):$(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@v$(KIND_VERSION)

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	test -s $(YQ) || GOBIN=$(LOCALBIN) go install github.com/mikefarah/yq/v4@$(YQ_VERSION)

.PHONY: yamlfmt
yamlfmt: $(YAMLFMT) ## Download yamlfmt locally if necessary
$(YAMLFMT):$(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/google/yamlfmt/cmd/yamlfmt@$(YAMLFMT_VERSION)

.PHONY: jq
jq: $(JQ) ## Download jq locally if necessary.
$(JQ): $(LOCALBIN)
ifeq ($(OS), darwin)
	curl --retry 30 -Ls https://github.com/jqlang/jq/releases/download/$(JQ_VERSION)/jq-macos-$(ARCH) -o $(JQ) && chmod +x $(JQ)
else ifeq ($(OS),linux)
ifeq ($(ARCH), ppc64le)
	curl --retry 30 -Ls https://github.com/jqlang/jq/releases/download/$(JQ_VERSION)/jq-linux-ppc64el -o $(JQ) && chmod +x $(JQ)
else
	curl --retry 30 -Ls https://github.com/jqlang/jq/releases/download/$(JQ_VERSION)/jq-linux-$(ARCH) -o $(JQ) && chmod +x $(JQ)
endif
else
	@echo "jq could not be installed."
endif

.PHONY: govulncheck
govulncheck: $(GOVULCHECK) ## Download govulncheck tool if necessary
$(GOVULCHECK): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: venv
venv: ## Setup and activate venv
	$(PYTHON) -m venv venv

.PHONY: api-docs
api-docs: crdoc manifests ## Generate docs.
	$(CRDOC) --resources config/crd/bases --output docs/api/v$(shell cat VERSION).md

controller-gen: $(LOCALBIN) $(CONTROLLER_GEN) ## Download controller-gen if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: kustomize
kustomize: $(LOCALBIN) $(KUSTOMIZE) ## Download kustomize if necessary
ifeq ("$(wildcard $(KUSTOMIZE))", "")
$(KUSTOMIZE): $(LOCALBIN)
	curl --retry 30 -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)
else
	@echo make: Nothing to be done for 'kustomize'.
endif

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm if necessary
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl --retry 30 -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.45.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

.PHONY: crdoc
crdoc:
	GOBIN=$(LOCALBIN) go install fybrik.io/crdoc@$(CRDOC_VERSION)

##@ Operator Artifacts

.PHONY: manifests
manifests: controller-gen yamlfmt ## Generate manifests
	$(CONTROLLER_GEN) rbac:roleName=manager-role,headerFile="hack/boilerplate.yaml.txt",year="2025" \
					  crd:headerFile="hack/boilerplate.yaml.txt",year="2025" \
					  webhook:headerFile="hack/boilerplate.yaml.txt",year="2025" \
					  paths="{./api/...,./controllers/...,./pkg/...,./internal/...}" \
					  output:crd:artifacts:config=config/crd/bases
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt -dstar "$(REPO_ROOT)/config/**/*.yaml"

.PHONY: generate
generate: controller-gen ## Generate code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="{./api/...,./controllers/...,./pkg/...,./internal/...}"

.PHONY: bundle
bundle: kustomize yq yamlfmt manifests ## Generate bundle manifests and metadata using base branch version
	operator-sdk generate kustomize manifests -q
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt $(REPO_ROOT)/config/manifests/bases/spyre-operator.clusterserviceversion.yaml
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE_NAME):$(VERSION)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle $(BUNDLE_GEN_FLAGS) --version $(VERSION)
	$(YQ) eval -i ".annotations[\"com.redhat.openshift.versions\"]=\"$(OCP_VERSIONS)\"" ${REPO_ROOT}/bundle/metadata/annotations.yaml
	$(YQ) eval -i ".spec.image=\"$(IMAGE_TAG_BASE)-catalog:$(VERSION)\"" ${REPO_ROOT}/config/olm/catalog-source.yaml
	find bundle -type d -exec chmod 755 {} \;
	find bundle -type f -exec chmod 644 {} \;
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt -dstar "$(REPO_ROOT)/config/**/*.yaml" "$(REPO_ROOT)/bundle/**/*.yaml" $(REPO_ROOT)/config/olm/catalog-source.yaml

.PHONY: bundle-validate
bundle-validate: ## Validate the bundle files
ifeq ($(DOCKER),docker)
	operator-sdk bundle validate ./bundle --optional-values=container-tools=docker --select-optional name=multiarch
else
	operator-sdk bundle validate ./bundle --optional-values=container-tools=podman --select-optional name=multiarch
endif

.PHONY: bundle-build
bundle-build: ## Build bundle image.
	$(DOCKER) build $(DOCKER_BUILD_OPTS) \
		--tag $(BUNDLE_IMG) \
		--file bundle.Dockerfile $(CURDIR)

.PHONY: bundle-push
bundle-push: ## Push bundle image.
	$(DOCKER) push $(BUNDLE_IMG)

## Build FBC from scratch
catalog/template.yaml:
	./catalog/fbc.sh "template" development $(IMAGE_TAG_BASE)-bundle

.PHONY: fbc-gen-template
fbc-gen-template: catalog/base_template.yaml ## Generate a File Based Catalog (FBC) yaml base template

.PHONY: fbc-bundle-add
fbc-bundle-add: fbc-gen-template ## Add current bundle to the base template
	$(REPO_ROOT)/catalog/fbc.sh "add_bundle" "$(BUNDLE_IMG)"

.PHONY: fbc-build-setup
fbc-build-setup: ## Setup for the catalog building
	mkdir -p catalog/spyre-operator
	$(OPM) alpha render-template semver catalog/base_template.yaml -oyaml --skip-tls-verify=true > catalog/spyre-operator/spyre-operator.yaml
	$(REPO_ROOT)/catalog/fbc.sh "update_fbc" "catalog/spyre-operator/spyre-operator.yaml"
	$(OPM) validate catalog/spyre-operator/

.PHONY: fbc-build
fbc-build: opm fbc-bundle-add fbc-build-setup  ## Build catalog image for the build host architecture
	$(DOCKER) build $(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG) \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
	$(REPO_ROOT)/catalog/fbc.sh "gen_catalogsource_cr" "$(CATALOG_IMG)"

.PHONY: fbc-push
fbc-push: ## Push catalog image build for the build host architecture
	$(DOCKER) push $(CATALOG_IMG)

##@ Test targets

.PHONY: ensure-deps
ensure-deps: yq ## Deploy dependent operators on the openshift local cluster
	$(REPO_ROOT)/test/script/ensure-deps.sh

.PHONY: test-docker-build
test-docker-build: docker-build ## Build test-purpose image.
	$(DOCKER) tag $(IMAGE) $(TEST_IMG)

.PHONY: test-docker-push
test-docker-push: ## Push test-purpose image.
	$(DOCKER) push $(TEST_IMG)

COVERAGE_FILE := coverage.out
.PHONY: test
test: fmt vet ginkgo jq manifests generate envtest ## Run unit tests.
	$(call fetch-external-crds,github.com/openshift/cluster-nfd-operator,api/v1alpha1)
	$(call fetch-external-crds,github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring,v1)
	$(call fetch-external-crds,github.com/openshift/api,security/v1)
	$(call fetch-external-crds,github.com/cert-manager/cert-manager,pkg/apis/certmanager/v1)
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(LOCALBIN)/ginkgo run --label-filter="!(e2e||integration)" --seed 777 --cover --coverprofile=$(COVERAGE_FILE) ./controllers/... ./pkg/... ./internal/...
	go tool cover -func $(COVERAGE_FILE)
	go tool cover -html $(COVERAGE_FILE) -o coverage-report.html
	@percentage=$$(go tool cover -func=$(COVERAGE_FILE) | grep ^total | awk '{print $$3}' | tr -d '%'); \
		if (( $$(echo "$$percentage < $(CODECOV_PERCENT)" | bc -l) )); then \
			echo "----------"; \
			echo "Total test coverage ($${percentage}%) is less than the coverage threshold ($(CODECOV_PERCENT)%)."; \
			exit 1; \
		fi

.PHONY: integration-test
integration-test: ginkgo jq ensure-deps ## Run integration test on the cluster pointed to in the current KUBECONFIG (expecting NFD instance running)
	$(YQ) eval -i '.pseudoDeviceMode=false' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.devicePluginInit.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.devicePluginInit.executePolicy="Always"' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.podValidator.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.exporter.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.healthChecker.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.hasDevice=true' ${REPO_ROOT}/test/config.yaml
	OC=$(OC) $(GINKGO) run --label-filter=$(INTEGRATION_TEST_LABEL) --seed 777 --cover --coverprofile=coverage-report.out -v ./test/integration/...

.PHONY: e2e-test
e2e-test: ginkgo jq ensure-deps ## Run e2e test on the cluster pointed to in the current KUBECONFIG (expecting NFD instance running)
	$(info TEST_CONFIG is set to $(TEST_CONFIG))
	$(info E2E_KUBECONFIG is set to $(E2E_KUBECONFIG))
	$(GINKGO) run --timeout=2h --label-filter="e2e" --cover --coverprofile=coverage-report.out -v ./test/e2e/...

##@ Development Targets

.PHONY: fmt
fmt: ## Run the formatter
	go fmt ./...

.PHONY: vet
vet: vendor ## Run the vet command
	go vet -mod vendor ./...

.PHONY: vendor
vendor: ## Run vendor
	go mod vendor

.PHONY: build
build: generate vendor ## Build local binary
	go build -mod vendor -race -a -o $(LOCALBIN)/manager main.go

.PHONY: lint
lint: golangci-lint vendor ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run --sort-results --config $(REPO_ROOT)/.golangci.yaml --go $(GOLANG_VERSION)

.PHONY: lint-fix
lint-fix: golangci-lint vendor ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run --fix --config $(REPO_ROOT)/.golangci.yaml --go $(GOLANG_VERSION)

.PHONY: vulcheck
vulcheck: govulncheck ## Scan for golang vulnerabilities
	$(GOVULCHECK) -show verbose	 ./...

.PHONY: clean
clean: ## Clean-up intermediate artifacts
	-rm -rf vendor
	-rm -rf $(LOCALBIN)
	-rm -rf local.mk
	-rm -rf catalog/base_template.yaml catalog/catalog-source.yaml catalog/spyre-operator/spyre-operator.yaml

.PHONY: propagate-version
propagate-version: yq yamlfmt ## Propagate version to all required files
	hack/propagate-version.bash $(VERSION) $(REGISTRY) $(DEFAULT_CHANNEL)

##@ Image operations

.PHONY: docker-build
docker-build: vendor ## Build sypre operator image for the build host architecture
	$(DOCKER) build $(DOCKER_BUILD_OPTS) --pull \
	--tag $(IMAGE) \
	--build-arg VERSION="$(VERSION)" \
	--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
	--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
	--file $(DOCKERFILE) $(CURDIR)

.PHONY: docker-push
docker-push: ## Push spyre operator image image for the build host architecture
	$(DOCKER) push $(IMAGE)

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push the spyre operator image for the build host

##@ Deployment
ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: run
run: manifests generate fmt vet lint ## Run a controller from your host.
	go run ./main.go

.PHONY: clean-resource
clean-resource: ## Delete spyre-related resources on a cluster
	-$(KUBECTL) get spyrenodestate -A | fgrep -v 'NAMESPACE' | while read line; do ns=`echo $${line} | awk '{print $$1}'` ; name=`echo $${line} | awk '{print $$2}'` ; $(KUBECTL) -n $${ns} delete spyrenodestate $${name} ; done

.PHONY: install-operator
install-operator: yq ## Install operator via olm and deploy SpyreClusterPolicy
	@$(REPO_ROOT)/hack/install-operator.bash install $(OPERATOR_NAMESPACE) $(CATALOG_IMG) $(CHANNELS) ARCH=${ARCH}

##@ Release targets
.PHONY: detect-secrets-install
detect-secrets-install: venv ## Install detect-secret tool
	$(eval TMP_CONSTRAINTS := $(shell mktemp))
	@echo "boxsdk<4" > $(TMP_CONSTRAINTS)
	@echo "chardet<6" >> $(TMP_CONSTRAINTS)
	. venv/bin/activate && $(PIP) install "git+$(DETECT_SECRETS_GIT)" -c $(TMP_CONSTRAINTS)
	@rm -f $(TMP_CONSTRAINTS)

.PHONY: secrets-scan
secrets-scan: detect-secrets-install venv ## Scan secrets and create secret-baseline for repo
	. venv/bin/activate; detect-secrets scan --no-ghe-scan --exclude-files go.sum --update .secrets.baseline

.PHONY: secrets-audit
secrets-audit: detect-secrets-install venv ## Audit secrets
	. venv/bin/activate; detect-secrets audit .secrets.baseline

# helper target for viewing the value of makefile variables.
print-%  : ;@echo $* = $($*)
