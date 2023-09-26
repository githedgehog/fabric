##@ Fabric

.PHONY: fabric-build
fabric-build: ## Build fabric binary
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/fabric -ldflags="-w -s -X main.version=$(VERSION)" ./cmd/main.go

.PHONY: fabric-image-build
fabric-image-build: fabric-build ## Build fabric image
	docker build -t $(OCI_REPO)/fabric:$(VERSION) -f Dockerfile .

.PHONY: fabric-image-push
fabric-image-push: fabric-image-build ## Push fabric image
	docker push $(OCI_REPO)/fabric:$(VERSION)

.PHONY: fabric-image-push-dev
fabric-image-push-dev: fabric-image-build ## Push fabric image
	skopeo copy --dest-tls-verify=false docker-daemon:$(OCI_REPO)/fabric:$(VERSION) docker://$(OCI_REPO)/fabric:$(VERSION)

.PHONY: fabric-chart-build
fabric-chart-build: ## Build fabric chart
	rm config/helm/fabric-*.tgz || true
	rm -rf config/helm/fabric/templates/*.yaml config/helm/fabric/values.yaml
	$(KUSTOMIZE) build config/default | $(HELMIFY) config/helm/fabric
	$(HELM) package config/helm/fabric --destination config/helm --version $(VERSION)

.PHONY: fabric-chart-push
fabric-chart-push: fabric-chart-build ## Push fabric chart
	$(HELM) push config/helm/fabric-$(VERSION).tgz oci://$(OCI_REPO)/charts

.PHONY: fabric-chart-push-dev
fabric-chart-push-dev: fabric-chart-build ## Push fabric chart
	$(HELM) push --insecure-skip-tls-verify config/helm/fabric-$(VERSION).tgz oci://$(OCI_REPO)/charts

.PHONY: agent-build
agent-build: ## Build agent
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent -ldflags="-w -s -X main.version=$(VERSION)" ./cmd/agent/main.go

.PHONY: agent-push
agent-push: agent-build ## Push agent
	cd bin && oras push $(OCI_REPO)/agent:$(VERSION) agent

.PHONY: agent-push-dev
agent-push-dev: agent-build ## Push agent
	cd bin && oras push --insecure registry.local:31000/githedgehog/agent/x86_64:latest agent
	cd bin && oras push --insecure $(OCI_REPO)/agent:$(VERSION) agent

.PHONY: hhfctl-build
hhfctl-build: ## Build hhfctl
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/hhfctl -ldflags="-w -s -X main.version=$(VERSION)" ./cmd/hhfctl/main.go

.PHONY: hhfctl-push
hhfctl-push: hhfctl-build ## Push hhfctl
	cd bin && oras push $(OCI_REPO)/hhfctl:$(VERSION) hhfctl
	cd bin && oras push $(OCI_REPO)/hhfctl:latest hhfctl

.PHONY: hhfctl-push-dev
hhfctl-push-dev: hhfctl-build ## Push hhfctl
	cd bin && oras push --insecure $(OCI_REPO)/hhfctl:$(VERSION) hhfctl

.PHONY: dev-push
dev-push: api-chart-push-dev fabric-image-push-dev fabric-chart-push-dev agent-push-dev hhfctl-push-dev

.PHONY: build
build: api-chart-build fabric-image-build fabric-chart-build agent-build hhfctl-build

.PHONY: push
push: api-chart-push fabric-image-push fabric-chart-push agent-push hhfctl-push

.PHONY: dev-patch
dev-patch:
	kubectl patch helmchart fabric-api --type=merge -p '{"spec":{"version":"$(VERSION)"}}'
	kubectl patch helmchart fabric --type=merge -p '{"spec":{"version":"$(VERSION)", "set":{"controllerManager.manager.image.tag":"$(VERSION)"}}}'

.PHONY: dev
dev:
	VERSION=$(VERSION)-$(shell date +%s) make dev-push dev-patch