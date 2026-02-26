## Overview

This file is an attempt to document the configuration that is pushed onto a Broadcom SONiC switch running our agent software.
The aim is to detail, for each of the objects in our API (e.g. Connections, VPCs, VPC Peerings, Externals...), what configuration is
pushed onto the switch and why. This will help us to understand the reasoning behind the configuration, and it will guide future
agent refactors and improvements.

## Switch Invariants

There's quite a bit of basic config that is applied on all switches regardless of the actual connections
instantiated via the CRDs. To start with:

1. We set the hostname as the switch name in the CRD (e.g. "ds5000-01"):
    ```
    hostname <SWITCH_NAME>
    ```
1. We enable LLDP and configure some basic parameters:
    ```
    lldp timer 5
    lldp system-name <SWITCH_NAME>
    lldp system-description "Hedgehog Fabric"
    ```
1. We enable IP anycast for both v4 and v6, and setup an anycast mac address (hardcoded for all switches):
    ```
    ip anycast-mac-address 00:00:00:11:11:11
    ip anycast-address enable
    ipv6 anycast-address enable
    ```
1. We create a protocol loopback, which is used as the router-id and source IP for
BGP sessions:
    ```
    interface Loopback 1
     description "Protocol loopback"
     ip address 172.30.8.0/32
    !
    ```
1. For leaves, we also create a VTEP loopback, which is used as the source for VxLAN traffic:
    ```
    interface Loopback 2
     description "VTEP loopback"
     ip address 172.30.12.0/32
    !
    ```
1. We configure the management port, which connects the switch with the controller:
    ```
    interface Management0
     description "Management link"
     mtu 1500
     autoneg on
     speed 1000
     ip address 172.30.0.7/21
    !
    ```
1. We create BGP community lists based on the gateway priority levels defined in the Agent configuration.
These will be used by gateways depending on the priority group of a particular Expose; together with
the route-map described further down, they allow us to prefer prefixes advertised by a particular
gateway over another. These are the community lists generated with the default configuration:
    ```
    bgp community-list standard gw-prio-0 permit 50001:0
    bgp community-list standard gw-prio-1 permit 50001:1
    bgp community-list standard gw-prio-2 permit 50001:2
    bgp community-list standard gw-prio-3 permit 50001:3
    bgp community-list standard gw-prio-4 permit 50001:4
    bgp community-list standard gw-prio-5 permit 50001:5
    bgp community-list standard gw-prio-6 permit 50001:6
    bgp community-list standard gw-prio-7 permit 50001:7
    bgp community-list standard gw-prio-8 permit 50001:8
    bgp community-list standard gw-prio-9 permit 50001:9
    ```
1. We create a route-map applied to each L2VPN neighbor in the default VRF (in the `in` direction).
This route-map serves multiple purposes:
    - it matches on the gateway priority-related community lists described above, and for each of
    those it sets a local preference, with a lower priority implying a higher local preference.
    This is needed to ensure that we prefer the desired routes in a multi-gateway scenario.
    - it is a place-holder for rules used in [remote peering](#remote-peering)
    - it sets the local preference of routes matching any [externals](#externals)' community to a high
    value of 500, to make sure that they win over similar VPC routes. Note that this value is currently
    higher than the highest preference value for gateways, which means that in case of conflict prefixes
    advertised by an external will always win over prefixes advertised by a gateway; we might want to make
    this behavior configurable at some point.
    - it allows any other non-matched prefix through.
    ```
    route-map l2vpn-neighbors permit 1
    match community gw-prio-0
    set local-preference 110
    !
    route-map l2vpn-neighbors permit 2
    match community gw-prio-1
    set local-preference 109
    !
    route-map l2vpn-neighbors permit 3
    match community gw-prio-2
    set local-preference 108
    !
    route-map l2vpn-neighbors permit 4
    match community gw-prio-3
    set local-preference 107
    !
    route-map l2vpn-neighbors permit 5
    match community gw-prio-4
    set local-preference 106
    !
    route-map l2vpn-neighbors permit 6
    match community gw-prio-5
    set local-preference 105
    !
    route-map l2vpn-neighbors permit 7
    match community gw-prio-6
    set local-preference 104
    !
    route-map l2vpn-neighbors permit 8
    match community gw-prio-7
    set local-preference 103
    !
    route-map l2vpn-neighbors permit 9
    match community gw-prio-8
    set local-preference 102
    !
    route-map l2vpn-neighbors permit 10
    match community gw-prio-9
    set local-preference 101
    !
    route-map l2vpn-neighbors permit 65525
     match community all-externals
     set local-preference 500
    !
    route-map l2vpn-neighbors permit 65535
    ```
1. We create the following prefix-list matching any /32 belonging to the VTEP subnet prefix:
    ```
    ip prefix-list all-vtep-prefixes seq 10 permit 172.30.12.0/22 le 32
    ```
    i.e. `172.30.12.0/22` above is the VTEP Subnet as defined in the Fabric config
1. We create the following route-map, used on both spines and mesh leaves to filter
which routes are redistributed, as we will see:
    ```
    route-map loopback-all-vteps permit 10
     match ip address prefix-list all-vtep-prefixes
    !
    route-map loopback-all-vteps permit 100
     match ip address prefix-list static-ext-subnets
    ```
1. We create the following prefix list and corresponding route-map to only advertise
the protocol loopback on the fabric point-to-point links between switches:
    ```
    ip prefix-list protocol-loopback-prefix seq 10 permit 172.30.8.0/32 le 32
    route-map protocol-loopback-only permit 10
     match ip address prefix-list protocol-loopback-prefix
    !
    ```
1. We create the following route-map to filter attached-host routes from BGP exports
in L2VNI mode (see #vpcs):
    ```
    route-map filter-attached-hosts deny 10
     match source-protocol attached-host
    !
    route-map filter-attached-hosts permit 100
    !
    ```
1. We create some basic BGP configuration for the default VRF:
  - ASN is picked based on the hydration rules and the fabric config
  - Protocol IP is used as router-id and also advertised in the IPv4 Unicast AF
  - ECMP of up to 64 paths for eBGP, none for iBGP (which we do not use)
  - redistribute static and connected routes in default VRF / underlay
  - enable L2VPN, advertise all VNIs, and duplicate address detection
    ```
    router bgp 65101
     router-id 172.30.8.0
     log-neighbor-changes
     timers 60 180
     !
     address-family ipv4 unicast
      redistribute connected
      redistribute static
      maximum-paths 64
      maximum-paths ibgp 1
      network 172.30.8.0/32
     !
     address-family l2vpn evpn
      advertise-all-vni
      dup-addr-detection
    !
    ```
1. For leaves, we create the vxlan interface, set its source address to the VTEP
loopback, and set qos-mode to preserve DSCP markings when encapsulating/decapsulating
traffic, which is needed for RoCEv2:
    ```
    interface vxlan vtepfabric
     source-ip Loopback2
     qos-mode pipe dscp 0
    !
    ```
1. We create an empty BFD profile (i.e. with all default values) which we will use
for the sessions running over spine-leaf links:
    ```
    bfd
     profile fabric
     !
    !
    ```
1. We configure NTP over the management port towards the controller:
    ```
    ntp server 172.30.0.1 minpoll 6 maxpoll 10 prefer true
    ntp source-interface Management0
    ```

On top of that, there is some additional configuration generated in response to
fields of the switch object itself, such as breakouts, port autonegotiation etc.
These are mostly self-explanatory, so I won't go over them in detail.

## IPv4 Namespaces
The following changes to the configuration are applied when creating an IPv4 namespace:

1. We create a prefix list matching the ranges in the namespace, and a route-map to
permit those, which will be used in the context of [BGP-speaking externals](#bgp-speaking-externals):
    ```
    ip prefix-list ipns-subnets--default seq 1 permit 10.0.0.0/16 le 32
    route-map ipns-subnets--default permit 10
     match ip address prefix-list ipns-subnets--default
    !
    ```
1. We create an access list to prevent traffic local to the namespace from being
routed to an external device in the presence of external peerings:
    ```
    ip access-list no-ipns-peering--default
     remark "Prevent VPCs to cross-talk via the external"
     seq 1 deny ip 10.0.0.0/16 10.0.0.0/16
     seq 65535 permit ip any any
    ```

## Connections

### Fabric Connections (i.e. spine-leaf)

For each link in a fabric connection, we:
1. configure the corresponding interface on the switch, setting it to admin-up
and assigning it a /31 IPv4 address from the hydration pool, e.g.:
    ```
    interface Ethernet104
     description "Fabric as7712-03/E1/27 as7712-03--fabric--s5232-03"
     mtu 9100
     speed 100000
     unreliable-los auto
     no shutdown
     ip address 172.30.128.5/31
    ```
1. create a BGP session in the default VRF with the peer /31 on the other side of
the fabric link, enabling IPv4 unicast with BFD and advertising our own protocol
loopback, e.g.:
    ```
    neighbor 172.30.128.6
     description "Fabric as7712-03/E1/28 as7712-03--fabric--s5232-03"
     remote-as 65100
     bfd
     bfd profile fabric
     !
     address-family ipv4 unicast
      activate
      route-map protocol-loopback-only out
     !
     address-family l2vpn evpn
    !
    ```

Additionally, once per neighboring node (no matter the number of fabric links to it)
we create a BGP session with its protocol IP, for which we have learned a route over
the point-to-point BGP sessions described above. We will use this session for both IPv4
unicast, where we will advertise all the VTEPs we know of, and for EVPN, where we will
exchange overlay routes. This session will go down if all of the point-to-point sessions
above are down, which would mean that this switch is no longer able to reach this
particular neighbor. Allowas-in is required to support remote peering, where
traffic goes to a remote fabric switch and then comes back, thus creating an ASN loop.

Here's an example config for a leaf:
```
neighbor 172.30.8.0
 description "Fabric as7712-03 loopback (spine-link)"
 remote-as 65100
 update-source 172.30.8.3
 disable-connected-check
 !
 address-family ipv4 unicast
  activate
  route-map loopback-all-vteps out
 !
 address-family l2vpn evpn
  activate
  allowas-in
  route-map evpn-default-remote-block in
!
```

And the corresponding configuration for the spine, the only difference being the v4
unicast route-map:
```
neighbor 172.30.8.3
 description "Fabric s5232-03 loopback (spine-link)"
 remote-as 65102
 update-source 172.30.8.0
 disable-connected-check
 !
 address-family ipv4 unicast
  activate
  route-map loopback-all-vteps out
 !
 address-family l2vpn evpn
  activate
  allowas-in
  route-map evpn-default-remote-block in
!
```

### Mesh Connections (i.e. leaf-leaf)

Mesh connections are very much similar to fabric ones, with the main difference being
their symmetry. In practice, the config applied to both mesh leaves is equivalent to that
of a spine in a fabric connection.

For each link in a mesh connection, we:
1. configure the corresponding interface on the switch, setting it to admin-up
and assigning it a /31 IPv4 address from the hydration pool, e.g.:
    ```
    interface Ethernet5
     description "Mesh leaf-02/E1/5 leaf-01--mesh--leaf-02"
     mtu 9100
     speed 25000
     unreliable-los auto
     no shutdown
     ip address 172.30.128.2/31
    ```
1. create a BGP session in the default VRF with the peer /31 on the other side of
the mesh link, enabling IPv4 unicast with BFD and advertising our own protocol
loopback, e.g.:
    ```
    neighbor 172.30.128.3
     description "Fabric leaf-02/E1/5 leaf-01--mesh--leaf-02"
     remote-as 65102
     bfd
     bfd profile fabric
     !
     address-family ipv4 unicast
      activate
      route-map protocol-loopback-only out
     !
     address-family l2vpn evpn
    !
    ```

Additionally, once per neighboring node (no matter the number of mesh links to it)
we create a BGP session with its protocol IP, for which we have learned a route over
the point-to-point BGP sessions described above. We will use this session for both IPv4
unicast, where we will advertise all VTEPs we know about, and for EVPN, where we will
exchange overlay routes. This session will go down if all of the point-to-point sessions
above are down, which would mean that this leaf is no longer able to reach this particular
neighbor. Allowas-in is required to support remote peering, where traffic goes to a remote
fabric switch and then comes back, thus creating an ASN loop.
```
neighbor 172.30.8.2
  description "Fabric leaf-03 loopback (mesh)"
  remote-as 65103
  update-source 172.30.8.0
  disable-connected-check
  !
  address-family ipv4 unicast
   activate
   route-map loopback-all-vteps out
  !
  address-family l2vpn evpn
   activate
   allowas-in
   route-map evpn-default-remote-block in
 !
```

#### Workaround for TH5-based platforms

For TH5-based platforms such as the DS5000, due to a limitation of the hardware, we do
something slightly different:
1. we take a VLAN from a range reserved specifically for this (by default, VLANs 3900
to 3999) and configure it as an access VLAN on the interface of the connection:
    ```
    interface Ethernet5
     description "Mesh leaf-02/E1/5 leaf-01--mesh--leaf-02"
     mtu 9100
     speed 25000
     unreliable-los auto
     no shutdown
    ```
1. we configure the hydration IP address on that VLAN interface:
    ```
    interface Vlan3901
     description "TH5 Workaround Mesh Port leaf-02/E1/5"
     ip address 172.30.128.2/31
    ```

The BGP configuration is unchanged.

### Gateway Connections

Gateway connections represent a connection between a Fabric switch and a Gateway.
For spine-leaf topologies the switch is typically a spine, while for mesh topologies
it will necessarily be a leaf.

For each link in a gateway connection, we:
1. configure the corresponding interface on the switch, setting it to admin-up
and assigning it a /31 IPv4 address from the hydration pool, e.g.:
    ```
    interface Ethernet6
     description "Gateway gateway-1/enp2s1 spine-01--gateway--gateway-1"
     mtu 9100
     speed 25000
     unreliable-los auto
     no shutdown
     ip address 172.30.128.12/31
    ```
1. create a BGP session with the other host in that /31 range. The ASN of the
gateway currently comes from config (note: we could use `remote-as external` instead).
Like for other EVPN peers in our config, we set `allowas-in` in the L2VPN AF.
We also set the `l2vpn-neighbors` route-map in the import direction, which ensures
that the correct gateway route will be picked based on priorities/communities.
    ```
    neighbor 172.30.128.13
     description "Gateway gateway-1/enp2s1 spine-01--gateway--gateway-1"
     remote-as 65534
     !
     address-family ipv4 unicast
      activate
     !
     address-family l2vpn evpn
      activate
      allowas-in
      route-map l2vpn-neighbors in
    ```

#### Workaround for TH5-based platforms

The same exact workaround steps described for Mesh connections also apply to the
gateway case, i.e. an Access VLAN from the dedicated range is configured on the switch
interface and the hydration IP address is configured on that VLAN instead.

### MCLAGDomain Connections

These are processed on switches that belong to a redundancy group of type MCLAG.
Each MCLAGDomain connection defines a pair of switches that act like a single logical
switch, and the connections between them. Specifically:
1. for each of the links defined in the `peerLinks` section of the CRD, we will add
those interfaces to a port channel, e.g.:
    ```
    interface Ethernet120
     description "PC250 MCLAG peer s5248-06/E1/55/1"
     mtu 9100
     speed 25000
     unreliable-los auto
     channel-group 250
     no shutdown
    ```
    and configure that port channel so that it can carry traffic from any usable VLAN,
    in case the MCLAG peer gets disconnected:
    ```
    interface PortChannel250
     description "MCLAG peer s5248-06"
     switchport trunk allowed Vlan 2-4094
     no shutdown
    ```
1. for each of the links defined in the `sessionLinks` section, we will similarly add
these to a port channel, e.g.:
    ```
    interface Ethernet122
     description "PC251 MCLAG session s5248-06/E1/55/3"
     mtu 9100
     speed 25000
     unreliable-los auto
     channel-group 251
     no shutdown
    ```
    and configure each end of that port channel with addresses from a /31 prefix; these
    channels are going to be used to exchange keepalives between the MCLAG peers, so that
    they can monitor each other's health.
    ```
    interface PortChannel251
     description "MCLAG session s5248-06"
     no shutdown
     ip address 172.30.95.0/31
    ```
1. each MCLAGDomain object identifies an MCLAG domain, which is configured on both
peering switches with some self-explanatory parameters:
    ```
    mclag domain 100
     source-ip 172.30.95.0
     peer-ip 172.30.95.1
     peer-link PortChannel250
     keepalive-interval 1
     session-timeout 30
     delay-restore 300
     backup-keepalive interval 30
    ```
1. we create a BGP session in the default VRF with the MCLAG peer. **TODO: why? the BCM
User Guide only mentions a BGP session for keepalives via the spine, but we always assume
a direct session connection between the peers**
    ```
    [...]
    neighbor 172.30.95.1
     description "MCLAG session s5248-06"
     remote-as internal
     !
     address-family ipv4 unicast
      activate
     !
     address-family l2vpn evpn
    !
    ```
1. we configure link state tracking; that is, we identify each link towards the spine
(or towards another mesh leaf, for mesh topologies) as belonging to a `spinelink` group:
    ```
    interface Ethernet104
     description "Fabric as7712-03/E1/27 as7712-03--fabric--s5232-03"
     [...]
     link state track spinelink upstream
    ```
    and then configure the switch to shutdown all downstream MCLAG connections
    if it detects that all of the interfaces in the link state group are down:
    ```
    link state track spinelink
     timeout 5
     downstream all-mclag
    ```

### MCLAG Connections

For each of the leaves in an MCLAG connection and each of the links, we will
configure that interface to be part of a port channel, and add that port channel
to the leaf MCLAG domain, e.g.:
```
interface Ethernet4
 description "PC1 MCLAG server-01 server-01--mclag--leaf-01--leaf-02"
 mtu 9036
 speed 25000
 unreliable-los auto
 channel-group 1
 no shutdown
!
interface PortChannel1
 description "MCLAG server-01 server-01--mclag--leaf-01--leaf-02"
 mtu 9036
 no shutdown
 mclag 100
```

### ESLAG Connections

For each of the leaves in an ESLAG connection and each of the links, we will
configure that interface to be part of a port channel, and assign that port channel
to an EVPN ethernet segment, e.g.:
```
interface Ethernet0
 description "PC2 ESLAG server-05 server-05--eslag--leaf-03--leaf-04"
 mtu 9036
 speed 25000
 unreliable-los auto
 channel-group 2
 no shutdown
!
interface PortChannel2
 description "ESLAG server-05 server-05--eslag--leaf-03--leaf-04"
 mtu 9036
 no shutdown
 system-mac f2:00:00:00:00:01
 !
 evpn ethernet-segment 00:f2:00:00:f2:00:00:00:00:01
```
The ethernet segment is created from:
- the ESLAGESIPrefix, which comes from the config and is defaulted in fabricator
to `00:f2:00:00:`
-  the ESLAGMACBase, which also comes from config and is defaulted in fabricator
to `f2:00:00:00:00:00`
- the connection id, which is allocated by the librarian and replaces the end of
the ESLAGMACBase

The system-mac is the ethernet segment minus the ESLAGESIPrefix.

*Note: there are other types of ES-ID which are autogenerated and would potentially
simplifies the config; we should at some point investigate them.*

On top of that, there is some config that is applied on all switches that belong to
a redundancy group of type ESLAG, regardless of the specific connection instances:
1. We configure some basic parameters of EVPN multi-homing. The startup delay is an
initial interval of time during the VTEP boot-up process where the ESLAG interfaces
are brought administratively down to avoid traffic loss; during this initial time,
traffic from the multi-homed servers is not load-balanced between the ESLAG servers.
The hold-time is the time in seconds to wait before the switch ages out MAC addresses
of downstream devices that are learned from the multi-homed VTEP and that have not
been used.
    ```
    evpn esi-multihoming
     mac-holdtime 60
     startup-delay 60
    ```
1. Similarly as for MCLAG, we configure link state tracking for spine / mesh upstream
links, and shutdown the downstream links towards the ethernet segments if they all go down:
    ```
    link state track spinelink
     timeout 60
     downstream all-evpn-es
    ```

### Bundled Connections

For each bundled connections we create a port channel and set all of the
interfaces that are part of the bundle as part of that port channel, e.g.:
```
interface Ethernet32
 description "PC1 Bundled server-2 server-2--bundled--ds5000-01"
 mtu 9036
 speed auto
 fec RS
 unreliable-los auto
 channel-group 1
 no shutdown
!
interface Ethernet36
 description "PC1 Bundled server-2 server-2--bundled--ds5000-01"
 mtu 9036
 speed auto
 fec RS
 unreliable-los auto
 channel-group 1
 no shutdown
!
interface PortChannel1
 description "Bundled server-2 server-2--bundled--ds5000-01"
 mtu 9036
 no shutdown
```

### Unbundled Connections

For unbundled connections we just set the interface on the leaf as admin-up
and configure some basic parameters, e.g.:
```
interface Ethernet4
 description "Unbundled server-04 server-04--unbundled--leaf-01"
 mtu 9036
 speed 25000
 unreliable-los auto
 no shutdown
```

### External Connections

An external connection represents a BGP speaker with which we want to establish
a session, exchanging routes using BGP communities as a filter. The external connection
is just the first part of the puzzle, and contains information on the interface over
which the session will be established. The result is just setting that interface up
and adding some description.
```
interface Ethernet513
 description "External ds5000-01--external--5835"
 mtu 9100
 speed 10000
 unreliable-los auto
 no shutdown
```

Mostly, the connection serves as a base for external attachments, which we will cover
in the [Externals](#externals) section.

### Static Externals

For each static external object we configure the corresponding switch interface,
with some nuances:
- if `vlan` is non-zero, we create a sub-interface with that VLAN, else
we configure the parent interface;
- if `withinVPC` is non null, we enslave the interface or sub-interface
to the VPC VRF.

We also create static ip route (in the VPC VRF if `withinVPC` is non null,
else in the default VRF) for each of the prefixes in the `subnets` list,
with the `nextHop` specified in the static external object.

Finally, we populate a prefix list with the network prefix of the `ip` field
and with all of the `subnets` listed; this prefix list is used in route maps
to filter the connected and static routes we redistribute in BGP, so essentially
this makes sure that these routes are redistributed to peers accordingly.
```
interface Ethernet0
 description "StaticExt release-test--static-external--ds5000-02"
 mtu 9036
 speed auto
 fec RS
 unreliable-los auto
 no shutdown
 ip vrf forwarding VrfVvpc-01
 ip address 172.31.255.5/24
!
ip route vrf VrfVvpc-01 10.199.0.100/32 172.31.255.1 interface Ethernet0
!
ip prefix-list vpc-static-ext-subnets--vpc-01 seq 105 permit 172.31.255.0/24 le 24
ip prefix-list vpc-static-ext-subnets--vpc-01 seq 106 permit 10.199.0.100/32 le 32
```

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
1. The VLAN above is enabled on the physical interface (or port channel) corresponding to the connection attached to the VPC, e.g. assuming this was Ethernet4:
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
      for the VPC on these routes, to tag them as originating from this VPC.
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

### Remote peering

Remote peering is deprecated; it was originally introduced as an alternative to the
loopback workaround, to deal with the CPU forwarding issue in older versions of Broadcom SONiC,
and it doesn't really have a reason to exist right now. However, it has not been removed from
the code base, and there is still a chance that we might use it if we find similar issues when
porting our product to other platforms, so for now we need to make sure we do not break it.

On any switch belonging to the remote group specified in the peering, we will configure VPCs
as in the [VPC peering](#vpc-peerings) section above, creating the VRF, the BGP configuration
etc. The only major difference is that we will add entries to the `l2vpn-neighbors` route-map,
which we first described in the [switch invariants](#switch-invariants) section, to block any
default route learned from EVPN peers with the VNI associated to that VPC; so for example:
```
route-map l2vpn-neighbors deny 101
 match evpn default-route
 match evpn vni 100
!
route-map l2vpn-neighbors deny 102
 match evpn default-route
 match evpn vni 200
!
```

The reason for this is not clear to me, and it could be a residual from old approaches using
default origination. **TODO: confirm whether this is needed and what it does in practice.**

## Externals
Even though there's a single `External` object and a single `ExternalAttachment` object, in practice
we support two types of externals. BGP speaking externals will have non-nil inbound and outbound
communities, while Static externals will have non-nil static configuration.

Creating an `External` object does not affect the switches in any major way; it is only when they are
attached to a particular leaf via an `ExternalAttachment` that that leaf configuration is impacted.
For this reason, even though some of the relevant config comes from information that lives in the
`External`, in terms of config changes we'll only refer to the creation or deletion of an attachment.

### Common external config
The following config is applied for all externals, regardless of their type:
1. We create a VRF with the prefix `VrfE` followed by the name of the external; we also create an
   IRB VLAN associated with the VRF, and we configure a VNI for both of these under the vxlan interface.
   Note that we also apply the `no-ipns-peering--<IPV4NAMESPACE>` ACL on the IRB VLAN, to prevent traffic
   local to the fabric from exiting via the external (see the [IPv4 Namespaces](#ipv4-namespaces) section).
    ```
    ip vrf VrfEext-name
    !
    interface Vlan3001
     description "External ext-name IRB"
     ip vrf forwarding VrfEext-name
     ip access-group no-ipns-peering--default in
    !
    interface vxlan vtepfabric
     map vni 300 vlan 3001
     map vni 300 vrf VrfEext-name
    !
    ```
1. We create a sub-interface to connect to the external device, place it in the VRF
of the external, and apply an access-list to prevent traffic destined to the fabric
from going out via the external attachment:
    ```
    interface Ethernet0.10
     encapsulation dot1q vlan-id 10
     no shutdown
     ip vrf forwarding VrfEext-name
     ip access-group ipns-egress--default out
    !
    ip access-list ipns-egress--default
     remark ipns-egress--default
     seq 10 deny ip any 10.0.0.0/16
     seq 65535 permit ip any any
    !
    ```
1. We configure a BGP instance for the external VRF. This is done for all externals,
not just BGP-speaking ones, because it's part of how we handle external VRFs "as VPCs"
and peer them via the gateway as needed. 
    ```
    router bgp 65101 vrf VrfEext-name
     router-id 172.30.8.1
     log-neighbor-changes
     timers 60 180
     !
     address-family ipv4 unicast
      maximum-paths 16
      maximum-paths ibgp 1
      import vrf route-map ipns-subnets--default
     !
     address-family l2vpn evpn
      advertise ipv4 unicast
      dup-addr-detection
     !
    !
    ```

### BGP-speaking externals

When attaching a BGP external to a leaf, in addition to the above, the following config is applied:
1. On the sub-interface connected to the external we configure the IP address specified
in the external attachment:
    ```
    interface Ethernet0.10
     ip address 100.1.10.1/24
     [...]
    !
    ```
1. We add the inbound community of the BGP external to two community lists,
`ext-inbound--<EXT-NAME>` and `ipns-ext-communities--<IPV4NAMESPACE>`:
    ```
    bgp community-list standard ext-inbound--ext-name permit 65102:1000
    bgp community-list standard ipns-ext-communities--default permit 65102:1000
    ```
1. We create several route-maps. In the inbound route-map, used in the in direction with the external:
  - we deny routes that match the IPv4 namespace the external belongs to
  - we allow routes that match the inbound community above, and set a local preference of 500 for these
  - we deny everything else
In the outbound route-map, used in the out direction with the external:
  - we permit any route matching the IPv4 namespace the external belongs to, and we tag those with the external's outbound community
  - we deny everything else
In the `ipns-ext-communities-<IPV4NAMESPACE>` route-map, used to filter routes leaked in the external VRF:
  - we permit routes that match the corresponding community list mentioned above
  - we deny everything else
  ```
  route-map ext-inbound--ext-name deny 5
   match ip address prefix-list ipns-subnets--default
  !
  route-map ext-inbound--ext-name permit 10
   match community ext-inbound--ext-name
   set local-preference 500
  !
  route-map ext-inbound--ext-name deny 100
  !
  route-map ext-outbound--ext-name permit 10
   match ip address prefix-list ipns-subnets--default
   set community 64102:1000
  !
  route-map ext-outbound--ext-name deny 100
  !
  route-map ipns-ext-communities--default permit 10
   match community ipns-ext-communities--default
  !
  route-map ipns-ext-communities--default deny 100
  !
  ```
1. In the BGP instance for the external VRF, we peer with the external device
using the IPv4 unicast AF. The route-maps described above are used to filter what is
advertised and accepted.
    ```
    router bgp 65101 vrf VrfEext-name
    [...]
     address-family l2vpn evpn
      advertise ipv4 unicast route-map ext-inbound--ext-name
      dup-addr-detection
     !
     neighbor 100.1.10.6
      description "External attach leaf-01--ext-name"
      remote-as 64102
      !
      address-family ipv4 unicast
       activate
       route-map ext-inbound--ext-name in
       route-map ext-outbound--ext-name out
      !
      address-family l2vpn evpn
     !
    !
    ```

### Static externals
For static externals, the following configuration is added on top of the common one:

1. For non-proxied static externals, we configure the IP address specified in the
attachment on the sub-interface connecting to the external device:
    ```
    interface Ethernet0.10
     ip address 100.1.10.2/24
     [...]
    !
    ```
For proxied externals, a /31 address is going to be allocated from the reserved range
configured in Fabricator, and proxy-arp will be enabled; the address from the range is
irrelevant but needs to be there for proxy-arp to be configurable. Proxy-arp is used so
that the border leaf will answer ARP requests for the IP which is used in the gateway
to masquerade the private VPCs' IPs.
    ```
    interface Ethernet0.10
     ip address 172.30.16.0/31
     ip proxy-arp enable remote-only
     [...]
    !
    ```
1. Static routes are added to the VRF of the external:
  - a direct route to reach the /32 address of the external device (**FIXME: not needed for the non-proxy version**)
  - one route per prefix reachable via the external, as defined by the `Prefixes` list in the external itself
    ```
    ip route vrf VrfEext-name 0.0.0.0/0 100.1.10.1 interface Ethernet0.10
    ip route vrf VrfEext-name 100.1.10.1/32 interface Ethernet0.10
    ```
1. In the BGP instance of the external VRF, static routes are redistributed,
so that peers of the static external can learn how to reach the prefixes that
are "advertised" by it:
    ```
    router bgp 65101 vrf VrfEext-name
    [...]
     address-family ipv4 unicast
      redistribute static
     !
    !
    ```

### External peerings
The configuration applied on an external peering is roughly the same
regardless of the external type, with one minor caveat I will point out.

1. We create a prefix list `import-vrf--<VPC-NAME>--<EXT-NAME>` with all
the prefixes we allow for that external:
    ```
    ip prefix-list import-vrf--vpc-01--ext-name seq 102 permit 0.0.0.0/0 le 32
    ```
1. We create another prefix list `vpc-ext-prefixes--<VPC-NAME>` with the same thing;
this appears to be used in the route-map to allow redistribution of static routes from
a VPC VRF. **TODO: why a separate prefix-list?**
    ```
    ip prefix-list vpc-ext-prefixes--vpc-01 seq 102 permit 0.0.0.0/0 le 32
    ```
1. We add entries to the route-map filtering routes leaked into the VPC VRF;
one rule is added to deny routes coming from the external for prefixes in the IPv4 namespace,
and another to set a high local preference of 500 for routes imported from the external.
Here is also the one difference between external types: for BGP externals, this second rule
will also match on the inbound community of that external.
    ```
    route-map import-vrf--vpc-01 deny 5010
     match ip address prefix-list ipns-subnets--default
     match source-vrf VrfEext-name
    !
    route-map import-vrf--vpc-01 permit 50010
     match ip address prefix-list import-vrf--vpc-01--ext-name
     match community ext-inbound--ext-name
     set local-preference 500
    !
    ```
1. We enable route leaking between the VRFs of the VPC and the external:
    ```
    router bgp 65101 vrf VrfEext-name
     address-family ipv4 unicast
      import vrf VrfVvpc-01
      [...]
     !
    !
    router bgp 65101 vrf VrfVvpc-01
     address-family ipv4 unicast
      import vrf VrfEext-name
      [...]
     !
    !
    ```
