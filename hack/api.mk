.PHONY: api-lint
api-lint: kubevious ## Lint all APIs (K8s CRDs) and samples
	$(KUBEVIOUS) guard config/crd config/samples

.PHONY: api-helm-build
api-helm-build: manifests kustomize helm ## Build Fabric API (CRDs) Helm chart
	$(KUSTOMIZE) build config/crd > config/helm/fabric-api/templates/crds.yaml
	$(HELM) package config/helm/fabric-api --destination config/helm --version $(VERSION)

HELM_CRD_PACKAGE ?= config/helm/fabric-api-$(VERSION).tgz

.PHONY: api-helm-lint
api-helm-lint: helm api-helm-build ## Lint Fabric API (CRDs) Helm chart
	$(HELM) lint $(HELM_CRD_PACKAGE)

.PHONY: api-helm-push
api-helm-push: api-helm-build helm ## Push Fabric API (CRDs) Helm chart
	$(HELM) push $(HELM_CRD_PACKAGE) $(HELM_REPO_URL)

.PHONY: api-helm-install
api-helm-install: api-helm-build helm ## Install Fabric API (CRDs) Helm chart
	$(HELM) upgrade --install fabric-api $(HELM_CRD_PACKAGE)

.PHONY: api-helm-uninstall
api-helm-uninstall: api-helm-build helm ## Uninstall Fabric API (CRDs) Helm chart
	$(HELM) uninstall fabric-api
