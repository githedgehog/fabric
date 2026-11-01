# BCM mutate rules

> [!NOTE]
> Added in the 26.05 release as an experimental (alpha) feature for switches running Broadcom SONiC.

> [!WARNING]
> BCM mutate is an experimental feature and will be removed in one of the future releases. Until then, the annotation
> name, rule format and behavior may change in any future release. It is a temporary escape hatch for urgent
> situations and is not meant for long-term use. If you use it, we highly recommend opening an issue in the
> [Fabric repository](https://github.com/githedgehog/fabric/issues) that describes your use case and contacting
> Hedgehog support, so we are aware of it and can plan a supported way to achieve the same result.
>
> BCM mutate rules make the agent accept configuration that is managed by hand instead of the configuration planned
> by Fabric. Incorrect or outdated rules can break the switch configuration entirely, compromise tenant isolation
> between VPCs, and leave the agent unable to apply any configuration. Use them only when there is no other way to
> achieve the result, review every rule carefully, and verify the switch configuration after each change.

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

> [!NOTE]
> After changing the rules, wait for at least one agent enforcement run (every 2 minutes by default, logged by the
> agent as `Config applied`) and check that `status.mutate.applied` lists every rule you expect to be applied. A rule
> that is missing there was skipped, the agent log shows why. Check it again after Fabric configuration changes that
> affect the switch, as they can make rules skipped as well.

Annotation changes don't change the generation of the Agent object, so they aren't covered by the applied generation
tracking: `lastAppliedGen`, the `Ready` state in `kubectl fabric inspect` and anything waiting for the applied
generation don't show whether a rule change was applied. Check `status.mutate.applied`, the notes and the agent log
instead.

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
- The same object or element can only have one rule. A whole ACL, route map, prefix list, community list or AS-path
  list can't be combined with rules for its elements or members.

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
  the switch. Members are sorted and de-duplicated.
  An add of a member that is already in the list, or a remove of a member that isn't or of the last member of the
  list, is skipped with a warning.
- Rules for an element (an ACL rule, a statement, an entry, a static route, an ACL binding, a list member) are
  skipped with a warning if Fabric doesn't plan the ACL, route map, prefix list, VRF, interface or list that contains
  it.
- Rules for whole objects are applied before the rules for elements, so the order of the rules doesn't matter.
- A kept or overridden ACL binding or route map statement is skipped with a warning if something it references
  wouldn't exist after the configuration run: the bound ACL, or the route map a statement calls or the prefix,
  community and AS-path lists it matches. That's the case if the object is neither planned by Fabric nor kept by a
  rule, including when Fabric stops planning an object it created before, e.g. after an external is deleted. The
  skipped binding or statement is then removed from the switch together with the object it references, as otherwise
  the switch would refuse to delete an object that is still in use. This check is best effort: it only covers the
  references the agent reads from the switch.

Content taken from the switch is identical on both sides of the comparison, so the agent never writes to it. Settings
the agent doesn't know about are kept as well.

Rules only see the configuration the agent reads from the switch. Objects in a state the agent doesn't support aren't
read at all, so rules for them are skipped as not on the switch and Fabric's plan applies, e.g.:

- disabled BFD profiles,
- community and AS-path lists with an action other than permit,
- route map statements with community actions other than adding inline communities, or without an action.

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

Elements kept with `keep` are lost if Fabric starts planning the same sequence number later, and the position of a
manual element relative to the Fabric ones decides what it does: lower sequence numbers are evaluated first, and a
matching ACL rule or route map statement usually ends the evaluation.

Fabric doesn't reserve any sequence numbers for manual elements, and the numbers it uses depend on the configuration
and can change between releases. Pick them carefully for each object: check the current configuration of the ACL or
route map on the switch, see which Fabric rules or statements come before and after your element and what they match,
and leave room for Fabric to grow.

The following ranges are only examples of numbers that are often unused by the current Fabric release:

| Object                               | Example sequence numbers |
|--------------------------------------|--------------------------|
| ACLs, except `ext-inbound--*`        | `65000`–`65534`          |
| `ext-inbound--*` ACLs                | `8`–`9`                  |
| Route maps, except `l2vpn-neighbors` | `1`–`4`, `65000`–`65499` |

For example, `8`–`9` in an `ext-inbound--*` ACL comes after the switch protection rules (`1`–`6`) and before the rules
from the ExternalAttachment ACL (`10` and higher), and `1`–`4` in a route map comes before all Fabric statements, so a
manual statement there that permits routes skips the Fabric statements for them.

## Caveats

- Your own ACLs, route maps and lists are only used if something refers to them: an ACL through a kept `aclInterface`
  binding, a route map through a `call` from a statement you keep in a Fabric route map, a list through a `match` in
  such a statement. On their own they just stay on the switch.
- Review the dependencies of your manual configuration before adding or removing rules. If Fabric deletes an object
  that something on the switch still uses, e.g. a bound ACL or a route map or list used by a statement, the switch
  can refuse the deletion and the agent keeps failing to apply its configuration until the rules or the manual
  configuration are fixed. The agent only checks the references it reads (see [How rules work](#how-rules-work)), not
  those of whole manual route maps or of configuration it doesn't read.
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
