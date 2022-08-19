#!/bin/bash
# Copyright Red Hat

#trap "kill 0" EXIT

#quit if exit status of any cmd is a non-zero value
set -euo pipefail

SCRIPT_DIR="$(
  cd "$(dirname "$0")" >/dev/null
  pwd
)"


###############################################################################
# Install Certificate Manager operator into current OpenShift
#     oc login should already be done
###############################################################################
echo "=== Preparing to install Certificate Manager for use by kcp ==="

echo "--- Check current hub cluster info and current context"
oc cluster-info
oc config get-contexts

echo "-- Check namespaces"
oc get ns

echo "-- Check catalogsource"
oc get catalogsource -A

echo "-- Install cert-manager operator "
export CERT_MGR_TEMP_DIR=$(mktemp -d)

# OpenShift 4.9 does not have RedHat Cert Manager available so use community one
kubectl kustomize "./operators/cert-manager" > ${CERT_MGR_TEMP_DIR}/certmgr.yaml
kubectl apply -f ${CERT_MGR_TEMP_DIR}/certmgr.yaml

sleep 30
echo "-- Check subscription"
# Only for OpenbShift 4.10 and above, does not work on 4.9
#oc get subscriptions -n openshift-cert-manager-operator openshift-cert-manager-operator -oyaml
oc get subscriptions -n openshift-cert-manager-operator || true

echo "-- Check CSV"
# Only for OpenbShift 4.10 and above, does not work on 4.9
#oc get csv -n openshift-cert-manager-operator
oc get csv -n openshift-operators || true

# Perform a dry-run create of a
# Certificate resource in order to verify that CRDs are installed and all the
# required webhooks are reachable by the K8S API server.
echo -n "  - Wait for cert-manager-operator to be ready: "
until kubectl create -f "./kcp/base/certs.yaml" --dry-run=client >/dev/null 2>&1; do
  echo -n "."
  sleep 5
done
echo "OK"

echo "-- Check namespaces"
oc get ns

echo "-- Check operators"
oc get operators

echo "-- Check subscription"
oc get subscriptions -n openshift-cert-manager-operator

echo "-- Check ClusterServiceVersion"
oc get csv -n openshift-operators

echo "-- Certificate Manager is installed and running."
