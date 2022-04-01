// Copyright Red Hat

package manager

import (
	"context"
	"errors"
	"os"

	singaporev1alpha1 "github.com/stolostron/cluster-registration-operator/api/singapore/v1alpha1"
	"github.com/stolostron/cluster-registration-operator/pkg/helpers"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clusterreg "github.com/stolostron/cluster-registration-operator/controllers/cluster-registration"
	"github.com/stolostron/cluster-registration-operator/controllers/workspace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
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
	_ = clusterapiv1.AddToScheme(scheme)
	_ = singaporev1alpha1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func NewManager() *cobra.Command {
	o := &managerOptions{}
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "manager for cluster-registration-operator",
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

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Setup Manager")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     o.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: o.probeAddr,
		LeaderElection:         o.enableLeaderElection,
		LeaderElectionID:       "628f2987.cluster-registratiion.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// add healthz/readyz check handler
	setupLog.Info("Add health check")
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add healthz check handler ")
		os.Exit(1)
	}

	setupLog.Info("Add ready check")
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add readyz check handler ")
		os.Exit(1)
	}

	setupLog.Info("Add RegisteredCluster reconciler")

	hubInstances, err := getHubClusters(mgr, scheme)
	if err != nil {
		setupLog.Error(err, "unable to retreive the hubClsuter", "controller", "Cluster Registration")
		os.Exit(1)
	}

	if err = (&clusterreg.RegisteredClusterReconciler{
		Client:             mgr.GetClient(),
		KubeClient:         kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		DynamicClient:      dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		APIExtensionClient: apiextensionsclient.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		Log:                ctrl.Log.WithName("controllers").WithName("RegistredCluster"),
		Scheme:             mgr.GetScheme(),
		HubClusters:        hubInstances,
	}).SetupWithManager(mgr, scheme); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster Registration")
		os.Exit(1)
	}

	setupLog.Info("Add workspace reconciler")

	if err = (&workspace.WorkspaceReconciler{
		Client:             mgr.GetClient(),
		KubeClient:         kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		DynamicClient:      dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		APIExtensionClient: apiextensionsclient.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		Log:                ctrl.Log.WithName("controllers").WithName("Workspace"),
		Scheme:             mgr.GetScheme(),
		MceClusters:        hubInstances,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "workspace")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func getHubClusters(mgr ctrl.Manager, scheme *runtime.Scheme) ([]helpers.HubInstance, error) {
	setupLog.Info("setup registeredCluster manager")
	setupLog.Info("create dynamic client")
	dynamicClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	setupLog.Info("create kube client")
	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	setupLog.Info("retrieve POD namespace")
	namespace := os.Getenv("POD_NAMESPACE")
	if len(namespace) == 0 {
		err := errors.New("POD_NAMESPACE not defined")
		return nil, err
	}

	gvr := schema.GroupVersionResource{Group: "singapore.open-cluster-management.io", Version: "v1alpha1", Resource: "hubconfigs"}

	setupLog.Info("retrieve list of hubConfig")
	hubConfigListU, err := dynamicClient.Resource(gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	setupLog.Info("nb of hubConfig unstructured found", "sze", len(hubConfigListU.Items))

	hubInstances := make([]helpers.HubInstance, 0)

	for _, hubConfigU := range hubConfigListU.Items {
		setupLog.Info("convert to hubConfig structure", "name", hubConfigU.GetName())
		hubConfig := &singaporev1alpha1.HubConfig{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(hubConfigU.Object, hubConfig); err != nil {
			return nil, err
		}

		setupLog.Info("get config secret", "name", hubConfig.Spec.KubeConfigSecretRef.Name)
		configSecret, err := kubeClient.CoreV1().Secrets(hubConfig.Namespace).Get(context.TODO(),
			hubConfig.Spec.KubeConfigSecretRef.Name,
			metav1.GetOptions{})
		if err != nil {
			setupLog.Error(err, "unable to read kubeconfig secret for MCE cluster",
				"HubConfig Name", hubConfig.GetName(),
				"HubConfig Secret Name", hubConfig.Spec.KubeConfigSecretRef.Name)
			return nil, err
		}

		setupLog.Info("generate hubKubeConfig")
		hubKubeconfig, err := clientcmd.RESTConfigFromKubeConfig(configSecret.Data["kubeConfig"])
		if err != nil {
			setupLog.Error(err, "unable to create REST config for MCE cluster")
			return nil, err
		}

		// Add MCE cluster
		hubCluster, err := cluster.New(hubKubeconfig,
			func(o *cluster.Options) {
				o.Scheme = scheme // Explicitly set the scheme which includes ManagedCluster
			},
		)
		if err != nil {
			setupLog.Error(err, "unable to setup MCE cluster")
			return nil, err
		}

		// Add MCE cluster to manager
		if err := mgr.Add(hubCluster); err != nil {
			setupLog.Error(err, "unable to add MCE cluster")
			return nil, err
		}

		hubInstance := helpers.HubInstance{
			HubConfig:          hubConfig,
			Cluster:            hubCluster,
			Client:             hubCluster.GetClient(),
			KubeClient:         kubernetes.NewForConfigOrDie(hubKubeconfig),
			DynamicClient:      dynamic.NewForConfigOrDie(hubKubeconfig),
			APIExtensionClient: apiextensionsclient.NewForConfigOrDie(hubKubeconfig),
		}

		hubInstances = append(hubInstances, hubInstance)
	}

	return hubInstances, nil
}
