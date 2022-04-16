#!/bin/bash

# each host: <ns>-<cls>.kube-api-server.external-crd.com
# generate certificate
mkdir -p /tmp/certs
(cd /tmp/certs && ./certs/gen-server.sh kube-api-server.external-crd.com)
cp /tmp/certs/server.* /etc/envoy/

cp ./etc-envoy/* /etc/envoy/
for FILE -n $(ls -1 /etc/business/*.json); do
  BUSINESS_CLUSTER=$(cat /etc/business/$FILE | ./jq -r '.clusterId')
  BUSINESS_NAMESPACE=$(cat /etc/business/$FILE | ./jq -r '.namespace')
  BUSINESS_APISERVER_HOST=$(cat /etc/business/$FILE | ./jq -r '.apiserver.host')
  BUSINESS_APISERVER_PORT=$(cat /etc/business/$FILE | ./jq -r '.apiserver.httpsPort')
  BUSINESS_APISERVER_TOKEN=$(cat /etc/business/$FILE | ./jq -r '.apiserver.token')
  BUSINESS_CRDSERVER_TOKEN=$(cat /etc/business/$FILE | ./jq -r '.externalCrdSAToken')

  (BUSINESS_CLUSTER="${BUSINESS_CLUSTER}" BUSINESS_NAMESPACE="${BUSINESS_NAMESPACE}" BUSINESS_APISERVER_HOST="${BUSINESS_APISERVER_HOST}" BUSINESS_APISERVER_PORT="${BUSINESS_APISERVER_PORT}" BUSINESS_APISERVER_TOKEN="${BUSINESS_APISERVER_TOKEN}" BUSINESS_CRDSERVER_TOKEN="${BUSINESS_CRDSERVER_TOKEN}" envsubst ./etc-envoy/dynamic/cds.tmpl.yaml >> /etc/envoy/dynamic/cds.yaml)
  (BUSINESS_CLUSTER="${BUSINESS_CLUSTER}" BUSINESS_NAMESPACE="${BUSINESS_NAMESPACE}" BUSINESS_APISERVER_HOST="${BUSINESS_APISERVER_HOST}" BUSINESS_APISERVER_PORT="${BUSINESS_APISERVER_PORT}" BUSINESS_APISERVER_TOKEN="${BUSINESS_APISERVER_TOKEN}" BUSINESS_CRDSERVER_TOKEN="${BUSINESS_CRDSERVER_TOKEN}" envsubst ./etc-envoy/dynamic/rds.tmpl.yaml >> /etc/envoy/dynamic/rds.yaml)
done
# todo: watch configmap & generate cds.yaml & rds.yaml

