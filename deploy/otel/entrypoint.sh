#!/bin/sh
set -eu

# The synthetic receiver lives on the Docker host gateway. All other
# destinations must use TLS, and credentials belong in the header, not URLs.
case "${LANGFUSE_OTLP_ENDPOINT:-}" in
  *'@'*|*'#'*|*'?'*|*' '*|'')
    echo "invalid telemetry endpoint" >&2
    exit 1
    ;;
  http://host.docker.internal:4319/*|https://?*) ;;
  *)
    echo "telemetry endpoint must use HTTPS or the local mock" >&2
    exit 1
    ;;
esac

exec /otelcol-contrib "$@"
