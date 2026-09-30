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

	// BackoffLimit is the number of retries allowed when the job fails.
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	// ActiveDeadlineSeconds is the maximum total runtime of a warming Job.
	// +optional
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=86400
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	// TTLSecondsAfterFinished controls how long a completed warming Job is retained.
	// +optional
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=604800
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// MaxConcurrentJobs limits how many warming Jobs may be active at the same time.
	// All targets are still attempted as capacity becomes available.
	// +optional
	// +kubebuilder:default=4
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxConcurrentJobs *int32 `json:"maxConcurrentJobs,omitempty"`

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

// ImageWarmupRunTrigger identifies what started a warming run.
type ImageWarmupRunTrigger string

const (
	ImageWarmupRunTriggerInitial   ImageWarmupRunTrigger = "Initial"
	ImageWarmupRunTriggerScheduled ImageWarmupRunTrigger = "Scheduled"
	ImageWarmupRunTriggerNode      ImageWarmupRunTrigger = "Node"
)

type ImageWarmupRunSummary struct {
	ID             string                `json:"id"`
	Trigger        ImageWarmupRunTrigger `json:"trigger"`
	TargetCount    int32                 `json:"targetCount"`
	SucceededCount int32                 `json:"succeededCount"`
	FailedCount    int32                 `json:"failedCount"`
	CompletionTime metav1.Time           `json:"completionTime"`
}

// ImageWarmupPolicyStatus defines the observed state of ImageWarmupPolicy.
type ImageWarmupPolicyStatus struct {
	// DiscoveredImageCount is the number of distinct image references
	// found during the most recent successful discovery.
	// +optional
	DiscoveredImageCount int32 `json:"discoveredImageCount,omitempty"`

	// CurrentRunID identifies the active warming run.
	// It remains stable across repeated reconciliations and controller restarts.
	// +optional
	CurrentRunID string `json:"currentRunID,omitempty"`

	// CurrentRunTrigger records what started the active run.
	// +optional
	CurrentRunTrigger ImageWarmupRunTrigger `json:"currentRunTrigger,omitempty"`

	// CurrentRunTargetCount is the desired number of targets in the active run.
	// +optional
	CurrentRunTargetCount int32 `json:"currentRunTargetCount,omitempty"`

	// CurrentRunActiveCount is the number of nonterminal Jobs in the active run.
	// +optional
	CurrentRunActiveCount int32 `json:"currentRunActiveCount,omitempty"`

	// CurrentRunSucceededCount is the number of successful Jobs in the active run.
	// +optional
	CurrentRunSucceededCount int32 `json:"currentRunSucceededCount,omitempty"`

	// CurrentRunFailedCount is the number of failed Jobs in the active run.
	// +optional
	CurrentRunFailedCount int32 `json:"currentRunFailedCount,omitempty"`

	// LastFinishedRun stores one bounded summary of the most recently finished run.
	// +optional
	LastFinishedRun *ImageWarmupRunSummary `json:"lastFinishedRun,omitempty"`
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
