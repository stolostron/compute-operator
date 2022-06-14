// Copyright Red Hat

package registeredcluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	giterrors "github.com/pkg/errors"

	b64 "encoding/base64"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	clusteradmapply "open-cluster-management.io/clusteradm/pkg/helpers/apply"

	// corev1 "k8s.io/api/core/v1"
	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	"github.com/stolostron/compute-operator/pkg/helpers"
	"github.com/stolostron/compute-operator/resources"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// +kubebuilder:rbac:groups="",resources={secrets},verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="singapore.open-cluster-management.io",resources={hubconfigs},verbs=get;list;watch
// +kubebuilder:rbac:groups="singapore.open-cluster-management.io",resources={registeredclusters},verbs=get;list;watch;create;update;delete

// +kubebuilder:rbac:groups="singapore.open-cluster-management.io",resources={registeredclusters/status},verbs=update;patch

// +kubebuilder:rbac:groups="coordination.k8s.io",resources={leases},verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups="";events.k8s.io,resources=events,verbs=create;update;patch

const (
	RegisteredClusterNamelabel      string = "registeredcluster.singapore.open-cluster-management.io/name"
	RegisteredClusterNamespacelabel string = "registeredcluster.singapore.open-cluster-management.io/namespace"
	ManagedClusterSetlabel          string = "cluster.open-cluster-management.io/clusterset"
	ManagedServiceAccountName       string = "appstudio"
	ManagedClusterAddOnName         string = "managed-serviceaccount"
)

// RegisteredClusterReconciler reconciles a RegisteredCluster object
type RegisteredClusterReconciler struct {
	client.Client
	KubeClient         kubernetes.Interface
	DynamicClient      dynamic.Interface
	APIExtensionClient apiextensionsclient.Interface
	HubApplier         clusteradmapply.Applier
	Log                logr.Logger
	Scheme             *runtime.Scheme
	HubClusters        []helpers.HubInstance
}

func (r *RegisteredClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	logger := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	logger.Info("Reconciling...")

	regCluster := &singaporev1alpha1.RegisteredCluster{}

	if err := r.Client.Get(
		context.TODO(),
		client.ObjectKey{
			NamespacedName: types.NamespacedName{Namespace: req.Namespace, Name: req.Name},
		},
		regCluster,
	); err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, giterrors.WithStack(err)
	}

	hubCluster, err := helpers.GetHubCluster(req.Namespace, r.HubClusters)
	if err != nil {
		logger.Error(err, "failed to get HubCluster for RegisteredCluster workspace")
		return ctrl.Result{}, err
	}

	controllerutil.AddFinalizer(regCluster, helpers.RegisteredClusterFinalizer)

	if err := r.Client.Update(context.TODO(), regCluster); err != nil {
		return ctrl.Result{}, giterrors.WithStack(err)
	}

	if regCluster.DeletionTimestamp == nil {
		// create managecluster on creation of registeredcluster CR
		if err := r.createManagedCluster(regCluster, &hubCluster, ctx); err != nil {
			logger.Error(err, "failed to create ManagedCluster")
			return ctrl.Result{}, err
		}
	}
	managedCluster, err := r.getManagedCluster(regCluster, &hubCluster)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Error(err, "failed to get ManagedCluster")
		return ctrl.Result{}, err
	}

	//if deletetimestamp then process deletion
	if regCluster.DeletionTimestamp != nil {
		if r, err := r.processRegclusterDeletion(regCluster, &managedCluster, &hubCluster); err != nil || r.Requeue {
			return r, err
		}
		controllerutil.RemoveFinalizer(regCluster, helpers.RegisteredClusterFinalizer)
		if err := r.Client.Update(context.TODO(), regCluster); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		return reconcile.Result{}, nil
	}

	// update status of registeredcluster - add import command
	if err := r.updateImportCommand(regCluster, &managedCluster, &hubCluster, ctx); err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
		}
		logger.Error(err, "failed to update import command")
		return ctrl.Result{}, err
	}

	// sync ManagedClusterAddOn, ManagedServiceAccount, ...
	if err := r.syncManagedServiceAccount(regCluster, &managedCluster, &hubCluster, ctx); err != nil {
		logger.Error(err, "failed to sync ManagedClusterAddOn, ManagedServiceAccount, ...")
		return ctrl.Result{}, err
	}

	// update status of registeredcluster
	if err := r.updateRegisteredClusterStatus(regCluster, &managedCluster, ctx); err != nil {
		logger.Error(err, "failed to update registered cluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RegisteredClusterReconciler) updateRegisteredClusterStatus(regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, ctx context.Context) error {

	patch := client.MergeFrom(regCluster.DeepCopy())
	if managedCluster.Status.Conditions != nil {
		regCluster.Status.Conditions = helpers.MergeStatusConditions(regCluster.Status.Conditions, managedCluster.Status.Conditions...)
	}
	if managedCluster.Status.Allocatable != nil {
		regCluster.Status.Allocatable = managedCluster.Status.Allocatable
	}
	if managedCluster.Status.Capacity != nil {
		regCluster.Status.Capacity = managedCluster.Status.Capacity
	}
	if managedCluster.Status.ClusterClaims != nil {
		regCluster.Status.ClusterClaims = managedCluster.Status.ClusterClaims
	}
	if managedCluster.Status.Version != (clusterapiv1.ManagedClusterVersion{}) {
		regCluster.Status.Version = managedCluster.Status.Version
	}
	if managedCluster.Spec.ManagedClusterClientConfigs != nil && len(managedCluster.Spec.ManagedClusterClientConfigs) > 0 {
		regCluster.Status.ApiURL = managedCluster.Spec.ManagedClusterClientConfigs[0].URL
	}
	if clusterID, ok := managedCluster.GetLabels()["clusterID"]; ok {
		regCluster.Status.ClusterID = clusterID
	}

	if err := r.Client.Status().Patch(ctx, regCluster, patch); err != nil {
		return err
	}

	return nil
}

func (r *RegisteredClusterReconciler) getManagedCluster(regCluster *singaporev1alpha1.RegisteredCluster, hubCluster *helpers.HubInstance) (clusterapiv1.ManagedCluster, error) {
	managedClusterList := &clusterapiv1.ManagedClusterList{}
	managedCluster := clusterapiv1.ManagedCluster{}
	if err := hubCluster.Client.List(context.Background(), managedClusterList, client.MatchingLabels{RegisteredClusterNamelabel: regCluster.Name, RegisteredClusterNamespacelabel: regCluster.Namespace}); err != nil {
		// Error reading the object - requeue the request.
		return managedCluster, err
	}

	if len(managedClusterList.Items) == 1 {
		return managedClusterList.Items[0], nil
	}

	if regCluster.DeletionTimestamp != nil {
		return managedCluster, nil
	}
	return managedCluster, fmt.Errorf("correct managedcluster not found")
}

func (r *RegisteredClusterReconciler) updateImportCommand(regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance, ctx context.Context) error {
	// get import secret from mce managecluster namespace
	importSecret := &corev1.Secret{}
	if err := hubCluster.Cluster.GetAPIReader().Get(ctx,
		client.ObjectKey{
			NamespacedName: types.NamespacedName{Namespace: managedCluster.Name, Name: managedCluster.Name + "-import"},
		}, importSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			return err
		}
		return giterrors.WithStack(err)
	}

	applierBuilder := &clusteradmapply.ApplierBuilder{}
	applier := applierBuilder.
		WithClient(r.KubeClient, r.APIExtensionClient, r.DynamicClient).
		WithOwner(regCluster, false, true, r.Scheme).
		Build()

	readerDeploy := resources.GetScenarioResourcesReader()

	files := []string{
		"cluster-registration/import_configmap.yaml",
	}

	// Get yaml representation of import command

	crdsv1Yaml, err := yaml.Marshal(importSecret.Data["crdsv1.yaml"])

	importYaml, err := yaml.Marshal(importSecret.Data["import.yaml"])

	importCommand := "echo \"" + strings.TrimSpace(string(crdsv1Yaml)) + "\" | base64 --decode | kubectl apply -f - && sleep 2 && echo \"" + strings.TrimSpace(string(importYaml)) + "\" | base64 --decode | kubectl apply -f -"

	values := struct {
		Name          string
		Namespace     string
		ImportCommand string
	}{
		Name:          regCluster.Name,
		Namespace:     regCluster.Namespace,
		ImportCommand: importCommand,
	}

	_, err = applier.ApplyDirectly(readerDeploy, values, false, "", files...)
	if err != nil {
		return giterrors.WithStack(err)
	}

	patch := client.MergeFrom(regCluster.DeepCopy())
	regCluster.Status.ImportCommandRef = corev1.LocalObjectReference{
		Name: regCluster.Name + "-import",
	}
	if err := r.Client.Status().Patch(ctx, regCluster, patch); err != nil {
		return err
	}

	return nil
}

func (r *RegisteredClusterReconciler) syncManagedServiceAccount(regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance, ctx context.Context) error {
	logger := r.Log.WithName("syncManagedServiceAccount").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "managed cluster name", managedCluster.Name)

	readerDeploy := resources.GetScenarioResourcesReader()

	files := []string{
		"cluster-registration/managed_cluster_addon.yaml",
		"cluster-registration/managed_service_account.yaml",
	}

	values := struct {
		ManagedClusterAddOnName         string
		ServiceAccountName              string
		Namespace                       string
		RegisteredClusterNameLabel      string
		RegisteredClusterNamespaceLabel string
		RegisteredClusterName           string
		RegisteredClusterNamespace      string
	}{
		ManagedClusterAddOnName:         ManagedClusterAddOnName,
		ServiceAccountName:              ManagedServiceAccountName,
		Namespace:                       managedCluster.Name,
		RegisteredClusterNameLabel:      RegisteredClusterNamelabel,
		RegisteredClusterNamespaceLabel: RegisteredClusterNamespacelabel,
		RegisteredClusterName:           regCluster.Name,
		RegisteredClusterNamespace:      regCluster.Namespace,
	}

	logger.V(1).Info("applying managedclusteraddon and managedserviceaccount")

	_, err := hubCluster.HubApplier.ApplyCustomResources(readerDeploy, values, false, "", files...)
	if err != nil {
		return giterrors.WithStack(err)
	}

	// If cluster has joined, sync the ManifestWork to create the roles and bindings for the service account
	if status, ok := helpers.GetConditionStatus(regCluster.Status.Conditions, clusterapiv1.ManagedClusterConditionJoined); ok && status == metav1.ConditionTrue {

		msa := &authv1alpha1.ManagedServiceAccount{}

		if err := hubCluster.Client.Get(
			context.TODO(),
			client.ObjectKey{
				NamespacedName: types.NamespacedName{Namespace: managedCluster.Name, Name: ManagedServiceAccountName},
			},
			msa,
		); err != nil {
			return giterrors.WithStack(err)
		}
		applierBuilder := clusteradmapply.NewApplierBuilder()
		applier := applierBuilder.
			WithClient(hubCluster.KubeClient, hubCluster.APIExtensionClient, hubCluster.DynamicClient).
			WithOwner(msa, true, true, hubCluster.Client.Scheme()).
			WithCache(hubCluster.HubApplier.GetCache()).
			Build()

		files = []string{
			"cluster-registration/service_account_roles.yaml",
		}
		_, err := applier.ApplyCustomResources(readerDeploy, values, false, "", files...)
		if err != nil {
			return giterrors.WithStack(err)
		}

		work := &manifestworkv1.ManifestWork{}

		err = hubCluster.Client.Get(ctx,
			client.ObjectKey{
				NamespacedName: types.NamespacedName{Name: ManagedServiceAccountName, Namespace: managedCluster.Name},
			}, work)
		if err != nil {
			return err
		}

		if status, ok := helpers.GetConditionStatus(work.Status.Conditions, string(manifestworkv1.ManifestApplied)); ok && status == metav1.ConditionTrue {
			logger.V(1).Info("manifestwork applied. preparing secret...")
			err := r.syncManagedClusterKubeconfig(regCluster, managedCluster, hubCluster, ctx)
			if err != nil {
				return giterrors.WithStack(err)
			}
		}
	}
	return nil
}

func (r *RegisteredClusterReconciler) processRegclusterDeletion(regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance) (ctrl.Result, error) {

	manifestwork := &manifestworkv1.ManifestWork{}
	err := hubCluster.Client.Get(context.TODO(),
		client.ObjectKey{
			NamespacedName: types.NamespacedName{
				Name:      ManagedServiceAccountName,
				Namespace: managedCluster.Name},
		},
		manifestwork)
	switch {
	case err == nil:
		r.Log.Info("delete manifestwork", "name", ManagedServiceAccountName)
		if err := hubCluster.Client.Delete(context.TODO(), manifestwork); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		r.Log.Info("waiting manifestwork to be deleted",
			"name", ManagedServiceAccountName,
			"namespace", managedCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
	case !k8serrors.IsNotFound(err):

		return ctrl.Result{}, giterrors.WithStack(err)
	}
	r.Log.Info("deleted manifestwork", "name", ManagedServiceAccountName)

	managed := &authv1alpha1.ManagedServiceAccount{}
	err = hubCluster.Client.Get(context.TODO(),
		client.ObjectKey{
			NamespacedName: types.NamespacedName{
				Name:      ManagedServiceAccountName,
				Namespace: managedCluster.Name},
		},
		managed)
	switch {
	case err == nil:
		r.Log.Info("delete managedserviceaccount", "name", ManagedServiceAccountName)
		if err := hubCluster.Client.Delete(context.TODO(), managed); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		r.Log.Info("waiting managedserviceaccount to be deleted",
			"name", ManagedServiceAccountName,
			"namespace", managedCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
	case !k8serrors.IsNotFound(err):
		return ctrl.Result{}, giterrors.WithStack(err)
	}
	r.Log.Info("deleted managedserviceaccount", "name", ManagedServiceAccountName)

	addon := &addonv1alpha1.ManagedClusterAddOn{}
	err = hubCluster.Client.Get(context.TODO(),
		client.ObjectKey{
			NamespacedName: types.NamespacedName{
				Name:      ManagedClusterAddOnName,
				Namespace: managedCluster.Name,
			},
		},
		addon)
	switch {
	case err == nil:
		r.Log.Info("delete mangedclusteraddon", "name", ManagedClusterAddOnName)
		if err := hubCluster.Client.Delete(context.TODO(), addon); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		r.Log.Info("waiting mangedclusteraddon to be deleted",
			"name", ManagedClusterAddOnName,
			"namespace", managedCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
	case !k8serrors.IsNotFound(err):
		return ctrl.Result{}, giterrors.WithStack(err)
	}
	r.Log.Info("deleted mangedclusteraddon", "name", ManagedClusterAddOnName)

	cluster := &clusterapiv1.ManagedCluster{}
	err = hubCluster.Client.Get(context.TODO(),
		client.ObjectKey{
			NamespacedName: types.NamespacedName{
				Name: managedCluster.Name},
		},
		cluster)
	switch {
	case err == nil:
		r.Log.Info("delete managedcluster", "name", managedCluster.Name)
		if err := hubCluster.Client.Delete(context.TODO(), cluster); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		r.Log.Info("waiting managedcluster to be deleted",
			"name", managedCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	case !k8serrors.IsNotFound(err):
		return ctrl.Result{}, giterrors.WithStack(err)
	}
	r.Log.Info("deleted managedcluster", "name", managedCluster.Name)

	return ctrl.Result{}, nil
}

func (r *RegisteredClusterReconciler) syncManagedClusterKubeconfig(regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance, ctx context.Context) error {
	logger := r.Log.WithName("syncManagedClusterKubeconfig").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "managed cluster name", managedCluster.Name)
	// Retrieve the API URL

	if managedCluster.Spec.ManagedClusterClientConfigs == nil && len(managedCluster.Spec.ManagedClusterClientConfigs) == 0 {
		return errors.New("ManagedClusterClientConfigs not configured as expected")
	}
	apiUrl := managedCluster.Spec.ManagedClusterClientConfigs[0].URL

	// Retrieve the secret containing the managedserviceaccount token
	token := &corev1.Secret{}

	err := hubCluster.Client.Get(ctx,
		client.ObjectKey{
			NamespacedName: types.NamespacedName{Name: ManagedServiceAccountName, Namespace: managedCluster.Name},
		}, token)
	if err != nil {
		return err
	}

	applierBuilder := &clusteradmapply.ApplierBuilder{}
	readerDeploy := resources.GetScenarioResourcesReader()
	applier := applierBuilder.
		WithClient(r.KubeClient, r.APIExtensionClient, r.DynamicClient).
		WithOwner(regCluster, false, true, r.Scheme).
		Build()

	files := []string{
		"cluster-registration/kubeconfig_secret.yaml",
	}

	secretName := fmt.Sprintf("%s-cluster-secret", regCluster.Name)
	values := struct {
		ApiURL      string
		Token       string
		CABundle    string
		SecretName  string
		Namespace   string
		ClusterName string
	}{
		ApiURL:      apiUrl,
		Token:       string(token.Data["token"]),
		CABundle:    b64.StdEncoding.EncodeToString(token.Data["ca.crt"]),
		SecretName:  secretName,
		ClusterName: regCluster.Name,
		Namespace:   regCluster.Namespace,
	}

	_, err = applier.ApplyCustomResources(readerDeploy, values, false, "", files...)
	if err != nil {
		return giterrors.WithStack(err)
	}
	logger.V(1).Info("cluster kubeconfig synced")

	// Patch the RegisteredCluster status with reference to the kubeconfig secret
	patch := client.MergeFrom(regCluster.DeepCopy())
	regCluster.Status.ClusterSecretRef = corev1.LocalObjectReference{
		Name: secretName,
	}
	if err := r.Client.Status().Patch(ctx, regCluster, patch); err != nil {
		return err
	}

	return nil
}

func (r *RegisteredClusterReconciler) createManagedCluster(regCluster *singaporev1alpha1.RegisteredCluster, hubCluster *helpers.HubInstance, ctx context.Context) error {

	// check if managedcluster is already exists
	managedClusterList := &clusterapiv1.ManagedClusterList{}
	if err := hubCluster.Client.List(context.Background(), managedClusterList, client.MatchingLabels{RegisteredClusterNamelabel: regCluster.Name, RegisteredClusterNamespacelabel: regCluster.Namespace}); err != nil {
		// Error reading the object - requeue the request.
		return err
	}

	mcsName := helpers.ManagedClusterSetNameForWorkspace(regCluster.Namespace)

	if len(managedClusterList.Items) < 1 {
		managedCluster := &clusterapiv1.ManagedCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterapiv1.SchemeGroupVersion.String(),
				Kind:       "ManagedCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "registered-cluster-",
				Labels: map[string]string{
					RegisteredClusterNamelabel:      regCluster.Name,
					RegisteredClusterNamespacelabel: regCluster.Namespace,
					ManagedClusterSetlabel:          mcsName,
				},
			},
			Spec: clusterapiv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		}

		if err := hubCluster.Cluster.GetClient().Create(context.TODO(), managedCluster, &client.CreateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func registeredClusterPredicate() predicate.Predicate {
	return predicate.Predicate(predicate.Funcs{
		GenericFunc: func(e event.GenericEvent) bool { return false },
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			new, okNew := e.ObjectNew.(*singaporev1alpha1.RegisteredCluster)
			old, okOld := e.ObjectOld.(*singaporev1alpha1.RegisteredCluster)
			if okNew && okOld {
				return equality.Semantic.DeepEqual(old.Status, new.Status)
			}
			return true
		},
	},
	)
}

func managedClusterPredicate() predicate.Predicate {
	f := func(obj client.Object) bool {
		log := ctrl.Log.WithName("controllers").WithName("RegisteredCluster").WithName("managedClusterPredicate").WithValues("namespace", obj.GetNamespace(), "name", obj.GetName())
		if _, ok := obj.GetLabels()[RegisteredClusterNamelabel]; ok {
			if _, ok := obj.GetLabels()[RegisteredClusterNamespacelabel]; ok {
				log.V(1).Info("process managedcluster")
				return true
			}

		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return f(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			new, okNew := event.ObjectNew.(*clusterapiv1.ManagedCluster)
			old, okOld := event.ObjectOld.(*clusterapiv1.ManagedCluster)
			if okNew && okOld {
				return f(event.ObjectNew) &&
					(!equality.Semantic.DeepEqual(old.Status, new.Status) ||
						!equality.Semantic.DeepEqual(old.Spec.ManagedClusterClientConfigs, new.Spec.ManagedClusterClientConfigs) ||
						old.GetLabels()["clusterID"] != new.GetLabels()["clusterID"])
			}
			return false
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

func manifestWorkPredicate() predicate.Predicate {
	f := func(obj client.Object) bool {
		log := ctrl.Log.WithName("controllers").WithName("RegisteredCluster").WithName("manifestWorkPredicate").WithValues("namespace", obj.GetNamespace(), "name", obj.GetName())
		if _, ok := obj.GetLabels()[RegisteredClusterNamelabel]; ok {
			if _, ok := obj.GetLabels()[RegisteredClusterNamespacelabel]; ok {
				log.V(1).Info("process manifestwork")
				return true
			}

		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			new, okNew := event.ObjectNew.(*manifestworkv1.ManifestWork)
			old, okOld := event.ObjectOld.(*manifestworkv1.ManifestWork)
			if okNew && okOld {
				return f(event.ObjectNew) && !equality.Semantic.DeepEqual(old.Status, new.Status)
			}
			return false
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.

func (r *RegisteredClusterReconciler) SetupWithManager(mgr ctrl.Manager, scheme *runtime.Scheme) error {

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&singaporev1alpha1.RegisteredCluster{}, builder.WithPredicates(registeredClusterPredicate()))

	for _, hubCluster := range r.HubClusters {

		r.Log.Info("add watchers for ", "hubConfig.Name", hubCluster.HubConfig.Name)
		controllerBuilder.Watches(source.NewKindWithCache(&clusterapiv1.ManagedCluster{}, hubCluster.Cluster.GetCache()), handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			managedCluster := o.(*clusterapiv1.ManagedCluster)
			r.Log.Info("Processing ManagedCluster event", "name", managedCluster.Name)

			req := make([]reconcile.Request, 0)
			req = append(req, reconcile.Request{
				ObjectKey: client.ObjectKey{
					NamespacedName: types.NamespacedName{
						Name:      managedCluster.GetLabels()[RegisteredClusterNamelabel],
						Namespace: managedCluster.GetLabels()[RegisteredClusterNamespacelabel],
					},
				},
			})
			return req
		}), builder.WithPredicates(managedClusterPredicate()))
		controllerBuilder.Watches(source.NewKindWithCache(&manifestworkv1.ManifestWork{}, hubCluster.Cluster.GetCache()), handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			manifestWork := o.(*manifestworkv1.ManifestWork)
			r.Log.Info("Processing ManifestWork event", "name", manifestWork.Name, "namespace", manifestWork.Namespace)

			req := make([]reconcile.Request, 0)
			req = append(req, reconcile.Request{
				ObjectKey: client.ObjectKey{
					NamespacedName: types.NamespacedName{
						Name:      manifestWork.GetLabels()[RegisteredClusterNamelabel],
						Namespace: manifestWork.GetLabels()[RegisteredClusterNamespacelabel],
					},
				},
			})
			return req
		}), builder.WithPredicates(manifestWorkPredicate()))
	}

	return controllerBuilder.
		Complete(r)
}
