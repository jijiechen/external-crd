#!/bin/bash

set -e

# the debug switch
# set -x

RANDOM_STR=$(tr -dc a-z0-9 </dev/urandom | head -c 4)
BUSINESS_CLUSTER=$1
BUSINESS_NAMESPACE=$2

if [ -z "${BUSINESS_CLUSTER}" ]; then
  echo "Please specify cluster id and namespace using parameters."
  exit 1
fi

function generate_kubeconfig(){
  SA_NAME=$1

  SECRET_NAME=$(kubectl get -n external-crd-system serviceaccount ${SA_NAME} -o jsonpath='{.secrets[].name}')
  TOKEN=$(kubectl get -n external-crd-system secret ${SECRET_NAME} -o go-template='{{.data.token | base64decode}}' && echo)
  CA_DATA=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster["certificate-authority-data"]')
  TLS_SERVER_NAME=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster["tls-server-name"]')
  API_SERVER=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster.server')

cat << DELIMITER > ./${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}.kubeconfig
apiVersion: v1
clusters:
- cluster:
    # certificate-authority-data: ${CA_DATA}
    # tls-server-name: ${TLS_SERVER_NAME}
    server: ${API_SERVER}
    insecure-skip-tls-verify: true
  name: kube
contexts:
- context:
    cluster: kube
    namespace: external-crd-system
    user: ${SA_NAME}
  name: kube
current-context: kube
kind: Config
preferences: {}
users:
- name: ${SA_NAME}
  user:
    token: ${TOKEN}
DELIMITER
}

SA_NAME=
EXISTING=$(kubectl get ClusterRoleBinding "external-crd-biz-${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}" -o Name || true)
if [ ! -z "$EXISTING" ]; then
  SA_NAME=$(kubectl get serviceaccount -n external-crd-system -o Name -l k8s.jijiechen.com/cluster=cluster-abcd -l  k8s.jijiechen.com/namespace=ns-xyz)
  if [ -z "$SA_NAME" ]; then
    # service account missing... delete the cluster role binding and let it be created later
    kubectl delete ClusterRoleBinding "external-crd-biz-${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}"
    sleep 1
  else
    SA_NAME=$(echo $SA_NAME | cut -d '/' -f 2)
    generate_kubeconfig $SA_NAME
    exit 0
  fi
fi

SA_NAME="biz-${RANDOM_STR}-${BUSINESS_CLUSTER}-${RANDOM_STR}-${BUSINESS_NAMESPACE}"
(
cat << EOF
kind: ServiceAccount
apiVersion: v1
metadata:
  name: ${SA_NAME}
  labels:
    k8s.jijiechen.com/cluster: ${BUSINESS_CLUSTER}
    k8s.jijiechen.com/namespace: ${BUSINESS_NAMESPACE}
  namespace: external-crd-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: external-crd-biz-${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: external-crd
subjects:
  - kind: ServiceAccount
    name: ${SA_NAME}
    namespace: external-crd-system
EOF
) | kubectl create -f -

sleep 1
generate_kubeconfig $SA_NAME

