// Copyright Red Hat

package workspace

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/stolostron/cluster-registration-operator/resources"

	giterrors "github.com/pkg/errors"
	"github.com/stolostron/cluster-registration-operator/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clusteradmapply "open-cluster-management.io/clusteradm/pkg/helpers/apply"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WorkspaceReconciler reconciles namespaces with workspace annotation
type WorkspaceReconciler struct {
	client.Client
	KubeClient         kubernetes.Interface
	DynamicClient      dynamic.Interface
	APIExtensionClient apiextensionsclient.Interface
	Log                logr.Logger
	Scheme             *runtime.Scheme
	MceClusters        []helpers.HubInstance
	// MceKubeClient         kubernetes.Interface
	// MceDynamicClient      dynamic.Interface
	// MceAPIExtensionClient apiextensionsclient.Interface
}

func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	logger.Info("Reconciling...")

	//TODO - handle delete

	if err := r.syncManagedClusterSet(req.Name, ctx); err != nil {
		logger.Error(err, "failed to sync ManagedClusterSet")
		//TODO - should we report a status on the namespace?
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) syncManagedClusterSet(name string, ctx context.Context) error {
	applierBuilder := &clusteradmapply.ApplierBuilder{}
	applier := applierBuilder.WithClient(r.MceClusters[0].KubeClient, r.MceClusters[0].APIExtensionClient, r.MceClusters[0].DynamicClient).Build() //TODO - support more than one
	readerDeploy := resources.GetScenarioResourcesReader()

	mcsName := helpers.ManagedClusterSetNameForWorkspace(name)

	files := []string{
		"workspace/managed_cluster_set.yaml",
	}

	values := struct {
		Name string
	}{
		Name: mcsName,
	}

	_, err := applier.ApplyCustomResources(readerDeploy, values, false, "", files...)
	if err != nil {
		return giterrors.WithStack(err)
	}
	return nil
}

func workspaceNamespacesPredicate() predicate.Predicate {
	f := func(obj client.Object) bool {
		log := ctrl.Log.WithName("controllers").WithName("workspace").WithName("workspaceNamespacesPredicate").WithValues("namespace", obj.GetNamespace(), "name", obj.GetName())
		if obj.GetLabels()["toolchain.dev.openshift.com/provider"] == "codeready-toolchain" {
			log.V(1).Info("process appstudio workspace")
			return true
		}
		// log.V(1).Info("not appstudio workspace... ignore")
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return f(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return f(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return f(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return f(event.Object)
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Log.Info("setup workspace manager")
	// clusterapiv1.AddToScheme(r.Scheme) //I think I don't need this..set in main
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}, builder.WithPredicates(workspaceNamespacesPredicate())). // only care about appstudio workspace namespaces
		Complete(r)
}
