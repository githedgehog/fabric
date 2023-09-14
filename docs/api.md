# API Reference

## Packages
- [agent.githedgehog.com/v1alpha2](#agentgithedgehogcomv1alpha2)
- [vpc.githedgehog.com/v1alpha2](#vpcgithedgehogcomv1alpha2)
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



## vpc.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the vpc v1alpha2 API group

### Resource Types
- [VPC](#vpc)
- [VPCMember](#vpcmember)



#### VPC



VPC is the Schema for the vpcs API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `VPC`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VPCSpec](#vpcspec)_ |  |
| `status` _[VPCStatus](#vpcstatus)_ |  |


#### VPCMember



VPCMember is the Schema for the vpcmembers API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `vpc.githedgehog.com/v1alpha2`
| `kind` _string_ | `VPCMember`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[VPCMemberSpec](#vpcmemberspec)_ |  |
| `status` _[VPCMemberStatus](#vpcmemberstatus)_ |  |


#### VPCMemberSpec



VPCMemberSpec defines the desired state of VPCMember

_Appears in:_
- [VPCMember](#vpcmember)

| Field | Description |
| --- | --- |
| `vpc` _string_ |  |
| `connection` _string_ |  |




#### VPCSpec



VPCSpec defines the desired state of VPC

_Appears in:_
- [VPC](#vpc)

| Field | Description |
| --- | --- |
| `subnet` _string_ |  |





## wiring.githedgehog.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the wiring v1alpha2 API group

### Resource Types
- [Connection](#connection)
- [Rack](#rack)
- [Server](#server)
- [ServerProfile](#serverprofile)
- [Switch](#switch)
- [SwitchProfile](#switchprofile)



#### BasePortName





_Appears in:_
- [ConnMgmtLinkServer](#connmgmtlinkserver)
- [ConnMgmtLinkSwitch](#connmgmtlinkswitch)
- [ServerToSwitchLink](#servertoswitchlink)
- [SwitchToSwitchLink](#switchtoswitchlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |


#### ConnMCLAG





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[ServerToSwitchLink](#servertoswitchlink) array_ |  |


#### ConnMCLAGDomain





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[SwitchToSwitchLink](#switchtoswitchlink) array_ |  |


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


#### ConnMgmtLinkSwitch





_Appears in:_
- [ConnMgmtLink](#connmgmtlink)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `ip` _string_ |  |
| `oniePortName` _string_ |  |


#### ConnUnbundled





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ServerToSwitchLink](#servertoswitchlink)_ |  |


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
- [Connection](#connection)

| Field | Description |
| --- | --- |
| `unbundled` _[ConnUnbundled](#connunbundled)_ |  |
| `management` _[ConnMgmt](#connmgmt)_ |  |
| `mclag` _[ConnMCLAG](#connmclag)_ |  |
| `mclagDomain` _[ConnMCLAGDomain](#connmclagdomain)_ |  |






#### LLDPConfig





_Appears in:_
- [SwitchSpec](#switchspec)

| Field | Description |
| --- | --- |
| `helloTimer` _Duration_ |  |
| `name` _string_ |  |
| `description` _string_ |  |


#### Location



Location defines the geopraphical position of the device in a datacenter

_Appears in:_
- [ServerSpec](#serverspec)
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
- [ServerSpec](#serverspec)
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
| `profile` _string_ |  |
| `location` _[Location](#location)_ |  |
| `locationSig` _[LocationSig](#locationsig)_ |  |




#### ServerToSwitchLink





_Appears in:_
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
- [Switch](#switch)

| Field | Description |
| --- | --- |
| `profile` _string_ |  |
| `location` _[Location](#location)_ |  |
| `locationSig` _[LocationSig](#locationsig)_ |  |
| `lldp` _[LLDPConfig](#lldpconfig)_ |  |




#### SwitchToSwitchLink





_Appears in:_
- [ConnMCLAGDomain](#connmclagdomain)

| Field | Description |
| --- | --- |
| `switch1` _[BasePortName](#baseportname)_ |  |
| `switch2` _[BasePortName](#baseportname)_ |  |


