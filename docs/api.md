# API Reference

## Packages
- [fabric.githedgehog.com/v1alpha1](#fabricgithedgehogcomv1alpha1)


## fabric.githedgehog.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the fabric v1alpha1 API group

### Resource Types
- [Agent](#agent)
- [Consumer](#consumer)
- [ConsumerList](#consumerlist)
- [Device](#device)
- [Fabric](#fabric)
- [Link](#link)



#### Agent



Agent is the Schema for the agents API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Agent`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[AgentSpec](#agentspec)_ |  |
| `status` _[AgentStatus](#agentstatus)_ |  |


#### AgentSpec



AgentSpec defines the desired state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `device` _string_ | Foo is an example field of Agent. Edit agent_types.go to remove/update |
| `tasks` _[AgentSpecTask](#agentspectask) array_ |  |


#### AgentSpecTask





_Appears in:_
- [AgentSpec](#agentspec)

| Field | Description |
| --- | --- |
| `vlan` _[AgentSpecTaskVlan](#agentspectaskvlan)_ |  |


#### AgentSpecTaskVlan





_Appears in:_
- [AgentSpecTask](#agentspectask)

| Field | Description |
| --- | --- |
| `port` _string_ |  |
| `id` _integer_ |  |
| `untagged` _boolean_ |  |


#### AgentStatus



AgentStatus defines the observed state of Agent

_Appears in:_
- [Agent](#agent)

| Field | Description |
| --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#condition-v1-meta) array_ |  |


#### Consumer



Consumer is the Schema for the consumers API

_Appears in:_
- [ConsumerList](#consumerlist)

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Consumer`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ConsumerSpec](#consumerspec)_ |  |
| `status` _[ConsumerStatus](#consumerstatus)_ |  |


#### ConsumerKubeCluster





_Appears in:_
- [ConsumerSpec](#consumerspec)

| Field | Description |
| --- | --- |
| `vlan` _[ConsumerKubeClusterVlanSpec](#consumerkubeclustervlanspec)_ |  |
| `ports` _[PortSpec](#portspec) array_ |  |


#### ConsumerKubeClusterVlanSpec





_Appears in:_
- [ConsumerKubeCluster](#consumerkubecluster)

| Field | Description |
| --- | --- |
| `id` _integer_ |  |
| `untagged` _boolean_ |  |


#### ConsumerList



ConsumerList contains a list of Consumer



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `ConsumerList`
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `items` _[Consumer](#consumer) array_ |  |


#### ConsumerSpec



ConsumerSpec defines the desired state of Consumer

_Appears in:_
- [Consumer](#consumer)

| Field | Description |
| --- | --- |
| `kubeCluster` _[ConsumerKubeCluster](#consumerkubecluster)_ | Foo is an example field of Consumer. Edit consumer_types.go to remove/update |




#### Device



Device is the Schema for the devices API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `fabric.githedgehog.com/v1alpha1`
| `kind` _string_ | `Device`
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[DeviceSpec](#devicespec)_ |  |
| `status` _[DeviceStatus](#devicestatus)_ |  |


#### DeviceSpec



DeviceSpec defines the desired state of Device

_Appears in:_
- [Device](#device)

| Field | Description |
| --- | --- |
| `type` _[DeviceType](#devicetype)_ | Foo is an example field of Device. Edit device_types.go to remove/update |
| `ports` _[DeviceSpecPort](#devicespecport) array_ |  |


#### DeviceSpecPort





_Appears in:_
- [DeviceSpec](#devicespec)

| Field | Description |
| --- | --- |
| `name` _string_ |  |




#### DeviceType

_Underlying type:_ `string`



_Appears in:_
- [DeviceSpec](#devicespec)



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
| `enabled` _boolean_ | Foo is an example field of Fabric. Edit fabric_types.go to remove/update |


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
| `ports` _[PortSpec](#portspec) array_ |  |




#### PortSpec





_Appears in:_
- [ConsumerKubeCluster](#consumerkubecluster)
- [LinkSpec](#linkspec)

| Field | Description |
| --- | --- |
| `device` _string_ |  |
| `port` _string_ |  |


