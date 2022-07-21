// Copyright Red Hat
package installer

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ghodss/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clusteradmasset "open-cluster-management.io/clusteradm/pkg/helpers/asset"

	croconfig "github.com/stolostron/compute-operator/config"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	installationNamespace string = "cluster-reg-config"
)

var (
	cfg       *rest.Config
	r         *ClusterRegistrarReconciler
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// fetch the current config
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// adjust it
	suiteConfig.SkipStrings = []string{"NEVER-RUN"}
	reporterConfig.FullTrace = true
	RunSpecs(t,
		"Installer Suite",
		reporterConfig)
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	err := appsv1.AddToScheme(kscheme.Scheme)
	Expect(err).Should(BeNil())
	// err = openshiftconfigv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	readerCROConfig := croconfig.GetScenarioResourcesReader()
	clusterRegistrarsCRD, err := getCRD(readerCROConfig, "crd/singapore.open-cluster-management.io_clusterregistrars.yaml")
	Expect(err).Should(BeNil())

	hubConfigsCRD, err := getCRD(readerCROConfig, "crd/singapore.open-cluster-management.io_hubconfigs.yaml")
	Expect(err).Should(BeNil())

	registeredClustersCRD, err := getCRD(readerCROConfig, "crd/singapore.open-cluster-management.io_registeredclusters.yaml")
	Expect(err).Should(BeNil())

	testEnv = &envtest.Environment{
		Scheme: kscheme.Scheme,
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			clusterRegistrarsCRD,
			hubConfigsCRD,
			registeredClustersCRD,
		},
		// CRDDirectoryPaths: []string{
		// 	filepath.Join("..", "..", "test", "config", "crd", "external"),
		// },
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: false,
		ControlPlaneStartTimeout: 1 * time.Minute,
		ControlPlaneStopTimeout:  1 * time.Minute,
	}

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: kscheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	Expect(os.Setenv("POD_NAME", "installer-pod")).To(BeNil())
	Expect(os.Setenv("POD_NAMESPACE", installationNamespace)).To(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: kscheme.Scheme,
	})

	By("Init the controller", func() {
		r = &ClusterRegistrarReconciler{
			Client:             k8sClient,
			KubeClient:         kubernetes.NewForConfigOrDie(cfg),
			DynamicClient:      dynamic.NewForConfigOrDie(cfg),
			APIExtensionClient: apiextensionsclient.NewForConfigOrDie(cfg),
			Log:                logf.Log,
			Scheme:             kscheme.Scheme,
		}
		err := r.SetupWithManager(mgr)
		Expect(err).To(BeNil())
	})

	go func() {
		defer GinkgoRecover()
		ctx, cancel = context.WithCancel(ctrl.SetupSignalHandler())
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Process installation: ", func() {
	It("Process ClusterRegistrar creation", func() {
		By(fmt.Sprintf("creation of installation namespace %s", installationNamespace), func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: installationNamespace,
				},
			}
			err := k8sClient.Create(context.TODO(), ns)
			Expect(err).To(BeNil())
		})
		By("Creating the installer pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "installer-pod",
					Namespace: installationNamespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "c1",
							Image: "cro-image",
						},
					},
				},
			}
			err := k8sClient.Create(context.TODO(), pod)
			Expect(err).To(BeNil())
		})
		By("Create the ClusterRegistrar", func() {
			clusterRegistrar := &singaporev1alpha1.ClusterRegistrar{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-registrar",
					Namespace: installationNamespace,
				},
				Spec: singaporev1alpha1.ClusterRegistrarSpec{},
			}
			err := k8sClient.Create(context.TODO(), clusterRegistrar)
			Expect(err).To(BeNil())
		})
		By("Checking manager deployment", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				if err := k8sClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      "compute-operator-manager",
						Namespace: installationNamespace,
					},
					deployment); err != nil {
					logf.Log.Info("Waiting deployment", "Error", err)
					return err
				}
				return nil
			}, 30, 1).Should(BeNil())
		})
	})

	It("Proccess ClusterRegistrar deletion", func() {
		By("Delete the ClusterRegistrar", func() {
			clusterRegistrar := &singaporev1alpha1.ClusterRegistrar{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-registrar",
					Namespace: installationNamespace,
				},
				Spec: singaporev1alpha1.ClusterRegistrarSpec{},
			}
			err := k8sClient.Delete(context.TODO(), clusterRegistrar)
			Expect(err).To(BeNil())
		})
		By("Checking manager undeployment", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				if err := k8sClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      "compute-operator-manager",
						Namespace: installationNamespace,
					},
					deployment); err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					logf.Log.Info("Waiting deployment", "Error", err)
					return err
				}
				return fmt.Errorf("deployment still exists")
			}, 30, 1).Should(BeNil())
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
