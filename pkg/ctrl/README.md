# Admission webhooks

The `*_wh.go` files in this package are the defaulting and validating webhooks for the Fabric and Gateway API objects.

## Validation on create is the object's own `Validate()` only

`ValidateCreate` calls the object's `Validate(ctx, kube, fabricCfg)` and does nothing else.

The wiring is validated by hhfab before it's deployed: every object is defaulted and validated with `Default()` and
`Validate()` against an in-memory client holding the whole wiring. The installer then creates the same objects in the
cluster, going through these webhooks. Whatever passes hhfab validation has to pass the webhooks too, otherwise the
installation fails half way. A check that only lives in a webhook isn't run by hhfab and breaks that.

## Where to put a new check

- In the object's `Validate()` in the API package.
- If it needs objects from a package the API package can't import (e.g. the wiring API can't import the VPC or Gateway
  ones), as an extra validator: a field in `meta.ExtraValidators`, implemented in `api/valid`, called from `Validate()`
  and set by every `FabricConfig.Init` caller, both fabric-ctrl (`cmd/main.go`) and hhfab in fabricator. See
  `valid.Peering` and `valid.Connection`.

`Validate()` may look up other objects, but it must not require objects of a kind the installer creates later, or of
the same kind, as the installer creates them one by one in a fixed order of kinds.

## What's fine on update only

`ValidateUpdate` may check what only makes sense for a change, e.g. fields that are immutable, or existing objects
depending on the updated one that have to still fit it (e.g. VPC subnets in an updated IPv4Namespace). Anything else
belongs in `Validate()`.

## Known exception

Gateway and GatewayPeering creates still build the gateway agent config and run the dataplane validator in the webhook,
until it can run from `Validate()` and so in hhfab as well.
