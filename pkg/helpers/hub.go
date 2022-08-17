// Copyright Red Hat

package helpers

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/stolostron/applier/pkg/apply"
	singaporev1alpha1 "github.com/stolostron/compute-operator/api/singapore/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

type HubInstance struct {
	HubConfig      *singaporev1alpha1.HubConfig
	Cluster        cluster.Cluster
	Client         client.Client
	ApplierBuilder *apply.ApplierBuilder
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

func GetHubClusters(ctx context.Context, mgr ctrl.Manager, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) ([]HubInstance, error) {
	setupLog := ctrl.Log.WithName("setup")
	hubInstances := make([]HubInstance, 0)
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
	hubConfigListU, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	setupLog.Info("nb of hubConfig unstructured found", "size", len(hubConfigListU.Items))

	for _, hubConfigU := range hubConfigListU.Items {

		kubeConfigData, hubConfig, err := getKubeConfigDataFromHubConfig(ctx, hubConfigU, kubeClient)
		if err != nil {
			return nil, err
		}

		hubInstance, err := getHubInstance(kubeConfigData, mgr, hubConfig)
		if err != nil {
			return nil, err
		}

		hubInstances = append(hubInstances, *hubInstance)
	}
	return hubInstances, nil
}

func getKubeConfigDataFromHubConfig(ctx context.Context, hubConfigU unstructured.Unstructured,
	kubeClient kubernetes.Interface) ([]byte, *singaporev1alpha1.HubConfig, error) {
	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("convert to hubConfig structure", "name", hubConfigU.GetName())
	hubConfig := &singaporev1alpha1.HubConfig{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(hubConfigU.Object, hubConfig); err != nil {
		return nil, hubConfig, err
	}

	setupLog.Info("get config secret", "name", hubConfig.Spec.KubeConfigSecretRef.Name)
	configSecret, err := kubeClient.CoreV1().Secrets(hubConfig.Namespace).Get(ctx,
		hubConfig.Spec.KubeConfigSecretRef.Name,
		metav1.GetOptions{})
	if err != nil {
		setupLog.Error(err, "unable to read kubeconfig secret for MCE cluster",
			"HubConfig Name", hubConfig.GetName(),
			"HubConfig Secret Name", hubConfig.Spec.KubeConfigSecretRef.Name)
		return nil, hubConfig, err
	}

	kubeConfigData, ok := configSecret.Data["kubeconfig"]
	if !ok {
		setupLog.Error(err, "HubConfig secret missing kubeconfig data",
			"HubConfig Name", hubConfig.GetName(),
			"HubConfig Secret Name", hubConfig.Spec.KubeConfigSecretRef.Name)
		return nil, hubConfig, errors.New("HubConfig secret missing kubeconfig data")
	}
	return kubeConfigData, hubConfig, nil
}

func getHubInstance(kubeConfigData []byte, mgr ctrl.Manager, hubConfig *singaporev1alpha1.HubConfig) (*HubInstance, error) {
	setupLog := ctrl.Log.WithName("setup")
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
			o.Scheme = mgr.GetScheme() // Explicitly set the scheme which includes ManagedCluster
			// o.NewCache = NewCacheFunc
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
	hubApplierBuilder := apply.NewApplierBuilder().
		WithClient(kubeClient, apiExtensionClient, dynamicClient)

	hubInstance := HubInstance{
		HubConfig:      hubConfig,
		Cluster:        hubCluster,
		Client:         hubCluster.GetClient(),
		ApplierBuilder: hubApplierBuilder,
	}
	return &hubInstance, nil
}
