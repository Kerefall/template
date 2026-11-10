package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceHandlerAddsTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(traceHandler{Handler: slog.NewJSONHandler(&buf, nil)})

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	logger.InfoContext(ctx, "with span")
	logger.Info("without span")

	dec := json.NewDecoder(&buf)

	var withSpan, withoutSpan map[string]any
	if err := dec.Decode(&withSpan); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&withoutSpan); err != nil {
		t.Fatal(err)
	}

	if withSpan["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" || withSpan["span_id"] != "00f067aa0ba902b7" {
		t.Errorf("trace ids are missing or wrong: %v", withSpan)
	}
	if _, ok := withoutSpan["trace_id"]; ok {
		t.Errorf("trace_id must not be set outside of a span: %v", withoutSpan)
	}
}
