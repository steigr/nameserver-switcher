//go:build integration

// Package integration provides integration tests for nameserver-switcher.
package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DNSModeSuite tests nameserver-switcher with CoreDNS using the forward (DNS) plugin.
type DNSModeSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *DNSModeSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = Setup(s.ctx, DNSMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *DNSModeSuite) TearDownSuite() {
	// Print summary
	s.printSummary()

	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *DNSModeSuite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - DNSModeSuite")
	s.T().Log("========================================")
	passed := 0
	failed := 0
	for _, r := range s.results {
		status := "✓ PASS"
		if !r.passed {
			status = "✗ FAIL"
			failed++
		} else {
			passed++
		}
		s.T().Logf("%s: %s", status, r.name)
		s.T().Logf("  Query:    dig @%s %s A", r.server, r.query)
		s.T().Logf("  Expected: %s", r.expected)
		s.T().Logf("  Actual:   %s", r.actual)
	}
	s.T().Log("----------------------------------------")
	s.T().Logf("Total: %d passed, %d failed", passed, failed)
	s.T().Log("========================================")
}

func (s *DNSModeSuite) logQuery(name, server, query, expected string, result *QueryResult) testResult {
	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		if cname := result.GetFirstCNAME(); cname != "" {
			actual = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
		} else if a := result.GetFirstARecord(); a != "" {
			actual = fmt.Sprintf("A=%s", a)
		} else {
			actual = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Logf("--- %s ---", name)
	s.T().Logf("Query:    dig @%s %s A", server, query)
	s.T().Logf("Expected: %s", expected)
	s.T().Logf("Actual:   %s", actual)

	return testResult{
		name:     name,
		query:    query,
		server:   server,
		expected: expected,
		actual:   actual,
	}
}

// TestFooExampleComViaNameserverSwitcherDirect tests querying foo.example.com directly to nameserver-switcher.
// Expected: CNAME to bar-match.example.com (from explicit resolver).
func (s *DNSModeSuite) TestFooExampleComViaNameserverSwitcherDirect() {
	hostPort, err := s.infra.GetNameserverSwitchDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	tr := s.logQuery(
		"foo.example.com via nameserver-switcher direct",
		hostPort,
		"foo.example.com",
		"CNAME=bar-match.example.com (from explicit resolver)",
		result,
	)

	tr.passed = result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, tr)

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Expected CNAME to bar-match.example.com, got: %s", result.GetFirstCNAME())
}

// TestFooExampleComViaCoreDNS tests querying foo.example.com via CoreDNS (forward plugin).
// Expected: CNAME to bar-match.example.com (from explicit resolver via nameserver-switcher).
func (s *DNSModeSuite) TestFooExampleComViaCoreDNS() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	tr := s.logQuery(
		"foo.example.com via CoreDNS forward",
		hostPort,
		"foo.example.com",
		"CNAME=bar-match.example.com (from explicit resolver via DNS forward)",
		result,
	)

	tr.passed = result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, tr)

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Expected CNAME to bar-match.example.com, got: %s", result.GetFirstCNAME())
}

// TestHelloExampleComViaNameserverSwitcherDirect tests querying hello.example.com directly to nameserver-switcher.
// Expected: Query processed by system resolver (CNAME doesn't match pattern).
func (s *DNSModeSuite) TestHelloExampleComViaNameserverSwitcherDirect() {
	hostPort, err := s.infra.GetNameserverSwitchDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "hello.example.com")

	tr := s.logQuery(
		"hello.example.com via nameserver-switcher direct",
		hostPort,
		"hello.example.com",
		"CNAME=bar-nomatch.example.com or NXDOMAIN (system resolver)",
		result,
	)

	tr.passed = result.Error == nil && result.Response != nil
	s.results = append(s.results, tr)

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
}

// TestHelloExampleComViaCoreDNS tests querying hello.example.com via CoreDNS (forward plugin).
// Expected: Query routed through system resolver.
func (s *DNSModeSuite) TestHelloExampleComViaCoreDNS() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "hello.example.com")

	tr := s.logQuery(
		"hello.example.com via CoreDNS forward",
		hostPort,
		"hello.example.com",
		"CNAME=bar-nomatch.example.com or NXDOMAIN (system resolver via DNS forward)",
		result,
	)

	tr.passed = result.Error == nil && result.Response != nil
	s.results = append(s.results, tr)

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
}

// TestDNSMasqSystemReturnsDifferentIPForBarMatch verifies the system resolver returns a different IP.
// This proves the routing is working correctly.
func (s *DNSModeSuite) TestDNSMasqSystemReturnsDifferentIPForBarMatch() {
	server := s.infra.GetDNSMasqSystemInternalAddr()
	result, err := s.infra.RunDNSQuery(s.ctx, server, "bar-match.example.com")

	actual := result
	if err != nil {
		actual = fmt.Sprintf("Error: %v", err)
	}

	s.T().Log("")
	s.T().Log("--- bar-match.example.com via dnsmasq-system ---")
	s.T().Logf("Query:    dig @%s bar-match.example.com A", server)
	s.T().Logf("Expected: A=127.0.0.99 (different from explicit resolver)")
	s.T().Logf("Actual:   A=%s", strings.TrimSpace(actual))

	passed := err == nil && strings.Contains(result, "127.0.0.99")
	s.results = append(s.results, testResult{
		name:     "bar-match.example.com via dnsmasq-system",
		query:    "bar-match.example.com",
		server:   server,
		expected: "A=127.0.0.99",
		actual:   fmt.Sprintf("A=%s", strings.TrimSpace(actual)),
		passed:   passed,
	})

	require.NoError(s.T(), err, "DNS query failed")
	assert.Contains(s.T(), result, "127.0.0.99",
		"Expected A record 127.0.0.99, got: %s", result)
}

// TestDNSMasqExplicitReturnsCorrectIPForBarMatch verifies the explicit resolver returns correct IP.
func (s *DNSModeSuite) TestDNSMasqExplicitReturnsCorrectIPForBarMatch() {
	server := s.infra.GetDNSMasqExplicitInternalAddr()
	result, err := s.infra.RunDNSQuery(s.ctx, server, "bar-match.example.com")

	actual := result
	if err != nil {
		actual = fmt.Sprintf("Error: %v", err)
	}

	s.T().Log("")
	s.T().Log("--- bar-match.example.com via dnsmasq-explicit ---")
	s.T().Logf("Query:    dig @%s bar-match.example.com A", server)
	s.T().Logf("Expected: A=127.0.0.2")
	s.T().Logf("Actual:   A=%s", strings.TrimSpace(actual))

	passed := err == nil && strings.Contains(result, "127.0.0.2")
	s.results = append(s.results, testResult{
		name:     "bar-match.example.com via dnsmasq-explicit",
		query:    "bar-match.example.com",
		server:   server,
		expected: "A=127.0.0.2",
		actual:   fmt.Sprintf("A=%s", strings.TrimSpace(actual)),
		passed:   passed,
	})

	require.NoError(s.T(), err, "DNS query failed")
	assert.Contains(s.T(), result, "127.0.0.2",
		"Expected A record 127.0.0.2, got: %s", result)
}

func TestDNSModeSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(DNSModeSuite))
}
