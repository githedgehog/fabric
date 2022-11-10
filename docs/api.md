# API Reference

## Packages
- [fabric.githedgehog.com/v1alpha1](#fabricgithedgehogcomv1alpha1)


## fabric.githedgehog.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the fabric v1alpha1 API group

### Resource Types
- [Agent](#agent)
- [AgentList](#agentlist)
- [Device](#device)
- [DeviceList](#devicelist)
- [Fabric](#fabric)
- [Link](#link)



#### Agent



Agent is the Schema for the agents API

_Appears in:_
- [AgentList](#agentlist)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Agent`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[AgentSpec](#agentspec)_ |  |
| `status` _[AgentStatus](#agentstatus)_ |  |


#### AgentList



AgentList contains a list of Agent



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `AgentList`
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `items` _[Agent](#agent) array_ |  |


#### AgentSpec



AgentSpec defines the desired state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `foo` _string_ | Foo is an example field of Agent. Edit agent_types.go to remove/update |




#### Device



Device is the Schema for the devices API

_Appears in:_
- [DeviceList](#devicelist)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Device`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[DeviceSpec](#devicespec)_ |  |
| `status` _[DeviceStatus](#devicestatus)_ |  |


#### DeviceList



DeviceList contains a list of Device



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `DeviceList`
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `items` _[Device](#device) array_ |  |


#### DeviceSpec



DeviceSpec defines the desired state of Device

_Appears in:_
- [Device](#device)

| Field | Description |
| --- | --- |
| `foo` _string_ | Foo is an example field of Device. Edit device_types.go to remove/update |




#### Fabric



Fabric is the Schema for the fabrics API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Fabric`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[FabricSpec](#fabricspec)_ |  |
| `status` _[FabricStatus](#fabricstatus)_ |  |


#### FabricSpec



FabricSpec defines the desired state of Fabric

_Appears in:_
- [Fabric](#fabric)

| Field | Description |
| --- | --- |
| `foo` _string_ | Foo is an example field of Fabric. Edit fabric_types.go to remove/update |


#### FabricStatus



FabricStatus defines the observed state of Fabric

_Appears in:_
- [Fabric](#fabric)

| Field | Description |
| --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#condition-v1-meta) array_ |  |


#### Link



Link is the Schema for the links API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Link`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[LinkSpec](#linkspec)_ |  |
| `status` _[LinkStatus](#linkstatus)_ |  |


#### LinkSpec



LinkSpec defines the desired state of Link

_Appears in:_
- [Link](#link)

| Field | Description |
| --- | --- |
| `foo` _string_ | Foo is an example field of Link. Edit link_types.go to remove/update |




