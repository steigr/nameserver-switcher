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
	requestMatcher   matcher.Matcher
	cnameMatcher     matcher.Matcher
	requestResolver  Resolver
	explicitResolver Resolver
	systemResolver   Resolver
}

// RouterConfig holds configuration for the router.
type RouterConfig struct {
	RequestMatcher   matcher.Matcher
	CNAMEMatcher     matcher.Matcher
	RequestResolver  Resolver
	ExplicitResolver Resolver
	SystemResolver   Resolver
}

// NewRouter creates a new Router with the given configuration.
func NewRouter(cfg RouterConfig) *Router {
	return &Router{
		requestMatcher:   cfg.RequestMatcher,
		cnameMatcher:     cfg.CNAMEMatcher,
		requestResolver:  cfg.RequestResolver,
		explicitResolver: cfg.ExplicitResolver,
		systemResolver:   cfg.SystemResolver,
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
// 1. If request matches request patterns, do non-recursive lookup to request resolver
// 2. If response is CNAME and matches CNAME patterns, recursive lookup to explicit resolver
// 3. Otherwise, use system resolver
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

		// Do non-recursive lookup to request resolver
		if r.requestResolver != nil {
			resp, err := r.requestResolver.Resolve(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("request resolver failed: %w", err)
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
						if r.explicitResolver != nil {
							explicitResp, err := r.explicitResolver.Resolve(ctx, req)
							if err != nil {
								return nil, fmt.Errorf("explicit resolver failed: %w", err)
							}
							result.Response = explicitResp
							result.ResolverUsed = r.explicitResolver.Name()
							return result, nil
						}
						break
					}
				}
			}

			// No CNAME match, return the request resolver response
			// or fall through to system resolver
			if !result.CNAMEMatched {
				// If we got a response but CNAME didn't match, use system resolver
				if r.systemResolver != nil {
					sysResp, err := r.systemResolver.Resolve(ctx, req)
					if err != nil {
						return nil, fmt.Errorf("system resolver failed: %w", err)
					}
					result.Response = sysResp
					result.ResolverUsed = r.systemResolver.Name()
					return result, nil
				}
			}
		}
	}

	// Step 4: No pattern match, use system resolver
	if r.systemResolver != nil {
		resp, err := r.systemResolver.Resolve(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("system resolver failed: %w", err)
		}
		result.Response = resp
		result.ResolverUsed = r.systemResolver.Name()
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
