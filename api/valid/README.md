# Extra validators

This package has the checks an API object's `Validate()` needs but can't implement itself, as they look up objects from
packages its API package can't import (e.g. the wiring API can't import the VPC or Gateway ones). See
`pkg/ctrl/README.md` for why every check has to be reachable from `Validate()`.

Each one is a field in `meta.ExtraValidators`, implemented here and called from `Validate()`:

- `Peering` for VPCPeering, ExternalPeering and GatewayPeering
- `Connection` for Connection

## Rules

- Every extra validator is required: `FabricConfig.Init` fails if one isn't set. Every `Init` caller sets all of them,
  both fabric-ctrl (`cmd/main.go`) and hhfab in fabricator, so a new one needs a fabricator change before it can be
  required.
- `Validate()` returns an error if the fabric config is set but the extra validator isn't, it's never skipped silently.
  With no client at all, the checks against other objects aren't run, extra validators included.
- With a client but no fabric config, peering validation skips the extra validator, while connection validation returns
  an error, as it needs the fabric config for its own checks too.
- An extra validator returns an error, and never panics, if any of its inputs is missing: a nil client, a nil object
  (incl. a nil pointer of the expected type) or an object of an unexpected type.
