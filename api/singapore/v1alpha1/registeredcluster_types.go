// Copyright Red Hat

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// +kubebuilder:printcolumn:JSONPath=`.status.conditions[?(@.type=="ManagedClusterJoined")].status`,name="Joined",type=string
// +kubebuilder:printcolumn:JSONPath=`.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status`,name="Available",type=string
// +kubebuilder:printcolumn:JSONPath=`.metadata.creationTimestamp`,name="Age",type=date

// RegisteredClusterSpec defines the desired state of RegisteredCluster
type RegisteredClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file

}

// RegisteredClusterStatus defines the observed state of RegisteredCluster
type RegisteredClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make generate" to regenerate code after modifying this file

	//ImportCommandRef is reference to configmap containing import command.
	ImportCommandRef corev1.LocalObjectReference `json:"importCommandRef,omitempty"`

	//ClusterSecretRef is a reference to the secret containing the registered cluster kubeconfig.
	ClusterSecretRef corev1.LocalObjectReference `json:"clusterSecretRef,omitempty"`

	// Conditions contains the different condition statuses for this RegisteredCluster.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Capacity represents the total resource capacity from all nodeStatuses
	// on the registered cluster.
	// +optional
	Capacity clusterv1.ResourceList `json:"capacity,omitempty"`

	// Allocatable represents the total allocatable resources on the registered cluster.
	// +optional
	Allocatable clusterv1.ResourceList `json:"allocatable,omitempty"`

	// Version represents the kubernetes version of the registered cluster.
	// +optional
	Version clusterv1.ManagedClusterVersion `json:"version,omitempty"`

	// ClusterClaims represents cluster information that a registered cluster claims,
	// for example a unique cluster identifier (id.k8s.io) and kubernetes version
	// (kubeversion.open-cluster-management.io). They are written from the registered
	// cluster. The set of claims is not uniform across a fleet, some claims can be
	// vendor or version specific and may not be included from all registered clusters.
	// +optional
	ClusterClaims []clusterv1.ManagedClusterClaim `json:"clusterClaims,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RegisteredCluster represents the desired state and current status of registered
// cluster. The name is the cluster
// UID.
type RegisteredCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegisteredClusterSpec   `json:"spec,omitempty"`
	Status RegisteredClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// RegisteredClusterList contains a list of RegisteredCluster
type RegisteredClusterList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of RegisteredCluster.
	// +listType=set
	Items []RegisteredCluster `json:"items"`
}
