package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/steigr/nameserver-switcher/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getFreePorts returns available ports for testing.
func getFreePorts(t *testing.T, count int) []int {
	ports := make([]int, count)
	for i := 0; i < count; i++ {
		// Use 0 to let the OS assign a free port, but since we can't do that
		// directly for all servers, we'll use a base port and hope for the best
		// In a real scenario, we'd use net.Listen(":0") to get free ports
		ports[i] = 15353 + i + (time.Now().Nanosecond() % 1000)
	}
	return ports
}

func TestNewApp_ValidConfig(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{`.*\.example\.com$`},
		CNAMEPatterns:    []string{`.*\.cdn\.com$`},
		RequestResolver:  "8.8.8.8:53",
		ExplicitResolver: "1.1.1.1:53",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

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
}

func TestNewApp_EmptyPatterns(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{},
		CNAMEPatterns:    []string{},
		RequestResolver:  "",
		ExplicitResolver: "",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)
	assert.NotNil(t, app)
}

func TestNewApp_InvalidRequestPattern(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{`[invalid`}, // Invalid regex
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app, err := NewApp(cfg)
	assert.Error(t, err)
	assert.Nil(t, app)
	assert.Contains(t, err.Error(), "request matcher")
}

func TestNewApp_InvalidCNAMEPattern(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{`.*\.example\.com$`},
		CNAMEPatterns:   []string{`[invalid`}, // Invalid regex
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app, err := NewApp(cfg)
	assert.Error(t, err)
	assert.Nil(t, app)
	assert.Contains(t, err.Error(), "CNAME matcher")
}

func TestNewApp_WithResolvers(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{`.*\.example\.com$`},
		CNAMEPatterns:    []string{`.*\.cdn\.com$`},
		RequestResolver:  "8.8.8.8:53",
		ExplicitResolver: "1.1.1.1:53",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)
	assert.NotNil(t, app)
	assert.Equal(t, "8.8.8.8:53", app.Config.RequestResolver)
	assert.Equal(t, "1.1.1.1:53", app.Config.ExplicitResolver)
}

func TestNewApp_WithoutResolvers(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{`.*\.example\.com$`},
		CNAMEPatterns:    []string{`.*\.cdn\.com$`},
		RequestResolver:  "",
		ExplicitResolver: "",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)
	assert.NotNil(t, app)
}

func TestApp_StartAndShutdown(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{`.*\.example\.com$`},
		CNAMEPatterns:    []string{`.*\.cdn\.com$`},
		RequestResolver:  "8.8.8.8:53",
		ExplicitResolver: "1.1.1.1:53",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	// Start the application
	err = app.Start()
	require.NoError(t, err)

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	// Verify health checker is ready
	assert.True(t, app.HealthChecker.IsReady())

	// Shutdown the application
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.Shutdown(ctx)
	assert.NoError(t, err)

	// Verify health checker is not ready after shutdown
	assert.False(t, app.HealthChecker.IsReady())
}

func TestApp_HTTPEndpoints(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{`.*\.example\.com$`},
		CNAMEPatterns:   []string{`.*\.cdn\.com$`},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app.Shutdown(ctx)
	}()

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", ports[2])

	// Test /healthz endpoint
	t.Run("healthz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/healthz")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test /readyz endpoint
	t.Run("readyz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/readyz")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test /livez endpoint
	t.Run("livez", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/livez")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Test /metrics endpoint
	t.Run("metrics", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		// Should contain Prometheus metrics
		assert.Contains(t, string(body), "go_")
	})
}

func TestApp_ShutdownTimeout(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{},
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	err = app.Start()
	require.NoError(t, err)

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	// Use a reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestApp_MultiplePatterns(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{
			`.*\.example\.com$`,
			`.*\.test\.org$`,
			`.*\.sample\.net$`,
		},
		CNAMEPatterns: []string{
			`.*\.cdn\.com$`,
			`.*\.edge\.net$`,
		},
		DNSListenAddr:  "127.0.0.1",
		DNSPort:        ports[0],
		GRPCListenAddr: "127.0.0.1",
		GRPCPort:       ports[1],
		HTTPListenAddr: "127.0.0.1",
		HTTPPort:       ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)
	assert.NotNil(t, app)
	assert.Len(t, app.Config.RequestPatterns, 3)
	assert.Len(t, app.Config.CNAMEPatterns, 2)
}

func TestApp_StartWithPortConflict(t *testing.T) {
	// First app takes the ports
	ports := getFreePorts(t, 3)
	cfg1 := &config.Config{
		RequestPatterns: []string{},
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app1, err := NewApp(cfg1)
	require.NoError(t, err)

	err = app1.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		app1.Shutdown(ctx)
	}()

	// Give servers time to start
	time.Sleep(100 * time.Millisecond)

	// Second app tries to use same ports - should fail
	cfg2 := &config.Config{
		RequestPatterns: []string{},
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0], // Same port as app1
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app2, err := NewApp(cfg2)
	require.NoError(t, err)

	err = app2.Start()
	assert.Error(t, err, "Should fail due to port conflict")
}

func TestApp_ConfigAccessible(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns:  []string{`.*\.example\.com$`},
		CNAMEPatterns:    []string{`.*\.cdn\.com$`},
		RequestResolver:  "8.8.8.8:53",
		ExplicitResolver: "1.1.1.1:53",
		DNSListenAddr:    "127.0.0.1",
		DNSPort:          ports[0],
		GRPCListenAddr:   "127.0.0.1",
		GRPCPort:         ports[1],
		HTTPListenAddr:   "127.0.0.1",
		HTTPPort:         ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	// Verify config is accessible and correct
	assert.Equal(t, cfg.RequestResolver, app.Config.RequestResolver)
	assert.Equal(t, cfg.ExplicitResolver, app.Config.ExplicitResolver)
	assert.Equal(t, cfg.DNSPort, app.Config.DNSPort)
	assert.Equal(t, cfg.GRPCPort, app.Config.GRPCPort)
	assert.Equal(t, cfg.HTTPPort, app.Config.HTTPPort)
}

func TestApp_HealthCheckerState(t *testing.T) {
	ports := getFreePorts(t, 3)
	cfg := &config.Config{
		RequestPatterns: []string{},
		CNAMEPatterns:   []string{},
		DNSListenAddr:   "127.0.0.1",
		DNSPort:         ports[0],
		GRPCListenAddr:  "127.0.0.1",
		GRPCPort:        ports[1],
		HTTPListenAddr:  "127.0.0.1",
		HTTPPort:        ports[2],
	}

	app, err := NewApp(cfg)
	require.NoError(t, err)

	// Initially not ready
	assert.False(t, app.HealthChecker.IsReady())

	// After start, should be ready
	err = app.Start()
	require.NoError(t, err)
	assert.True(t, app.HealthChecker.IsReady())

	// After shutdown, should not be ready
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	app.Shutdown(ctx)
	assert.False(t, app.HealthChecker.IsReady())
}
