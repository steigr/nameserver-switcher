// Package grpc provides the gRPC server implementation for nameserver-switcher.
package grpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/steigr/nameserver-switcher/internal/config"
	"github.com/steigr/nameserver-switcher/internal/matcher"
	"github.com/steigr/nameserver-switcher/internal/metrics"
	"github.com/steigr/nameserver-switcher/internal/resolver"
	coredns "github.com/steigr/nameserver-switcher/pkg/api/coredns"
	pb "github.com/steigr/nameserver-switcher/pkg/api/v1"
)

// Server implements the gRPC NameserverSwitcherService and CoreDNS DnsService.
type Server struct {
	pb.UnimplementedNameserverSwitcherServiceServer
	coredns.UnimplementedDnsServiceServer
	router           *resolver.Router
	cfg              *config.Config
	metrics          *metrics.Metrics
	requestMatcher   *matcher.RegexMatcher
	cnameMatcher     *matcher.RegexMatcher
	grpcServer       *grpc.Server
	startTime        time.Time
	totalRequests    uint64
	requestResolver  string
	explicitResolver string
	addr             string
	port             int
}

// ServerConfig holds configuration for the gRPC server.
type ServerConfig struct {
	Addr             string
	Port             int
	Router           *resolver.Router
	Config           *config.Config
	Metrics          *metrics.Metrics
	RequestMatcher   *matcher.RegexMatcher
	CNAMEMatcher     *matcher.RegexMatcher
	RequestResolver  string
	ExplicitResolver string
}

// NewServer creates a new gRPC server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		router:           cfg.Router,
		cfg:              cfg.Config,
		metrics:          cfg.Metrics,
		requestMatcher:   cfg.RequestMatcher,
		cnameMatcher:     cfg.CNAMEMatcher,
		startTime:        time.Now(),
		requestResolver:  cfg.RequestResolver,
		explicitResolver: cfg.ExplicitResolver,
		addr:             cfg.Addr,
		port:             cfg.Port,
	}

	s.grpcServer = grpc.NewServer()
	pb.RegisterNameserverSwitcherServiceServer(s.grpcServer, s)
	coredns.RegisterDnsServiceServer(s.grpcServer, s)
	reflection.Register(s.grpcServer)

	return s
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	listenAddr := fmt.Sprintf("%s:%d", s.addr, s.port)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Printf("Starting gRPC server on %s", listenAddr)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the gRPC server.
func (s *Server) Shutdown(ctx context.Context) error {
	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		s.grpcServer.Stop()
		return ctx.Err()
	case <-stopped:
		return nil
	}
}

// Resolve implements the Resolve RPC method.
func (s *Server) Resolve(ctx context.Context, req *pb.ResolveRequest) (*pb.ResolveResponse, error) {
	atomic.AddUint64(&s.totalRequests, 1)

	if s.metrics != nil {
		s.metrics.RecordRequest("grpc", req.Type)
	}

	// Parse the query type
	qtype, ok := dns.StringToType[strings.ToUpper(req.Type)]
	if !ok {
		qtype = dns.TypeA
	}

	// Create DNS message
	dnsReq := &dns.Msg{}
	dnsReq.SetQuestion(dns.Fqdn(req.Name), qtype)

	// Route the request
	result, err := s.router.Route(ctx, dnsReq)
	if err != nil {
		if s.metrics != nil {
			s.metrics.RecordError("routing")
		}
		return nil, fmt.Errorf("resolution failed: %w", err)
	}

	// Build response
	resp := &pb.ResolveResponse{
		ResolverUsed:   result.ResolverUsed,
		RequestMatched: result.RequestMatched,
		CnameMatched:   result.CNAMEMatched,
		MatchedPattern: result.MatchedPattern,
		CnamePattern:   result.CNAMEPattern,
		Rcode:          dns.RcodeToString[result.Response.Rcode],
	}

	// Convert DNS records
	for _, rr := range result.Response.Answer {
		record := &pb.DNSRecord{
			Name: rr.Header().Name,
			Type: dns.TypeToString[rr.Header().Rrtype],
			Ttl:  rr.Header().Ttl,
		}

		switch r := rr.(type) {
		case *dns.A:
			record.Value = r.A.String()
		case *dns.AAAA:
			record.Value = r.AAAA.String()
		case *dns.CNAME:
			record.Value = r.Target
		case *dns.MX:
			record.Value = fmt.Sprintf("%d %s", r.Preference, r.Mx)
		case *dns.TXT:
			record.Value = strings.Join(r.Txt, " ")
		case *dns.NS:
			record.Value = r.Ns
		case *dns.PTR:
			record.Value = r.Ptr
		default:
			record.Value = rr.String()
		}

		resp.Records = append(resp.Records, record)
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordResolverUsed(result.ResolverUsed)
		if result.RequestMatched {
			s.metrics.RecordPatternMatch(result.MatchedPattern)
		}
		if result.CNAMEMatched {
			s.metrics.RecordCNAMEMatch(result.CNAMEPattern)
		}
		s.metrics.RecordResponseCode(resp.Rcode)
	}

	return resp, nil
}

// GetConfig implements the GetConfig RPC method.
func (s *Server) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	resp := &pb.GetConfigResponse{
		RequestResolver:  s.requestResolver,
		ExplicitResolver: s.explicitResolver,
	}

	if s.requestMatcher != nil {
		resp.RequestPatterns = s.requestMatcher.Patterns()
	}

	if s.cnameMatcher != nil {
		resp.CnamePatterns = s.cnameMatcher.Patterns()
	}

	return resp, nil
}

// UpdateRequestPatterns implements the UpdateRequestPatterns RPC method.
func (s *Server) UpdateRequestPatterns(ctx context.Context, req *pb.UpdatePatternsRequest) (*pb.UpdatePatternsResponse, error) {
	if s.requestMatcher == nil {
		return &pb.UpdatePatternsResponse{
			Success: false,
			Error:   "request matcher not configured",
		}, nil
	}

	if err := s.requestMatcher.UpdatePatterns(req.Patterns); err != nil {
		return &pb.UpdatePatternsResponse{
			Success:  false,
			Error:    err.Error(),
			Patterns: s.requestMatcher.Patterns(),
		}, nil
	}

	return &pb.UpdatePatternsResponse{
		Success:  true,
		Patterns: s.requestMatcher.Patterns(),
	}, nil
}

// UpdateCNAMEPatterns implements the UpdateCNAMEPatterns RPC method.
func (s *Server) UpdateCNAMEPatterns(ctx context.Context, req *pb.UpdatePatternsRequest) (*pb.UpdatePatternsResponse, error) {
	if s.cnameMatcher == nil {
		return &pb.UpdatePatternsResponse{
			Success: false,
			Error:   "CNAME matcher not configured",
		}, nil
	}

	if err := s.cnameMatcher.UpdatePatterns(req.Patterns); err != nil {
		return &pb.UpdatePatternsResponse{
			Success:  false,
			Error:    err.Error(),
			Patterns: s.cnameMatcher.Patterns(),
		}, nil
	}

	return &pb.UpdatePatternsResponse{
		Success:  true,
		Patterns: s.cnameMatcher.Patterns(),
	}, nil
}

// GetStats implements the GetStats RPC method.
func (s *Server) GetStats(ctx context.Context, req *pb.GetStatsRequest) (*pb.GetStatsResponse, error) {
	uptime := time.Since(s.startTime)

	return &pb.GetStatsResponse{
		TotalRequests:      atomic.LoadUint64(&s.totalRequests),
		UptimeSeconds:      uint64(uptime.Seconds()),
		RequestsByResolver: make(map[string]uint64),
		PatternMatches:     make(map[string]uint64),
		CnameMatches:       make(map[string]uint64),
	}, nil
}

// Addr returns the listen address.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.addr, s.port)
}

// Query implements the CoreDNS DnsService Query RPC method.
// This allows CoreDNS to use the grpc plugin to forward DNS queries.
func (s *Server) Query(ctx context.Context, req *coredns.DnsPacket) (*coredns.DnsPacket, error) {
	// Parse the incoming DNS message
	msg := new(dns.Msg)
	if err := msg.Unpack(req.Msg); err != nil {
		return nil, fmt.Errorf("failed to unpack DNS message: %w", err)
	}

	// Increment request counter
	atomic.AddUint64(&s.totalRequests, 1)

	// Record metrics if available
	if s.metrics != nil && len(msg.Question) > 0 {
		qtype := dns.TypeToString[msg.Question[0].Qtype]
		s.metrics.RecordRequest("grpc-coredns", qtype)
	}

	// Use the router to resolve the query
	if s.router != nil && len(msg.Question) > 0 {
		result, err := s.router.Route(ctx, msg)
		if err != nil {
			// Return SERVFAIL on error
			reply := new(dns.Msg)
			reply.SetRcode(msg, dns.RcodeServerFailure)
			packed, _ := reply.Pack()
			return &coredns.DnsPacket{Msg: packed}, nil
		}

		// Pack and return the response
		if result.Response != nil {
			// Ensure the response has the same ID as the request
			result.Response.Id = msg.Id
			packed, err := result.Response.Pack()
			if err != nil {
				return nil, fmt.Errorf("failed to pack DNS response: %w", err)
			}
			return &coredns.DnsPacket{Msg: packed}, nil
		}
	}

	// If no router or no result, return SERVFAIL
	reply := new(dns.Msg)
	reply.SetRcode(msg, dns.RcodeServerFailure)
	packed, _ := reply.Pack()
	return &coredns.DnsPacket{Msg: packed}, nil
}
