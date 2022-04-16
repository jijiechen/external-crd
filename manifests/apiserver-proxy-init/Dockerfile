FROM busybox:latest

VOLUME /etc/business
VOLUME /etc/envoy

COPY . /proxy-init/
ENTRYPOINT ["/proxy-init/execute.sh"]

