package config

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupTLSConfigRejectsInvalidCertificatePaths(t *testing.T) {
	tests := []struct {
		name string
		cfg  TLSConfig
	}{
		{
			name: "missing certificate",
			cfg: TLSConfig{
				CertFile: filepath.Join(t.TempDir(), "missing.crt"),
				KeyFile:  filepath.Join(t.TempDir(), "missing.key"),
			},
		},
		{
			name: "missing key",
			cfg: TLSConfig{
				CertFile: filepath.Join(t.TempDir(), "missing.crt"),
				KeyFile:  filepath.Join(t.TempDir(), "missing.key"),
			},
		},
		{
			name: "missing CA",
			cfg: TLSConfig{
				CAFile: filepath.Join(t.TempDir(), "missing-ca.crt"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SetupTLSConfig(tt.cfg)
			if err == nil {
				t.Fatal("expected an error for invalid certificate paths")
			}
		})
	}
}

func TestSetupTLSConfigRejectsInvalidCertificatePair(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")

	if err := os.WriteFile(cert, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("not a private key"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := SetupTLSConfig(TLSConfig{
		CertFile: cert,
		KeyFile:  key,
	})
	if err == nil {
		t.Fatal("expected invalid certificate/key error")
	}
}

func TestTLSConfigCertificatePairMustBeComplete(t *testing.T) {
	// A partial certificate/key configuration must not silently produce
	// a TLS configuration that appears to be mutually authenticated.
	_, err := SetupTLSConfig(TLSConfig{
		CertFile: filepath.Join(t.TempDir(), "sever.crt"),
	})
	if err == nil {
		t.Fatal("expected an error for a partial certificate/key pair")
	}
}

func TestTLSMinimumVersion(t *testing.T) {
	// This assertion applies if SetupTLSConfig sets a minimum TLS version.
	cfg, err := SetupTLSConfig(TLSConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil && cfg.MinVersion != 0 &&
		cfg.MinVersion < tls.VersionTLS12 {
		t.Fatalf("minimum TLS version = %x; want TLS 1.2 or later",
			cfg.MinVersion)
	}
}