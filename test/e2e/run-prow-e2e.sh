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
# Test Setup
###############################################################################

BROWSER=chrome
BUILD_WEB_URL=https://prow.ci.openshift.org/view/gs/origin-ci-test/${JOB_NAME}/${BUILD_ID}
GIT_PULL_NUMBER=$PULL_NUMBER
GIT_REPO_SLUG=${REPO_OWNER}/${REPO_NAME}
HUB_CREDS=$(cat "${SHARED_DIR}/hub-1.json")
OCM_NAMESPACE=open-cluster-management
OCM_ROUTE=multicloud-console
# Hub cluster
export KUBECONFIG="${SHARED_DIR}/hub-1.kc"

export KCP_GIT_BRANCH="v0.7.0"
export KCP_SYNCER_IMAGE="ghcr.io/kcp-dev/kcp/syncer:release-0.7"
export KCP_REPO_TEMP_DIR=$(mktemp -d)

export KCP_KUBECONFIG_DIR="${SHARED_DIR}/kcp"
export KCP_KUBECONFIG="${KCP_KUBECONFIG_DIR}/admin.kubeconfig"


KUBECONFIG_KCP="${KCP_KUBECONFIG}"

# The compute workspace (where RegisteredCluster is created)
export COMPUTE_WORKSPACE="my-compute-ws"
# The location workspace (where SyncTarget is generated)
export LOCATION_WORKSPACE="my-location-ws"
# The controller service account on the compute
export CONTROLLER_COMPUTE_SERVICE_ACCOUNT="compute-operator"
# the namespace on the compute
export CONTROLLER_COMPUTE_SERVICE_ACCOUNT_NAMESPACE="sa-ws"
# The compute organization
export COMPUTE_ORGANIZATION="my-org"
# The compute organization workspace
export ORGANIZATION_WORKSPACE="root:"${COMPUTE_ORGANIZATION}
# The compute cluster workspace
export ABSOLUTE_COMPUTE_WORKSPACE=${ORGANIZATION_WORKSPACE}":"${COMPUTE_WORKSPACE}
export ABSOLUTE_LOCATION_WORKSPACE=${ORGANIZATION_WORKSPACE}":"${LOCATION_WORKSPACE}


OCM_ADDRESS=https://`oc -n $OCM_NAMESPACE get route $OCM_ROUTE -o json | jq -r '.spec.host'`

# # Cypress env variables
# #export ANSIBLE_URL=$(cat "/etc/e2e-secrets/ansible-url")
# #export ANSIBLE_TOKEN=$(cat "/etc/e2e-secrets/ansible-token")
# export BROWSER=$BROWSER
# export BUILD_WEB_URL=$BUILD_WEB_URL
# export CYPRESS_JOB_ID=$PROW_JOB_ID
# #export CYPRESS_RBAC_TEST=$(cat "/etc/e2e-secrets/cypress-rbac-test")
# export CYPRESS_TEST_MODE=BVT
# For pulling source from Git.  install-signed-cert.sh needs this
export GITHUB_USER=$(cat "/etc/ocm-mgdsvcs-e2e-test/github-user")
export GITHUB_TOKEN=$(cat "/etc/ocm-mgdsvcs-e2e-test/github-token")

export ACME_REPO="github.com/acmesh-official/acme.sh"
export COMPUTE_OPERATOR_REPO="github.com/stolostron/compute-operator"

#export GITHUB_PRIVATE_URL=$(cat "/etc/e2e-secrets/github-private-url")
export GIT_PULL_NUMBER=$PULL_NUMBER
export GIT_REPO_SLUG=$GIT_REPO_SLUG
#export SLACK_TOKEN=$(cat "/etc/e2e-secrets/slack-token")

#In order to verify the signed certifiate, we need to use AWS for route53 domain stuff
export AWS_ACCESS_KEY_ID=$(cat "/etc/ocm-mgdsvcs-e2e-test/aws-access-key")
export AWS_SECRET_ACCESS_KEY=$(cat "/etc/ocm-mgdsvcs-e2e-test/aws-secret-access-key")

# Workaround for "error: x509: certificate signed by unknown authority" problem with oc login
mkdir -p ${HOME}/certificates
OAUTH_POD=$(oc -n openshift-authentication get pods -o jsonpath='{.items[0].metadata.name}')
export CYPRESS_OC_CLUSTER_INGRESS_CA=/certificates/ingress-ca.crt
oc rsh -n openshift-authentication $OAUTH_POD cat /run/secrets/kubernetes.io/serviceaccount/ca.crt > ${HOME}${CYPRESS_OC_CLUSTER_INGRESS_CA}

# managed cluster
#MANAGED_CREDS=$(cat "${SHARED_DIR}/managed-1.json")
#export CYPRESS_MANAGED_OCP_URL=$(echo $MANAGED_CREDS | jq -r '.api_url')
#export CYPRESS_MANAGED_OCP_USER=$(echo $MANAGED_CREDS | jq -r '.username')
#export CYPRESS_MANAGED_OCP_PASS=$(echo $MANAGED_CREDS | jq -r '.password')
#export CYPRESS_PROW="true"

# Set up git credentials.
echo "--- Setting up git credentials."
{
  echo "https://${GITHUB_USER}:${GITHUB_TOKEN}@${ACME_REPO}.git"
  echo "https://${GITHUB_USER}:${GITHUB_TOKEN}@${COMPUTE_OPERATOR_REPO}.git"
} >> ghcreds
git config --global credential.helper 'store --file=ghcreds'

# Set up Quay credentials.
echo "--- Setting up Quay credentials."
export QUAY_TOKEN=$(cat "/etc/acm-cicd-quay-pull/token")

echo "--- Check current hub cluster info and current context"
oc cluster-info
oc config get-contexts

# Install vcluster
export VC_MANAGED=vc-managed
export VC_COMPUTE=vc-compute
export VC_KCP=vc-kcp

cat <<EOF > vcluster-values.yml
openshift:
  enable: true
sync:
  networkpolicies:
    enabled: true
  serviceaccounts:
    enabled: true
  services:
    syncServiceSelector: true
EOF

echo "-- Creating a vcluster to import as a managed cluster"
oc create ns ${VC_MANAGED}
#vcluster create ${VC_MANAGED} --connect=false --namespace=${VC_MANAGED}
oc config current-context view | vcluster create ${VC_MANAGED} --connect=false --expose -f vcluster-values.yml --namespace=${VC_MANAGED} --context=
echo
echo "--- Export vcluster kubeconfig for managed cluster"
vcluster connect ${VC_MANAGED} -n ${VC_MANAGED} --update-current=false --kube-config="${SHARED_DIR}/${VC_MANAGED}.kubeconfig"

#echo "--- vcluster kubeconfig data: "
#cat "${SHARED_DIR}/${VC_MANAGED}.kubeconfig"

echo "--- Import vcluster into hub as managed"
cm get clusters
cm attach cluster --cluster ${VC_MANAGED} --cluster-kubeconfig "${SHARED_DIR}/${VC_MANAGED}.kubeconfig"
oc label managedcluster -n ${VC_MANAGED} ${VC_MANAGED} vcluster=true
oc label ns ${VC_MANAGED} vcluster=true

echo "--- Show managed cluster"
sleep 3m
oc get managedclusters

echo "-- Creating vcluster to host compute service"
oc create ns ${VC_COMPUTE}
oc config current-context view | vcluster create ${VC_COMPUTE} --connect=false --expose --namespace=${VC_COMPUTE} --context=
echo "-- Sleep a few minutes while vcluster starts..."
sleep 5m
echo "-- Export vcluster kubeconfig for compute cluster"
vcluster connect ${VC_COMPUTE} -n ${VC_COMPUTE} --update-current=false --kube-config="${SHARED_DIR}/${VC_COMPUTE}.kubeconfig"
vcluster connect ${VC_COMPUTE} -n ${VC_COMPUTE}
echo "-- Check compute vcluster namespaces"
oc get ns
echo "-- compute vcluster disconnect"
vcluster disconnect

# echo "-- Creating vcluster to host KCP service"
# oc create ns ${VC_KCP}
# ## this fails on oc get ns due to dialup error
# # oc config current-context view | vcluster create ${VC_KCP} --expose --connect=true --namespace=${VC_KCP} -f vcluster-values.yml --context=
# # echo "-- Excplitily state context"
# # oc config current-context view
# # echo "-- Run without setting context"
# # oc get ns
# # vcluster disconnect
#
# oc config current-context view | vcluster create ${VC_KCP} --expose --connect=false --namespace=${VC_KCP} --context=
# echo "-- Sleep a few minutes while vcluster starts..."
# sleep 5m
# echo "-- Connect to and then export vcluster kubeconfig for kcp cluster, try oc get ns, and disconnect"
# vcluster connect ${VC_KCP} -n ${VC_KCP} --update-current=false --kube-config="${SHARED_DIR}/${VC_KCP}.kubeconfig"
# vcluster connect ${VC_KCP} -n ${VC_KCP}
# oc get ns
# vcluster disconnect


echo "-- Check namespaces"
oc get ns

echo "-- Check catalogsource"
oc get catalogsource -A


echo "-- Install cert-manager operator "
export CERT_MGR_TEMP_DIR=$(mktemp -d)

kubectl kustomize "./operators/cert-manager" > ${CERT_MGR_TEMP_DIR}/certmgr.yaml
kubectl apply -f ${CERT_MGR_TEMP_DIR}/certmgr.yaml


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

echo "-- Download KCP "
pushd ${KCP_REPO_TEMP_DIR}
git clone https://github.com/kcp-dev/kcp.git
pushd kcp
git checkout ${KCP_GIT_BRANCH}

echo "-- Install kubectl kcp plugin extensions"
make install

# Ensure the kcp plugins are installed
if [ "$(kubectl plugin list 2>/dev/null | grep -c 'kubectl-kcp')" -eq 0 ]; then
  echo "kcp plugin could not be found"
  exit 1
fi


# echo "-- Start kcp"
# # Maybe want to add: -v=6
# kcp start &>"${ARTIFACT_DIR}/kcp.log" &
# export KCP_PID=${!}
# echo "-- PID for kcp is ${KCP_PID}"
#
#
# sleep 10
# ls -al ${KCP_REPO_TEMP_DIR}/kcp/.kcp

# Copy the KCP .kcp contents to a shared location for easier use by other scripts
#mkdir -p "${KCP_KUBECONFIG_DIR}"
#cp "${KCP_REPO_TEMP_DIR}/kcp/.kcp/admin.kubeconfig"  "${KCP_KUBECONFIG}"

popd
popd


# Need to install containerized kcp
echo "-- Install containerized kcp"

#containerized KCP
CKCP_DIR="${SCRIPT_DIR}/kcp"


# #############################################################################
# # Deploy KCP
# #############################################################################
ckcp_manifest_dir=$CKCP_DIR
ckcp_dev_dir=$ckcp_manifest_dir/overlays/dev
ckcp_temp_dir=$ckcp_manifest_dir/overlays/temp

kcp_org="root:default"
#kcp_workspace="pipeline-service-compute"
# older versions of yq need the 'e' parameter
kcp_version="$(yq e '.images[] | select(.name == "kcp") | .newTag' "$SCRIPT_DIR/kcp/overlays/dev/kustomization.yaml")"

# To ensure kustomization.yaml file under overlays/temp won't be changed, remove the directory overlays/temp if it exists
if [ -d "$ckcp_temp_dir" ]; then
  rm -rf "$ckcp_temp_dir"
fi
cp -rf "$ckcp_dev_dir" "$ckcp_temp_dir"

domain_name="$(kubectl get ingresses.config/cluster -o jsonpath='{.spec.domain}')"
ckcp_route="ckcp-ckcp.$domain_name"
echo "
patches:
  - target:
      kind: Ingress
      name: ckcp
    patch: |-
      - op: add
        path: /spec/rules/0/host
        description: An ingress host needs to be defined which has the routing suffix of your cluster.
        value: $ckcp_route
  - target:
      kind: Deployment
      name: ckcp
    patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/env/0/value
        description: This value refers to the hostAddress defined in the Route.
        value: $ckcp_route
  - target:
      kind: Certificate
      name: kcp
    patch: |-
      - op: add
        path: /spec/dnsNames/-
        description: This value refers to the hostAddress defined in the Route.
        value: $ckcp_route " >>"$ckcp_temp_dir/kustomization.yaml"

echo -n "  - kcp $kcp_version: "
# older versions of kubectl will have kustomize errors. 4.10.20 works
kubectl apply -k "$ckcp_temp_dir" >/dev/null 2>&1
# Check if ckcp pod status is Ready
kubectl wait --for=condition=Ready -n ckcp pod -l=app=kcp-in-a-pod --timeout=90s >/dev/null 2>&1
# Clean up kustomize temp dir
rm -rf "$ckcp_temp_dir"

#############################################################################
# Post install
#############################################################################
# Copy the kubeconfig of kcp from inside the pod onto the local filesystem
podname="$(kubectl get pods --ignore-not-found -n ckcp -l=app=kcp-in-a-pod -o jsonpath='{.items[0].metadata.name}')"
mkdir -p "$(dirname "$KUBECONFIG_KCP")"
# Wait until admin.kubeconfig file is generated inside ckcp pod
while [[ $(kubectl exec -n ckcp "$podname" -- ls /etc/kcp/config/admin.kubeconfig >/dev/null 2>&1; echo $?) -ne 0 ]]; do
  echo -n "."
  sleep 5
done
kubectl cp "ckcp/$podname:/etc/kcp/config/admin.kubeconfig" "$KUBECONFIG_KCP" >/dev/null 2>&1
echo "OK"

# Check if external ip is assigned and replace kcp's external IP in the kubeconfig file
echo -n "  - Route: "
if grep -q "ckcp-ckcp.apps.domain.org" "$KUBECONFIG_KCP"; then
  yq e -i "(.clusters[].cluster.server) |= sub(\"ckcp-ckcp.apps.domain.org:6443\", \"$ckcp_route:443\")" "$KUBECONFIG_KCP"
fi
echo "OK"

# Workaround to prevent the creation of a new workspace until KCP is ready.
# This fixes `error: creating a workspace under a Universal type workspace is not supported`.
ws_name=$(echo "$kcp_org" | cut -d: -f2)
while ! KUBECONFIG="$KUBECONFIG_KCP" kubectl kcp workspace create "$ws_name" --type root:organization --ignore-existing >/dev/null; do
  sleep 5
done
KUBECONFIG="$KUBECONFIG_KCP" kubectl kcp workspace use "$ws_name"

#echo "  - Setup kcp access:"
#"$SCRIPT_DIR/../images/access-setup/content/bin/setup_kcp.sh" \
#  --kubeconfig "$KUBECONFIG_KCP" \
#  --kcp-org "$kcp_org" \
#  --kcp-workspace "$kcp_workspace" \
#  --work-dir "$WORK_DIR"
#KUBECONFIG_KCP="$WORK_DIR/credentials/kubeconfig/kcp/ckcp-ckcp.${ws_name}.${kcp_workspace}.kubeconfig"

# copy kcp KUBECONFIG to SHARED_DIRECTORY so it can be used by other tasks
cp  "{$KUBECONFIG_KCP}" "${KCP_KUBECONFIG}"
echo "=============== "

echo "-- Check namespaces"
oc get ns




echo "-- Test kcp api-resources"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl api-resources

echo "-- Show kcp context"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl config get-contexts

echo "-- Show current kcp workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl ws .

echo "-- Change to home kcp workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl ws

echo "-- Change to previous kcp workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl ws -

echo "-- Show cluster server for kcp"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl config view - o jsonpath='{.clusters[?(@.name == "root")].cluster.server}'

echo "-- Show current kcp workspace 2"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws .

# echo "-- Export vcluster kubeconfig for kcp cluster"
# vcluster connect ${VC_KCP} -n ${VC_KCP} --update-current=false --insecure --kube-config="${SHARED_DIR}/${VC_KCP}.kubeconfig"

# # Make sure the managed cluster is ready to be used
# echo "Waiting up to 15 minutes for managed cluster to be ready"
# _timeout=900 _elapsed='' _step=30
# while true; do
#     # Wait for _step seconds, except for first iteration.
#     if [[ -z "$_elapsed" ]]; then
#         _elapsed=0
#     else
#         sleep $_step
#         _elapsed=$(( _elapsed + _step ))
#     fi
#
#     mc_url=`oc get managedclusters --selector name!=local-cluster --no-headers -o jsonpath='{.items[0].spec.managedClusterClientConfigs[0].url}'`
#     if [[ ! -z "$mc_url" ]]; then
#         echo "Managed cluster is ready after ${_elapsed}s"
#         break
#     fi
#
#     # Check timeout
#     if (( _elapsed > _timeout )); then
#             echo "Timeout (${_timeout}s) managed cluster is not ready"
#             return 1
#     fi
#
#     echo "Managed cluster is not ready. Will retry (${_elapsed}/${_timeout}s)"
# done
#
# echo "--- Show managed cluster"
# oc get managedclusters

# echo "--- Configure OpenShift to use a signed certificate..."
# ./install-signed-cert.sh

# Location of repo in docker test image
export COMPUTE_OPERATOR_DIR=${COMPUTE_OPERATOR_DIR:-"/compute-operator"}

## Grab the repo contents
#idp_dir=$(mktemp -d -t idp-XXXXX)
#cd "$idp_dir" || exit 1
#export HOME="$idp_dir"

## Set up repo URLs.
## PULL_BASE_REF is a Prow variable as described here:
## https://github.com/kubernetes/test-infra/blob/master/prow/jobs.md#job-environment-variables
#echo "--- Cloning branch idp-mgmt-operator ${PULL_BASE_REF}"
#COMPUTE_OPERATOR_url="https://${COMPUTE_OPERATOR_REPO}.git"
#export COMPUTE_OPERATOR_DIR="${idp_dir}/idp-mgmt-operator"
#git clone -b "${PULL_BASE_REF}" "$COMPUTE_OPERATOR_url" "$COMPUTE_OPERATOR_DIR" || {
#    echo "ERROR Could not clone branch ${PULL_BASE_REF} from idp-mgmt-operator repo $idp_mgmt_operator_url"
#    exit 1
#}

# TEMP disable and install compute operator to hub cluster
# echo "-- Connect to compute vcluster"
# ls -alh "${SHARED_DIR}"
# vcluster connect ${VC_COMPUTE} -n ${VC_COMPUTE} --kube-config="${SHARED_DIR}/${VC_COMPUTE}.kubeconfig"

echo "-- Check hub cluster namespaces"
oc get ns

echo "--- Show managed clusters on hub"
oc get managedclusters


echo "--- Install compute operator on hub ..."
./install-compute-operator.sh

# vcluster disconnect

# echo "--- Running ginkgo E2E tests"
# ./run-ginkgo-e2e-tests.sh
#
#
#
# echo "--- Running Cypress tests"
# ./start-cypress-tests.sh
