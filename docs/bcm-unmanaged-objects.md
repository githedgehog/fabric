# Unmanaged objects (alpha, Broadcom SONiC only)

By default, the Fabric agent deletes any switch configuration it didn't create. Unmanaged objects mode lets you keep
selected objects that you configure by hand on a switch running Broadcom SONiC. The agent ignores any object of a
supported kind whose name starts with `nohh-`: it never changes or deletes it.

This is an alpha feature. The annotation name and behavior may change in future releases.

## Supported objects

Only these kinds, and only when the name starts with `nohh-`:

- ACLs
- route maps
- prefix lists
- community lists
- AS-path lists
- BFD profiles
- link state tracking (LST) groups

Everything else is still fully managed by the agent: interfaces, VRFs, BGP neighbors, static routes, users and so on.

## Enabling

Annotate the **Switch** object. Setting the annotation on the Agent object doesn't work, because the controller
overwrites the Agent's annotations from the Switch.

```bash
kubectl annotate switch leaf-01 fabric.alpha.githedgehog.com/bcm-unmanaged-objects=true
```

The value must be a boolean. The annotation is rejected on switches that don't run Broadcom SONiC.

The agent picks up the change on its next enforcement run, by default within 2 minutes. **Wait until the agent
reports the mode before you create any `nohh-` objects.** Before that, the agent still deletes them.

```bash
kubectl get agent leaf-01
# NAME      ...   NOTES
# leaf-01   ...   unmanaged-objects (0)

kubectl fabric inspect switch --name leaf-01
```

The number in parentheses is how many `nohh-` objects the agent found on the switch during its last run. To see
which ones:

```bash
kubectl get agent leaf-01 -o jsonpath='{.status.unmanagedObjects}'
# {"enabled":true,"objects":["acl/nohh-mgmt","routeMap/nohh-peer-in"]}
```

## Rules for manual objects

- **Name every manual object with the `nohh-` prefix.** An object without the prefix is deleted on the next run,
  even if a `nohh-` object uses it.
- **Manual objects may reference only other `nohh-` objects.** For example, a `nohh-` route map may match a `nohh-`
  prefix list, but not a prefix list created by Fabric. If Fabric later removes an object that a manual one still
  uses, the switch can refuse the deletion, and the agent keeps failing to apply its configuration.
- **Don't attach manual objects to objects managed by Fabric.** Attaching a `nohh-` route map to a fabric BGP
  neighbor, or binding a `nohh-` ACL to a port that Fabric manages, is reverted on the next run.
- **Don't add entries to objects managed by Fabric.** Extra rules in a fabric ACL or extra statements in a fabric
  route map are removed, whatever they're named.

## Restarts before the next configuration change

The agent saves the annotation locally only when it applies a new configuration generation. Adding the annotation
doesn't create a new generation.

If the agent restarts, gets upgraded, or the switch reboots before the next configuration change, the agent starts
without the mode and deletes all `nohh-` objects once. To avoid this, make a configuration change that affects the
switch after enabling the mode, and wait for the agent to apply it (`AppliedG` equals `CurrentG` in
`kubectl get agent`).

## Disabling

```bash
kubectl annotate switch leaf-01 fabric.alpha.githedgehog.com/bcm-unmanaged-objects-
```

On its next run, the agent treats `nohh-` objects as unknown configuration and **deletes them**. The
`unmanaged-objects` note disappears from the agent status once that run completes.

## Not covered

- **Other switch operating systems:** Celestica SONiC+ and Cumulus are not supported.
- **Other object kinds:** users, VRFs, interfaces and similar objects have names fixed by the switch or set by the
  Fabric configuration, so the `nohh-` prefix can't be used to exclude them.
