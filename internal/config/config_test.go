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
	assert.Empty(t, cfg.PassthroughResolver)
	assert.Empty(t, cfg.NoCnameResponseResolver)
	assert.Empty(t, cfg.NoCnameMatchResolver)
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
	origPassthroughResolver := os.Getenv("PASSTHROUGH_RESOLVER")
	origNoCnameResponseResolver := os.Getenv("NO_CNAME_RESPONSE_RESOLVER")
	origNoCnameMatchResolver := os.Getenv("NO_CNAME_MATCH_RESOLVER")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")
	origGRPCListenAddr := os.Getenv("GRPC_LISTEN_ADDR")
	origHTTPListenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")
	origDebug := os.Getenv("DEBUG")
	origLogRequests := os.Getenv("LOG_REQUESTS")
	origLogResponses := os.Getenv("LOG_RESPONSES")
	origLogFormat := os.Getenv("LOG_FORMAT")

	defer func() {
		_ = os.Setenv("REQUEST_PATTERNS", origRequestPatterns)
		_ = os.Setenv("CNAME_PATTERNS", origCNAMEPatterns)
		_ = os.Setenv("REQUEST_RESOLVER", origRequestResolver)
		_ = os.Setenv("EXPLICIT_RESOLVER", origExplicitResolver)
		_ = os.Setenv("PASSTHROUGH_RESOLVER", origPassthroughResolver)
		_ = os.Setenv("NO_CNAME_RESPONSE_RESOLVER", origNoCnameResponseResolver)
		_ = os.Setenv("NO_CNAME_MATCH_RESOLVER", origNoCnameMatchResolver)
		_ = os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
		_ = os.Setenv("GRPC_LISTEN_ADDR", origGRPCListenAddr)
		_ = os.Setenv("HTTP_LISTEN_ADDR", origHTTPListenAddr)
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
		_ = os.Setenv("DEBUG", origDebug)
		_ = os.Setenv("LOG_REQUESTS", origLogRequests)
		_ = os.Setenv("LOG_RESPONSES", origLogResponses)
		_ = os.Setenv("LOG_FORMAT", origLogFormat)
	}()

	_ = os.Setenv("REQUEST_PATTERNS", "example.com\ntest.org")
	_ = os.Setenv("CNAME_PATTERNS", "cdn.com")
	_ = os.Setenv("REQUEST_RESOLVER", "8.8.8.8:53")
	_ = os.Setenv("EXPLICIT_RESOLVER", "1.1.1.1:53")
	_ = os.Setenv("PASSTHROUGH_RESOLVER", "9.9.9.9:53")
	_ = os.Setenv("NO_CNAME_RESPONSE_RESOLVER", "208.67.222.222:53")
	_ = os.Setenv("NO_CNAME_MATCH_RESOLVER", "208.67.220.220:53")
	_ = os.Setenv("DNS_LISTEN_ADDR", "127.0.0.1")
	_ = os.Setenv("GRPC_LISTEN_ADDR", "127.0.0.2")
	_ = os.Setenv("HTTP_LISTEN_ADDR", "127.0.0.3")
	_ = os.Setenv("DNS_PORT", "1053")
	_ = os.Setenv("GRPC_PORT", "1054")
	_ = os.Setenv("HTTP_PORT", "9090")
	_ = os.Setenv("DEBUG", "true")
	_ = os.Setenv("LOG_REQUESTS", "false")
	_ = os.Setenv("LOG_RESPONSES", "false")
	_ = os.Setenv("LOG_FORMAT", "json")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, []string{"example.com", "test.org"}, cfg.RequestPatterns)
	assert.Equal(t, []string{"cdn.com"}, cfg.CNAMEPatterns)
	assert.Equal(t, "8.8.8.8:53", cfg.RequestResolver)
	assert.Equal(t, "1.1.1.1:53", cfg.ExplicitResolver)
	assert.Equal(t, "9.9.9.9:53", cfg.PassthroughResolver)
	assert.Equal(t, "208.67.222.222:53", cfg.NoCnameResponseResolver)
	assert.Equal(t, "208.67.220.220:53", cfg.NoCnameMatchResolver)
	assert.Equal(t, "127.0.0.1", cfg.DNSListenAddr)
	assert.Equal(t, "127.0.0.2", cfg.GRPCListenAddr)
	assert.Equal(t, "127.0.0.3", cfg.HTTPListenAddr)
	assert.Equal(t, 1053, cfg.DNSPort)
	assert.Equal(t, 1054, cfg.GRPCPort)
	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.True(t, cfg.Debug)
	assert.False(t, cfg.LogRequests)
	assert.False(t, cfg.LogResponses)
	assert.Equal(t, "json", cfg.LogFormat)
}

func TestLoadFromEnv_EmptyValues(t *testing.T) {
	origRequestPatterns := os.Getenv("REQUEST_PATTERNS")
	origCNAMEPatterns := os.Getenv("CNAME_PATTERNS")
	origRequestResolver := os.Getenv("REQUEST_RESOLVER")
	origExplicitResolver := os.Getenv("EXPLICIT_RESOLVER")
	origPassthroughResolver := os.Getenv("PASSTHROUGH_RESOLVER")
	origNoCnameResponseResolver := os.Getenv("NO_CNAME_RESPONSE_RESOLVER")
	origNoCnameMatchResolver := os.Getenv("NO_CNAME_MATCH_RESOLVER")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")
	origGRPCListenAddr := os.Getenv("GRPC_LISTEN_ADDR")
	origHTTPListenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")

	defer func() {
		_ = os.Setenv("REQUEST_PATTERNS", origRequestPatterns)
		_ = os.Setenv("CNAME_PATTERNS", origCNAMEPatterns)
		_ = os.Setenv("REQUEST_RESOLVER", origRequestResolver)
		_ = os.Setenv("EXPLICIT_RESOLVER", origExplicitResolver)
		_ = os.Setenv("PASSTHROUGH_RESOLVER", origPassthroughResolver)
		_ = os.Setenv("NO_CNAME_RESPONSE_RESOLVER", origNoCnameResponseResolver)
		_ = os.Setenv("NO_CNAME_MATCH_RESOLVER", origNoCnameMatchResolver)
		_ = os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
		_ = os.Setenv("GRPC_LISTEN_ADDR", origGRPCListenAddr)
		_ = os.Setenv("HTTP_LISTEN_ADDR", origHTTPListenAddr)
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
	}()

	_ = os.Unsetenv("REQUEST_PATTERNS")
	_ = os.Unsetenv("CNAME_PATTERNS")
	_ = os.Unsetenv("REQUEST_RESOLVER")
	_ = os.Unsetenv("EXPLICIT_RESOLVER")
	_ = os.Unsetenv("PASSTHROUGH_RESOLVER")
	_ = os.Unsetenv("NO_CNAME_RESPONSE_RESOLVER")
	_ = os.Unsetenv("NO_CNAME_MATCH_RESOLVER")
	_ = os.Unsetenv("DNS_LISTEN_ADDR")
	_ = os.Unsetenv("GRPC_LISTEN_ADDR")
	_ = os.Unsetenv("HTTP_LISTEN_ADDR")
	_ = os.Unsetenv("DNS_PORT")
	_ = os.Unsetenv("GRPC_PORT")
	_ = os.Unsetenv("HTTP_PORT")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	assert.Empty(t, cfg.RequestPatterns)
	assert.Empty(t, cfg.CNAMEPatterns)
	assert.Empty(t, cfg.RequestResolver)
	assert.Empty(t, cfg.ExplicitResolver)
	assert.Empty(t, cfg.PassthroughResolver)
	assert.Empty(t, cfg.NoCnameResponseResolver)
	assert.Empty(t, cfg.NoCnameMatchResolver)
	assert.Equal(t, "0.0.0.0", cfg.DNSListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.GRPCListenAddr)
	assert.Equal(t, "0.0.0.0", cfg.HTTPListenAddr)
	// Ports should remain at default values when env vars are not set
	assert.Equal(t, 5353, cfg.DNSPort)
	assert.Equal(t, 5354, cfg.GRPCPort)
	assert.Equal(t, 8080, cfg.HTTPPort)
}

func TestLoadFromEnv_InvalidPortValues(t *testing.T) {
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")

	defer func() {
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
	}()

	// Set invalid port values
	_ = os.Setenv("DNS_PORT", "invalid")
	_ = os.Setenv("GRPC_PORT", "not-a-number")
	_ = os.Setenv("HTTP_PORT", "")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	// Ports should remain at default values when env vars have invalid values
	assert.Equal(t, 5353, cfg.DNSPort)
	assert.Equal(t, 5354, cfg.GRPCPort)
	assert.Equal(t, 8080, cfg.HTTPPort)
}

func TestLoadFromEnv_PortsOverrideDefaults(t *testing.T) {
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")

	defer func() {
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
	}()

	_ = os.Setenv("DNS_PORT", "5555")
	_ = os.Setenv("GRPC_PORT", "6666")
	_ = os.Setenv("HTTP_PORT", "7777")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, 5555, cfg.DNSPort)
	assert.Equal(t, 6666, cfg.GRPCPort)
	assert.Equal(t, 7777, cfg.HTTPPort)
}

func TestLoadFromEnv_BooleanValues(t *testing.T) {
	tests := []struct {
		name            string
		debugEnv        string
		logRequestsEnv  string
		logResponsesEnv string
		expectDebug     bool
		expectLogReq    bool
		expectLogResp   bool
	}{
		{
			name:            "all true with 'true'",
			debugEnv:        "true",
			logRequestsEnv:  "true",
			logResponsesEnv: "true",
			expectDebug:     true,
			expectLogReq:    true,
			expectLogResp:   true,
		},
		{
			name:            "all true with '1'",
			debugEnv:        "1",
			logRequestsEnv:  "1",
			logResponsesEnv: "1",
			expectDebug:     true,
			expectLogReq:    true,
			expectLogResp:   true,
		},
		{
			name:            "all false with 'false'",
			debugEnv:        "false",
			logRequestsEnv:  "false",
			logResponsesEnv: "false",
			expectDebug:     false,
			expectLogReq:    false,
			expectLogResp:   false,
		},
		{
			name:            "all false with '0'",
			debugEnv:        "0",
			logRequestsEnv:  "0",
			logResponsesEnv: "0",
			expectDebug:     false,
			expectLogReq:    false,
			expectLogResp:   false,
		},
		{
			name:            "mixed values",
			debugEnv:        "true",
			logRequestsEnv:  "false",
			logResponsesEnv: "1",
			expectDebug:     true,
			expectLogReq:    false,
			expectLogResp:   true,
		},
		{
			name:            "invalid values default to false",
			debugEnv:        "invalid",
			logRequestsEnv:  "yes",
			logResponsesEnv: "no",
			expectDebug:     false,
			expectLogReq:    false,
			expectLogResp:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origDebug := os.Getenv("DEBUG")
			origLogRequests := os.Getenv("LOG_REQUESTS")
			origLogResponses := os.Getenv("LOG_RESPONSES")

			defer func() {
				_ = os.Setenv("DEBUG", origDebug)
				_ = os.Setenv("LOG_REQUESTS", origLogRequests)
				_ = os.Setenv("LOG_RESPONSES", origLogResponses)
			}()

			_ = os.Setenv("DEBUG", tt.debugEnv)
			_ = os.Setenv("LOG_REQUESTS", tt.logRequestsEnv)
			_ = os.Setenv("LOG_RESPONSES", tt.logResponsesEnv)

			cfg := DefaultConfig()
			cfg.LoadFromEnv()

			assert.Equal(t, tt.expectDebug, cfg.Debug, "DEBUG mismatch")
			assert.Equal(t, tt.expectLogReq, cfg.LogRequests, "LOG_REQUESTS mismatch")
			assert.Equal(t, tt.expectLogResp, cfg.LogResponses, "LOG_RESPONSES mismatch")
		})
	}
}

func TestLoadFromEnv_BooleanUnset(t *testing.T) {
	origDebug := os.Getenv("DEBUG")
	origLogRequests := os.Getenv("LOG_REQUESTS")
	origLogResponses := os.Getenv("LOG_RESPONSES")

	defer func() {
		_ = os.Setenv("DEBUG", origDebug)
		_ = os.Setenv("LOG_REQUESTS", origLogRequests)
		_ = os.Setenv("LOG_RESPONSES", origLogResponses)
	}()

	// Unset boolean env vars
	_ = os.Unsetenv("DEBUG")
	_ = os.Unsetenv("LOG_REQUESTS")
	_ = os.Unsetenv("LOG_RESPONSES")

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	// Defaults should be preserved
	assert.False(t, cfg.Debug, "DEBUG should remain default (false)")
	assert.True(t, cfg.LogRequests, "LOG_REQUESTS should remain default (true)")
	assert.True(t, cfg.LogResponses, "LOG_RESPONSES should remain default (true)")
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
		"--passthrough-resolver=9.9.9.9:53",
		"--no-cname-response-resolver=208.67.222.222:53",
		"--no-cname-match-resolver=208.67.220.220:53",
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
	assert.Equal(t, "9.9.9.9:53", cfg.PassthroughResolver)
	assert.Equal(t, "208.67.222.222:53", cfg.NoCnameResponseResolver)
	assert.Equal(t, "208.67.220.220:53", cfg.NoCnameMatchResolver)
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

// TestConfigPriority_FlagOverridesEnvAndDefault tests that CLI flags take precedence
// over environment variables and defaults.
// Priority order: flag (highest) > environment variable > default (lowest)
func TestConfigPriority_FlagOverridesEnvAndDefault(t *testing.T) {
	// Save original state
	origArgs := os.Args
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")
	origRequestResolver := os.Getenv("REQUEST_RESOLVER")

	defer func() {
		os.Args = origArgs
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
		_ = os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
		_ = os.Setenv("REQUEST_RESOLVER", origRequestResolver)
	}()

	// Set environment variables
	_ = os.Setenv("DNS_PORT", "2222")
	_ = os.Setenv("GRPC_PORT", "3333")
	_ = os.Setenv("HTTP_PORT", "4444")
	_ = os.Setenv("DNS_LISTEN_ADDR", "10.0.0.1")
	_ = os.Setenv("REQUEST_RESOLVER", "env-resolver:53")

	// Set CLI flags that should override env vars
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	os.Args = []string{
		"test",
		"--dns-port=9999",
		"--grpc-port=8888",
		"--http-port=7777",
		"--dns-listen-addr=192.168.1.100",
		"--request-resolver=flag-resolver:53",
	}

	cfg := DefaultConfig()
	cfg.LoadFromEnv() // First load env vars
	cfg.ParseFlags()  // Then parse flags (should override)

	// Flags should win over env vars
	assert.Equal(t, 9999, cfg.DNSPort, "DNS_PORT flag should override env var")
	assert.Equal(t, 8888, cfg.GRPCPort, "GRPC_PORT flag should override env var")
	assert.Equal(t, 7777, cfg.HTTPPort, "HTTP_PORT flag should override env var")
	assert.Equal(t, "192.168.1.100", cfg.DNSListenAddr, "DNS_LISTEN_ADDR flag should override env var")
	assert.Equal(t, "flag-resolver:53", cfg.RequestResolver, "REQUEST_RESOLVER flag should override env var")
}

// TestConfigPriority_EnvOverridesDefault tests that environment variables take precedence
// over default values.
func TestConfigPriority_EnvOverridesDefault(t *testing.T) {
	// Save original state
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")

	defer func() {
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
		_ = os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
	}()

	// Set environment variables
	_ = os.Setenv("DNS_PORT", "1111")
	_ = os.Setenv("GRPC_PORT", "2222")
	_ = os.Setenv("HTTP_PORT", "3333")
	_ = os.Setenv("DNS_LISTEN_ADDR", "172.16.0.1")

	cfg := DefaultConfig()

	// Verify defaults before LoadFromEnv
	assert.Equal(t, 5353, cfg.DNSPort, "Default DNS_PORT should be 5353")
	assert.Equal(t, 5354, cfg.GRPCPort, "Default GRPC_PORT should be 5354")
	assert.Equal(t, 8080, cfg.HTTPPort, "Default HTTP_PORT should be 8080")
	assert.Equal(t, "0.0.0.0", cfg.DNSListenAddr, "Default DNS_LISTEN_ADDR should be 0.0.0.0")

	cfg.LoadFromEnv()

	// Env vars should override defaults
	assert.Equal(t, 1111, cfg.DNSPort, "DNS_PORT env var should override default")
	assert.Equal(t, 2222, cfg.GRPCPort, "GRPC_PORT env var should override default")
	assert.Equal(t, 3333, cfg.HTTPPort, "HTTP_PORT env var should override default")
	assert.Equal(t, "172.16.0.1", cfg.DNSListenAddr, "DNS_LISTEN_ADDR env var should override default")
}

// TestConfigPriority_DefaultUsedWhenNoOverride tests that defaults are used when neither
// flags nor environment variables are set.
func TestConfigPriority_DefaultUsedWhenNoOverride(t *testing.T) {
	// Save original state
	origArgs := os.Args
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")
	origDNSListenAddr := os.Getenv("DNS_LISTEN_ADDR")

	defer func() {
		os.Args = origArgs
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
		_ = os.Setenv("DNS_LISTEN_ADDR", origDNSListenAddr)
	}()

	// Unset all environment variables
	_ = os.Unsetenv("DNS_PORT")
	_ = os.Unsetenv("GRPC_PORT")
	_ = os.Unsetenv("HTTP_PORT")
	_ = os.Unsetenv("DNS_LISTEN_ADDR")

	// No flags set
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	os.Args = []string{"test"}

	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	cfg.ParseFlags()

	// Defaults should be used
	assert.Equal(t, 5353, cfg.DNSPort, "DNS_PORT should use default")
	assert.Equal(t, 5354, cfg.GRPCPort, "GRPC_PORT should use default")
	assert.Equal(t, 8080, cfg.HTTPPort, "HTTP_PORT should use default")
	assert.Equal(t, "0.0.0.0", cfg.DNSListenAddr, "DNS_LISTEN_ADDR should use default")
}

// TestConfigPriority_PartialOverride tests mixed configuration where some values come from
// flags, some from env vars, and some from defaults.
func TestConfigPriority_PartialOverride(t *testing.T) {
	// Save original state
	origArgs := os.Args
	origDNSPort := os.Getenv("DNS_PORT")
	origGRPCPort := os.Getenv("GRPC_PORT")
	origHTTPPort := os.Getenv("HTTP_PORT")

	defer func() {
		os.Args = origArgs
		_ = os.Setenv("DNS_PORT", origDNSPort)
		_ = os.Setenv("GRPC_PORT", origGRPCPort)
		_ = os.Setenv("HTTP_PORT", origHTTPPort)
	}()

	// DNS_PORT: from flag (highest priority)
	// GRPC_PORT: from env var (middle priority)
	// HTTP_PORT: from default (lowest priority)

	_ = os.Unsetenv("DNS_PORT") // Not set in env
	_ = os.Setenv("GRPC_PORT", "6666")
	_ = os.Unsetenv("HTTP_PORT") // Not set in env

	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	os.Args = []string{
		"test",
		"--dns-port=9999", // Only DNS_PORT from flag
	}

	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	cfg.ParseFlags()

	assert.Equal(t, 9999, cfg.DNSPort, "DNS_PORT should come from flag")
	assert.Equal(t, 6666, cfg.GRPCPort, "GRPC_PORT should come from env var")
	assert.Equal(t, 8080, cfg.HTTPPort, "HTTP_PORT should come from default")
}

// TestConfigPriority_AllResolvers tests the priority for all resolver configurations.
func TestConfigPriority_AllResolvers(t *testing.T) {
	// Save original state
	origArgs := os.Args
	origRequestResolver := os.Getenv("REQUEST_RESOLVER")
	origExplicitResolver := os.Getenv("EXPLICIT_RESOLVER")
	origPassthroughResolver := os.Getenv("PASSTHROUGH_RESOLVER")
	origNoCnameResponseResolver := os.Getenv("NO_CNAME_RESPONSE_RESOLVER")
	origNoCnameMatchResolver := os.Getenv("NO_CNAME_MATCH_RESOLVER")

	defer func() {
		os.Args = origArgs
		_ = os.Setenv("REQUEST_RESOLVER", origRequestResolver)
		_ = os.Setenv("EXPLICIT_RESOLVER", origExplicitResolver)
		_ = os.Setenv("PASSTHROUGH_RESOLVER", origPassthroughResolver)
		_ = os.Setenv("NO_CNAME_RESPONSE_RESOLVER", origNoCnameResponseResolver)
		_ = os.Setenv("NO_CNAME_MATCH_RESOLVER", origNoCnameMatchResolver)
	}()

	// Set all resolvers via env vars
	_ = os.Setenv("REQUEST_RESOLVER", "env-request:53")
	_ = os.Setenv("EXPLICIT_RESOLVER", "env-explicit:53")
	_ = os.Setenv("PASSTHROUGH_RESOLVER", "env-passthrough:53")
	_ = os.Setenv("NO_CNAME_RESPONSE_RESOLVER", "env-nocname-response:53")
	_ = os.Setenv("NO_CNAME_MATCH_RESOLVER", "env-nocname-match:53")

	// Override only some via flags
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	os.Args = []string{
		"test",
		"--request-resolver=flag-request:53",
		"--explicit-resolver=flag-explicit:53",
	}

	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	cfg.ParseFlags()

	// Flags should override env vars
	assert.Equal(t, "flag-request:53", cfg.RequestResolver, "REQUEST_RESOLVER should come from flag")
	assert.Equal(t, "flag-explicit:53", cfg.ExplicitResolver, "EXPLICIT_RESOLVER should come from flag")
	// Env vars should be used when no flag is provided
	assert.Equal(t, "env-passthrough:53", cfg.PassthroughResolver, "PASSTHROUGH_RESOLVER should come from env var")
	assert.Equal(t, "env-nocname-response:53", cfg.NoCnameResponseResolver, "NO_CNAME_RESPONSE_RESOLVER should come from env var")
	assert.Equal(t, "env-nocname-match:53", cfg.NoCnameMatchResolver, "NO_CNAME_MATCH_RESOLVER should come from env var")
}

// TestConfigPriority_Patterns tests the priority for pattern configurations.
func TestConfigPriority_Patterns(t *testing.T) {
	// Save original state
	origArgs := os.Args
	origRequestPatterns := os.Getenv("REQUEST_PATTERNS")
	origCNAMEPatterns := os.Getenv("CNAME_PATTERNS")

	defer func() {
		os.Args = origArgs
		_ = os.Setenv("REQUEST_PATTERNS", origRequestPatterns)
		_ = os.Setenv("CNAME_PATTERNS", origCNAMEPatterns)
	}()

	// Set patterns via env vars
	_ = os.Setenv("REQUEST_PATTERNS", "env-pattern1\nenv-pattern2")
	_ = os.Setenv("CNAME_PATTERNS", "env-cname1\nenv-cname2")

	// Override request patterns via flag, leave CNAME patterns from env
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	os.Args = []string{
		"test",
		"--request-patterns=flag-pattern1\nflag-pattern2",
	}

	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	cfg.ParseFlags()

	// Request patterns should come from flag
	assert.Equal(t, []string{"flag-pattern1", "flag-pattern2"}, cfg.RequestPatterns, "REQUEST_PATTERNS should come from flag")
	// CNAME patterns should come from env var
	assert.Equal(t, []string{"env-cname1", "env-cname2"}, cfg.CNAMEPatterns, "CNAME_PATTERNS should come from env var")
}
