#!/bin/bash

# Copyright Red Hat

# Install OpenShift CLI.
echo "Installing oc and kubectl clis..."
curl -kLo oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.8.24/openshift-client-linux-4.8.24.tar.gz
mkdir oc-unpacked
tar -xzf oc.tar.gz -C oc-unpacked
chmod 755 ./oc-unpacked/oc
chmod 755 ./oc-unpacked/kubectl
mv ./oc-unpacked/oc /usr/local/bin/oc
mv ./oc-unpacked/kubectl /usr/local/bin/kubectl
rm -rf ./oc-unpacked ./oc.tar.gz

# Install jq to parse json within bash scripts
curl -o /usr/local/bin/jq http://stedolan.github.io/jq/download/linux64/jq
chmod +x /usr/local/bin/jq

# Install vcluster to deploy virtual clusters
curl -kL "https://github.com/loft-sh/vcluster/releases/latest" | sed -nE 's!.*"([^"]*vcluster-linux-amd64)".*!https://github.com\1!p' | xargs -n 1 curl -L -o vcluster && chmod +x vcluster;
mv vcluster /usr/local/bin/vcluster

echo 'set up complete'
