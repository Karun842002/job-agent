# Local tracing

Start the collector and Jaeger with `docker compose --profile observability up -d jaeger otel-collector`. Jaeger is available at `http://127.0.0.1:16686`. Set `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:4318/v1/traces` when starting the API or worker. The collector accepts OTLP on loopback ports 4317 and 4318 and routes operational traces to Jaeger.

Remote telemetry export is disabled pending a compliant backend (ADR-0007). The GenAI pipeline targets a local mock on port 4319 for synthetic verification only. Do not send real candidate data to this development stack until retention enforcement is verified.

The collector sends only spans with a GenAI operation attribute to the Langfuse pipeline. It removes unapproved attributes, events and status messages. Spans with links are dropped. The Go generation helper accepts a model identifier and token counts. Callers must avoid inserting sensitive content into the model identifier or other allowed telemetry metadata. Export failures must not alter business state.

To test the collector on Linux, start the observability profile and run:

```sh
gateway=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')
TEST_OTEL_MOCK_LISTEN="${gateway}:4319" \
TEST_OTEL_COLLECTOR="http://127.0.0.1:4318/v1/traces" \
go test -count=1 -run TestCollectorRedactsGeneration ./internal/platform/langfuse
```

The local trace stack is for synthetic data. Automatic 24-hour deletion across telemetry and backups is tracked in issue #27 and must be verified before ingesting real candidate data.
