[comment]: # ( Copyright Red Hat )
## Run E2E tests (Ginkgo) locally in OCP cluster

#### Prerequisites:
- An OCP cluster with ACM or MCE installed (Hub cluster)
- A managed cluster imported successfully into the hub cluster
- The Red Hat Certificate Manager operator installed and running on the hub cluster
- KCP running on the hub cluster (containerized kcp)

#### Steps to run tests:

1. Clone this repository and enter its root directory:
    ```
    git clone git@github.com:stolostron/compute-operator.git && cd compute-operator
    ```

2. Set configuration for the OCP Hub cluster:
   - export `KUBECONFIG` environment variable to the kubeconfig of the Hub OCP cluster:
     ```
     export KUBECONFIG=<kubeconfig-file-of-the-ocp-hub-cluster>
     ```
   - export `CLUSTER_SERVER_URL` environment variable to the cluster server URL of the Hub OCP cluster:
     ```
     export CLUSTER_SERVER_URL=<cluster-server-url-of-the-ocp-hub-cluster>
     ```

3. Set configuration for the managed cluster:
   - export `MANAGED_CLUSTER_KUBECONTEXT` environment variable to the kubecontext of the managed cluster:

        ```
        export MANAGED_CLUSTER_KUBECONTEXT=<kubecontext-of-the-managed-cluster>
        ```
    - optionally, you can copy the kubeconfig of the managed cluster into a new file and then set the `MANAGED_CLUSTER_KUBECONFIG` environment variable. (In this case, you don't need to set `MANAGED_CLUSTER_KUBECONTEXT`)
     Example:
      ```
      cp {IMPORT_CLUSTER_KUBE_CONFIG_PATH} ~/.kube/import-kubeconfig
      export MANAGED_CLUSTER_KUBECONFIG="~/.kube/import-kubeconfig"
      ```
   - export `MANAGED_CLUSTER_SERVER_URL` environment variable to the cluster server URL of the managed cluster:
     ```
     export MANAGED_CLUSTER_SERVER_URL=<cluster-server-url-of-the-managed-cluster>
     ```
   - export `MANAGED_CLUSTER_NAME` environment variable to the name of the managed cluster:
     ```
     export MANAGED_CLUSTER_NAME=<name-of-the-managed-cluster>
     ```

4. Then execute the following command to run e2e testing:

    ```
    make e2e-ginkgo-test
    ```
    NOTE: Or you may want to use
    ```
    ginkgo -tags e2e  test/e2e -- --ginkgo.vv --ginkgo.trace
    ```
