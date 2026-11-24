package langfuse

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestCollectorRedactsGeneration(t *testing.T) {
	listenAddress := os.Getenv("TEST_OTEL_MOCK_LISTEN")
	collectorURL := os.Getenv("TEST_OTEL_COLLECTOR")
	if listenAddress == "" || collectorURL == "" {
		t.Skip("collector integration requires TEST_OTEL_MOCK_LISTEN and TEST_OTEL_COLLECTOR")
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		t.Fatal(err)
	}
	bodies := make(chan []byte, 4)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expected := os.Getenv("TEST_OTEL_MOCK_PATH"); expected != "" && r.URL.Path != expected {
			t.Errorf("collector exported to path %q, want %q", r.URL.Path, expected)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			reader, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer reader.Close()
			input = reader
		}
		body, err := io.ReadAll(input)
		if err != nil {
			t.Error(err)
		}
		bodies <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}), ReadHeaderTimeout: 2 * time.Second}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())
	ctx := context.Background()
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(collectorURL))
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("secret.resource", "secret-value"))))
	_, ordinary := provider.Tracer("ordinary").Start(ctx, "not_genai")
	ordinary.End()
	_, span := provider.Tracer("secret-value", trace.WithInstrumentationVersion("secret-value"), trace.WithInstrumentationAttributes(attribute.String("secret.scope", "secret-value"))).Start(ctx, "secret-value",
		trace.WithAttributes(attribute.String("gen_ai.operation.name", "generate_content"),
			attribute.String("gen_ai.request.model", "fixture-model"),
			attribute.Int64("gen_ai.usage.input_tokens", 12),
			attribute.String("gen_ai.input.messages", "secret-value")))
	span.AddEvent("secret-value", trace.WithAttributes(attribute.String("secret.event", "secret-value")))
	span.SetStatus(codes.Error, "secret-value")
	span.End()
	_, linked := provider.Tracer("test").Start(ctx, "linked",
		trace.WithAttributes(attribute.String("gen_ai.operation.name", "generate_content")),
		trace.WithLinks(trace.Link{Attributes: []attribute.KeyValue{attribute.String("secret.link", "secret-value")}}))
	linked.End()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-bodies:
		if strings.Contains(string(body), "secret-value") {
			t.Fatal("collector exported sensitive data")
		}
		var req collectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		spans := req.ResourceSpans[0].ScopeSpans[0].Spans
		if len(spans) != 1 || spans[0].Name != "gen_ai.generation" {
			t.Fatalf("unexpected spans: %v", spans)
		}
		var count int64
		for _, attr := range spans[0].Attributes {
			if attr.Key == "gen_ai.usage.input_tokens" {
				count = attr.Value.GetIntValue()
			}
		}
		if count != 12 {
			t.Fatalf("input tokens = %d", count)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("collector did not export to mock endpoint")
	}
}
