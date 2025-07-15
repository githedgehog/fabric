# API Reference

## Packages
- [agent.githedgehog.com/v1beta1](#agentgithedgehogcomv1beta1)
- [dhcp.githedgehog.com/v1beta1](#dhcpgithedgehogcomv1beta1)
- [vpc.githedgehog.com/v1beta1](#vpcgithedgehogcomv1beta1)
- [wiring.githedgehog.com/v1beta1](#wiringgithedgehogcomv1beta1)


## agent.githedgehog.com/v1beta1

Package v1beta1 contains API Schema definitions for the agent v1beta1 API group. This is the internal API group
for the switch and control node agents. Not intended to be modified by the user.

### Resource Types
- [Agent](#agent)



#### AdminStatus

_Underlying type:_ _string_





_Appears in:_
- [SwitchStateInterface](#switchstateinterface)

| Field | Description |
| --- | --- |
| `` |  |
| `up` |  |
| `down` |  |
| `testing` |  |


#### Agent



Agent is an internal API object used by the controller to pass all relevant information to the agent running on a
specific switch in order to fully configure it and manage its lifecycle. It is not intended to be used directly by
users. Spec of the object isn't user-editable, it is managed by the controller. Status of the object is updated by
the agent and is used by the controller to track the state of the agent and the switch it is running on. Name of the
Agent object is the same as the name of the switch it is running on and it's created in the same namespace as the
Switch object.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `agent.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `Agent` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `status` _[AgentStatus](#agentstatus)_ | Status is the observed state of the Agent |  |  |


#### AgentStatus



AgentStatus defines the observed state of the agent running on a specific switch and includes information about the
switch itself as well as the state of the agent and applied configuration.



_Appears in:_
- [Agent](#agent)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `version` _string_ | Current running agent version |  |  |
| `installID` _string_ | ID of the agent installation, used to track NOS re-installs |  |  |
| `runID` _string_ | ID of the agent run, used to track agent restarts |  |  |
| `bootID` _string_ | ID for the NOS boot, used to track NOS reboots |  |  |
| `lastHeartbeat` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Time of the last heartbeat from the agent |  |  |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Time of the last attempt to apply configuration |  |  |
| `lastAttemptGen` _integer_ | Generation of the last attempt to apply configuration |  |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Time of the last successful configuration application |  |  |
| `lastAppliedGen` _integer_ | Generation of the last successful configuration application |  |  |
| `state` _[SwitchState](#switchstate)_ | Detailed switch state updated with each heartbeat |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions of the agent, includes readiness marker for use with kubectl wait |  |  |


#### BGPMessages







_Appears in:_
- [SwitchStateBGPNeighbor](#switchstatebgpneighbor)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `received` _[BGPMessagesCounters](#bgpmessagescounters)_ |  |  |  |
| `sent` _[BGPMessagesCounters](#bgpmessagescounters)_ |  |  |  |


#### BGPMessagesCounters







_Appears in:_
- [BGPMessages](#bgpmessages)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `capability` _integer_ |  |  |  |
| `keepalive` _integer_ |  |  |  |
| `notification` _integer_ |  |  |  |
| `open` _integer_ |  |  |  |
| `routeRefresh` _integer_ |  |  |  |
| `update` _integer_ |  |  |  |


#### BGPNeighborSessionState

_Underlying type:_ _string_





_Appears in:_
- [SwitchStateBGPNeighbor](#switchstatebgpneighbor)

| Field | Description |
| --- | --- |
| `` |  |
| `idle` |  |
| `connect` |  |
| `active` |  |
| `openSent` |  |
| `openConfirm` |  |
| `established` |  |


#### BGPPeerType

_Underlying type:_ _string_





_Appears in:_
- [SwitchStateBGPNeighbor](#switchstatebgpneighbor)

| Field | Description |
| --- | --- |
| `` |  |
| `internal` |  |
| `external` |  |


#### OperStatus

_Underlying type:_ _string_





_Appears in:_
- [SwitchStateInterface](#switchstateinterface)

| Field | Description |
| --- | --- |
| `` |  |
| `up` |  |
| `down` |  |
| `testing` |  |
| `unknown` |  |
| `dormant` |  |
| `notPresent` |  |
| `lowerLayerDown` |  |


#### SwitchState







_Appears in:_
- [AgentStatus](#agentstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nos` _[SwitchStateNOS](#switchstatenos)_ | Information about the switch and NOS |  |  |
| `interfaces` _object (keys:string, values:[SwitchStateInterface](#switchstateinterface))_ | Switch interfaces state (incl. physical, management and port channels) |  |  |
| `breakouts` _object (keys:string, values:[SwitchStateBreakout](#switchstatebreakout))_ | Breakout ports state (port -> breakout state) |  |  |
| `bgpNeighbors` _object (keys:string, values:[map[string]SwitchStateBGPNeighbor](#map[string]switchstatebgpneighbor))_ | State of all BGP neighbors (VRF -> neighbor address -> state) |  |  |
| `platform` _[SwitchStatePlatform](#switchstateplatform)_ | State of the switch platform (fans, PSUs, sensors) |  |  |
| `criticalResources` _[SwitchStateCRM](#switchstatecrm)_ | State of the critical resources (ACLs, routes, etc.) |  |  |
| `roce` _boolean_ | State of the roce configuration |  |  |


#### SwitchStateBGPNeighbor







_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionsDropped` _integer_ |  |  |  |
| `enabled` _boolean_ |  |  |  |
| `establishedTransitions` _integer_ |  |  |  |
| `lastEstablished` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `lastRead` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `lastResetReason` _string_ |  |  |  |
| `lastResetTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `lastWrite` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `localAS` _integer_ |  |  |  |
| `messages` _[BGPMessages](#bgpmessages)_ |  |  |  |
| `peerAS` _integer_ |  |  |  |
| `peerGroup` _string_ |  |  |  |
| `peerPort` _integer_ |  |  |  |
| `peerType` _[BGPPeerType](#bgppeertype)_ |  |  |  |
| `remoteRouterID` _string_ |  |  |  |
| `sessionState` _[BGPNeighborSessionState](#bgpneighborsessionstate)_ |  |  |  |
| `shutdownMessage` _string_ |  |  |  |
| `prefixes` _object (keys:string, values:[SwitchStateBGPNeighborPrefixes](#switchstatebgpneighborprefixes))_ |  |  |  |


#### SwitchStateBGPNeighborPrefixes







_Appears in:_
- [SwitchStateBGPNeighbor](#switchstatebgpneighbor)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `received` _integer_ |  |  |  |
| `receivedPrePolicy` _integer_ |  |  |  |
| `sent` _integer_ |  |  |  |


#### SwitchStateBreakout







_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mode` _string_ |  |  |  |
| `nosMembers` _string array_ |  |  |  |
| `status` _string_ |  |  |  |


#### SwitchStateCRM







_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `aclStats` _[SwitchStateCRMACLStats](#switchstatecrmaclstats)_ |  |  |  |
| `stats` _[SwitchStateCRMStats](#switchstatecrmstats)_ |  |  |  |


#### SwitchStateCRMACLDetails







_Appears in:_
- [SwitchStateCRMACLInfo](#switchstatecrmaclinfo)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `groupsAvailable` _integer_ |  |  |  |
| `groupsUsed` _integer_ |  |  |  |
| `tablesAvailable` _integer_ |  |  |  |
| `tablesUsed` _integer_ |  |  |  |


#### SwitchStateCRMACLInfo







_Appears in:_
- [SwitchStateCRMACLStats](#switchstatecrmaclstats)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lag` _[SwitchStateCRMACLDetails](#switchstatecrmacldetails)_ |  |  |  |
| `port` _[SwitchStateCRMACLDetails](#switchstatecrmacldetails)_ |  |  |  |
| `rif` _[SwitchStateCRMACLDetails](#switchstatecrmacldetails)_ |  |  |  |
| `switch` _[SwitchStateCRMACLDetails](#switchstatecrmacldetails)_ |  |  |  |
| `vlan` _[SwitchStateCRMACLDetails](#switchstatecrmacldetails)_ |  |  |  |


#### SwitchStateCRMACLStats







_Appears in:_
- [SwitchStateCRM](#switchstatecrm)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `egress` _[SwitchStateCRMACLInfo](#switchstatecrmaclinfo)_ |  |  |  |
| `ingress` _[SwitchStateCRMACLInfo](#switchstatecrmaclinfo)_ |  |  |  |


#### SwitchStateCRMStats







_Appears in:_
- [SwitchStateCRM](#switchstatecrm)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `dnatEntriesAvailable` _integer_ |  |  |  |
| `dnatEntriesUsed` _integer_ |  |  |  |
| `fdbEntriesAvailable` _integer_ |  |  |  |
| `fdbEntriesUsed` _integer_ |  |  |  |
| `ipmcEntriesAvailable` _integer_ |  |  |  |
| `ipmcEntriesUsed` _integer_ |  |  |  |
| `ipv4NeighborsAvailable` _integer_ |  |  |  |
| `ipv4NeighborsUsed` _integer_ |  |  |  |
| `ipv4NexthopsAvailable` _integer_ |  |  |  |
| `ipv4NexthopsUsed` _integer_ |  |  |  |
| `ipv4RoutesAvailable` _integer_ |  |  |  |
| `ipv4RoutesUsed` _integer_ |  |  |  |
| `ipv6NeighborsAvailable` _integer_ |  |  |  |
| `ipv6NeighborsUsed` _integer_ |  |  |  |
| `ipv6NexthopsAvailable` _integer_ |  |  |  |
| `ipv6NexthopsUsed` _integer_ |  |  |  |
| `ipv6RoutesAvailable` _integer_ |  |  |  |
| `ipv6RoutesUsed` _integer_ |  |  |  |
| `nexthopGroupMembersAvailable` _integer_ |  |  |  |
| `nexthopGroupMembersUsed` _integer_ |  |  |  |
| `nexthopGroupsAvailable` _integer_ |  |  |  |
| `nexthopGroupsUsed` _integer_ |  |  |  |
| `snatEntriesAvailable` _integer_ |  |  |  |
| `snatEntriesUsed` _integer_ |  |  |  |


#### SwitchStateInterface







_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ |  |  |  |
| `adminStatus` _[AdminStatus](#adminstatus)_ |  |  |  |
| `operStatus` _[OperStatus](#operstatus)_ |  |  |  |
| `mac` _string_ |  |  |  |
| `lastChanged` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `speed` _string_ |  |  |  |
| `counters` _[SwitchStateInterfaceCounters](#switchstateinterfacecounters)_ |  |  |  |
| `transceiver` _[SwitchStateTransceiver](#switchstatetransceiver)_ |  |  |  |
| `lldpNeighbors` _[SwitchStateLLDPNeighbor](#switchstatelldpneighbor) array_ |  |  |  |


#### SwitchStateInterfaceCounters







_Appears in:_
- [SwitchStateInterface](#switchstateinterface)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `inBits` _integer_ |  |  |  |
| `inBitsPerSecond` _float_ |  |  |  |
| `inDiscards` _integer_ |  |  |  |
| `inErrors` _integer_ |  |  |  |
| `inPktsPerSecond` _float_ |  |  |  |
| `inUtilization` _integer_ |  |  |  |
| `lastClear` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |
| `outBits` _integer_ |  |  |  |
| `outBitsPerSecond` _float_ |  |  |  |
| `outDiscards` _integer_ |  |  |  |
| `outErrors` _integer_ |  |  |  |
| `outPktsPerSecond` _float_ |  |  |  |
| `outUtilization` _integer_ |  |  |  |
| `queues` _object (keys:string, values:[SwitchStateInterfaceCountersQueue](#switchstateinterfacecountersqueue))_ |  |  |  |


#### SwitchStateInterfaceCountersQueue







_Appears in:_
- [SwitchStateInterfaceCounters](#switchstateinterfacecounters)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `droppedBits` _integer_ |  |  |  |
| `droppedPkts` _integer_ |  |  |  |
| `ecnMarkedBits` _integer_ |  |  |  |
| `ecnMarkedPkts` _integer_ |  |  |  |
| `transmitBits` _integer_ |  |  |  |
| `transmitBitsPerSecond` _integer_ |  |  |  |
| `transmitPkts` _integer_ |  |  |  |
| `transmitPktsPerSecond` _integer_ |  |  |  |
| `wredDroppedPkts` _integer_ |  |  |  |


#### SwitchStateLLDPNeighbor







_Appears in:_
- [SwitchStateInterface](#switchstateinterface)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `chassisID` _string_ |  |  |  |
| `systemName` _string_ |  |  |  |
| `systemDescription` _string_ |  |  |  |
| `portID` _string_ |  |  |  |
| `portDescription` _string_ |  |  |  |
| `manufacturer` _string_ |  |  |  |
| `model` _string_ |  |  |  |
| `serialNumber` _string_ |  |  |  |


#### SwitchStateNOS



SwitchStateNOS contains information about the switch and NOS received from the switch itself by the agent



_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `asicVersion` _string_ | ASIC name, such as "broadcom" or "vs" |  |  |
| `buildCommit` _string_ | NOS build commit |  |  |
| `buildDate` _string_ | NOS build date |  |  |
| `builtBy` _string_ | NOS build user |  |  |
| `configDBVersion` _string_ | NOS config DB version, such as "version_4_2_1" |  |  |
| `distributionVersion` _string_ | Distribution version, such as "Debian 10.13" |  |  |
| `hardwareVersion` _string_ | Hardware version, such as "X01" |  |  |
| `hwskuVersion` _string_ | Hwsku version, such as "DellEMC-S5248f-P-25G-DPB" |  |  |
| `kernelVersion` _string_ | Kernel version, such as "5.10.0-21-amd64" |  |  |
| `mfgName` _string_ | Manufacturer name, such as "Dell EMC" |  |  |
| `platformName` _string_ | Platform name, such as "x86_64-dellemc_s5248f_c3538-r0" |  |  |
| `productDescription` _string_ | NOS product description, such as "Enterprise SONiC Distribution by Broadcom - Enterprise Base package" |  |  |
| `productVersion` _string_ | NOS product version, empty for Broadcom SONiC |  |  |
| `serialNumber` _string_ | Switch serial number |  |  |
| `softwareVersion` _string_ | NOS software version, such as "4.2.0-Enterprise_Base" |  |  |
| `uptime` _string_ | Switch uptime, such as "21:21:27 up 1 day, 23:26, 0 users, load average: 1.92, 1.99, 2.00 " |  |  |


#### SwitchStatePlatform







_Appears in:_
- [SwitchState](#switchstate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fans` _object (keys:string, values:[SwitchStatePlatformFan](#switchstateplatformfan))_ |  |  |  |
| `psus` _object (keys:string, values:[SwitchStatePlatformPSU](#switchstateplatformpsu))_ |  |  |  |
| `temperatures` _object (keys:string, values:[SwitchStatePlatformTemperature](#switchstateplatformtemperature))_ |  |  |  |


#### SwitchStatePlatformFan







_Appears in:_
- [SwitchStatePlatform](#switchstateplatform)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `direction` _string_ |  |  |  |
| `speed` _float_ |  |  |  |
| `presence` _boolean_ |  |  |  |
| `status` _boolean_ |  |  |  |


#### SwitchStatePlatformPSU







_Appears in:_
- [SwitchStatePlatform](#switchstateplatform)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `inputCurrent` _float_ |  |  |  |
| `inputPower` _float_ |  |  |  |
| `inputVoltage` _float_ |  |  |  |
| `outputCurrent` _float_ |  |  |  |
| `outputPower` _float_ |  |  |  |
| `outputVoltage` _float_ |  |  |  |
| `presence` _boolean_ |  |  |  |
| `status` _boolean_ |  |  |  |


#### SwitchStatePlatformTemperature







_Appears in:_
- [SwitchStatePlatform](#switchstateplatform)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `temperature` _float_ |  |  |  |
| `alarms` _string_ |  |  |  |
| `highThreshold` _float_ |  |  |  |
| `criticalHighThreshold` _float_ |  |  |  |
| `lowThreshold` _float_ |  |  |  |
| `criticalLowThreshold` _float_ |  |  |  |


#### SwitchStateTransceiver







_Appears in:_
- [SwitchStateInterface](#switchstateinterface)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ |  |  |  |
| `cableClass` _string_ |  |  |  |
| `formFactor` _string_ |  |  |  |
| `connectorType` _string_ |  |  |  |
| `present` _string_ |  |  |  |
| `cableLength` _float_ |  |  |  |
| `operStatus` _string_ |  |  |  |
| `temperature` _float_ |  |  |  |
| `voltage` _float_ |  |  |  |
| `serialNumber` _string_ |  |  |  |
| `vendor` _string_ |  |  |  |
| `vendorPart` _string_ |  |  |  |
| `vendorOUI` _string_ |  |  |  |
| `vendorRev` _string_ |  |  |  |



## dhcp.githedgehog.com/v1beta1

Package v1beta1 contains API Schema definitions for the dhcp v1beta1 API group. It is the primary internal API
group for the intended Hedgehog DHCP server configuration and storing leases as well as making them available to the
end user through API. Not intended to be modified by the user.

### Resource Types
- [DHCPSubnet](#dhcpsubnet)



#### DHCPAllocated



DHCPAllocated is a single allocated IP with expiry time and hostname from DHCP requests, it's effectively a DHCP lease



_Appears in:_
- [DHCPSubnetStatus](#dhcpsubnetstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ip` _string_ | Allocated IP address |  |  |
| `expiry` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | Expiry time of the lease |  |  |
| `hostname` _string_ | Hostname from DHCP request |  |  |


#### DHCPRoute







_Appears in:_
- [DHCPSubnetSpec](#dhcpsubnetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `destination` _string_ | Destination is the destination prefix for the route |  |  |
| `gateway` _string_ | Gateway is the gateway IP address for the route |  |  |


#### DHCPSubnet



DHCPSubnet is the configuration (spec) for the Hedgehog DHCP server and storage for the leases (status). It's
primary internal API group, but it makes allocated IPs / leases information available to the end user through API.
Not intended to be modified by the user.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `dhcp.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `DHCPSubnet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DHCPSubnetSpec](#dhcpsubnetspec)_ | Spec is the desired state of the DHCPSubnet |  |  |
| `status` _[DHCPSubnetStatus](#dhcpsubnetstatus)_ | Status is the observed state of the DHCPSubnet |  |  |


#### DHCPSubnetSpec



DHCPSubnetSpec defines the desired state of DHCPSubnet



_Appears in:_
- [DHCPSubnet](#dhcpsubnet)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subnet` _string_ | Full VPC subnet name (including VPC name), such as "vpc-0/default" |  |  |
| `cidrBlock` _string_ | CIDR block to use for VPC subnet, such as "10.10.10.0/24" |  |  |
| `gateway` _string_ | Gateway, such as 10.10.10.1 |  |  |
| `startIP` _string_ | Start IP from the CIDRBlock to allocate IPs, such as 10.10.10.10 |  |  |
| `endIP` _string_ | End IP from the CIDRBlock to allocate IPs, such as 10.10.10.99 |  |  |
| `leaseTimeSeconds` _integer_ | Lease time in seconds, such as 3600 |  |  |
| `vrf` _string_ | VRF name to identify specific VPC (will be added to DHCP packets by DHCP relay in suboption 151), such as "VrfVvpc-1" as it's named on switch |  |  |
| `circuitID` _string_ | VLAN ID to identify specific subnet within the VPC, such as "Vlan1000" as it's named on switch |  |  |
| `pxeURL` _string_ | PXEURL (optional) to identify the pxe server to use to boot hosts connected to this segment such as http://10.10.10.99/bootfilename or tftp://10.10.10.99/bootfilename, http query strings are not supported |  |  |
| `dnsServers` _string array_ | DNSservers (optional) to configure Domain Name Servers for this particular segment such as: 10.10.10.1, 10.10.10.2 |  |  |
| `timeServers` _string array_ | TimeServers (optional) NTP server addresses to configure for time servers for this particular segment such as: 10.10.10.1, 10.10.10.2 |  |  |
| `interfaceMTU` _integer_ | InterfaceMTU (optional) is the MTU setting that the dhcp server will send to the clients. It is dependent on the client to honor this option. |  |  |
| `defaultURL` _string_ | DefaultURL (optional) is the option 114 "default-url" to be sent to the clients |  |  |
| `l3Mode` _boolean_ | L3 mode is used to indicate that this subnet is for a VPC in L3 mode meaning that /32 should be advertised to the clients |  |  |
| `disableDefaultRoute` _boolean_ | Disable default route advertisement in DHCP |  |  |
| `advertisedRoutes` _[DHCPRoute](#dhcproute) array_ | AdvertisedRoutes (optional) is a list of custom routes to advertise in DHCP |  |  |


#### DHCPSubnetStatus



DHCPSubnetStatus defines the observed state of DHCPSubnet



_Appears in:_
- [DHCPSubnet](#dhcpsubnet)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allocated` _object (keys:string, values:[DHCPAllocated](#dhcpallocated))_ | Allocated is a map of allocated IPs with expiry time and hostname from DHCP requests |  |  |



## vpc.githedgehog.com/v1beta1

Package v1beta1 contains API Schema definitions for the vpc v1beta1 API group. It is public API group for the VPCs
and Externals APIs. Intended to be used by the user.

### Resource Types
- [External](#external)
- [ExternalAttachment](#externalattachment)
- [ExternalPeering](#externalpeering)
- [IPv4Namespace](#ipv4namespace)
- [VPC](#vpc)
- [VPCAttachment](#vpcattachment)
- [VPCPeering](#vpcpeering)



#### External



External object represents an external system connected to the Fabric and available to the specific IPv4Namespace.
Users can do external peering with the external system by specifying the name of the External Object without need to
worry about the details of how external system is attached to the Fabric.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `External` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ExternalSpec](#externalspec)_ | Spec is the desired state of the External |  |  |
| `status` _[ExternalStatus](#externalstatus)_ | Status is the observed state of the External |  |  |


#### ExternalAttachment



ExternalAttachment is a definition of how specific switch is connected with external system (External object).
Effectively it represents BGP peering between the switch and external system including all needed configuration.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `ExternalAttachment` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ExternalAttachmentSpec](#externalattachmentspec)_ | Spec is the desired state of the ExternalAttachment |  |  |
| `status` _[ExternalAttachmentStatus](#externalattachmentstatus)_ | Status is the observed state of the ExternalAttachment |  |  |


#### ExternalAttachmentNeighbor



ExternalAttachmentNeighbor defines the BGP neighbor configuration for the external attachment



_Appears in:_
- [ExternalAttachmentSpec](#externalattachmentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `asn` _integer_ | ASN is the ASN of the BGP neighbor |  |  |
| `ip` _string_ | IP is the IP address of the BGP neighbor to peer with |  |  |


#### ExternalAttachmentSpec



ExternalAttachmentSpec defines the desired state of ExternalAttachment



_Appears in:_
- [ExternalAttachment](#externalattachment)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `external` _string_ | External is the name of the External object this attachment belongs to |  |  |
| `connection` _string_ | Connection is the name of the Connection object this attachment belongs to (essentially the name of the switch/port) |  |  |
| `switch` _[ExternalAttachmentSwitch](#externalattachmentswitch)_ | Switch is the switch port configuration for the external attachment |  |  |
| `neighbor` _[ExternalAttachmentNeighbor](#externalattachmentneighbor)_ | Neighbor is the BGP neighbor configuration for the external attachment |  |  |


#### ExternalAttachmentStatus



ExternalAttachmentStatus defines the observed state of ExternalAttachment



_Appears in:_
- [ExternalAttachment](#externalattachment)



#### ExternalAttachmentSwitch



ExternalAttachmentSwitch defines the switch port configuration for the external attachment



_Appears in:_
- [ExternalAttachmentSpec](#externalattachmentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `vlan` _integer_ | VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is used |  |  |
| `ip` _string_ | IP is the IP address of the subinterface on a switch port specified in the connection |  |  |


#### ExternalPeering



ExternalPeering is the Schema for the externalpeerings API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `ExternalPeering` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ExternalPeeringSpec](#externalpeeringspec)_ | Spec is the desired state of the ExternalPeering |  |  |
| `status` _[ExternalPeeringStatus](#externalpeeringstatus)_ | Status is the observed state of the ExternalPeering |  |  |


#### ExternalPeeringSpec



ExternalPeeringSpec defines the desired state of ExternalPeering



_Appears in:_
- [ExternalPeering](#externalpeering)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `permit` _[ExternalPeeringSpecPermit](#externalpeeringspecpermit)_ | Permit defines the peering policy - which VPC and External to peer with and which subnets/prefixes to permit |  |  |


#### ExternalPeeringSpecExternal



ExternalPeeringSpecExternal defines the External-side of the configuration to peer with



_Appears in:_
- [ExternalPeeringSpecPermit](#externalpeeringspecpermit)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the External to peer with |  |  |
| `prefixes` _[ExternalPeeringSpecPrefix](#externalpeeringspecprefix) array_ | Prefixes is the list of prefixes to permit from the External to the VPC |  |  |


#### ExternalPeeringSpecPermit



ExternalPeeringSpecPermit defines the peering policy - which VPC and External to peer with and which subnets/prefixes to permit



_Appears in:_
- [ExternalPeeringSpec](#externalpeeringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `vpc` _[ExternalPeeringSpecVPC](#externalpeeringspecvpc)_ | VPC is the VPC-side of the configuration to peer with |  |  |
| `external` _[ExternalPeeringSpecExternal](#externalpeeringspecexternal)_ | External is the External-side of the configuration to peer with |  |  |


#### ExternalPeeringSpecPrefix



ExternalPeeringSpecPrefix defines the prefix to permit from the External to the VPC



_Appears in:_
- [ExternalPeeringSpecExternal](#externalpeeringspecexternal)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `prefix` _string_ | Prefix is the subnet to permit from the External to the VPC, e.g. 0.0.0.0/0 for any route including default route.<br />It matches any prefix length less than or equal to 32 effectively permitting all prefixes within the specified one. |  |  |


#### ExternalPeeringSpecVPC



ExternalPeeringSpecVPC defines the VPC-side of the configuration to peer with



_Appears in:_
- [ExternalPeeringSpecPermit](#externalpeeringspecpermit)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the VPC to peer with |  |  |
| `subnets` _string array_ | Subnets is the list of subnets to advertise from VPC to the External |  |  |


#### ExternalPeeringStatus



ExternalPeeringStatus defines the observed state of ExternalPeering



_Appears in:_
- [ExternalPeering](#externalpeering)



#### ExternalSpec



ExternalSpec describes IPv4 namespace External belongs to and inbound/outbound communities which are used to
filter routes from/to the external system.



_Appears in:_
- [External](#external)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ipv4Namespace` _string_ | IPv4Namespace is the name of the IPv4Namespace this External belongs to |  |  |
| `inboundCommunity` _string_ | InboundCommunity is the inbound community to filter routes from the external system (e.g. 65102:5000) |  |  |
| `outboundCommunity` _string_ | OutboundCommunity is theoutbound community that all outbound routes will be stamped with (e.g. 50000:50001) |  |  |


#### ExternalStatus



ExternalStatus defines the observed state of External



_Appears in:_
- [External](#external)



#### IPv4Namespace



IPv4Namespace represents a namespace for VPC subnets allocation. All VPC subnets within a single IPv4Namespace are
non-overlapping. Users can create multiple IPv4Namespaces to allocate same VPC subnets.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `IPv4Namespace` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[IPv4NamespaceSpec](#ipv4namespacespec)_ | Spec is the desired state of the IPv4Namespace |  |  |
| `status` _[IPv4NamespaceStatus](#ipv4namespacestatus)_ | Status is the observed state of the IPv4Namespace |  |  |


#### IPv4NamespaceSpec



IPv4NamespaceSpec defines the desired state of IPv4Namespace



_Appears in:_
- [IPv4Namespace](#ipv4namespace)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subnets` _string array_ | Subnets is the list of subnets to allocate VPC subnets from, couldn't overlap between each other and with Fabric reserved subnets |  | MaxItems: 20 <br />MinItems: 1 <br /> |


#### IPv4NamespaceStatus



IPv4NamespaceStatus defines the observed state of IPv4Namespace



_Appears in:_
- [IPv4Namespace](#ipv4namespace)



#### VPC



VPC is Virtual Private Cloud, similar to the public cloud VPC it provides an isolated private network for the
resources with support for multiple subnets each with user-provided VLANs and on-demand DHCP.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `VPC` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[VPCSpec](#vpcspec)_ | Spec is the desired state of the VPC |  |  |
| `status` _[VPCStatus](#vpcstatus)_ | Status is the observed state of the VPC |  |  |


#### VPCAttachment



VPCAttachment is the Schema for the vpcattachments API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `VPCAttachment` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[VPCAttachmentSpec](#vpcattachmentspec)_ | Spec is the desired state of the VPCAttachment |  |  |
| `status` _[VPCAttachmentStatus](#vpcattachmentstatus)_ | Status is the observed state of the VPCAttachment |  |  |


#### VPCAttachmentSpec



VPCAttachmentSpec defines the desired state of VPCAttachment



_Appears in:_
- [VPCAttachment](#vpcattachment)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subnet` _string_ | Subnet is the full name of the VPC subnet to attach to, such as "vpc-1/default" |  |  |
| `connection` _string_ | Connection is the name of the connection to attach to the VPC |  |  |
| `nativeVLAN` _boolean_ | NativeVLAN is the flag to indicate if the native VLAN should be used for attaching the VPC subnet |  |  |


#### VPCAttachmentStatus



VPCAttachmentStatus defines the observed state of VPCAttachment



_Appears in:_
- [VPCAttachment](#vpcattachment)



#### VPCDHCP



VPCDHCP defines the on-demand DHCP configuration for the subnet



_Appears in:_
- [VPCSubnet](#vpcsubnet)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `relay` _string_ | Relay is the DHCP relay IP address, if specified, DHCP server will be disabled |  |  |
| `enable` _boolean_ | Enable enables DHCP server for the subnet |  |  |
| `range` _[VPCDHCPRange](#vpcdhcprange)_ | Range (optional) is the DHCP range for the subnet if DHCP server is enabled |  |  |
| `options` _[VPCDHCPOptions](#vpcdhcpoptions)_ | Options (optional) is the DHCP options for the subnet if DHCP server is enabled |  |  |


#### VPCDHCPOptions



VPCDHCPOptions defines the DHCP options for the subnet if DHCP server is enabled



_Appears in:_
- [VPCDHCP](#vpcdhcp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pxeURL` _string_ | PXEURL (optional) to identify the pxe server to use to boot hosts connected to this segment such as http://10.10.10.99/bootfilename or tftp://10.10.10.99/bootfilename, http query strings are not supported |  |  |
| `dnsServers` _string array_ | DNSservers (optional) to configure Domain Name Servers for this particular segment such as: 10.10.10.1, 10.10.10.2 |  | Optional: \{\} <br /> |
| `timeServers` _string array_ | TimeServers (optional) NTP server addresses to configure for time servers for this particular segment such as: 10.10.10.1, 10.10.10.2 |  | Optional: \{\} <br /> |
| `interfaceMTU` _integer_ | InterfaceMTU (optional) is the MTU setting that the dhcp server will send to the clients. It is dependent on the client to honor this option. |  |  |
| `leaseTimeSeconds` _integer_ | Lease time in seconds, such as 3600 |  |  |
| `disableDefaultRoute` _boolean_ | Disable default route advertisement. For L3VNI VPCs, a classless static route to the VPC subnet<br />will be advertised if this option is enabled. |  |  |
| `advertisedRoutes` _[VPCDHCPRoute](#vpcdhcproute) array_ | Advertise custom routes to the clients via the classless static route option. If non-empty,<br />and unless the disable default route flag is enabled, a default route via the VPC gateway<br />will be added automatically. |  | Optional: \{\} <br /> |


#### VPCDHCPRange



VPCDHCPRange defines the DHCP range for the subnet if DHCP server is enabled



_Appears in:_
- [VPCDHCP](#vpcdhcp)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `start` _string_ | Start is the start IP address of the DHCP range |  |  |
| `end` _string_ | End is the end IP address of the DHCP range |  |  |


#### VPCDHCPRoute







_Appears in:_
- [VPCDHCPOptions](#vpcdhcpoptions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `destination` _string_ | Destination is the destination prefix for the route |  |  |
| `gateway` _string_ | Gateway is the gateway IP address for the route |  |  |


#### VPCMode

_Underlying type:_ _string_

VPCMode defines how VPCs are implemented on the switches



_Appears in:_
- [VPCSpec](#vpcspec)

| Field | Description |
| --- | --- |
| `` | L2VNI: L2VPN EVPN with VRF per VPC and L2VNI per VPC subnet<br /> |
| `l3vni` | L3VNI: L2VPN EVPN with VRF per VPC and only L3VNI per VPC, hosts should have /32 IP addresses in the VPC subnets<br /> |
| `l3flat` | (EXPERIMENTAL) L3Flat: All VPCs are configured within default VRF and ACLs are used for isolation and VPC peerings, hosts should have /32 IP addresses in the VPC subnets<br /> |


#### VPCPeer







_Appears in:_
- [VPCPeeringSpec](#vpcpeeringspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subnets` _string array_ | Subnets is the list of subnets to advertise from current VPC to the peer VPC |  | MaxItems: 10 <br />MinItems: 1 <br /> |


#### VPCPeering



VPCPeering represents a peering between two VPCs with corresponding filtering rules.
Minimal example of the VPC peering showing vpc-1 to vpc-2 peering with all subnets allowed:


	spec:
	  permit:
	  - vpc-1: {}
	    vpc-2: {}





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `VPCPeering` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[VPCPeeringSpec](#vpcpeeringspec)_ | Spec is the desired state of the VPCPeering |  |  |
| `status` _[VPCPeeringStatus](#vpcpeeringstatus)_ | Status is the observed state of the VPCPeering |  |  |


#### VPCPeeringSpec



VPCPeeringSpec defines the desired state of VPCPeering



_Appears in:_
- [VPCPeering](#vpcpeering)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _string_ |  |  |  |
| `permit` _[map[string]VPCPeer](#map[string]vpcpeer) array_ | Permit defines a list of the peering policies - which VPC subnets will have access to the peer VPC subnets. |  | MaxItems: 10 <br />MinItems: 1 <br /> |


#### VPCPeeringStatus



VPCPeeringStatus defines the observed state of VPCPeering



_Appears in:_
- [VPCPeering](#vpcpeering)



#### VPCSpec



VPCSpec defines the desired state of VPC.
At least one subnet is required.



_Appears in:_
- [VPC](#vpc)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mode` _[VPCMode](#vpcmode)_ | Mode is the VPC mode that defines how the VPCs are configured on the switches |  |  |
| `subnets` _object (keys:string, values:[VPCSubnet](#vpcsubnet))_ | Subnets is the list of VPC subnets to configure |  |  |
| `ipv4Namespace` _string_ | IPv4Namespace is the name of the IPv4Namespace this VPC belongs to (if not specified, "default" is used) |  |  |
| `vlanNamespace` _string_ | VLANNamespace is the name of the VLANNamespace this VPC belongs to (if not specified, "default" is used) |  |  |
| `defaultIsolated` _boolean_ | DefaultIsolated sets default behavior for isolated mode for the subnets (disabled by default) |  |  |
| `defaultRestricted` _boolean_ | DefaultRestricted sets default behavior for restricted mode for the subnets (disabled by default) |  |  |
| `permit` _string array array_ | Permit defines a list of the access policies between the subnets within the VPC - each policy is a list of subnets that have access to each other.<br />It's applied on top of the subnet isolation flag and if subnet isn't isolated it's not required to have it in a permit list while if vpc is marked<br />as isolated it's required to have it in a permit list to have access to other subnets. |  |  |
| `staticRoutes` _[VPCStaticRoute](#vpcstaticroute) array_ | StaticRoutes is the list of additional static routes for the VPC |  |  |


#### VPCStaticRoute



VPCStaticRoute defines the static route for the VPC



_Appears in:_
- [VPCSpec](#vpcspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `prefix` _string_ | Prefix for the static route (mandatory), e.g. 10.42.0.0/24 |  |  |
| `nextHops` _string array_ | NextHops for the static route (at least one is required), e.g. 10.99.0.0 |  |  |


#### VPCStatus



VPCStatus defines the observed state of VPC



_Appears in:_
- [VPC](#vpc)



#### VPCSubnet



VPCSubnet defines the VPC subnet configuration



_Appears in:_
- [VPCSpec](#vpcspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subnet` _string_ | Subnet is the subnet CIDR block, such as "10.0.0.0/24", should belong to the IPv4Namespace and be unique within the namespace |  |  |
| `gateway` _string_ | Gateway (optional) for the subnet, if not specified, the first IP (e.g. 10.0.0.1) in the subnet is used as the gateway |  |  |
| `dhcp` _[VPCDHCP](#vpcdhcp)_ | DHCP is the on-demand DHCP configuration for the subnet |  |  |
| `vlan` _integer_ | VLAN is the VLAN ID for the subnet, should belong to the VLANNamespace and be unique within the namespace |  |  |
| `isolated` _boolean_ | Isolated is the flag to enable isolated mode for the subnet which means no access to and from the other subnets within the VPC |  |  |
| `restricted` _boolean_ | Restricted is the flag to enable restricted mode for the subnet which means no access between hosts within the subnet itself |  |  |



## wiring.githedgehog.com/v1beta1

Package v1beta1 contains API Schema definitions for the wiring v1beta1 API group. It is public API group mainly for
the underlay definition including Switches, Server, wiring between them and etc. Intended to be used by the user.

### Resource Types
- [Connection](#connection)
- [Server](#server)
- [Switch](#switch)
- [SwitchGroup](#switchgroup)
- [SwitchProfile](#switchprofile)
- [VLANNamespace](#vlannamespace)





#### BasePortName



BasePortName defines the full name of the switch port



_Appears in:_
- [ConnExternalLink](#connexternallink)
- [ConnFabricLinkSwitch](#connfabriclinkswitch)
- [ConnGatewayLinkGateway](#conngatewaylinkgateway)
- [ConnStaticExternalLinkSwitch](#connstaticexternallinkswitch)
- [ServerToSwitchLink](#servertoswitchlink)
- [SwitchToSwitchLink](#switchtoswitchlink)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _string_ | Port defines the full name of the switch port in the format of "device/port", such as "spine-1/E1/1".<br />SONiC port name is used as a port name and switch name should be same as the name of the Switch object. |  |  |


#### ConnBundled



ConnBundled defines the bundled connection (port channel, single server to a single switch with multiple links)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ | Links is the list of server-to-switch links |  |  |
| `mtu` _integer_ | MTU is the MTU to be configured on the switch port or port channel |  |  |


#### ConnESLAG



ConnESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ | Links is the list of server-to-switch links |  | MinItems: 2 <br /> |
| `mtu` _integer_ | MTU is the MTU to be configured on the switch port or port channel |  |  |
| `fallback` _boolean_ | Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link |  |  |


#### ConnExternal



ConnExternal defines the external connection (single switch to a single external device with a single link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `link` _[ConnExternalLink](#connexternallink)_ | Link is the external connection link |  |  |


#### ConnExternalLink



ConnExternalLink defines the external connection link



_Appears in:_
- [ConnExternal](#connexternal)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `switch` _[BasePortName](#baseportname)_ |  |  |  |


#### ConnFabric



ConnFabric defines the fabric connection (single spine to a single leaf with at least one link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[FabricLink](#fabriclink) array_ | Links is the list of spine-to-leaf links |  | MinItems: 1 <br /> |


#### ConnFabricLinkSwitch



ConnFabricLinkSwitch defines the switch side of the fabric (or gateway) link



_Appears in:_
- [FabricLink](#fabriclink)
- [GatewayLink](#gatewaylink)
- [MeshLink](#meshlink)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _string_ | Port defines the full name of the switch port in the format of "device/port", such as "spine-1/E1/1".<br />SONiC port name is used as a port name and switch name should be same as the name of the Switch object. |  |  |
| `ip` _string_ | IP is the IP address of the switch side of the fabric link (switch port configuration) |  | Pattern: `^((25[0-5]\|(2[0-4]\|1\d\|[1-9]\|)\d)\.?\b)\{4\}/([1-2]?[0-9]\|3[0-2])$` <br /> |


#### ConnGateway



ConnGateway defines the gateway connection (single spine to a single gateway with at least one link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[GatewayLink](#gatewaylink) array_ | Links is the list of spine to gateway links |  | MinItems: 1 <br /> |


#### ConnGatewayLinkGateway



ConnGatewayLinkGateway defines the gateway side of the gateway link



_Appears in:_
- [GatewayLink](#gatewaylink)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _string_ | Port defines the full name of the switch port in the format of "device/port", such as "spine-1/E1/1".<br />SONiC port name is used as a port name and switch name should be same as the name of the Switch object. |  |  |
| `ip` _string_ | IP is the IP address of the switch side of the fabric link (switch port configuration) |  | Pattern: `^((25[0-5]\|(2[0-4]\|1\d\|[1-9]\|)\d)\.?\b)\{4\}/([1-2]?[0-9]\|3[0-2])$` <br /> |


#### ConnMCLAG



ConnMCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ | Links is the list of server-to-switch links |  | MinItems: 2 <br /> |
| `mtu` _integer_ | MTU is the MTU to be configured on the switch port or port channel |  |  |
| `fallback` _boolean_ | Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link |  |  |


#### ConnMCLAGDomain



ConnMCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch or
redundancy group and allows to use MCLAG connections to connect servers in a multi-homed way.



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `peerLinks` _[SwitchToSwitchLink](#switchtoswitchlink) array_ | PeerLinks is the list of peer links between the switches, used to pass server traffic between switch |  | MinItems: 1 <br /> |
| `sessionLinks` _[SwitchToSwitchLink](#switchtoswitchlink) array_ | SessionLinks is the list of session links between the switches, used only to pass MCLAG control plane and BGP<br />traffic between switches |  | MinItems: 1 <br /> |


#### ConnMesh



ConnMesh defines the mesh connection (direct leaf to leaf connection with at least one link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[MeshLink](#meshlink) array_ | Links is the list of leaf to leaf links |  | MinItems: 1 <br /> |


#### ConnStaticExternal



ConnStaticExternal defines the static external connection (single switch to a single external device with a single link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `link` _[ConnStaticExternalLink](#connstaticexternallink)_ | Link is the static external connection link |  |  |
| `withinVPC` _string_ | WithinVPC is the optional VPC name to provision the static external connection within the VPC VRF instead of default one to make resource available to the specific VPC |  |  |


#### ConnStaticExternalLink



ConnStaticExternalLink defines the static external connection link



_Appears in:_
- [ConnStaticExternal](#connstaticexternal)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `switch` _[ConnStaticExternalLinkSwitch](#connstaticexternallinkswitch)_ | Switch is the switch side of the static external connection link |  |  |


#### ConnStaticExternalLinkSwitch



ConnStaticExternalLinkSwitch defines the switch side of the static external connection link



_Appears in:_
- [ConnStaticExternalLink](#connstaticexternallink)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _string_ | Port defines the full name of the switch port in the format of "device/port", such as "spine-1/E1/1".<br />SONiC port name is used as a port name and switch name should be same as the name of the Switch object. |  |  |
| `ip` _string_ | IP is the IP address of the switch side of the static external connection link (switch port configuration) |  | Pattern: `^((25[0-5]\|(2[0-4]\|1\d\|[1-9]\|)\d)\.?\b)\{4\}/([1-2]?[0-9]\|3[0-2])$` <br /> |
| `nextHop` _string_ | NextHop is the next hop IP address for static routes that will be created for the subnets |  | Pattern: `^((25[0-5]\|(2[0-4]\|1\d\|[1-9]\|)\d)\.?\b)\{4\}$` <br /> |
| `subnets` _string array_ | Subnets is the list of subnets that will get static routes using the specified next hop |  |  |
| `vlan` _integer_ | VLAN is the optional VLAN ID to be configured on the switch port |  |  |


#### ConnUnbundled



ConnUnbundled defines the unbundled connection (no port channel, single server to a single switch with a single link)



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `link` _[ServerToSwitchLink](#servertoswitchlink)_ | Link is the server-to-switch link |  |  |
| `mtu` _integer_ | MTU is the MTU to be configured on the switch port or port channel |  |  |


#### ConnVPCLoopback



ConnVPCLoopback defines the VPC loopback connection (multiple port pairs on a single switch) that enables automated
workaround named "VPC Loopback" that allow to avoid switch hardware limitations and traffic going through CPU in some
cases



_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `links` _[SwitchToSwitchLink](#switchtoswitchlink) array_ | Links is the list of VPC loopback links |  | MinItems: 1 <br /> |


#### Connection



Connection object represents a logical and physical connections between any devices in the Fabric (Switch, Server
and External objects). It's needed to define all physical and logical connections between the devices in the Wiring
Diagram. Connection type is defined by the top-level field in the ConnectionSpec. Exactly one of them could be used
in a single Connection object.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `Connection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ConnectionSpec](#connectionspec)_ | Spec is the desired state of the Connection |  |  |
| `status` _[ConnectionStatus](#connectionstatus)_ | Status is the observed state of the Connection |  |  |


#### ConnectionSpec



ConnectionSpec defines the desired state of Connection



_Appears in:_
- [Connection](#connection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unbundled` _[ConnUnbundled](#connunbundled)_ | Unbundled defines the unbundled connection (no port channel, single server to a single switch with a single link) |  |  |
| `bundled` _[ConnBundled](#connbundled)_ | Bundled defines the bundled connection (port channel, single server to a single switch with multiple links) |  |  |
| `mclag` _[ConnMCLAG](#connmclag)_ | MCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links) |  |  |
| `eslag` _[ConnESLAG](#conneslag)_ | ESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links) |  |  |
| `mclagDomain` _[ConnMCLAGDomain](#connmclagdomain)_ | MCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch for server multi-homing |  |  |
| `fabric` _[ConnFabric](#connfabric)_ | Fabric defines the fabric connection (single spine to a single leaf with at least one link) |  |  |
| `mesh` _[ConnMesh](#connmesh)_ | Mesh defines the mesh connection (direct leaf to leaf connection with at least one link) |  |  |
| `gateway` _[ConnGateway](#conngateway)_ | Gateway defines the gateway connection (single spine to a single gateway with at least one link) |  |  |
| `vpcLoopback` _[ConnVPCLoopback](#connvpcloopback)_ | VPCLoopback defines the VPC loopback connection (multiple port pairs on a single switch) for automated workaround |  |  |
| `external` _[ConnExternal](#connexternal)_ | External defines the external connection (single switch to a single external device with a single link) |  |  |
| `staticExternal` _[ConnStaticExternal](#connstaticexternal)_ | StaticExternal defines the static external connection (single switch to a single external device with a single link) |  |  |


#### ConnectionStatus



ConnectionStatus defines the observed state of Connection



_Appears in:_
- [Connection](#connection)



#### FabricLink



FabricLink defines the fabric connection link



_Appears in:_
- [ConnFabric](#connfabric)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `spine` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ | Spine is the spine side of the fabric link |  |  |
| `leaf` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ | Leaf is the leaf side of the fabric link |  |  |


#### GatewayLink



GatewayLink defines the gateway connection link



_Appears in:_
- [ConnGateway](#conngateway)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `switch` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ | Switch is the switch (spine or leaf) side of the gateway link |  |  |
| `gateway` _[ConnGatewayLinkGateway](#conngatewaylinkgateway)_ | Gateway is the gateway side of the gateway link |  |  |




#### MeshLink



MeshLink defines the mesh connection link, i.e. a direct leaf to leaf connection



_Appears in:_
- [ConnMesh](#connmesh)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `leaf1` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ |  |  |  |
| `leaf2` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ |  |  |  |


#### Server



Server is the Schema for the servers API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `Server` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ServerSpec](#serverspec)_ | Spec is desired state of the server |  |  |
| `status` _[ServerStatus](#serverstatus)_ | Status is the observed state of the server |  |  |


#### ServerFacingConnectionConfig



ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration



_Appears in:_
- [ConnBundled](#connbundled)
- [ConnESLAG](#conneslag)
- [ConnMCLAG](#connmclag)
- [ConnUnbundled](#connunbundled)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `mtu` _integer_ | MTU is the MTU to be configured on the switch port or port channel |  |  |


#### ServerSpec



ServerSpec defines the desired state of Server



_Appears in:_
- [Server](#server)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | Description is a description of the server |  |  |
| `profile` _string_ | Profile is the profile of the server, name of the ServerProfile object to be used for this server, currently not used by the Fabric |  |  |


#### ServerStatus



ServerStatus defines the observed state of Server



_Appears in:_
- [Server](#server)



#### ServerToSwitchLink



ServerToSwitchLink defines the server-to-switch link



_Appears in:_
- [ConnBundled](#connbundled)
- [ConnESLAG](#conneslag)
- [ConnMCLAG](#connmclag)
- [ConnUnbundled](#connunbundled)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `server` _[BasePortName](#baseportname)_ | Server is the server side of the connection |  |  |
| `switch` _[BasePortName](#baseportname)_ | Switch is the switch side of the connection |  |  |


#### Switch



Switch is the Schema for the switches API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `Switch` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SwitchSpec](#switchspec)_ | Spec is desired state of the switch |  |  |
| `status` _[SwitchStatus](#switchstatus)_ | Status is the observed state of the switch |  |  |


#### SwitchBoot







_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serial` _string_ | Identify switch by serial number |  |  |
| `mac` _string_ | Identify switch by MAC address of the management port |  |  |


#### SwitchECMP



SwitchECMP is a struct that defines the ECMP configuration for the switch



_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `roceQPN` _boolean_ | RoCEQPN is a flag to enable RoCE QPN hashing |  |  |


#### SwitchGroup



SwitchGroup is the marker API object to group switches together, switch can belong to multiple groups





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `SwitchGroup` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SwitchGroupSpec](#switchgroupspec)_ | Spec is the desired state of the SwitchGroup |  |  |
| `status` _[SwitchGroupStatus](#switchgroupstatus)_ | Status is the observed state of the SwitchGroup |  |  |


#### SwitchGroupSpec



SwitchGroupSpec defines the desired state of SwitchGroup



_Appears in:_
- [SwitchGroup](#switchgroup)



#### SwitchGroupStatus



SwitchGroupStatus defines the observed state of SwitchGroup



_Appears in:_
- [SwitchGroup](#switchgroup)



#### SwitchProfile



SwitchProfile represents switch capabilities and configuration





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `SwitchProfile` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SwitchProfileSpec](#switchprofilespec)_ |  |  |  |
| `status` _[SwitchProfileStatus](#switchprofilestatus)_ |  |  |  |


#### SwitchProfileConfig



Defines switch-specific configuration options



_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxPathsEBGP` _integer_ | MaxPathsIBGP defines the maximum number of IBGP paths to be configured |  |  |


#### SwitchProfileFeatures



Defines features supported by a specific switch which is later used for roles and Fabric API features usage validation



_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subinterfaces` _boolean_ | Subinterfaces defines if switch supports subinterfaces |  |  |
| `acls` _boolean_ | ACLs defines if switch supports ACLs |  |  |
| `l2vni` _boolean_ | L2VNI defines if switch supports L2 VNIs |  |  |
| `l3vni` _boolean_ | L3VNI defines if switch supports L3 VNIs |  |  |
| `roce` _boolean_ | RoCE defines if switch supports RoCEv2 over VXLAN and related features used by the fabric |  |  |
| `mclag` _boolean_ | MCLAG defines if switch supports MCLAG (with VXLAN) |  |  |
| `eslag` _boolean_ | ESLAG defines if switch supports ESLAG (ESI multi-homing) |  |  |
| `ecmpRoCEQPN` _boolean_ | ECMPRoCEQPN defines if switch supports ECMP QPN hashing |  |  |
| `vxlan` _boolean_ | Deprecated: use L2VNI and L3VNI instead as more precise features |  |  |


#### SwitchProfilePort



Defines a switch port configuration
Only one of Profile or Group can be set



_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nos` _string_ | NOSName defines how port is named in the NOS |  |  |
| `baseNOSName` _string_ | BaseNOSName defines the base NOS name that could be used together with the profile to generate the actual NOS name (e.g. breakouts) |  |  |
| `label` _string_ | Label defines the physical port label you can see on the actual switch |  |  |
| `group` _string_ | If port isn't directly manageable, group defines the group it belongs to, exclusive with profile |  |  |
| `profile` _string_ | If port is directly configurable, profile defines the profile it belongs to, exclusive with group |  |  |
| `management` _boolean_ | Management defines if port is a management port, it's a special case and it can't have a group or profile |  |  |
| `oniePortName` _string_ | OniePortName defines the ONIE port name for management ports only |  |  |


#### SwitchProfilePortGroup



Defines a switch port group configuration



_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nos` _string_ | NOSName defines how group is named in the NOS |  |  |
| `profile` _string_ | Profile defines the possible configuration profile for the group, could only have speed profile |  |  |


#### SwitchProfilePortProfile



Defines a switch port profile configuration



_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `speed` _[SwitchProfilePortProfileSpeed](#switchprofileportprofilespeed)_ | Speed defines the speed configuration for the profile, exclusive with breakout |  |  |
| `breakout` _[SwitchProfilePortProfileBreakout](#switchprofileportprofilebreakout)_ | Breakout defines the breakout configuration for the profile, exclusive with speed |  |  |
| `autoNegAllowed` _boolean_ | AutoNegAllowed defines if configuring auto-negotiation is allowed for the port |  |  |
| `autoNegDefault` _boolean_ | AutoNegDefault defines the default auto-negotiation state for the port |  |  |


#### SwitchProfilePortProfileBreakout



Defines a switch port profile breakout configuration



_Appears in:_
- [SwitchProfilePortProfile](#switchprofileportprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `default` _string_ | Default defines the default breakout mode for the profile |  |  |
| `supported` _object (keys:string, values:[SwitchProfilePortProfileBreakoutMode](#switchprofileportprofilebreakoutmode))_ | Supported defines the supported breakout modes for the profile with the NOS name offsets |  |  |


#### SwitchProfilePortProfileBreakoutMode



Defines a switch port profile breakout mode configuration



_Appears in:_
- [SwitchProfilePortProfileBreakout](#switchprofileportprofilebreakout)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `offsets` _string array_ | Offsets defines the breakout NOS port name offset from the port NOS Name for each breakout mode |  |  |


#### SwitchProfilePortProfileSpeed



Defines a switch port profile speed configuration



_Appears in:_
- [SwitchProfilePortProfile](#switchprofileportprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `default` _string_ | Default defines the default speed for the profile |  |  |
| `supported` _string array_ | Supported defines the supported speeds for the profile |  |  |




#### SwitchProfileSpec



SwitchProfileSpec defines the desired state of SwitchProfile



_Appears in:_
- [SwitchProfile](#switchprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `displayName` _string_ | DisplayName defines the human-readable name of the switch |  |  |
| `otherNames` _string array_ | OtherNames defines alternative names for the switch |  |  |
| `switchSilicon` _string_ | SwitchSilicon defines the switch silicon name |  |  |
| `features` _[SwitchProfileFeatures](#switchprofilefeatures)_ | Features defines the features supported by the switch |  |  |
| `notes` _string_ | Notes is a free-form field for any additional information about the switch |  |  |
| `config` _[SwitchProfileConfig](#switchprofileconfig)_ | Config defines the switch-specific configuration options |  |  |
| `ports` _object (keys:string, values:[SwitchProfilePort](#switchprofileport))_ | Ports defines the switch port configuration |  |  |
| `portGroups` _object (keys:string, values:[SwitchProfilePortGroup](#switchprofileportgroup))_ | PortGroups defines the switch port group configuration |  |  |
| `portProfiles` _object (keys:string, values:[SwitchProfilePortProfile](#switchprofileportprofile))_ | PortProfiles defines the switch port profile configuration |  |  |
| `nosType` _[NOSType](#nostype)_ | NOSType defines the NOS type to be used for the switch |  |  |
| `platform` _string_ | Platform is what expected to be request by ONIE and displayed in the NOS |  |  |


#### SwitchProfileStatus



SwitchProfileStatus defines the observed state of SwitchProfile



_Appears in:_
- [SwitchProfile](#switchprofile)



#### SwitchRedundancy



SwitchRedundancy is the switch redundancy configuration which includes name of the redundancy group switch belongs
to and its type, used both for MCLAG and ESLAG connections. It defines how redundancy will be configured and handled
on the switch as well as which connection types will be available. If not specified, switch will not be part of any
redundancy group. If name isn't empty, type must be specified as well and name should be the same as one of the
SwitchGroup objects.



_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `group` _string_ | Group is the name of the redundancy group switch belongs to |  |  |
| `type` _[RedundancyType](#redundancytype)_ | Type is the type of the redundancy group, could be mclag or eslag |  |  |


#### SwitchRole

_Underlying type:_ _string_

SwitchRole is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf

_Validation:_
- Enum: [spine server-leaf border-leaf mixed-leaf virtual-edge]

_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `spine` |  |
| `server-leaf` |  |
| `border-leaf` |  |
| `mixed-leaf` |  |


#### SwitchSpec



SwitchSpec defines the desired state of Switch



_Appears in:_
- [Switch](#switch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `role` _[SwitchRole](#switchrole)_ | Role is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf |  | Enum: [spine server-leaf border-leaf mixed-leaf virtual-edge] <br />Required: \{\} <br /> |
| `description` _string_ | Description is a description of the switch |  |  |
| `profile` _string_ | Profile is the profile of the switch, name of the SwitchProfile object to be used for this switch, currently not used by the Fabric |  |  |
| `groups` _string array_ | Groups is a list of switch groups the switch belongs to |  |  |
| `redundancy` _[SwitchRedundancy](#switchredundancy)_ | Redundancy is the switch redundancy configuration including name of the redundancy group switch belongs to and its type, used both for MCLAG and ESLAG connections |  |  |
| `vlanNamespaces` _string array_ | VLANNamespaces is a list of VLAN namespaces the switch is part of, their VLAN ranges could not overlap |  |  |
| `asn` _integer_ | ASN is the ASN of the switch |  |  |
| `ip` _string_ | IP is the IP of the switch that could be used to access it from other switches and control nodes in the Fabric |  |  |
| `vtepIP` _string_ | VTEPIP is the VTEP IP of the switch |  |  |
| `protocolIP` _string_ | ProtocolIP is used as BGP Router ID for switch configuration |  |  |
| `portGroupSpeeds` _object (keys:string, values:string)_ | PortGroupSpeeds is a map of port group speeds, key is the port group name, value is the speed, such as '"2": 10G' |  |  |
| `portSpeeds` _object (keys:string, values:string)_ | PortSpeeds is a map of port speeds, key is the port name, value is the speed |  |  |
| `portBreakouts` _object (keys:string, values:string)_ | PortBreakouts is a map of port breakouts, key is the port name, value is the breakout configuration, such as "1/55: 4x25G" |  |  |
| `portAutoNegs` _object (keys:string, values:boolean)_ | PortAutoNegs is a map of port auto negotiation, key is the port name, value is true or false |  |  |
| `boot` _[SwitchBoot](#switchboot)_ | Boot is the boot/provisioning information of the switch |  |  |
| `enableAllPorts` _boolean_ | EnableAllPorts is a flag to enable all ports on the switch regardless of them being used or not |  |  |
| `roce` _boolean_ | RoCE is a flag to enable RoCEv2 support on the switch which includes lossless queues and QoS configuration |  |  |
| `ecmp` _[SwitchECMP](#switchecmp)_ | ECMP is the ECMP configuration for the switch |  |  |


#### SwitchStatus



SwitchStatus defines the observed state of Switch



_Appears in:_
- [Switch](#switch)



#### SwitchToSwitchLink



SwitchToSwitchLink defines the switch-to-switch link



_Appears in:_
- [ConnMCLAGDomain](#connmclagdomain)
- [ConnVPCLoopback](#connvpcloopback)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `switch1` _[BasePortName](#baseportname)_ | Switch1 is the first switch side of the connection |  |  |
| `switch2` _[BasePortName](#baseportname)_ | Switch2 is the second switch side of the connection |  |  |


#### VLANNamespace



VLANNamespace is the Schema for the vlannamespaces API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1beta1` | | |
| `kind` _string_ | `VLANNamespace` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[VLANNamespaceSpec](#vlannamespacespec)_ | Spec is the desired state of the VLANNamespace |  |  |
| `status` _[VLANNamespaceStatus](#vlannamespacestatus)_ | Status is the observed state of the VLANNamespace |  |  |


#### VLANNamespaceSpec



VLANNamespaceSpec defines the desired state of VLANNamespace



_Appears in:_
- [VLANNamespace](#vlannamespace)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ranges` _VLANRange array_ | Ranges is a list of VLAN ranges to be used in this namespace, couldn't overlap between each other and with Fabric reserved VLAN ranges |  | MaxItems: 20 <br />MinItems: 1 <br /> |


#### VLANNamespaceStatus



VLANNamespaceStatus defines the observed state of VLANNamespace



_Appears in:_
- [VLANNamespace](#vlannamespace)



