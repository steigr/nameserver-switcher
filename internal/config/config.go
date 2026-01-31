// Package config handles configuration parsing from CLI flags and environment variables.
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// Config holds all configuration for the nameserver-switcher.
type Config struct {
	// RequestPatterns are regex patterns to match incoming DNS requests.
	RequestPatterns []string

	// CNAMEPatterns are regex patterns to match CNAME responses.
	CNAMEPatterns []string

	// RequestResolver is the DNS server for initial non-recursive lookups (system default).
	RequestResolver string

	// ExplicitResolver is the DNS server for recursive lookups when CNAME matches.
	ExplicitResolver string

	// PassthroughResolver is the DNS server for requests that don't match any request pattern.
	// Falls back to RequestResolver if not set.
	PassthroughResolver string

	// NoCnameResponseResolver is the DNS server for responses without CNAME records.
	// Falls back to RequestResolver if not set.
	NoCnameResponseResolver string

	// NoCnameMatchResolver is the DNS server for CNAME responses that don't match CNAME patterns.
	// Falls back to RequestResolver if not set.
	NoCnameMatchResolver string

	// DNSListenAddr is the address to listen for DNS requests.
	DNSListenAddr string

	// GRPCListenAddr is the address to listen for gRPC requests.
	GRPCListenAddr string

	// HTTPListenAddr is the address for health/metrics endpoints.
	HTTPListenAddr string

	// DNSPort is the DNS server port (for UDP and TCP).
	DNSPort int

	// GRPCPort is the gRPC server port.
	GRPCPort int

	// HTTPPort is the HTTP server port for health and metrics.
	HTTPPort int

	// Debug enables debug logging.
	Debug bool

	// LogRequests enables logging of all DNS requests.
	LogRequests bool

	// LogResponses enables logging of all DNS responses.
	LogResponses bool

	// LogFormat specifies the log output format: "text" or "json".
	LogFormat string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		RequestPatterns:         []string{},
		CNAMEPatterns:           []string{},
		RequestResolver:         "",
		ExplicitResolver:        "",
		PassthroughResolver:     "",
		NoCnameResponseResolver: "",
		NoCnameMatchResolver:    "",
		DNSListenAddr:           "0.0.0.0",
		GRPCListenAddr:          "0.0.0.0",
		HTTPListenAddr:          "0.0.0.0",
		DNSPort:                 5353,
		GRPCPort:                5354,
		HTTPPort:                8080,
		Debug:                   false,
		LogRequests:             true,
		LogResponses:            true,
		LogFormat:               "text",
	}
}

// ParseFlags parses command line flags into the config.
func (c *Config) ParseFlags() {
	var requestPatternsStr string
	var cnamePatternsStr string
	var requestResolver string
	var explicitResolver string
	var passthroughResolver string
	var noCnameResponseResolver string
	var noCnameMatchResolver string

	pflag.StringVar(&requestPatternsStr, "request-patterns", "", "Newline-delimited regex patterns for matching incoming requests")
	pflag.StringVar(&cnamePatternsStr, "cname-patterns", "", "Newline-delimited regex patterns for matching CNAME responses")
	pflag.StringVar(&requestResolver, "request-resolver", "", "DNS server for initial non-recursive lookups (e.g., 8.8.8.8:53)")
	pflag.StringVar(&explicitResolver, "explicit-resolver", "", "DNS server for recursive lookups when CNAME matches (e.g., 1.1.1.1:53)")
	pflag.StringVar(&passthroughResolver, "passthrough-resolver", "", "DNS server for requests not matching any pattern (falls back to request-resolver)")
	pflag.StringVar(&noCnameResponseResolver, "no-cname-response-resolver", "", "DNS server for responses without CNAME (falls back to request-resolver)")
	pflag.StringVar(&noCnameMatchResolver, "no-cname-match-resolver", "", "DNS server for CNAME responses not matching patterns (falls back to request-resolver)")
	pflag.StringVar(&c.DNSListenAddr, "dns-listen-addr", c.DNSListenAddr, "Address to listen for DNS requests")
	pflag.StringVar(&c.GRPCListenAddr, "grpc-listen-addr", c.GRPCListenAddr, "Address to listen for gRPC requests")
	pflag.StringVar(&c.HTTPListenAddr, "http-listen-addr", c.HTTPListenAddr, "Address to listen for HTTP health/metrics requests")
	pflag.IntVar(&c.DNSPort, "dns-port", c.DNSPort, "Port for DNS server")
	pflag.IntVar(&c.GRPCPort, "grpc-port", c.GRPCPort, "Port for gRPC server")
	pflag.IntVar(&c.HTTPPort, "http-port", c.HTTPPort, "Port for HTTP health/metrics server")
	pflag.BoolVar(&c.Debug, "debug", c.Debug, "Enable debug logging")
	pflag.BoolVar(&c.LogRequests, "log-requests", c.LogRequests, "Log all DNS requests")
	pflag.BoolVar(&c.LogResponses, "log-responses", c.LogResponses, "Log all DNS responses")
	pflag.StringVar(&c.LogFormat, "log-format", c.LogFormat, "Log output format: text or json")

	pflag.Parse()

	if requestPatternsStr != "" {
		c.RequestPatterns = splitPatterns(requestPatternsStr)
	}
	if cnamePatternsStr != "" {
		c.CNAMEPatterns = splitPatterns(cnamePatternsStr)
	}
	// Only override resolver settings if CLI flags were provided
	if requestResolver != "" {
		c.RequestResolver = requestResolver
	}
	if explicitResolver != "" {
		c.ExplicitResolver = explicitResolver
	}
	if passthroughResolver != "" {
		c.PassthroughResolver = passthroughResolver
	}
	if noCnameResponseResolver != "" {
		c.NoCnameResponseResolver = noCnameResponseResolver
	}
	if noCnameMatchResolver != "" {
		c.NoCnameMatchResolver = noCnameMatchResolver
	}
}

// LoadFromEnv loads configuration from environment variables.
// Environment variables take precedence over default values but not CLI flags.
func (c *Config) LoadFromEnv() {
	isTrue := "true"

	if patterns := os.Getenv("REQUEST_PATTERNS"); patterns != "" {
		c.RequestPatterns = splitPatterns(patterns)
	}
	if patterns := os.Getenv("CNAME_PATTERNS"); patterns != "" {
		c.CNAMEPatterns = splitPatterns(patterns)
	}
	if resolver := os.Getenv("REQUEST_RESOLVER"); resolver != "" {
		c.RequestResolver = resolver
	}
	if resolver := os.Getenv("EXPLICIT_RESOLVER"); resolver != "" {
		c.ExplicitResolver = resolver
	}
	if resolver := os.Getenv("PASSTHROUGH_RESOLVER"); resolver != "" {
		c.PassthroughResolver = resolver
	}
	if resolver := os.Getenv("NO_CNAME_RESPONSE_RESOLVER"); resolver != "" {
		c.NoCnameResponseResolver = resolver
	}
	if resolver := os.Getenv("NO_CNAME_MATCH_RESOLVER"); resolver != "" {
		c.NoCnameMatchResolver = resolver
	}
	if addr := os.Getenv("DNS_LISTEN_ADDR"); addr != "" {
		c.DNSListenAddr = addr
	}
	if addr := os.Getenv("GRPC_LISTEN_ADDR"); addr != "" {
		c.GRPCListenAddr = addr
	}
	if addr := os.Getenv("HTTP_LISTEN_ADDR"); addr != "" {
		c.HTTPListenAddr = addr
	}
	if port := os.Getenv("DNS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.DNSPort = p
		}
	}
	if port := os.Getenv("GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.GRPCPort = p
		}
	}
	if port := os.Getenv("HTTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.HTTPPort = p
		}
	}
	if debug := os.Getenv("DEBUG"); debug != "" {
		c.Debug = debug == isTrue || debug == "1"
	}
	if logReq := os.Getenv("LOG_REQUESTS"); logReq != "" {
		c.LogRequests = logReq == isTrue || logReq == "1"
	}
	if logResp := os.Getenv("LOG_RESPONSES"); logResp != "" {
		c.LogResponses = logResp == isTrue || logResp == "1"
	}
	if logFormat := os.Getenv("LOG_FORMAT"); logFormat != "" {
		c.LogFormat = logFormat
	}
}

// splitPatterns splits a newline-delimited string into a slice of patterns.
func splitPatterns(s string) []string {
	lines := strings.Split(s, "\n")
	patterns := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			patterns = append(patterns, line)
		}
	}
	return patterns
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	// No strict validation required - empty patterns means nothing matches
	return nil
}
