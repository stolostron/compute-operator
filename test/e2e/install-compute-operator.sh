#!/bin/bash

# Copyright Red Hat

set -e

echo "--- Install the compute operator ..."

cd ${COMPUTE_OPERATOR_DIR}

export IMG="quay.io/stolostron/compute-operator:2.6.0-PR${PULL_NUMBER}-${PULL_PULL_SHA}"
echo "--- Quay image is ${IMG}"

echo "--- Check namespace - before"
oc get namespaces



echo "--- Create compute-config namespace"
# DOES NOT WORK ON vcluster
# oc new-project compute-config
kubectl create namespace compute-config
kubectl config set-context --current --namespace=compute-config

echo "--- Start deploy"
make deploy
echo "--- Sleep a bit for installer pod to start..."
sleep 60

echo "--- Show compute installer deployment"
oc get deployment -n compute-config compute-installer-controller-manager -o yaml

echo "--- Check that the pod is running"
oc wait --for=condition=ready pods --all --timeout=5m -n compute-config || {
  echo "ERROR - No compute operator pods running!"
  oc get pods -n compute-config
  oc get deployment -n compute-config compute-installer-controller-manager -o yaml
  exit 1
}
echo "--- Show compute pods"
oc get pods -n compute-config
oc get pods -n compute-config | grep compute-installer-controller-manager || {
  echo "ERROR compute-installer-controller-manager pod not found!"
  exit 1
}

ls -alh "${SHARED_DIR}"
echo "--- Create secret using hub kubeconfig"
# TEMP disable and install compute operator to hub cluster
#oc create secret generic e2e-hub-kubeconfig --from-file=kubeconfig="${SHARED_DIR}/${VC_COMPUTE}.kubeconfig" -n compute-config
oc create secret generic e2e-hub-kubeconfig --from-file=kubeconfig="${SHARED_DIR}/hub-1.kc" -n compute-config

echo "--- Create HubConfig"
cat > e2e-HubConfig.yaml <<EOF
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: HubConfig
metadata:
  name: e2e-hub-config
  namespace: compute-config
spec:
  kubeconfigSecretRef:
    name: e2e-hub-kubeconfig
EOF
oc create -f e2e-HubConfig.yaml


sleep 10
oc logs --selector='control-plane=controller-manager'

##kubectl create secret generic kcp-kubeconfig -n compute-config --from-file=kubeconfig=/tmp/kubeconfig-compute-operator.yaml
#oc create secret generic kcp-kubeconfig --from-file=kubeconfig="${SHARED_DIR}/${VC_COMPUTE}.kubeconfig" -n compute-config
#oc create secret generic kcp-kubeconfig --from-file=kubeconfig="${SHARED_DIR}/${VC_KCP}.kubeconfig" -n compute-config
oc create secret generic kcp-kubeconfig --from-file=kubeconfig="${KCP_TMP_DIR}/kcp/.kcp/admin.kubeconfig" -n compute-config


echo "--- Create ClusterRegistrar"
cat > e2e-ClusterRegistrar.yaml <<EOF
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: ClusterRegistrar
metadata:
  name: cluster-reg
spec:
  computeService:
    computeKubeconfigSecretRef:
      name: kcp-kubeconfig
EOF
oc create -f e2e-ClusterRegistrar.yaml


sleep 10
oc logs --selector='control-plane=controller-manager'

sleep 20


echo "--- Check for operator manager and webhook pods also running"
oc wait --for=condition=ready pods --all --timeout=5m -n compute-config

echo "--- Done waiting, list pods"
oc get pods -n compute-config -o wide
oc get pods -n compute-config | grep compute-operator-manager || {
  echo "ERROR compute-operator-manager pod not found!"

  oc logs --selector='control-plane=controller-manager'
  exit 1
}
oc get pods -n compute-config | grep compute-webhook-service || {
  echo "ERROR compute-webhook-service pod not found!"

  oc logs --selector='control-plane=controller-manager'
  exit 1
}

echo "--- Done installing compute operator"
