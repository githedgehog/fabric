# API Reference

## Packages
- [agent.githedgehog.com/v1alpha2](#agentgithedgehogcomv1alpha2)
- [vpc.githedgehog.com/v1alpha2](#vpcgithedgehogcomv1alpha2)
- [wiring.githedgehog.com/v1alpha2](#wiringgithedgehogcomv1alpha2)


## agent.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the agent v1alpha2 API group

### Resource Types
- [Agent](#agent)
- [ControlAgent](#controlagent)



#### Agent



Agent is the Schema for the agents API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `agent.githedgehog.com/v1alpha2`
| `kind` _string_ | `Agent`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[AgentSpec](#agentspec)_ |  |
| `status` _[AgentStatus](#agentstatus)_ |  |


#### AgentSpec



AgentSpec defines the desired state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `role` _SwitchRole_ |  |
| `description` _string_ |  |
| `config` _[AgentSpecConfig](#agentspecconfig)_ |  |
| `version` _[AgentVersion](#agentversion)_ |  |
| `users` _[UserCreds](#usercreds) array_ |  |
| `switch` _[SwitchSpec](#switchspec)_ |  |
| `switches` _object (keys:string, values:[SwitchSpec](#switchspec))_ |  |
| `connections` _object (keys:string, values:[ConnectionSpec](#connectionspec))_ |  |
| `vpcs` _object (keys:string, values:[VPCSpec](#vpcspec))_ |  |
| `vpcAttachments` _object (keys:string, values:[VPCAttachmentSpec](#vpcattachmentspec))_ |  |
| `vpcPeers` _object (keys:string, values:[VPCPeeringSpec](#vpcpeeringspec))_ |  |
| `vpcLoopbackLinks` _object (keys:string, values:string)_ |  |
| `vpcLoopbackVLANs` _object (keys:string, values:integer)_ |  |
| `vnis` _object (keys:string, values:integer)_ |  |
| `irbVLANs` _object (keys:string, values:integer)_ |  |
| `portChannels` _object (keys:string, values:integer)_ |  |
| `reinstall` _string_ |  |
| `reboot` _string_ |  |
| `statusUpdates` _[ApplyStatusUpdate](#applystatusupdate) array_ | TODO impl |


#### AgentSpecConfig





_Appears in:_
- [AgentSpec](#agentspec)

| Field | Description |
| --- | --- |
| `controlVIP` _string_ |  |
| `vpcPeeringDisabled` _boolean_ |  |
| `collapsedCore` _[AgentSpecConfigCollapsedCore](#agentspecconfigcollapsedcore)_ |  |
| `spineLeaf` _[AgentSpecConfigSpineLeaf](#agentspecconfigspineleaf)_ |  |


#### AgentSpecConfigCollapsedCore





_Appears in:_
- [AgentSpecConfig](#agentspecconfig)



#### AgentSpecConfigSpineLeaf





_Appears in:_
- [AgentSpecConfig](#agentspecconfig)



#### AgentStatus



AgentStatus defines the observed state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `version` _string_ |  |
| `installID` _string_ |  |
| `runID` _string_ |  |
| `lastHeartbeat` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAttemptGen` _integer_ |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAppliedGen` _integer_ |  |
| `nosInfo` _[NOSInfo](#nosinfo)_ |  |
| `statusUpdates` _[ApplyStatusUpdate](#applystatusupdate) array_ |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#condition-v1-meta) array_ |  |


#### AgentVersion





_Appears in:_
- [AgentSpec](#agentspec)
- [ControlAgentSpec](#controlagentspec)

| Field | Description |
| --- | --- |
| `default` _string_ |  |
| `override` _string_ |  |
| `repo` _string_ |  |
| `ca` _string_ |  |


#### ApplyStatusUpdate





_Appears in:_
- [AgentSpec](#agentspec)
- [AgentStatus](#agentstatus)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ |  |
| `kind` _string_ |  |
| `name` _string_ |  |
| `namespace` _string_ |  |
| `generation` _integer_ |  |


#### ControlAgent



ControlAgent is the Schema for the controlagents API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `agent.githedgehog.com/v1alpha2`
| `kind` _string_ | `ControlAgent`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ControlAgentSpec](#controlagentspec)_ |  |
| `status` _[ControlAgentStatus](#controlagentstatus)_ |  |


#### ControlAgentSpec



ControlAgentSpec defines the desired state of ControlAgent

_Appears in:_
- [ControlAgent](#controlagent)

| Field | Description |
| --- | --- |
| `controlVIP` _string_ |  |
| `version` _[AgentVersion](#agentversion)_ |  |
| `networkd` _object (keys:string, values:string)_ |  |
| `hosts` _object (keys:string, values:string)_ |  |


#### ControlAgentStatus



ControlAgentStatus defines the observed state of ControlAgent

_Appears in:_
- [ControlAgent](#controlagent)

| Field | Description |
| --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#condition-v1-meta) array_ |  |
| `version` _string_ |  |
| `lastHeartbeat` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAttemptTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAttemptGen` _integer_ |  |
| `lastAppliedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |
| `lastAppliedGen` _integer_ |  |


#### NOSInfo





_Appears in:_
- [AgentStatus](#agentstatus)

| Field | Description |
| --- | --- |
| `asicVersion` _string_ |  |
| `buildCommit` _string_ |  |
| `buildDate` _string_ |  |
| `builtBy` _string_ |  |
| `configDbVersion` _string_ |  |
| `distributionVersion` _string_ |  |
| `hardwareVersion` _string_ |  |
| `hwskuVersion` _string_ |  |
| `kernelVersion` _string_ |  |
| `mfgName` _string_ |  |
| `platformName` _string_ |  |
| `productDescription` _string_ |  |
| `productVersion` _string_ |  |
| `serialNumber` _string_ |  |
| `softwareVersion` _string_ |  |
| `upTime` _string_ |  |


#### UserCreds





_Appears in:_
- [AgentSpec](#agentspec)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `password` _string_ |  |
| `role` _string_ |  |
| `sshKeys` _string array_ |  |



## vpc.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the vpc v1alpha2 API group

### Resource Types
- [IPv4Namespace](#ipv4namespace)
- [VPC](#vpc)
- [VPCAttachment](#vpcattachment)
- [VPCPeering](#vpcpeering)



#### IPv4Namespace



IPv4Namespace is the Schema for the ipv4namespaces API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `IPv4Namespace`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[IPv4NamespaceSpec](#ipv4namespacespec)_ |  |
| `status` _[IPv4NamespaceStatus](#ipv4namespacestatus)_ |  |


#### IPv4NamespaceSpec



IPv4NamespaceSpec defines the desired state of IPv4Namespace

_Appears in:_
- [IPv4Namespace](#ipv4namespace)

| Field | Description |
| --- | --- |
| `subnets` _string array_ |  |




#### VPC



VPC is the Schema for the vpcs API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `VPC`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VPCSpec](#vpcspec)_ |  |
| `status` _[VPCStatus](#vpcstatus)_ |  |


#### VPCAttachment



VPCAttachment is the Schema for the vpcattachments API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `VPCAttachment`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VPCAttachmentSpec](#vpcattachmentspec)_ |  |
| `status` _[VPCAttachmentStatus](#vpcattachmentstatus)_ |  |


#### VPCAttachmentSpec



VPCAttachmentSpec defines the desired state of VPCAttachment

_Appears in:_
- [AgentSpec](#agentspec)
- [VPCAttachment](#vpcattachment)

| Field | Description |
| --- | --- |
| `subnet` _string_ |  |
| `connection` _string_ |  |




#### VPCDHCP





_Appears in:_
- [VPCSubnet](#vpcsubnet)

| Field | Description |
| --- | --- |
| `enable` _boolean_ |  |
| `range` _[VPCDHCPRange](#vpcdhcprange)_ |  |


#### VPCDHCPRange





_Appears in:_
- [VPCDHCP](#vpcdhcp)

| Field | Description |
| --- | --- |
| `start` _string_ |  |
| `end` _string_ |  |




#### VPCPeering



VPCPeering is the Schema for the vpcpeerings API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `VPCPeering`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VPCPeeringSpec](#vpcpeeringspec)_ |  |
| `status` _[VPCPeeringStatus](#vpcpeeringstatus)_ |  |


#### VPCPeeringSpec



VPCPeeringSpec defines the desired state of VPCPeering

_Appears in:_
- [AgentSpec](#agentspec)
- [VPCPeering](#vpcpeering)

| Field | Description |
| --- | --- |
| `permit` _[map[string]VPCPeer](#map[string]vpcpeer) array_ |  |




#### VPCSpec



VPCSpec defines the desired state of VPC

_Appears in:_
- [AgentSpec](#agentspec)
- [VPC](#vpc)

| Field | Description |
| --- | --- |
| `subnets` _object (keys:string, values:[VPCSubnet](#vpcsubnet))_ |  |
| `ipv4Namespace` _string_ |  |
| `vlanNamespace` _string_ |  |


#### VPCStatus



VPCStatus defines the observed state of VPC

_Appears in:_
- [VPC](#vpc)

| Field | Description |
| --- | --- |
| `vni` _integer_ |  |
| `subnetVNIs` _object (keys:string, values:integer)_ |  |


#### VPCSubnet





_Appears in:_
- [VPCSpec](#vpcspec)

| Field | Description |
| --- | --- |
| `subnet` _string_ |  |
| `dhcp` _[VPCDHCP](#vpcdhcp)_ |  |
| `vlan` _string_ |  |



## wiring.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the wiring v1alpha2 API group

### Resource Types
- [Connection](#connection)
- [Rack](#rack)
- [Server](#server)
- [ServerProfile](#serverprofile)
- [Switch](#switch)
- [SwitchGroup](#switchgroup)
- [SwitchProfile](#switchprofile)
- [VLANNamespace](#vlannamespace)





#### BasePortName





_Appears in:_
- [ConnFabricLinkSwitch](#connfabriclinkswitch)
- [ConnMgmtLinkServer](#connmgmtlinkserver)
- [ConnMgmtLinkSwitch](#connmgmtlinkswitch)
- [ConnNATLink](#connnatlink)
- [ConnNATLinkSwitch](#connnatlinkswitch)
- [ServerToSwitchLink](#servertoswitchlink)
- [SwitchToSwitchLink](#switchtoswitchlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |


#### ConnBundled





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ |  |


#### ConnFabric





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[FabricLink](#fabriclink) array_ |  |


#### ConnFabricLinkSwitch





_Appears in:_
- [FabricLink](#fabriclink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `ip` _string_ |  |


#### ConnMCLAG





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ |  |
| `mtu` _integer_ |  |


#### ConnMCLAGDomain





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `peerLinks` _[SwitchToSwitchLink](#switchtoswitchlink) array_ |  |
| `sessionLinks` _[SwitchToSwitchLink](#switchtoswitchlink) array_ |  |


#### ConnMgmt





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ConnMgmtLink](#connmgmtlink)_ |  |


#### ConnMgmtLink





_Appears in:_
- [ConnMgmt](#connmgmt)

| Field | Description |
| --- | --- |
| `server` _[ConnMgmtLinkServer](#connmgmtlinkserver)_ |  |
| `switch` _[ConnMgmtLinkSwitch](#connmgmtlinkswitch)_ |  |


#### ConnMgmtLinkServer





_Appears in:_
- [ConnMgmtLink](#connmgmtlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `ip` _string_ |  |
| `mac` _string_ |  |


#### ConnMgmtLinkSwitch





_Appears in:_
- [ConnMgmtLink](#connmgmtlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `ip` _string_ |  |
| `oniePortName` _string_ |  |


#### ConnNAT





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ConnNATLink](#connnatlink)_ |  |


#### ConnNATLink





_Appears in:_
- [ConnNAT](#connnat)

| Field | Description |
| --- | --- |
| `switch` _[ConnNATLinkSwitch](#connnatlinkswitch)_ |  |
| `nat` _[BasePortName](#baseportname)_ |  |


#### ConnNATLinkSwitch





_Appears in:_
- [ConnNATLink](#connnatlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `ip` _string_ |  |
| `neighborIP` _string_ |  |
| `remoteAS` _integer_ |  |
| `snat` _[SNAT](#snat)_ |  |


#### ConnUnbundled





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ServerToSwitchLink](#servertoswitchlink)_ |  |


#### ConnVPCLoopback





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[SwitchToSwitchLink](#switchtoswitchlink) array_ |  |


#### Connection



Connection is the Schema for the connections API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Connection`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ConnectionSpec](#connectionspec)_ |  |
| `status` _[ConnectionStatus](#connectionstatus)_ |  |


#### ConnectionSpec



ConnectionSpec defines the desired state of Connection

_Appears in:_
- [AgentSpec](#agentspec)
- [Connection](#connection)

| Field | Description |
| --- | --- |
| `unbundled` _[ConnUnbundled](#connunbundled)_ |  |
| `bundled` _[ConnBundled](#connbundled)_ |  |
| `management` _[ConnMgmt](#connmgmt)_ |  |
| `mclag` _[ConnMCLAG](#connmclag)_ |  |
| `mclagDomain` _[ConnMCLAGDomain](#connmclagdomain)_ |  |
| `nat` _[ConnNAT](#connnat)_ |  |
| `fabric` _[ConnFabric](#connfabric)_ |  |
| `vpcLoopback` _[ConnVPCLoopback](#connvpcloopback)_ |  |




#### FabricLink





_Appears in:_
- [ConnFabric](#connfabric)

| Field | Description |
| --- | --- |
| `spine` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ |  |
| `leaf` _[ConnFabricLinkSwitch](#connfabriclinkswitch)_ |  |




#### Location



Location defines the geopraphical position of the device in a datacenter

_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `location` _string_ |  |
| `aisle` _string_ |  |
| `row` _string_ |  |
| `rack` _string_ |  |
| `slot` _string_ |  |


#### LocationSig



LocationSig contains signatures for the location UUID as well as the device location itself

_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `sig` _string_ |  |
| `uuidSig` _string_ |  |


#### Rack



Rack is the Schema for the racks API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Rack`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[RackSpec](#rackspec)_ |  |
| `status` _[RackStatus](#rackstatus)_ |  |


#### RackPosition



RackPosition defines the geopraphical position of the rack in a datacenter

_Appears in:_
- [RackSpec](#rackspec)

| Field | Description |
| --- | --- |
| `location` _string_ |  |
| `aisle` _string_ |  |
| `row` _string_ |  |


#### RackSpec



RackSpec defines the properties of a rack which we are modelling

_Appears in:_
- [Rack](#rack)

| Field | Description |
| --- | --- |
| `numServers` _integer_ |  |
| `hasControlNode` _boolean_ |  |
| `hasConsoleServer` _boolean_ |  |
| `position` _[RackPosition](#rackposition)_ |  |




#### SNAT





_Appears in:_
- [ConnNATLinkSwitch](#connnatlinkswitch)

| Field | Description |
| --- | --- |
| `pool` _string array_ |  |


#### Server



Server is the Schema for the servers API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Server`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ServerSpec](#serverspec)_ |  |
| `status` _[ServerStatus](#serverstatus)_ |  |


#### ServerProfile



ServerProfile is the Schema for the serverprofiles API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `ServerProfile`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ServerProfileSpec](#serverprofilespec)_ |  |
| `status` _[ServerProfileStatus](#serverprofilestatus)_ |  |


#### ServerProfileNIC





_Appears in:_
- [ServerProfileSpec](#serverprofilespec)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `ports` _[ServerProfileNICPort](#serverprofilenicport) array_ |  |


#### ServerProfileNICPort





_Appears in:_
- [ServerProfileNIC](#serverprofilenic)

| Field | Description |
| --- | --- |
| `name` _string_ |  |


#### ServerProfileSpec



ServerProfileSpec defines the desired state of ServerProfile

_Appears in:_
- [ServerProfile](#serverprofile)

| Field | Description |
| --- | --- |
| `nics` _[ServerProfileNIC](#serverprofilenic) array_ |  |




#### ServerSpec



ServerSpec defines the desired state of Server

_Appears in:_
- [Server](#server)

| Field | Description |
| --- | --- |
| `type` _ServerType_ |  |
| `description` _string_ |  |
| `profile` _string_ |  |




#### ServerToSwitchLink





_Appears in:_
- [ConnBundled](#connbundled)
- [ConnMCLAG](#connmclag)
- [ConnUnbundled](#connunbundled)

| Field | Description |
| --- | --- |
| `server` _[BasePortName](#baseportname)_ |  |
| `switch` _[BasePortName](#baseportname)_ |  |


#### Switch



Switch is the Schema for the switches API 
 All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents name of the rack it belongs to.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Switch`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[SwitchSpec](#switchspec)_ |  |
| `status` _[SwitchStatus](#switchstatus)_ |  |


#### SwitchGroup



SwitchGroup is the Schema for the switchgroups API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `SwitchGroup`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[SwitchGroupSpec](#switchgroupspec)_ |  |
| `status` _[SwitchGroupStatus](#switchgroupstatus)_ |  |






#### SwitchProfile



SwitchProfile is the Schema for the switchprofiles API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `SwitchProfile`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[SwitchProfileSpec](#switchprofilespec)_ |  |
| `status` _[SwitchProfileStatus](#switchprofilestatus)_ |  |


#### SwitchProfileLimits





_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description |
| --- | --- |
| `vpc` _integer_ |  |
| `policy` _integer_ |  |


#### SwitchProfilePort





_Appears in:_
- [SwitchProfileSpec](#switchprofilespec)

| Field | Description |
| --- | --- |
| `id` _integer_ |  |
| `name` _string_ |  |
| `management` _boolean_ |  |


#### SwitchProfileSpec



SwitchProfileSpec defines the desired state of SwitchProfile

_Appears in:_
- [SwitchProfile](#switchprofile)

| Field | Description |
| --- | --- |
| `limits` _[SwitchProfileLimits](#switchprofilelimits)_ |  |
| `ports` _[SwitchProfilePort](#switchprofileport) array_ |  |




#### SwitchSpec



SwitchSpec defines the desired state of Switch

_Appears in:_
- [AgentSpec](#agentspec)
- [Switch](#switch)

| Field | Description |
| --- | --- |
| `role` _SwitchRole_ |  |
| `description` _string_ |  |
| `profile` _string_ |  |
| `location` _[Location](#location)_ |  |
| `locationSig` _[LocationSig](#locationsig)_ |  |
| `groups` _string array_ |  |
| `vlanNamespaces` _string array_ |  |
| `asn` _integer_ |  |
| `ip` _string_ |  |
| `vtepIP` _string_ |  |
| `protocolIP` _string_ |  |
| `portGroupSpeeds` _object (keys:string, values:string)_ |  |
| `portSpeeds` _object (keys:string, values:string)_ |  |
| `portBreakouts` _object (keys:string, values:string)_ |  |




#### SwitchToSwitchLink





_Appears in:_
- [ConnMCLAGDomain](#connmclagdomain)
- [ConnVPCLoopback](#connvpcloopback)

| Field | Description |
| --- | --- |
| `switch1` _[BasePortName](#baseportname)_ |  |
| `switch2` _[BasePortName](#baseportname)_ |  |


#### VLANNamespace



VLANNamespace is the Schema for the vlannamespaces API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `VLANNamespace`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VLANNamespaceSpec](#vlannamespacespec)_ |  |
| `status` _[VLANNamespaceStatus](#vlannamespacestatus)_ |  |


#### VLANNamespaceSpec



VLANNamespaceSpec defines the desired state of VLANNamespace

_Appears in:_
- [VLANNamespace](#vlannamespace)

| Field | Description |
| --- | --- |
| `ranges` _[VLANRange](#vlanrange) array_ |  |




