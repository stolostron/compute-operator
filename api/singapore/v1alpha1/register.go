// Copyright Red Hat

// Package v1alpha1 contains API Schema definitions for the auth v1alpha1 API group
//+kubebuilder:object:generate=true
//+groupName=singapore.open-cluster-management.io
package v1alpha1

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: "singapore.open-cluster-management.io", Version: "v1alpha1"}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// localSchemeBuilder and AddToScheme will stay in k8s.io/kubernetes.
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	// Depreciated: use Install instead
	AddToScheme = localSchemeBuilder.AddToScheme
	Install     = localSchemeBuilder.AddToScheme
)

func init() {
	// We only register manually written functions here. The registration of the
	// generated functions takes place in the generated files. The separation
	// makes the code compile even when the generated files are missing.
	localSchemeBuilder.Register(addKnownTypes)
}

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ClusterRegistrar{},
		&ClusterRegistrarList{},
		&RegisteredCluster{},
		&RegisteredClusterList{},
	)
	// AddToGroupVersion allows the serialization of client types like ListOptions.
	v1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
