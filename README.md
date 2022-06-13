[comment]: # ( Copyright Red Hat )

# compute-operator

The Compute operator ....

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

- compute-operator-installer-controller-manager

Check using the following command:

```bash
oc get pods -n cluster-reg-config
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

2. Create the Compute config on the AppStudio cluster:
```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: Compute
metadata:
  name: <name_of_your_hub>
  namespace: <your_namespace>
spec: {}
' | oc create -f -
```

3. Create the clusterregistrar on the AppStudio cluster:

```bash
echo '
apiVersion: singapore.open-cluster-management.io/v1alpha1
kind: ComputeConfig
metadata:
  name: compute-config
spec:' | oc create -f -
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


**NOTE: Restart the `compute-operator-manager` pod
if you make any changes to the HubConfig.  This will allow the operator to onboard the new hub config.**
