set shell := ["bash", "-euo", "pipefail", "-c"]

import "hack/tools.just"

# Print list of available recipes
default:
  @just --list

export CGO_ENABLED := "0"

_gotools: _touch_embed
  go fmt ./...
  go vet {{go_flags}} ./...

# Called in CI
_lint: _license_headers _gotools

# Generate, lint, test and build everything
all: gen docs lint lint-gha test build kube-build && version

# Run linters against code (incl. license headers)
lint: _lint _golangci_lint
  {{golangci_lint}} run --show-stats ./...

# Run golangci-lint to attempt to fix issues
lint-fix: _lint _golangci_lint
  {{golangci_lint}} run --show-stats --fix ./...

go_flags := "--tags containers_image_openpgp,containers_image_storage_stub -ldflags=\"-w -s -X go.githedgehog.com/fabric/pkg/version.Version=" + version + "\""
go_build := "go build " + go_flags
go_linux_build := "GOOS=linux GOARCH=amd64 " + go_build

_touch_embed:
  @touch pkg/boot/nosinstall/bin/fabric-nos-install

_embed: _touch_embed
  # Build fabric-nos-install binary for embedding
  {{go_linux_build}} -o ./pkg/boot/nosinstall/bin/fabric-nos-install ./cmd/fabric-nos-install

_kube_gen: _controller_gen
  # Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject implementations
  {{controller_gen}} object:headerFile="hack/boilerplate.go.txt" paths="./..."
  # Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
  {{controller_gen}} rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate docs, code/manifests, things to embed, etc
gen: _kube_gen _embed _crd_ref_docs
  {{crd_ref_docs}} --source-path=./api/ --config=api/docs.config.yaml --renderer=markdown --output-path=./docs/api.md
  go run cmd/fabric-gen/main.go profiles-ref

# Build all artifacts
build: _license_headers _kube_gen _gotools _embed && version
  {{go_linux_build}} -o ./bin/fabric ./cmd
  {{go_linux_build}} -o ./bin/agent ./cmd/agent
  {{go_linux_build}} -o ./bin/hhfctl ./cmd/hhfctl
  {{go_linux_build}} -o ./bin/fabric-boot ./cmd/fabric-boot
  {{go_linux_build}} -o ./bin/fabric-dhcpd ./cmd/fabric-dhcpd
  # Build complete

oci_repo := "127.0.0.1:30000"
oci_prefix := "githedgehog/fabric"

_helm-fabric-api: _kustomize _helm _kube_gen
  @rm config/helm/fabric-api-v*.tgz || true
  {{kustomize}} build config/crd > config/helm/fabric-api/templates/crds.yaml
  {{helm}} package config/helm/fabric-api --destination config/helm --version {{version}}
  {{helm}} lint config/helm/fabric-api-{{version}}.tgz

_helm-fabric: _kustomize _helm _helmify _kube_gen
  @rm config/helm/fabric-v*.tgz || true
  @rm config/helm/fabric/templates/*.yaml config/helm/fabric/values.yaml || true
  {{kustomize}} build config/default | {{helmify}} config/helm/fabric
  {{helm}} package config/helm/fabric --destination config/helm --version {{version}}
  {{helm}} lint config/helm/fabric-{{version}}.tgz

# Build all K8s artifacts (images and charts)
kube-build: build (_docker-build "fabric") _helm-fabric-api _helm-fabric (_kube-build "fabric-dhcpd") (_kube-build "fabric-boot") (_helm-build "fabric-proxy") && version
  # Docker images and Helm charts built

# Push all K8s artifacts (images and charts)
kube-push: kube-build (_helm-push "fabric-api") (_kube-push "fabric") (_kube-push "fabric-dhcpd") (_kube-push "fabric-boot") (_helm-push "fabric-proxy") && version
  # Docker images and Helm charts pushed

# Push all K8s artifacts (images and charts) and binaries
push: kube-push && version
  cd bin && oras push {{oci_repo}}/{{oci_prefix}}/agent:{{version}} agent
  cd bin && oras push {{oci_repo}}/{{oci_prefix}}/hhfctl:{{version}} hhfctl

# Install API on a kind cluster and wait for CRDs to be ready
test-api: _helm-fabric-api
    kind export kubeconfig --name kind || kind create cluster --name kind
    kind export kubeconfig --name kind
    {{helm}} install -n default fabric-api config/helm/fabric-api-{{version}}.tgz
    sleep 10
    kubectl wait --for condition=established --timeout=60s crd/connections.wiring.githedgehog.com
    kubectl wait --for condition=established --timeout=60s crd/vpcs.vpc.githedgehog.com
    kubectl wait --for condition=established --timeout=60s crd/agents.agent.githedgehog.com
    kubectl wait --for condition=established --timeout=60s crd/dhcpsubnets.dhcp.githedgehog.com
    kubectl get crd | grep hedgehog
    kind delete cluster --name kind
