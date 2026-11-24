#!/bin/sh
set -eu

# --version exits before any network connection. Use synthetic addresses.
LANGFUSE_OTLP_ENDPOINT='https://telemetry.example/v1/traces' \
  docker compose --profile observability run --rm --no-deps otel-collector --version >/dev/null
LANGFUSE_OTLP_ENDPOINT='http://host.docker.internal:4319/v1/traces' \
  docker compose --profile observability run --rm --no-deps otel-collector --version >/dev/null

for endpoint in 'http://telemetry.example/v1/traces' \
  'https://user:secret@telemetry.example/v1/traces' \
  'https://telemetry.example/v1/traces?token=synthetic'; do
  if LANGFUSE_OTLP_ENDPOINT="$endpoint" \
    docker compose --profile observability run --rm --no-deps otel-collector --version >/dev/null 2>&1; then
    echo "unsafe telemetry endpoint was accepted" >&2
    exit 1
  fi
done
