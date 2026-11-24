FROM busybox:1.37.0-musl AS shell
FROM otel/opentelemetry-collector-contrib:0.117.0
COPY --from=shell /bin/busybox /bin/sh
COPY deploy/otel/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
