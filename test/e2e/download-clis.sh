#!/bin/bash

# Copyright Red Hat

# Install OpenShift CLI.
echo "Installing oc and kubectl clis..."
curl -kLo oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.10.20/openshift-client-linux-4.10.20.tar.gz
mkdir oc-unpacked
tar -xzf oc.tar.gz -C oc-unpacked
chmod 755 ./oc-unpacked/oc
chmod 755 ./oc-unpacked/kubectl
mv ./oc-unpacked/oc /usr/local/bin/oc
mv ./oc-unpacked/kubectl /usr/local/bin/kubectl
rm -rf ./oc-unpacked ./oc.tar.gz

echo "Installing helm..."
curl -kLo helm.tar.gz https://get.helm.sh/helm-v3.9.0-linux-amd64.tar.gz
mkdir helm-unpacked
tar -xzf helm.tar.gz -C helm-unpacked
chmod 755 ./helm-unpacked/linux-amd64/helm
mv ./helm-unpacked/linux-amd64/helm /usr/local/bin/helm

echo "Installing jq..."
# Install jq to parse json within bash scripts
curl -kLo /usr/local/bin/jq http://stedolan.github.io/jq/download/linux64/jq
chmod +x /usr/local/bin/jq

echo "Installing yq..."
# Install yq to parse yaml within bash scripts
curl -kLo /usr/local/bin/yq https://github.com/mikefarah/yq/releases/download/v4.5.1/yq_linux_amd64
chmod +x /usr/local/bin/yq

echo "Installing vcluster cli..."
# Install vcluster to deploy virtual clusters
curl -kLo /usr/local/bin/vcluster https://github.com/loft-sh/vcluster/releases/download/v0.11.0/vcluster-linux-amd64
chmod +x /usr/local/bin/vcluster

# Check vcluster installed properly and can be called
echo "Check install with vcluster --version"
vcluster --version

echo "Installing cm-cli..."
curl -kLo cm.tar.gz https://github.com/stolostron/cm-cli/releases/download/v1.0.14/cm_linux_amd64.tar.gz
mkdir cm-unpacked
tar -xzf cm.tar.gz -C cm-unpacked
chmod 755 ./cm-unpacked/cm
mv ./cm-unpacked/cm /usr/local/bin/cm
cm version

echo 'set up complete'
