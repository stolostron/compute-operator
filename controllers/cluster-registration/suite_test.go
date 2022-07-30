// Copyright Red Hat
package registeredcluster

import (
	"context"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	utilflag "k8s.io/component-base/cli/flag"

	"github.com/kcp-dev/logicalcluster"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/compute-operator/pkg/helpers"
	"github.com/stolostron/compute-operator/test"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	// the namespace on the compute
	workingClusterComputeNamespace string = "rc-ws"
	// The registered cluster name
	registeredClusterName string = "registered-cluster"

	controllerNamespace string = "controller-ns"
)

var (
	computeContext                      context.Context
	computeRuntimeWorkspaceClient       client.Client
	controllerRestConfig                *rest.Config
	apiExportVirtualWorkspaceKubeClient kubernetes.Interface
	virtualWorkspaceDynamicClient       dynamic.Interface
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// fetch the current config
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// adjust it
	suiteConfig.SkipStrings = []string{"NEVER-RUN"}
	reporterConfig.FullTrace = true
	RunSpecs(t,
		"Cluster Registration Suite",
		reporterConfig)
}

var _ = BeforeSuite(func() {
	var hubKubeconfigString string
	computeContext,
		computeRuntimeWorkspaceClient,
		apiExportVirtualWorkspaceKubeClient,
		virtualWorkspaceDynamicClient = test.SetupCompute(scheme,
		controllerNamespace,
		"../../build/")
	controllerRestConfig, hubKubeconfigString = test.SetupControllerEnvironment(scheme, controllerNamespace,
		[]string{
			filepath.Join("..", "..", "test", "config", "crd", "external"),
		})
	test.InitControllerEnvironment(scheme, controllerNamespace, controllerRestConfig, hubKubeconfigString)
	// Launch the compute-operator manager
	go func() {
		defer GinkgoRecover()
		klog.Info("run controller")

		Expect(os.Setenv("POD_NAMESPACE", controllerNamespace)).To(BeNil())

		rand.Seed(time.Now().UTC().UnixNano())
		pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
		klog.InitFlags(goflag.CommandLine)
		pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
		goflag.Set("v", "2")
		goflag.Parse()
		//TODO Customize Verbosity Level
		defer klog.Flush()
		logs.InitLogs()
		defer logs.FlushLogs()

		// setup kubeconfig
		pflag.CommandLine.Set("kubeconfig", test.TestEnvKubeconfigFile)

		managerLog := os.Getenv("MANAGER_LOG")
		cmd := NewManager()
		if len(managerLog) != 0 {
			cmd.SetArgs([]string{
				"--logtostderr=false",
				"--log-file=" + managerLog,
			})
		}
		err := cmd.Execute()
		Expect(err).To(BeNil())
	}()

})

var _ = AfterSuite(func() {
	test.TearDownCompute()
})

var _ = Describe("Process registeredCluster: ", func() {
	It("Process cluster-registration registeredCluster", func() {
		controllerRuntimeClient, err := client.New(controllerRestConfig, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(controllerRuntimeClient).ToNot(BeNil())

		By("Create the compute namespace", func() {
			Eventually(func() error {
				klog.Info("create namespace")
				cmd := exec.Command("kubectl",
					"create",
					"ns",
					workingClusterComputeNamespace,
					"--kubeconfig",
					test.AdminComputeKubeconfigFile)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					klog.Error(err, "while create ns")
				}
				return err
			}, 60, 3).Should(BeNil())
		})
		// create the registeredcluster on kcp workspace
		var registeredCluster *singaporev1alpha1.RegisteredCluster
		By("Create the RegisteredCluster", func() {
			Eventually(func() error {
				klog.Info("apply registeredCluster")
				cmd := exec.Command("kubectl",
					"apply",
					"-f",
					"../../test/resources/compute/registeredCluster.yaml",
					"--kubeconfig",
					test.AdminComputeKubeconfigFile,
				)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					klog.Error(err, "while applying registeredCluster")
				}
				return err
			}, 60, 3).Should(BeNil())
			registeredCluster = &singaporev1alpha1.RegisteredCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registeredClusterName,
					Namespace: workingClusterComputeNamespace,
				},
				Spec: singaporev1alpha1.RegisteredClusterSpec{
					Location: test.AbsoluteLocationWorkspace,
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
					klog.Info("Waiting managedCluster", "Error", err)
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

		// Check if the synctarget created in the location workspace
		By("Checking synctarget in location workspace", func() {
			Eventually(func() error {
				locationContext := logicalcluster.WithCluster(computeContext, logicalcluster.New(test.AbsoluteLocationWorkspace))
				locationClusterName, _ := logicalcluster.ClusterFromContext(locationContext)
				klog.Infof("getting synctarget in location workspace %s", test.AbsoluteLocationWorkspace)
				labels := RegisteredClusterNamelabel + "=" + registeredCluster.Name + "," + RegisteredClusterNamespacelabel + "=" + registeredCluster.Namespace + "," + RegisteredClusterWorkspace + "=" + strings.ReplaceAll(locationClusterName.String(), ":", "-")

				syncTargetList, err := virtualWorkspaceDynamicClient.Resource(clusterGVR).List(locationContext, metav1.ListOptions{
					LabelSelector: labels,
				})
				if err != nil {
					return err
				}

				if len(syncTargetList.Items) == 0 || len(syncTargetList.Items) > 1 {
					return fmt.Errorf("Synctarget not found in the location workspace")
				}
				klog.Infof("synctarget found in the location workspace %s", syncTargetList.Items)
				return nil
			}, 60, 10).Should(BeNil())
		})

		// Check if the service account was created in the location workspace
		By("Checking syncer service account in location workspace", func() {
			Eventually(func() error {
				locationContext := logicalcluster.WithCluster(computeContext, logicalcluster.New(test.AbsoluteLocationWorkspace))
				klog.Infof("getting service account %s in workspace %s", helpers.GetSyncerServiceAccountName(), test.AbsoluteLocationWorkspace)
				_, err := apiExportVirtualWorkspaceKubeClient.CoreV1().ServiceAccounts("default").Get(locationContext, helpers.GetSyncerServiceAccountName(), metav1.GetOptions{})
				if err != nil {
					klog.Errorf("failed getting service account %s", err)
				}
				return err
			}, 30, 10).Should(BeNil())
		})

		// Check if the manifestwork was created on the hub
		By("Checking manifestwork", func() {
			Eventually(func() error {
				manifestwork := &manifestworkv1.ManifestWork{}

				err := controllerRuntimeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      helpers.GetSyncerName(registeredCluster.Name),
						Namespace: managedCluster.Name,
					},
					manifestwork)
				if err != nil {
					klog.Info("Waiting manifestwork", "Error", err)
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
					Name:      helpers.GetSyncerName(registeredCluster.Name),
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
					klog.Info("While deleting registeredcluster", "Error", err)
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
