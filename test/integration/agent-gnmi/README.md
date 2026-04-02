# SONiC Agent–gNMI Integration Tests

Integration test infrastructure for validating the interaction between the Hedgehog Fabric Agent and the gNMI server on a **single SONiC switch**.

## Scope

**In Scope:**
- Agent applies configuration to SONiC via gNMI Set operations
- Agent reads SONiC state via gNMI Get operations
- Configuration idempotency and correctness
- Single SONiC Virtual Switch testing
- Agent behavior: config parsing, gNMI interaction, error handling

**Out of Scope:**
- Multi-switch scenarios (orchestration testing)
- Controller functionality (that's controller's test suite)
- Full fabric control plane (that's system-level testing)
- Chaos/resilience testing
- Performance testing (maybe future)

## Architecture

### Testing Virtual Switch (SONiC VS)

```
Host Machine
├─ Test Harness (Go)
│  ├─ Config generator
│  ├─ gNMI client (validation)
│  └─ SSH client (control)
│
└─ KVM Virtual Machine
   └─ SONiC OS (Broadcom VS build)
      ├─ gNMI server :8080 (bridged)
      ├─ SSH :22 (bridged)
      └─ Agent (injected by test)
```

## Directory Structure

```
agent-gnmi/
├── README.md                 # This file
├── main_test.go             # Test suite entry point
├── agent_test.go            # Agent behavior tests
├── gnmi_test.go             # gNMI validation tests
├── fixtures/
│   ├── agent-configs/       # Agent config templates
│   └── expected-states/     # Expected gNMI responses (optional)
├── testdata/                # Runtime data (gitignored)
│   ├── serial-logs/
│   └── agent-logs/
├── images/                  # SONiC VS images (cached, gitignored)
└── pkg/
    ├── sonicvm/             # VM lifecycle management
    ├── testenv/             # Test environment setup
    └── gnmiutil/            # gNMI assertions
```

## Prerequisites

**System Requirements:**
- Linux with KVM support (Ubuntu 22.04+)
- 8GB RAM (4GB for VM)
- 20GB disk space

**Software:**
```bash
sudo apt-get install -y qemu-kvm libvirt-daemon-system socat
sudo usermod -aG kvm $USER  # logout/login required
```

## Quick Start

### Using Just Targets (Recommended)

```bash
cd fabric/

# Build agent utilities
just build-agent-utils

# Start SONiC VS manually
just sonic-vs-up

# In another terminal - check status
just sonic-vs-status

# Install agent on running VM
just sonic-vs-agent-install

# Check agent status
just sonic-vs-agent-status

# Connect to VM
just sonic-vs-ssh

# Stop VM
just sonic-vs-down

# Run full test suite
just test-agent-gnmi-vs
```

### Automated Testing (Full Test Suite)

```bash
cd fabric/test/integration/agent-gnmi

# Run tests with automatic agent build and lifecycle management
go test -v -timeout 15m

# Run specific test
go test -v -run TestVMBoots

# Keep VM running on failure for debugging
go test -v -keep-on-failure=true

# Skip agent build (use existing binary)
go test -v -build-agent=false
```

### Manual Testing (Step-by-Step)

This is useful for development and debugging the agent lifecycle.

**Using just targets (recommended):**

```bash
# Start VM
just sonic-vs-up  # Blocks terminal, open another terminal for next steps

# In another terminal:
just sonic-vs-status            # Check if VM is ready
just sonic-vs-agent-install     # Install agent
just sonic-vs-agent-status      # Check agent status
just sonic-vs-ssh               # Connect to VM
just sonic-vs-down              # Stop VM
```

**Manual steps (alternative):**

#### 1. Start SONiC VS Manually

```bash
cd fabric/test/integration/agent-gnmi/images

# Start VM with serial console access
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
```

**Note:** This blocks the terminal. Open another terminal for the next steps.

#### 2. Build Agent Binary

```bash
cd fabric/
# Use the justfile target for proper build (CGO_ENABLED=0, GOOS=linux, ldflags, etc.)
just build-agent
```

#### 3. Build Utilities

```bash
just build-agent-utils
```

#### 4. Check Agent Status

```bash
cd fabric/test/integration/agent-gnmi

# Check if agent is already installed
./bin/agent-ctl status

# Expected output if clean: "Agent status: not installed"
```

#### 5. Install Agent on Running VM

```bash
# Install agent (uses VLAB SSH key by default)
./bin/agent-ctl -v install

# Verify installation
./bin/agent-ctl status
# Expected: "Agent status: installed, running"
```

#### 6. Test gNMI Interaction

```bash
# Test gNMI capabilities from host
gnmic -a localhost:8080 \
  -u admin \
  --skip-verify \
  capabilities

# Get interface config
gnmic -a localhost:8080 \
  -u admin \
  --skip-verify \
  get \
  --path /openconfig-interfaces:interfaces
```

#### 7. Uninstall Agent

```bash
# Clean up after testing
./bin/agent-ctl -v uninstall

# Verify uninstallation
./bin/agent-ctl status
# Expected: "Agent status: not installed"
```

#### 8. Stop VM

```bash
# Using just
just sonic-vs-down

# Or manually: In the VM terminal, press Ctrl+A then X (QEMU monitor)
# Or send poweroff via SSH
ssh -i ./sshkey admin@localhost 'sudo poweroff'
```

### Agent Development Workflow

For faster agent development iteration, you can run the agent **on your host** instead of installing it in the VM. Both SSH and gNMI are port-forwarded from the VM, making them accessible from the host:

- **SSH**: `localhost:2222` → VM port 22
- **gNMI**: `localhost:8080` → VM port 8080

**Benefits:**
- **Faster iteration**: No rebuild/reinstall cycle
- **Full debugging**: Use debugger, breakpoints, hot reload
- **Quick config testing**: Test different agent configs instantly
- **Native development**: Run agent with your IDE/tools

**Workflow:**

```bash
# 1. Start SONiC VS (no agent installation needed)
just sonic-vs-up

# 2. In another terminal, configure agent to connect to localhost:8080
#    Modify your agent config to point to:
#    - gNMI endpoint: localhost:8080
#    - Credentials: admin (with appropriate password)

# 3. Run agent directly on host
cd fabric/
go run ./cmd/agent start --config=/path/to/test-config.yaml

# 4. Agent connects to VM's gNMI server and configures SONiC
#    You can modify code, restart agent, debug, etc.

# 5. Verify configuration via gNMI
gnmic -a localhost:8080 -u admin --skip-verify get --path /openconfig-interfaces:interfaces

# 6. Stop VM when done
just sonic-vs-down
```

**Note:** This workflow is ideal for agent development but not for testing the full agent deployment (systemd service, startup behavior, etc.). For that, use the standard installation workflow above.

## Automated Test Lifecycle

When you run `go test -v`, the test harness automatically:

### Phase 1: Setup (TestMain)

1. **Parse flags** - Target type, paths, options
2. **Build agent binary** (if `-build-agent=true`, default)
   - Runs `just build-agent` for proper static binary
   - Built with `CGO_ENABLED=0` for glibc compatibility
   - Ensures fresh binary for each test run
3. **Start SONiC VS** (if target=vs)
   - Creates QEMU VM with 4 CPUs, 4GB RAM
   - Boots SONiC image from `./images/sonic-vs.qcow2`
   - Forwards SSH (2222→22) and gNMI (8080→8080)
   - Waits up to 5 minutes for boot
4. **Verify SONiC ready**
   - SSH connection succeeds
   - Can execute commands
5. **Prepare test environment**
   - Fixes `eth0-dhcp` service for virtualized testing
   - Service reconfigured to report as "active" after DHCP success
   - Installs Grafana Alloy from GitHub (agent config references alloy for observability)
6. **Clean slate**
   - Uninstalls any existing agent from the image
   - Cleans old agent logs
7. **Install agent**
   - Copies agent binary via SFTP
   - Creates systemd service file
   - Starts and enables hedgehog-agent service
   - Verifies agent is running

### Phase 2: Tests Execution

```
For each test:
  - Tests have access to:
    - testVM (sonicvm.VM): Control the VM
    - testAgentMgr (agent.Manager): Control agent lifecycle
  - Tests can:
    - Generate agent configs
    - Interact via SSH
    - Query via gNMI
    - Restart agent with new config
    - Check logs
```

### Phase 3: Cleanup

1. **Uninstall agent** - Removes agent, service file, configs
2. **Stop VM** - Graceful shutdown (unless `-keep-on-failure`)
3. **Exit** - Returns test results

### What Gets Persisted

The image (`sonic-vs.qcow2`) is **modified during tests**:
- Agent uninstalled before cleanup = clean for next run
- If test crashes, image may have agent installed

To clean manually:
```bash
./bin/agent-ctl uninstall  # While VM is running
```

## Writing Tests

### Example Test

```go
func TestAgentAppliesVLANConfig(t *testing.T) {
    ctx := context.Background()
    target := testEnv.Target(t)

    // 1. Generate config
    agentConfig := testenv.GenerateAgentConfig(t, testenv.AgentConfigParams{
        Fixture: "fixtures/agent-configs/vlan-config.yaml",
        VLANs:   []int{100, 200},
    })

    // 2. Inject and start agent
    if err := target.InjectAgent(ctx, agentBinary, agentConfig); err != nil {
        t.Fatalf("Inject failed: %v", err)
    }
    defer target.StopAgent(ctx)

    // 3. Wait for agent to apply
    testEnv.WaitForAgentApplied(ctx, target, 2*time.Minute)

    // 4. Validate via gNMI
    gnmi := testEnv.GNMI(t, target)
    gnmi.AssertVLANExists(ctx, 100)
    gnmi.AssertVLANExists(ctx, 200)
}
```
