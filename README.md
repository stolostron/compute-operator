[comment]: # ( Copyright Red Hat )

# compute-operator

The Cluster Registration operator enables users to register clusters to their AppStudio workspace. We leverage the [multicluster engine](https://stolostron.github.io/mce-docs/) to import each cluster and add it to a ManagedClusterSet per workspace.

Please fork this repo and clone from the fork.  All your work should be against the forked repo.

# Installing

## Prereqs

You must meet the following requirements:

- `kustomize` (ver. 4.2.0+)
- The managed hub must be MCE 2.0.0+
- On the managed hub, the multiclusterengine CR must have the managedserviceaccount-preview enabled. Ensure you are logged into the correct managed hub cluster:
```bash
oc cluster-info
```
and then use one of the two methods shown below to make the change:
   - Manually edit using `oc edit multiclusterengine`
     then ensure the following:
```
    - enabled: true
      name: managedserviceaccount-preview
```
   - Run a command to make the change:
```
oc patch multiclusterengine multiclusterengine --type=merge -p '{"spec":{"overrides":{"components":[{"name":"managedserviceaccount-preview","enabled":true}]}}}'
```
or
```
cm enable component managedserviceaccount-preview
cm get components #to verify
```
## Ensure you are logged in to the AppStudio cluster

```bash
oc cluster-info
```

## Install the operator from this repo
**NOTE: This step is only required if you have not used the [infra-deployments repo](https://github.com/redhat-appstudio/infra-deployments) to deploy Cluster Registration and the other AppStudio pieces to your cluster**

1. Fork and clone this repo

```bash
git clone https://github.com/<git username>/compute-operator.git
cd compute-operator
```

2. Verify you are logged into the AppStudio cluster

```bash
oc cluster-info
```

3. From the cloned compute-operator directory:

```bash
export QUAY_USER=<your_user>
export IMG_TAG=<tag_you_want_to_use>
export IMG=quay.io/${QUAY_USER}/compute-operator:${IMG_TAG}
make docker-build docker-push deploy
```

4. Verify the installer is running

There is one pod that should be running:

- compute-installer-controller-manager

Check using the following command:

```bash
oc get pods -n compute-config
```


## Onboard a managed hub cluster

**Ensure the managed hub cluster meets the prereq listed in the [Prereqs section above](https://github.com/stolostron/compute-operator#prereqs)**


1. Get the kubeconfig of the managed hub cluster:
```bash
rm -rf /tmp/managed-hub-cluster
mkdir -p /tmp/managed-hub-cluster
touch /tmp/managed-hub-cluster/kubeconfig
export KUBECONFIG=/tmp/managed-hub-cluster/kubeconfig
```
- `oc login` to the managed hub cluster
- `unset KUBECONFIG` or set it as before.

2. Create config secret on the AppStudio cluster to access the managed hub cluster.
- Login to the AppStudio cluster
```bash
oc login
```
- Verify you are logged into the AppStudio cluster
```bash
oc cluster-info
```
- Create the secret using the managed hub cluster kubeconfig
```bash
oc create secret generic <secret_name> --from-file=kubeconfig=/tmp/managed-hub-cluster/kubeconfig -n <your_namespace>
```

## Start the Cluster Registration controller
1. Verify you are logged into the AppStudio cluster
```bash
oc cluster-info
```

2. Create the hub config on the AppStudio cluster:
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
' | oc create -f -
```

3. Create the clusterregistrar on the AppStudio cluster:

```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: ClusterRegistrar
metadata:
  name: cluster-reg
spec:' | oc create -f -
```

4. Verify pods are running

There is now three pods that should be running

- cluster-registration-installer-controller-manager
- compute-operator-manager
- cluster-registration-webhook-service

Check using the following command:

```bash
oc get pods -n cluster-reg-config
```


**NOTE: Restart the `compute-operator-manager` pod
if you make any changes to the HubConfig.  This will allow the operator to onboard the new hub config.**

## Import a user cluster into AppStudio cluster
1. Verify you are logged into the AppStudio cluster
```bash
oc cluster-info
```

2. Create a registeredcluster CR on the AppStudio cluster

```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: RegisteredCluster
metadata:
  name: <name_of_cluster_to_import>
  namespace: <your_namespace>
spec: {}
' | oc create -f -
```

3. Import the user cluster

- On the AppStudio cluster, run `oc get configmap -n <your_namespace> <name_of_cluster_to_import>-import -o jsonpath='{.data.importCommand}'`
- Copy the results.   This is the command that needs to be run on the user cluster to trigger the import process. **NOTE: This is a very large command, ensure you copy it completely!**
- Login to the user cluster you want to import
- Verify you are logged into the user cluster you want to import
```bash
oc cluster-info
```
- Paste the result and run the commands
- Login to the AppStudio cluster
- Verify you are logged into the AppStudio cluster
```bash
oc cluster-info
```
- Watch the status.conditions of the RegisteredCluster CR. After several minutes the cluster should be successfully imported.
```bash
oc get registeredcluster -n <your_namespace> -oyaml
```
- The staus.clusterSecretRef will point to the Secret, <name_of_cluster_to_import>-cluster-secret ,containing the kubeconfig of the user cluster in data.kubeconfig.
```bash
oc get secrets <name_of_cluster_to_import>-cluster-secret -n <your_namespace> -ojsonpath='{.data.kubeconfig}' | base64 -d
```

# Listing user clusters that are imported into AppStudio cluster
1. Verify you are logged into the AppStudio cluster
```bash
oc cluster-info
```

2. List all registered clusters on the AppStudio cluster

```bash
oc get registeredcluster -A
```

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
