package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steigr/nameserver-switcher/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCounter is used to generate unique ports for each test
var testCounter uint64

// testMutex ensures tests that create apps run sequentially
// to avoid Prometheus metric registration conflicts
var testMutex sync.Mutex

// getTestPorts returns available ports for testing.
func getTestPorts(t *testing.T) (dnsPort, grpcPort, httpPort int) {
	counter := atomic.AddUint64(&testCounter, 1)
	basePort := 15000 + int(counter)*10 + (time.Now().Nanosecond() % 100)
	return basePort, basePort + 1, basePort + 2
}

// getTestConfig returns a test configuration with unique ports.
func getTestConfig(t *testing.T) *config.Config {
	dnsPort, grpcPort, httpPort := getTestPorts(t)
	return &config.Config{
		RequestPatterns:  []string{},
		CNAMEPatterns:    []string{},
		RequestResolver:  "",
		ExplicitResolver: "",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          dnsPort,
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         grpcPort,
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         httpPort,
	}
}

// TestNewApp tests NewApp with various configurations.
// All NewApp tests are combined to avoid Prometheus metric registration conflicts.
func TestNewApp(t *testing.T) {
	testMutex.Lock()
	defer testMutex.Unlock()

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{`.*\.example\.com$`}
		cfg.CNAMEPatterns = []string{`.*\.cdn\.com$`}
		cfg.RequestResolver = "8.8.8.8:53"
		cfg.ExplicitResolver = "1.1.1.1:53"

		app, err := NewApp(cfg)
		require.NoError(t, err)
		assert.NotNil(t, app)
		assert.NotNil(t, app.Config)
		assert.NotNil(t, app.HealthChecker)
		assert.NotNil(t, app.Metrics)
		assert.NotNil(t, app.Router)
		assert.NotNil(t, app.DNSServer)
		assert.NotNil(t, app.GRPCServer)
		assert.NotNil(t, app.HTTPServer)

		// Verify config values
		assert.Equal(t, "8.8.8.8:53", app.Config.RequestResolver)
		assert.Equal(t, "1.1.1.1:53", app.Config.ExplicitResolver)
		assert.Equal(t, cfg.DNSPort, app.Config.DNSPort)
		assert.Equal(t, cfg.GRPCPort, app.Config.GRPCPort)
		assert.Equal(t, cfg.HTTPPort, app.Config.HTTPPort)
	})

	t.Run("InvalidRequestPattern", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{`[invalid`}

		app, err := NewApp(cfg)
		assert.Error(t, err)
		assert.Nil(t, app)
		assert.Contains(t, err.Error(), "request matcher")
	})

	t.Run("InvalidCNAMEPattern", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{`.*\.example\.com$`}
		cfg.CNAMEPatterns = []string{`[invalid`}

		app, err := NewApp(cfg)
		assert.Error(t, err)
		assert.Nil(t, app)
		assert.Contains(t, err.Error(), "CNAME matcher")
	})

	t.Run("WithoutResolvers", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{`.*\.example\.com$`}
		cfg.CNAMEPatterns = []string{`.*\.cdn\.com$`}

		app, err := NewApp(cfg)
		require.NoError(t, err)
		assert.NotNil(t, app)
	})

	t.Run("EmptyPatterns", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{}
		cfg.CNAMEPatterns = []string{}

		app, err := NewApp(cfg)
		require.NoError(t, err)
		assert.NotNil(t, app)
	})

	t.Run("MultiplePatterns", func(t *testing.T) {
		cfg := getTestConfig(t)
		cfg.RequestPatterns = []string{
			`.*\.example\.com$`,
			`.*\.test\.org$`,
			`.*\.sample\.net$`,
		}
		cfg.CNAMEPatterns = []string{
			`.*\.cdn\.com$`,
			`.*\.edge\.net$`,
		}

		app, err := NewApp(cfg)
		require.NoError(t, err)
		assert.NotNil(t, app)
		assert.Len(t, app.Config.RequestPatterns, 3)
		assert.Len(t, app.Config.CNAMEPatterns, 2)
	})
}

// TestApp_StartAndShutdown tests the Start and Shutdown methods.
func TestApp_StartAndShutdown(t *testing.T) {
	testMutex.Lock()
	defer testMutex.Unlock()

	cfg := getTestConfig(t)
	cfg.RequestPatterns = []string{`.*\.example\.com$`}
	cfg.CNAMEPatterns = []string{`.*\.cdn\.com$`}
	cfg.RequestResolver = "8.8.8.8:53"
	cfg.ExplicitResolver = "1.1.1.1:53"

	app, err := NewApp(cfg)
	require.NoError(t, err)

	// Initially not ready
	assert.False(t, app.HealthChecker.IsReady())

	// Start the application
	err = app.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify health checker is ready after start
	assert.True(t, app.HealthChecker.IsReady())

	// Shutdown the application
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.Shutdown(ctx)
	assert.NoError(t, err)

	// Verify health checker is not ready after shutdown
	assert.False(t, app.HealthChecker.IsReady())
}

// TestApp_HTTPEndpoints tests the HTTP endpoints.
func TestApp_HTTPEndpoints(t *testing.T) {
	testMutex.Lock()
	defer testMutex.Unlock()

	cfg := getTestConfig(t)
	cfg.RequestPatterns = []string{`.*\.example\.com$`}
	cfg.CNAMEPatterns = []string{`.*\.cdn\.com$`}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.HTTPPort)

	t.Run("healthz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/healthz")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("readyz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/readyz")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("livez", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/livez")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("metrics", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "go_")
	})
}

// TestApp_StartWithPortConflict tests that starting with conflicting ports fails.
func TestApp_StartWithPortConflict(t *testing.T) {
	testMutex.Lock()
	defer testMutex.Unlock()

	cfg1 := getTestConfig(t)

	app1, err := NewApp(cfg1)
	require.NoError(t, err)

	err = app1.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app1.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Second app tries to use same ports - should fail
	cfg2 := &config.Config{
		RequestPatterns: []string{},
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         cfg1.DNSPort,
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        cfg1.GRPCPort,
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        cfg1.HTTPPort,
	}

	app2, err := NewApp(cfg2)
	require.NoError(t, err)

	err = app2.Start()
	assert.Error(t, err, "Should fail due to port conflict")
}
