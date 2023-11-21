##@ Fabric API (CRDs)

.PHONY: api
api: generate manifests api-lint api-chart-build ## Build and lint all APIs (K8s CRDs) including samples

.PHONY: api-lint
api-lint: kubevious ## Lint all APIs (K8s CRDs)
	$(KUBEVIOUS) guard config/crd

.PHONY: api-lint-crds
api-lint-crds: kubevious ## Lint all APIs (K8s CRDs)
	$(KUBEVIOUS) guard config/crd

API_HELM ?= config/helm/fabric-api
API_HELM_PACKAGE ?= $(API_HELM)-$(VERSION).tgz

.PHONY: api-chart-build
api-chart-build: generate manifests kustomize helm ## Build Fabric API (CRDs) Helm chart
	rm $(API_HELM)-*.tgz || true
	$(KUSTOMIZE) build config/crd > $(API_HELM)/templates/crds.yaml
	$(HELM) package $(API_HELM) --destination config/helm --version $(VERSION)
	$(HELM) lint $(API_HELM_PACKAGE)

.PHONY: api-chart-push
api-chart-push: api-chart-build helm ## Push Fabric API (CRDs) Helm chart
	$(HELM) push $(API_HELM_PACKAGE) oci://$(OCI_REPO)/charts

.PHONY: api-chart-push-dev
api-chart-push-dev: api-chart-build helm ## Push Fabric API (CRDs) Helm chart
	$(HELM) push --insecure-skip-tls-verify $(API_HELM_PACKAGE) oci://$(OCI_REPO)/charts

.PHONY: api-chart-install
api-chart-install: api-chart-build helm ## Install Fabric API (CRDs) Helm chart
	$(HELM) upgrade --install fabric-api $(API_HELM_PACKAGE)

# .PHONY: api-helm-install
# api-helm-install: api-helm-build helm ## Install Fabric API (CRDs) Helm chart
# 	$(HELM) upgrade --install fabric-api $(API_HELM_PACKAGE)

# .PHONY: api-helm-uninstall
# api-helm-uninstall: api-helm-build helm ## Uninstall Fabric API (CRDs) Helm chart
# 	$(HELM) uninstall fabric-api
