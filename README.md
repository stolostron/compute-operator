[comment]: # ( Copyright Red Hat )

# cluster-registration-operator

This operator is in charge of creating a ManagedClusterSet for each AppStudio workspace.

Please fork this repo and clone from the fork.  All your work should be against the forked repo.

# Installing

## Prereqs

You must meet the following requirements:

- `kustomize` (ver. 4.2.0+)

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
export IMG=quay.io/<your_user>/cluster-registration-operator:<tag_you_want_to_use>
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