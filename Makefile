# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

GOLANG_VERSION		?= $(shell cd $(REPO_ROOT) && go list -f {{.GoVersion}} -m)
BUILDER_IMAGE		?= registry.access.redhat.com/ubi9/go-toolset:1.24.6-1758501173
MAKEFILE_PATH		:= $(abspath $(lastword $(MAKEFILE_LIST)))
REPO_ROOT 			:= $(abspath $(patsubst %/,%,$(dir $(MAKEFILE_PATH))))
CURRENT_DIR			:= $(shell pwd)
BASE_VERSION		= $(shell $(REPO_ROOT)/hack/get-version.bash --type base --version $(shell cat $(REPO_ROOT)/VERSION))
VERSION				= $(shell $(REPO_ROOT)/hack/get-version.bash --type relative --version $(shell cat $(REPO_ROOT)/VERSION))
OCP_VERSIONS		?= v4.16-v4.20
REGISTRY			?= docker.io
NAMESPACE			?= spyre-operator
DOCKER				?= $(shell command -v podman 2> /dev/null || echo docker)
DOCKERFILE			= $(REPO_ROOT)/Dockerfile
DOCKER_BUILD_OPTS	?= --progress=plain

IMAGE_NAME 			:= $(REGISTRY)/$(NAMESPACE)/spyre-operator
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

# mkdocs
PYTHON                  ?= python3
PIP                     ?= pip3
MKDOCS_VERSION          ?= 1.6.0
MKDOCS_MATERIAL_VERSION ?= 9.5.29
MKDOCS_SERVE_OPT        ?= -w docs -o

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

# Shamesly copied from: https://github.com/opendatahub-io/opendatahub-operator/blob/a08c94a226585e43387ad263e2653c0fd43130f1/Makefile#L132C1-L139C1
define go-mod-version
$(shell go mod graph | grep $(1) | head -n 1 | cut -d'@' -f 2)
endef

# Using controller-gen to fetch external CRDs and put them in config/crd/external folder
# They're used in tests, as they have to be created for controller to work
define fetch-external-crds
GOFLAGS="-mod=readonly" $(CONTROLLER_GEN) crd \
paths=$(shell go env GOPATH)/pkg/mod/$(1)@$(call go-mod-version,$(1))/$(2)/... \
output:crd:artifacts:config=test/crd/external
endef

## Tool Binaries
BUTANE          ?= $(LOCALBIN)/butane
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
STERN			?= $(LOCALBIN)/stern
YQ				?= $(LOCALBIN)/yq
YAMLFMT			?= $(LOCALBIN)/yamlfmt

## Tool Versions
BUTANE_VERSION 				?= v0.23.0
CONTROLLER_TOOLS_VERSION 	?= v0.17.3
CRDOC_VERSION 				?= v0.6.4
ENVTEST_K8S_VERSION			?= 1.31
GINKGO_VERSION				?= v2.25.2
GOLANGCI_LINT_VERSION		?= 1.64.8
JQ_VERSION 					?= jq-1.7.1
KIND_VERSION				?= 0.20.0
KUSTOMIZE_VERSION 			?= v5.4.1
STERN_VERSION 				?= v1.30.0
YQ_VERSION 					?= v4.29.2
KUSTOMIZE_INSTALL_SCRIPT 	?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
YAMLFMT_VERSION				?= v0.17.0

# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make fbc-build).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(VERSION)
DOCKER_BASENAME = $(shell basename $(DOCKER))
# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator

DOCKER_GO_BUILD_FLAGS ?=
BUILD_TYPE = $(shell $(REPO_ROOT)/hack/get-build-type.bash)
ifeq ($(strip $(BUILD_TYPE)), pr)
DOCKER_GO_BUILD_FLAGS += -race
endif


ifeq (release , $(BUILD_TYPE))
ADDITIONAL_IMAGE_TAG := stable
else ifeq (development, $(BUILD_TYPE))
ADDITIONAL_IMAGE_TAG := fast
else ifneq (, $(strip $(CHANGE_ID)))
ADDITIONAL_IMAGE_TAG := PR-$(CHANGE_ID)
else
ADDITIONAL_IMAGE_TAG := latest-pr
endif

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

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


##@ Development tools
.PHONY: stern
stern : $(STERN) ## Download stern
$(STERN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/stern/stern@$(STERN_VERSION)

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

# Set BUTANE_ARCH based on OS and ARCH
ifeq ($(OS),darwin)
    BUTANE_ARCH := x86_64-apple-darwin
else ifeq ($(OS),linux)
    ifeq ($(ARCH),amd64)
        BUTANE_ARCH := x86_64
    else
        BUTANE_ARCH := $(ARCH)
    endif
else
    BUTANE_ARCH := unsupported
endif

.PHONY: butane
butane: $(BUTANE) ## Download butane locally if necessary
$(BUTANE):$(LOCALBIN)
ifeq ($(BUTANE_ARCH),unsupported)
	@echo "butane could not be installed."
else ifeq ($(OS), darwin)
	test -s butane || \
	curl --retry 30 -Ls https://github.com/coreos/butane/releases/download/${BUTANE_VERSION}/butane-x86_64-apple-darwin -o $(BUTANE) && chmod +x $(BUTANE)
else ifeq ($(OS),linux)
	test -s butane || \
	curl --retry 30 -Ls https://github.com/coreos/butane/releases/download/${BUTANE_VERSION}/butane-${BUTANE_ARCH}-unknown-linux-gnu -o $(BUTANE) && chmod +x $(BUTANE)
else
	@echo "butane could not be installed."
endif

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

.PHONY: mkdocs
mkdocs-build: venv ## Download mkdoc and build docs
	. venv/bin/activate; $(PIP) install mkdocs==$(MKDOCS_VERSION) mkdocs-material==$(MKDOCS_MATERIAL_VERSION) \
	&& cd document && mkdocs build

mkdocs-serve: venv ## Download mkdoc and start server
	. venv/bin/activate; $(PIP) install mkdocs==$(MKDOCS_VERSION) mkdocs-material==$(MKDOCS_MATERIAL_VERSION) \
	&& cd document && mkdocs serve $(MKDOCS_SERVE_OPT)

.PHONY: api-docs
api-docs: crdoc manifests ## Generate docs.
	$(CRDOC) --resources config/crd/bases --output docs/api/v$(shell cat VERSION).md

.PHONY: controller-gen
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
	$(MAKE) api-docs

.PHONY: bundle
bundle: kustomize yq yamlfmt manifests ## Generate bundle manifests and metadata using base branch version
	operator-sdk generate kustomize manifests -q
	$(REPO_ROOT)/hack/add-copyright.bash $(REPO_ROOT)/hack/boilerplate.yaml.txt $(REPO_ROOT)/config/manifests/bases/spyre-operator.clusterserviceversion.yaml
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt $(REPO_ROOT)/config/manifests/bases/spyre-operator.clusterserviceversion.yaml
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE_NAME):$(BASE_VERSION)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle $(BUNDLE_GEN_FLAGS) --version $(BASE_VERSION)
	$(YQ) eval -i ".annotations[\"com.redhat.openshift.versions\"]=\"$(OCP_VERSIONS)\"" ${REPO_ROOT}/bundle/metadata/annotations.yaml
	$(YQ) eval -i ".spec.image=\"$(IMAGE_TAG_BASE)-catalog:$(BASE_VERSION)\"" ${REPO_ROOT}/config/olm/catalog-source.yaml
	find bundle -type d -exec chmod 755 {} \;
	find bundle -type f -exec chmod 644 {} \;
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt -dstar "$(REPO_ROOT)/config/**/*.yaml" "$(REPO_ROOT)/bundle/**/*.yaml" $(REPO_ROOT)/config/olm/catalog-source.yaml

.PHONY: bundle-pr
bundle-pr: manifests kustomize yq yamlfmt ## Generate bundle manifests and metadata using current branch version (PR)
	operator-sdk generate kustomize manifests -q
	$(REPO_ROOT)/hack/add-copyright.bash $(REPO_ROOT)/hack/boilerplate.yaml.txt $(REPO_ROOT)/config/manifests/bases/spyre-operator.clusterserviceversion.yaml
	$(YAMLFMT) -conf=$(REPO_ROOT)/.yamlfmt $(REPO_ROOT)/config/manifests/bases/spyre-operator.clusterserviceversion.yaml
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE)
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
		--tag $(IMAGE_TAG_BASE)-bundle:$(ADDITIONAL_IMAGE_TAG) \
		--file bundle.Dockerfile $(CURDIR)

.PHONY: bundle-push
bundle-push: ## Push bundle image.
	$(DOCKER) push $(BUNDLE_IMG)

## Build FBC from scratch
catalog/base_template.yaml:
	./catalog/fbc.sh "template" $(BUILD_TYPE) $(IMAGE_TAG_BASE)-bundle

.PHONY: fbc-gen-base-template
fbc-gen-base-template: catalog/base_template.yaml ## Generate a File Based Catalog (FBC) yaml base template

.PHONY: fbc-bundle-add
fbc-bundle-add: fbc-gen-base-template ## Add current bundle to the base template
	$(REPO_ROOT)/catalog/fbc.sh "add_bundle" "$(BUNDLE_IMG)"


.PHONY: fbc-build-setup
fbc-build-setup: opm fbc-bundle-add ## Setup for the catalog building
	mkdir -p catalog/spyre-operator
	$(OPM) alpha render-template semver catalog/base_template.yaml -oyaml --skip-tls-verify=true > catalog/spyre-operator/spyre-operator.yaml
	$(REPO_ROOT)/catalog/fbc.sh "update_fbc" "catalog/spyre-operator/spyre-operator.yaml"
	$(OPM) validate catalog/spyre-operator/

.PHONY: fbc-build
fbc-build: opm fbc-build-setup  ## Build catalog image for the build host architecture
	$(DOCKER) build $(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG) \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG) \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
	$(REPO_ROOT)/catalog/fbc.sh "gen_catalogsource_cr" "$(CATALOG_IMG)"

.PHONY: fbc-push
fbc-push: ## Push catalog image build for the build host architecture
	$(DOCKER) push $(CATALOG_IMG)


.PHONY: fbc-build-power
fbc-build-power: opm fbc-gen-base-template fbc-bundle-add fbc-build-setup # Build file catalog image for the ppc64le architecture
ifeq ($(DOCKER),docker)
	docker buildx build --platform linux/ppc64le  \
		--push --pull --no-cache  \
		--provenance false --sbom false \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-ppc64le \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-ppc64le \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
else
	podman build --platform linux/ppc64le  \
		--format docker \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-ppc64le \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-ppc64le \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
endif
.PHONY: fbc-build-amd64
fbc-build-amd64: opm fbc-gen-base-template fbc-bundle-add fbc-build-setup # Build file catalog image for the amd64 architecture
ifeq ($(DOCKER),docker)
	docker buildx build --platform linux/amd64  \
		--push --pull --no-cache  \
		--provenance false --sbom false \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-amd64 \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-amd64 \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
else
	podman build --platform linux/amd64 \
		--format docker \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-amd64 \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-amd64 \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
endif

.PHONY: fbc-build-s390x
fbc-build-s390x: opm fbc-gen-base-template fbc-bundle-add fbc-build-setup # Build file catalog image for the s390x architecture
ifeq ($(DOCKER),docker)
	docker buildx build --platform linux/s390x  \
		--push --pull --no-cache  \
		--provenance false --sbom false \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-s390x \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-s390x \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
else
	podman build --platform linux/s390x \
		--format docker \
		$(DOCKER_BUILD_OPTS) \
		--tag $(CATALOG_IMG)-s390x \
		--tag $(IMAGE_TAG_BASE)-catalog:$(ADDITIONAL_IMAGE_TAG)-s390x \
		--file $(REPO_ROOT)/catalog/catalog.Dockerfile catalog/
endif

.PHONY: fbc-push-power
fbc-push-power: # Push the power image catalog to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(CATALOG_IMG)-ppc64le
endif

.PHONY: fbc-push-s390x
fbc-push-s390x: # Push the power image catalog to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(CATALOG_IMG)-s390x
endif

.PHONY: fbc-push-amd64
fbc-push-amd64: ## Push the amd64 catalog to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(CATALOG_IMG)-amd64
endif

.PHONY: fbc-build-manifest
fbc-build-manifest:  ## Build multi architecture catalog image manifest
ifeq ($(DOCKER), docker)
	docker manifest create   $(CATALOG_IMG) $(CATALOG_IMG)-ppc64le $(CATALOG_IMG)-amd64 $(CATALOG_IMG)-s390x
	docker manifest annotate $(CATALOG_IMG) $(CATALOG_IMG)-ppc64le --os linux --arch ppc64le
	docker manifest annotate $(CATALOG_IMG) $(CATALOG_IMG)-amd64   --os linux --arch amd64
	docker manifest annotate $(CATALOG_IMG) $(CATALOG_IMG)-s390x   --os linux --arch s390x
else
	podman manifest create $(CATALOG_IMG)
	podman manifest add $(CATALOG_IMG) $(CATALOG_IMG)-ppc64le
	podman manifest add $(CATALOG_IMG) $(CATALOG_IMG)-amd64
	podman manifest add $(CATALOG_IMG) $(CATALOG_IMG)-s390x
endif
	$(REPO_ROOT)/catalog/fbc.sh "gen_catalogsource_cr" "$(CATALOG_IMG)"

.PHONY: fbc-push-manifest
fbc-push-manifest: ## Build multi architecture catalog image manifest
	$(DOCKER) manifest push $(CATALOG_IMG)

.PHONY: fbc-buildx
fbc-buildx: opm fbc-gen-base-template fbc-bundle-add fbc-build-setup fbc-build-power fbc-build-amd64 fbc-build-s390x ## Build file based catalog yaml and multi architecture image

.PHONY: fbc-pushx
fbc-pushx: fbc-push-power fbc-push-amd64 fbc-push-s390x fbc-build-manifest fbc-push-manifest ## Push catalog multi architecture image

.PHONY: fbc-build-pushx
fbc-build-pushx: opm fbc-buildx fbc-pushx ## Build and push multi architecture catalog image

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

.PHONY: test-shell-scripts
test-shell-scripts: yq yamlfmt ## Run unit tests for shell scripts.
	$(REPO_ROOT)/hack/test-get-build-type.bash
	$(REPO_ROOT)/hack/test-get-version.bash
	$(REPO_ROOT)/hack/test-propagate-version.bash

.PHONY: test
test: fmt vet ginkgo jq test-shell-scripts manifests generate envtest ## Run unit tests.
	$(call fetch-external-crds,github.com/openshift/cluster-nfd-operator,api/v1alpha1)
	$(call fetch-external-crds,github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring,v1)
	$(call fetch-external-crds,github.com/openshift/api,security/v1)
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(LOCALBIN)/ginkgo run --label-filter="!(e2e||integration)" --seed 777 --cover --coverprofile=coverage-report.out --json-report unittest-report.json  ./controllers/... ./pkg/... ./internal/...
	PATH=${PATH}:$(LOCALBIN) $(REPO_ROOT)/hack/convert-to-markdown.sh unittest-report "Unit Tests"
	go tool cover -func coverage-report.out
	go tool cover -html coverage-report.out -o coverage-report.html
	@percentage=$$(go tool cover -func=coverage-report.out | grep ^total | awk '{print $$3}' | tr -d '%'); \
		if (( $$(echo "$$percentage < $(CODECOV_PERCENT)" | bc -l) )); then \
			echo "----------"; \
			echo "Total test coverage ($${percentage}%) is less than the coverage threshold ($(CODECOV_PERCENT)%)."; \
			exit 1; \
		fi


.PHONY: integration-test
integration-test: ginkgo jq ensure-deps ## Run integration test on the cluster pointed to in the current KUBECONFIG (expecting NFD instance running)
	$(YQ) eval -i '.repository="${REGISTRY}/${NAMESPACE}"' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.pseudoDeviceMode=false' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.devicePluginInit.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.devicePluginInit.executePolicy="Always"' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.podValidator.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.exporter.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.healthChecker.enabled=true' ${REPO_ROOT}/test/config.yaml
	$(YQ) eval -i '.hasDevice=true' ${REPO_ROOT}/test/config.yaml
	OC=$(OC) $(GINKGO) run --label-filter=$(INTEGRATION_TEST_LABEL) --seed 777 --cover --coverprofile=coverage-report.out --json-report integration-test-report.json -v ./test/integration/...
	PATH=${PATH}:$(LOCALBIN) ./hack/convert-to-markdown.sh integration-test-report "Integration Tests"

.PHONY: integration-test-spec-md
integration-test-spec-md: ginkgo
	$(GINKGO) run --dry-run --succinct --seed 777 --json-report=integration-test-report.json ./test/integration/ >/dev/null && hack/convert-to-markdown.sh integration-test-report "Integration Tests"

.PHONY: capture-logs
capture-logs: stern ## Capture the spyre operator component pod logs from the cluster pointed to in the current KUBECONFIG
	$(STERN) -n $(OPERATOR_NAMESPACE) .

.PHONY: e2e-test
e2e-test: ginkgo jq ensure-deps ## Run e2e test on the cluster pointed to in the current KUBECONFIG (expecting NFD instance running)
	$(info TEST_CONFIG is set to $(TEST_CONFIG))
	$(info E2E_KUBECONFIG is set to $(E2E_KUBECONFIG))
	$(GINKGO) run --timeout=2h --label-filter="e2e" --cover --coverprofile=coverage-report.out --json-report e2e-test-report.json -v ./test/e2e/...
	PATH=${PATH}:$(LOCALBIN) ./hack/convert-to-markdown.sh e2e-test-report "E2E Tests"

.PHONY: e2e-test-spec-md
e2e-test-spec-md: ginkgo
	$(GINKGO) run --dry-run --succinct --json-report=e2e-test-report.json ./test/e2e/ >/dev/null && hack/convert-to-markdown.sh e2e-test-report "E2E Tests"

## Set global image pull-secret
.PHONY: configure-global-pull-secret
GPS_FILE := $(shell mktemp)
REG_AUTH_FILE := $(shell mktemp -u)
NEW_GPS_FILE := $(shell mktemp)
AUTH_VALUE_FILE := $(shell mktemp)
configure-global-pull-secret: jq
	$(OC) get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' > $(GPS_FILE)
	$(OC) registry login --registry="$(REGISTRY)" --auth-basic="$(REGISTRY_USERNAME):$(REGISTRY_TOKEN)" --to=$(REG_AUTH_FILE)
	cat $(REG_AUTH_FILE) | $(JQ) -r '.auths["$(REGISTRY)"].auth' > $(AUTH_VALUE_FILE)
	$(JQ) ".auths |= .+ {\"$(REGISTRY)\":{\"auth\": \"$$(cat ${AUTH_VALUE_FILE})\"}}" < $(GPS_FILE) > $(NEW_GPS_FILE)
	$(OC) set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=$(NEW_GPS_FILE)

##@ Redhat certified operator targets

.PHONY: preflight
preflight:
ifneq ($(shell uname -s),Linux)
	@echo "preflight requires at lease RHEL 8.5, CentOS 8.5 or later Fedora 35"
	@echo "check requirements: https://github.com/redhat-openshift-ecosystem/openshift-preflight?tab=readme-ov-file#requirements"
	@false
endif
ifeq (,$(shell which preflight 2>/dev/null))
	@echo "preflight command not installed; see https://github.com/redhat-openshift-ecosystem/openshift-preflight?tab=readme-ov-file#installation"
	@false
endif

.PHONY: preflight-check
preflight-check: preflight ## Do preflight check
ifeq (,$(PFLT_PYXIS_API_TOKEN))
	@echo "PFLT_PYXIS_API_TOKEN is not defined; see https://access.redhat.com/documentation/en-us/red_hat_software_certification/2024/html-single/red_hat_software_certification_workflow_guide/index#proc_running-the-certification-test-suite_openshift-sw-cert-workflow-working-with-containers"
	@false
endif
ifeq (,$(RH_COMPONENT_ID))
	@echo "RH_COMPONENT_ID is not defined; declare component ID of operator container image (\"RH_COMPONENT_ID=6XXXXXXX... make preflight-check\")"
	@false
endif
	preflight check container $(IMAGE) --certification-component-id=$(RH_COMPONENT_ID) --docker-config $${XDG_RUNTIME_DIR}/containers/auth.json --submit

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

.PHONY: propagate-base-version
propagate-base-version: yq yamlfmt ## Propagate base version version to all required files
	hack/propagate-version.bash $(BASE_VERSION) $(REGISTRY) $(NAMESPACE) $(DEFAULT_CHANNEL)

.PHONY: propagate-version
propagate-version: yq yamlfmt ## Propagate version to all required files
	hack/propagate-version.bash $(VERSION) $(REGISTRY) $(NAMESPACE) $(DEFAULT_CHANNEL)

.PHONY: pr
pr: fmt vet lint test docker-build docker-push propagate-version bundle-pr bundle-validate bundle-build bundle-push fbc-bundle-add fbc-build fbc-push ## Execute a pull request build locally

##@ Image operations

.PHONY: docker-build
docker-build: vendor ## Build sypre operator image for the build host architecture
	$(DOCKER) build $(DOCKER_BUILD_OPTS) --pull \
	--tag $(IMAGE) \
	--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG) \
	--build-arg VERSION="$(VERSION)" \
	--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
	--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
	--file $(DOCKERFILE) $(CURDIR)

.PHONY: docker-push
docker-push: ## Push spyre operator image image for the build host architecture
	$(DOCKER) push $(IMAGE)

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push the spyre operator image for the build host

.PHONY: synch-to-registry
synch-to-registry: ## Synchronize the image from the source to the target registry
	skopeo copy --multi-arch all --preserve-digests docker://$(IMAGE) docker://$(ALTERNATE_REGISTRY)/$(ALTERNATE_NAMESPACE)/spyre-operator:$(IMAGE_TAG)
	skopeo copy --multi-arch all --preserve-digests docker://$(BUNDLE_IMG) docker://$(ALTERNATE_REGISTRY)/$(ALTERNATE_NAMESPACE)/spyre-operator-bundle:$(IMAGE_TAG)
	skopeo copy --multi-arch all --preserve-digests docker://$(CATALOG_IMG) docker://$(ALTERNATE_REGISTRY)/$(ALTERNATE_NAMESPACE)/spyre-operator-catalog:$(IMAGE_TAG)

.PHONY: docker-build-power
docker-build-power: vendor ## Build the spyre operator image image for linux/ppc64le
# Building with the -race flag breaks builds on Power, thus not including the --build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)"
ifeq ($(DOCKER), docker)
	docker buildx build --platform linux/ppc64le \
		$(DOCKER_BUILD_OPTS) \
		--push --pull  --no-cache \
		--provenance false --sbom false \
		--tag $(IMAGE)-ppc64le \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-ppc64le \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--file $(DOCKERFILE) $(CURDIR)
else
	podman build --platform linux/ppc64le \
		$(DOCKER_BUILD_OPTS) \
		--format docker \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--tag $(IMAGE)-ppc64le \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-ppc64le \
		--file $(DOCKERFILE) $(CURDIR)
endif

.PHONY: docker-build-amd64
docker-build-amd64: vendor ## Build the spyre operator image image for linux/amd64
ifeq ($(DOCKER), docker)
	docker buildx build --platform linux/amd64 \
		$(DOCKER_BUILD_OPTS) \
		--push --pull  --load --no-cache \
		--provenance false --sbom false \
		--tag $(IMAGE)-amd64 \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-amd64 \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
		--file $(DOCKERFILE) $(CURDIR)
else
	podman build --platform linux/amd64 \
		$(DOCKER_BUILD_OPTS) \
		--format docker \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--tag $(IMAGE)-amd64 \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-amd64 \
		--file $(DOCKERFILE) $(CURDIR)
endif

.PHONY: docker-build-s390x
docker-build-s390x: vendor ## Build the spyre operator image image for linux/s390x
ifeq ($(DOCKER), docker)
	docker buildx build --platform linux/s390x \
		$(DOCKER_BUILD_OPTS) \
		--push --pull --load \
		--provenance false --sbom false \
		--tag $(IMAGE)-s390x \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-s390x \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
		--file $(DOCKERFILE) $(CURDIR)
else
	podman build --platform linux/s390x \
		$(DOCKER_BUILD_OPTS) \
		--format docker \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_FLAGS="$(DOCKER_GO_BUILD_FLAGS)" \
		--build-arg BUILDER_IMAGE="$(BUILDER_IMAGE)" \
		--tag $(IMAGE)-s390x \
		--tag $(IMAGE_NAME):$(ADDITIONAL_IMAGE_TAG)-s390x \
		--file $(DOCKERFILE) $(CURDIR)
endif

.PHONY: docker-push-power
docker-push-power: ## Push the ppc64le spyre operator image to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(IMAGE)-ppc64le
endif

.PHONY: docker-push-s390x
docker-push-s390x: ## Push the s390x spyre operator to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(IMAGE)-s390x
endif

.PHONY: docker-push-amd64
docker-push-amd64: ## Push the amd64 spyre operator to the registry
ifeq ($(DOCKER), docker)
	echo "Images already pushed by Docker"
else
	$(DOCKER) push $(IMAGE)-amd64
endif

.PHONY: docker-build-manifest
docker-build-manifest: ## Build spyre operator image manifest for all architectures
ifeq ($(DOCKER), docker)
	docker manifest create $(IMAGE) $(IMAGE)-ppc64le $(IMAGE)-amd64 $(IMAGE)-s390x
	docker manifest annotate $(IMAGE) $(IMAGE)-ppc64le --os linux --arch ppc64le
	docker manifest annotate $(IMAGE) $(IMAGE)-amd64 --os linux --arch amd64
	docker manifest annotate $(IMAGE) $(IMAGE)-s390x --os linux --arch s390x
else
	podman manifest create $(IMAGE)
	podman manifest add $(IMAGE) $(IMAGE)-ppc64le
	podman manifest add $(IMAGE) $(IMAGE)-amd64
	podman manifest add $(IMAGE) $(IMAGE)-s390x
endif

.PHONY: docker-push-manifest
docker-push-manifest: ## Push spyre operator manifest for all architectures
	$(DOCKER) manifest push $(IMAGE)

.PHONY: docker-buildx
docker-buildx: docker-build-power docker-build-amd64 docker-build-s390x  ## Build spyre operator image image for all architectures

.PHONY: docker-pushx ## Push spyre operator image image for all architectures
docker-pushx: docker-push-power docker-push-amd64 docker-push-s390x docker-build-manifest docker-push-manifest

.PHONY: docker-build-pushx
docker-build-pushx: docker-buildx docker-pushx ## Build and push the multi architecture spyre operator image

.PHONY: docker-remove-images
docker-remove-images: ## Remove images from build host
	$(DOCKER) rmi -f $(BUNDLE_IMG) || true
	$(DOCKER) manifest rm $(IMAGE) || true
	$(DOCKER) manifest rm $(CATALOG_IMG) || true
	$(DOCKER) rmi -f $(IMAGE)-ppc64le $(IMAGE)-amd64 $(IMAGE)-s390x || true
	$(DOCKER) rmi -f $(CATALOG_IMG)-ppc64le $(CATALOG_IMG)-amd64 $(CATALOG_IMG)-s390x || true

## TT scan

TT_URL := https://na.artifactory.swg-devops.com/artifactory/css-ets-scs-consec-team-public-generic-local/Twistlock%20Executable/tt_latest.zip
TT_DIR := $(LOCALBIN)/tt_*/linux_x86_64
TT_BIN := $(TT_DIR)/tt
W3_TT_URL := https://w3twistlock.sos.ibm.com

.PHONY: tt-install
tt-install: $(TT_BIN) ## Download and install Twistlock scanner (tt)

$(TT_BIN): $(LOCALBIN)
	@echo "Downloading tt for Linux x86_64..."
	curl --retry 30 -u "$(ARTIFACTORY_USER):$(ARTIFACTORY_PASS)" \
	     --silent --fail --location "$(TT_URL)" \
	     --output $(LOCALBIN)/tt_latest.zip
	unzip -qo $(LOCALBIN)/tt_latest.zip -d $(LOCALBIN) > /dev/null
	chmod 755 $(TT_BIN)
	$(TT_BIN) check-dependencies > /dev/null

.PHONY: tt-scan-amd64
tt-scan-amd64:
	@echo "Scanning amd64 image: $(IMAGE)-amd64"
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" $(IMAGE)-amd64 > /dev/null 2>&1 && awk -F, -v col=25 'NR==1 || $$col == "Y"' "$$(ls -t twistlock-scan-results-*.results.csv | head -1)"

.PHONY: tt-scan-s390x
tt-scan-s390x:
	@echo "Scanning s390x image: $(IMAGE)-s390x"
	mkdir -pv ./twistlock-scan-output/s390x
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --has-fix-filter Y --output-file image-scan $(IMAGE)-s390x
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --output-file image-scan $(IMAGE)-s390x
else
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --output-file image-scan $(IMAGE)-s390x
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/s390x/image-scan.results.csv
	@echo "------------------------------------------------"

.PHONY: tt-scan-bundle-s390x
tt-scan-bundle-s390x:
	@echo "Scanning bundle image: $(BUNDLE_IMG)"
	mkdir -pv ./twistlock-scan-output/s390x
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --has-fix-filter Y --output-file bundle-scan $(BUNDLE_IMG)
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --output-file bundle-scan $(BUNDLE_IMG)
else
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --output-file bundle-scan $(BUNDLE_IMG)
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/s390x/bundle-scan.results.csv
	@echo "------------------------------------------------"

.PHONY: tt-scan-fbc-s390x
tt-scan-fbc-s390x:
	@echo "Scanning fbc s390x image: $(CATALOG_IMG)-s390x"
	mkdir -pv ./twistlock-scan-output/s390x
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir ./twistlock-scan-output/s390x --has-fix-filter Y --output-file catalog-scan $(CATALOG_IMG)-s390x
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir ./twistlock-scan-output/s390x --output-file catalog-scan $(CATALOG_IMG)-s390x
else
	$(TT_BIN) images pull-and-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/s390x --output-file catalog-scan $(CATALOG_IMG)-s390x
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/s390x/catalog-scan.results.csv
	@echo "------------------------------------------------"

.PHONY: tt-scan-ppc64le
tt-scan-ppc64le:
	@echo "Scanning ppc64le image: $(IMAGE)-ppc64le"
	mkdir -pv ./twistlock-scan-output/ppc64le
	$(DOCKER) pull $(IMAGE)-ppc64le
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --has-fix-filter Y --output-file image-scan $(IMAGE)-ppc64le
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --output-file image-scan $(IMAGE)-ppc64le
else
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --output-file image-scan $(IMAGE)-ppc64le
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/ppc64le/image-scan.results.csv
	@echo "------------------------------------------------"

.PHONY: tt-scan-bundle-ppc64le
tt-scan-bundle-ppc64le:
	@echo "Scanning bundle image: $(BUNDLE_IMG)"
	mkdir -pv ./twistlock-scan-output/ppc64le
	$(DOCKER) pull $(BUNDLE_IMG)
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --has-fix-filter Y --output-file bundle-scan $(BUNDLE_IMG)
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --output-file bundle-scan $(BUNDLE_IMG)
else
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --output-file bundle-scan $(BUNDLE_IMG)
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/ppc64le/bundle-scan.results.csv
	@echo "------------------------------------------------"

.PHONY: tt-scan-fbc-ppc64le
tt-scan-fbc-ppc64le:
	@echo "Scanning fbc ppc64le image: $(CATALOG_IMG)-ppc64le"
	mkdir -pv ./twistlock-scan-output/ppc64le
	$(DOCKER) pull $(CATALOG_IMG)-ppc64le
ifeq ($(BUILD_TYPE),pr)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir ./twistlock-scan-output/ppc64le --has-fix-filter Y --output-file catalog-scan $(CATALOG_IMG)-ppc64le
else ifeq ($(BUILD_TYPE),development)
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype nonprod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir ./twistlock-scan-output/ppc64le --output-file catalog-scan $(CATALOG_IMG)-ppc64le
else
	$(TT_BIN) images  local-scan --user $(TT_USER) --url "$(W3_TT_URL)" --control-group $(TT_CONTROL_GROUP) --imagetype prod --iam-api-key "$(TWIST_LOCK_API_KEY)" --output-dir twistlock-scan-output/ppc64le --output-file catalog-scan $(CATALOG_IMG)-ppc64le
endif
	@echo "Scan results:"
	@echo "------------------------------------------------"
	cat ./twistlock-scan-output/ppc64le/catalog-scan.results.csv
	@echo "------------------------------------------------"


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

.PHONY: uninstall-operator
uninstall-operator: ## Uninstall operator via olm and its corresponding resources
	@$(REPO_ROOT)/hack/install-operator.bash uninstall $(OPERATOR_NAMESPACE) ARCH=${ARCH}

.PHONE: power-apply-machineconfig
power-apply-machineconfig: ## Apply MachineConfig resources for Power architecture
	$(KUBECTL) apply -f $(REPO_ROOT)/config/machineconfig/ppc64le

.PHONE: power-delete-machineconfig
power-delete-machineconfig: ## Apply MachineConfig resources for Power architecture
	$(KUBECTL) delete -f $(REPO_ROOT)/config/machineconfig/ppc64le

.PHONY: power-update-machine-configs
power-update-machine-configs: butane ## Create Machine Configs from template/source
	$(REPO_ROOT)/hack/power-update-machine-configs.sh $(BUTANE) $(REPO_ROOT) $(REPO_ROOT)/hack/bind-vfio.sh

.PHONY: s390x-apply-machineconfig
s390x-apply-machineconfig: ## Apply MachineConfig resources for S390X architecture
	$(KUBECTL) apply -f $(REPO_ROOT)/config/machineconfig/s390x

.PHONY: s390x-delete-machineconfig
s390x-delete-machineconfig: ## Delete MachineConfig resources for S390X architecture
	$(KUBECTL) delete -f $(REPO_ROOT)/config/machineconfig/s390x

##@ Release targets

.PHONY: echo-version
echo-version: ## Print (echo) the current version
	$(info $(VERSION))
	@echo > /dev/null

.PHONY: increment-patch-version
increment-patch-version: ## Increment patch version and create branch
	$(REPO_ROOT)/hack/increment-version.bash --patch
	$(REPO_ROOT)/hack/create-branch.bash --type version-upgrade

.PHONY: increment-minor-version
increment-minor-version: ## Increment minor version and create branch
	$(REPO_ROOT)/hack/increment-version.bash --minor
	$(REPO_ROOT)/hack/create-branch.bash --type version-upgrade

.PHONY: increment-major-version
increment-major-version: ## Increment major version and create branch
	$(REPO_ROOT)/hack/increment-version.bash --major
	$(REPO_ROOT)/hack/create-branch.bash --type version-upgrade

.PHONY: minor-release-branch
minor-release-branch: ## Create a minor release branch (i.e release_v2.3.0)
	$(REPO_ROOT)/hack/create-branch.bash --type minor-release

.PHONY: major-release-branch
major-release-branch: ## Create a minor release branch (i.e release_v3.0.0)
	$(REPO_ROOT)/hack/create-branch.bash --type major-release

.PHONY: patch-release-branch
patch-release-branch: ## Create a patch release branch from an existing release branch
	$(REPO_ROOT)/hack/create-branch.bash --type patch-release

#find the last rc number based upon the tags published
LAST_RC_NUMBER ?= $(shell $(REPO_ROOT)/hack/get-last-rc-number.bash)
.PHONY: release-candidate-branch
release-candidate-branch: ## Create a release branch (i.e release_v2.2.0)
	$(REPO_ROOT)/hack/create-branch.bash --type rc $(LAST_RC_NUMBER)

.PHONY: github-release
github-release: ## Create a GitHub release for the tag and branch
	$(REPO_ROOT)/hack/create-gh-release.bash --build-type $(BUILD_TYPE) --version $(VERSION)

.PHONY: detect-secrets-install
detect-secrets-install: venv ## Install detect-secret tool
	$(eval TMP_CONSTRAINTS := $(shell mktemp))
	@echo "boxsdk<4" > $(TMP_CONSTRAINTS)
	@echo "chardet<6" >> $(TMP_CONSTRAINTS)
	. venv/bin/activate && $(PIP) install "git+$(DETECT_SECRETS_GIT)" -c $(TMP_CONSTRAINTS)
	@rm -f $(TMP_CONSTRAINTS)

.PHONY: secrets-scan
secrets-scan: detect-secrets-install venv ## Scan secrets and create secret-baseline for repo
	. venv/bin/activate; detect-secrets scan --exclude-files go.sum --update .secrets.baseline

.PHONY: secrets-audit
secrets-audit: detect-secrets-install venv ## Audit secrets
	. venv/bin/activate; detect-secrets audit .secrets.baseline

# helper target for viewing the value of makefile variables.
print-%  : ;@echo $* = $($*)
