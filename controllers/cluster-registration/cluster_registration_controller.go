// Copyright Red Hat

package registeredcluster

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	giterrors "github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	// corev1 "k8s.io/api/core/v1"
	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"
	"github.com/stolostron/applier/pkg/apply"
	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"

	"github.com/stolostron/compute-operator/pkg/helpers"
	"github.com/stolostron/compute-operator/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/kcp-dev/logicalcluster"
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
	RegisteredClusterWorkspace      string = "registeredcluster.singapore.open-cluster-management.io/clustername"
	RegisteredClusterUidLabel       string = "registeredcluster.singapore.open-cluster-management.io/uid"
	ClusterNameAnnotation           string = "registeredcluster.singapore.open-cluster-management.io/clustername"
	ManagedClusterSetlabel          string = "cluster.open-cluster-management.io/clusterset"
)

const defaultSyncerImage = "ghcr.io/kcp-dev/kcp/syncer:v0.6.1"

var syncTargetGVR = schema.GroupVersionResource{
	Group:    "workload.kcp.dev",
	Version:  "v1alpha1",
	Resource: "synctargets",
}

// RegisteredClusterReconciler reconciles a RegisteredCluster object
type RegisteredClusterReconciler struct {
	client.Client
	// KubeClient         kubernetes.Interface
	// DynamicClient      dynamic.Interface
	// APIExtensionClient apiextensionsclient.Interface
	ComputeConfig             *rest.Config
	ComputeKubeClient         kubernetes.Interface
	ComputeDynamicClient      dynamic.Interface
	ComputeAPIExtensionClient apiextensionsclient.Interface
	//KCPClusterClient          *kcpclient.Cluster
	Log         logr.Logger
	Scheme      *runtime.Scheme
	HubClusters []helpers.HubInstance
}

func (r *RegisteredClusterReconciler) Reconcile(computeContextOri context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	ctx := context.TODO()
	// Return a copy of the conext and injects the cluster name in the copied context
	computeContext := logicalcluster.WithCluster(computeContextOri, logicalcluster.New(req.ClusterName))
	logger := r.Log.WithValues("clusterName", req.ClusterName, "namespace", req.Namespace, "name", req.Name)
	logger.V(1).Info("Reconciling....")

	regCluster := &singaporev1alpha1.RegisteredCluster{}

	if err := r.Client.Get(
		computeContext,
		types.NamespacedName{Namespace: req.Namespace, Name: req.Name},
		regCluster); err != nil {
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

	logger.V(2).Info("Add finalizer")
	if err := r.Client.Update(computeContext, regCluster); err != nil {
		return ctrl.Result{}, giterrors.WithStack(err)
	}

	// TODO create managedclusterset for workspace

	if regCluster.DeletionTimestamp == nil {
		// create managecluster on creation of registeredcluster CR
		if err := r.createManagedCluster(ctx, regCluster, &hubCluster, req.ClusterName); err != nil {
			logger.Error(err, "failed to create ManagedCluster")
			return ctrl.Result{}, err
		}
	}
	managedCluster, err := r.getManagedCluster(ctx, regCluster, &hubCluster, req.ClusterName)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Error(err, "failed to get ManagedCluster")
		return ctrl.Result{}, err
	}

	//if deletetimestamp then process deletion
	if regCluster.DeletionTimestamp != nil {
		if r, err := r.processRegclusterDeletion(ctx, regCluster, &managedCluster, &hubCluster); err != nil || r.Requeue {
			return r, err
		}
		controllerutil.RemoveFinalizer(regCluster, helpers.RegisteredClusterFinalizer)
		if err := r.Client.Update(computeContext, regCluster); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		return reconcile.Result{}, nil
	}

	// update status of registeredcluster - add import command
	// TODO - skip creating the secret if cluster is already imported - and maybe delete it once cluster is imported?
	if err := r.updateImportCommand(computeContext, ctx, regCluster, &managedCluster, &hubCluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
		}
		logger.Error(err, "failed to update import command")
		return ctrl.Result{}, err
	}

	// sync SyncTarget
	if err := r.syncSyncTarget(computeContext, regCluster, &managedCluster); err != nil {
		logger.Error(err, "failed to sync SyncTarget in location workspace")
		return ctrl.Result{}, giterrors.WithStack(err)
	}

	// sync kcp-syncer service account (currently one per location workspace - probably change to one per syncer, owned by the syncer) in kcp workspace
	token := ""
	if token, err = r.syncServiceAccount(computeContext, ctx, regCluster, &managedCluster, &hubCluster); err != nil {
		logger.Error(err, "failed to sync ServiceAccount")
		return ctrl.Result{}, err
	}

	// sync kcp-syncer deployment and supporting resources
	if err := r.syncKcpSyncer(computeContext, ctx, regCluster, &managedCluster, &hubCluster, token); err != nil {
		logger.Error(err, "failed to sync kcp-syncer")
		return ctrl.Result{}, err
	}

	// update status of registeredcluster
	if err := r.updateRegisteredClusterStatus(computeContext, regCluster, &managedCluster); err != nil {
		logger.Error(err, "failed to update registered cluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// List of regexes to exclude from labels and cluster claims copied to sync target labels
var excludeLabelREs = []string{
	"^feature\\.open-cluster-management\\.io\\/addon",
}

// Return all of the ManagedCluster labels and cluster claims that should be exposed as labels on the SyncTarget
func (r *RegisteredClusterReconciler) getSyncTargetLabels(cluster clusterapiv1.ManagedCluster, excludeLabelRegExps []string) map[string]string {
	logger := r.Log.WithName("getSyncTargetLabels").WithValues("namespace", cluster.Namespace, "name", cluster.Name, "cluster")
	labels := make(map[string]string)

	for k, v := range cluster.Labels {
		labels[k] = v
	}

	for _, clusterClaim := range cluster.Status.ClusterClaims {
		if errs := validation.IsValidLabelValue(clusterClaim.Value); len(errs) != 0 {
			logger.V(4).Info("excluding cluster claim", "claim", clusterClaim.Value)
		} else {
			labels[clusterClaim.Name] = clusterClaim.Value
		}

	}

	for _, excludeLabelRegex := range excludeLabelRegExps {
		for k := range labels {
			r, e := regexp.MatchString(excludeLabelRegex, k)
			if e != nil {
				logger.Error(e, "Error evaluating regex", "regex", excludeLabelRegex)
				continue
			}

			if r {
				logger.V(4).Info("excluding label", "label", k)
				delete(labels, k)
			}
		}
	}
	return labels
}

func (r *RegisteredClusterReconciler) getSyncTarget(locationContext context.Context, regCluster *singaporev1alpha1.RegisteredCluster) (*unstructured.Unstructured, error) {
	logger := r.Log.WithName("getSyncTarget").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "cluster", regCluster.Annotations["clusterName"])

	labels := RegisteredClusterNamelabel + "=" + regCluster.Name + "," + RegisteredClusterNamespacelabel + "=" + regCluster.Namespace + "," + RegisteredClusterWorkspace + "=" + strings.ReplaceAll(regCluster.Annotations["clusterName"], ":", "-") + "," + RegisteredClusterUidLabel + "=" + string(regCluster.UID)
	syncTargetList, err := r.ComputeDynamicClient.Resource(syncTargetGVR).List(locationContext, metav1.ListOptions{
		LabelSelector: labels,
	})

	if err != nil {
		r.Log.Error(err, "error getting SyncTarget list")
		return nil, giterrors.WithStack(err)
	}

	r.Log.V(2).Info("Number of synctargets found with labels",
		"number", len(syncTargetList.Items),
		RegisteredClusterNamelabel, regCluster.Name,
		RegisteredClusterNamespacelabel, regCluster.Namespace)

	if len(syncTargetList.Items) == 0 {
		return nil, nil
	}
	if len(syncTargetList.Items) > 1 {
		logger.Error(err, "more than one synctarget found for registered cluster")
	}

	return &syncTargetList.Items[0], nil

}

func (r *RegisteredClusterReconciler) syncSyncTarget(computeContext context.Context, regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster) error {

	logger := r.Log.WithName("syncSyncTarget").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "managed cluster name", managedCluster.Name)

	if status, ok := helpers.GetConditionStatus(regCluster.Status.Conditions, clusterapiv1.ManagedClusterConditionJoined); ok && status == metav1.ConditionTrue {

		locationContext := logicalcluster.WithCluster(computeContext, logicalcluster.New(regCluster.Spec.Location))

		syncTarget, err := r.getSyncTarget(locationContext, regCluster)
		if err != nil {
			return giterrors.WithStack(err)
		}

		// Add labels to uniquely identify RegisteredCluster
		labels := map[string]string{
			RegisteredClusterNamelabel:      regCluster.Name,
			RegisteredClusterNamespacelabel: regCluster.Namespace,
			RegisteredClusterWorkspace:      strings.ReplaceAll(regCluster.Annotations["clusterName"], ":", "-"),
			RegisteredClusterUidLabel:       string(regCluster.UID),
		}
		// Copy the labels from the RegsiteredCluster
		for k, v := range regCluster.Labels {
			labels[k] = v
		}
		// Copy the labels and clusterclaims from the ManagedCluster
		managedClusterLabels := r.getSyncTargetLabels(*managedCluster, excludeLabelREs)
		for k, v := range managedClusterLabels {
			labels[k] = v
		}

		if syncTarget == nil {
			syncTarget := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": workloadv1alpha1.SchemeGroupVersion.String(),
					"kind":       "SyncTarget",
					"metadata": map[string]interface{}{
						"generateName": regCluster.Name + "-",
						"labels":       labels,
					},
					"spec": map[string]interface{}{
						"unschedulable": false,
					},
				},
			}
			if _, err := r.ComputeDynamicClient.Resource(syncTargetGVR).Create(locationContext, syncTarget, metav1.CreateOptions{}); err != nil {
				return err
			}
			logger.V(2).Info("SyncTarget is created in the location workspace")
		} else {
			// Update SyncTarget labels. Merge with existing labels found on SyncTarget since kcp adds some too
			syncTargetLabels := syncTarget.GetLabels()
			modified := mergeMap(&syncTargetLabels, labels)

			if modified {
				syncTarget.SetLabels(syncTargetLabels)
				if _, err := r.ComputeDynamicClient.Resource(syncTargetGVR).Update(locationContext, syncTarget, metav1.UpdateOptions{}); err != nil {
					return err
				}
				logger.V(2).Info("SyncTarget is updated in the location workspace")
			} else {
				r.Log.V(2).Info("no changes detected to SyncTarget", "labels", labels)
			}
		}
	}
	return nil
}

// Adapted from openshift/library-go
func mergeMap(existing *map[string]string, required map[string]string) bool {
	modified := false

	if *existing == nil {
		*existing = map[string]string{}
	}
	for k, v := range required {
		actualKey := k
		removeKey := false

		if existingV, ok := (*existing)[actualKey]; removeKey {
			if !ok {
				continue
			}
			// value found -> it should be removed
			delete(*existing, actualKey)
			modified = true

		} else if !ok || v != existingV {
			modified = true
			(*existing)[actualKey] = v
		}
	}
	return modified
}

func (r *RegisteredClusterReconciler) updateRegisteredClusterStatus(computeContext context.Context, regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster) error {
	r.Log.V(2).Info("updateRegisteredClusterStatus",
		"regcluster", regCluster.Name,
		"managedCluster", managedCluster.Name)
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
	r.Log.V(2).Info("updateRegisteredClusterStatus",
		"patch", patch,
		"regcluster", regCluster.Status)
	if err := r.Client.Status().Patch(computeContext, regCluster, patch); err != nil {
		return giterrors.WithStack(err)
	}

	return nil
}

func (r *RegisteredClusterReconciler) getManagedCluster(ctx context.Context, regCluster *singaporev1alpha1.RegisteredCluster, hubCluster *helpers.HubInstance, clusterName string) (clusterapiv1.ManagedCluster, error) {
	managedClusterList := &clusterapiv1.ManagedClusterList{}
	managedCluster := clusterapiv1.ManagedCluster{}
	if err := hubCluster.Client.List(ctx, managedClusterList, client.MatchingLabels(getRegisteredClusterLabels(regCluster, clusterName))); err != nil {
		// Error reading the object - requeue the request.
		return managedCluster, giterrors.WithStack(err)
	}

	r.Log.V(2).Info("Number of managed cluster found with labels",
		"number", len(managedClusterList.Items),
		RegisteredClusterNamelabel, regCluster.Name,
		RegisteredClusterNamespacelabel, regCluster.Namespace,
		ManagedClusterSetlabel, helpers.ManagedClusterSetNameForWorkspace(clusterName))
	if len(managedClusterList.Items) == 1 {
		return managedClusterList.Items[0], nil
	}

	if regCluster.DeletionTimestamp != nil {
		return managedCluster, nil
	}
	return managedCluster, fmt.Errorf("correct managedcluster not found")
}

func (r *RegisteredClusterReconciler) updateImportCommand(computeContext context.Context,
	ctx context.Context,
	regCluster *singaporev1alpha1.RegisteredCluster,
	managedCluster *clusterapiv1.ManagedCluster,
	hubCluster *helpers.HubInstance) error {
	r.Log.V(2).Info("updateImportCommand",
		"registered cluster", regCluster.Name)
	// get import secret from mce managecluster namespace
	importSecret := &corev1.Secret{}
	if err := hubCluster.Cluster.GetAPIReader().Get(ctx,
		types.NamespacedName{Namespace: managedCluster.Name, Name: managedCluster.Name + "-import"},
		importSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			return giterrors.WithStack(err)
		}
		return giterrors.WithStack(err)
	}

	applier := apply.NewApplierBuilder().
		WithClient(r.ComputeKubeClient,
			r.ComputeAPIExtensionClient,
			r.ComputeDynamicClient).
		WithOwner(regCluster, false, true, r.Scheme).
		WithContext(computeContext).
		Build()

	readerDeploy := resources.GetScenarioResourcesReader()

	files := []string{
		"cluster-registration/import_secret.yaml",
	}

	// Get yaml representation of import command

	crdsv1Yaml, err := yaml.Marshal(importSecret.Data["crdsv1.yaml"])
	if err != nil {
		return giterrors.WithStack(err)
	}

	importYaml, err := yaml.Marshal(importSecret.Data["import.yaml"])
	if err != nil {
		return giterrors.WithStack(err)
	}

	importCommand := "echo \"" + strings.TrimSpace(string(crdsv1Yaml)) + "\" | base64 --decode | kubectl apply -f - && sleep 2 && echo \"" + strings.TrimSpace(string(importYaml)) + "\" | base64 --decode | kubectl apply -f -"

	values := struct {
		Name          string
		Namespace     string
		ImportCommand string
		ClusterName   string
	}{
		Name:          regCluster.Name,
		Namespace:     regCluster.Namespace,
		ImportCommand: importCommand,
		ClusterName:   regCluster.ClusterName,
	}

	r.Log.V(2).Info("create secret on compute",
		"cluster", regCluster.ClusterName,
		"namespace", regCluster.Namespace,
		"name", regCluster.Name)

	_, err = applier.ApplyDirectly(readerDeploy, values, false, "", files...)
	if err != nil {
		return giterrors.WithStack(err)
	}

	r.Log.V(2).Info("patch registeredCluster on compute with import secret",
		"namespace", regCluster.Namespace,
		"name", regCluster.Name)
	patch := client.MergeFrom(regCluster.DeepCopy())
	regCluster.Status.ImportCommandRef = corev1.LocalObjectReference{
		Name: regCluster.Name + "-import",
	}
	if err := r.Client.Status().Patch(computeContext, regCluster, patch); err != nil {
		return giterrors.WithStack(err)
	}

	return nil
}

func (r *RegisteredClusterReconciler) syncServiceAccount(computeContext context.Context,
	ctx context.Context,
	regCluster *singaporev1alpha1.RegisteredCluster,
	managedCluster *clusterapiv1.ManagedCluster,
	hubCluster *helpers.HubInstance) (string, error) {

	r.Log.V(2).Info("syncServiceAccount",
		"registered cluster", regCluster.Name,
		"location", regCluster.Spec.Location)

	// Create the ServiceAccount if it doesn't yet exist
	saName := helpers.GetSyncerServiceAccountName()

	// sa, err := r.ComputeKubeClient.Cluster(logicalcluster.New(regCluster.Spec.Location)).CoreV1().ServiceAccounts("default").Get(ctx, saName, metav1.GetOptions{})
	locationContext := logicalcluster.WithCluster(computeContext, logicalcluster.New(regCluster.Spec.Location))
	sa, err := r.ComputeKubeClient.CoreV1().ServiceAccounts("default").Get(locationContext, saName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return "", err
		}

		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: saName,
			},
		}
		r.Log.V(2).Info("syncServiceAccount",
			"creating service account", regCluster.Name)
		sa, err = r.ComputeKubeClient.CoreV1().ServiceAccounts("default").Create(locationContext, sa, metav1.CreateOptions{})
		if err != nil {
			return "", err
		}
	}

	// Sync the ClusterRole and ClusterRoleBinding

	// applier := apply.NewApplierBuilder().
	// 	WithClient(r.ComputeKubeClient,
	// 		r.ComputeAPIExtensionClient,
	// 		r.ComputeDynamicClient).
	// 	// WithOwner(regCluster, false, true, r.Scheme). //TODO - add owner synctarget
	// 	WithContext(locationContext).
	// 	Build()

	// readerDeploy := resources.GetScenarioResourcesReader()

	// files := []string{
	// 	"cluster-registration/kcp_syncer_clusterrole.yaml",
	// 	"cluster-registration/kcp_syncer_clusterrolebinding.yaml",
	// }

	// values := struct {
	// 	KcpSyncerName      string
	// 	SyncTargetName     string
	// 	ServiceAccountName string
	// }{
	// 	KcpSyncerName:      helpers.GetSyncerName(regCluster.Name),
	// 	SyncTargetName:     regCluster.Name, // TODO - Get this from SyncTarget.Name
	// 	ServiceAccountName: saName,
	// }
	// fmt.Println("Sleep Start.....")

	// // Calling Sleep method so I can see what the KCP log is doing...
	// time.Sleep(10 * time.Second)

	// // Printed after sleep is over
	// fmt.Println("Sleep Over.....")
	// _, err = applier.ApplyDirectly(readerDeploy, values, false, "", files...)
	// fmt.Println("AFTER Sleep Start.....")

	// // Calling Sleep method
	// time.Sleep(10 * time.Second)

	// Printed after sleep is over
	r.Log.V(1).Info("SKIPPED create clusterrole and clusterrolebinding for now... permission not yet allowed",
		"cluster", regCluster.ClusterName,
		"namespace", regCluster.Namespace,
		"name", regCluster.Name)
	if err != nil {
		return "", giterrors.WithStack(err)
	}

	// Return the ServiceAccount token
	token, err := r.getKcpSyncerSAToken(computeContext, regCluster, sa)
	return token, err

}

func (r *RegisteredClusterReconciler) getKcpSyncerSAToken(computeContext context.Context, regCluster *singaporev1alpha1.RegisteredCluster, sa *corev1.ServiceAccount) (string, error) {

	r.Log.V(2).Info("getKcpSyncerSAToken",
		"service account", sa.Name)

	saName := helpers.GetSyncerServiceAccountName()
	locationContext := logicalcluster.WithCluster(computeContext, logicalcluster.New(regCluster.Spec.Location))

	for _, secretRef := range sa.Secrets {
		r.Log.V(4).Info("checking secret",
			"secret", secretRef.Name)
		if !strings.HasPrefix(secretRef.Name, saName) {
			continue
		}
		r.Log.V(4).Info("reading secret",
			"secret", secretRef.Name)

		secret, err := r.ComputeKubeClient.CoreV1().Secrets("default").Get(locationContext, secretRef.Name, metav1.GetOptions{})
		if err != nil {
			r.Log.Error(err,
				"secret", secretRef.Name)
			continue
		}
		r.Log.V(4).Info("read secret",
			"secret", secretRef.Name)

		if secret.Type != corev1.SecretTypeServiceAccountToken {
			r.Log.V(4).Info("wronog secret type",
				"type", secret.Type)

			continue
		}

		token, ok := secret.Data["token"]
		if !ok {
			r.Log.V(4).Info("wrong data",
				"data", secret.Data)
			continue
		}
		if len(token) == 0 {
			continue
		}

		return string(token), nil
	}

	return "", fmt.Errorf("failed to get the token of workspace sa %s in namespace default", saName) // TODO - better error with more specific context
}

func getSyncerImage() string {
	syncerImage := os.Getenv("KCP_SYNCER_IMAGE")
	if len(syncerImage) > 0 {
		return syncerImage
	}
	return defaultSyncerImage
}

func (r *RegisteredClusterReconciler) syncKcpSyncer(computeContext context.Context, ctx context.Context, regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance, token string) error {
	logger := r.Log.WithName("syncKcpSyncer").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "managed cluster name", managedCluster.Name)

	// If cluster has joined, sync the ManifestWork to create the kcp-syncer deployment and supporting resources
	if status, ok := helpers.GetConditionStatus(regCluster.Status.Conditions, clusterapiv1.ManagedClusterConditionJoined); ok && status == metav1.ConditionTrue {

		readerDeploy := resources.GetScenarioResourcesReader()

		applier := hubCluster.ApplierBuilder.Build()

		syncerName := helpers.GetSyncerName(regCluster.Name)

		kcpURL, err := url.Parse(r.ComputeConfig.Host)
		if err != nil {
			return err
		}

		logger.V(2).Info("syncKcpSyncer", "url path", kcpURL.Path)
		logger.V(2).Info("syncKcpSyncer", "reg cluster location", regCluster.Spec.Location)

		values := struct {
			KcpSyncerName                   string
			KcpToken                        string
			KcpServer                       string
			SyncTargetName                  string
			ManagedClusterName              string
			RegisteredClusterNameLabel      string
			RegisteredClusterNamespaceLabel string
			RegisteredClusterName           string
			RegisteredClusterNamespace      string
			ClusterNameAnnotation           string
			RegisteredClusterClusterName    string
			LogicalClusterLabel             string
			LogicalCluster                  string
			Image                           string
		}{
			KcpSyncerName:                   syncerName,
			KcpToken:                        token,
			KcpServer:                       fmt.Sprintf("%s://%s", kcpURL.Scheme, kcpURL.Host),
			SyncTargetName:                  regCluster.Name, // TODO - Get this from SyncTarget.Name
			ManagedClusterName:              managedCluster.Name,
			RegisteredClusterNameLabel:      RegisteredClusterNamelabel,
			RegisteredClusterNamespaceLabel: RegisteredClusterNamespacelabel,
			RegisteredClusterName:           regCluster.Name,
			RegisteredClusterNamespace:      regCluster.Namespace,
			ClusterNameAnnotation:           ClusterNameAnnotation,
			RegisteredClusterClusterName:    managedCluster.Annotations[ClusterNameAnnotation],
			LogicalCluster:                  regCluster.Spec.Location,
			LogicalClusterLabel:             strings.ReplaceAll(regCluster.Spec.Location, ":", "_"),
			Image:                           getSyncerImage(),
		}

		logger.V(2).Info("values", "Values", values)

		files := []string{
			"cluster-registration/kcp_syncer_manifestwork.yaml",
		}

		_, err = applier.ApplyCustomResources(readerDeploy, values, false, "", files...)
		if err != nil {
			return giterrors.WithStack(err)
		}

		work := &manifestworkv1.ManifestWork{}

		err = hubCluster.Client.Get(ctx,
			types.NamespacedName{Name: values.KcpSyncerName, Namespace: managedCluster.Name},
			work)

		if err != nil {
			return giterrors.WithStack(err)
		}

		if status, ok := helpers.GetConditionStatus(work.Status.Conditions, string(manifestworkv1.ManifestApplied)); ok && status == metav1.ConditionTrue {
			logger.V(1).Info("manifestwork applied. TODO: update status...")
			//TODO - update status
		}
	}
	return nil
}

func (r *RegisteredClusterReconciler) processRegclusterDeletion(ctx context.Context, regCluster *singaporev1alpha1.RegisteredCluster, managedCluster *clusterapiv1.ManagedCluster, hubCluster *helpers.HubInstance) (ctrl.Result, error) {

	// TODO - update this
	manifestwork := &manifestworkv1.ManifestWork{}
	manifestworkName := helpers.GetSyncerName(regCluster.Name)
	err := hubCluster.Client.Get(ctx,
		types.NamespacedName{
			Name:      manifestworkName,
			Namespace: managedCluster.Name},
		manifestwork)
	switch {
	case err == nil:
		r.Log.Info("delete manifestwork", "name", manifestworkName)
		if err := hubCluster.Client.Delete(ctx, manifestwork); err != nil {
			return ctrl.Result{}, giterrors.WithStack(err)
		}
		r.Log.Info("waiting manifestwork to be deleted",
			"name", manifestworkName,
			"namespace", managedCluster.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
	case !k8serrors.IsNotFound(err):

		return ctrl.Result{}, giterrors.WithStack(err)
	}
	r.Log.Info("deleted manifestwork", "name", manifestworkName)

	// TODO - remaining cleanup - https://issues.redhat.com/browse/CMCS-145

	cluster := &clusterapiv1.ManagedCluster{}
	err = hubCluster.Client.Get(ctx,
		types.NamespacedName{
			Name: managedCluster.Name},
		cluster)
	switch {
	case err == nil:
		r.Log.Info("delete managedcluster", "name", managedCluster.Name)
		if err := hubCluster.Client.Delete(ctx, cluster); err != nil {
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

func getRegisteredClusterLabels(regCluster *singaporev1alpha1.RegisteredCluster, clusterName string) map[string]string {
	return map[string]string{
		RegisteredClusterNamelabel:      regCluster.Name,
		RegisteredClusterNamespacelabel: regCluster.Namespace,
		RegisteredClusterUidLabel:       string(regCluster.UID),
		ManagedClusterSetlabel:          helpers.ManagedClusterSetNameForWorkspace(clusterName),
	}
}

func (r *RegisteredClusterReconciler) createManagedCluster(ctx context.Context, regCluster *singaporev1alpha1.RegisteredCluster, hubCluster *helpers.HubInstance, clusterName string) error {
	logger := r.Log.WithName("createManagedCluster").WithValues("namespace", regCluster.Namespace, "name", regCluster.Name, "hub", hubCluster.HubConfig.Name)
	// check if managedcluster is already exists
	managedClusterList := &clusterapiv1.ManagedClusterList{}
	labels := getRegisteredClusterLabels(regCluster, clusterName)
	logger.V(2).Info("get managedclusterlist", "labels", labels)
	if err := hubCluster.Client.List(ctx, managedClusterList, client.MatchingLabels(labels)); err != nil {
		// Error reading the object - requeue the request.
		return giterrors.WithStack(err)
	}

	if len(managedClusterList.Items) < 1 {
		managedCluster := &clusterapiv1.ManagedCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterapiv1.SchemeGroupVersion.String(),
				Kind:       "ManagedCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "registered-cluster-",
				Labels:       labels,
				Annotations: map[string]string{
					"open-cluster-management/service-name": "compute",
					ClusterNameAnnotation:                  clusterName,
				},
			},
			Spec: clusterapiv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		}

		if err := hubCluster.Client.Create(ctx, managedCluster, &client.CreateOptions{}); err != nil {
			return giterrors.WithStack(err)
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
				if equality.Semantic.DeepEqual(old.Status, new.Status) {
					log := ctrl.Log.WithName("controllers").WithName("RegisteredCluster").WithName("registeredClusterPredicate").WithValues("namespace", new.GetNamespace(), "name", new.GetName())
					log.V(1).Info("process registeredcluster update")
					return true
				}
				return false
			}
			return true
		},
	},
	)
}

func managedClusterPredicate() predicate.Predicate {
	f := func(obj client.Object) bool {
		if _, ok := obj.GetLabels()[RegisteredClusterNamelabel]; ok {
			if _, ok := obj.GetLabels()[RegisteredClusterNamespacelabel]; ok {
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
				if f(event.ObjectNew) &&
					(!equality.Semantic.DeepEqual(old.Status, new.Status) ||
						!equality.Semantic.DeepEqual(old.Spec.ManagedClusterClientConfigs, new.Spec.ManagedClusterClientConfigs) ||
						old.GetLabels()["clusterID"] != new.GetLabels()["clusterID"]) {
					log := ctrl.Log.WithName("controllers").WithName("RegisteredCluster").WithName("managedClusterPredicate").WithValues("namespace", new.GetNamespace(), "name", new.GetName())
					log.V(1).Info("process managedcluster update")
					return true
				}
				return false
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

// Watch manifest work for status updates so we can update registeredcluster status with status of syncer deployment
func manifestWorkPredicate() predicate.Predicate {
	f := func(obj client.Object) bool {
		if _, ok := obj.GetLabels()[RegisteredClusterNamelabel]; ok {
			if _, ok := obj.GetLabels()[RegisteredClusterNamespacelabel]; ok {
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
				if f(event.ObjectNew) && !equality.Semantic.DeepEqual(old.Status, new.Status) {
					log := ctrl.Log.WithName("controllers").WithName("RegisteredCluster").WithName("manifestWorkPredicate").WithValues("namespace", new.GetNamespace(), "name", new.GetName())
					log.V(1).Info("process manifestwork update")
					return true
				}
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

		r.Log.V(1).Info("add watchers for ", "hubConfig.Name", hubCluster.HubConfig.Name)
		controllerBuilder.Watches(source.NewKindWithCache(&clusterapiv1.ManagedCluster{}, hubCluster.Cluster.GetCache()), handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			managedCluster := o.(*clusterapiv1.ManagedCluster)
			r.Log.Info("Processing ManagedCluster event", "name", managedCluster.Name)

			req := make([]ctrl.Request, 0)
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      managedCluster.GetLabels()[RegisteredClusterNamelabel],
					Namespace: managedCluster.GetLabels()[RegisteredClusterNamespacelabel],
				},
				ClusterName: managedCluster.GetAnnotations()[ClusterNameAnnotation],
			})
			return req
		}), builder.WithPredicates(managedClusterPredicate())).
			Watches(source.NewKindWithCache(&manifestworkv1.ManifestWork{}, hubCluster.Cluster.GetCache()), handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
				manifestWork := o.(*manifestworkv1.ManifestWork)
				r.Log.Info("Processing ManifestWork event", "name", manifestWork.Name, "namespace", manifestWork.Namespace)

				req := make([]reconcile.Request, 0)
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      manifestWork.GetLabels()[RegisteredClusterNamelabel],
						Namespace: manifestWork.GetLabels()[RegisteredClusterNamespacelabel],
					},
					ClusterName: manifestWork.GetAnnotations()[ClusterNameAnnotation],
				})
				return req
			}), builder.WithPredicates(manifestWorkPredicate()))
	}

	return controllerBuilder.
		Complete(r)
}
