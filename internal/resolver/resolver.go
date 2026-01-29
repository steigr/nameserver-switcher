// Package resolver provides DNS resolution with different strategies.
package resolver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Resolver defines the interface for DNS resolution.
type Resolver interface {
	// Resolve performs a DNS lookup and returns the response.
	Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error)
	// Name returns the resolver name for logging/metrics.
	Name() string
}

// DNSResolver performs DNS lookups against a specific server.
type DNSResolver struct {
	server    string
	client    *dns.Client
	recursive bool
	name      string
}

// NewDNSResolver creates a new DNS resolver.
func NewDNSResolver(server string, recursive bool, name string) *DNSResolver {
	if !strings.Contains(server, ":") {
		server = server + ":53"
	}

	return &DNSResolver{
		server: server,
		client: &dns.Client{
			Net:     "udp",
			Timeout: 5 * time.Second,
		},
		recursive: recursive,
		name:      name,
	}
}

// Resolve performs a DNS lookup.
func (r *DNSResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	reqCopy := req.Copy()

	if r.recursive {
		reqCopy.RecursionDesired = true
	} else {
		reqCopy.RecursionDesired = false
	}

	resp, _, err := r.client.ExchangeContext(ctx, reqCopy, r.server)
	if err != nil {
		return nil, fmt.Errorf("DNS query failed: %w", err)
	}

	return resp, nil
}

// Name returns the resolver name.
func (r *DNSResolver) Name() string {
	return r.name
}

// Server returns the server address.
func (r *DNSResolver) Server() string {
	return r.server
}

// SystemResolver uses the system's default DNS resolver.
type SystemResolver struct {
	servers []string
	client  *dns.Client
}

// NewSystemResolver creates a resolver using system DNS settings.
func NewSystemResolver() (*SystemResolver, error) {
	config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to read /etc/resolv.conf: %w", err)
	}

	servers := make([]string, len(config.Servers))
	for i, s := range config.Servers {
		if !strings.Contains(s, ":") {
			servers[i] = net.JoinHostPort(s, config.Port)
		} else {
			servers[i] = s
		}
	}

	if len(servers) == 0 {
		servers = []string{"8.8.8.8:53", "8.8.4.4:53"}
	}

	return &SystemResolver{
		servers: servers,
		client: &dns.Client{
			Net:     "udp",
			Timeout: 5 * time.Second,
		},
	}, nil
}

// NewSystemResolverWithServers creates a system resolver with explicit servers.
func NewSystemResolverWithServers(servers []string) *SystemResolver {
	for i, s := range servers {
		if !strings.Contains(s, ":") {
			servers[i] = s + ":53"
		}
	}

	return &SystemResolver{
		servers: servers,
		client: &dns.Client{
			Net:     "udp",
			Timeout: 5 * time.Second,
		},
	}
}

// Resolve performs a DNS lookup using system resolvers.
func (r *SystemResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	reqCopy := req.Copy()
	reqCopy.RecursionDesired = true

	var lastErr error
	for _, server := range r.servers {
		resp, _, err := r.client.ExchangeContext(ctx, reqCopy, server)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all system resolvers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no system resolvers available")
}

// Name returns the resolver name.
func (r *SystemResolver) Name() string {
	return "system"
}

// Servers returns the configured servers.
func (r *SystemResolver) Servers() []string {
	return r.servers
}

// ExtractCNAME extracts CNAME targets from a DNS response.
func ExtractCNAME(resp *dns.Msg) []string {
	var cnames []string
	for _, rr := range resp.Answer {
		if cname, ok := rr.(*dns.CNAME); ok {
			cnames = append(cnames, cname.Target)
		}
	}
	return cnames
}

// HasCNAME checks if the response contains any CNAME records.
func HasCNAME(resp *dns.Msg) bool {
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.CNAME); ok {
			return true
		}
	}
	return false
}
