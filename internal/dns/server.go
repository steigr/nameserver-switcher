// Package dns provides the DNS server implementation.
package dns

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/miekg/dns"

	"github.com/steigr/nameserver-switcher/internal/metrics"
	"github.com/steigr/nameserver-switcher/internal/resolver"
)

// Server is a DNS server that routes requests through the resolver router.
type Server struct {
	udpServer *dns.Server
	tcpServer *dns.Server
	router    *resolver.Router
	metrics   *metrics.Metrics
	addr      string
	port      int
}

// ServerConfig holds configuration for the DNS server.
type ServerConfig struct {
	Addr    string
	Port    int
	Router  *resolver.Router
	Metrics *metrics.Metrics
}

// NewServer creates a new DNS server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		router:  cfg.Router,
		metrics: cfg.Metrics,
		addr:    cfg.Addr,
		port:    cfg.Port,
	}

	handler := dns.HandlerFunc(s.handleRequest)

	listenAddr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)

	s.udpServer = &dns.Server{
		Addr:    listenAddr,
		Net:     "udp",
		Handler: handler,
	}

	s.tcpServer = &dns.Server{
		Addr:    listenAddr,
		Net:     "tcp",
		Handler: handler,
	}

	return s
}

// Start starts the DNS server (UDP and TCP).
func (s *Server) Start() error {
	errCh := make(chan error, 2)

	go func() {
		log.Printf("Starting DNS server (UDP) on %s:%d", s.addr, s.port)
		if err := s.udpServer.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("UDP server failed: %w", err)
		}
	}()

	go func() {
		log.Printf("Starting DNS server (TCP) on %s:%d", s.addr, s.port)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("TCP server failed: %w", err)
		}
	}()

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	// Check if either server failed immediately
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// Shutdown gracefully shuts down the DNS server.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	if err := s.udpServer.ShutdownContext(ctx); err != nil {
		errs = append(errs, fmt.Errorf("UDP shutdown failed: %w", err))
	}

	if err := s.tcpServer.ShutdownContext(ctx); err != nil {
		errs = append(errs, fmt.Errorf("TCP shutdown failed: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

// handleRequest handles incoming DNS requests.
func (s *Server) handleRequest(w dns.ResponseWriter, req *dns.Msg) {
	start := time.Now()

	// Determine protocol
	protocol := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		protocol = "tcp"
	}

	// Get query type
	qtype := "unknown"
	if len(req.Question) > 0 {
		qtype = dns.TypeToString[req.Question[0].Qtype]
	}

	if s.metrics != nil {
		s.metrics.RecordRequest(protocol, qtype)
		s.metrics.IncActiveConnections()
		defer s.metrics.DecActiveConnections()
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Route the request
	result, err := s.router.Route(ctx, req)
	if err != nil {
		log.Printf("Error routing request: %v", err)
		if s.metrics != nil {
			s.metrics.RecordError("routing")
		}

		// Send SERVFAIL response
		resp := &dns.Msg{}
		resp.SetRcode(req, dns.RcodeServerFailure)
		w.WriteMsg(resp)

		if s.metrics != nil {
			s.metrics.RecordResponseCode("SERVFAIL")
		}
		return
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	if s.metrics != nil {
		s.metrics.RecordDuration(result.ResolverUsed, duration)
		s.metrics.RecordResolverUsed(result.ResolverUsed)

		if result.RequestMatched {
			s.metrics.RecordPatternMatch(result.MatchedPattern)
		}
		if result.CNAMEMatched {
			s.metrics.RecordCNAMEMatch(result.CNAMEPattern)
		}

		rcode := dns.RcodeToString[result.Response.Rcode]
		s.metrics.RecordResponseCode(rcode)
	}

	// Set response ID to match request
	result.Response.Id = req.Id

	// Write response
	if err := w.WriteMsg(result.Response); err != nil {
		log.Printf("Error writing response: %v", err)
		if s.metrics != nil {
			s.metrics.RecordError("write")
		}
	}
}

// Addr returns the listen address.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.addr, s.port)
}

// Query performs a DNS query through the router (for testing).
func (s *Server) Query(ctx context.Context, req *dns.Msg) (*resolver.RouteResult, error) {
	return s.router.Route(ctx, req)
}
