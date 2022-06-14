// Copyright Red Hat

package helpers

import (
	"context"
	"errors"
	"os"
	"strconv"

	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kcpcache "github.com/kcp-dev/apimachinery/pkg/cache"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	manifestworkv1 "open-cluster-management.io/api/work/v1"
	clusteradmapply "open-cluster-management.io/clusteradm/pkg/helpers/apply"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

var (
	scheme       = runtime.NewScheme()
	newCacheFunc = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		opts.KeyFunction = kcpcache.ClusterAwareKeyFunc
		opts.Indexers = k8scache.Indexers{
			kcpcache.ClusterIndexName:             kcpcache.ClusterIndexFunc,
			kcpcache.ClusterAndNamespaceIndexName: kcpcache.ClusterAndNamespaceIndexFunc,
		}
		return cache.New(config, opts)
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterapiv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = addonv1alpha1.AddToScheme(scheme)
	_ = authv1alpha1.AddToScheme(scheme)
	_ = manifestworkv1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

type HubInstance struct {
	HubConfig          *singaporev1alpha1.HubConfig
	Cluster            cluster.Cluster
	Client             client.Client
	KubeClient         kubernetes.Interface
	DynamicClient      dynamic.Interface
	APIExtensionClient apiextensionsclient.Interface
	HubApplier         clusteradmapply.Applier
}

// GetConditionStatus returns the status for a given condition type and whether the condition was found
func GetConditionStatus(conditions []metav1.Condition, t string) (status metav1.ConditionStatus, ok bool) {
	log := ctrl.Log.WithName("GetConditionStatus")
	for i := range conditions {
		condition := conditions[i]

		if condition.Type == t {
			log.V(1).Info("condition found", "type", condition.Type, "status", condition.Status)
			return condition.Status, true
		}
	}
	log.V(1).Info("condition not found", "type", t)
	return "", false
}

func GetHubCluster(workspace string, hubInstances []HubInstance) (HubInstance, error) {
	// For now, we always assume there is only one hub cluster. //TODO Later we will replace this with a lookup.
	if len(hubInstances) == 0 {
		return HubInstance{}, errors.New("hub cluster is not configured")
	}
	return hubInstances[0], nil
}

func GetHubClusters(mgr ctrl.Manager) ([]HubInstance, error) {
	setupLog := ctrl.Log.WithName("setup")
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

	gvr := schema.GroupVersionResource{
		Group:    "singapore.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "hubconfigs"}

	setupLog.Info("retrieve list of hubConfig")
	hubConfigListU, err := dynamicClient.Resource(gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	setupLog.Info("nb of hubConfig unstructured found", "sze", len(hubConfigListU.Items))

	hubInstances := make([]HubInstance, 0)

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

		kubeConfigData, ok := configSecret.Data["kubeconfig"]
		if !ok {
			setupLog.Error(err, "HubConfig secret missing kubeconfig data",
				"HubConfig Name", hubConfig.GetName(),
				"HubConfig Secret Name", hubConfig.Spec.KubeConfigSecretRef.Name)
			return nil, errors.New("HubConfig secret missing kubeconfig data")
		}

		setupLog.Info("generate hubKubeConfig")
		hubKubeconfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigData)
		if err != nil {
			setupLog.Error(err, "unable to create REST config for MCE cluster")
			return nil, err
		}

		if hubConfig.Spec.QPS != "" {
			qps, err := strconv.ParseFloat(hubConfig.Spec.QPS, 32)
			if err != nil {
				return nil, err
			}
			hubKubeconfig.QPS = float32(qps)
		}
		hubKubeconfig.Burst = hubConfig.Spec.Burst

		if hubConfig.Spec.Burst == 0 {
			hubKubeconfig.Burst = 200
		}
		if hubConfig.Spec.QPS == "" {
			hubKubeconfig.QPS = 100.0
		}

		// Add MCE cluster
		hubCluster, err := cluster.New(hubKubeconfig,
			func(o *cluster.Options) {
				o.Scheme = scheme // Explicitly set the scheme which includes ManagedCluster
				o.NewCache = newCacheFunc
			},
		)
		if err != nil {
			setupLog.Error(err, "unable to setup MCE cluster.  For \"Unauthorized\" error message, the HubConfig secret is expired.",
				"HubConfig Secret Name", hubConfig.Spec.KubeConfigSecretRef.Name)
			return nil, err
		}

		// Add MCE cluster to manager
		if err := mgr.Add(hubCluster); err != nil {
			setupLog.Error(err, "unable to add MCE cluster")
			return nil, err
		}

		kubeClient := kubernetes.NewForConfigOrDie(hubKubeconfig)
		dynamicClient := dynamic.NewForConfigOrDie(hubKubeconfig)
		apiExtensionClient := apiextensionsclient.NewForConfigOrDie(hubKubeconfig)
		hubApplier := clusteradmapply.NewApplierBuilder().WithClient(kubeClient, apiExtensionClient, dynamicClient).Build()

		hubInstance := HubInstance{
			HubConfig:          hubConfig,
			Cluster:            hubCluster,
			Client:             hubCluster.GetClient(),
			KubeClient:         kubeClient,
			DynamicClient:      dynamicClient,
			APIExtensionClient: apiExtensionClient,
			HubApplier:         hubApplier,
		}

		hubInstances = append(hubInstances, hubInstance)
	}

	return hubInstances, nil
}
