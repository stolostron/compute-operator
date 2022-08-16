#!/bin/bash
#
# Deploy Compute Operator in KCP using KCP Service Account kubeconfig
#
# 1. This script needs to be run in the compute-operator folder
#    git clone compute-operator
#    cd compute-operator
#    ./hack/deploy-compute-operator-in-kcp-sa.sh
#
# 2. The KCP kubeconfig and ACM kubeconfig need to be provided
#
#    The kubeconfig for KCP
#    ➜ kubectl ws .
#    Current workspace is "root:rh-sso-xxxx:compute-operator".
#
#
set -x

echo "deploying the compute operator into KCP"

# When running from Pipelines, load the kubeconfig from secret, and dump to /tmp ?
export KUBECONFIG=${1:-"/Users/cdoan/Downloads/kcp-stable-sa-robin.yaml"}
export ACM_KUBECONFIG=${2:-"/Users/cdoan/Downloads/kcp-sgs-hubs-dtldp-kubeconfig.yaml"}
export IMG=${3:-"quay.io/stolostron/compute-operator:2.7.0-PR36-6a4560b1fb3a35e320c5cd84cd9db4cff4fdbab7"}

# static path
export KCP_SA_KUBECONFIG="/tmp/kcp-sa-kubeconfig"

# KUBECONFIG should point to sa referencing an existing workspace
kubectl kcp ws .
kubectl get ns
kubectl api-resources
kubectl api-resources | wc -l

echo "apiVersion: apis.kcp.dev/v1alpha1
kind: APIBinding
metadata:
  name: acm-kubernetes
spec:
  reference:
    workspace:
      path: root:redhat-acm-compute
      exportName: kubernetes" | kubectl create -f -

kubectl create namespace compute-config || true
kubectl create serviceaccount compute-operator -n compute-config

./build/generate_kubeconfig_from_sa.sh $KUBECONFIG compute-operator compute-config $KCP_SA_KUBECONFIG

echo "apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: compute-operator-cluster-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: compute-operator
  namespace: compute-config" | kubectl apply -f -

KUBECONFIG=$ACM_KUBECONFIG kubectl config view --minify --flatten > /tmp/managed-hub-cluster.kubeconfig
kubectl create secret generic mce-kubeconfig-secret -n compute-config --from-file=kubeconfig=/tmp/managed-hub-cluster.kubeconfig

kubectl apply -f config/crd/singapore.open-cluster-management.io_hubconfigs.yaml
kubectl apply -f config/crd/singapore.open-cluster-management.io_clusterregistrars.yaml

kubectl apply -f hack/hubconfig.yaml -n compute-config

kubectl create secret generic kcp-kubeconfig -n compute-config --from-file=kubeconfig=$KCP_SA_KUBECONFIG
kubectl apply -f hack/clusterregistrar.yaml -n compute-config

export HASH=$(kubectl get apibindings workload.kcp.dev -o jsonpath='{.status.boundResources[?(@.resource=="synctargets")].schema.identityHash}')

# set identityhash
cat > ./resources/compute-templates/hack-values.yaml <<EOF
# Copyright Red Hat
Organization: my-org
ControllerComputeServiceAccountNamespace: default
identityhash: $HASH
EOF
make samples

kubectl apply -f hack/compute/apiexport.yaml

SKIP_WEBHOOK=true make deploy

sleep 30
kubectl get deployments -n compute-config -o yaml | grep ReplicaSet

set +x
