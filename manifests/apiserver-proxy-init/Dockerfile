FROM alpine:3.14

VOLUME /etc/business
VOLUME /etc/envoy

RUN apk upgrade --update-cache --available && \
    apk add openssl && \
    apk add --virtual build_deps gettext &&  \
    rm -rf /var/cache/apk/*

COPY . /proxy-init/
WORKDIR /proxy-init/
ENTRYPOINT ["/proxy-init/execute.sh"]

