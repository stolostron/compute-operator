[comment]: # ( Copyright Red Hat )

# cluster-registration-operator

The Cluster Registration operator enables users to register clusters to their AppStudio workspace. We leverage the [multicluster engine](https://stolostron.github.io/mce-docs/) to import each cluster and add it to a ManagedClusterSet per workspace.

Please fork this repo and clone from the fork.  All your work should be against the forked repo.

# Installing

## Prereqs

You must meet the following requirements:

- `kustomize` (ver. 4.2.0+)
- The managed hub must be MCE 2.0.0+
- On the managed hub, the multiclusterengine CR must have the managed-service-account enabled.
`oc edit multiclusterengine`
then set the following
```
    - enabled: true
      name: managed-service-account
```

## Ensure you are logged in to the correct cluster

```bash
kubectl cluster-info
```
## Install the operator from this repo

1. Fork and clone this repo

```bash
git clone https://github.com/<git username>/cluster-registration-operator.git
cd cluster-registration-operator
```

2. Verify you are logged into the right cluster

```bash
kubectl cluster-info
```

3. From the cloned cluster-registration-operator directory:

```bash
export QUAY_USER=<your_user>
export IMG_TAG=<tag_you_want_to_use>
export IMG=quay.io/${QUAY_USER}/cluster-registration-operator:${IMG_TAG}
make docker-build docker-push deploy
```

4. Verify the installer is running

There is one pod that should be running:

- cluster-registration-installer-controller-manager

Check using the following command:

```bash
oc get pods -n cluster-reg-config
```

5. Create clusterregistrar

```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: ClusterRegistrar
metadata:
  name: cluster-reg
  namespace: cluster-reg-config
spec:' | kubectl create -f -
```

6. Verify pods are running

There is now three pods that should be running

- cluster-registration-installer-controller-manager
- cluster-registration-operator-manager

Check using the following command:

```bash
oc get pods -n cluster-reg-config
```

# Onboard a hub cluster

## hub cluster pre-req
- The managed hub must be MCE 2.0.0+
- On the managed hub, the multiclusterengine CR must have the managed-service-account enabled.
`oc edit multiclusterengine`
then set the following
```
    - enabled: true
      name: managed-service-account
```
## Onboarding

1. Create config secret on the external cluster to access the hub cluster:

To get the kubeconfig:

- `export KUBECONFIG=$(mktemp)`
- `oc login` to the hub cluster
- `unset KUBECONFIG` or set it as before.

```bash
`oc login` to the external cluster
oc create secret generic <secret_name> --from-file=kubeconfig=${KUBECONFIG} -n <your_namespace> # Expects a kubeconfig file named kubeconfig
```

2. Create the hub config:
```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: HubConfig
metadata:
  name: <name_of_your_hub>
  namespace: <your_namespace>
spec:
  kubeConfigSecretRef: 
    name: <above_secret_name>
' | kubectl create -f -
```

3. Restart the `cluster-registration-operator-manager` pods
This will allow the operator to onboard the new hub config.

# Import a cluster

1. Create the registeredcluster CR

```bash
echo 'apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: RegisteredCluster
metadata:
  name: <name_of_cluster_to_import>
  namespace: <your_namespace>
spec: {}
' | kubectl create -f -
```

2. Import the cluster

- Run `oc get cm -n <your_namespace> <name_of_cluster_to_import>-import -o jsonpath='{.data.importCommand}'`
- Copy the result
- Login to the cluster to import
- Paste the result
# Local development

To run the operator locally, you can:

```bash
make generate
oc apply -f config/crd/singapore.open-cluster-management.io_registeredclusters.yaml
oc apply -f config/crd/singapore.open-cluster-management.io_hubconfigs.yaml
oc apply -f hack/hubconfig.yaml
oc create secret generic mce-kubeconfig-secret --from-file=kubeconfig=kubeconfig # Expects a kubeconfig file named kubeconfig
export POD_NAMESPACE=default
go run main.go manager
```
