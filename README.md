[comment]: # ( Copyright Red Hat )

# compute-operator

The Compute operator enables users to register clusters to kcp. Once registered, the Compute operator is responsible for maintaining a SyncTarget (WorkloadCluster) in the desired kcp Location workspace and installing and configuring the kcp syncer.

Please fork this repo and clone from the fork.  All of your work should be against the forked repo.

```bash
git clone https://github.com/<git username>/compute-operator.git
cd compute-operator
```

# Installing

## Prereqs

You must meet the following requirements:

- `kustomize` (ver. 4.2.0+)
- access to a current ACM/MCE/OCM hub (TODO: provide more specific requirements)
- access to a kcp instance

## Unit Test
To run a suite of unit tests using ginkgo, please refer to the documentation in the [Contributing Guide](CONTRIBUTING.md#pre-check-before-submitting-a-pr)

## Prepare your kubeconfigs

To use this operator, you will need kubeconfigs for a managed hub cluster and a KCP workspace.

### Generating a kubeconfig for your managed hub cluster

Use one of the following techniques to generate a kubeconfig:

A. Set a temporary KUBECONFIG and `oc login`
1. Run the following:
```bash
rm -f /tmp/managed-hub-cluster.kubeconfig
touch /tmp/managed-hub-cluster.kubeconfig
export KUBECONFIG=/tmp/managed-hub-cluster.kubeconfig
```
2.  `oc login` to the managed hub cluster
3.  `unset KUBECONFIG` or set it as before.

-OR-

B. Copy an existing context into a temp file
```bash
kubectl config view --context=<context_of_the_managed_hub_cluster> --minify --flatten > /tmp/managed-hub-cluster.kubeconfig
```

### Generating a kubeconfig for your kcp cluster

1. Login to kcp
2. Create a new workspace or enter an existing workspace where you will install the APIResourceSchema and create a ServiceAccount for this operator
3. Create a service account in your workspace for example:
```bash
# kubectl create serviceaccount sa_name -n sa_namespace
kubectl create serviceaccount compute-operator -n default
```
4. Generate the kubeconfig from this SA
```bash
# build/generate_kubeconfig_from_sa.sh sa_name sa_namespace kubeconfig_filename
build/generate_kubeconfig_from_sa.sh compute-operator default /tmp/kubeconfig-compute-operator.yaml
```
The location of the new kubeconfig will be displayed
```
New kubeconfig at /tmp/kubeconfig-compute-operator.yaml
```
5. Create a ClusterRole and ClusterRoleBinding for this service account
```bash
kubectl apply -f hack/compute/role.yaml
kubectl apply -f hack/compute/role_binding.yaml
```

## Run the code

You can deploy the code from this repo onto a cluster, or you can run it locally.


### Local development

To run the operator locally, you can:

```bash
export POD_NAMESPACE=compute-config
export HUB_KUBECONFIG=/tmp/managed-hub-cluster.kubeconfig
export KCP_KUBECONFIG=/tmp/kubeconfig-compute-operator.yaml
oc new-project $POD_NAMESPACE
make run-local
```

Once you've run the steps above, you are also set up to use the included [launch.json](.vscode/launch.json) file to run in the debugger. Ensure your default kubeconfig's current context is set to the cluster you'd like to run the controller against, or add a KUBECONFIG env var to the launch.json.

### Deploy operator to a cluster

1. Verify you are logged into the cluster where you'd like to deploy the operator. This does not need to be the hub cluster.

```bash
oc cluster-info
```

You can also deploy the cluster to kcp. Verify you are logged in to the workspace you created for the controller [above](https://github.com/stolostron/compute-operator#generating-a-kubeconfig-for-your-managed-hub-cluster).

```bash
kubectl kcp ws .
```

Your kcp workspace will need the deployments API available. Bind to any available kubernetes API export or set up your own SyncTarget, then confirm.
```bash
kubectl api-resources | grep deployment
```

2. From the cloned compute-operator directory:

```bash
export QUAY_USER=<your_user>
export IMG_TAG=<tag_you_want_to_use>
export IMG=quay.io/${QUAY_USER}/compute-operator:${IMG_TAG}
make docker-build docker-push deploy
```

If you are running on kcp, you will need to skip installing the webhook for now until Services are supported. Run the `make deploy` with SKIP_WEBHOOK=true.

```bash
SKIP_WEBHOOK=true make deploy
```


3. Verify the installer is running

There is one pod that should be running:

- compute-installer-controller-manager

Check using the following command:

```bash
oc get pods -n compute-config
```

#### Onboard a managed hub cluster

**Ensure the managed hub cluster meets the prereq listed in the [Prereqs section above](#prereqs)**


1. Follow the steps above in [Generating a kubeconfig for your managed hub cluster](#generating-a-kubeconfig-for-your-managed-hub-cluster)

2. Create config secret on the controller cluster to access the managed hub cluster.

- Login to the controller cluster
```bash
oc login
```
- Verify you are logged into the controller cluster
```bash
oc cluster-info
```
- Create the secret using the managed hub cluster kubeconfig
```bash
oc create secret generic <secret_name> --from-file=kubeconfig=/tmp/managed-hub-cluster.kubeconfig -n <controller_namespace>
```

3. Create the hub config on the controller cluster:

If you are running on kcp, you will first need to install the APIResourceSchema for HubConfig.
```bash
kubectl apply -f config/apiresourceschema/singapore.open-cluster-management.io_hubconfigs.yaml
```

Create the HubConfig:

```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: HubConfig
metadata:
  name: <name_of_your_hub>
  namespace: <controller_namespace>
spec:
  kubeconfigSecretRef:
    name: <above_secret_name>
' | oc apply -f -
```

- Restart the controller if the ClusterRegistrar CR was already created in order to take into account this new hub.

#### Start the Cluster Registration controller
1. Follow the steps above in [Generating a kubeconfig for your kcp cluster](#generating-a-kubeconfig-for-your-kcp-cluster)

2. Create the clusterregistrar on the controller cluster with the kcp kubeconfig secret as reference:

- switch to the controller cluster and Verify you are logged into the cluster
```bash
oc cluster-info
```

or if running on kcp:

```bash
kubectl kcp ws .
```

- Create the kubeconfig secret
The secret must have the kcp kubeconfig in key `kubeconfig`.

```bash
kubectl create secret generic kcp-kubeconfig -n compute-config --from-file=kubeconfig=/tmp/kubeconfig-compute-operator.yaml
```

If you are running on kcp, install the APIResourceSchema for ClusterRegistrar.
```bash
kubectl apply -f config/apiresourceschema/singapore.open-cluster-management.io_clusterregistrars.yaml
```

- Create the ClusterRegistrar
```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: ClusterRegistrar
metadata:
  name: cluster-reg
spec:
  computeService:
    computeKubeconfigSecretRef:
      name: kcp-kubeconfig
' | oc create -f -
```

4. Verify pods are running

There is now three pods that should be running

- compute-operator-installer-controller-manager
- compute-operator-manager
- compute-operator-webhook-service

Check using the following command:

```bash
oc get pods -n compute-config
```

If you are running on kcp, you will need to log in to the cluster where the deployment was synced in order to view the pods and logs.


**NOTE: Restart the `compute-operator-manager` pod
if you make any changes to the ClusterRegistrar or HubConfig.  This will allow the operator to onboard the new hub config.**

# Using
## Import a user cluster into controller cluster
1. Create and enter a new workspace in kcp

2. Create an APIBinding to the compute-apis APIExport

Edit the file hack/compute/apibinding.yaml spec.reference.workspace.path to point at the workspace you created above in [Generating a kubeconfig for your kcp cluster](#generating-a-kubeconfig-for-your-kcp-cluster)
```bash
kubectl apply -f hack/compute/apibinding.yaml
```
3. Confirm the RegisteredCluster API is now available
```bash
% k api-resources | grep registered
registeredclusters                             singapore.open-cluster-management.io/v1alpha1   true         RegisteredCluster
```

4. Create a registeredcluster CR in the kcp workspace

```bash
kubectl create ns <a_namespace>
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: RegisteredCluster
metadata:
  name: <your_cluster_name>
  namespace: <a_namespace>
spec: {}
' | oc create -f -
```

5. Import the user cluster

- In your kcp workspace, run `oc get configmap -n <your_namespace> <name_of_cluster_to_import>-import -o jsonpath='{.data.importCommand}'`
- Copy the results.   This is the command that needs to be run on the user cluster to trigger the import process. **NOTE: This is a very large command, ensure you copy it completely!**
- Login to the user cluster you want to import
- Verify you are logged into the user cluster you want to import
```bash
oc cluster-info
```
- Paste the result and run the commands
- Login to the controller cluster
- Verify you are logged into the controller cluster
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

## Listing user clusters that are imported into controller cluster
1. Verify you are logged into the controller cluster
```bash
oc cluster-info
```

2. List all registered clusters on the controller cluster

```bash
oc get registeredcluster -A
```
