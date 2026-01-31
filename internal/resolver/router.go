// Package resolver provides the routing logic for DNS resolution.
package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/miekg/dns"

	"github.com/steigr/nameserver-switcher/internal/matcher"
)

// Router routes DNS requests to appropriate resolvers based on pattern matching.
type Router struct {
	requestMatcher          matcher.Matcher
	cnameMatcher            matcher.Matcher
	explicitResolver        Resolver
	passthroughResolver     Resolver
	noCnameResponseResolver Resolver
	noCnameMatchResolver    Resolver
}

// RouterConfig holds configuration for the router.
type RouterConfig struct {
	RequestMatcher          matcher.Matcher
	CNAMEMatcher            matcher.Matcher
	ExplicitResolver        Resolver
	PassthroughResolver     Resolver // Used when request doesn't match any pattern
	NoCnameResponseResolver Resolver // Used when response has no CNAME
	NoCnameMatchResolver    Resolver // Used when CNAME doesn't match pattern
	// Deprecated: Use PassthroughResolver, NoCnameResponseResolver, or NoCnameMatchResolver instead
	SystemResolver Resolver
}

// NewRouter creates a new Router with the given configuration.
func NewRouter(cfg RouterConfig) *Router {
	// Support backward compatibility: if new resolvers are not set, use SystemResolver
	passthroughResolver := cfg.PassthroughResolver
	if passthroughResolver == nil {
		passthroughResolver = cfg.SystemResolver
	}
	noCnameResponseResolver := cfg.NoCnameResponseResolver
	if noCnameResponseResolver == nil {
		noCnameResponseResolver = cfg.SystemResolver
	}
	noCnameMatchResolver := cfg.NoCnameMatchResolver
	if noCnameMatchResolver == nil {
		noCnameMatchResolver = cfg.SystemResolver
	}

	return &Router{
		requestMatcher:          cfg.RequestMatcher,
		cnameMatcher:            cfg.CNAMEMatcher,
		explicitResolver:        cfg.ExplicitResolver,
		passthroughResolver:     passthroughResolver,
		noCnameResponseResolver: noCnameResponseResolver,
		noCnameMatchResolver:    noCnameMatchResolver,
	}
}

// RouteResult contains information about how a request was routed.
type RouteResult struct {
	Response       *dns.Msg
	ResolverUsed   string
	RequestMatched bool
	CNAMEMatched   bool
	MatchedPattern string
	CNAMEPattern   string
}

// Route processes a DNS request according to the routing rules:
// 1. If request matches request patterns, do non-recursive lookup to explicit resolver
// 2. If response is CNAME and matches CNAME patterns, recursive lookup to explicit resolver
// 3. If CNAME doesn't match, use noCnameMatchResolver
// 4. If no CNAME in response, use noCnameResponseResolver
// 5. If no request pattern match, use passthroughResolver
func (r *Router) Route(ctx context.Context, req *dns.Msg) (*RouteResult, error) {
	if len(req.Question) == 0 {
		return nil, fmt.Errorf("no question in request")
	}

	qname := strings.TrimSuffix(req.Question[0].Name, ".")
	result := &RouteResult{}

	// Check if request matches any request pattern
	if r.shouldUseRequestMatcher(qname) {
		return r.routeMatchedRequest(ctx, req, qname, result)
	}

	// No pattern match, use passthroughResolver
	return r.routePassthrough(ctx, req, result)
}

// shouldUseRequestMatcher checks if the request matches any pattern
func (r *Router) shouldUseRequestMatcher(qname string) bool {
	return r.requestMatcher != nil && r.requestMatcher.Match(qname)
}

// routeMatchedRequest handles requests that match the request pattern
func (r *Router) routeMatchedRequest(ctx context.Context, req *dns.Msg, qname string, result *RouteResult) (*RouteResult, error) {
	result.RequestMatched = true
	result.MatchedPattern = r.requestMatcher.MatchingPattern(qname)

	// If no explicit resolver, use fallback
	if r.explicitResolver == nil {
		return r.useFallbackResolver(ctx, req, result)
	}

	// Query explicit resolver
	resp, err := r.explicitResolver.Resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("explicit resolver failed: %w", err)
	}

	// Process response based on CNAME presence
	return r.processExplicitResponse(ctx, req, resp, result)
}

// processExplicitResponse handles the response from explicit resolver
func (r *Router) processExplicitResponse(ctx context.Context, req *dns.Msg, resp *dns.Msg, result *RouteResult) (*RouteResult, error) {
	if !HasCNAME(resp) {
		return r.useNoCnameResponseResolver(ctx, req, result)
	}

	if r.cnameMatcher == nil {
		return r.useNoCnameResponseResolver(ctx, req, result)
	}

	// Check if any CNAME matches the pattern
	return r.processCNAMEMatch(ctx, req, resp, result)
}

// processCNAMEMatch checks CNAME records and routes accordingly
func (r *Router) processCNAMEMatch(ctx context.Context, req *dns.Msg, resp *dns.Msg, result *RouteResult) (*RouteResult, error) {
	cnames := ExtractCNAME(resp)

	for _, cname := range cnames {
		cname = strings.TrimSuffix(cname, ".")
		if r.cnameMatcher.Match(cname) {
			return r.handleMatchedCNAME(ctx, req, cname, result)
		}
	}

	// CNAME exists but doesn't match pattern
	return r.useNoCnameMatchResolver(ctx, req, result)
}

// handleMatchedCNAME handles a CNAME that matches the pattern
func (r *Router) handleMatchedCNAME(ctx context.Context, req *dns.Msg, cname string, result *RouteResult) (*RouteResult, error) {
	result.CNAMEMatched = true
	result.CNAMEPattern = r.cnameMatcher.MatchingPattern(cname)

	// Do recursive lookup to explicit resolver
	explicitResp, err := r.explicitResolver.Resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("explicit resolver failed: %w", err)
	}

	result.Response = explicitResp
	result.ResolverUsed = r.explicitResolver.Name()
	return result, nil
}

// useNoCnameMatchResolver uses the resolver for unmatched CNAMEs
func (r *Router) useNoCnameMatchResolver(ctx context.Context, req *dns.Msg, result *RouteResult) (*RouteResult, error) {
	if r.noCnameMatchResolver == nil {
		return nil, fmt.Errorf("no resolver available for matched pattern")
	}

	resp, err := r.noCnameMatchResolver.Resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("no-cname-match resolver failed: %w", err)
	}

	result.Response = resp
	result.ResolverUsed = r.noCnameMatchResolver.Name()
	return result, nil
}

// useNoCnameResponseResolver uses the resolver for responses without CNAME
func (r *Router) useNoCnameResponseResolver(ctx context.Context, req *dns.Msg, result *RouteResult) (*RouteResult, error) {
	if r.noCnameResponseResolver == nil {
		return nil, fmt.Errorf("no resolver available for matched pattern")
	}

	resp, err := r.noCnameResponseResolver.Resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("no-cname-response resolver failed: %w", err)
	}

	result.Response = resp
	result.ResolverUsed = r.noCnameResponseResolver.Name()
	return result, nil
}

// useFallbackResolver uses noCnameResponseResolver when no explicit resolver is configured
func (r *Router) useFallbackResolver(ctx context.Context, req *dns.Msg, result *RouteResult) (*RouteResult, error) {
	return r.useNoCnameResponseResolver(ctx, req, result)
}

// routePassthrough handles requests that don't match any pattern
func (r *Router) routePassthrough(ctx context.Context, req *dns.Msg, result *RouteResult) (*RouteResult, error) {
	if r.passthroughResolver == nil {
		return nil, fmt.Errorf("no resolver available")
	}

	resp, err := r.passthroughResolver.Resolve(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("passthrough resolver failed: %w", err)
	}

	result.Response = resp
	result.ResolverUsed = r.passthroughResolver.Name()
	return result, nil
}

// GetRequestMatcher returns the request matcher.
func (r *Router) GetRequestMatcher() matcher.Matcher {
	return r.requestMatcher
}

// GetCNAMEMatcher returns the CNAME matcher.
func (r *Router) GetCNAMEMatcher() matcher.Matcher {
	return r.cnameMatcher
}
