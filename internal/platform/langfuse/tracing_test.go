package langfuse

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestProviderRejectsRemoteEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"https://example.com/v1/traces", "http://192.168.1.1:4318/v1/traces",
		"http://localhost:4318/v1/traces", "http://127.0.0.1:4318/other",
		"http://user@127.0.0.1:4318/v1/traces", "http://127.0.0.1:4318/v1/traces?x=y",
	} {
		if provider, err := NewProvider(context.Background(), endpoint); err == nil {
			provider.Shutdown(context.Background())
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
}

func TestGenerationOTLP(t *testing.T) {
	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx := context.Background()
	provider, err := NewProvider(ctx, server.URL+"/v1/traces")
	if err != nil {
		t.Fatal(err)
	}
	tracer := provider.Tracer("test")
	if err := RecordGeneration(ctx, tracer, "fixture-model", 12, 7); err != nil {
		t.Fatal(err)
	}
	if err := RecordGeneration(ctx, tracer, "private prompt with spaces", 0, 0); err == nil {
		t.Fatal("unsafe model metadata accepted")
	}
	shutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := provider.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-bodies:
		var request collectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		spans := request.ResourceSpans[0].ScopeSpans[0].Spans
		if len(spans) != 1 || spans[0].Name != "gen_ai.generation" {
			t.Fatalf("spans: %v", spans)
		}
		values := map[string]int64{}
		for _, attr := range spans[0].Attributes {
			values[attr.Key] = attr.Value.GetIntValue()
			if strings.Contains(attr.Key, "prompt") || strings.Contains(attr.Key, "output.messages") {
				t.Fatalf("sensitive attribute: %s", attr.Key)
			}
		}
		if values["gen_ai.usage.input_tokens"] != 12 || values["gen_ai.usage.output_tokens"] != 7 {
			t.Fatalf("token attributes: %v", values)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no OTLP export")
	}
}
