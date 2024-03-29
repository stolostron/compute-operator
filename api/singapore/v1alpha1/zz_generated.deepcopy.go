//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright Red Hat

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRegistrar) DeepCopyInto(out *ClusterRegistrar) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRegistrar.
func (in *ClusterRegistrar) DeepCopy() *ClusterRegistrar {
	if in == nil {
		return nil
	}
	out := new(ClusterRegistrar)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRegistrar) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRegistrarList) DeepCopyInto(out *ClusterRegistrarList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterRegistrar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRegistrarList.
func (in *ClusterRegistrarList) DeepCopy() *ClusterRegistrarList {
	if in == nil {
		return nil
	}
	out := new(ClusterRegistrarList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRegistrarList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRegistrarSpec) DeepCopyInto(out *ClusterRegistrarSpec) {
	*out = *in
	out.ComputeService = in.ComputeService
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRegistrarSpec.
func (in *ClusterRegistrarSpec) DeepCopy() *ClusterRegistrarSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterRegistrarSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRegistrarStatus) DeepCopyInto(out *ClusterRegistrarStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRegistrarStatus.
func (in *ClusterRegistrarStatus) DeepCopy() *ClusterRegistrarStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterRegistrarStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ComputeService) DeepCopyInto(out *ComputeService) {
	*out = *in
	out.ComputeKubeconfigSecretRef = in.ComputeKubeconfigSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ComputeService.
func (in *ComputeService) DeepCopy() *ComputeService {
	if in == nil {
		return nil
	}
	out := new(ComputeService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HubConfig) DeepCopyInto(out *HubConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HubConfig.
func (in *HubConfig) DeepCopy() *HubConfig {
	if in == nil {
		return nil
	}
	out := new(HubConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HubConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HubConfigList) DeepCopyInto(out *HubConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]HubConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HubConfigList.
func (in *HubConfigList) DeepCopy() *HubConfigList {
	if in == nil {
		return nil
	}
	out := new(HubConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HubConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HubConfigSpec) DeepCopyInto(out *HubConfigSpec) {
	*out = *in
	out.KubeConfigSecretRef = in.KubeConfigSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HubConfigSpec.
func (in *HubConfigSpec) DeepCopy() *HubConfigSpec {
	if in == nil {
		return nil
	}
	out := new(HubConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HubConfigStatus) DeepCopyInto(out *HubConfigStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HubConfigStatus.
func (in *HubConfigStatus) DeepCopy() *HubConfigStatus {
	if in == nil {
		return nil
	}
	out := new(HubConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegisteredCluster) DeepCopyInto(out *RegisteredCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegisteredCluster.
func (in *RegisteredCluster) DeepCopy() *RegisteredCluster {
	if in == nil {
		return nil
	}
	out := new(RegisteredCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RegisteredCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegisteredClusterList) DeepCopyInto(out *RegisteredClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RegisteredCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegisteredClusterList.
func (in *RegisteredClusterList) DeepCopy() *RegisteredClusterList {
	if in == nil {
		return nil
	}
	out := new(RegisteredClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RegisteredClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegisteredClusterSpec) DeepCopyInto(out *RegisteredClusterSpec) {
	*out = *in
	if in.Location != nil {
		in, out := &in.Location, &out.Location
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegisteredClusterSpec.
func (in *RegisteredClusterSpec) DeepCopy() *RegisteredClusterSpec {
	if in == nil {
		return nil
	}
	out := new(RegisteredClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegisteredClusterStatus) DeepCopyInto(out *RegisteredClusterStatus) {
	*out = *in
	out.ImportCommandRef = in.ImportCommandRef
	out.ClusterSecretRef = in.ClusterSecretRef
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Capacity != nil {
		in, out := &in.Capacity, &out.Capacity
		*out = make(clusterv1.ResourceList, len(*in))
		for key, val := range *in {
			(*out)[key] = val.DeepCopy()
		}
	}
	if in.Allocatable != nil {
		in, out := &in.Allocatable, &out.Allocatable
		*out = make(clusterv1.ResourceList, len(*in))
		for key, val := range *in {
			(*out)[key] = val.DeepCopy()
		}
	}
	out.Version = in.Version
	if in.ClusterClaims != nil {
		in, out := &in.ClusterClaims, &out.ClusterClaims
		*out = make([]clusterv1.ManagedClusterClaim, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegisteredClusterStatus.
func (in *RegisteredClusterStatus) DeepCopy() *RegisteredClusterStatus {
	if in == nil {
		return nil
	}
	out := new(RegisteredClusterStatus)
	in.DeepCopyInto(out)
	return out
}
