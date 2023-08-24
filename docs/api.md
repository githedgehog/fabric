# API Reference

## Packages
- [agent.githedgehog.com/v1alpha2](#agentgithedgehogcomv1alpha2)
- [wiring.githedgehog.com/v1alpha2](#wiringgithedgehogcomv1alpha2)


## agent.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the agent v1alpha2 API group

### Resource Types
- [Agent](#agent)



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
| `ports` _[Port](#port) array_ |  |


#### AgentStatus



AgentStatus defines the observed state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `applied` _boolean_ |  |
| `lastApplied` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#time-v1-meta)_ |  |


#### Interface





_Appears in:_
- [Port](#port)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `vlan` _integer_ |  |
| `vlanUntagged` _boolean_ |  |
| `ipAddress` _string_ |  |


#### Port





_Appears in:_
- [AgentSpec](#agentspec)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `interfaces` _[Interface](#interface) array_ |  |



## wiring.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the wiring v1alpha2 API group

### Resource Types
- [Rack](#rack)
- [Server](#server)
- [ServerPort](#serverport)
- [Switch](#switch)
- [SwitchPort](#switchport)



#### AddressFamily





_Appears in:_
- [BGPRouterConfig](#bgprouterconfig)

| Field | Description |
| --- | --- |
| `family` _string_ |  |
| `importTarget` _string array_ |  |
| `exportTarget` _string array_ |  |


#### BGPConfig





_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `loopbackInterfaceNum` _integer_ |  |
| `loopbackAddress` _string_ |  |
| `bgpRouterConfig` _[BGPRouterConfig](#bgprouterconfig) array_ |  |
| `borderConfig` _[BorderConfig](#borderconfig)_ |  |


#### BGPNeighborInfo





_Appears in:_
- [BGPRouterConfig](#bgprouterconfig)

| Field | Description |
| --- | --- |
| `id` _string_ |  |
| `asn` _integer_ |  |
| `filterInfo` _FilterInfo_ |  |


#### BGPRouterConfig





_Appears in:_
- [BGPConfig](#bgpconfig)

| Field | Description |
| --- | --- |
| `asn` _integer_ |  |
| `vrf` _string_ |  |
| `routerID` _string_ |  |
| `neighborInfo` _[BGPNeighborInfo](#bgpneighborinfo) array_ |  |
| `addressFamily` _[AddressFamily](#addressfamily)_ |  |


#### BorderConfig





_Appears in:_
- [BGPConfig](#bgpconfig)

| Field | Description |
| --- | --- |
| `vrf` _string_ |  |
| `defaultRoute` _string_ |  |
| `exportSummarized` _string_ |  |


#### BundleConfig





_Appears in:_
- [Bundled](#bundled)
- [ServerConnection](#serverconnection)

| Field | Description |
| --- | --- |
| `bundleType` _BundleType_ |  |


#### Bundled





_Appears in:_
- [ServerPortSpec](#serverportspec)

| Field | Description |
| --- | --- |
| `id` _string_ |  |
| `type` _BundleType_ |  |
| `members` _[ServerPortInfo](#serverportinfo) array_ |  |
| `config` _[BundleConfig](#bundleconfig)_ |  |


#### CtrlMgmt





_Appears in:_
- [ServerPortSpec](#serverportspec)

| Field | Description |
| --- | --- |
| `vlan` _integer_ |  |
| `ipAddress` _string_ |  |


#### CtrlMgmtInfo





_Appears in:_
- [ServerConnection](#serverconnection)

| Field | Description |
| --- | --- |
| `vlanInfo` _[VlanInfo](#vlaninfo)_ |  |
| `ipAddress` _string_ |  |


#### Interface



Interfaces are pseudo ports ( vlan interfaces,subinterfaces). They always have a parent Port.

_Appears in:_
- [SwitchPortSpec](#switchportspec)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `vlans` _integer array_ |  |
| `ipAddress` _string_ |  |
| `bgpEnabled` _boolean_ |  |
| `bfdEnabled` _boolean_ |  |
| `vrf` _string_ |  |
| `mode` _InterfaceMode_ |  |
| `bundle` _string_ |  |


#### LLDPConfig





_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `helloTimer` _Duration_ |  |
| `managementIP` _string_ |  |
| `systemDescription` _string_ |  |
| `systemName` _string_ |  |


#### Neighbor



Neighbor represents the neighbor of a particular port which could be either be a Switch or Server

_Appears in:_
- [Nic](#nic)
- [ServerPortInfo](#serverportinfo)
- [SwitchPortSpec](#switchportspec)

| Field | Description |
| --- | --- |
| `switch` _[NeighborInfo](#neighborinfo)_ |  |
| `server` _[NeighborInfo](#neighborinfo)_ |  |


#### NeighborInfo





_Appears in:_
- [Neighbor](#neighbor)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `port` _string_ |  |


#### Nic





_Appears in:_
- [ServerConnection](#serverconnection)

| Field | Description |
| --- | --- |
| `neighbor` _[Neighbor](#neighbor)_ |  |
| `nicName` _string_ |  |
| `nicIndex` _integer_ |  |


#### ONIEConfig



ONIEConfig holds all the port configuration at installation/ONIE time. They are being consumed by the seeder (DAS BOOT).

_Appears in:_
- [SwitchPortSpec](#switchportspec)

| Field | Description |
| --- | --- |
| `portNum` _integer_ |  |
| `portName` _string_ |  |
| `bootstrapIP` _string_ |  |
| `vlan` _integer_ |  |
| `routes` _[ONIERoutes](#onieroutes) array_ |  |


#### ONIERoutes



ONIERoutes holds additional routing information to be applied in ONIE at installation/ONIE time. They are being consumed by the seeder (DAS BOOT).

_Appears in:_
- [ONIEConfig](#onieconfig)

| Field | Description |
| --- | --- |
| `destinations` _string array_ |  |
| `gateway` _string_ |  |


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




#### Server



Server is the Schema for the servers API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Server`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ServerSpec](#serverspec)_ |  |
| `status` _[ServerStatus](#serverstatus)_ |  |


#### ServerConnection





_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description |
| --- | --- |
| `isBundled` _boolean_ |  |
| `connectionType` _ServerConnectionType_ |  |
| `nics` _[Nic](#nic) array_ |  |
| `bundleConfig` _[BundleConfig](#bundleconfig)_ | Connection Config |
| `ctrlMgmtInfo` _[CtrlMgmtInfo](#ctrlmgmtinfo)_ |  |


#### ServerPort



ServerPort is the Schema for the serverports API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `ServerPort`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ServerPortSpec](#serverportspec)_ |  |
| `status` _[ServerPortStatus](#serverportstatus)_ |  |


#### ServerPortInfo





_Appears in:_
- [Bundled](#bundled)
- [ServerPortSpec](#serverportspec)

| Field | Description |
| --- | --- |
| `nicName` _string_ |  |
| `nicIndex` _integer_ |  |
| `neighbor` _[Neighbor](#neighbor)_ |  |


#### ServerPortSpec



ServerPortSpec defines the desired state of ServerPort

_Appears in:_
- [ServerPort](#serverport)

| Field | Description |
| --- | --- |
| `bundled` _[Bundled](#bundled)_ |  |
| `unbundled` _[ServerPortInfo](#serverportinfo)_ |  |
| `ctrlMgmt` _[CtrlMgmt](#ctrlmgmt)_ |  |




#### ServerSpec



ServerSpec defines the desired state of Server

_Appears in:_
- [Server](#server)

| Field | Description |
| --- | --- |
| `serverConnections` _[ServerConnection](#serverconnection) array_ |  |




#### Switch



Switch is the Schema for the switches API 
 All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents names of the rack it belongs to.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `Switch`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[SwitchSpec](#switchspec)_ |  |
| `status` _[SwitchStatus](#switchstatus)_ |  |


#### SwitchLocation



SwitchLocation defines the geopraphical position of the switch in a datacenter

_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `location` _string_ |  |
| `aisle` _string_ |  |
| `row` _string_ |  |
| `rack` _string_ |  |
| `slot` _string_ |  |


#### SwitchLocationSig



SwitchLocationSig contains signatures for the location UUID as well as the Switch location itself

_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `sig` _string_ |  |
| `uuidSig` _string_ |  |


#### SwitchPort



SwitchPort is the Schema for the ports API 
 All ports should always have 2 labels defined: wiring.githedgehog.com/rack and wiring.githedgehog.com/switch. It represents names of the rack and switch it belongs to.



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `wiring.githedgehog.com/v1alpha2`
| `kind` _string_ | `SwitchPort`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[SwitchPortSpec](#switchportspec)_ |  |
| `status` _[SwitchPortStatus](#switchportstatus)_ |  |


#### SwitchPortSpec



SwitchPortSpec is the model used to represent a switch port

_Appears in:_
- [SwitchPort](#switchport)

| Field | Description |
| --- | --- |
| `role` _SwitchPortRole_ |  |
| `isConnected` _boolean_ |  |
| `nosPortNum` _integer_ |  |
| `nosPortName` _string_ |  |
| `portSpeed` _string_ |  |
| `connectorType` _string_ |  |
| `cableType` _CableType_ |  |
| `neighbor` _[Neighbor](#neighbor)_ |  |
| `onie` _[ONIEConfig](#onieconfig)_ |  |
| `interfaces` _[Interface](#interface) array_ |  |
| `adminState` _string_ |  |
| `vrf` _string_ |  |




#### SwitchSpec



SwitchSpec defines the desired state of Switch

_Appears in:_
- [Switch](#switch)

| Field | Description |
| --- | --- |
| `secureBootCapable` _boolean_ |  |
| `remoteAttestationRequired` _boolean_ |  |
| `location` _[SwitchLocation](#switchlocation)_ |  |
| `locationUUID` _string_ |  |
| `locationSig` _[SwitchLocationSig](#switchlocationsig)_ |  |
| `connectedPorts` _integer_ |  |
| `maxPorts` _integer_ |  |
| `serverFacingPorts` _integer_ |  |
| `fabricFacingPorts` _integer_ |  |
| `role` _SwitchRole_ |  |
| `bgpConfig` _[BGPConfig](#bgpconfig) array_ |  |
| `lldpConfig` _[LLDPConfig](#lldpconfig)_ |  |
| `vendorName` _string_ |  |
| `modelNumber` _string_ |  |
| `sonicVersion` _string_ |  |
| `vlan` _[VlanInfo](#vlaninfo) array_ |  |
| `vrfs` _string array_ |  |




#### VlanInfo





_Appears in:_
- [CtrlMgmtInfo](#ctrlmgmtinfo)
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `vlanID` _integer_ |  |
| `vlanInterfaceEnabled` _boolean_ |  |
| `taggedVlan` _boolean_ |  |


