package observability

import (
	"context"
	"testing"
	"time"

	"github.com/ccrabbai/logos/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger()
	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}

	logger.Info("logger test", "component", "test")
}

func TestNewMetrics(t *testing.T) {
	metrics, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics(): %v", err)
	}
	if metrics == nil {
		t.Fatal("NewMetrics() returned nil metrics")
	}
}

func TestInitTracingRejectsInvalidSampler(t *testing.T) {
	ctx := context.Background()
	cfg := testObservabilityConfig()
	cfg.TraceSampler = "invalid_sampler"

	_, err := InitTracing(ctx, cfg)
	if err == nil {
		t.Fatal("expected invalid sampler error")
	}
}

func TestInitTracingRejectsInvalidRatio(t *testing.T) {
	ctx := context.Background()
	cfg := testObservabilityConfig()
	cfg.TraceSampler = "traceidratio"
	cfg.TraceSamplerArg = 2

	_, err := InitTracing(ctx, cfg)
	if err == nil {
		t.Fatal("expected invalid sampling ratio error")
	}
}

func TestInitTracingCreatesProvider(t *testing.T) {
	ctx := context.Background()
	cfg := testObservabilityConfig()

	shutdown, err := InitTracing(ctx, cfg)
	if err != nil {
		t.Fatalf("InitTracing(): %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracing() returned nil shutdown function")
	}

	tracer := otel.Tracer("logos/test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	span.SetAttributes(attribute.String("component", "test"))
	span.SetStatus(codes.Ok, "completed")
	span.End()
	_ = ctx

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	// Adjust this call if the project's InitTracing returns a
	// shutdown function accepting a context.
	_ = shutdownCtx
}

func testObservabilityConfig() config.ObservabilityConfig {
	return config.ObservabilityConfig{
		ServiceName:          "logos-test",
		Environment:           "test",
		OTLPEndpoint:          "localhost:4317",
		OTLPInsecure:          true,
		TraceSampler:          "always_on",
		TraceSamplerArg:       1,
		MetricExportInterval:  10 * time.Second,
	}
}