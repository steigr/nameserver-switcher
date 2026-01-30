// Package main provides the entry point for the nameserver-switcher.
package main

import (
	"context"
	"fmt"
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
	"github.com/steigr/nameserver-switcher/internal/logging"
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
	// Initialize logger
	logFormat := logging.FormatText
	if cfg.LogFormat == "json" {
		logFormat = logging.FormatJSON
	}
	logger := logging.NewLogger(logging.Config{
		Format: logFormat,
		Debug:  cfg.Debug,
	})
	logging.SetDefault(logger)

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
	var explicitResolver resolver.Resolver
	if cfg.ExplicitResolver != "" {
		explicitResolver = resolver.NewDNSResolver(cfg.ExplicitResolver, true, "explicit")
		logging.Infof("Using explicit resolver: %s", cfg.ExplicitResolver)
	}

	// System resolver: use REQUEST_RESOLVER if configured, otherwise use system /etc/resolv.conf
	var systemResolver resolver.Resolver
	if cfg.RequestResolver != "" {
		systemResolver = resolver.NewDNSResolver(cfg.RequestResolver, true, "system")
		logging.Infof("Using configured system resolver: %s", cfg.RequestResolver)
	} else {
		sysRes, err := resolver.NewSystemResolver()
		if err != nil {
			logging.Warnf("Failed to create system resolver, using fallback: %v", err)
			sysRes = resolver.NewSystemResolverWithServers([]string{"8.8.8.8:53", "8.8.4.4:53"})
		}
		systemResolver = sysRes
		logging.Infof("Using system resolvers: %v", sysRes.Servers())
	}

	// Create specialized fallback resolvers, defaulting to systemResolver if not configured
	var passthroughResolver resolver.Resolver
	if cfg.PassthroughResolver != "" {
		passthroughResolver = resolver.NewDNSResolver(cfg.PassthroughResolver, true, "passthrough")
		logging.Infof("Using passthrough resolver: %s", cfg.PassthroughResolver)
	} else {
		passthroughResolver = systemResolver
		logging.Info("Passthrough resolver not configured, using system resolver")
	}

	var noCnameResponseResolver resolver.Resolver
	if cfg.NoCnameResponseResolver != "" {
		noCnameResponseResolver = resolver.NewDNSResolver(cfg.NoCnameResponseResolver, true, "no-cname-response")
		logging.Infof("Using no-cname-response resolver: %s", cfg.NoCnameResponseResolver)
	} else {
		noCnameResponseResolver = systemResolver
		logging.Info("No-cname-response resolver not configured, using system resolver")
	}

	var noCnameMatchResolver resolver.Resolver
	if cfg.NoCnameMatchResolver != "" {
		noCnameMatchResolver = resolver.NewDNSResolver(cfg.NoCnameMatchResolver, true, "no-cname-match")
		logging.Infof("Using no-cname-match resolver: %s", cfg.NoCnameMatchResolver)
	} else {
		noCnameMatchResolver = systemResolver
		logging.Info("No-cname-match resolver not configured, using system resolver")
	}

	// Create router
	router := resolver.NewRouter(resolver.RouterConfig{
		RequestMatcher:          requestMatcher,
		CNAMEMatcher:            cnameMatcher,
		ExplicitResolver:        explicitResolver,
		PassthroughResolver:     passthroughResolver,
		NoCnameResponseResolver: noCnameResponseResolver,
		NoCnameMatchResolver:    noCnameMatchResolver,
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
	logging.Info("Starting nameserver-switcher...")

	// Start DNS server
	if err := a.DNSServer.Start(); err != nil {
		return fmt.Errorf("failed to start DNS server: %w", err)
	}
	logging.Infof("DNS server listening on %s", a.DNSServer.Addr())

	// Start gRPC server
	if err := a.GRPCServer.Start(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	logging.Infof("gRPC server listening on %s", a.GRPCServer.Addr())

	// Start HTTP server
	go func() {
		logging.Infof("HTTP server listening on %s", a.HTTPServer.Addr)
		if err := a.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Errorf("HTTP server error: %v", err)
		}
	}()

	// Mark as ready
	a.HealthChecker.SetReady(true)
	logging.Info("Server is ready")

	// Log configuration summary
	logging.Infof("Request patterns: %d configured", len(a.Config.RequestPatterns))
	logging.Infof("CNAME patterns: %d configured", len(a.Config.CNAMEPatterns))

	return nil
}

// Shutdown gracefully shuts down all servers.
func (a *App) Shutdown(ctx context.Context) error {
	logging.Info("Shutting down...")

	// Mark as not ready
	a.HealthChecker.SetReady(false)

	var shutdownErr error

	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		logging.Errorf("HTTP server shutdown error: %v", err)
		shutdownErr = err
	}

	if err := a.GRPCServer.Shutdown(ctx); err != nil {
		logging.Errorf("gRPC server shutdown error: %v", err)
		shutdownErr = err
	}

	if err := a.DNSServer.Shutdown(ctx); err != nil {
		logging.Errorf("DNS server shutdown error: %v", err)
		shutdownErr = err
	}

	if shutdownErr != nil {
		return fmt.Errorf("shutdown completed with errors: %w", shutdownErr)
	}

	logging.Info("Shutdown completed successfully")
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

	logging.Infof("Received signal %v, shutting down...", sig)

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
		// Use fmt for fatal errors before logger is initialized
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	app, err := NewApp(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create application: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(); err != nil {
		logging.Errorf("Application error: %v", err)
		os.Exit(1)
	}
}
