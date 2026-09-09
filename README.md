# Hedgehog Fabric

The core of Hedgehog Open Network Fabric: the Kubernetes API (CRDs) that models a
fabric, the controller that turns that model into per-switch configuration, and the
agent that runs on each switch and applies it.

This repo does **not** install anything by itself. A fabric is deployed by
[fabricator](https://github.com/githedgehog/fabricator) (`hhfab`), which builds the
control node, installs these CRDs and the controller, and runs the VLAB.

User-facing docs live at [docs.githedgehog.com](https://docs.githedgehog.com).
`docs/api.md` (generated) is the API reference; `pkg/agent/dozer/bcm/README.plan.md`
documents what config we actually push to a Broadcom SONiC switch and why — read that
before changing the planner.

## Building

Prerequisites: Go 1.26+ (see `go.mod`), [just](https://github.com/casey/just) 1.36+, Docker.

```sh
just build      # generate, then build all binaries into bin/ (linux/amd64)
just test       # run tests
just lint       # golangci-lint + license headers + actionlint
just --list     # everything else
```

Always go through `just` — raw `go build`/`go test` miss the build tags, ldflags and
codegen steps and will produce misleading results.

`just gen` (a dependency of both `build` and `test`) ends with `test-docs`, which
builds the API reference in a container, so **Docker is required to run the tests**.

## Code structure

### `api/` — the CRDs

These packages are imported by fabricator, gateway and other Hedgehog repos, so treat
them as public API: a change here ripples outward. Run `just gen` after editing any type
— it regenerates deepcopy funcs, CRDs, RBAC and `docs/api.md`.

| Group | Contents |
| --- | --- |
| `wiring/v1beta1` | Physical layer: `Switch`, `Server`, `Connection`, `SwitchGroup`, `SwitchProfile`, `ServerProfile`, `VLANNamespace` |
| `vpc/v1beta1` | Overlay: `VPC`, `VPCAttachment`, `VPCPeering`, `External`, `ExternalAttachment`, `ExternalPeering`, `IPv4Namespace` |
| `agent/v1beta1` | `Agent` (the per-switch desired state, written by the controller, read by the agent) and `Catalog` (allocated IDs: VNIs, ASNs, per-switch identifiers) |
| `gateway/v1alpha1` | Gateway-side API: `Gateway`, `GatewayGroup`, `GatewayPeering`, `VPCInfo` |
| `gwint/v1alpha1` | `GatewayAgent` — the gateway's equivalent of `Agent` |
| `dhcp/v1beta1` | `DHCPSubnet`, consumed by `fabric-dhcpd` |
| `meta` | Shared types and constants, including `FabricConfig` (the knobs the controller and agent both read) |
| `valid` | Peering validation, injected into `FabricConfig` so webhooks and `hhfctl` share one implementation |

### `cmd/` — the binaries

| Binary | Role |
| --- | --- |
| `fabric` (`cmd/`) | The controller: reconcilers + admission webhooks, runs on the control node |
| `agent` | Runs on every switch as a systemd unit (as root), applies its own `Agent` CR |
| `hhfctl` | Operator CLI for creating/inspecting VPCs, connections, externals, switches |
| `fabric-boot` | ONIE/ZTP boot server for switch provisioning |
| `fabric-dhcpd` | DHCP server for VPC subnets, driven by `DHCPSubnet` |
| `fabric-nos-install` | NOS installer, embedded into `fabric-boot` |
| `fabric-gen` | Codegen/doc tool (switch profile reference) |

### `pkg/`

| Path | Role |
| --- | --- |
| `ctrl/` | Reconcilers (`*_ctrl.go`) and admission webhooks (`*_wh.go`). `agent_ctrl.go` is the important one: it collects everything relevant to a switch and writes its `Agent` CR |
| `manager/librarian/` | The "librarian": allocates and persists fabric-wide identifiers (VNIs, ASNs, port channel IDs, external leak IDs) into the `Catalog` |
| `agent/` | The switch agent: watch loop, enforce loop, self-upgrade, metrics (`alloy/`), switch state collection (`switchstate/`) |
| `agent/dozer/` | The NOS abstraction. `dozer.go` defines `Spec` (the whole switch config as data) and the `Processor` interface |
| `agent/dozer/bcm/` | Broadcom SONiC implementation: `plan.go` builds the desired `Spec` from an `Agent` CR, `spec_*.go` files map each piece of `Spec` to OpenConfig/gNMI and back, `state.go` reads switch state |
| `agent/clsp/`, `agent/cmls/` | Celestica SONiC+ and Cumulus Linux processors (work in progress) |
| `boot/` | ZTP/ONIE boot server and NOS install logic |
| `hhfctl/` | `hhfctl` command implementations, incl. `inspect/` |
| `client/apiabbr/` | Compact API representation used by `hhfab` and `hhfctl` for bulk create/diff |
| `util/` | Small helpers; `apiutil` holds the shared queries over API objects that both the controller and `hhfctl` use |

`config/` holds kustomize bases and Helm charts, `hack/` the tool versions and extra
just recipes, `docs/` the generated reference docs.

## How configuration flows

The controller follows the Kubernetes
[operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/): the
CRDs in `api/` are the desired state, and the
[controllers](https://kubernetes.io/docs/concepts/architecture/controller/) in `pkg/ctrl`
reconcile until the fabric matches it. The layout (`api/`, `cmd/`, `config/`, `PROJECT`)
is kubebuilder-scaffolded — see the
[kubebuilder book](https://book.kubebuilder.io/introduction.html) for the conventions.
The switch agent then repeats the same pattern one level down, reconciling its `Agent` CR
against the switch itself:

1. A user (or `hhfab`) creates wiring and VPC objects. Webhooks in `pkg/ctrl/*_wh.go`
   validate them at admission time — that's where most invariants are enforced.
2. `AgentReconciler` gathers everything that concerns one switch, asks the librarian for
   any identifiers it needs, and writes a single `Agent` CR per switch.
3. The agent on the switch watches its own `Agent` CR. On every change, and otherwise
   every 2 minutes (`EnforcePeriod`), it runs:
   `PlanDesiredState` → `LoadActualState` → `CalculateActions` (diff of two `Spec`s) →
   `ApplyActions` (gNMI writes).
4. It writes `last-desired.yaml` / `last-actual.yaml` into its basedir and reports state
   back into `Agent.Status`.

The consequence of step 3 being a diff of round-tripped data: if a field doesn't survive
the read-back from the switch, the agent rewrites that object on *every* pass, every two
minutes.

## Testing and iterating

`just test` runs everything. The most valuable tests are the golden planner tests in
`pkg/agent/dozer/bcm/` (`plan_test.go`): each case is an `Agent` CR in `testdata/`, and
the test asserts the planned `Spec`, the computed actions, and the resulting gNMI
payloads against checked-in expectations.

To add a case: drop `testdata/<name>.in.agent.yaml` (easiest source is a real
`kubectl get agent <switch> -o yaml` from a VLAB), add `<name>` to the table in
`plan_test.go` with a comment saying what topology it covers, then:

```sh
just test-update                                   # regenerate all goldens
just test-update ./pkg/agent/dozer/bcm/...         # or just this package
```

`test-update` overwrites `*.expected.yaml`; **review that diff** — it is the only review
the planner's output gets. The `*.actual.yaml` files are written on every run and are
gitignored, so on a failure you can diff them against the expected ones directly.

`just test-api` installs the API chart onto a kind cluster and waits for the CRDs
(`just test-api-auto` creates and deletes the cluster around it).

### Iterating on the agent, without deploying

Run the *locally built* agent against a live switch, taking desired state from the
cluster, e.g.:

```sh
KUBECONFIG=<vlab-workdir>/vlab/kubeconfig \
  just run agent remote -n leaf-01 -k -a <control-node-host> -u admin
```

`-k` is dry-run (drop it to actually enforce), `-a` is the ssh jump host used to reach the
switch (e.g. an alias from your `~/.ssh/config` — the VLAB's control node).
It writes `last-desired.yaml` / `last-actual.yaml` in the repo root, so an empty
`diff last-actual.yaml last-desired.yaml` is a fast, exact check that a spec round-trips.
This takes seconds; prefer it to push+patch while iterating. Note that individual actions
are only logged during apply, so a dry run just reports a count.

### Deploying a change to a running VLAB

```sh
just oci_repo=<REGISTRY> version_extra=-<YOUR_INITIALS> push patch
```

`oci_repo` is the registry you are targeting - ask us for the one we typically target
for development. `version_extra` is an optional suffix to tell your builds apart.

`push` and `patch` must be in a **single** `just` invocation: the version string gets a
random suffix whenever the tree is dirty, so two separate invocations compute different
versions and the patch ends up pointing at an image that was never pushed.

## Gotchas

Things that cost us time once already, so they are written down rather than rediscovered.

### gNMI

- **Path form is `/<module>/<table>/<list>`**, never `/<module>/<module>/<table>` — the
  ygot Go type names repeat the module and tempt you into the doubled form.
- **The destination struct must be the *parent* of the node the path names.** Pointing at
  `.../IF_REASON_EVENT` with dest `*..._IF_REASON_EVENT` unmarshals zero rows and returns
  no error. `/sonic-vrf/VRF/VRF_LIST` with dest `*SonicVrf_SonicVrf_VRF` is the correct
  in-repo template.
- Both failures above are **silent**, so verify a new table read by counting rows, not by
  checking the error.
- `SetRequest` values must be rooted under the path's last segment: path `.../config`
  takes `{"config": {"enabled": false}}`; a bare `{"enabled": false}` is rejected.
- `ON_CHANGE` is a per-leaf allowlist. `counters/*` and the whole `state` subtree return
  nothing at all — no error, no initial value, no sync-response. Treat a missing
  sync-response as a failure, not as "no events yet".

### sonic-cli

On Broadcom SONiC, `sonic-cli` is the only supported way to query or configure the box.
Don't use `vtysh`, and don't use the bash `show` utilities — they can report state that
has drifted from what Broadcom considers the source of truth.

- It **refuses to run as root** (`FATAL: root cannot launch CLI`). The agent runs as root,
  so anything shelling out to it needs `sudo -u admin sonic-cli -c "..."`.
- Under a pty it **paginates and hangs** at `--more--`. Append `| no-more` *inside* the
  sonic-cli command. (That's sonic-cli syntax — if bash says `no-more: command not found`,
  your command leaked out of sonic-cli.)
- Syntax differs from FRR: `show bgp ipv4 unicast summary`, not `show ip bgp summary`.
  Interface config takes the NOS name (`interface Ethernet6`), not the alias (`Eth1/7`).
- Route-maps hand-created through repeated `-c` args never persist (exit 0, nothing
  created) — submode needs a real pty. If you do hand-create something to probe, delete it
  afterwards: config the agent cannot parse can wedge it in a crash loop inside
  `LoadActualState`, before it gets far enough to upgrade itself out of the problem.

**To find which gNMI path an operation uses**, run `show logging -af | grep -i translib`
from the switch's bash console while running the equivalent `sonic-cli` command in another
window. Stop the agent first (`sudo systemctl stop hedgehog-agent`) for a clean trace.

## License

Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
