// Package integration provides integration testing for nameserver-switcher.
package integration

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

// DNSClient provides DNS query functionality for tests.
type DNSClient struct {
	Timeout time.Duration
}

// NewDNSClient creates a new DNS client with default settings.
func NewDNSClient() *DNSClient {
	return &DNSClient{
		Timeout: 5 * time.Second,
	}
}

// QueryResult holds the result of a DNS query.
type QueryResult struct {
	Response *dns.Msg
	RTT      time.Duration
	Error    error
}

// Query performs a DNS query for the given domain and record type.
func (c *DNSClient) Query(server string, domain string, qtype uint16) *QueryResult {
	client := &dns.Client{
		Timeout: c.Timeout,
		Net:     "udp",
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), qtype)
	msg.RecursionDesired = true

	resp, rtt, err := client.Exchange(msg, server)
	return &QueryResult{
		Response: resp,
		RTT:      rtt,
		Error:    err,
	}
}

// QueryA performs an A record query.
func (c *DNSClient) QueryA(server, domain string) *QueryResult {
	return c.Query(server, domain, dns.TypeA)
}

// QueryCNAME performs a CNAME record query.
func (c *DNSClient) QueryCNAME(server, domain string) *QueryResult {
	return c.Query(server, domain, dns.TypeCNAME)
}

// HasCNAME checks if the response contains a CNAME record with the given target.
func (r *QueryResult) HasCNAME(target string) bool {
	if r.Response == nil {
		return false
	}
	targetFQDN := dns.Fqdn(target)
	for _, ans := range r.Response.Answer {
		if cname, ok := ans.(*dns.CNAME); ok {
			if cname.Target == targetFQDN {
				return true
			}
		}
	}
	return false
}

// HasARecord checks if the response contains an A record with the given IP.
func (r *QueryResult) HasARecord(ip string) bool {
	if r.Response == nil {
		return false
	}
	targetIP := net.ParseIP(ip)
	for _, ans := range r.Response.Answer {
		if a, ok := ans.(*dns.A); ok {
			if a.A.Equal(targetIP) {
				return true
			}
		}
	}
	return false
}

// GetFirstCNAME returns the first CNAME target from the response.
func (r *QueryResult) GetFirstCNAME() string {
	if r.Response == nil {
		return ""
	}
	for _, ans := range r.Response.Answer {
		if cname, ok := ans.(*dns.CNAME); ok {
			return cname.Target
		}
	}
	return ""
}

// GetFirstARecord returns the first A record IP from the response.
func (r *QueryResult) GetFirstARecord() string {
	if r.Response == nil {
		return ""
	}
	for _, ans := range r.Response.Answer {
		if a, ok := ans.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

// IsSuccess returns true if the query was successful (NOERROR).
func (r *QueryResult) IsSuccess() bool {
	return r.Error == nil && r.Response != nil && r.Response.Rcode == dns.RcodeSuccess
}

// IsNXDomain returns true if the response is NXDOMAIN.
func (r *QueryResult) IsNXDomain() bool {
	return r.Error == nil && r.Response != nil && r.Response.Rcode == dns.RcodeNameError
}

// String returns a string representation of the result.
func (r *QueryResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("Error: %v", r.Error)
	}
	if r.Response == nil {
		return "No response"
	}
	return r.Response.String()
}
