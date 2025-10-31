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
all: gen lint lint-gha test build kube-build && version

# Run linters against code (incl. license headers)
lint: _lint _golangci_lint
  {{golangci_lint}} run --show-stats ./...

# Run golangci-lint to attempt to fix issues
lint-fix: _lint _golangci_lint
  {{golangci_lint}} run --show-stats --fix ./...

go_base_flags := "--tags containers_image_openpgp,containers_image_storage_stub"
go_flags := go_base_flags + " -ldflags=\"-w -s -X go.githedgehog.com/fabric/pkg/version.Version=" + version + "\""
go_build := "go build " + go_flags
go_linux_build := "GOOS=linux GOARCH=amd64 " + go_build

_touch_embed:
  @touch pkg/boot/nosinstall/bin/fabric-nos-install

_embed: _touch_embed
  # Build fabric-nos-install binary for embedding
  {{go_linux_build}} -o ./pkg/boot/nosinstall/bin/fabric-nos-install ./cmd/fabric-nos-install

_kube_gen:
  # Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject implementations
  {{controller_gen}} object:headerFile="hack/boilerplate.go.txt" paths="./..."
  # Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
  {{controller_gen}} rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate docs, code/manifests, things to embed, etc
gen: _kube_gen _embed _crd_ref_docs
  {{crd_ref_docs}} --source-path=./api/ --config=api/docs.config.yaml --renderer=markdown --output-path=./docs/api.md
  go run cmd/fabric-gen/main.go profiles-ref

# Build all artifacts
build: _license_headers _gotools gen _embed && version
  {{go_linux_build}} -o ./bin/fabric ./cmd
  {{go_linux_build}} -o ./bin/agent ./cmd/agent
  {{go_linux_build}} -o ./bin/hhfctl ./cmd/hhfctl
  {{go_linux_build}} -o ./bin/fabric-boot ./cmd/fabric-boot
  {{go_linux_build}} -o ./bin/fabric-dhcpd ./cmd/fabric-dhcpd
  # Build complete

_hhfctl-build GOOS GOARCH: _license_headers _kube_gen _gotools _embed
  GOOS={{GOOS}} GOARCH={{GOARCH}} {{go_build}} -o ./bin/hhfctl-{{GOOS}}-{{GOARCH}}/hhfctl ./cmd/hhfctl
  cd bin && tar -czvf hhfctl-{{GOOS}}-{{GOARCH}}-{{version}}.tar.gz hhfctl-{{GOOS}}-{{GOARCH}}/hhfctl

# Build hhfctl and other user-facing binaries for all supported OS/Arch
build-multi: (_hhfctl-build "linux" "amd64") (_hhfctl-build "linux" "arm64") (_hhfctl-build "darwin" "amd64") (_hhfctl-build "darwin" "arm64") && version

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
kube-build: build (_docker-build "fabric") _helm-fabric-api _helm-fabric (_kube-build "fabric-dhcpd") (_kube-build "fabric-boot") && version
  # Docker images and Helm charts built

# Push all K8s artifacts (images and charts)
kube-push: kube-build (_helm-push "fabric-api") (_kube-push "fabric") (_kube-push "fabric-dhcpd") (_kube-push "fabric-boot") && version
  # Docker images and Helm charts pushed

# Push all K8s artifacts (images and charts) and binaries
push: kube-push && version
  cd bin && oras push {{oras_insecure}} {{oci_repo}}/{{oci_prefix}}/agent:{{version}} agent
  cd bin && oras push {{oras_insecure}} {{oci_repo}}/{{oci_prefix}}/hhfctl:{{version}} hhfctl

_hhfctl-push GOOS GOARCH: _oras (_hhfctl-build GOOS GOARCH)
  cd bin/hhfctl-{{GOOS}}-{{GOARCH}} && oras push {{oras_insecure}} {{oci_repo}}/{{oci_prefix}}/hhfctl-{{GOOS}}-{{GOARCH}}:{{version}} hhfctl

# Publish hhfctl and other user-facing binaries for all supported OS/Arch
push-multi: (_hhfctl-push "linux" "amd64") (_hhfctl-push "linux" "arm64") (_hhfctl-push "darwin" "amd64") (_hhfctl-push "darwin" "arm64") && version

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

# Patch deployment using the default kubeconfig (KUBECONFIG env or ~/.kube/config)
patch: && version
  kubectl -n fab patch fab/default --type=merge -p '{"spec":{"overrides":{"versions":{"fabric":{"api":"{{version}}","agent":"{{version}}","boot":"{{version}}","controller":"{{version}}","ctl":"{{version}}","dhcpd":"{{version}}"}}}}}'

# Run specified command with args with minimal Go flags (no version provided)
run cmd *args:
  @echo "Running: {{cmd}} {{args}} (run gen manually if needed)"
  @go run {{go_base_flags}} ./cmd/{{cmd}} {{args}}

# =============================================================================
# Integration Tests
# =============================================================================

agent_gnmi_test_dir := "test/integration/agent-gnmi"
agent_gnmi_images_dir := agent_gnmi_test_dir + "/images"

# Build agent binary for integration tests
build-agent:
  @echo "Building agent binary for integration tests..."
  {{go_linux_build}} -o ./bin/agent ./cmd/agent
  @echo "Agent binary built: ./bin/agent"

# Prepare SONiC VS image
sonic-vs-prep:
  @echo "Preparing SONiC VS images..."
  @mkdir -p {{agent_gnmi_images_dir}}
  @echo ""
  @echo "TODO: Download SONiC VS image"
  @echo "For now, you need to manually place the following files in {{agent_gnmi_images_dir}}:"
  @echo "  - sonic-vs.qcow2    (SONiC VS disk image)"
  @echo "  - efi_code.fd       (EFI code firmware)"
  @echo "  - efi_vars.fd       (EFI vars firmware)"

# Internal helper to run agent-gnmi integration tests with custom parameters
_test-agent-gnmi extra_args="": build-agent
  cd {{agent_gnmi_test_dir}} && \
    go test -v -timeout 30m \
      -target=vs \
      -agent-binary=../../../bin/agent \
      -cache-dir=./images \
      {{extra_args}} \
      ./...

# Run agent-gnmi integration tests (Virtual Switch)
test-agent-gnmi-vs: (_test-agent-gnmi "")

# Run agent-gnmi integration tests with verbose output
test-agent-gnmi-verbose: (_test-agent-gnmi "-test.v")

# Run agent-gnmi integration tests and keep VM on failure for debugging
test-agent-gnmi-debug: (_test-agent-gnmi "-keep-on-failure=true")

# Build agent control utility
build-agent-utils:
  @echo "Building agent control utility..."
  cd {{agent_gnmi_test_dir}} && \
    go build -o bin/agent-ctl ./cmd/agent-ctl
  @echo "Utility built: {{agent_gnmi_test_dir}}/bin/agent-ctl"

# Start SONiC VS manually for development/debugging
sonic-vs-up:
  @echo "Starting SONiC VS..."
  @if [ ! -f {{agent_gnmi_images_dir}}/sonic-vs.qcow2 ]; then \
    echo "ERROR: SONiC VS image not found at {{agent_gnmi_images_dir}}/sonic-vs.qcow2"; \
    echo "Run 'just sonic-vs-prep' for instructions"; \
    exit 1; \
  fi
  @echo ""
  @echo "VM will start with:"
  @echo "  - SSH:  localhost:2222"
  @echo "  - gNMI: localhost:8080"
  @echo "  - Serial console: ./test/integration/agent-gnmi/images/serial.log"
  @echo ""
  @echo "To stop: just sonic-vs-down"
  @echo "To connect: ssh -i {{agent_gnmi_test_dir}}/sshkey admin@localhost -p 2222"
  @echo ""
  @cd {{agent_gnmi_images_dir}} && \
    qemu-system-x86_64 \
      -name sonic-vs-debug \
      -m 4096M \
      -machine q35,accel=kvm,smm=on \
      -cpu host \
      -smp 4 \
      -drive if=none,file=sonic-vs.qcow2,id=disk1 \
      -device virtio-blk-pci,drive=disk1,bootindex=1 \
      -drive if=pflash,file=efi_code.fd,format=raw,readonly=on \
      -drive if=pflash,file=efi_vars.fd,format=raw \
      -netdev user,id=mgmt,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:8080 \
      -device e1000,netdev=mgmt \
      -nographic \
      -serial mon:stdio

# Stop SONiC VS (graceful shutdown)
sonic-vs-down:
  @echo "Stopping SONiC VS..."
  @if ! pgrep -f "qemu-system-x86_64.*sonic-vs" > /dev/null; then \
    echo "SONiC VS is not running"; \
    exit 0; \
  fi
  @ssh -i {{agent_gnmi_test_dir}}/sshkey \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=5 \
    -p 2222 admin@localhost 'sudo poweroff' 2>/dev/null || \
    (echo "Could not connect via SSH, sending SIGTERM to QEMU..." && \
     pkill -TERM -f "qemu-system-x86_64.*sonic-vs")
  @echo "SONiC VS shutdown initiated"

# Check if SONiC VS is running
sonic-vs-status:
  @echo "SONiC VS status:"
  @if pgrep -f "qemu-system-x86_64.*sonic-vs" > /dev/null; then \
    echo "  VM: Running (PID: $$(pgrep -f 'qemu-system-x86_64.*sonic-vs'))"; \
    if nc -z localhost 2222 2>/dev/null; then \
      echo "  SSH: Accessible on localhost:2222"; \
    else \
      echo "  SSH: Not accessible (VM may still be booting)"; \
    fi; \
    if nc -z localhost 8080 2>/dev/null; then \
      echo "  gNMI: Accessible on localhost:8080"; \
    else \
      echo "  gNMI: Not accessible"; \
    fi; \
  else \
    echo "  VM: Not running"; \
  fi

# Connect to SONiC VS via SSH
sonic-vs-ssh:
  @ssh -i {{agent_gnmi_test_dir}}/sshkey \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    admin@localhost -p 2222

# Install agent on running SONiC VS
sonic-vs-agent-install: build-agent build-agent-utils
  @cd {{agent_gnmi_test_dir}} && \
    ./bin/agent-ctl -v install

# Uninstall agent from running SONiC VS
sonic-vs-agent-uninstall: build-agent-utils
  @cd {{agent_gnmi_test_dir}} && \
    ./bin/agent-ctl -v uninstall

# Check agent status on running SONiC VS
sonic-vs-agent-status: build-agent-utils
  @cd {{agent_gnmi_test_dir}} && \
    ./bin/agent-ctl status
