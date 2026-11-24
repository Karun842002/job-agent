// Package observability provides bounded operational telemetry.
package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var safeToken = regexp.MustCompile(`^[a-zA-Z0-9_:-]{1,128}$`)

var allowedFields = map[string]bool{
	"service": true, "environment": true, "event_name": true, "trace_id": true,
	"span_id": true, "run_id": true, "workflow_id": true, "step_run_id": true,
	"policy_version": true, "step_key": true, "mode": true, "outcome": true,
	"error_class": true, "reason_code": true, "provider": true, "duration_ms": true,
}

// Logger emits JSON using a bounded allowlist of structured metadata fields.
// Callers must still avoid putting secrets or personal data into allowed fields.
func Logger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey || attr.Key == slog.LevelKey {
				return attr
			}
			if attr.Key != slog.MessageKey && !allowedFields[attr.Key] {
				return slog.Attr{}
			}
			if attr.Value.Kind() == slog.KindString {
				if !safeToken.MatchString(attr.Value.String()) {
					return slog.String(attr.Key, "redacted")
				}
				return attr
			}
			if attr.Key == slog.MessageKey {
				return slog.String(attr.Key, "redacted")
			}
			switch attr.Value.Kind() {
			case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool, slog.KindDuration:
				return attr
			default:
				return slog.String(attr.Key, "redacted")
			}
		},
	}))
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
