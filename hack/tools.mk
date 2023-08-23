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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ Build Dependencies & Dev Tools

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
ACTIONLINT ?= $(LOCALBIN)/actionlint
KUBEVIOUS ?= PATH=./bin $(LOCALBIN)/kubevious
CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs
HELM ?= $(LOCALBIN)/helm
ORAS ?= $(LOCALBIN)/oras

## Tool Versions
KUSTOMIZE_VERSION ?= v5.0.1
CONTROLLER_TOOLS_VERSION ?= v0.12.0
ENVTEST_K8S_VERSION = 1.27.1 # Version of kubebuilder assets to be downloaded by envtest binary
ACTIONLINT_VERSION ?= v1.6.22
CRD_REF_DOCS_VERSION ?= master # TODO: install specific version 
ORAS_VERSION ?= v1.0.1

.PHONY: tools
tools: kustomize controller-gen envtest envtest-k8s kubevious crd-ref-docs actionlint helm oras ## Prepare all tools

# TODO: Enable back version check when it'll start returning version instead of (devel)
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	# @if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
	# 	echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
	# 	rm -rf $(LOCALBIN)/kustomize; \
	# fi
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: envtest-k8s
envtest-k8s: envtest ## Download envtest assets if necessary.
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path
	chmod -R u+w $(LOCALBIN)/k8s

# TODO: Install specific version
KUBEVIOUS_INSTALL_SCRIPT ?= "https://get.kubevious.io/cli.sh"
.PHONY: kubevious
kubevious: $(KUBEVIOUS) ## Download kustomize locally if necessary.
$(KUBEVIOUS): $(LOCALBIN)
	test -s $(LOCALBIN)/kubevious || { curl -Ss $(KUBEVIOUS_INSTALL_SCRIPT) | bash -s -- $(LOCALBIN); }

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	test -s $(LOCALBIN)/crd-ref-docs || GOBIN=$(LOCALBIN) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

.PHONY: actionlint
actionlint: $(ACTIONLINT) ## Download actionlint locally if necessary.
$(ACTIONLINT): $(LOCALBIN)
	test -s $(LOCALBIN)/actionlint || GOBIN=$(LOCALBIN) go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# TODO: Install specific version
HELM_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3"
.PHONY: helm
helm: $(HELM)
$(HELM): $(LOCALBIN)
	test -s $(LOCALBIN)/helm || { curl -fsSL $(HELM_INSTALL_SCRIPT) | HELM_INSTALL_DIR=$(LOCALBIN) USE_SUDO=false PATH=bin:$(PATH) bash -s - ; }

.PHONY: oras
oras: $(ORAS) ## Download oras locally if necessary.
$(ORAS): $(LOCALBIN)
	test -s $(LOCALBIN)/oras || GOBIN=$(LOCALBIN) go install oras.land/oras/cmd/oras@$(ORAS_VERSION)
