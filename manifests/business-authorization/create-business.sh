#!/bin/bash

set -e

# the debug switch
set +x

RANDOM_STR=$(tr -dc a-z0-9 </dev/urandom | head -c 4)
BUSINESS_CLUSTER=$1
BUSINESS_NAMESPACE=$2
ORIGINAL_KUBECONFIG=$3
PROXY_APISERVER_HOST=${PROXY_APISERVER_BASE_HOST:-kube-api-server.external-crd.com}

if [ -z "${BUSINESS_CLUSTER}" ]; then
  echo "Please specify cluster id and namespace using parameters."
  exit 1
fi

if [ -z "${ORIGINAL_KUBECONFIG}" ]; then
  echo "Please specify business cluster kubeconfig file path as parameter 3."
  exit 1
fi

if [ ! -f "${ORIGINAL_KUBECONFIG}" ]; then
  echo "Can't find kubeconfig to business cluster."
  exit 1
fi

# todo: support client-key-data & client-certificate-data
BUSINESS_APISERVER_ADDR=$(kubectl --kubeconfig=$ORIGINAL_KUBECONFIG config view --raw -o json | ./jq -r '.clusters[0].cluster.server')
if [ "${BUSINESS_APISERVER_ADDR:0:8}" != "https://" ]; then
  echo "Only support original kube api server of https scheme."
  exit 1
fi

BUSINESS_APISERVER_TOKEN=$(kubectl --kubeconfig=$ORIGINAL_KUBECONFIG config view --raw -o json | ./jq -r '.users[0].user.token')
if [ "$BUSINESS_APISERVER_TOKEN" == "null" ]; then
  echo "Only support original kube api servers using token authorization."
  exit 1
fi

function generate_kubeconfig(){
  SA_NAME=$1

  # CA_DATA=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster["certificate-authority-data"]')
  # TLS_SERVER_NAME=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster["tls-server-name"]')
  # API_SERVER=$(kubectl config view --raw -o json | ./jq -r '.clusters[0].cluster.server')
  API_SERVER="${BUSINESS_NAMESPACE}-${BUSINESS_CLUSTER}.${PROXY_APISERVER_HOST}"

cat << DELIMITER > ./${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}.kubeconfig
apiVersion: v1
clusters:
- cluster:
    # certificate-authority-data: ${CA_DATA}
    # tls-server-name: ${TLS_SERVER_NAME}
    server: https://${API_SERVER}
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
    token: ${BUSINESS_APISERVER_TOKEN}
DELIMITER
}

function authorize_and_setup(){
  SA_NAME=$1

  SECRET_NAME=$(kubectl get -n external-crd-system serviceaccount ${SA_NAME} -o jsonpath='{.secrets[].name}')
  SECRET_TOKEN=$(kubectl get -n external-crd-system secret ${SECRET_NAME} -o go-template='{{.data.token | base64decode}}' && echo)

  generate_kubeconfig $SA_NAME

  SERVER_ADDR=${BUSINESS_APISERVER_ADDR#https://}
  BUSINESS_APISERVER_HOST=$(echo $SERVER_ADDR | cut -d ':' -f 1)
  BUSINESS_APISERVER_PORT=$(echo $SERVER_ADDR | cut -d ':' -f 2)
  if [ -z "$BUSINESS_APISERVER_PORT" ]; then
    BUSINESS_APISERVER_PORT=443
  fi

read BIZ_JSON_RAW < <((
cat << EOF
{
  "clusterId": "${BUSINESS_CLUSTER}",
  "namespace": "${BUSINESS_NAMESPACE}",
  "apiserver": {
    "token": "${BUSINESS_APISERVER_TOKEN}",
    "host": "${BUSINESS_APISERVER_HOST}",
    "httpsPort": ${BUSINESS_APISERVER_PORT}
  },
  "externalCrdSAToken": "${SECRET_TOKEN}"
}
EOF
) | ./jq -r tostring )

  EXISTING_CM=$(kubectl get -n external-crd-system configmap/apiserver-proxy-business-config -o Name || true)
  BIZ_JSON_NAME="${BUSINESS_NAMESPACE}-${BUSINESS_CLUSTER}.json"
  if [ ! -z "$EXISTING_CM" ]; then
    kubectl get -n external-crd-system configmap/apiserver-proxy-business-config -o json > /tmp/biz-cfg-backup.json
    EXISTING_KEY=$(cat /tmp/biz-cfg-backup.json  | jq -r ".data[\"${BIZ_JSON_NAME}\"]")

    if [ ! -z "$EXISTING_KEY" ]; then
      cat /tmp/biz-cfg-backup.json | ./jq -r "del(.data[\"${BIZ_JSON_NAME}\"])" > /tmp/biz-cfg-backup2.json
      mv -f /tmp/biz-cfg-backup2.json /tmp/biz-cfg-backup.json
    fi
    BIZ_JSON_ESCAPED=$(echo $BIZ_JSON_RAW | sed 's/"/\\"/g')
    cat /tmp/biz-cfg-backup.json | ./jq -r ".data += {\"${BIZ_JSON_NAME}\":\"${BIZ_JSON_ESCAPED}\"}" > /tmp/biz-cfg.json
  else
    echo "creating new configmap"
cat << EOF > /tmp/biz-cfg.json
kind: ConfigMap
apiVersion: v1
metadata:
  name: apiserver-proxy-business-config
  namespace: external-crd-system
data:
  ${BIZ_JSON_NAME}: |
    ${BIZ_JSON_RAW}
EOF
  fi

  kubectl apply -f /tmp/biz-cfg.json

  echo "Please get your kubeconfig from:"
  echo "./${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}.kubeconfig"
}

SA_NAME=
EXISTING=$(kubectl get ClusterRoleBinding "external-crd-biz-${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}" -o Name || true)
if [ ! -z "$EXISTING" ]; then
  SA_NAME=$(kubectl get serviceaccount -n external-crd-system -o Name -l k8s.jijiechen.com/cluster=${BUSINESS_CLUSTER} -l  k8s.jijiechen.com/namespace=${BUSINESS_NAMESPACE})
  if [ -z "$SA_NAME" ]; then
    # service account missing... delete the cluster role binding and let it be created later
    kubectl delete ClusterRoleBinding "external-crd-biz-${BUSINESS_CLUSTER}-${BUSINESS_NAMESPACE}"
    sleep 1
  else
    SA_NAME=$(echo $SA_NAME | cut -d '/' -f 2)
    echo "Reusing existing serviceaccount $SA_NAME"
    authorize_and_setup $SA_NAME
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
authorize_and_setup $SA_NAME

