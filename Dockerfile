FROM alpine

RUN apk update && \
    apk upgrade && \
    rm -rf /var/cache/apk/*

COPY ./bin/node-resource-exporter /usr/local/bin/node-resource-exporter

USER nobody

ENTRYPOINT /usr/local/bin/node-resource-exporter
