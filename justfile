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

# Generate code/manifests, things to embed, etc
gen: _kube_gen _embed

# Build all artifacts
build: _license_headers _kube_gen _gotools _embed && version
  # Build fabric-boot
  {{go_linux_build}} -o ./bin/fabric-boot ./cmd/fabric-boot
  @echo "Build complete"
