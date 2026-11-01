# BCM mutate rules

!!! note
    Added in the 26.05 release as an alpha feature for switches running Broadcom SONiC. The annotation name, rule
    format and behavior may change in future releases.

!!! warning
    BCM mutate rules make the agent accept configuration that is managed by hand instead of the configuration planned
    by Fabric. Incorrect or outdated rules can break the switch configuration entirely, compromise tenant isolation
    between VPCs, and leave the agent unable to apply any configuration. Use them only when there is no other way to
    achieve the result, review every rule carefully, and verify the switch configuration after each change.

By default, the Fabric agent enforces the whole configuration it manages on a switch and removes anything it didn't
create. BCM mutate rules let you change that for selected parts of the configuration on a switch running Broadcom
SONiC:

- **keep** configuration you created by hand on the switch, where Fabric doesn't plan anything,
- **override** the Fabric configuration with whatever is configured on the switch; while the rule is in place, the
  object or element stays as it is on the switch, even if Fabric configured it in the first place and later plans to
  change it,
- **add** or **remove** members of the community and AS-path lists that Fabric creates.

## Setting rules

Rules are set with an annotation on the **Switch** object. Setting it on the Agent object doesn't work, because the
controller overwrites the Agent's annotations from the Switch.

The value is YAML or JSON with a list of rules:

```yaml
apiVersion: wiring.githedgehog.com/v1beta1
kind: Switch
metadata:
  name: leaf-01
  annotations:
    fabric.alpha.githedgehog.com/bcm-mutate: |
      rules:
        - keep@acl/vpc-filtering--vpc-01--default/65000
        - keep@aclInterface/Ethernet0.10/ingress
        - override@bfdProfile/fabric
```

```bash
kubectl annotate switch leaf-01 --overwrite \
  fabric.alpha.githedgehog.com/bcm-mutate='{"rules": ["keep@staticRoute/default/10.99.0.0/16"]}'
```

Invalid annotations are rejected: unknown fields, malformed rules, conflicting rules, or rules on a switch that doesn't
run Broadcom SONiC.

The agent applies an annotation change right away, the same way it applies any other configuration change, and saves it
locally so the rules also apply after an agent restart or a switch reboot.

To see what the agent did, check the agent status:

```bash
kubectl get agent leaf-01
# NAME      ...   NOTES
# leaf-01   ...   mutated (3)

kubectl get agent leaf-01 -o jsonpath='{.status.mutate}'
# {"enabled":true,"applied":["keep@acl/vpc-filtering--vpc-01--default/65000","keep@aclInterface/Ethernet0.10/ingress","override@bfdProfile/fabric"]}

kubectl fabric inspect switch --name leaf-01
```

`applied` lists the rules that took effect during the last configuration run. Rules that were skipped are logged by
the agent on the switch with the reason, e.g. `journalctl -u hedgehog-agent | grep "Skipped bcm mutate rule"`.

## Rule format

A rule is `<mode>@<kind>/<name>[/<element>]`:

| Kind            | Object or element                                         | Modes                                         |
|-----------------|-----------------------------------------------------------|-----------------------------------------------|
| `acl`           | `acl/<name>` or a single rule `acl/<name>/<seq>`          | `keep`, `override`                            |
| `routeMap`      | `routeMap/<name>` or a statement `routeMap/<name>/<seq>`  | `keep`, `override`                            |
| `prefixList`    | `prefixList/<name>` or an entry `prefixList/<name>/<seq>` | `keep`, `override`                            |
| `aclInterface`  | `aclInterface/<interface>/<ingress or egress>`            | `keep`, `override`                            |
| `staticRoute`   | `staticRoute/<vrf>/<prefix>`                              | `keep`, `override`                            |
| `bfdProfile`    | `bfdProfile/<name>`                                       | `keep`, `override`                            |
| `communityList` | `communityList/<name>`                                    | `keep`, `override`                            |
| `communityList` | a member `communityList/<name>/<ASN:value>`               | `add`, `remove`                               |
| `asPathList`    | `asPathList/<name>`                                       | `keep`, `override`                            |
| `asPathList`    | a member `asPathList/<name>/<regex>`                      | `add`, `remove`                               |

- Sequence numbers are written as plain numbers: `1` to `65535` for ACL rules and route map statements, up to
  `4294967295` for prefix list entries.
- Interfaces use the names on the switch: `Ethernet0` for a port, `Ethernet0.10` for a subinterface with VLAN 10,
  `Vlan1000` for a VLAN interface.
- Static route prefixes must be in canonical form, e.g. `10.0.0.0/8`, not `10.0.0.1/8`.
- The same object or element can only have one rule. A whole ACL, route map or prefix list can't be combined with
  rules for its elements.

## How rules work

On every configuration run, the agent loads the configuration from the switch, plans the Fabric configuration, and
then applies the rules before comparing the two:

- **keep** takes the object or element from the switch only if Fabric doesn't plan anything there. If Fabric plans
  the same object or element, Fabric's version wins and the rule is skipped with a warning.
- **override** takes the object or element from the switch even if Fabric plans a different version of it, including
  when Fabric configured it in the first place. It stays as it is on the switch until the rule is removed.
- For both, if the object or element isn't on the switch, the rule is skipped with a warning and Fabric's plan applies.
  So deleting your manual version on the switch restores Fabric's on the next run.
- **add** and **remove** change the members of a community or AS-path list planned by Fabric, regardless of what's on
  the switch. They are applied after keep and override, and remove after add. Members are sorted and de-duplicated.
  An add of a member that is already in the list, or a remove of a member that isn't, is skipped with a warning.
- Rules for an element (an ACL rule, a statement, an entry, a static route, an ACL binding, a list member) are
  skipped with a warning if Fabric doesn't plan the ACL, route map, prefix list, VRF, interface or list that contains
  it.

Content taken from the switch is identical on both sides of the comparison, so the agent never writes to it. Settings
the agent doesn't know about are kept as well.

Removing a rule makes the agent enforce the Fabric configuration there again on the next run, which deletes or replaces
the manual configuration.

## Examples

Keep an extra rule in a Fabric ACL that is already bound to an interface:

```
keep@acl/vpc-filtering--vpc-01--default/65000
```

Bind your own ACL to the ingress of a subinterface where Fabric only binds an egress ACL:

```
keep@acl/manual-ext-filter
keep@aclInterface/Ethernet0.10/ingress
```

Run your own route map from a Fabric route map, e.g. to filter what an external advertises, with its own prefix list:

```
keep@routeMap/ext-inbound--ext-01/1
keep@routeMap/manual-ext-01-in
keep@prefixList/manual-ext-01-allowed
```

Statement `1` of `ext-inbound--ext-01` is created by hand on the switch and calls `manual-ext-01-in`.

Keep a static route in the default VRF and tune the BFD timers of the Fabric BGP sessions:

```
keep@staticRoute/default/10.99.0.0/16
override@bfdProfile/fabric
```

Add a community to a Fabric community list and an AS to a Fabric AS-path list:

```
add@communityList/all-externals/65000:100
add@asPathList/fabric-gw-aspath/_65099_
```

## Choosing sequence numbers

Elements kept with `keep` are lost if Fabric starts planning the same sequence number later. These ranges are not
used by the current Fabric release and are recommended for manual elements, but they are not reserved:

| Object                               | Free sequence numbers    |
|--------------------------------------|--------------------------|
| ACLs, except `ext-inbound--*`        | `65000`–`65534`          |
| `ext-inbound--*` ACLs                | `8`–`9`                  |
| Route maps, except `l2vpn-neighbors` | `1`–`4`, `65000`–`65499` |

Lower sequence numbers are evaluated first:

- In ACLs, `65000`–`65534` comes after the Fabric rules and before the final `65535` accept, so manual rules there can
  add restrictions but can't allow traffic that Fabric drops. In `ext-inbound--*` ACLs, `8`–`9` comes after the
  switch protection rules (`1`–`6`) and before the rules from the ExternalAttachment ACL (`10` and higher).
- In route maps, `1`–`4` comes before all Fabric statements and `65000`–`65499` after them. A statement that matches
  a route ends the route map for it, so a manual statement at `1`–`4` that permits routes skips the Fabric statements,
  and one at the end only sees routes that no Fabric statement matched.

## Caveats

- Your own ACLs, route maps and lists are only used if something refers to them: an ACL through a kept `aclInterface`
  binding, a route map through a `call` from a statement you keep in a Fabric route map, a list through a `match` in
  such a statement. On their own they just stay on the switch.
- Don't make Fabric objects depend on your objects or the other way around beyond that. If Fabric deletes an ACL that
  is still bound by hand, or a list that is still used by a manual statement, the switch can refuse the deletion and
  the agent keeps failing to apply its configuration.
- If Fabric removes a whole ACL, route map or list (e.g. when an external is deleted), the elements kept in it go with
  it.
- If the description of an existing Fabric ACL changes, the agent replaces the whole ACL, which removes the kept rules
  from the switch. They have to be created by hand again.
- If Fabric changes the egress ACL of an interface with a kept ingress binding, the agent sends the egress binding
  first and the ingress binding separately, so the kept ingress binding is briefly removed.
- `override` freezes the object or element in whatever state it is on the switch, no matter who configured it. This
  includes configuration created by Fabric itself: once an `override` rule is set for it, changes Fabric plans for it
  later, e.g. after changes to VPCs, peerings or externals, are not applied until the rule is removed or the object
  or element is deleted from the switch. This can break Fabric connectivity and policies.
- `keep` only applies where Fabric doesn't plan anything. If a later Fabric change starts planning the same object or
  element, Fabric's version replaces the manual one and the rule is skipped with a warning.

## Not covered

- Other switch operating systems: Celestica SONiC+ and Cumulus are not supported, the annotation is rejected on them.
- Other configuration, e.g. BGP neighbors, VRFs, interfaces, LST groups.
