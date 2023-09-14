##@ Fabric API (CRDs)

.PHONY: api
api: generate manifests api-samples api-lint api-helm-build ## Build and lint all APIs (K8s CRDs) including samples

.PHONY: api-samples
api-samples: ## Generate all API (K8s CRDs) samples
	rm -rf config/samples/*.gen.yaml
	go run ./cmd/hhf wiring sample --type=collapsedcore --preset vlab > config/samples/collapsedcore.vlab.gen.yaml
	go run ./cmd/hhf wiring sample --type=collapsedcore --preset lab > config/samples/collapsedcore.lab.gen.yaml


.PHONY: api-lint
api-lint: kubevious ## Lint all APIs (K8s CRDs) and samples
	$(KUBEVIOUS) guard config/crd config/samples

.PHONY: api-lint-crds
api-lint-crds: kubevious ## Lint all APIs (K8s CRDs)
	$(KUBEVIOUS) guard config/crd

API_HELM ?= config/helm/fabric-api
API_HELM_PACKAGE ?= $(API_HELM)-$(VERSION).tgz

.PHONY: api-helm-build
api-helm-build: manifests kustomize helm ## Build Fabric API (CRDs) Helm chart
	rm $(API_HELM)-*.tgz || true
	$(KUSTOMIZE) build config/crd > $(API_HELM)/templates/crds.yaml
	$(HELM) package $(API_HELM) --destination config/helm --version $(VERSION)
	$(HELM) lint $(API_HELM_PACKAGE)

.PHONY: api-helm-push
api-helm-push: api-helm-build helm ## Push Fabric API (CRDs) Helm chart
	$(HELM) push $(API_HELM_PACKAGE) $(HELM_REPO_URL)

.PHONY: api-helm-install
api-helm-install: api-helm-build helm ## Install Fabric API (CRDs) Helm chart
	$(HELM) upgrade --install fabric-api $(API_HELM_PACKAGE)

.PHONY: api-helm-uninstall
api-helm-uninstall: api-helm-build helm ## Uninstall Fabric API (CRDs) Helm chart
	$(HELM) uninstall fabric-api
