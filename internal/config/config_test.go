package config

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Empty(t, cfg.RequestPatterns)
	assert.Empty(t, cfg.CNAMEPatterns)
	assert.Empty(t, cfg.RequestResolver)
	assert.Empty(t, cfg.ExplicitResolver)
	assert.Equal(t, "0.0.0.0", cfg.DNSListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.GRPCListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.HTTPListenAddr)
	assert.Equal(t, 5353, cfg.DNSPort)
	assert.Equal(t, 5354, cfg.GRPCPort)
	assert.Equal(t, 8080, cfg.HTTPPort)
}

func TestSplitPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single pattern",
			input:    "example.com",
			expected: []string{"example.com"},
		},
		{
			name:     "multiple patterns",
			input:    "example.com\ntest.org",
			expected: []string{"example.com", "test.org"},
		},
		{
			name:     "patterns with empty lines",
			input:    "example.com\n\ntest.org\n",
			expected: []string{"example.com", "test.org"},
		},
		{
			name:     "patterns with whitespace",
			input:    "  example.com  \n  test.org  ",
			expected: []string{"example.com", "test.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPatterns(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	origRequestPatterns := os.Getenv("REQUEST_PATTERNS")
	origCNAMEPatterns := os.Getenv("CNAME_PATTERNS")
	origRequestResolver := os.Getenv("REQUEST_RESOLVER")
	origExplicitResolver := os.Getenv("EXPLICIT_RESOLVER")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")
	origGRPCListenAddr := os.Getenv("GRPC_LISTEN_ADDR")
	origHTTPListenAddr := os.Getenv("HTTP_LISTEN_ADDR")

	defer func() {
		os.Setenv("REQUEST_PATTERNS", origRequestPatterns)
		os.Setenv("CNAME_PATTERNS", origCNAMEPatterns)
		os.Setenv("REQUEST_RESOLVER", origRequestResolver)
		os.Setenv("EXPLICIT_RESOLVER", origExplicitResolver)
		os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
		os.Setenv("GRPC_LISTEN_ADDR", origGRPCListenAddr)
		os.Setenv("HTTP_LISTEN_ADDR", origHTTPListenAddr)
	}()

	os.Setenv("REQUEST_PATTERNS", "example.com\ntest.org")
	os.Setenv("CNAME_PATTERNS", "cdn.com")
	os.Setenv("REQUEST_RESOLVER", "8.8.8.8:53")
	os.Setenv("EXPLICIT_RESOLVER", "1.1.1.1:53")
	os.Setenv("DNS_LISTEN_ADDR", "127.0.0.1")
	os.Setenv("GRPC_LISTEN_ADDR", "127.0.0.2")
	os.Setenv("HTTP_LISTEN_ADDR", "127.0.0.3")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, []string{"example.com", "test.org"}, cfg.RequestPatterns)
	assert.Equal(t, []string{"cdn.com"}, cfg.CNAMEPatterns)
	assert.Equal(t, "8.8.8.8:53", cfg.RequestResolver)
	assert.Equal(t, "1.1.1.1:53", cfg.ExplicitResolver)
	assert.Equal(t, "127.0.0.1", cfg.DNSListenAddr)
	assert.Equal(t, "127.0.0.2", cfg.GRPCListenAddr)
	assert.Equal(t, "127.0.0.3", cfg.HTTPListenAddr)
}

func TestLoadFromEnv_EmptyValues(t *testing.T) {
	origRequestPatterns := os.Getenv("REQUEST_PATTERNS")
	origCNAMEPatterns := os.Getenv("CNAME_PATTERNS")
	origRequestResolver := os.Getenv("REQUEST_RESOLVER")
	origExplicitResolver := os.Getenv("EXPLICIT_RESOLVER")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")
	origGRPCListenAddr := os.Getenv("GRPC_LISTEN_ADDR")
	origHTTPListenAddr := os.Getenv("HTTP_LISTEN_ADDR")

	defer func() {
		os.Setenv("REQUEST_PATTERNS", origRequestPatterns)
		os.Setenv("CNAME_PATTERNS", origCNAMEPatterns)
		os.Setenv("REQUEST_RESOLVER", origRequestResolver)
		os.Setenv("EXPLICIT_RESOLVER", origExplicitResolver)
		os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
		os.Setenv("GRPC_LISTEN_ADDR", origGRPCListenAddr)
		os.Setenv("HTTP_LISTEN_ADDR", origHTTPListenAddr)
	}()

	os.Unsetenv("REQUEST_PATTERNS")
	os.Unsetenv("CNAME_PATTERNS")
	os.Unsetenv("REQUEST_RESOLVER")
	os.Unsetenv("EXPLICIT_RESOLVER")
	os.Unsetenv("DNS_LISTEN_ADDR")
	os.Unsetenv("GRPC_LISTEN_ADDR")
	os.Unsetenv("HTTP_LISTEN_ADDR")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	assert.Empty(t, cfg.RequestPatterns)
	assert.Empty(t, cfg.CNAMEPatterns)
	assert.Empty(t, cfg.RequestResolver)
	assert.Empty(t, cfg.ExplicitResolver)
	assert.Equal(t, "0.0.0.0", cfg.DNSListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.GRPCListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.HTTPListenAddr)
}

func TestParseFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)

	os.Args = []string{
		"test",
		"--request-patterns=example.com\ntest.org",
		"--cname-patterns=cdn.com",
		"--request-resolver=8.8.8.8:53",
		"--explicit-resolver=1.1.1.1:53",
		"--dns-listen-addr=192.168.1.1",
		"--grpc-listen-addr=192.168.1.2",
		"--http-listen-addr=192.168.1.3",
		"--dns-port=1053",
		"--grpc-port=1054",
		"--http-port=9090",
	}

	cfg := DefaultConfig()
	cfg.ParseFlags()

	assert.Equal(t, []string{"example.com", "test.org"}, cfg.RequestPatterns)
	assert.Equal(t, []string{"cdn.com"}, cfg.CNAMEPatterns)
	assert.Equal(t, "8.8.8.8:53", cfg.RequestResolver)
	assert.Equal(t, "1.1.1.1:53", cfg.ExplicitResolver)
	assert.Equal(t, "192.168.1.1", cfg.DNSListenAddr)
	assert.Equal(t, "192.168.1.2", cfg.GRPCListenAddr)
	assert.Equal(t, "192.168.1.3", cfg.HTTPListenAddr)
	assert.Equal(t, 1053, cfg.DNSPort)
	assert.Equal(t, 1054, cfg.GRPCPort)
	assert.Equal(t, 9090, cfg.HTTPPort)
}

func TestParseFlags_EmptyPatterns(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)

	os.Args = []string{
		"test",
		"--dns-port=5353",
	}

	cfg := DefaultConfig()
	cfg.ParseFlags()

	assert.Empty(t, cfg.RequestPatterns)
	assert.Empty(t, cfg.CNAMEPatterns)
	assert.Equal(t, 5353, cfg.DNSPort)
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	require.NoError(t, err)
}
