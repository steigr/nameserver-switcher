package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics("test")

	assert.NotNil(t, m)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.ResolverUsed)
	assert.NotNil(t, m.PatternMatches)
	assert.NotNil(t, m.CNAMEMatches)
	assert.NotNil(t, m.Errors)
	assert.NotNil(t, m.ActiveConnections)
	assert.NotNil(t, m.DNSResponseCodes)
}

func TestNewMetrics_DefaultNamespace(t *testing.T) {
	m := NewMetrics("")

	assert.NotNil(t, m)
}

func TestMetrics_RecordRequest(t *testing.T) {
	m := NewMetrics("test_request")

	// Should not panic
	m.RecordRequest("udp", "A")
	m.RecordRequest("tcp", "AAAA")
	m.RecordRequest("grpc", "CNAME")
}

func TestMetrics_RecordDuration(t *testing.T) {
	m := NewMetrics("test_duration")

	// Should not panic
	m.RecordDuration("request", 0.5)
	m.RecordDuration("explicit", 0.1)
	m.RecordDuration("system", 0.05)
}

func TestMetrics_RecordResolverUsed(t *testing.T) {
	m := NewMetrics("test_resolver")

	// Should not panic
	m.RecordResolverUsed("request")
	m.RecordResolverUsed("explicit")
	m.RecordResolverUsed("system")
}

func TestMetrics_RecordPatternMatch(t *testing.T) {
	m := NewMetrics("test_pattern")

	// Should not panic
	m.RecordPatternMatch(".*\\.example\\.com$")
}

func TestMetrics_RecordCNAMEMatch(t *testing.T) {
	m := NewMetrics("test_cname")

	// Should not panic
	m.RecordCNAMEMatch(".*\\.cdn\\.com$")
}

func TestMetrics_RecordError(t *testing.T) {
	m := NewMetrics("test_error")

	// Should not panic
	m.RecordError("resolution_failed")
	m.RecordError("timeout")
}

func TestMetrics_RecordResponseCode(t *testing.T) {
	m := NewMetrics("test_rcode")

	// Should not panic
	m.RecordResponseCode("NOERROR")
	m.RecordResponseCode("NXDOMAIN")
	m.RecordResponseCode("SERVFAIL")
}

func TestMetrics_ActiveConnections(t *testing.T) {
	m := NewMetrics("test_conn")

	// Should not panic
	m.IncActiveConnections()
	m.IncActiveConnections()
	m.DecActiveConnections()
}
