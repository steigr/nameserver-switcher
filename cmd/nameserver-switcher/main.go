// Package main provides the entry point for the nameserver-switcher.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/steigr/nameserver-switcher/internal/config"
	dnsserver "github.com/steigr/nameserver-switcher/internal/dns"
	grpcserver "github.com/steigr/nameserver-switcher/internal/grpc"
	"github.com/steigr/nameserver-switcher/internal/health"
	"github.com/steigr/nameserver-switcher/internal/matcher"
	"github.com/steigr/nameserver-switcher/internal/metrics"
	"github.com/steigr/nameserver-switcher/internal/resolver"
)

// App holds all the application components.
type App struct {
	Config        *config.Config
	HealthChecker *health.Checker
	Metrics       *metrics.Metrics
	Router        *resolver.Router
	DNSServer     *dnsserver.Server
	GRPCServer    *grpcserver.Server
	HTTPServer    *http.Server
}

// NewApp creates a new application instance with the given configuration.
func NewApp(cfg *config.Config) (*App, error) {
	// Create health checker
	healthChecker := health.NewChecker()

	// Create metrics
	m := metrics.NewMetrics("nameserver_switcher")

	// Create matchers
	requestMatcher, err := matcher.NewRegexMatcher(cfg.RequestPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create request matcher: %w", err)
	}

	cnameMatcher, err := matcher.NewRegexMatcher(cfg.CNAMEPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create CNAME matcher: %w", err)
	}

	// Create resolvers
	var requestResolver resolver.Resolver
	if cfg.RequestResolver != "" {
		requestResolver = resolver.NewDNSResolver(cfg.RequestResolver, false, "request")
		log.Printf("Using request resolver: %s", cfg.RequestResolver)
	}

	var explicitResolver resolver.Resolver
	if cfg.ExplicitResolver != "" {
		explicitResolver = resolver.NewDNSResolver(cfg.ExplicitResolver, true, "explicit")
		log.Printf("Using explicit resolver: %s", cfg.ExplicitResolver)
	}

	systemResolver, err := resolver.NewSystemResolver()
	if err != nil {
		log.Printf("Warning: Failed to create system resolver, using fallback: %v", err)
		systemResolver = resolver.NewSystemResolverWithServers([]string{"8.8.8.8:53", "8.8.4.4:53"})
	}
	log.Printf("Using system resolvers: %v", systemResolver.Servers())

	// Create router
	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:   requestMatcher,
		CNAMEMatcher:     cnameMatcher,
		RequestResolver:  requestResolver,
		ExplicitResolver: explicitResolver,
		SystemResolver:   systemResolver,
	})

	// Create DNS server
	dnsServer := dnsserver.NewServer(dnsserver.ServerConfig{
		Addr:    cfg.DNSListenAddr,
		Port:    cfg.DNSPort,
		Router:  router,
		Metrics: m,
		Config:  cfg,
	})

	// Create gRPC server
	grpcServer := grpcserver.NewServer(grpcserver.ServerConfig{
		Addr:             cfg.GRPCListenAddr,
		Port:             cfg.GRPCPort,
		Router:           router,
		Config:           cfg,
		Metrics:          m,
		RequestMatcher:   requestMatcher,
		CNAMEMatcher:     cnameMatcher,
		RequestResolver:  cfg.RequestResolver,
		ExplicitResolver: cfg.ExplicitResolver,
	})

	// Create HTTP server for health and metrics
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/healthz", healthChecker.HealthHandler())
	httpMux.HandleFunc("/readyz", healthChecker.ReadyHandler())
	httpMux.HandleFunc("/livez", healthChecker.LiveHandler())
	httpMux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.HTTPListenAddr, cfg.HTTPPort),
		Handler: httpMux,
	}

	return &App{
		Config:        cfg,
		HealthChecker: healthChecker,
		Metrics:       m,
		Router:        router,
		DNSServer:     dnsServer,
		GRPCServer:    grpcServer,
		HTTPServer:    httpServer,
	}, nil
}

// Start starts all the application servers.
func (a *App) Start() error {
	log.Println("Starting nameserver-switcher...")

	// Start DNS server
	if err := a.DNSServer.Start(); err != nil {
		return fmt.Errorf("failed to start DNS server: %w", err)
	}
	log.Printf("DNS server listening on %s", a.DNSServer.Addr())

	// Start gRPC server
	if err := a.GRPCServer.Start(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	log.Printf("gRPC server listening on %s", a.GRPCServer.Addr())

	// Start HTTP server
	go func() {
		log.Printf("HTTP server listening on %s", a.HTTPServer.Addr)
		if err := a.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Mark as ready
	a.HealthChecker.SetReady(true)
	log.Println("Server is ready")

	// Log configuration summary
	log.Printf("Request patterns: %d configured", len(a.Config.RequestPatterns))
	log.Printf("CNAME patterns: %d configured", len(a.Config.CNAMEPatterns))

	return nil
}

// Shutdown gracefully shuts down all servers.
func (a *App) Shutdown(ctx context.Context) error {
	log.Println("Shutting down...")

	// Mark as not ready
	a.HealthChecker.SetReady(false)

	var shutdownErr error

	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
		shutdownErr = err
	}

	if err := a.GRPCServer.Shutdown(ctx); err != nil {
		log.Printf("gRPC server shutdown error: %v", err)
		shutdownErr = err
	}

	if err := a.DNSServer.Shutdown(ctx); err != nil {
		log.Printf("DNS server shutdown error: %v", err)
		shutdownErr = err
	}

	if shutdownErr != nil {
		return fmt.Errorf("shutdown completed with errors: %w", shutdownErr)
	}

	log.Println("Shutdown completed successfully")
	return nil
}

// Run starts the application and waits for a shutdown signal.
func (a *App) Run() error {
	if err := a.Start(); err != nil {
		return err
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Printf("Received signal %v, shutting down...", sig)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.Shutdown(ctx)
}

func main() {
	// Load configuration
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()
	cfg.ParseFlags()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	app, err := NewApp(cfg)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
