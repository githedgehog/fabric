/*
Copyright 2022 The Hedgehog Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ConsumerSpec defines the desired state of Consumer
type ConsumerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Consumer. Edit consumer_types.go to remove/update
	KubeCluster *ConsumerKubeCluster `json:"kubeCluster,omitempty"`
}

type ConsumerKubeCluster struct {
	Vlan  ConsumerKubeClusterVlanSpec `json:"vlan,omitempty"`
	Ports []PortSpec                  `json:"ports,omitempty"`
}

type ConsumerKubeClusterVlanSpec struct {
	Id       int  `json:"id,omitempty"`
	Untagged bool `json:"untagged,omitempty"`
}

// ConsumerStatus defines the observed state of Consumer
type ConsumerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=hedgehog

// Consumer is the Schema for the consumers API
type Consumer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsumerSpec   `json:"spec,omitempty"`
	Status ConsumerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConsumerList contains a list of Consumer
type ConsumerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Consumer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Consumer{}, &ConsumerList{})
}