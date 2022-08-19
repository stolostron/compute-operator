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
# Install Containerixed kcp into current OpenShift
#     oc login should already be done
###############################################################################
echo "=== Preparing to install containerized kcp ==="

export KCP_GIT_BRANCH=${KCP_GIT_BRANCH:-"v0.7.6"}
export KCP_SYNCER_IMAGE=${KCP_SYNCER_IMAGE:-"ghcr.io/kcp-dev/kcp/syncer:v0.7.6"}
export KCP_REPO_TEMP_DIR=$(mktemp -d)

KCP_KUBECONFIG_TEMP_DIR=$(mktemp -d)
echo "kcp kubeconfig directory is ${KCP_KUBECONFIG_TEMP_DIR}"
export KCP_KUBECONFIG_DIR=${KCP_KUBECONFIG_DIR:-$KCP_KUBECONFIG_TEMP_DIR}
if [ ! -d ${KCP_KUBECONFIG_DIR} ]; then
  mkdir -p ${KCP_KUBECONFIG_DIR}
fi
export KCP_KUBECONFIG=${KCP_KUBECONFIG:-${KCP_KUBECONFIG_DIR}/admin.kubeconfig}

# code from AppStudio used a different name so just map for now
KUBECONFIG_KCP="${KCP_KUBECONFIG}"

echo "--- Check current hub cluster info and current context"
oc cluster-info
oc config get-contexts

echo "-- Check namespaces"
oc get ns

echo "-- Download KCP "
pushd ${KCP_REPO_TEMP_DIR}
git clone https://github.com/kcp-dev/kcp.git
pushd kcp
echo " -- Switching to branch/tag ${KCP_GIT_BRANCH}"
git checkout ${KCP_GIT_BRANCH}

echo "-- Install kubectl kcp plugin extensions"
make install

# Ensure the kcp plugins are installed
if [ "$(kubectl plugin list 2>/dev/null | grep -c 'kubectl-kcp')" -eq 0 ]; then
  echo "kcp plugin could not be found"
  exit 1
fi

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

kcp_org=${ORGANIZATION_WORKSPACE:-"root:my-org"}
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

echo -n "  - kcp branch/tag $kcp_version: "
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

echo "-- kcp kubeconfig is located at $KUBECONFIG_KCP"
echo "=============== kcp kubeconfig ===================="
cat "$KUBECONFIG_KCP"
echo "=============== kcp kubeconfig ===================="


# Check if external ip is assigned and replace kcp's external IP in the kubeconfig file
echo -n "  - Route: "
if grep -q "ckcp-ckcp.apps.domain.org" "$KUBECONFIG_KCP"; then
  yq e -i "(.clusters[].cluster.server) |= sub(\"ckcp-ckcp.apps.domain.org:6443\", \"$ckcp_route:443\")" "$KUBECONFIG_KCP"
fi
echo "OK"

# Workaround to prevent the creation of a new workspace until KCP is ready.
# This fixes `error: creating a workspace under a Universal type workspace is not supported`.
ws_name=$(echo "$kcp_org" | cut -d: -f2)
while ! KUBECONFIG="$KUBECONFIG_KCP" kubectl kcp workspace create "$ws_name" --type root:organization --ignore-existing >/dev/null 2>&1; do
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

echo "=============== "

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

echo "-- containerized kcp is installed and running.  kcp kubeconfig is located at $KUBECONFIG_KCP"
