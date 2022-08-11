# KCP in Openshift

This is adopted from what the AppStudio team is doing in https://github.com/openshift-pipelines/pipeline-service/tree/main/ckcp.  It is basically a
containerized KCP instance that gets started in a pod on the OpenShift cluster.

## Description

This script essentially does this :  

Short Version:

1. Run kcp in a container in an Openshift cluster.

Long Version:

1. Create ns, sa, and add appropriate scc.
2. Create deployment and service resources.
3. Copy kubeconfig from inside the pod to the local system.
4. Update route of kcp-service in the just copied admin.kubeconfig file.

