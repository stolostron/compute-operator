// Copyright Red Hat

package webhook

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	computesv1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	GROUP_SUFFIX = "singapore.open-cluster-management.io"
)

type ComputesAdmissionHook struct {
	Client                 dynamic.ResourceInterface
	ClusterRegistrarClient dynamic.ResourceInterface
	KubeClient             kubernetes.Interface
	lock                   sync.RWMutex
	initialized            bool
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
func (a *ComputesAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "admission." + GROUP_SUFFIX,
			Version:  "v1alpha1",
			Resource: "computevalidators",
		},
		"computevalidators"
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
func (a *ComputesAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}
	klog.V(4).Infof("Computes Validate %q operation for object %q, group: %s, resource: %s", admissionSpec.Operation, admissionSpec.Object, admissionSpec.Resource.Group, admissionSpec.Resource.Resource)

	// only validate the request for authrealm
	if !strings.HasSuffix(admissionSpec.Resource.Group, GROUP_SUFFIX) {
		status.Allowed = true
		return status
	}

	switch admissionSpec.Resource.Resource {
	case "computes":
		return a.ValidateComputes(admissionSpec)
	case "computeconfigs":
		return a.ValidateComputeConfigs(admissionSpec)

	}
	status.Allowed = true
	return status
}

func (a *ComputesAdmissionHook) ValidateComputes(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}

	compute := &computesv1alpha1.Compute{}

	err := json.Unmarshal(admissionSpec.Object.Raw, compute)
	if err != nil {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
			Message: err.Error(),
		}
		return status
	}

	klog.V(4).Infof("Validate webhook for RegisteredCluster name: %s, namespace: %s", compute.Name, compute.Namespace)
	switch admissionSpec.Operation {
	case admissionv1beta1.Create:
		klog.V(4).Info("Validate Compute create")
		status.Allowed = true
		return status
	}
	status.Allowed = true
	return status
}

func (a *ComputesAdmissionHook) ValidateComputeConfigs(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}

	computeConfig := &computesv1alpha1.ComputeConfig{}

	err := json.Unmarshal(admissionSpec.Object.Raw, computeConfig)
	if err != nil {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
			Message: err.Error(),
		}
		return status
	}

	klog.V(4).Infof("Validate webhook for ComputeConfig name: %s", computeConfig.Name)

	status.Allowed = true
	return status

}

// Initialize is called by generic-admission-server on startup to setup initialization that webhook needs.
func (a *ComputesAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	klog.V(0).Infof("Initialize admission webhook for Compute")

	a.initialized = true

	shallowClientConfigCopy := *kubeClientConfig
	shallowClientConfigCopy.GroupVersion = &schema.GroupVersion{
		Group:   GROUP_SUFFIX,
		Version: "v1alpha1",
	}
	shallowClientConfigCopy.APIPath = "/apis"
	kubeClient, err := kubernetes.NewForConfig(&shallowClientConfigCopy)
	if err != nil {
		return err
	}
	a.KubeClient = kubeClient

	dynamicClient, err := dynamic.NewForConfig(&shallowClientConfigCopy)
	if err != nil {
		return err
	}
	a.Client = dynamicClient.Resource(schema.GroupVersionResource{
		Group:    GROUP_SUFFIX,
		Version:  "v1alpha1",
		Resource: "computes",
	})

	a.ClusterRegistrarClient = dynamicClient.Resource(schema.GroupVersionResource{
		Group:    GROUP_SUFFIX,
		Version:  "v1alpha1",
		Resource: "computeconfigs",
	})

	return nil
}
