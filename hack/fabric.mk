##@ Fabric

.PHONY: fabric-build
fabric-build: ## Build fabric binary
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/fabric -ldflags="-w -s -X main.version=$(VERSION)" ./cmd/main.go

.PHONY: fabric-image-build
fabric-image-build: fabric-build ## Build fabric image
	docker build -t $(OCI_REPO)/fabric:$(VERSION) -f Dockerfile .

.PHONY: fabric-image-push-local
fabric-image-push-local: fabric-image-build ## Push fabric image (using skopeo)
	skopeo copy docker-daemon:$(OCI_REPO)/fabric:$(VERSION) docker://$(OCI_REPO)/fabric:$(VERSION)

.PHONY: fabric-image-push
fabric-image-push: fabric-image-build ## Push fabric image (using skopeo)
	docker push $(OCI_REPO)/fabric:$(VERSION)


.PHONY: fabric-chart-build
fabric-chart-build: ## Build fabric chart
	rm -rf config/helm/fabric/templates/*.yaml config/helm/fabric/values.yaml
	$(KUSTOMIZE) build config/default | $(HELMIFY) config/helm/fabric
	$(HELM) package config/helm/fabric --destination config/helm --version $(VERSION)

.PHONY: fabric-chart-push
fabric-chart-push: fabric-chart-build ## Push fabric chart
	$(HELM) push config/helm/fabric-$(VERSION).tgz oci://$(OCI_REPO)/charts

.PHONY: agent-build
agent-build: ## Build agent
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent -ldflags="-w -s -X main.version=$(VERSION)" ./cmd/agent/main.go

.PHONY: agent-push
agent-push: agent-build ## Push agent to the control node
	cd bin && oras push --insecure $(OCI_REPO)/agent:$(VERSION) agent

.PHONY: agent-push-dev
agent-push-dev: agent-build ## Push agent to the control node # TODO
	cd bin && oras push --insecure registry.local:31000/githedgehog/agent/x86_64:latest agent