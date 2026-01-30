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

// SeparateResolversSuite tests nameserver-switcher with separate resolvers for each fallback case.
// This validates that the new configuration options work correctly:
// - PASSTHROUGH_RESOLVER: for requests that don't match any pattern
// - NO_CNAME_RESPONSE_RESOLVER: for responses without CNAME
// - NO_CNAME_MATCH_RESOLVER: for CNAME responses that don't match the pattern
type SeparateResolversSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *SeparateResolversSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = SetupWithSeparateResolvers(s.ctx, DNSMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *SeparateResolversSuite) TearDownSuite() {
	// Print summary
	s.printSummary()

	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *SeparateResolversSuite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - SeparateResolversSuite")
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

// TestPassthroughResolver tests that unmatched requests use the passthrough resolver.
// Query: unmatched.example.org (doesn't match .*\.example\.com$ pattern)
// Expected: 127.0.0.10 (from passthrough resolver)
func (s *SeparateResolversSuite) TestPassthroughResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "unmatched.example.org")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else {
			actual = fmt.Sprintf("Rcode=%d, no A records", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Log("--- unmatched.example.org (Passthrough Resolver Test) ---")
	s.T().Logf("Query:    dig @%s unmatched.example.org A", hostPort)
	s.T().Logf("Expected: A=127.0.0.10 (from passthrough resolver)")
	s.T().Logf("Actual:   %s", actual)

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.10")
	s.results = append(s.results, testResult{
		name:     "Passthrough resolver for unmatched request",
		query:    "unmatched.example.org",
		server:   hostPort,
		expected: "A=127.0.0.10",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.10",
		"Expected A=127.0.0.10 from passthrough resolver, but got %s", actual)
}

// TestNoCnameResponseResolver tests that responses without CNAME use the no-cname-response resolver.
// Query: direct.example.com (matches request pattern, but explicit resolver returns A record, not CNAME)
// Expected: 127.0.0.20 (from no-cname-response resolver)
func (s *SeparateResolversSuite) TestNoCnameResponseResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "direct.example.com")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else {
			actual = fmt.Sprintf("Rcode=%d, no A records", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Log("--- direct.example.com (No CNAME Response Resolver Test) ---")
	s.T().Logf("Query:    dig @%s direct.example.com A", hostPort)
	s.T().Logf("Expected: A=127.0.0.20 (from no-cname-response resolver)")
	s.T().Logf("Note: If got 127.0.0.5, explicit resolver was incorrectly used")
	s.T().Logf("Actual:   %s", actual)

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.20")
	s.results = append(s.results, testResult{
		name:     "No-CNAME-response resolver for direct A record",
		query:    "direct.example.com",
		server:   hostPort,
		expected: "A=127.0.0.20",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.20",
		"Expected A=127.0.0.20 from no-cname-response resolver, but got %s. "+
			"If got 127.0.0.5, the explicit resolver was incorrectly used when there's no CNAME.", actual)
}

// TestNoCnameMatchResolver tests that CNAME responses not matching the pattern use the no-cname-match resolver.
// Query: hello.example.com (matches request pattern, CNAME=bar-nomatch.example.com doesn't match CNAME pattern)
// Expected: 127.0.0.30 (from no-cname-match resolver)
func (s *SeparateResolversSuite) TestNoCnameMatchResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "hello.example.com")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else if cname := result.GetFirstCNAME(); cname != "" {
			actual = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
		} else {
			actual = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Log("--- hello.example.com (No CNAME Match Resolver Test) ---")
	s.T().Logf("Query:    dig @%s hello.example.com A", hostPort)
	s.T().Logf("Expected: A=127.0.0.30 (from no-cname-match resolver)")
	s.T().Logf("Actual:   %s", actual)

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.30")
	s.results = append(s.results, testResult{
		name:     "No-CNAME-match resolver for CNAME not matching pattern",
		query:    "hello.example.com",
		server:   hostPort,
		expected: "A=127.0.0.30",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.30",
		"Expected A=127.0.0.30 from no-cname-match resolver, but got %s", actual)
}

// TestExplicitResolver tests that matched CNAME patterns use the explicit resolver.
// Query: foo.example.com (matches request pattern, CNAME=bar-match.example.com matches CNAME pattern)
// Expected: 127.0.0.2 (from explicit resolver)
func (s *SeparateResolversSuite) TestExplicitResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

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
	s.T().Log("--- foo.example.com (Explicit Resolver Test) ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: CNAME=bar-match.example.com (from explicit resolver)")
	s.T().Logf("Actual:   %s", actual)

	passed := result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, testResult{
		name:     "Explicit resolver for matching CNAME pattern",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "CNAME=bar-match.example.com",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Expected CNAME to bar-match.example.com from explicit resolver, got: %s", result.GetFirstCNAME())
}

func TestSeparateResolversSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(SeparateResolversSuite))
}

// GRPCSeparateResolversSuite tests nameserver-switcher with separate resolvers using gRPC mode.
type GRPCSeparateResolversSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *GRPCSeparateResolversSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = SetupWithSeparateResolvers(s.ctx, GRPCMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *GRPCSeparateResolversSuite) TearDownSuite() {
	// Print summary
	s.printSummary()

	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *GRPCSeparateResolversSuite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - GRPCSeparateResolversSuite")
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

// TestPassthroughResolver tests that unmatched requests use the passthrough resolver via gRPC.
func (s *GRPCSeparateResolversSuite) TestPassthroughResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "unmatched.example.org")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else {
			actual = fmt.Sprintf("Rcode=%d, no A records", result.Response.Rcode)
		}
	}

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.10")
	s.results = append(s.results, testResult{
		name:     "gRPC: Passthrough resolver for unmatched request",
		query:    "unmatched.example.org",
		server:   hostPort,
		expected: "A=127.0.0.10",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.10",
		"Expected A=127.0.0.10 from passthrough resolver, but got %s", actual)
}

// TestNoCnameResponseResolver tests that responses without CNAME use the no-cname-response resolver via gRPC.
func (s *GRPCSeparateResolversSuite) TestNoCnameResponseResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "direct.example.com")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else {
			actual = fmt.Sprintf("Rcode=%d, no A records", result.Response.Rcode)
		}
	}

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.20")
	s.results = append(s.results, testResult{
		name:     "gRPC: No-CNAME-response resolver for direct A record",
		query:    "direct.example.com",
		server:   hostPort,
		expected: "A=127.0.0.20",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.20",
		"Expected A=127.0.0.20 from no-cname-response resolver, but got %s", actual)
}

// TestNoCnameMatchResolver tests that CNAME responses not matching the pattern use the no-cname-match resolver via gRPC.
func (s *GRPCSeparateResolversSuite) TestNoCnameMatchResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "hello.example.com")

	actual := ""
	if result.Error != nil {
		actual = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		ips := result.GetARecords()
		if len(ips) > 0 {
			actual = fmt.Sprintf("A=%s", ips[0])
		} else {
			actual = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
		}
	}

	passed := result.Error == nil && result.Response != nil && strings.Contains(actual, "127.0.0.30")
	s.results = append(s.results, testResult{
		name:     "gRPC: No-CNAME-match resolver for CNAME not matching pattern",
		query:    "hello.example.com",
		server:   hostPort,
		expected: "A=127.0.0.30",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.Contains(s.T(), actual, "127.0.0.30",
		"Expected A=127.0.0.30 from no-cname-match resolver, but got %s", actual)
}

// TestExplicitResolver tests that matched CNAME patterns use the explicit resolver via gRPC.
func (s *GRPCSeparateResolversSuite) TestExplicitResolver() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

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

	passed := result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, testResult{
		name:     "gRPC: Explicit resolver for matching CNAME pattern",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "CNAME=bar-match.example.com",
		actual:   actual,
		passed:   passed,
	})

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Expected CNAME to bar-match.example.com from explicit resolver, got: %s", result.GetFirstCNAME())
}

func TestGRPCSeparateResolversSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(GRPCSeparateResolversSuite))
}
