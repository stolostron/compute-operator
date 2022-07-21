// Copyright Red Hat

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"os"

	admissionserver "github.com/openshift/generic-admission-server/pkg/cmd/server"
	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"

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

type RegisteredClusterAdmissionHook struct {
	Client                 dynamic.ResourceInterface
	ClusterRegistrarClient dynamic.ResourceInterface
	KubeClient             kubernetes.Interface
	lock                   sync.RWMutex
	initialized            bool
}

func NewAdmissionHook() *cobra.Command {
	o := admissionserver.NewAdmissionServerOptions(os.Stdout, os.Stderr, &RegisteredClusterAdmissionHook{})

	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Start Cluster Admission Server",
		RunE: func(c *cobra.Command, args []string) error {
			stopCh := genericapiserver.SetupSignalHandler()

			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunAdmissionServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	o.RecommendedOptions.AddFlags(cmd.Flags())

	return cmd
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
func (a *RegisteredClusterAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "admission." + GROUP_SUFFIX,
			Version:  "v1alpha1",
			Resource: "registeredclustervalidators",
		},
		"registeredclustervalidators"
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
func (a *RegisteredClusterAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}
	klog.V(4).Infof("RegisteredCluster Validate %q operation for object %q, group: %s, resource: %s", admissionSpec.Operation, admissionSpec.Object, admissionSpec.Resource.Group, admissionSpec.Resource.Resource)

	// only validate the request for authrealm
	if !strings.HasSuffix(admissionSpec.Resource.Group, GROUP_SUFFIX) {
		status.Allowed = true
		return status
	}

	switch admissionSpec.Resource.Resource {
	case "registeredclusters":
		return a.ValidateRegisteredCluster(admissionSpec)
	case "clusterregistrars":
		return a.ValidateClusterRegistrar(admissionSpec)

	}
	status.Allowed = true
	return status
}

func (a *RegisteredClusterAdmissionHook) ValidateRegisteredCluster(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}

	regCluster := &singaporev1alpha1.RegisteredCluster{}

	err := json.Unmarshal(admissionSpec.Object.Raw, regCluster)
	if err != nil {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
			Message: err.Error(),
		}
		return status
	}

	klog.V(4).Infof("Validate webhook for RegisteredCluster name: %s, namespace: %s", regCluster.Name, regCluster.Namespace)
	switch admissionSpec.Operation {
	case admissionv1beta1.Create:
		klog.V(4).Info("Validate RegisteredCluster create ")

		if len(regCluster.Name) > 50 {
			status.Allowed = false
			status.Result = &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusForbidden, Reason: metav1.StatusReasonForbidden,
				Message: "RegisteredCluster name is too long (max 50 characters)",
			}
			return status
		}

		status.Allowed = true
		return status
	}
	status.Allowed = true
	return status
}

func (a *RegisteredClusterAdmissionHook) ValidateClusterRegistrar(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	status := &admissionv1beta1.AdmissionResponse{}

	clusterRegistrar := &singaporev1alpha1.ClusterRegistrar{}

	err := json.Unmarshal(admissionSpec.Object.Raw, clusterRegistrar)
	if err != nil {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
			Message: err.Error(),
		}
		return status
	}

	klog.V(4).Infof("Validate webhook for ClusterRegistrar name: %s", clusterRegistrar.Name)

	l, err := a.ClusterRegistrarClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonInternalError,
			Message: err.Error(),
		}
		return status
	}
	if len(l.Items) > 0 {
		status.Allowed = false
		status.Result = &metav1.Status{
			Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonForbidden,
			Message: "a clusterregistrar custom resource already exists",
		}
		return status
	}
	status.Allowed = true
	return status

}

// Initialize is called by generic-admission-server on startup to setup initialization that webhook needs.
func (a *RegisteredClusterAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	klog.V(0).Infof("Initialize admission webhook for RegisteredCluster")

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
		Resource: "registeredclusters",
	})

	a.ClusterRegistrarClient = dynamicClient.Resource(schema.GroupVersionResource{
		Group:    GROUP_SUFFIX,
		Version:  "v1alpha1",
		Resource: "clusterregistrars",
	})

	return nil
}
