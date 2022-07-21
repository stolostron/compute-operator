#!/bin/bash
# Copyright Red Hat

# Update these to match your environment
# set -ex
set -e
CURRENT_KUBECONFIG=$1
SERVICE_ACCOUNT_NAME=$2
CONTEXT=$(kubectl config current-context --kubeconfig ${CURRENT_KUBECONFIG})
NAMESPACE=$3

NEW_CONTEXT=ws-kcp-context
KUBECONFIG_FILE=$4

CURRENT_DIR=`pwd`
echo $CURRENT_DIR

WORKING_DIR=`mktemp -d`

cd $WORKING_DIR
SECRET_NAME=$(kubectl get serviceaccount ${SERVICE_ACCOUNT_NAME} --kubeconfig ${CURRENT_KUBECONFIG} \
  --context ${CONTEXT} \
  --namespace ${NAMESPACE} \
  -o jsonpath='{.secrets[0].name}')
TOKEN_DATA=$(kubectl get secret ${SECRET_NAME} --kubeconfig ${CURRENT_KUBECONFIG} \
  --context ${CONTEXT} \
  --namespace ${NAMESPACE} \
  -o jsonpath='{.data.token}')

TOKEN=$(echo ${TOKEN_DATA} | base64 -d)

# Create dedicated kubeconfig
# Create a full copy
kubectl config view --raw --kubeconfig ${CURRENT_KUBECONFIG} > ${KUBECONFIG_FILE}.full.tmp
# Switch working context to correct context
kubectl --kubeconfig ${KUBECONFIG_FILE}.full.tmp config use-context ${CONTEXT}
# Minify
kubectl --kubeconfig ${KUBECONFIG_FILE}.full.tmp \
  config view --flatten --minify > ${KUBECONFIG_FILE}.tmp
# Rename context
kubectl config --kubeconfig ${KUBECONFIG_FILE}.tmp \
  rename-context ${CONTEXT} ${NEW_CONTEXT}
# Create token user
kubectl config --kubeconfig ${KUBECONFIG_FILE}.tmp \
  set-credentials ${CONTEXT}-${NAMESPACE}-token-user \
  --token ${TOKEN}
# Set context to use token user
kubectl config --kubeconfig ${KUBECONFIG_FILE}.tmp \
  set-context ${NEW_CONTEXT} --user ${CONTEXT}-${NAMESPACE}-token-user
# Set context to correct namespace
kubectl config --kubeconfig ${KUBECONFIG_FILE}.tmp \
  set-context ${NEW_CONTEXT} --namespace ${NAMESPACE}
# Flatten/minify kubeconfig
kubectl config --kubeconfig ${KUBECONFIG_FILE}.tmp \
  view --flatten --minify > ${KUBECONFIG_FILE}
# Remove tmp
rm ${KUBECONFIG_FILE}.full.tmp
rm ${KUBECONFIG_FILE}.tmp
cd $CURRENT_DIR
echo "New kubeconfig at "$KUBECONFIG_FILE
