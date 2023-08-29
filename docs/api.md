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
- [Connection](#connection)
- [Rack](#rack)
- [Server](#server)
- [ServerProfile](#serverprofile)
- [Switch](#switch)
- [SwitchProfile](#switchprofile)



#### ConnLink





_Appears in:_
- [MCLAGConn](#mclagconn)
- [MCLAGDomainConn](#mclagdomainconn)

| Field | Description |
| --- | --- |
| `switchPort` _[ConnLinkPort](#connlinkport)_ |  |
| `serverPort` _[ConnLinkPort](#connlinkport)_ |  |


#### ConnLinkPart





_Appears in:_
- [UnbundledConn](#unbundledconn)

| Field | Description |
| --- | --- |
| `switchPort` _[ConnLinkPort](#connlinkport)_ |  |
| `serverPort` _[ConnLinkPort](#connlinkport)_ |  |


#### ConnLinkPort





_Appears in:_
- [ConnLinkPart](#connlinkpart)
- [ManagementConnLinkPart](#managementconnlinkpart)
- [ManagementConnSwitchPort](#managementconnswitchport)

| Field | Description |
| --- | --- |
| `name` _string_ |  |


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
| `unbundled` _[UnbundledConn](#unbundledconn)_ |  |
| `management` _[ManagementConn](#managementconn)_ |  |
| `mclag` _[MCLAGConn](#mclagconn)_ |  |
| `mclagDomain` _[MCLAGDomainConn](#mclagdomainconn)_ |  |




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


#### MCLAGConn





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[ConnLink](#connlink) array_ |  |


#### MCLAGDomainConn





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `links` _[ConnLink](#connlink) array_ |  |


#### ManagementConn





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ManagementConnLinkPart](#managementconnlinkpart) array_ |  |




#### ManagementConnLinkPart





_Appears in:_
- [ManagementConn](#managementconn)

| Field | Description |
| --- | --- |
| `switchPort` _[ManagementConnSwitchPort](#managementconnswitchport)_ |  |
| `serverPort` _[ConnLinkPort](#connlinkport)_ |  |


#### ManagementConnSwitchPort





_Appears in:_
- [ManagementConnLinkPart](#managementconnlinkpart)

| Field | Description |
| --- | --- |
| `name` _string_ |  |
| `ip` _string_ |  |


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




#### UnbundledConn





_Appears in:_
- [ConnectionSpec](#connectionspec)

| Field | Description |
| --- | --- |
| `link` _[ConnLinkPart](#connlinkpart) array_ |  |


