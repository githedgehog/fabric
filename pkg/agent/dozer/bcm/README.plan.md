## Overview

This file is an attempt to document the configuration that is pushed onto a Broadcom SONiC switch running our agent software.
The aim is to detail, for each of the objects in our API (e.g. Connections, VPCs, VPC Peerings, Externals...), what configuration is
pushed onto the switch and why. This will help us to understand the reasoning behind the configuration, and it will guide future
agent refactors and improvements.

## Connections

### Fabric Connections (i.e. spine-leaf)

### Mesh Connections (i.e. leaf-leaf)

### MCLAG Connections

### ESLAG Connections

### Bundled and Unbundled Connections

### External Connections

### Static Externals

## VPCs

### L2VNI (default) VPCs
The following configuration is pushed onto a leaf only when a subnet of the VPC is _attached_
to a connection which belongs to it (i.e. where one of the two endpoints is a port on the switch):
1. We create a VRF for the VPC:
    ```
    ip vrf VrfVvpc-01
    ```
1. We create a VLAN for each of the subnets of the VPC attached to the leaf:
    - the VLAN is placed in the VRF of the VPC
    - an anycast address is configured on the interface, using the address from the subnet gateway
    - if DHCP was enabled in the subnet, we enable DHCP relay on the interface, with link select and VRF select
    - neighbor suppression is enabled on the interface with default parameters
    - we advertise attached host routes in BGP for the subnet
    ```
    interface Vlan1001
      description "VPC vpc-01/default"
      neigh-suppress
      ip vrf forwarding VrfVvpc-01
      ip anycast-address 10.0.1.1/24
      ip dhcp-relay 172.30.0.1
      ip dhcp-relay source-interface Management0
      ip dhcp-relay link-select
      ip dhcp-relay vrf-select
      ip attached-host advertise 250
    ```
1. The VLAN above is enabled on the physical interface corresponding to the connection attached to the VPC, e.g. assuming this was Ethernet4:
    ```
    interface Ethernet4
      [..]
      switchport trunk allowed Vlan 1001
    ```
1. We create an IRB VLAN interface for this VPC:
    - the IRB interface is placed in the VRF of the VPC
    - neighbor suppression is enabled on the interface with default parameters (**TODO: is this needed?**)
    ```
    interface Vlan3000
      description "VPC vpc-01 IRB"
      neigh-suppress
      ip vrf forwarding VrfVvpc-01
    ```
1. Under the vtep interface configuration, we map the Subnet VLAN to an L2VNI, and the IRB VLAN + VPC VRF to an L3VNI:
    ```
    interface vxlan vtepfabric
      [..]
      map vni 101 vlan 1001
      map vni 100 vlan 3000
      map vni 100 vrf VrfVvpc-01
    ```
1. We create a prefix list for prefixes not belonging to the attached subnet(s). This is used later in VPC peering.
    ```
    ip prefix-list vpc-not-subnets--vpc-01 seq 1 deny 10.0.1.0/24 le 32
    ip prefix-list vpc-not-subnets--vpc-01 seq 65535 permit 0.0.0.0/0 le 32
    ```
1. We create a prefix list for prefixes belonging to the attached subnet(s):
    ```
    ip prefix-list vpc-subnets--vpc-01 seq 1 permit 10.0.1.0/24 le 32
    ```
1. We create a BGP community list for peers of this VPC. At first it will contain a single element, which is
the community for the VPC itself. These communities use a base from the agent config (in our vlabs this is
going to be `50000`) and an index which is the VNI of the VPC divided by 100. The community will then be
in the form `<base>:<vni/100>`, e.g. for VNI `100` the community will be `50000:1`.
    ```
    bgp community-list standard vpc-peers--vpc-01 permit 50000:1
    ```
1. We create a route map to filter the redistribution of connected routes:
    - we deny any route that matches the prefix list of the VPC loopback addresses, used
      for the deprecated loopback workaround. This should go as soon as we fully remove the workaround.
    - we permit any route that matches the prefix list of the VPC subnets, and we set the community
      for the VPC on these routes, to tag them as origining from this VPC.
    - we permit any route that matches the prefix list of the VPC static external subnets
    - we explicitly deny everything else. This is superfluous as the default action is to deny
    ```
    route-map vpc-redistribute-connected--vpc-01 deny 1
     match ip address prefix-list vpc-loopback-prefix
    !
    route-map vpc-redistribute-connected--vpc-01 permit 5
     match ip address prefix-list vpc-subnets--vpc-01
     set community 50000:1
    !
    route-map vpc-redistribute-connected--vpc-01 permit 6
     match ip address prefix-list vpc-static-ext-subnets--vpc-01
    !
    route-map vpc-redistribute-connected--vpc-01 deny 10
    !
    ```
1. We create a similar route map for static routes redistribution:
    - we deny any route that matches the prefix list of the VPC loopback addresses, used
      for the deprecated loopback workaround. This should go as soon as we fully remove the workaround.
    - we permit any route that matches the prefix list of the VPC [static external](#static-externals) subnets
    - we permit any route that matches the prefix list of the VPC external prefixes (see the [Externals section](#externals))
    - we implicitly deny everything else
   ```
   route-map vpc-redistribute-static--vpc-01 deny 1
    match ip address prefix-list vpc-loopback-prefix
   !
   route-map vpc-redistribute-static--vpc-01 permit 5
    match ip address prefix-list vpc-static-ext-subnets--vpc-01
   !
   route-map vpc-redistribute-static--vpc-01 permit 10
    match ip address prefix-list vpc-ext-prefixes--vpc-01
   !
   ```
1. We create a route map to filter routes imported in the VPC VRF, e.g. from VPC we are peering with:
    - we deny any route whose next-hop matches the prefix list of the VPC loopback addresses, used
      for the deprecated loopback workaround. This should go as soon as we fully remove the workaround.
    - we permit any route that matches the community list for peers of this VPC (which includes the VPC itself).
      As shown in the redistribute connected route map, this will include the subnets of all the peered VPCs.
    - we also permit any route that matches the prefix list of the VPC peers and that has no community.
      This prefix list so far only contains the prefixes of the VPC subnets peered with this VPC, so they
      should already be covered by the previous statement. **TODO: understand why we are using this two
      rules in a chain.**
    - we explicitly deny everything else. This is superfluous as the default action is to deny
    ```
    route-map import-vrf--vpc-01 deny 1
     match ip next-hop prefix-list vpc-loopback-prefix
    !
    route-map import-vrf--vpc-01 permit 50000
     match community vpc-peers--vpc-01
    !
    route-map import-vrf--vpc-01 permit 50001
     match ip address prefix-list vpc-peers--vpc-01
     match community no-community
    !
    route-map import-vrf--vpc-01 deny 65535
    !
    ```
1. Finally, we create a BGP instance for this VRF:
    - we set the router-id to the Protocol IP of the switch
    - we redistribute attached-host routes
    - we redistribute connected routes using the route map created above
    - we redistribute static routes using the route map created above
    - we set maximum-paths to 16 for eBGP multipath
    - we disable iBGP multipath
    - we import routes into the VRF using the route map created above
    - we enable EVPN advertisement of IPv4 unicast routes and filter out attached-host routes
      to avoid duplicate advertisements, as indicated in the BCM SONiC user guide. The route-map
      is created by default on all switches as it is VPC independent
    - we enable duplicate address detection
    ```
    router bgp 65101 vrf VrfVvpc-01
     router-id 172.30.8.2
     log-neighbor-changes
     timers 60 180
     !
     address-family ipv4 unicast
      redistribute attached-host
      redistribute connected route-map vpc-redistribute-connected--vpc-01
      redistribute static route-map vpc-redistribute-static--vpc-01
      maximum-paths 16
      maximum-paths ibgp 1
      import vrf route-map import-vrf--vpc-01
     !
     address-family l2vpn evpn
      advertise ipv4 unicast route-map filter-attached-hosts
      dup-addr-detection
    !
    ```

#### Subnet restrictions
A subnet from a VPC can be isolated (i.e. not allowed to communicate with other subnets in the same VPC)
and/or restricted (i.e. hosts on that subnet are not allowed to communicate with each other). Let's see
how the configuration changes, based on an example where we have VPC vpc-01 with two subnets subnet-01
(VLAN 1001, 10.0.1.0/24) and subnet-02 (VLAN 1002, 10.0.2.0/24).

When restricting subnet-01, we create an ACL to prevent hosts on that subnet from communicating with each other,
and we add it to the VLAN interface of subnet-01:
```
ip access-list vpc-filtering--vpc-01--subnet-01
 remark vpc-filtering--vpc-01--subnet-01
 seq 1 deny ip 10.0.1.0/24 10.0.1.0/24
 seq 65535 permit ip any any
!
interface Vlan1001
 ip access-group vpc-filtering--vpc-01--subnet-01 in
```

When isolating subnet-01, we create an ACL to drop any traffic destined to that subnet, and we apply it to
all of the other subnet VLAN interfaces in the VPC, so in this case to VLAN 1002 of subnet-02:
```
ip access-list vpc-filtering--vpc-01--subnet-02
 remark vpc-filtering--vpc-01--subnet-02
 seq 100 discard ip any 10.0.1.0/24
 seq 65535 permit ip any any
!
interface Vlan1002
 ip access-group vpc-filtering--vpc-01--subnet-02 in
```
The isolation flag can be overridden by an explicit permit list in the VPC object, whose effect
is to suppress the above config for the subnets included in the permit list.

### L3VNI VPCs
L3VNI VPCs attached to a switch generate exactly the same config as L2VNI VPCs, with only two differences:
1. We do not create VNI mappings for the subnet VLANs (there is no L2VNI, obviously)
1. We do not filter attached host routes in the BGP EVPN address family. Without L2VNIs, these routes
   have to be advertised to allow communication between hosts in the VPC.

On top of that, there is the DHCP hack that we implement in our server to learn the IP/MAC association,
but that has no impact on the switch configuration.

## VPC Peerings
Here is what happens when we peer a VPC subnet attached on our leaf (e.g. `vpc-01/subnet-01` with prefix `10.0.1.0/24`)
with another VPC subnet (`vpc-02/subnet-01` with prefix `10.0.2.0/24`):
1. If a subnet of `vpc-02` is not already attached to this leaf, we go through all of the steps described
   above for the L2VNI VPC attachment, **with the exception of the subnets VLANs** (steps 2 and 3). In other words,
   we create the VRF, the IRB VLAN interface, the VNI mappings, the prefix lists and route-maps, the community list,
   and the BGP instance for `vpc-02`.
1. We update the community list for peers of `vpc-01` to include also the community for `vpc-02`, and viceversa:
    ```
    bgp community-list standard vpc-peers--vpc-01 permit 50000:2
    bgp community-list standard vpc-peers--vpc-02 permit 50000:1
    ```
1. We add the prefixes of the peered subnets to the prefix list of VPC peers for both VPCs, where
   the index used is the VNI associated with the subnet's VLAN:
    ```
    ip prefix-list vpc-peers--vpc-01 seq 201 permit 10.0.2.0/24 le 32
    ip prefix-list vpc-peers--vpc-02 seq 101 permit 10.0.1.0/24 le 32
    ```
1. We configure route leaking between the two VPC VRFs by adding an import VRF statement in each
   of the two BGP instances:
    ```
    router bgp 65101 vrf VrfVvpc-01
     address-family ipv4 unicast
      import vrf VrfVvpc-02
    !
    router bgp 65101 vrf VrfVvpc-02
     address-family ipv4 unicast
      import vrf VrfVvpc-01
    !
    ```

### Subnet permits for peering
When peering two VPC subnets, it is possible to specify lists of permitted subnets for each side. In this case,
only the subnets included in each of the permit list will be allowed to communicate with each other. This is
implemented by adding ACLs to the VLAN interfaces of each of the subnets attached to the leaf. Continuing
the example above, let's assume we have a vpc-02 with two subnets, subnet-01 with prefix `10.0.3.0/24` and
subnet-02 with prefix `10.0.4.0/24`, and we want to peer `vpc-01/subnet-01` only with `vpc-02/subnet-02`.
We would add the following ACL to VLAN 1001 of `vpc-01/subnet-01`:
```
ip access-list vpc-filtering--vpc-01--subnet-01
 remark vpc-filtering--vpc-01--subnet-01
 seq 102 deny ip any 10.0.3.0/24
 seq 65535 permit ip any any
!
interface Vlan1001
 ip access-group vpc-filtering--vpc-01--subnet-01 in
```
And the following ACL to VLAN 1002 of `vpc-02/subnet-02`:
```
ip access-list vpc-filtering--vpc-01--subnet-02
 remark vpc-filtering--vpc-01--subnet-02
 seq 102 deny ip any 10.0.3.0/24
 seq 103 deny ip any 10.0.4.0/24
 seq 65535 permit ip any any
!
interface Vlan1002
 ip access-group vpc-filtering--vpc-01--subnet-02 in
```
And of course we would do something similar on `vpc-02` side.

## Externals

### BGP speaking Externals

#### The External object

#### External Attachments

#### External Peerings

### L2 Externals

#### The External object

#### External Attachments
