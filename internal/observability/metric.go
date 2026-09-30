package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"github.com/ccrabbai/logos/internal/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

type Metrics struct {
	AppendTotal       metric.Int64Counter
	ReadTotal         metric.Int64Counter
	AppendErrorsTotal metric.Int64Counter
	ReadErrorsTotal   metric.Int64Counter
	AppendDuration    metric.Float64Histogram
	ReadDuration      metric.Float64Histogram
}

func InitMetrics(
	ctx context.Context,
	cfg config.ObservabilityConfig,
) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.OTLPInsecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
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

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				exporter,
				sdkmetric.WithInterval(cfg.MetricExportInterval),
			),
		),
		sdkmetric.WithResource(resource),
	)

	otel.SetMeterProvider(mp)

	return mp, nil
}

func NewMetrics() (*Metrics, error) {
	meter := otel.Meter("logos/internal/log")

	appendTotal, err := meter.Int64Counter(
		"logos.log.append.total",
		metric.WithDescription("Number of records successfully appended to the log"),
	)
	if err != nil {
		return nil, err
	}

	readTotal, err := meter.Int64Counter(
		"logos.log.read.total",
		metric.WithDescription("Number of records successfully read from the log"),
	)
	if err != nil {
		return nil, err
	}

	appendErrorsTotal, err := meter.Int64Counter(
		"logos.log.append.errors",
		metric.WithDescription("Number of failed log append operations"),
	)
	if err != nil {
		return nil, err
	}

	readErrorsTotal, err := meter.Int64Counter(
		"logos.log.read.errors",
		metric.WithDescription("Number of failed log read operations"),
	)
	if err != nil {
		return nil, err
	}

	appendDuration, err := meter.Float64Histogram(
		"logos.log.append.duration",
		metric.WithDescription("Duration of log append operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	readDuration, err := meter.Float64Histogram(
		"logos.log.read.duration",
		metric.WithDescription("Duration of log read operations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		AppendTotal:       appendTotal,
		ReadTotal:         readTotal,
		AppendErrorsTotal: appendErrorsTotal,
		ReadErrorsTotal:   readErrorsTotal,
		AppendDuration:    appendDuration,
		ReadDuration:      readDuration,
	}, nil
}