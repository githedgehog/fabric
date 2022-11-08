# API Reference

## Packages
- [fabric.githedgehog.com/v1alpha1](#fabricgithedgehogcomv1alpha1)


## fabric.githedgehog.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the fabric v1alpha1 API group

### Resource Types
- [Fabric](#fabric)



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


