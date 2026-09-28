/*
Copyright 2026.

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
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageWarmupPolicySpec defines the desired state of ImageWarmupPolicy
type ImageWarmupPolicySpec struct {
	// Suspend prevents the operator from starting new warming work.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// NamespaceSelector selects the namespaces by their labels.
	// An omitted or empty selector does not include additional restrictions.
	// +optional
	NamespaceSelector metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// NamespaceList selects the namespaces by their names.
	// An omitted or empty selector does not include additional restrictions.
	// +optional
	NamespaceList []string `json:"namespaceList,omitempty"`

	// NamespaceSkipList excludes the namespaces by their names.
	// An omitted or empty list excludes no namespaces.
	// +optional
	NamespaceSkipList []string `json:"namespaceSkipList,omitempty"`

	// WorkLoadSelector select the source workloads based on the labels.
	// An omitted or empty selector includes all supported workloads.
	// +optional
	WorkloadSelector metav1.LabelSelector `json:"workloadSelector,omitempty"`

	// WorkLoadSkipSelector excludes the source workloads based on the labels.
	// An omitted or empty selector excludes no workloads.
	// +optional
	WorkloadSkipSelector metav1.LabelSelector `json:"workloadSkipSelector,omitempty"`

	// NodeSelector selects the nodes by their labels.
	// An omitted or empty selector does not include additional restrictions.
	// +optional
	NodeSelector metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// NodeSkipSelector excludes the nodes by their labels.
	// An omitted or empty selector excludes no nodes.
	// +optional
	NodeSkipSelector metav1.LabelSelector `json:"nodeSkipSelector,omitempty"`
}

// ImageWarmupPolicyStatus defines the observed state of ImageWarmupPolicy.
type ImageWarmupPolicyStatus struct {
	// DiscoveredImageCount is the number of distinct image references
	// found during the most recent successful discovery.
	// +optional
	DiscoveredImageCount int32 `json:"discoveredImageCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// ImageWarmupPolicy is the Schema for the imagewarmuppolicies API
type ImageWarmupPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ImageWarmupPolicy
	// +required
	Spec ImageWarmupPolicySpec `json:"spec"`

	// status defines the observed state of ImageWarmupPolicy
	// +optional
	Status ImageWarmupPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ImageWarmupPolicyList contains a list of ImageWarmupPolicy
type ImageWarmupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ImageWarmupPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ImageWarmupPolicy{}, &ImageWarmupPolicyList{})
		return nil
	})
}
