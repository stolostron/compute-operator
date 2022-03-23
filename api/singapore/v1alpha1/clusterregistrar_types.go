// Copyright Red Hat

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterRegistrarSpec defines the desired state of ClusterRegistrar
type ClusterRegistrarSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

}

// ClusterRegistrarStatus defines the observed state of ClusterRegistrar
type ClusterRegistrarStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make generate-clients" to regenerate code after modifying this file

	// Conditions contains the different condition statuses for this ClusterRegistrar.
	// +optional
	Conditions []metav1.Condition `json:"conditions"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClusterRegistrar is the Schema for the clusterregistrars API
type ClusterRegistrar struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRegistrarSpec   `json:"spec,omitempty"`
	Status ClusterRegistrarStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterRegistrarList contains a list of ClusterRegistrar
type ClusterRegistrarList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of ClusterRegistrar.
	// +listType=set
	Items []ClusterRegistrar `json:"items"`
}
