// Copyright Red Hat

package installer

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	kcpcache "github.com/kcp-dev/apimachinery/pkg/cache"
	// "k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	"github.com/stolostron/compute-operator/controllers/installer"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	// "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	// "sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/spf13/cobra"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type installerOptions struct {
	metricsAddr          string
	probeAddr            string
	enableLeaderElection bool
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = singaporev1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func NewInstaller() *cobra.Command {
	o := &installerOptions{}
	cmd := &cobra.Command{
		Use:   "installer",
		Short: "installer for compute-operator",
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

func (o *installerOptions) run() {

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Setup Installer")
	newCacheFunc := func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		opts.KeyFunction = kcpcache.ClusterAwareKeyFunc
		opts.Indexers = k8scache.Indexers{
			kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
			kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
		}
		return cache.New(config, opts)
	}

	opts := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     o.metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: o.probeAddr,
		LeaderElection:         o.enableLeaderElection,
		LeaderElectionID:       "installer.open-cluster-management.io",
		NewCache:               newCacheFunc,
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("Add Installer reconciler")

	if err = (&installer.ClusterRegistrarReconciler{
		Client:             mgr.GetClient(),
		KubeClient:         kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		DynamicClient:      dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		APIExtensionClient: apiextensionsclient.NewForConfigOrDie(ctrl.GetConfigOrDie()),
		Log:                ctrl.Log.WithName("controllers").WithName("Installer"),
		Scheme:             mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Installer")
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

	// +kubebuilder:scaffold:builder

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
