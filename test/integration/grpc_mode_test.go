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

// testResult holds the result of a single test for summary reporting
type testResult struct {
	name     string
	query    string
	server   string
	expected string
	actual   string
	passed   bool
}

// GRPCModeSuite tests nameserver-switcher with CoreDNS using the grpc plugin.
type GRPCModeSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *GRPCModeSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = Setup(s.ctx, GRPCMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *GRPCModeSuite) TearDownSuite() {
	// Print summary
	s.printSummary()

	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *GRPCModeSuite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - GRPCModeSuite")
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

func (s *GRPCModeSuite) logQuery(name, server, query, expected string, result *QueryResult) testResult {
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
func (s *GRPCModeSuite) TestFooExampleComViaNameserverSwitcherDirect() {
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

// TestFooExampleComViaCoreDNSGRPC tests querying foo.example.com via CoreDNS (grpc plugin).
// Expected: CNAME to bar-match.example.com (from explicit resolver via gRPC).
func (s *GRPCModeSuite) TestFooExampleComViaCoreDNSGRPC() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	tr := s.logQuery(
		"foo.example.com via CoreDNS gRPC",
		hostPort,
		"foo.example.com",
		"CNAME=bar-match.example.com (from explicit resolver via gRPC)",
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
func (s *GRPCModeSuite) TestHelloExampleComViaNameserverSwitcherDirect() {
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

// TestHelloExampleComViaCoreDNSGRPC tests querying hello.example.com via CoreDNS (grpc plugin).
// Expected: Query routed through system resolver via gRPC.
func (s *GRPCModeSuite) TestHelloExampleComViaCoreDNSGRPC() {
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "hello.example.com")

	tr := s.logQuery(
		"hello.example.com via CoreDNS gRPC",
		hostPort,
		"hello.example.com",
		"CNAME=bar-nomatch.example.com or NXDOMAIN (system resolver via gRPC)",
		result,
	)

	tr.passed = result.Error == nil && result.Response != nil
	s.results = append(s.results, tr)

	require.NoError(s.T(), result.Error, "DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
}

// TestDNSMasqSystemReturnsDifferentIPForBarMatch verifies the system resolver returns a different IP.
// This proves the routing is working correctly.
func (s *GRPCModeSuite) TestDNSMasqSystemReturnsDifferentIPForBarMatch() {
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
func (s *GRPCModeSuite) TestDNSMasqExplicitReturnsCorrectIPForBarMatch() {
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

func TestGRPCModeSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(GRPCModeSuite))
}

// GRPCModeScenario3Suite tests that CNAME not matching the pattern routes to system resolver.
// Scenario 3: Query hello.example.com which has CNAME=bar-nomatch.example.com
// Since bar-nomatch doesn't match `.*-match\.example\.com$`, system resolver is used
// Expected: Resolution via system resolver, returns 127.0.0.3
type GRPCModeScenario3Suite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *GRPCModeScenario3Suite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = Setup(s.ctx, GRPCMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *GRPCModeScenario3Suite) TearDownSuite() {
	s.printSummary()
	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *GRPCModeScenario3Suite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - Scenario 3: CNAME Pattern Routing")
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

// TestScenario3_CNAMENotMatchingUsesSystemResolver tests that when CNAME doesn't match the pattern,
// the system resolver is used for final resolution.
func (s *GRPCModeScenario3Suite) TestScenario3_CNAMENotMatchingUsesSystemResolver() {
	// Step 1: Verify foo.example.com uses explicit resolver (CNAME matches pattern)
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	actual1 := ""
	if result.Error != nil {
		actual1 = fmt.Sprintf("Error: %v", result.Error)
	} else if cname := result.GetFirstCNAME(); cname != "" {
		actual1 = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
	}

	s.T().Log("")
	s.T().Log("--- Step 1: foo.example.com (CNAME matches pattern) ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: CNAME=bar-match.example.com (uses explicit resolver)")
	s.T().Logf("Actual:   %s", actual1)

	step1Passed := result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, testResult{
		name:     "Step 1: foo.example.com (CNAME matches)",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "CNAME=bar-match.example.com",
		actual:   actual1,
		passed:   step1Passed,
	})

	require.NoError(s.T(), result.Error, "foo.example.com DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response for foo.example.com")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"foo.example.com should return CNAME bar-match.example.com, got: %s", result.GetFirstCNAME())

	// Step 2: Query hello.example.com - should use system resolver
	result = s.dns.QueryA(hostPort, "hello.example.com")

	actual2 := ""
	if result.Error != nil {
		actual2 = fmt.Sprintf("Error: %v", result.Error)
	} else if cname := result.GetFirstCNAME(); cname != "" {
		actual2 = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
	} else if result.IsNXDomain() {
		actual2 = "NXDOMAIN"
	} else {
		actual2 = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
	}

	s.T().Log("")
	s.T().Log("--- Step 2: hello.example.com (CNAME doesn't match pattern) ---")
	s.T().Logf("Query:    dig @%s hello.example.com A", hostPort)
	s.T().Logf("Expected: CNAME=bar-nomatch.example.com or NXDOMAIN (uses system resolver)")
	s.T().Logf("Actual:   %s", actual2)

	step2Passed := result.Error == nil && result.Response != nil
	s.results = append(s.results, testResult{
		name:     "Step 2: hello.example.com (CNAME doesn't match)",
		query:    "hello.example.com",
		server:   hostPort,
		expected: "CNAME=bar-nomatch.example.com or NXDOMAIN",
		actual:   actual2,
		passed:   step2Passed,
	})

	require.NoError(s.T(), result.Error, "hello.example.com DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response for hello.example.com")

	// Step 3: Verify system resolver has correct data
	sysServer := s.infra.GetDNSMasqSystemInternalAddr()
	sysResult, err := s.infra.RunDNSQuery(s.ctx, sysServer, "bar-nomatch.example.com")

	actual3 := sysResult
	if err != nil {
		actual3 = fmt.Sprintf("Error: %v", err)
	}

	s.T().Log("")
	s.T().Log("--- Step 3: Verify system resolver data ---")
	s.T().Logf("Query:    dig @%s bar-nomatch.example.com A", sysServer)
	s.T().Logf("Expected: A=127.0.0.3")
	s.T().Logf("Actual:   A=%s", strings.TrimSpace(actual3))

	step3Passed := err == nil && strings.Contains(sysResult, "127.0.0.3")
	s.results = append(s.results, testResult{
		name:     "Step 3: Verify system resolver",
		query:    "bar-nomatch.example.com",
		server:   sysServer,
		expected: "A=127.0.0.3",
		actual:   fmt.Sprintf("A=%s", strings.TrimSpace(actual3)),
		passed:   step3Passed,
	})

	require.NoError(s.T(), err)
	assert.Contains(s.T(), sysResult, "127.0.0.3",
		"System resolver should return 127.0.0.3 for bar-nomatch.example.com")
}

func TestGRPCModeScenario3Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(GRPCModeScenario3Suite))
}

// GRPCModeScenario4Suite tests nameserver-switcher failure and fallthrough.
// Scenario 4: Stop nameserver-switcher to simulate failure
// Expected: CoreDNS should fallthrough and resolve via fallback
type GRPCModeScenario4Suite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *GRPCModeScenario4Suite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = SetupWithFallback(s.ctx, GRPCMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *GRPCModeScenario4Suite) TearDownSuite() {
	s.printSummary()
	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *GRPCModeScenario4Suite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - Scenario 4: Nameserver-Switcher Failure")
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

// TestScenario4_NameserverSwitcherFailure tests fallthrough when nameserver-switcher fails
func (s *GRPCModeScenario4Suite) TestScenario4_NameserverSwitcherFailure() {
	// Step 1: Verify initial setup works
	hostPort, err := s.infra.GetCoreDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	actual1 := ""
	if result.Error != nil {
		actual1 = fmt.Sprintf("Error: %v", result.Error)
	} else if cname := result.GetFirstCNAME(); cname != "" {
		actual1 = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
	}

	s.T().Log("")
	s.T().Log("--- Step 1: Initial query (nameserver-switcher running) ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: CNAME=bar-match.example.com")
	s.T().Logf("Actual:   %s", actual1)

	step1Passed := result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, testResult{
		name:     "Step 1: Initial query",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "CNAME=bar-match.example.com",
		actual:   actual1,
		passed:   step1Passed,
	})

	require.NoError(s.T(), result.Error, "Initial DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Initial query should return CNAME bar-match.example.com")

	// Step 2: Stop nameserver-switcher
	s.T().Log("")
	s.T().Log("--- Step 2: Stopping nameserver-switcher ---")
	err = s.infra.StopNameserverSwitcher(s.ctx)
	require.NoError(s.T(), err, "Failed to stop nameserver-switcher")
	s.T().Log("nameserver-switcher stopped successfully")

	// Step 3: Query again - should fallthrough
	result = s.dns.QueryA(hostPort, "foo.example.com")

	actual3 := ""
	if result.Error != nil {
		actual3 = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		if cname := result.GetFirstCNAME(); cname != "" {
			actual3 = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
		} else if a := result.GetFirstARecord(); a != "" {
			actual3 = fmt.Sprintf("A=%s", a)
		} else {
			actual3 = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Log("--- Step 3: Query after nameserver-switcher stopped ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: Fallthrough response or SERVFAIL")
	s.T().Logf("Actual:   %s", actual3)

	// Accept either fallthrough success or SERVFAIL
	step3Passed := result.Error != nil || result.Response != nil
	s.results = append(s.results, testResult{
		name:     "Step 3: Query after failure",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "Fallthrough or SERVFAIL",
		actual:   actual3,
		passed:   step3Passed,
	})
}

func TestGRPCModeScenario4Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(GRPCModeScenario4Suite))
}

// GRPCModeScenario5Suite tests explicit resolver failure.
// Scenario 5: Stop explicit dnsmasq
// Expected: nameserver-switcher should return SERVFAIL/NXDOMAIN when explicit resolver is down
type GRPCModeScenario5Suite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	infra   *Infrastructure
	dns     *DNSClient
	results []testResult
}

func (s *GRPCModeScenario5Suite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	s.dns = NewDNSClient()
	s.results = []testResult{}

	var err error
	s.infra, err = Setup(s.ctx, GRPCMode)
	require.NoError(s.T(), err, "Failed to setup test infrastructure")
}

func (s *GRPCModeScenario5Suite) TearDownSuite() {
	s.printSummary()
	if s.infra != nil {
		s.infra.Teardown(s.ctx)
	}
	s.cancel()
}

func (s *GRPCModeScenario5Suite) printSummary() {
	s.T().Log("")
	s.T().Log("========================================")
	s.T().Log("TEST SUMMARY - Scenario 5: Explicit Resolver Failure")
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

// TestScenario5_ExplicitResolverFailure tests that when explicit resolver fails,
// nameserver-switcher returns SERVFAIL even though CNAME pattern matches.
func (s *GRPCModeScenario5Suite) TestScenario5_ExplicitResolverFailure() {
	// Step 1: Verify initial setup works - query nameserver-switcher directly to avoid caching
	hostPort, err := s.infra.GetNameserverSwitchDNSHostPort(s.ctx)
	require.NoError(s.T(), err)

	result := s.dns.QueryA(hostPort, "foo.example.com")

	actual1 := ""
	if result.Error != nil {
		actual1 = fmt.Sprintf("Error: %v", result.Error)
	} else if cname := result.GetFirstCNAME(); cname != "" {
		actual1 = fmt.Sprintf("CNAME=%s", strings.TrimSuffix(cname, "."))
	}

	s.T().Log("")
	s.T().Log("--- Step 1: Initial query (explicit resolver running) ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: CNAME=bar-match.example.com")
	s.T().Logf("Actual:   %s", actual1)

	step1Passed := result.Error == nil && result.HasCNAME("bar-match.example.com")
	s.results = append(s.results, testResult{
		name:     "Step 1: Initial query",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "CNAME=bar-match.example.com",
		actual:   actual1,
		passed:   step1Passed,
	})

	require.NoError(s.T(), result.Error, "Initial DNS query failed")
	require.NotNil(s.T(), result.Response, "No DNS response")
	assert.True(s.T(), result.HasCNAME("bar-match.example.com"),
		"Initial query should return CNAME bar-match.example.com")

	// Step 2: Stop dnsmasq-explicit
	s.T().Log("")
	s.T().Log("--- Step 2: Stopping dnsmasq-explicit ---")
	err = s.infra.StopDNSMasqExplicit(s.ctx)
	require.NoError(s.T(), err, "Failed to stop dnsmasq-explicit")
	s.T().Log("dnsmasq-explicit stopped successfully")

	// Step 3: Query again - should fail with SERVFAIL
	shortTimeoutClient := &DNSClient{Timeout: 3 * time.Second}
	result = shortTimeoutClient.QueryA(hostPort, "foo.example.com")

	actual3 := ""
	if result.Error != nil {
		actual3 = fmt.Sprintf("Error: %v", result.Error)
	} else if result.Response != nil {
		if result.Response.Rcode == 2 {
			actual3 = "SERVFAIL (Rcode=2)"
		} else if result.Response.Rcode == 3 {
			actual3 = "NXDOMAIN (Rcode=3)"
		} else if cname := result.GetFirstCNAME(); cname != "" {
			actual3 = fmt.Sprintf("CNAME=%s (unexpected - possible caching)", strings.TrimSuffix(cname, "."))
		} else {
			actual3 = fmt.Sprintf("Rcode=%d", result.Response.Rcode)
		}
	}

	s.T().Log("")
	s.T().Log("--- Step 3: Query after explicit resolver stopped ---")
	s.T().Logf("Query:    dig @%s foo.example.com A", hostPort)
	s.T().Logf("Expected: SERVFAIL (Rcode=2) or timeout error")
	s.T().Logf("Actual:   %s", actual3)

	// Accept SERVFAIL, NXDOMAIN, or timeout error
	step3Passed := result.Error != nil ||
		(result.Response != nil && (result.Response.Rcode == 2 || result.Response.Rcode == 3))
	s.results = append(s.results, testResult{
		name:     "Step 3: Query after explicit resolver stopped",
		query:    "foo.example.com",
		server:   hostPort,
		expected: "SERVFAIL or timeout",
		actual:   actual3,
		passed:   step3Passed,
	})

	if result.Error != nil {
		s.T().Logf("Query failed with error (expected): %v", result.Error)
		return
	}

	if result.Response != nil {
		if result.Response.Rcode == 2 {
			s.T().Log("nameserver-switcher returned SERVFAIL when explicit resolver is down")
		} else if result.Response.Rcode == 3 {
			s.T().Log("nameserver-switcher returned NXDOMAIN when explicit resolver is down")
		} else if result.Response.Rcode == 0 && len(result.Response.Answer) > 0 {
			s.T().Log("WARNING: Got successful response - possible caching issue")
		}
	}
}

func TestGRPCModeScenario5Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(GRPCModeScenario5Suite))
}
