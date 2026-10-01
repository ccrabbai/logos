
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadObservabilityConfigDefaults(t *testing.T) {
	clearObservabilityEnv(t)

	cfg, err := LoadObservabilityConfig()
	if err != nil {
		t.Fatalf("LoadObservabilityConfig(): %v", err)
	}

	if cfg.ServiceName != "logos" {
		t.Errorf("ServiceName = %q; want logos", cfg.ServiceName)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %q; want development", cfg.Environment)
	}
	if cfg.OTLPEndpoint != "localhost:4317" {
		t.Errorf("OTLPEndpoint = %q; want localhost:4317",
			cfg.OTLPEndpoint)
	}
	if cfg.TraceSampler != "always_on" {
		t.Errorf("TraceSampler = %q; want always_on", cfg.TraceSampler)
	}
	if cfg.TraceSamplerArg != 1.0 {
		t.Errorf("TraceSamplerArg = %v; want 1", cfg.TraceSamplerArg)
	}
	if cfg.MetricExportInterval != 10*time.Second {
		t.Errorf("MetricExportInterval = %v; want 10s",
			cfg.MetricExportInterval)
	}
}

func TestLoadObservabilityConfigEnvironmentOverrides(t *testing.T) {
	clearObservabilityEnv(t)

	env := map[string]string{
		"LOGOS_SERVICE_NAME":                 "logos-test",
		"LOGOS_ENVIRONMENT":                  "test",
		"OTEL_EXPORTER_OTLP_ENDPOINT":         "localhost:14317",
		"OTEL_TRACES_SAMPLER":                "always_off",
		"OTEL_TRACES_SAMPLER_ARG":            "0.5",
		"OTEL_EXPORTER_OTLP_INSECURE":        "false",
		"LOGOS_METRIC_EXPORT_INTERVAL":       "5s",
	}

	for key, value := range env {
		t.Setenv(key, value)
	}

	cfg, err := LoadObservabilityConfig()
	if err != nil {
		t.Fatalf("LoadObservabilityConfig(): %v", err)
	}

	if cfg.ServiceName != "logos-test" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.Environment != "test" {
		t.Errorf("Environment = %q", cfg.Environment)
	}
	if cfg.OTLPEndpoint != "localhost:14317" {
		t.Errorf("OTLPEndpoint = %q", cfg.OTLPEndpoint)
	}
	if cfg.TraceSampler != "always_off" {
		t.Errorf("TraceSampler = %q", cfg.TraceSampler)
	}
	if cfg.TraceSamplerArg != 0.5 {
		t.Errorf("TraceSamplerArg = %v", cfg.TraceSamplerArg)
	}
	if cfg.OTLPInsecure {
		t.Error("OTLPInsecure = true; want false")
	}
	if cfg.MetricExportInterval != 5*time.Second {
		t.Errorf("MetricExportInterval = %v; want 5s",
			cfg.MetricExportInterval)
	}
}

func TestLoadObservabilityConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "invalid sampler",
			key:   "OTEL_TRACES_SAMPLER",
			value: "invalid_sampler",
		},
		{
			name:  "invalid sampler ratio",
			key:   "OTEL_TRACES_SAMPLER_ARG",
			value: "2",
		},
		{
			name:  "negative sampler ratio",
			key:   "OTEL_TRACES_SAMPLER_ARG",
			value: "-0.1",
		},
		{
			name:  "invalid metric interval",
			key:   "LOGOS_METRIC_EXPORT_INTERVAL",
			value: "not-a-duration",
		},
		{
			name:  "zero metric interval",
			key:   "LOGOS_METRIC_EXPORT_INTERVAL",
			value: "0s",
		},
		{
			name:  "invalid insecure flag",
			key:   "OTEL_EXPORTER_OTLP_INSECURE",
			value: "not-a-bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearObservabilityEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := LoadObservabilityConfig()
			if err == nil {
				t.Fatalf("%s=%q: expected an error",
					tt.key, tt.value)
			}
		})
	}
}

func clearObservabilityEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"LOGOS_SERVICE_NAME",
		"LOGOS_ENVIRONMENT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_TRACES_SAMPLER",
		"OTEL_TRACES_SAMPLER_ARG",
		"OTEL_EXPORTER_OTLP_INSECURE",
		"LOGOS_METRIC_EXPORT_INTERVAL",
	} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}