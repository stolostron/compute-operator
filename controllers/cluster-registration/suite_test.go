// Copyright Red Hat
package registeredcluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghodss/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"

	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/apimachinery/pkg/runtime"
	clusteradmapply "open-cluster-management.io/clusteradm/pkg/helpers/apply"
	clusteradmasset "open-cluster-management.io/clusteradm/pkg/helpers/asset"

	croconfig "github.com/stolostron/cluster-registration-operator/config"
	"github.com/stolostron/cluster-registration-operator/pkg/helpers"

	singaporev1alpha1 "github.com/stolostron/cluster-registration-operator/api/singapore/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	userNamespace      string = "user-namespace"
	managedClusterName string = ""
)

var (
	cfg       *rest.Config
	r         *RegisteredClusterReconciler
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	scheme    = runtime.NewScheme()
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
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

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

	readerIDP := croconfig.GetScenarioResourcesReader()
	clusterRegistrarsCRD, err := getCRD(readerIDP, "crd/singapore.open-cluster-management.io_clusterregistrars.yaml")
	Expect(err).Should(BeNil())

	hubConfigsCRD, err := getCRD(readerIDP, "crd/singapore.open-cluster-management.io_hubconfigs.yaml")
	Expect(err).Should(BeNil())

	registeredClustersCRD, err := getCRD(readerIDP, "crd/singapore.open-cluster-management.io_registeredclusters.yaml")
	Expect(err).Should(BeNil())

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
		AttachControlPlaneOutput: true,
		ControlPlaneStartTimeout: 1 * time.Minute,
		ControlPlaneStopTimeout:  1 * time.Minute,
	}

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	os.Setenv("POD_NAME", "installer-pod")
	os.Setenv("POD_NAMESPACE", userNamespace)

	adminInfo := envtest.User{Name: "admin", Groups: []string{"system:masters"}}
	authenticatedUser, err := testEnv.AddUser(adminInfo, cfg)
	Expect(err).To(BeNil())
	kubectl, err := authenticatedUser.Kubectl()
	Expect(err).To(BeNil())
	out, _, err := kubectl.Run("config", "view", "--raw")
	Expect(err).To(BeNil())
	buf := new(strings.Builder)
	_, err = io.Copy(buf, out)
	Expect(err).To(BeNil())

	By(fmt.Sprintf("creation of namespace %s", userNamespace), func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: userNamespace,
			},
		}
		err := k8sClient.Create(context.TODO(), ns)
		Expect(err).To(BeNil())
	})

	By("Create a hubconfig secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hub-kube-config",
				Namespace: userNamespace,
			},
			Data: map[string][]byte{
				"kubeconfig": []byte(buf.String()),
			},
		}
		err := k8sClient.Create(context.TODO(), secret)
		Expect(err).To(BeNil())
	})

	var hubConfig *singaporev1alpha1.HubConfig
	By("Create a HubConfig", func() {
		hubConfig = &singaporev1alpha1.HubConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-hubconfig",
				Namespace: userNamespace,
			},
			Spec: singaporev1alpha1.HubConfigSpec{
				KubeConfigSecretRef: corev1.LocalObjectReference{
					Name: "my-hub-kube-config",
				},
			},
		}
		err := k8sClient.Create(context.TODO(), hubConfig)
		Expect(err).To(BeNil())
	})

	By("Init the controller", func() {
		hubClusters, err := helpers.GetHubClusters(mgr)
		Expect(err).To(BeNil())
		kubeClient := kubernetes.NewForConfigOrDie(cfg)
		dynamicClient := dynamic.NewForConfigOrDie(cfg)
		apiExtensionClient := apiextensionsclient.NewForConfigOrDie(cfg)
		hubApplier := clusteradmapply.NewApplierBuilder().WithClient(kubeClient, apiExtensionClient, dynamicClient).Build()
		r = &RegisteredClusterReconciler{
			Client:             k8sClient,
			KubeClient:         kubernetes.NewForConfigOrDie(cfg),
			DynamicClient:      dynamic.NewForConfigOrDie(cfg),
			APIExtensionClient: apiextensionsclient.NewForConfigOrDie(cfg),
			Log:                logf.Log,
			Scheme:             scheme,
			HubClusters:        hubClusters,
			HubApplier:         hubApplier,
		}
		err = r.SetupWithManager(mgr, scheme)
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

var _ = Describe("Process registeredCluster: ", func() {
	It("Process registeredCluster", func() {
		var registeredCluster *singaporev1alpha1.RegisteredCluster
		By("Create the RegisteredCluster", func() {
			registeredCluster = &singaporev1alpha1.RegisteredCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "registered-cluster",
					Namespace: userNamespace,
				},
				Spec: singaporev1alpha1.RegisteredClusterSpec{},
			}
			err := k8sClient.Create(context.TODO(), registeredCluster)
			Expect(err).To(BeNil())
		})
		var managedCluster *clusterapiv1.ManagedCluster
		By("Checking managedCluster", func() {
			Eventually(func() error {
				managedClusters := &clusterapiv1.ManagedClusterList{}

				if err := k8sClient.List(context.TODO(),
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
		By("Patching manageecluster spec", func() {
			managedCluster.Spec.ManagedClusterClientConfigs = []clusterapiv1.ClientConfig{
				{
					URL:      "https://example.com:443",
					CABundle: []byte("cabbundle"),
				},
			}
			err := k8sClient.Update(context.TODO(), managedCluster)
			Expect(err).Should(BeNil())
		})
		By("Updating managedcluster label", func() {
			managedCluster.ObjectMeta.Labels["clusterID"] = "8bcc855c-259f-46fd-adda-485ef99f2438"
			err := k8sClient.Update(context.TODO(), managedCluster)
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
			err := k8sClient.Status().Update(context.TODO(), managedCluster)
			Expect(err).Should(BeNil())
		})
		By("Create managedcluster namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: managedCluster.Name,
				},
			}
			err := k8sClient.Create(context.TODO(), ns)
			Expect(err).To(BeNil())
		})
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
		By("Create import secret", func() {
			err := k8sClient.Create(context.TODO(), importSecret)
			Expect(err).To(BeNil())
		})
		By("Checking registeredCluster ImportCommandRef", func() {
			Eventually(func() error {
				err := k8sClient.Get(context.TODO(),
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
		cm := &corev1.ConfigMap{}
		importCommand :=
			`echo "bXktY3Jkc3YxLnlhbWw=" | base64 --decode | kubectl apply -f - && ` +
				`sleep 2 && ` +
				`echo "bXktaW1wb3J0LnlhbWw=" | base64 --decode | kubectl apply -f -
`

		By("Checking import configMap", func() {
			Eventually(func() error {
				err := k8sClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      registeredCluster.Status.ImportCommandRef.Name,
						Namespace: registeredCluster.Namespace,
					},
					cm)
				if err != nil {
					return err
				}
				if cm.Data["importCommand"] != importCommand {
					return fmt.Errorf("invalid import expect %s, got %s", importCommand, cm.Data["importCommand"])
				}
				return nil
			}, 30, 1).Should(BeNil())
		})
		By("Checking registeredCluster status", func() {
			Eventually(func() error {
				err := k8sClient.Get(context.TODO(),
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
		By("Checking managedclusteraddon", func() {
			Eventually(func() error {
				managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}

				if err := k8sClient.Get(context.TODO(),
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

		By("Checking managedserviceaccount", func() {
			Eventually(func() error {
				managed := &authv1alpha1.ManagedServiceAccount{}

				if err := k8sClient.Get(context.TODO(),
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

		By("Checking manifestwork", func() {
			Eventually(func() error {
				manifestwork := &manifestworkv1.ManifestWork{}

				err := k8sClient.Get(context.TODO(),
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

		By("Patching manifestwork status", func() {

			manifestwork := &manifestworkv1.ManifestWork{}

			err := k8sClient.Get(context.TODO(),
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
			err = k8sClient.Update(context.TODO(), manifestwork)
			Expect(err).Should(BeNil())
		})

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
			err := k8sClient.Create(context.TODO(), secret)
			Expect(err).To(BeNil())
		})

		By("Deleting registeredcluster", func() {
			Eventually(func() error {
				registeredCluster := &singaporev1alpha1.RegisteredCluster{}

				err := k8sClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      "registered-cluster",
						Namespace: userNamespace,
					},
					registeredCluster)
				if err != nil {
					return err
				}

				if err := k8sClient.Delete(context.TODO(),
					registeredCluster); err != nil {
					logf.Log.Info("Waiting deletion of registeredcluster", "Error", err)
					return err
				}
				return nil
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
