#!/bin/sh

set -e
set +x

# each host: <ns>-<cls>.kube-api-server.external-crd.com
# generate certificate
mkdir -p /tmp/working
trap "rm -rf /tmp/working" EXIT TERM

echo "Generating server certificate..."
SCRIPT_PATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
(cd /tmp/working && $SCRIPT_PATH/certs/gen-server.sh kube-api-server.external-crd.com)
cp /tmp/working/server.* /etc/envoy/

echo "Generating envoy configuration..."
cp -R ./etc-envoy/* /etc/envoy/
for FILE in $(ls -1 /etc/business/*.json); do
  BUSINESS_CLUSTER=$(cat $FILE | ./jq -r '.clusterId')
  BUSINESS_NAMESPACE=$(cat $FILE | ./jq -r '.namespace')
  BUSINESS_APISERVER_HOST=$(cat $FILE | ./jq -r '.apiserver.host')
  BUSINESS_APISERVER_PORT=$(cat $FILE | ./jq -r '.apiserver.httpsPort')
  BUSINESS_APISERVER_TOKEN=$(cat $FILE | ./jq -r '.apiserver.token')
  BUSINESS_CRDSERVER_TOKEN=$(cat $FILE | ./jq -r '.externalCrdSAToken')

  rm -f /tmp/working/env
cat << DELIMITER > /tmp/working/env
export BUSINESS_CLUSTER="${BUSINESS_CLUSTER}"
export BUSINESS_NAMESPACE="${BUSINESS_NAMESPACE}"
export BUSINESS_APISERVER_HOST="${BUSINESS_APISERVER_HOST}"
export BUSINESS_APISERVER_PORT="${BUSINESS_APISERVER_PORT}"
export BUSINESS_APISERVER_TOKEN="${BUSINESS_APISERVER_TOKEN}"
export BUSINESS_CRDSERVER_TOKEN="${BUSINESS_CRDSERVER_TOKEN}"
DELIMITER

  (source /tmp/working/env && cat ./etc-envoy/dynamic/cds-tmpl.yaml | envsubst >> /etc/envoy/dynamic/cds.yaml)
  (source /tmp/working/env && cat ./etc-envoy/dynamic/rds-tmpl.yaml | envsubst >> /etc/envoy/dynamic/rds.yaml)
done
echo "Done."
# todo: watch configmap & generate cds.yaml & rds.yaml

