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

	qname := req.Question[0].Name
	qname = strings.TrimSuffix(qname, ".")

	result := &RouteResult{}

	// Step 1: Check if request matches any request pattern
	if r.requestMatcher != nil && r.requestMatcher.Match(qname) {
		result.RequestMatched = true
		result.MatchedPattern = r.requestMatcher.MatchingPattern(qname)

		// Do non-recursive lookup to explicit resolver
		if r.explicitResolver != nil {
			resp, err := r.explicitResolver.Resolve(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("explicit resolver failed: %w", err)
			}

			// Step 2: Check if response contains CNAME that matches CNAME patterns
			if HasCNAME(resp) && r.cnameMatcher != nil {
				cnames := ExtractCNAME(resp)
				for _, cname := range cnames {
					cname = strings.TrimSuffix(cname, ".")
					if r.cnameMatcher.Match(cname) {
						result.CNAMEMatched = true
						result.CNAMEPattern = r.cnameMatcher.MatchingPattern(cname)

						// Step 3: Do recursive lookup to explicit resolver
						// Use the same explicit resolver for recursive lookup
						explicitResp, err := r.explicitResolver.Resolve(ctx, req)
						if err != nil {
							return nil, fmt.Errorf("explicit resolver failed: %w", err)
						}
						result.Response = explicitResp
						result.ResolverUsed = r.explicitResolver.Name()
						return result, nil
					}
				}

				// CNAME exists but doesn't match pattern: use noCnameMatchResolver
				if r.noCnameMatchResolver != nil {
					sysResp, err := r.noCnameMatchResolver.Resolve(ctx, req)
					if err != nil {
						return nil, fmt.Errorf("no-cname-match resolver failed: %w", err)
					}
					result.Response = sysResp
					result.ResolverUsed = r.noCnameMatchResolver.Name()
					return result, nil
				}
			} else {
				// No CNAME in response: use noCnameResponseResolver
				if r.noCnameResponseResolver != nil {
					sysResp, err := r.noCnameResponseResolver.Resolve(ctx, req)
					if err != nil {
						return nil, fmt.Errorf("no-cname-response resolver failed: %w", err)
					}
					result.Response = sysResp
					result.ResolverUsed = r.noCnameResponseResolver.Name()
					return result, nil
				}
			}
		}

		// If no explicit resolver configured but pattern matched, fall back to noCnameResponseResolver
		if r.noCnameResponseResolver != nil {
			sysResp, err := r.noCnameResponseResolver.Resolve(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("no-cname-response resolver failed: %w", err)
			}
			result.Response = sysResp
			result.ResolverUsed = r.noCnameResponseResolver.Name()
			return result, nil
		}

		return nil, fmt.Errorf("no resolver available for matched pattern")
	}

	// Step 5: No pattern match, use passthroughResolver
	if r.passthroughResolver != nil {
		resp, err := r.passthroughResolver.Resolve(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("passthrough resolver failed: %w", err)
		}
		result.Response = resp
		result.ResolverUsed = r.passthroughResolver.Name()
		return result, nil
	}

	return nil, fmt.Errorf("no resolver available")
}

// GetRequestMatcher returns the request matcher.
func (r *Router) GetRequestMatcher() matcher.Matcher {
	return r.requestMatcher
}

// GetCNAMEMatcher returns the CNAME matcher.
func (r *Router) GetCNAMEMatcher() matcher.Matcher {
	return r.cnameMatcher
}
