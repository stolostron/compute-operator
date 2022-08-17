// Copyright Red Hat
package webhook

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	"github.com/stolostron/compute-operator/pkg/helpers"
	"github.com/stolostron/compute-operator/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	controllerNamespace string = "controller-ns"
)

var (
	computeContext                      context.Context
	controllerRestConfig                *rest.Config
	computeRuntimeWorkspaceClient       client.Client
	apiExportVirtualWorkspaceKubeClient kubernetes.Interface
	virtualWorkspaceDynamicClient       dynamic.Interface
	scheme                              = runtime.NewScheme()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// fetch the current config
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// adjust it
	suiteConfig.SkipStrings = []string{"NEVER-RUN"}
	reporterConfig.FullTrace = true
	RunSpecs(t,
		"Webhook Suite",
		reporterConfig)
}

var _ = BeforeSuite(func() {
	var hubKubeconfigString string

	computeContext,
		computeRuntimeWorkspaceClient,
		apiExportVirtualWorkspaceKubeClient,
		virtualWorkspaceDynamicClient = test.SetupCompute(scheme,
		controllerNamespace,
		"../build/")

	controllerRestConfig, hubKubeconfigString = test.SetupControllerEnvironment(scheme, controllerNamespace,
		[]string{
			filepath.Join("..", "test", "config", "crd", "external"),
		})

	test.InitControllerEnvironment(scheme, controllerNamespace, controllerRestConfig, hubKubeconfigString)

})

var _ = AfterSuite(func() {
	test.TearDownComputeAndHub()
})

var _ = Describe("Process clusterRegistrar: ", func() {
	// It will read the existing clusterRegistar, change the name and reapply
	It("Validate clusterRegistrar webhook", func() {
		// Read existing clusterRegistrar
		regCluster := &singaporev1alpha1.ClusterRegistrar{}
		controllerDynamicClient := dynamic.NewForConfigOrDie(test.TestEnv.Config)
		By("Read existing clusterRegistrar", func() {
			klog.Info("apply clusterRegistrar")

			uRegCluster, err := controllerDynamicClient.Resource(helpers.GvrCR).Get(context.TODO(), "cluster-reg", metav1.GetOptions{})
			Expect(err).To(BeNil())
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(uRegCluster.Object, regCluster)
			Expect(err).To(BeNil())

		})
		registeredClusterAdmissionHook := &RegisteredClusterAdmissionHook{}
		registeredClusterAdmissionHook.Initialize(test.TestEnv.Config, genericapiserver.SetupSignalHandler())
		regCluster.Name = "cluster-reg-2"
		regClusterJson, err := json.Marshal(regCluster)
		Expect(err).To(BeNil())
		admissionRequest := &admissionv1beta1.AdmissionRequest{
			Resource: metav1.GroupVersionResource(helpers.GvrCR),
			Object: runtime.RawExtension{
				Raw: regClusterJson,
			},
		}
		By("Validate new one", func() {
			admissionResponse := registeredClusterAdmissionHook.Validate(admissionRequest)
			Expect(admissionResponse.Allowed).To(BeFalse())
		})
		By("Deleting the existing one and validate new creation", func() {
			err = controllerDynamicClient.Resource(helpers.GvrCR).Delete(context.TODO(), "cluster-reg", metav1.DeleteOptions{})
			Expect(err).To(BeNil())
			admissionResponse := registeredClusterAdmissionHook.Validate(admissionRequest)
			Expect(admissionResponse.Allowed).To(BeTrue())
		})
	})
})
