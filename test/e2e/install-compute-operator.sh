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

echo "--- Show compute  installer deployment"
oc get deployment -n compute-config compute-installer-controller-manager -o yaml

echo "--- Check that the pod is running"
oc wait --for=condition=ready pods --all --timeout=5m -n compute-config || {
  echo "ERROR - No compute operator pods running!"
  oc get pods -n compute-config
  oc get deployment -n compute-config compute-installer-controller-manager -o yaml
  exit 1
}
oc get pods -n compute-config
oc get pods -n compute-config | grep compute-installer-controller-manager || {
  echo "ERROR compute-installer-controller-manager pod not found!"
  exit 1
}

# TODO
echo "--- Create secret using hub kubeconfig"
oc create secret generic e2e-hub-kubeconfig --from-file=kubeconfig=${SHARED_DIR}/hub-1.kc -n compute-config

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

sleep 30


echo "--- Check for operator manager and webhook pods also running"
oc wait --for=condition=ready pods --all --timeout=5m -n compute-config
oc get pods -n compute-config
oc get pods -n compute-config | grep compute-operator-manager || {
  echo "ERROR compute-operator-manager pod not found!"
  exit 1
}
oc get pods -n compute-config | grep compute-webhook-service || {
  echo "ERROR compute-webhook-service pod not found!"
  exit 1
}

echo "--- Done installing compute operator"
