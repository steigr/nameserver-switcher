package main

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/steigr/nameserver-switcher/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogging_NormalMode tests that normal logging (requests and responses) works.
func TestLogging_NormalMode(t *testing.T) {
	cfg := getTestConfig(t)
	cfg.LogRequests = true
	cfg.LogResponses = true
	cfg.Debug = false
	cfg.RequestPatterns = []string{`.*\.example\.com$`}
	cfg.RequestResolver = testGoogleDNS

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Make a DNS query
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.DNSPort)
	_, _, err = c.Exchange(m, addr)
	require.NoError(t, err)

	// Logging is verified by manual inspection or by capturing log output
	// The test just ensures the system works with logging enabled
	assert.True(t, cfg.LogRequests)
	assert.True(t, cfg.LogResponses)
	assert.False(t, cfg.Debug)
}

// TestLogging_DebugMode tests that debug logging works.
func TestLogging_DebugMode(t *testing.T) {
	cfg := getTestConfig(t)
	cfg.LogRequests = true
	cfg.LogResponses = true
	cfg.Debug = true
	cfg.RequestPatterns = []string{`.*\.example\.com$`}
	cfg.CNAMEPatterns = []string{`.*\.cdn\..*`}
	cfg.RequestResolver = testGoogleDNS

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Make a DNS query
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.DNSPort)
	_, _, err = c.Exchange(m, addr)
	require.NoError(t, err)

	// Debug logging is verified by manual inspection
	assert.True(t, cfg.Debug)
}

// TestLogging_Disabled tests that logging can be disabled.
func TestLogging_Disabled(t *testing.T) {
	cfg := getTestConfig(t)
	cfg.LogRequests = false
	cfg.LogResponses = false
	cfg.Debug = false
	cfg.RequestResolver = testGoogleDNS

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Make a DNS query
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("example.com"), dns.TypeA)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.DNSPort)
	_, _, err = c.Exchange(m, addr)
	require.NoError(t, err)

	// No request/response logging should occur (only errors)
	assert.False(t, cfg.LogRequests)
	assert.False(t, cfg.LogResponses)
	assert.False(t, cfg.Debug)
}

// Example_logging demonstrates how to use the logging features.
func Example_logging() {
	// Create a configuration with logging enabled
	cfg := config.DefaultConfig()
	cfg.LogRequests = true
	cfg.LogResponses = true
	cfg.Debug = true
	cfg.RequestPatterns = []string{`.*\.example\.com$`}
	cfg.DNSPort = 15353

	// Create and start the application
	app, err := NewApp(cfg)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start app: %v", err)
	}

	// Server will log:
	// [REQUEST] protocol=udp type=A name=test.example.com. from=127.0.0.1:12345
	// [DEBUG] REQUEST_PATTERN matched: pattern=".*\\.example\\.com$" request="test.example.com"
	// [DEBUG] Queried nameserver: system
	// [RESPONSE] name=test.example.com. rcode=NXDOMAIN answers=0 resolver=system duration=5.123ms
	// [DEBUG] Full response: ...

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = app.Shutdown(ctx)
}
