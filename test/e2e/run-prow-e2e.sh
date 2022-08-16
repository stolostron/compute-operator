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

# Location of repo in docker test image
export COMPUTE_OPERATOR_DIR=${COMPUTE_OPERATOR_DIR:-"/compute-operator"}

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
mkdir -p ${KCP_KUBECONFIG_DIR}
export KCP_KUBECONFIG="${KCP_KUBECONFIG_DIR}/admin.kubeconfig"
export KCP_SA_KUBECONFIG="${KCP_SA_KUBECONFIG}/sa.admin.kubeconfig"

# code from AppStudio used a different name so just map for now
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

echo "--- Show managed clusters"
oc get managedclusters


# # Install vcluster
# export VC_MANAGED=vc-managed
# export VC_COMPUTE=vc-compute
# export VC_KCP=vc-kcp
#
# cat <<EOF > vcluster-values.yml
# openshift:
#   enable: true
# sync:
#   networkpolicies:
#     enabled: true
#   serviceaccounts:
#     enabled: true
#   services:
#     syncServiceSelector: true
# EOF
#
# echo "-- Creating a vcluster to import as a managed cluster"
# oc create ns ${VC_MANAGED}
# #vcluster create ${VC_MANAGED} --connect=false --namespace=${VC_MANAGED}
# oc config current-context view | vcluster create ${VC_MANAGED} --connect=false --expose -f vcluster-values.yml --namespace=${VC_MANAGED} --context=
# echo
# echo "--- Export vcluster kubeconfig for managed cluster"
# vcluster connect ${VC_MANAGED} -n ${VC_MANAGED} --update-current=false --kube-config="${SHARED_DIR}/${VC_MANAGED}.kubeconfig"
#
# #echo "--- vcluster kubeconfig data: "
# #cat "${SHARED_DIR}/${VC_MANAGED}.kubeconfig"
#
# echo "--- Import vcluster into hub as managed"
# cm get clusters
# cm attach cluster --cluster ${VC_MANAGED} --cluster-kubeconfig "${SHARED_DIR}/${VC_MANAGED}.kubeconfig"
# oc label managedcluster -n ${VC_MANAGED} ${VC_MANAGED} vcluster=true
# oc label ns ${VC_MANAGED} vcluster=true
#
# echo "--- Show managed cluster"
# sleep 3m
# oc get managedclusters
#
# echo "-- Creating vcluster to host compute service"
# oc create ns ${VC_COMPUTE}
# oc config current-context view | vcluster create ${VC_COMPUTE} --connect=false --expose --namespace=${VC_COMPUTE} --context=
# echo "-- Sleep a few minutes while vcluster starts..."
# sleep 5m
# echo "-- Export vcluster kubeconfig for compute cluster"
# vcluster connect ${VC_COMPUTE} -n ${VC_COMPUTE} --update-current=false --kube-config="${SHARED_DIR}/${VC_COMPUTE}.kubeconfig"
# vcluster connect ${VC_COMPUTE} -n ${VC_COMPUTE}
# echo "-- Check compute vcluster namespaces"
# oc get ns
# echo "-- compute vcluster disconnect"
# vcluster disconnect
#
# # echo "-- Creating vcluster to host KCP service"
# # oc create ns ${VC_KCP}
# # ## this fails on oc get ns due to dialup error
# # # oc config current-context view | vcluster create ${VC_KCP} --expose --connect=true --namespace=${VC_KCP} -f vcluster-values.yml --context=
# # # echo "-- Excplitily state context"
# # # oc config current-context view
# # # echo "-- Run without setting context"
# # # oc get ns
# # # vcluster disconnect
# #
# # oc config current-context view | vcluster create ${VC_KCP} --expose --connect=false --namespace=${VC_KCP} --context=
# # echo "-- Sleep a few minutes while vcluster starts..."
# # sleep 5m
# # echo "-- Connect to and then export vcluster kubeconfig for kcp cluster, try oc get ns, and disconnect"
# # vcluster connect ${VC_KCP} -n ${VC_KCP} --update-current=false --kube-config="${SHARED_DIR}/${VC_KCP}.kubeconfig"
# # vcluster connect ${VC_KCP} -n ${VC_KCP}
# # oc get ns
# # vcluster disconnect


echo "-- Check namespaces"
oc get ns

echo "-- Check catalogsource"
oc get catalogsource -A

./install-cert-manager.sh

./install-containerized-kcp.sh

echo "-- kcp kubeconfig file is ${KCP_KUBECONFIG}"

echo "== Configure kcp for use by compute-operator"

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

#echo "-- Show cluster server for kcp"
#KUBECONFIG="${KCP_KUBECONFIG}" kubectl config view -o jsonpath='{.clusters[?(@.name == "root")].cluster.server}'

echo "-- Use kcp workspace ${ORGANIZATION_WORKSPACE}"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws use ${ORGANIZATION_WORKSPACE}

echo "-- Create service account ${CONTROLLER_COMPUTE_SERVICE_ACCOUNT}"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl create serviceaccount ${CONTROLLER_COMPUTE_SERVICE_ACCOUNT}

IDENTITY_HASH=`KUBECONFIG="${KCP_KUBECONFIG}" kubectl get apibindings workload.kcp.dev -o jsonpath='{.status.boundResources[?(@.resource=="synctargets")].schema.identityHash}'`
echo "IdentityHash: ${IDENTITY_HASH}"

pushd ${COMPUTE_OPERATOR_DIR}
echo "\nIdentityHash: ${IDENTITY_HASH}" >> resources/compute-templates/hack-values.yaml

echo "-- make samples"
make samples

echo "-- Apply role and rolebinding"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply -f hack/compute/role.yaml
KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply -f hack/compute/role_binding.yaml

KUBECONFIG="${KCP_KUBECONFIG}" build/generate_kubeconfig_from_sa.sh ${KCP_KUBECONFIG} ${CONTROLLER_COMPUTE_SERVICE_ACCOUNT} default ${KCP_SA_KUBECONFIG}

IDENTITY_HASH2=`KUBECONFIG="${KCP_KUBECONFIG}" kubectl get apibindings workload.kcp.dev -o jsonpath='{.status.boundResources[?(@.resource=="synctargets")].schema.identityHash}'`

echo "IdentityHash(2): ${IDENTITY_HASH}"

echo "-- Test kcp api-resources"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl api-resources


echo "-- Change to org workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp use ${ORGANIZATION_WORKSPACE}
echo "-- Create compute workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws create ${COMPUTE_WORKSPACE} --enter
KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply -f hack/compute/apibinding.yaml
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws ..
echo "-- Create location workspace"
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws create ${LOCATION_WORKSPACE} --enter
KUBECONFIG="${KCP_KUBECONFIG}" kubectl apply -f hack/compute/apibinding.yaml
KUBECONFIG="${KCP_KUBECONFIG}" kubectl kcp ws ..

popd












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
