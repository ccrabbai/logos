package observability

import (
	"context"
	"fmt"

	"github.com/ccrabbai/logos/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func InitTracing(
	ctx context.Context,
	cfg config.ObservabilityConfig,
) (*sdktrace.TracerProvider, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.OTLPInsecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	resource, err := sdkresource.New(
		ctx,
		sdkresource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironmentName(cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	var sampler sdktrace.Sampler

	switch cfg.TraceSampler {
	case "always_on":
		sampler = sdktrace.AlwaysSample()
	case "always_off":
		sampler = sdktrace.NeverSample()
	case "parentbased_always_on":
		sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		sampler = sdktrace.ParentBased(sdktrace.NeverSample())
	case "traceidratio":
		sampler = sdktrace.TraceIDRatioBased(cfg.TraceSamplerArg)
	case "parentbased_traceidratio":
		sampler = sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.TraceSamplerArg),
		)
	default:
		return nil, fmt.Errorf("unsupported trace sampler: %s", cfg.TraceSampler)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}