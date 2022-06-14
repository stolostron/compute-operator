// Copyright Red Hat

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HubConfigSpec defines the desired state of HubConfig
type HubConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file
	KubeConfigSecretRef corev1.LocalObjectReference `json:"kubeConfigSecretRef,omitempty"`

	// Maximum burst for throttle.
	// If it's zero, the created Client will use DefaultBurst: 200.
	// +optional
	Burst int `json:"Burst,omitempty"`

	// QPS indicates the maximum QPS to the master from this client.
	// If it's zero, the created Client will use DefaultQPS: 100.0
	// +optional
	QPS string `json:"QPS,omitempty"`
}

// HubConfigStatus defines the observed state of HubConfig
type HubConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file

	// Conditions contains the different condition statuses for this HubConfig.
	// +optional
	Conditions []metav1.Condition `json:"conditions"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HubConfig is the Schema for the clusterregistrars API
type HubConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HubConfigSpec   `json:"spec,omitempty"`
	Status HubConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HubConfigList contains a list of HubConfig
type HubConfigList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of HubConfig.
	// +listType=set
	Items []HubConfig `json:"items"`
}
