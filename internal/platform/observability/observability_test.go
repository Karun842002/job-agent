package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestJSONLogging(t *testing.T) {
	var buf bytes.Buffer
	Logger(&buf).Info("startup", "service", "api", "email", "person@example.com", "error", "Bearer secret")
	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if _, ok := event["email"]; ok {
		t.Fatal("sensitive attribute was logged")
	}
	if _, ok := event["error"]; ok {
		t.Fatal("untrusted error was logged")
	}
	if event["msg"] != "startup" || event["service"] != "api" {
		t.Fatalf("event: %v", event)
	}
}

func TestTracePropagation(t *testing.T) {
	var got trace.SpanContext
	handler := TraceHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = trace.SpanContextFromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !got.IsValid() || got.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace context: %v", got)
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(context.Background(), got), carrier)
	if carrier.Get("traceparent") == "" {
		t.Fatal("trace context was not injected")
	}
}

func TestMetricDimensions(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	defer provider.Shutdown(context.Background())
	metrics, err := NewRequestMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	if err := metrics.Record(context.Background(), "discovery", "success"); err != nil {
		t.Fatal(err)
	}
	if err := metrics.Record(context.Background(), "user@example.com", "success"); err == nil {
		t.Fatal("unbounded metric stage accepted")
	}
	if err := metrics.Record(context.Background(), "api", "candidate-id"); err == nil {
		t.Fatal("unbounded metric outcome accepted")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	sum := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])
	if len(sum.DataPoints) != 1 || sum.DataPoints[0].Attributes.Len() != 2 || sum.DataPoints[0].Value != 1 {
		t.Fatalf("unexpected metric data: %+v", sum.DataPoints)
	}
}
