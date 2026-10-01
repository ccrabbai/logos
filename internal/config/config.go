package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Store struct {
		BufferBytes uint64
	}

	Segment struct {
		MaxStoreBytes     uint64
		MaxIndexBytes     uint64
		InitialBaseOffset uint64
	}
}

type ObservabilityConfig struct {
	ServiceName          string
	Environment          string
	OTLPEndpoint         string
	OTLPInsecure         bool
	TraceSampler         string
	TraceSamplerArg      float64
	MetricExportInterval time.Duration
}

func LoadObservabilityConfig() (ObservabilityConfig, error) {
	cfg := ObservabilityConfig{
		ServiceName:          "logos",
		Environment:          "development",
		OTLPEndpoint:         "localhost:4317",
		OTLPInsecure:         true,
		TraceSampler:         "always_on",
		TraceSamplerArg:      1.0,
		MetricExportInterval: 10 * time.Second,
	}

	if v := os.Getenv("LOGOS_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if v := os.Getenv("LOGOS_ENVIRONMENT"); v != "" {
		cfg.Environment = v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.OTLPEndpoint = v
	}
	if v := os.Getenv("OTEL_TRACES_SAMPLER"); v != "" {
		cfg.TraceSampler = v
	}
	if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid OTEL_TRACES_SAMPLER_ARG: %w", err)
		}
		if n < 0 || n > 1 {
			return cfg, fmt.Errorf(
				"OTEL_TRACES_SAMPLER_ARG must be between 0 and 1",
			)
		}
		cfg.TraceSamplerArg = n
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid OTEL_EXPORTER_OTLP_INSECURE: %w", err)
		}
		cfg.OTLPInsecure = b
	}
	if v := os.Getenv("LOGOS_METRIC_EXPORT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid LOGOS_METRIC_EXPORT_INTERVAL (use a duration such as 10s): %w", err)
		}
		cfg.MetricExportInterval = d
	}

	switch cfg.TraceSampler {
	case "always_on", "always_off",
		"parentbased_always_on", "parentbased_always_off":
	case "traceidratio", "parentbased_traceidratio":
		if cfg.TraceSamplerArg < 0 || cfg.TraceSamplerArg > 1 {
			return cfg, fmt.Errorf(
				"OTEL_TRACES_SAMPLER_ARG must be between 0 and 1",
			)
		}
	default:
		return cfg, fmt.Errorf(
			"unsupported OTEL_TRACES_SAMPLER %q",
			cfg.TraceSampler,
		)
	}

	if cfg.ServiceName == "" || cfg.Environment == "" || cfg.OTLPEndpoint == "" {
		return cfg, fmt.Errorf("service name, environment, and OTLP endpoint must not be empty")
	}
	if cfg.MetricExportInterval <= 0 {
		return cfg, fmt.Errorf("LOGOS_METRIC_EXPORT_INTERVAL must be positive")
	}

	return cfg, nil
}
