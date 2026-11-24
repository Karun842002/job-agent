# Local tracing

Start the collector and Jaeger with `docker compose --profile observability up -d jaeger otel-collector`. Jaeger is available at `http://127.0.0.1:16686`. Set `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:4318/v1/traces` when starting the API or worker. The collector accepts OTLP on loopback ports 4317 and 4318 and routes operational traces to Jaeger.

The application exporter accepts only a numeric loopback HTTP collector URL ending in `/v1/traces`. The GenAI pipeline defaults to a local mock on port 4319. For an authorized Langfuse deployment, set `LANGFUSE_OTLP_ENDPOINT` to its full OTLP/HTTP trace endpoint and `LANGFUSE_OTLP_AUTHORIZATION` to its authorization header before starting the collector. Obtain credentials from a secret manager. ADR-0010 lifts the retention-based gate; other privacy and authorization controls still apply.

The collector sends only spans with a GenAI operation attribute to the Langfuse pipeline. It removes unapproved attributes, events and status messages. Spans with links are dropped. The Go generation helper accepts a model identifier and token counts. Callers must avoid inserting sensitive content into the model identifier or other allowed telemetry metadata. Export failures must not alter business state.

To test the collector on Linux, start the observability profile and run:

```sh
gateway=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')
TEST_OTEL_MOCK_LISTEN="${gateway}:4319" \
TEST_OTEL_COLLECTOR="http://127.0.0.1:4318/v1/traces" \
go test -count=1 -run TestCollectorRedactsGeneration ./internal/platform/langfuse
```

See [current retention behavior](RETENTION.md); tool defaults may retain copies indefinitely.
