// Copyright Red Hat

package manager

import (
	"context"
	"fmt"
	"os"

	giterrors "github.com/pkg/errors"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	"github.com/stolostron/compute-operator/pkg/helpers"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/kcp"

	clusterreg "github.com/stolostron/compute-operator/controllers/cluster-registration"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type managerOptions struct {
	metricsAddr          string
	probeAddr            string
	enableLeaderElection bool
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = singaporev1alpha1.AddToScheme(scheme)
	_ = clusterapiv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = addonv1alpha1.AddToScheme(scheme)
	_ = authv1alpha1.AddToScheme(scheme)
	_ = manifestworkv1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func NewManager() *cobra.Command {
	o := &managerOptions{}
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "manager for compute-operator",
		Run: func(cmd *cobra.Command, args []string) {
			o.run()
			os.Exit(1)
		},
	}
	cmd.Flags().StringVar(&o.metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&o.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&o.enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	return cmd
}

func (o *managerOptions) run() {

	// ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	ctrl.SetLogger(klog.NewKlogr())

	setupLog.Info("Setup Manager")

	// hub clients
	kubeClient := kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie())
	dynamicClient := dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie())
	// apiExtensionClient := apiextensionsclient.NewForConfigOrDie(ctrl.GetConfigOrDie())
	// hubApplierBuilder := clusteradmapply.NewApplierBuilder().WithClient(kubeClient, apiExtensionClient, dynamicClient).Build()

	// get the clusterRegistrar
	clusterRegistrarList, err := dynamicClient.Resource(helpers.GvrCR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to read the clusterRegistrar")
		os.Exit(1)
	}
	if len(clusterRegistrarList.Items) != 1 {
		setupLog.Error(fmt.Errorf("zero or more than one clusterRegistrar"), "zero or more than one clusterRegistrar")
		os.Exit(1)
	}

	// convert to cluster registrar
	clusterRegistrar := &singaporev1alpha1.ClusterRegistrar{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(clusterRegistrarList.Items[0].Object, clusterRegistrar)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to convert the clusterRegistrar")
		os.Exit(1)
	}

	// get Compute kubeconfig secret
	podNamespace := os.Getenv("POD_NAMESPACE")
	if len(podNamespace) == 0 {
		setupLog.Error(fmt.Errorf("POD_NAMESPACE not set"), "")
		os.Exit(1)
	}
	var computeKubeConfigSecretData []byte
	computeKubeConfigSecret, err := kubeClient.CoreV1().
		Secrets(podNamespace).
		Get(context.TODO(), clusterRegistrar.Spec.ComputeService.ComputeKubeconfigSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), fmt.Sprintf("unable to read the computeKubeconfigSecret: %s/%s",
			podNamespace,
			clusterRegistrar.Spec.ComputeService.ComputeKubeconfigSecretRef.Name))
		os.Exit(1)
	}

	computeKubeConfigSecretData, ok := computeKubeConfigSecret.Data["kubeconfig"]
	if !ok {
		setupLog.Error(giterrors.WithStack(err), "computeKubeConfigSecret secret missing kubeconfig data")
		os.Exit(1)
	}

	setupLog.Info("generate kubeConfigSecretData")
	computeKubeconfig, err := clientcmd.RESTConfigFromKubeConfig(computeKubeConfigSecretData)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to create REST config for MCE cluster")
		os.Exit(1)
	}

	opts := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     o.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: o.probeAddr,
		LeaderElection:         o.enableLeaderElection,
		// The leader must be created on the compute-operator cluster and not on the compute service
		LeaderElectionConfig: ctrl.GetConfigOrDie(),
		LeaderElectionID:     "628f2987.cluster-registration.io",
		// NewCache:             helpers.NewClusterAwareCacheFunc,
	}

	setupLog.Info("server u:", "computeKubeconfig.Host", computeKubeconfig.Host)
	cfg, err := helpers.RestConfigForAPIExport(context.TODO(), computeKubeconfig, "compute-operator", scheme)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "error looking up virtual workspace URL")
		os.Exit(1)
	}

	computeKubeClient, err := kubernetes.NewClusterForConfig(cfg)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "error creating kubernetes.ClusterClient up virtual workspace URL")
		os.Exit(1)
	}
	computeDynamicClient, err := dynamic.NewClusterForConfig(cfg)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "error creating dynamic.ClusterClient up virtual workspace URL")
		os.Exit(1)
	}
	computeApiExtensionClient, err := apiextensionsclient.NewClusterForConfig(cfg)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "error creating apiextensionsclient.ClusterClient up virtual workspace URL")
		os.Exit(1)
	}

	setupLog.Info("server url:", "cfg.Host", cfg.Host)
	mgr, err := kcp.NewClusterAwareManager(cfg, opts)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("server url:", "cfg.Host", cfg.Host)

	// add healthz/readyz check handler
	setupLog.Info("Add health check")
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to add healthz check handler ")
		os.Exit(1)
	}

	setupLog.Info("Add ready check")
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to add readyz check handler ")
		os.Exit(1)
	}

	setupLog.Info("Add RegisteredCluster reconciler")

	hubInstances, err := helpers.GetHubClusters(context.Background(), mgr, kubeClient, dynamicClient)
	if err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to retreive the hubCluster", "controller", "Cluster Registration")
		os.Exit(1)
	}
	if err = (&clusterreg.RegisteredClusterReconciler{
		Client:                    mgr.GetClient(),
		Log:                       ctrl.Log.WithName("controllers").WithName("RegistredCluster"),
		Scheme:                    scheme,
		HubClusters:               hubInstances,
		ComputeKubeClient:         computeKubeClient,
		ComputeDynamicClient:      computeDynamicClient,
		ComputeAPIExtensionClient: computeApiExtensionClient,
	}).SetupWithManager(mgr, scheme); err != nil {
		setupLog.Error(giterrors.WithStack(err), "unable to create controller", "controller", "Cluster Registration")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(giterrors.WithStack(err), "problem running manager")
		os.Exit(1)
	}

}

// restConfigForAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace.
func restConfigForAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string, scheme *runtime.Scheme) (*rest.Config, error) {
	if err := apisv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error adding apis.kcp.dev/v1alpha1 to scheme: %w", err)
	}

	apiExportClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	var apiExport apisv1alpha1.APIExport

	if err := apiExportClient.Get(ctx, client.ObjectKey{
		NamespacedName: types.NamespacedName{
			Name: apiExportName,
		},
	}, &apiExport); err != nil {
		return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
	}

	if len(apiExport.Status.VirtualWorkspaces) < 1 {
		return nil, fmt.Errorf("APIExport %q status.virtualWorkspaces is empty", apiExportName)
	}

	cfg = rest.CopyConfig(cfg)
	// TODO(ncdc): sharding support
	cfg.Host = apiExport.Status.VirtualWorkspaces[0].URL

	return cfg, nil
}
