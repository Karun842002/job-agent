// Package langfuse emits redacted GenAI spans through an OTLP/HTTP collector.
package langfuse

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var modelName = regexp.MustCompile(`^[A-Za-z0-9_./:-]{1,128}$`)

// NewProvider sends spans to an OTLP/HTTP endpoint with a bounded queue.
func NewProvider(ctx context.Context, endpoint string) (*sdktrace.TracerProvider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1/traces" {
		return nil, errors.New("telemetry endpoint must be a local collector")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsLoopback() {
		return nil, errors.New("telemetry endpoint must be a local collector")
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithTimeout(3*time.Second))
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "job-agent"))), sdktrace.WithBatcher(exporter,
		sdktrace.WithMaxQueueSize(512), sdktrace.WithMaxExportBatchSize(128))), nil
}

// RecordGeneration records only model metadata and token counts. Prompt and
// response text are deliberately absent from this API.
func RecordGeneration(ctx context.Context, tracer trace.Tracer, model string, inputTokens, outputTokens int64) error {
	if !modelName.MatchString(model) || inputTokens < 0 || outputTokens < 0 {
		return errors.New("invalid generation metadata")
	}
	_, span := tracer.Start(ctx, "gen_ai.generation", trace.WithAttributes(
		attribute.String("gen_ai.operation.name", "generate_content"),
		attribute.String("gen_ai.request.model", model),
		attribute.Int64("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int64("gen_ai.usage.output_tokens", outputTokens),
	))
	span.End()
	return nil
}
