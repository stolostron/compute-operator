// Copyright Red Hat

package installer

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"

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

	// +kubebuilder:scaffold:scheme
}

func NewInstaller() *cobra.Command {
	o := &installerOptions{}
	cmd := &cobra.Command{
		Use:   "installer",
		Short: "installer for cluster-registration-operator",
		Run: func(cmd *cobra.Command, args []string) {
			//o.run()
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
