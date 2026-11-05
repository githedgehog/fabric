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
  `valid.Peering` and `valid.Connection`, and `api/valid/README.md` for how they behave.

`Validate()` may look up other objects, but it must not require objects of a kind the installer creates later, or of
the same kind, as the installer creates them one by one in a fixed order of kinds.

## What's fine on update only

`ValidateUpdate` may check what only makes sense for a change, e.g. fields that are immutable, or existing objects
depending on the updated one that have to still fit it (e.g. VPC subnets in an updated IPv4Namespace). Anything else
belongs in `Validate()`.

## Known exception

Gateway and GatewayPeering creates still build the gateway agent config and run the dataplane validator in the webhook,
until it can run from `Validate()` and so in hhfab as well.

# Initialization and the lock

The fabric controller stays locked until the running version is initialized. Its state is the `FabricController`
singleton `default` in the fabric controller's namespace (`kubectl get fc -A`).

## Initialization

The initializer (`fabriccontroller_init.go`) is a leader-elected reconciler of the `FabricController`. For a new version
or spec generation it enforces the built-in switch profiles, runs the one-time migrations if there are any, refreshes
the stored objects (`fabriccontroller_refresh.go`) and records `Ready`. A restart of an initialized version only
enforces the profiles.

The refresh writes the current `Default()` to every object that doesn't carry it, in passes over the cache until one
finds nothing to change, and validates each object on its own. Objects it can't fix (rejected, stale or invalid) fail
the initialization: it stays locked without retrying until a spec change or a restart, see `status.problems`. Other
errors are retried with backoff, see `status.error`.

- `Default()` only depends on the object itself and is idempotent.
- A new kind with a defaulting webhook goes into `refreshKinds`.
- A new one-time migration goes into the initializer before the refresh and is idempotent.

## Lock

Each replica has its own `Lock`, set by the `FabricController` controller (`fabriccontroller_ctrl.go`).

| | Locked | Maintenance | Unlocked |
|---|---|---|---|
| Reconcilers | wait | wait | run |
| Fabric controller writes | not validated | not validated | validated |
| Other writes | 503 | validation errors as warnings | validated |

Maintenance (`spec.maintenance`) is turned on by users to fix the objects that keep it locked, leaving it initializes
again. Objects are only validated on their own then, and it relies on a single replica: a reconcile already running when
it's turned on may still read an edit admitted right after.

## Reconcilers and mappers

- Reconcilers return `RequeueAfter: lockedRequeueAfter` while locked, so their events stay queued.
- Fan-out mappers return nothing while locked, as the refresh writes every object.
- A controller with such a mapper watches `lock.Unlocked()` and enqueues all its objects, as the ones reconciled before
  a later lock (maintenance, re-initialization) aren't queued anymore: agent, gateway and switch boot today.
