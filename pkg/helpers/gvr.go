// Copyright Red Hat

package helpers

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	GvrCR schema.GroupVersionResource = schema.GroupVersionResource{
		Group:    "singapore.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "clusterregistrars"}
)
