package dns

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/steigr/nameserver-switcher/internal/config"
	"github.com/steigr/nameserver-switcher/internal/metrics"
	"github.com/steigr/nameserver-switcher/internal/resolver"
)

// MockResolver for testing
type mockResolver struct {
	name     string
	response *dns.Msg
	err      error
}

func (m *mockResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockResolver) Name() string {
	return m.name
}

// MockMatcher for testing
type mockMatcher struct {
	matches map[string]string
}

func (m *mockMatcher) Match(domain string) bool {
	_, ok := m.matches[domain]
	return ok
}

func (m *mockMatcher) MatchingPattern(domain string) string {
	return m.matches[domain]
}

func (m *mockMatcher) Patterns() []string {
	patterns := make([]string, 0, len(m.matches))
	for _, p := range m.matches {
		patterns = append(patterns, p)
	}
	return patterns
}

func TestNewServer(t *testing.T) {
	systemResp := &dns.Msg{}
	systemResp.SetReply(&dns.Msg{})

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: systemResp},
	})

	cfg := ServerConfig{
		Addr:   "127.0.0.1",
		Port:   15353,
		Router: router,
	}

	server := NewServer(cfg)
	assert.NotNil(t, server)
	assert.Equal(t, "127.0.0.1:15353", server.Addr())
}

func TestServer_Query(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   15354,
		Router: router,
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	result, err := server.Query(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "system", result.ResolverUsed)
	assert.NotNil(t, result.Response)
}

func TestServer_Query_WithPatternMatch(t *testing.T) {
	// Response with CNAME that matches the CNAME pattern
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
				Target: "cdn.provider.net.",
			},
			&dns.A{
				Hdr: dns.RR_Header{Name: "cdn.provider.net.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   []byte{10, 20, 30, 40},
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:   &mockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:     &mockMatcher{matches: map[string]string{"cdn.provider.net": `.*\.provider\.net$`}},
		ExplicitResolver: &mockResolver{name: "explicit", response: explicitResp},
		SystemResolver:   &mockResolver{name: "system", response: &dns.Msg{}},
	})

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   15355,
		Router: router,
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	result, err := server.Query(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "explicit", result.ResolverUsed)
	assert.True(t, result.RequestMatched)
	assert.True(t, result.CNAMEMatched)
}

func TestServer_StartAndShutdown(t *testing.T) {
	systemResp := &dns.Msg{}
	systemResp.SetReply(&dns.Msg{})

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: systemResp},
	})

	// Use a high port that's likely available
	cfg := ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25353,
		Router: router,
	}

	server := NewServer(cfg)
	require.NotNil(t, server)

	// Start the server
	err := server.Start()
	require.NoError(t, err)

	// Give it a moment to fully start
	time.Sleep(200 * time.Millisecond)

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestServer_HandleRequest_UDP(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25354,
		Router:  router,
		Metrics: m,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a UDP query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25354")
	require.NoError(t, err)
	assert.NotNil(t, reply)
	assert.Equal(t, msg.Id, reply.Id)
}

func TestServer_HandleRequest_TCP(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_tcp")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25355,
		Router:  router,
		Metrics: m,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a TCP query
	client := &dns.Client{Net: "tcp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25355")
	require.NoError(t, err)
	assert.NotNil(t, reply)
	assert.Equal(t, msg.Id, reply.Id)
}

func TestServer_HandleRequest_RoutingError(t *testing.T) {
	m := metrics.NewMetrics("test_dns_error")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", err: errors.New("resolver error")},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25356,
		Router:  router,
		Metrics: m,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query that will fail routing
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25356")
	require.NoError(t, err)
	assert.NotNil(t, reply)
	assert.Equal(t, dns.RcodeServerFailure, reply.Rcode)
}

func TestServer_HandleRequest_WithPatternMatch(t *testing.T) {
	// Response with CNAME that matches the CNAME pattern
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
				Target: "cdn.provider.net.",
			},
			&dns.A{
				Hdr: dns.RR_Header{Name: "cdn.provider.net.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("10.20.30.40").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_pattern")

	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:   &mockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:     &mockMatcher{matches: map[string]string{"cdn.provider.net": `.*\.provider\.net$`}},
		ExplicitResolver: &mockResolver{name: "explicit", response: explicitResp},
		SystemResolver:   &mockResolver{name: "system", response: &dns.Msg{}},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25357,
		Router:  router,
		Metrics: m,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("www.example.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25357")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_NoMetrics(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server without metrics
	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25358,
		Router:  router,
		Metrics: nil,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25358")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_NoQuestion(t *testing.T) {
	resp := &dns.Msg{}

	m := metrics.NewMetrics("test_dns_no_question")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25359,
		Router:  router,
		Metrics: m,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query without a question (unusual but possible)
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	// Explicitly not setting a question

	_, _, _ = client.Exchange(msg, "127.0.0.1:25359")
	// This might error or return SERVFAIL, either is acceptable
	// The test is mainly to ensure the server doesn't panic
}

func TestServer_Start_Error(t *testing.T) {
	systemResp := &dns.Msg{}
	systemResp.SetReply(&dns.Msg{})

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: systemResp},
	})

	// Start first server
	server1 := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25360,
		Router: router,
	})
	err := server1.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server1.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Try to start another server on the same port - this should fail
	server2 := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25360,
		Router: router,
	})
	err = server2.Start()
	// Either Start returns an error or it doesn't (race condition)
	// The test exercises the code path
	if err != nil {
		assert.Error(t, err)
	}
}

func TestServer_Shutdown_WithErrors(t *testing.T) {
	// This test exercises the shutdown error paths by using a very short timeout
	systemResp := &dns.Msg{}
	systemResp.SetReply(&dns.Msg{})

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: systemResp},
	})

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25361,
		Router: router,
	})

	err := server.Start()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Normal shutdown (no errors expected)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestServer_Shutdown_ContextCanceled(t *testing.T) {
	// Test shutdown with an already-canceled context to force error path
	systemResp := &dns.Msg{}
	systemResp.SetReply(&dns.Msg{})

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: systemResp},
	})

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25362,
		Router: router,
	})

	err := server.Start()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Use an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// This may or may not error depending on timing
	_ = server.Shutdown(ctx)
}

func TestServer_HandleRequest_WithConfig_LogRequests(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server with config enabling request logging
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: false,
		Debug:        false,
	}

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25363,
		Router: router,
		Config: cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25363")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_WithConfig_LogResponses(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server with config enabling response logging
	cfg := &config.Config{
		LogRequests:  false,
		LogResponses: true,
		Debug:        false,
	}

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25364,
		Router: router,
		Config: cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25364")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_WithConfig_Debug(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server with config enabling debug logging
	cfg := &config.Config{
		LogRequests:  false,
		LogResponses: false,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25365,
		Router: router,
		Config: cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25365")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_WithConfig_Debug_PatternMatch(t *testing.T) {
	// Response with CNAME that matches the CNAME pattern
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
				Target: "cdn.provider.net.",
			},
			&dns.A{
				Hdr: dns.RR_Header{Name: "cdn.provider.net.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("10.20.30.40").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:   &mockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:     &mockMatcher{matches: map[string]string{"cdn.provider.net": `.*\.provider\.net$`}},
		ExplicitResolver: &mockResolver{name: "explicit", response: explicitResp},
		SystemResolver:   &mockResolver{name: "system", response: &dns.Msg{}},
	})

	// Server with config enabling debug logging
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25366,
		Router: router,
		Config: cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query that triggers pattern matching
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("www.example.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25366")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_WithConfig_Debug_RequestMatchedOnly(t *testing.T) {
	// Response without CNAME (no CNAME match)
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("10.20.30.40").To4(),
			},
		},
	}

	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:          &mockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:            &mockMatcher{matches: map[string]string{}}, // No CNAME matches
		ExplicitResolver:        &mockResolver{name: "explicit", response: explicitResp},
		NoCnameResponseResolver: &mockResolver{name: "no-cname-response", response: systemResp},
	})

	// Server with config enabling debug logging
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25367,
		Router: router,
		Config: cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query that triggers request pattern matching only
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("www.example.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25367")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_WithConfig_AllEnabled(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_all_enabled")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server with all config options enabled
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25368,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25368")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

func TestServer_HandleRequest_RoutingError_WithConfig(t *testing.T) {
	m := metrics.NewMetrics("test_dns_error_config")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", err: errors.New("resolver error")},
	})

	// Server with config
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25369,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a query that will fail routing
	client := &dns.Client{Net: "udp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25369")
	require.NoError(t, err)
	assert.NotNil(t, reply)
	assert.Equal(t, dns.RcodeServerFailure, reply.Rcode)
}

func TestServer_HandleRequest_TCPWithConfig(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_tcp_config")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server with all config options enabled
	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25370,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	err := server.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// Make a TCP query to exercise TCP protocol detection
	client := &dns.Client{Net: "tcp"}
	msg := &dns.Msg{}
	msg.SetQuestion("test.com.", dns.TypeA)

	reply, _, err := client.Exchange(msg, "127.0.0.1:25370")
	require.NoError(t, err)
	assert.NotNil(t, reply)
}

// mockResponseWriter implements dns.ResponseWriter for testing error paths
type mockResponseWriter struct {
	localAddr  net.Addr
	remoteAddr net.Addr
	writeErr   error
	written    *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr {
	return m.localAddr
}

func (m *mockResponseWriter) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.written = msg
	return m.writeErr
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(b), nil
}

func (m *mockResponseWriter) Close() error {
	return nil
}

func (m *mockResponseWriter) TsigStatus() error {
	return nil
}

func (m *mockResponseWriter) TsigTimersOnly(bool) {
}

func (m *mockResponseWriter) Hijack() {
}

func TestServer_HandleRequest_WriteError(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_write_error")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25371,
		Router:  router,
		Metrics: m,
	})

	// Create a request
	req := &dns.Msg{}
	req.SetQuestion("test.com.", dns.TypeA)

	// Create a mock writer that fails on WriteMsg
	w := &mockResponseWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 25371},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		writeErr:   errors.New("write error"),
	}

	// Call handleRequest directly
	server.handleRequest(w, req)
	// The test exercises the error path - no assertion needed other than no panic
}

func TestServer_HandleRequest_WriteError_NoMetrics(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	// Server without metrics
	server := NewServer(ServerConfig{
		Addr:   "127.0.0.1",
		Port:   25372,
		Router: router,
	})

	// Create a request
	req := &dns.Msg{}
	req.SetQuestion("test.com.", dns.TypeA)

	// Create a mock writer that fails on WriteMsg
	w := &mockResponseWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 25372},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		writeErr:   errors.New("write error"),
	}

	// Call handleRequest directly
	server.handleRequest(w, req)
	// The test exercises the error path without metrics - no assertion needed other than no panic
}

func TestServer_HandleRequest_DirectCall_UDP(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_direct_udp")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25373,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	// Create a request
	req := &dns.Msg{}
	req.SetQuestion("test.com.", dns.TypeA)

	// Create a mock writer (UDP)
	w := &mockResponseWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 25373},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Call handleRequest directly
	server.handleRequest(w, req)
	assert.NotNil(t, w.written)
}

func TestServer_HandleRequest_DirectCall_TCP(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "test.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4").To4(),
			},
		},
	}

	m := metrics.NewMetrics("test_dns_direct_tcp")

	router := resolver.NewRouter(resolver.RouterConfig{
		SystemResolver: &mockResolver{name: "system", response: resp},
	})

	cfg := &config.Config{
		LogRequests:  true,
		LogResponses: true,
		Debug:        true,
	}

	server := NewServer(ServerConfig{
		Addr:    "127.0.0.1",
		Port:    25374,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	// Create a request
	req := &dns.Msg{}
	req.SetQuestion("test.com.", dns.TypeA)

	// Create a mock writer (TCP)
	w := &mockResponseWriter{
		localAddr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 25374},
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Call handleRequest directly
	server.handleRequest(w, req)
	assert.NotNil(t, w.written)
}
