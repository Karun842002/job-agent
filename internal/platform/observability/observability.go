// Package observability provides bounded operational telemetry.
package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Logger returns a JSON logger. Callers must avoid untrusted or sensitive content.
func Logger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}

// TraceHandler extracts W3C trace context and starts a server span.
func TraceHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("job-agent/http").Start(ctx, "http.request", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestMetrics contains a counter with fixed, low-cardinality dimensions.
type RequestMetrics struct {
	counter metric.Int64Counter
}

// NewRequestMetrics creates the operational request counter.
func NewRequestMetrics(meter metric.Meter) (RequestMetrics, error) {
	counter, err := meter.Int64Counter("job_agent_requests_total")
	if err != nil {
		return RequestMetrics{}, err
	}
	return RequestMetrics{counter: counter}, nil
}

// Record increments a request counter using bounded stage and outcome values.
func (m RequestMetrics) Record(ctx context.Context, stage, outcome string) error {
	switch stage {
	case "api", "worker", "discovery", "poll", "monitor":
	default:
		return errors.New("invalid metric stage")
	}
	switch outcome {
	case "success", "failure", "denied":
	default:
		return errors.New("invalid metric outcome")
	}
	m.counter.Add(ctx, 1, metric.WithAttributes(attribute.String("stage", stage), attribute.String("outcome", outcome)))
	return nil
}
