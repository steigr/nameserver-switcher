package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDNSResolver(t *testing.T) {
	tests := []struct {
		name           string
		server         string
		recursive      bool
		resolverName   string
		expectedServer string
	}{
		{
			name:           "with port",
			server:         "8.8.8.8:53",
			recursive:      true,
			resolverName:   "google",
			expectedServer: "8.8.8.8:53",
		},
		{
			name:           "without port",
			server:         "8.8.8.8",
			recursive:      false,
			resolverName:   "google",
			expectedServer: "8.8.8.8:53",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDNSResolver(tt.server, tt.recursive, tt.resolverName)
			assert.NotNil(t, r)
			assert.Equal(t, tt.expectedServer, r.Server())
			assert.Equal(t, tt.resolverName, r.Name())
		})
	}
}

func TestNewSystemResolverWithServers(t *testing.T) {
	servers := []string{"8.8.8.8", "1.1.1.1:53"}
	r := NewSystemResolverWithServers(servers)

	assert.NotNil(t, r)
	assert.Equal(t, "system", r.Name())
	assert.Contains(t, r.Servers(), "8.8.8.8:53")
	assert.Contains(t, r.Servers(), "1.1.1.1:53")
}

func TestExtractCNAME(t *testing.T) {
	tests := []struct {
		name     string
		response *dns.Msg
		expected []string
	}{
		{
			name: "no CNAME",
			response: &dns.Msg{
				Answer: []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA},
						A:   []byte{1, 2, 3, 4},
					},
				},
			},
			expected: nil,
		},
		{
			name: "single CNAME",
			response: &dns.Msg{
				Answer: []dns.RR{
					&dns.CNAME{
						Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME},
						Target: "cdn.example.com.",
					},
				},
			},
			expected: []string{"cdn.example.com."},
		},
		{
			name: "multiple CNAMEs",
			response: &dns.Msg{
				Answer: []dns.RR{
					&dns.CNAME{
						Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME},
						Target: "cdn.example.com.",
					},
					&dns.A{
						Hdr: dns.RR_Header{Name: "cdn.example.com.", Rrtype: dns.TypeA},
						A:   []byte{1, 2, 3, 4},
					},
					&dns.CNAME{
						Hdr:    dns.RR_Header{Name: "cdn.example.com.", Rrtype: dns.TypeCNAME},
						Target: "origin.example.com.",
					},
				},
			},
			expected: []string{"cdn.example.com.", "origin.example.com."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCNAME(tt.response)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasCNAME(t *testing.T) {
	tests := []struct {
		name     string
		response *dns.Msg
		expected bool
	}{
		{
			name: "no CNAME",
			response: &dns.Msg{
				Answer: []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA},
						A:   []byte{1, 2, 3, 4},
					},
				},
			},
			expected: false,
		},
		{
			name: "has CNAME",
			response: &dns.Msg{
				Answer: []dns.RR{
					&dns.CNAME{
						Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME},
						Target: "cdn.example.com.",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasCNAME(tt.response)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// MockResolver is a mock resolver for testing.
type MockResolver struct {
	name     string
	response *dns.Msg
	err      error
}

func (m *MockResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *MockResolver) Name() string {
	return m.name
}

// MockMatcher is a mock matcher for testing.
type MockMatcher struct {
	matches map[string]string
}

func (m *MockMatcher) Match(domain string) bool {
	_, ok := m.matches[domain]
	return ok
}

func (m *MockMatcher) MatchingPattern(domain string) string {
	return m.matches[domain]
}

func (m *MockMatcher) Patterns() []string {
	patterns := make([]string, 0, len(m.matches))
	for _, p := range m.matches {
		patterns = append(patterns, p)
	}
	return patterns
}

func TestRouter_Route_NoPatternMatch(t *testing.T) {
	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "random.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:      &MockMatcher{matches: map[string]string{}},
		PassthroughResolver: &MockResolver{name: "passthrough", response: systemResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("random.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "passthrough", result.ResolverUsed)
	assert.False(t, result.RequestMatched)
	assert.False(t, result.CNAMEMatched)
}

func TestRouter_Route_RequestMatch_NoCNAME(t *testing.T) {
	requestResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA},
				A:   []byte{5, 6, 7, 8},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:          &MockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:            &MockMatcher{matches: map[string]string{}},
		ExplicitResolver:        &MockResolver{name: "explicit", response: requestResp},
		NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: systemResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "no-cname-response", result.ResolverUsed)
	assert.True(t, result.RequestMatched)
	assert.False(t, result.CNAMEMatched)
}

func TestRouter_Route_RequestMatch_CNAMEMatch(t *testing.T) {
	explicitRespWithCNAME := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME},
				Target: "cdn.provider.net.",
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:       &MockMatcher{matches: map[string]string{"www.example.com": `.*\.example\.com$`}},
		CNAMEMatcher:         &MockMatcher{matches: map[string]string{"cdn.provider.net": `.*\.provider\.net$`}},
		ExplicitResolver:     &MockResolver{name: "explicit", response: explicitRespWithCNAME},
		NoCnameMatchResolver: &MockResolver{name: "no-cname-match", response: &dns.Msg{}},
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "explicit", result.ResolverUsed)
	assert.True(t, result.RequestMatched)
	assert.True(t, result.CNAMEMatched)
}

func TestRouter_Route_NoQuestion(t *testing.T) {
	router := NewRouter(RouterConfig{})

	req := &dns.Msg{}

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no question in request")
}

func TestRouter_GetRequestMatcher(t *testing.T) {
	mockMatcher := &MockMatcher{matches: map[string]string{"example.com": "pattern"}}
	router := NewRouter(RouterConfig{
		RequestMatcher: mockMatcher,
	})

	result := router.GetRequestMatcher()
	assert.Equal(t, mockMatcher, result)
}

func TestRouter_GetCNAMEMatcher(t *testing.T) {
	mockMatcher := &MockMatcher{matches: map[string]string{"cdn.com": "pattern"}}
	router := NewRouter(RouterConfig{
		CNAMEMatcher: mockMatcher,
	})

	result := router.GetCNAMEMatcher()
	assert.Equal(t, mockMatcher, result)
}

func TestRouter_Route_NoResolver(t *testing.T) {
	router := NewRouter(RouterConfig{
		RequestMatcher: &MockMatcher{matches: map[string]string{}},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no resolver available")
}

func TestRouter_Route_ExplicitResolverError_OnMatch(t *testing.T) {
	router := NewRouter(RouterConfig{
		RequestMatcher:          &MockMatcher{matches: map[string]string{"example.com": "pattern"}},
		ExplicitResolver:        &MockResolver{name: "explicit", err: assert.AnError},
		NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: &dns.Msg{}},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "explicit resolver failed")
}

func TestRouter_Route_ExplicitResolverError(t *testing.T) {
	router := NewRouter(RouterConfig{
		RequestMatcher:       &MockMatcher{matches: map[string]string{"www.example.com": "pattern"}},
		CNAMEMatcher:         &MockMatcher{matches: map[string]string{"cdn.provider.net": "cname-pattern"}},
		ExplicitResolver:     &MockResolver{name: "explicit", err: assert.AnError},
		NoCnameMatchResolver: &MockResolver{name: "no-cname-match", response: &dns.Msg{}},
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "explicit resolver failed")
}

func TestRouter_Route_PassthroughResolverError(t *testing.T) {
	router := NewRouter(RouterConfig{
		RequestMatcher:      &MockMatcher{matches: map[string]string{}},
		PassthroughResolver: &MockResolver{name: "passthrough", err: assert.AnError},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "passthrough resolver failed")
}

func TestRouter_Route_CNAMENoMatch_UsesNoCnameMatchResolver(t *testing.T) {
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME},
				Target: "cdn.other.net.",
			},
		},
	}

	noCnameMatchResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:       &MockMatcher{matches: map[string]string{"www.example.com": "pattern"}},
		CNAMEMatcher:         &MockMatcher{matches: map[string]string{"cdn.provider.net": "cname-pattern"}},
		ExplicitResolver:     &MockResolver{name: "explicit", response: explicitResp},
		NoCnameMatchResolver: &MockResolver{name: "no-cname-match", response: noCnameMatchResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "no-cname-match", result.ResolverUsed)
	assert.True(t, result.RequestMatched)
	assert.False(t, result.CNAMEMatched)
}

func TestRouter_Route_NilRequestMatcher(t *testing.T) {
	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:      nil,
		PassthroughResolver: &MockResolver{name: "passthrough", response: systemResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "passthrough", result.ResolverUsed)
	assert.False(t, result.RequestMatched)
}

func TestRouter_Route_RequestMatch_NoExplicitResolver(t *testing.T) {
	// Test case: Request matches but no explicit resolver is configured
	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:          &MockMatcher{matches: map[string]string{"www.example.com": "pattern"}},
		ExplicitResolver:        nil, // No explicit resolver
		NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: systemResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("www.example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "no-cname-response", result.ResolverUsed)
	assert.True(t, result.RequestMatched)
}

// Test backward compatibility: SystemResolver is used when specific resolvers are not set
func TestRouter_Route_BackwardCompatibility_SystemResolver(t *testing.T) {
	systemResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	// Using deprecated SystemResolver field for backward compatibility
	router := NewRouter(RouterConfig{
		RequestMatcher: &MockMatcher{matches: map[string]string{}},
		SystemResolver: &MockResolver{name: "system", response: systemResp},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	result, err := router.Route(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "system", result.ResolverUsed)
}

// Test that each resolver can be independently configured
func TestRouter_Route_IndependentResolvers(t *testing.T) {
	passthroughResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "unmatched.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 1, 1, 1},
			},
		},
	}

	noCnameResponseResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "direct.example.com.", Rrtype: dns.TypeA},
				A:   []byte{2, 2, 2, 2},
			},
		},
	}

	noCnameMatchResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "external.example.com.", Rrtype: dns.TypeA},
				A:   []byte{3, 3, 3, 3},
			},
		},
	}

	explicitRespNoCname := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "direct.example.com.", Rrtype: dns.TypeA},
				A:   []byte{10, 10, 10, 10},
			},
		},
	}

	explicitRespWithCname := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "external.example.com.", Rrtype: dns.TypeCNAME},
				Target: "external.provider.net.",
			},
		},
	}

	t.Run("passthrough resolver for unmatched requests", func(t *testing.T) {
		router := NewRouter(RouterConfig{
			RequestMatcher:          &MockMatcher{matches: map[string]string{"example.com": "pattern"}},
			CNAMEMatcher:            &MockMatcher{matches: map[string]string{"match.cdn.net": "cname-pattern"}},
			PassthroughResolver:     &MockResolver{name: "passthrough", response: passthroughResp},
			NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: noCnameResponseResp},
			NoCnameMatchResolver:    &MockResolver{name: "no-cname-match", response: noCnameMatchResp},
		})

		req := &dns.Msg{}
		req.SetQuestion("unmatched.com.", dns.TypeA)

		result, err := router.Route(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "passthrough", result.ResolverUsed)
		assert.False(t, result.RequestMatched)
	})

	t.Run("no-cname-response resolver for responses without CNAME", func(t *testing.T) {
		router := NewRouter(RouterConfig{
			RequestMatcher:          &MockMatcher{matches: map[string]string{"direct.example.com": "pattern"}},
			CNAMEMatcher:            &MockMatcher{matches: map[string]string{"match.cdn.net": "cname-pattern"}},
			ExplicitResolver:        &MockResolver{name: "explicit", response: explicitRespNoCname},
			PassthroughResolver:     &MockResolver{name: "passthrough", response: passthroughResp},
			NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: noCnameResponseResp},
			NoCnameMatchResolver:    &MockResolver{name: "no-cname-match", response: noCnameMatchResp},
		})

		req := &dns.Msg{}
		req.SetQuestion("direct.example.com.", dns.TypeA)

		result, err := router.Route(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "no-cname-response", result.ResolverUsed)
		assert.True(t, result.RequestMatched)
		assert.False(t, result.CNAMEMatched)
	})

	t.Run("no-cname-match resolver for CNAME that doesn't match pattern", func(t *testing.T) {
		router := NewRouter(RouterConfig{
			RequestMatcher:          &MockMatcher{matches: map[string]string{"external.example.com": "pattern"}},
			CNAMEMatcher:            &MockMatcher{matches: map[string]string{"match.cdn.net": "cname-pattern"}},
			ExplicitResolver:        &MockResolver{name: "explicit", response: explicitRespWithCname},
			PassthroughResolver:     &MockResolver{name: "passthrough", response: passthroughResp},
			NoCnameResponseResolver: &MockResolver{name: "no-cname-response", response: noCnameResponseResp},
			NoCnameMatchResolver:    &MockResolver{name: "no-cname-match", response: noCnameMatchResp},
		})

		req := &dns.Msg{}
		req.SetQuestion("external.example.com.", dns.TypeA)

		result, err := router.Route(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "no-cname-match", result.ResolverUsed)
		assert.True(t, result.RequestMatched)
		assert.False(t, result.CNAMEMatched)
	})
}

// Test error handling for each resolver type
func TestRouter_Route_NoCnameResponseResolverError(t *testing.T) {
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA},
				A:   []byte{1, 2, 3, 4},
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:          &MockMatcher{matches: map[string]string{"example.com": "pattern"}},
		ExplicitResolver:        &MockResolver{name: "explicit", response: explicitResp},
		NoCnameResponseResolver: &MockResolver{name: "no-cname-response", err: assert.AnError},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no-cname-response resolver failed")
}

func TestRouter_Route_NoCnameMatchResolverError(t *testing.T) {
	explicitResp := &dns.Msg{
		Answer: []dns.RR{
			&dns.CNAME{
				Hdr:    dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeCNAME},
				Target: "external.net.",
			},
		},
	}

	router := NewRouter(RouterConfig{
		RequestMatcher:       &MockMatcher{matches: map[string]string{"example.com": "pattern"}},
		CNAMEMatcher:         &MockMatcher{matches: map[string]string{"match.cdn.net": "cname-pattern"}},
		ExplicitResolver:     &MockResolver{name: "explicit", response: explicitResp},
		NoCnameMatchResolver: &MockResolver{name: "no-cname-match", err: assert.AnError},
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	_, err := router.Route(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no-cname-match resolver failed")
}

func TestDNSResolver_Resolve(t *testing.T) {
	// This test requires a real DNS server
	// Skip if running in a restricted environment
	t.Run("basic resolve", func(t *testing.T) {
		r := NewDNSResolver("8.8.8.8:53", true, "google")

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := r.Resolve(ctx, req)
		if err != nil {
			// Network may not be available, skip gracefully
			t.Skipf("Network error (this is okay in isolated environments): %v", err)
		}
		assert.NotNil(t, resp)
	})

	t.Run("non-recursive resolve", func(t *testing.T) {
		r := NewDNSResolver("8.8.8.8:53", false, "google-non-recursive")

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := r.Resolve(ctx, req)
		if err != nil {
			t.Skipf("Network error (this is okay in isolated environments): %v", err)
		}
		assert.NotNil(t, resp)
	})

	t.Run("timeout", func(t *testing.T) {
		// Use a non-routable IP to force timeout
		r := NewDNSResolver("10.255.255.1:53", true, "timeout-test")

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := r.Resolve(ctx, req)
		// In some network environments, this might not error (e.g., quick ICMP unreachable)
		// The important thing is the code path is exercised
		if err == nil {
			t.Skip("Network responded unexpectedly fast (this is okay)")
		}
		assert.Error(t, err)
	})
}

func TestSystemResolver_Resolve(t *testing.T) {
	t.Run("with custom servers", func(t *testing.T) {
		r := NewSystemResolverWithServers([]string{"8.8.8.8:53", "8.8.4.4:53"})

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := r.Resolve(ctx, req)
		if err != nil {
			t.Skipf("Network error (this is okay in isolated environments): %v", err)
		}
		assert.NotNil(t, resp)
	})

	t.Run("all servers fail", func(t *testing.T) {
		// Use non-routable IPs to force all failures
		r := NewSystemResolverWithServers([]string{"10.255.255.1:53", "10.255.255.2:53"})

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		_, err := r.Resolve(ctx, req)
		// In some network environments, this might not error
		if err == nil {
			t.Skip("Network responded unexpectedly (this is okay)")
		}
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "all system resolvers failed")
	})

	t.Run("no servers available", func(t *testing.T) {
		// Create a resolver with an empty servers list
		r := &SystemResolver{
			servers: []string{},
			client: &dns.Client{
				Net:     "udp",
				Timeout: 5 * time.Second,
			},
		}

		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := r.Resolve(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no system resolvers available")
	})
}

func TestNewSystemResolver(t *testing.T) {
	// This test depends on /etc/resolv.conf existing
	r, err := NewSystemResolver()
	if err != nil {
		// This is okay in environments without /etc/resolv.conf
		t.Skipf("Could not create system resolver (this is okay in some environments): %v", err)
	}

	assert.NotNil(t, r)
	assert.Equal(t, "system", r.Name())
	assert.NotEmpty(t, r.Servers())
}

func TestDNSResolver_Server(t *testing.T) {
	r := NewDNSResolver("1.1.1.1:53", true, "cloudflare")
	assert.Equal(t, "1.1.1.1:53", r.Server())

	r2 := NewDNSResolver("1.1.1.1", true, "cloudflare")
	assert.Equal(t, "1.1.1.1:53", r2.Server())
}
