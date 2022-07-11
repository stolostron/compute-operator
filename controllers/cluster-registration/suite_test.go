// Copyright Red Hat
package registeredcluster

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ghodss/yaml"

	dynamicapimachinery "github.com/kcp-dev/apimachinery/pkg/dynamic"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	kcpclient "github.com/kcp-dev/apimachinery/pkg/client"
	"github.com/kcp-dev/logicalcluster"
	"k8s.io/apimachinery/pkg/runtime"
	clusteradmapply "open-cluster-management.io/clusteradm/pkg/helpers/apply"
	clusteradmasset "open-cluster-management.io/clusteradm/pkg/helpers/asset"

	croconfig "github.com/stolostron/compute-operator/config"
	"github.com/stolostron/compute-operator/hack"
	"github.com/stolostron/compute-operator/pkg/helpers"
	"github.com/stolostron/compute-operator/test"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	// The compute workspace
	workspace string = "my-ws"
	// The controller service account on the compute
	controllerComputeServiceAccount string = "compute-operator"
	// the namespace on the compute
	workingComputeNamespace string = "default"
	// The controller namespace
	controllerNamespace string = "controller-ns"
	// The compute organization
	computeOrganization string = "my-org"
	// The compute organization workspace
	organizationWorkspace string = "root:" + computeOrganization
	// The compute cluster workspace
	clusterWorkspace string = organizationWorkspace + ":" + workspace
	// The registered cluster name
	registeredClusterName string = "registered-cluster"
	// The compute kubeconfig file
	adminComputeKubeconfigFile string = ".kcp/admin.kubeconfig"
	// The directory for test environment assets
	testEnvDir string = ".testenv"
	// the test environment kubeconfig file
	testEnvKubeconfigFile string = testEnvDir + "/testenv.kubeconfig"
	// the main executable
	controllerExecutable string = testEnvDir + "/manager"
	// The service account compute kubeconfig file
	saComputeKubeconfigFile string = testEnvDir + "/kubeconfig-" + controllerComputeServiceAccount + ".yaml"

	// The compute kubeconfig secret name on the controller cluster
	computeKubeconfigSecret string = "kcp-kubeconfig"
)

var (
	controllerRestConfig             *rest.Config
	computeContext                   context.Context
	organizationContext              context.Context
	testEnv                          *envtest.Environment
	scheme                           = runtime.NewScheme()
	controllerManager                *exec.Cmd
	controllerRuntimeClient          client.Client
	computeServer                    *exec.Cmd
	computeAdminApplierBuilder       *clusteradmapply.ApplierBuilder
	organizationAdminApplierBuilder  *clusteradmapply.ApplierBuilder
	readerTest                       *clusteradmasset.ScenarioResourcesReader
	readerHack                       *clusteradmasset.ScenarioResourcesReader
	readerConfig                     *clusteradmasset.ScenarioResourcesReader
	saComputeKubeconfigFileAbs       string
	computeAdminKubconfigData        []byte
	computeOrganizationKubconfigData []byte
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// fetch the current config
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// adjust it
	suiteConfig.SkipStrings = []string{"NEVER-RUN"}
	reporterConfig.FullTrace = true
	RunSpecs(t,
		"Controller Suite",
		reporterConfig)
}

var _ = BeforeSuite(func() {
	logf.SetLogger(klog.NewKlogr())

	// Generate readers for appliers
	readerTest = test.GetScenarioResourcesReader()
	readerHack = hack.GetScenarioResourcesReader()
	readerConfig = croconfig.GetScenarioResourcesReader()

	By("bootstrapping test environment")
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = appsv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = clusterapiv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = singaporev1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = addonv1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = authv1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = manifestworkv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = apisv1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())

	// Get the CRDs
	clusterRegistrarsCRD, err := getCRD(readerConfig, "crd/singapore.open-cluster-management.io_clusterregistrars.yaml")
	Expect(err).Should(BeNil())

	hubConfigsCRD, err := getCRD(readerConfig, "crd/singapore.open-cluster-management.io_hubconfigs.yaml")
	Expect(err).Should(BeNil())

	registeredClustersCRD, err := getCRD(readerConfig, "crd/singapore.open-cluster-management.io_registeredclusters.yaml")
	Expect(err).Should(BeNil())

	// set useExistingCluster, if set to true then the cluster with
	// the $KUBECONFIG will be used as target instead of the in memory envtest
	useExistingClusterEnvVar := os.Getenv("USE_EXISTING_CLUSTER")
	var existingCluster bool
	if len(useExistingClusterEnvVar) != 0 {
		existingCluster, err = strconv.ParseBool(useExistingClusterEnvVar)
		Expect(err).To(BeNil())
	}

	testEnv = &envtest.Environment{
		Scheme: scheme,
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			clusterRegistrarsCRD,
			hubConfigsCRD,
			registeredClustersCRD,
		},
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "test", "config", "crd", "external"),
		},
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: false,
		ControlPlaneStartTimeout: 1 * time.Minute,
		ControlPlaneStopTimeout:  1 * time.Minute,
		UseExistingCluster:       &existingCluster,
	}

	// Clean testEnv Directory
	os.RemoveAll(testEnvDir)
	err = os.MkdirAll(testEnvDir, 0700)
	Expect(err).To(BeNil())

	// Set and save the testEnv.Config if using an existing cluster
	var hubKubeconfigString string
	var hubKubeconfig *rest.Config
	if *testEnv.UseExistingCluster {
		hubKubeconfigString, hubKubeconfig, err = persistAndGetRestConfig(*testEnv.UseExistingCluster)
		Expect(err).ToNot(HaveOccurred())
		testEnv.Config = hubKubeconfig
	} else {
		os.Setenv("KUBECONFIG", "")
	}

	// Start the testEnv.
	controllerRestConfig, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(controllerRestConfig).ToNot(BeNil())

	// Save the testenv kubeconfig
	if !*testEnv.UseExistingCluster {
		hubKubeconfigString, _, err = persistAndGetRestConfig(*testEnv.UseExistingCluster)
		Expect(err).ToNot(HaveOccurred())
	}

	// Clean kcp
	os.RemoveAll(".kcp")

	// Launch KCP
	go func() {
		defer GinkgoRecover()
		adminComputeKubeconfigFile, err := filepath.Abs(adminComputeKubeconfigFile)
		Expect(err).To(BeNil())
		os.Setenv("KUBECONFIG", adminComputeKubeconfigFile)
		computeServer = exec.Command("kcp",
			"start",
		)

		// Create io.writer for kcp log
		kcpLogFile := os.Getenv("KCP_LOG")
		if len(kcpLogFile) == 0 {
			computeServer.Stdout = os.Stdout
			computeServer.Stderr = os.Stderr
		} else {
			os.MkdirAll(filepath.Dir(filepath.Clean(kcpLogFile)), 0700)
			f, err := os.OpenFile(filepath.Clean(kcpLogFile), os.O_WRONLY|os.O_CREATE, 0600)
			Expect(err).To(BeNil())
			defer f.Close()
			computeServer.Stdout = f
			computeServer.Stderr = f
		}

		err = computeServer.Start()
		Expect(err).To(BeNil())
	}()

	// Switch to system:admin context in order to create a kubeconfig allowing KCP API configuration.
	By("switch context system:admin", func() {
		Eventually(func() error {
			logf.Log.Info("switch context")
			cmd := exec.Command("kubectl",
				"config",
				"use-context",
				"system:admin")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while switching context")
			}
			return err
		}, 30, 3).Should(BeNil())
	})

	By("reading the kcpkubeconfig", func() {
		Eventually(func() error {
			computeAdminKubconfigData, err = ioutil.ReadFile(adminComputeKubeconfigFile)
			return err
		}, 60, 3).Should(BeNil())
	})

	computeAdminRestConfig, err := clientcmd.RESTConfigFromKubeConfig(computeAdminKubconfigData)
	Expect(err).ToNot(HaveOccurred())

	// Create the kcp clients for the builder
	computeAdminKubernetesClient := kubernetes.NewForConfigOrDie(computeAdminRestConfig)
	computeAdminAPIExtensionClient := apiextensionsclient.NewForConfigOrDie(computeAdminRestConfig)
	computeAdminDynamicClient, err := dynamicapimachinery.NewClusterDynamicClientForConfig(computeAdminRestConfig)
	Expect(err).ToNot(HaveOccurred())

	// Create a builder for the workspace
	computeAdminApplierBuilder = clusteradmapply.NewApplierBuilder().
		WithClient(computeAdminKubernetesClient,
			computeAdminAPIExtensionClient,
			computeAdminDynamicClient.Cluster(logicalcluster.New(clusterWorkspace)))
	// Create a builder for the organization
	organizationAdminApplierBuilder = clusteradmapply.NewApplierBuilder().
		WithClient(computeAdminKubernetesClient,
			computeAdminAPIExtensionClient,
			computeAdminDynamicClient.Cluster(logicalcluster.New(organizationWorkspace)))

	// Switch to root in order to create the organization workspace
	By("switch context root", func() {
		Eventually(func() error {
			logf.Log.Info("switch context")
			cmd := exec.Command("kubectl",
				"config",
				"use-context",
				"root")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while switching context")
			}
			return err
		}, 30, 3).Should(BeNil())
	})

	// Create workspace on compute server and enter in the ws
	By(fmt.Sprintf("creation of organization %s", computeOrganization), func() {
		Eventually(func() error {
			logf.Log.Info("create workspace")
			cmd := exec.Command("kubectl-kcp",
				"ws",
				"create",
				computeOrganization,
				"--enter")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while create organization")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Save the kubeconfig for the organization
	computeOrganizationKubconfigData, err = ioutil.ReadFile(adminComputeKubeconfigFile)
	Expect(err).ToNot(HaveOccurred())

	organizationContext = kcpclient.WithCluster(context.Background(), logicalcluster.New(organizationWorkspace))

	//Build compute admin applier
	By(fmt.Sprintf("apply resourceschema on workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create resourceschema")
			computeApplier := organizationAdminApplierBuilder.Build()
			files := []string{
				"apiresourceschema/singapore.open-cluster-management.io_registeredclusters.yaml",
			}
			_, err := computeApplier.ApplyCustomResources(readerConfig, nil, false, "", files...)
			if err != nil {
				logf.Log.Error(err, "while create role binding")
			}
			if err != nil {
				logf.Log.Error(err, "while applying resourceschema")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	By(fmt.Sprintf("apply APIExport on workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create APIExport")
			computeApplier := organizationAdminApplierBuilder.Build()
			files := []string{
				"compute/apiexport.yaml",
			}
			_, err := computeApplier.ApplyCustomResources(readerHack, nil, false, "", files...)
			if err != nil {
				logf.Log.Error(err, "while applying apiexport")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create SA on compute server in workspace
	By(fmt.Sprintf("creation of SA %s in workspace %s", controllerComputeServiceAccount, workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create service account")
			cmd := exec.Command("kubectl",
				"create",
				"serviceaccount",
				controllerComputeServiceAccount,
				"-n",
				workingComputeNamespace)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while create managerServiceAccount")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Generate the kubeconfig for the SA
	saComputeKubeconfigFileAbs, err = filepath.Abs(saComputeKubeconfigFile)
	Expect(err).To(BeNil())

	By(fmt.Sprintf("generate kubeconfig for sa %s in workspace %s", controllerComputeServiceAccount, workspace), func() {
		Eventually(func() error {
			logf.Log.Info(saComputeKubeconfigFile)
			adminComputeKubeconfigFile, err := filepath.Abs(adminComputeKubeconfigFile)
			Expect(err).To(BeNil())
			os.Setenv("KUBECONFIG", adminComputeKubeconfigFile)
			cmd := exec.Command("../../build/generate_kubeconfig_from_sa.sh",
				controllerComputeServiceAccount,
				workingComputeNamespace,
				saComputeKubeconfigFileAbs)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while generating sa kubeconfig")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	computeContext = kcpclient.WithCluster(context.Background(), logicalcluster.New(clusterWorkspace))

	// Create role for on compute server in workspace
	By(fmt.Sprintf("creation of role in workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create role")
			computeApplier := computeAdminApplierBuilder.
				WithContext(computeContext).Build()
			files := []string{
				"compute/role.yaml",
			}
			_, err := computeApplier.ApplyDirectly(readerHack, nil, false, "", files...)
			if err != nil {
				logf.Log.Error(err, "while create role")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create rolebinding for on compute server in workspace
	By(fmt.Sprintf("creation of rolebinding in workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create role binding")
			computeApplier := computeAdminApplierBuilder.
				WithContext(computeContext).Build()
			files := []string{
				"compute/role_binding.yaml",
			}
			_, err := computeApplier.ApplyDirectly(readerHack, nil, false, "", files...)
			if err != nil {
				logf.Log.Error(err, "while create role binding")
			}
			if err != nil {
				logf.Log.Error(err, "while create role binding")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create kcp runtime client for the controller
	computeSAKubconfigData, err := ioutil.ReadFile(saComputeKubeconfigFile)
	Expect(err).ToNot(HaveOccurred())
	computeRestSAConfig, err := clientcmd.RESTConfigFromKubeConfig(computeSAKubconfigData)
	Expect(err).ToNot(HaveOccurred())
	By("waiting virtualworkspace", func() {
		Eventually(func() error {
			logf.Log.Info("waiting virtual workspace")
			_, err = helpers.RestConfigForAPIExport(context.TODO(), computeRestSAConfig, "compute-apis", scheme)
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create workspace on compute server and enter in the ws
	By(fmt.Sprintf("creation of cluster workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create workspace")
			cmd := exec.Command("kubectl-kcp",
				"ws",
				"create",
				workspace,
				"--enter")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				logf.Log.Error(err, "while create cluster workspace")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create the APIBinding in the cluster workspace
	By(fmt.Sprintf("apply APIBinding on workspace %s", workspace), func() {
		Eventually(func() error {
			logf.Log.Info("create APIBinding")
			computeApplier := computeAdminApplierBuilder.
				WithContext(computeContext).Build()
			files := []string{
				"compute/apibinding.yaml",
			}
			// Values for the appliers
			values := struct {
				Organization string
			}{
				Organization: computeOrganization,
			}
			_, err := computeApplier.ApplyCustomResources(readerHack, values, false, "", files...)
			if err != nil {
				logf.Log.Error(err, "while applying APIBinding")
			}
			return err
		}, 60, 3).Should(BeNil())
	})

	// Create a runtime client to retrieve information from the hub cluster
	controllerRuntimeClient, err = client.New(controllerRestConfig, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(controllerRuntimeClient).ToNot(BeNil())

	// Create the controller namespace, that ns will hold the controller configuration.
	By(fmt.Sprintf("creation of namespace %s", controllerNamespace), func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: controllerNamespace,
			},
		}
		err := controllerRuntimeClient.Create(context.TODO(), ns)
		Expect(err).To(BeNil())
	})

	// Create the hub config secret for the controller
	By("Create a hubconfig secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hub-kube-config",
				Namespace: controllerNamespace,
			},
			Data: map[string][]byte{
				"kubeconfig": []byte(hubKubeconfigString),
			},
		}
		err = controllerRuntimeClient.Create(context.TODO(), secret)
		Expect(err).To(BeNil())
	})

	// Create a hubConfig CR with that secret
	var hubConfig *singaporev1alpha1.HubConfig
	By("Create a HubConfig", func() {
		hubConfig = &singaporev1alpha1.HubConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hubconfig",
				Namespace: controllerNamespace,
			},
			Spec: singaporev1alpha1.HubConfigSpec{
				KubeConfigSecretRef: corev1.LocalObjectReference{
					Name: "my-hub-kube-config",
				},
			},
		}
		err := controllerRuntimeClient.Create(context.TODO(), hubConfig)
		Expect(err).To(BeNil())
	})

	//Build compute admin applier
	By("Create a kcpconfig secret", func() {
		b, err := ioutil.ReadFile(saComputeKubeconfigFileAbs)
		Expect(err).To(BeNil())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      computeKubeconfigSecret,
				Namespace: controllerNamespace,
			},
			Data: map[string][]byte{
				"kubeconfig": b,
			},
		}
		err = controllerRuntimeClient.Create(context.TODO(), secret)
		Expect(err).To(BeNil())
	})

	// Create clients for the hub
	controllerKubernetesClient := kubernetes.NewForConfigOrDie(controllerRestConfig)
	controllerAPIExtensionClient := apiextensionsclient.NewForConfigOrDie(controllerRestConfig)
	controllerDynamicClient := dynamic.NewForConfigOrDie(controllerRestConfig)
	// Create the hub applier
	controllerApplierBuilder := clusteradmapply.NewApplierBuilder().
		WithClient(controllerKubernetesClient, controllerAPIExtensionClient, controllerDynamicClient)

	// Create the clusterRegistrar with the reference of the kcpConfig Secret
	By("Create the clusterRegistrar", func() {
		logf.Log.Info("apply clusterRegistrar")
		// Values for the appliers
		values := struct {
			KcpKubeconfigSecret string
		}{
			KcpKubeconfigSecret: computeKubeconfigSecret,
		}
		applier := controllerApplierBuilder.
			Build()
		files := []string{
			"resources/compute/clusterRegistrar.yaml",
		}

		_, err := applier.ApplyCustomResources(readerTest, values, false, "", files...)
		Expect(err).To(BeNil())
	})

	// Launch the compute-operator manager
	go func() {
		defer GinkgoRecover()
		logf.Log.Info("build controller")
		build := exec.Command("go",
			"build",
			"-o",
			controllerExecutable,
			"../../main.go")
		err := build.Run()
		Expect(err).To(BeNil())

		logf.Log.Info("run controller")

		os.Setenv("POD_NAME", "installer-pod")
		os.Setenv("POD_NAMESPACE", controllerNamespace)
		controllerManager = exec.Command(controllerExecutable,
			"manager",
			"--kubeconfig",
			testEnvKubeconfigFile,
			// "--v=6",
		)

		// Create io.writer for manager log
		managerLogFile := os.Getenv("MANAGER_LOG")
		if len(managerLogFile) == 0 {
			controllerManager.Stdout = os.Stdout
			controllerManager.Stderr = os.Stderr
		} else {
			os.MkdirAll(filepath.Dir(filepath.Clean(managerLogFile)), 0700)
			f, err := os.OpenFile(filepath.Clean(managerLogFile), os.O_WRONLY|os.O_CREATE, 0600)
			Expect(err).To(BeNil())
			defer f.Close()
			controllerManager.Stdout = f
			controllerManager.Stderr = f
		}
		err = controllerManager.Start()
		Expect(err).To(BeNil())
	}()

})

var _ = AfterSuite(func() {
	cleanup()
})

func cleanup() {
	// Kill the compute-operator managaer
	if controllerManager != nil {
		By("tearing down the manager")
		logf.Log.Info("Process", "Args", controllerManager.Args)
		controllerManager.Process.Signal(os.Interrupt)
		Eventually(func() error {
			if err := controllerManager.Process.Signal(os.Interrupt); err != nil {
				logf.Log.Error(err, "while tear down the manager")
				return err
			}
			return nil
		}, 60, 3).Should(BeNil())
		controllerManager.Process.Signal(os.Interrupt)
		// controllerManager.Wait()
	}
	// Kill KCP
	if computeServer != nil {
		By("tearing down the kcp")
		computeServer.Process.Signal(os.Interrupt)
		Eventually(func() error {
			if err := computeServer.Process.Signal(os.Interrupt); err != nil {
				logf.Log.Error(err, "while tear down the kcp")
				return err
			}
			return nil
		}, 60, 3).Should(BeNil())
		// computeServer.Wait()
	}
	if testEnv != nil {
		By("tearing down the test environment")
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
}

var _ = Describe("Process registeredCluster: ", func() {
	It("Process registeredCluster", func() {
		// create the registeredcluster on kcp workspace
		var registeredCluster *singaporev1alpha1.RegisteredCluster
		By("Create the RegisteredCluster", func() {
			Eventually(func() error {
				logf.Log.Info("apply registeredCluster")
				cmd := exec.Command("kubectl",
					"apply",
					"-f",
					"../../test/resources/compute/registeredCluster.yaml")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					logf.Log.Error(err, "while applying registeredCluster")
				}
				return err
			}, 60, 3).Should(BeNil())
			registeredCluster = &singaporev1alpha1.RegisteredCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registeredClusterName,
					Namespace: workingComputeNamespace,
				},
				Spec: singaporev1alpha1.RegisteredClusterSpec{
					Location: "FakeKcpLocation",
				},
			}
		})
		// Get the managedcluster for the registiredcluster
		// Searching by labels
		var managedCluster *clusterapiv1.ManagedCluster
		By("Checking managedCluster", func() {
			Eventually(func() error {
				managedClusters := &clusterapiv1.ManagedClusterList{}

				if err := controllerRuntimeClient.List(context.TODO(),
					managedClusters,
					client.MatchingLabels{
						RegisteredClusterNamelabel:      registeredCluster.Name,
						RegisteredClusterNamespacelabel: registeredCluster.Namespace,
					}); err != nil {
					logf.Log.Info("Waiting managedCluster", "Error", err)
					return err
				}
				if len(managedClusters.Items) != 1 {
					return fmt.Errorf("Number of managedCluster found %d", len(managedClusters.Items))
				}
				managedCluster = &managedClusters.Items[0]
				return nil
			}, 60, 3).Should(BeNil())
		})
		// As managedcluster controller is not running
		// patching the managedcluster with several information and status
		By("Patching managecluster spec", func() {
			managedCluster.Spec.ManagedClusterClientConfigs = []clusterapiv1.ClientConfig{
				{
					URL:      "https://example.com:443",
					CABundle: []byte("cabbundle"),
				},
			}
			err := controllerRuntimeClient.Update(context.TODO(), managedCluster)
			Expect(err).Should(BeNil())
		})
		By("Updating managedcluster label", func() {
			managedCluster.ObjectMeta.Labels["clusterID"] = "8bcc855c-259f-46fd-adda-485ef99f2438"
			err := controllerRuntimeClient.Update(context.TODO(), managedCluster)
			Expect(err).Should(BeNil())
		})
		By("Patching managedcluster status", func() {

			// patch := client.MergeFrom(managedCluster.DeepCopy())
			managedCluster.Status.Conditions = []metav1.Condition{
				{
					Type:               clusterapiv1.ManagedClusterConditionAvailable,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "Succeeded",
					Message:            "Managedcluster succeeded",
				},
				{
					Type:               clusterapiv1.ManagedClusterConditionJoined,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "Joined",
					Message:            "Managedcluster joined",
				},
			}
			managedCluster.Status.Allocatable = clusterapiv1.ResourceList{
				clusterapiv1.ResourceCPU:    *apiresource.NewQuantity(2, apiresource.DecimalSI),
				clusterapiv1.ResourceMemory: *apiresource.NewQuantity(2, apiresource.DecimalSI),
			}
			managedCluster.Status.Capacity = clusterapiv1.ResourceList{
				clusterapiv1.ResourceCPU:    *apiresource.NewQuantity(1, apiresource.DecimalSI),
				clusterapiv1.ResourceMemory: *apiresource.NewQuantity(1, apiresource.DecimalSI),
			}
			managedCluster.Status.Version.Kubernetes = "1.19.2"
			managedCluster.Status.ClusterClaims = []clusterapiv1.ManagedClusterClaim{
				{
					Name:  "registeredCluster",
					Value: registeredCluster.Name,
				},
			}
			err := controllerRuntimeClient.Status().Update(context.TODO(), managedCluster)
			Expect(err).Should(BeNil())
		})
		// As the managedcluster controller is not running, creating the managed cluster ns on the hub
		By("Create managedcluster namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: managedCluster.Name,
				},
			}
			err := controllerRuntimeClient.Create(context.TODO(), ns)
			Expect(err).To(BeNil())
		})
		// Define an fake import secret
		importSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedCluster.Name + "-import",
				Namespace: managedCluster.Name,
			},
			Data: map[string][]byte{
				"crds.yaml":        []byte("my-crds.yaml"),
				"crdsv1.yaml":      []byte("my-crdsv1.yaml"),
				"crdsv1beta1.yaml": []byte("my-crdsv1beta1.yaml"),
				"import.yaml":      []byte("my-import.yaml"),
			},
		}
		// Define the expected import command
		expectedImportCommand :=
			`echo "bXktY3Jkc3YxLnlhbWw=" | base64 --decode | kubectl apply -f - && ` +
				`sleep 2 && ` +
				`echo "bXktaW1wb3J0LnlhbWw=" | base64 --decode | kubectl apply -f -`
		// Create the fake import secret on the hub
		By("Create import secret", func() {
			err := controllerRuntimeClient.Create(context.TODO(), importSecret)
			Expect(err).To(BeNil())
		})

		// Create the runtime client for the cluster workspace in order to check the registedcluster on kcp
		computeWorkspaceKubconfigData, err := ioutil.ReadFile(adminComputeKubeconfigFile)
		Expect(err).ToNot(HaveOccurred())
		computeRestWorkspaceConfig, err := clientcmd.RESTConfigFromKubeConfig(computeWorkspaceKubconfigData)
		Expect(err).ToNot(HaveOccurred())
		computeRuntimeWorkspaceClient, err := client.New(computeRestWorkspaceConfig, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())

		// Check if the registeredcluster has the importCommandRef set
		By("Checking registeredCluster ImportCommandRef", func() {
			Eventually(func() error {
				err := computeRuntimeWorkspaceClient.Get(computeContext,
					types.NamespacedName{
						Name:      registeredCluster.Name,
						Namespace: registeredCluster.Namespace,
					},
					registeredCluster)
				if err != nil {
					return err
				}
				if registeredCluster.Status.ImportCommandRef.Name != registeredCluster.Name+"-import" {
					return fmt.Errorf("Get %s instead of %s",
						registeredCluster.Status.ImportCommandRef.Name,
						registeredCluster.Name+"-import")
				}
				return nil
			}, 30, 1).Should(BeNil())
		})

		// Retrieve the configmap in the cluster workspace holding the import command
		// and check if the import command is as expected.
		secret := &corev1.Secret{}
		By("Checking import secret", func() {
			Eventually(func() error {
				err := computeRuntimeWorkspaceClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      registeredCluster.Status.ImportCommandRef.Name,
						Namespace: registeredCluster.Namespace,
					},
					secret)
				if err != nil {
					return err
				}
				gotImportCommand := string(secret.Data["importCommand"])
				if gotImportCommand != expectedImportCommand {
					return fmt.Errorf("invalid import expect %s, got %s", expectedImportCommand, secret.Data["importCommand"])
				}
				return nil
			}, 30, 1).Should(BeNil())
		})
		// Check the registeredclsuter status
		By("Checking registeredCluster status", func() {
			Eventually(func() error {
				err := computeRuntimeWorkspaceClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      registeredCluster.Name,
						Namespace: registeredCluster.Namespace,
					},
					registeredCluster)
				if err != nil {
					return err
				}

				if len(registeredCluster.Status.Conditions) == 0 {
					return fmt.Errorf("Expecting 1 condtions got 0")
				}
				if q, ok := registeredCluster.Status.Allocatable[clusterapiv1.ResourceCPU]; !ok {
					return fmt.Errorf("Expecting Allocatable ResourceCPU exists")
				} else {
					if v, ok := q.AsInt64(); !ok || v != 2 {
						return fmt.Errorf("Expecting Allocatable ResourceCPU equal 2, got %d", v)
					}
				}
				if q, ok := registeredCluster.Status.Capacity[clusterapiv1.ResourceCPU]; !ok {
					return fmt.Errorf("Expecting Allocatable ResourceCPU exists")
				} else {
					if v, ok := q.AsInt64(); !ok || v != 1 {
						return fmt.Errorf("Expecting Allocatable ResourceCPU equal 1, got %d", v)
					}
				}
				if registeredCluster.Status.Version.Kubernetes != "1.19.2" {
					return fmt.Errorf("Expecting Version 1.19.2, got %s", registeredCluster.Status.Version)
				}
				if len(registeredCluster.Status.ClusterClaims) != 1 {
					return fmt.Errorf("Expecting 1 ClusterClaim got 0")
				}
				if registeredCluster.Status.ClusterID == "" {
					return fmt.Errorf("Expecting clusterID to be not empty")
				}
				return nil
			}, 60, 1).Should(BeNil())
		})
		// Check if the managedclusteraddon was created on the hub
		By("Checking managedclusteraddon", func() {
			Eventually(func() error {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}

				if err := controllerRuntimeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      ManagedClusterAddOnName,
						Namespace: managedCluster.Name,
					},
					managedClusterAddon); err != nil {
					logf.Log.Info("Waiting managedClusteraddon", "Error", err)
					return err
				}
				return nil
			}, 30, 1).Should(BeNil())
		})

		// Check if the managedserviceaccount was created on the hub
		By("Checking managedserviceaccount", func() {
			Eventually(func() error {
				managed := &authv1alpha1.ManagedServiceAccount{}

				if err := controllerRuntimeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      ManagedServiceAccountName,
						Namespace: managedCluster.Name,
					},
					managed); err != nil {
					logf.Log.Info("Waiting managedserviceaccount", "Error", err)
					return err
				}
				return nil
			}, 30, 1).Should(BeNil())
		})

		// Check if the manifestwork was created on the hub
		By("Checking manifestwork", func() {
			Eventually(func() error {
				manifestwork := &manifestworkv1.ManifestWork{}

				err := controllerRuntimeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      ManagedServiceAccountName,
						Namespace: managedCluster.Name,
					},
					manifestwork)
				if err != nil {
					logf.Log.Info("Waiting manifestwork", "Error", err)
					return err
				}
				return nil
			}, 60, 5).Should(BeNil())
		})

		// As the manifestwork controller is not installed, patch the manifestwork
		By("Patching manifestwork status", func() {

			manifestwork := &manifestworkv1.ManifestWork{}

			err := controllerRuntimeClient.Get(context.TODO(),
				types.NamespacedName{
					Name:      ManagedServiceAccountName,
					Namespace: managedCluster.Name,
				},
				manifestwork)
			Expect(err).Should(BeNil())

			manifestwork.Status.Conditions = []metav1.Condition{
				{
					Type:               manifestworkv1.WorkApplied,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "Applied",
					Message:            "Manifestwork applied",
				},
			}
			err = controllerRuntimeClient.Update(context.TODO(), manifestwork)
			Expect(err).Should(BeNil())
		})

		// Create a managedserviceaccoutn secret
		By("Create managedserviceaccount secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ManagedServiceAccountName,
					Namespace: managedCluster.Name,
				},
				Data: map[string][]byte{
					"token":  []byte("token"),
					"ca.crt": []byte("ca-cert"),
				},
			}
			err := controllerRuntimeClient.Create(context.TODO(), secret)
			Expect(err).To(BeNil())
		})

		// Delete the registeredcluster
		By("Deleting registeredcluster", func() {
			Eventually(func() error {
				err := computeRuntimeWorkspaceClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      registeredCluster.Name,
						Namespace: registeredCluster.Namespace,
					},
					registeredCluster)
				if err != nil {
					return err
				}

				if err := computeRuntimeWorkspaceClient.Delete(context.TODO(),
					registeredCluster); err != nil {
					logf.Log.Info("While deleting registeredcluster", "Error", err)
					return err
				}
				return nil
			}, 60, 1).Should(BeNil())
		})

		// Check if the registeredcluster is well deleted
		By("Check registeredcluster deletion", func() {
			Eventually(func() error {
				err := computeRuntimeWorkspaceClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      registeredCluster.Name,
						Namespace: registeredCluster.Namespace,
					},
					registeredCluster)
				switch {
				case err == nil:
					return fmt.Errorf("registeredCluster still exists %s/%s", registeredCluster.Namespace, registeredCluster.Name)
				case errors.IsNotFound(err):
					return nil
				default:
					return err
				}
			}, 60, 1).Should(BeNil())
		})

	})

})

func getCRD(reader *clusteradmasset.ScenarioResourcesReader, file string) (*apiextensionsv1.CustomResourceDefinition, error) {
	b, err := reader.Asset(file)
	if err != nil {
		return nil, err
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(b, crd); err != nil {
		return nil, err
	}
	return crd, nil
}

func persistAndGetRestConfig(useExistingCluster bool) (string, *rest.Config, error) {
	var err error
	buf := new(strings.Builder)
	if useExistingCluster {
		cmd := exec.Command("kubectl", "config", "view", "--raw")
		cmd.Stdout = buf
		cmd.Stderr = buf
		err = cmd.Run()
	} else {
		adminInfo := envtest.User{Name: "admin", Groups: []string{"system:masters"}}
		authenticatedUser, err := testEnv.AddUser(adminInfo, testEnv.Config)
		Expect(err).To(BeNil())
		kubectl, err := authenticatedUser.Kubectl()
		Expect(err).To(BeNil())
		var out io.Reader
		out, _, err = kubectl.Run("config", "view", "--raw")
		Expect(err).To(BeNil())
		_, err = io.Copy(buf, out)
		Expect(err).To(BeNil())
	}
	if err != nil {
		return "", nil, err
	}
	if err := ioutil.WriteFile(testEnvKubeconfigFile, []byte(buf.String()), 0644); err != nil {
		return "", nil, err
	}

	hubKubconfigData, err := ioutil.ReadFile(testEnvKubeconfigFile)
	if err != nil {
		return "", nil, err
	}
	hubKubeconfig, err := clientcmd.RESTConfigFromKubeConfig(hubKubconfigData)
	if err != nil {
		return "", nil, err
	}
	return buf.String(), hubKubeconfig, err
}
