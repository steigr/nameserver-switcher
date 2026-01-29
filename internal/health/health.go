// Package health provides health check and readiness endpoints.
package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker provides health and readiness status.
type Checker struct {
	mu        sync.RWMutex
	ready     bool
	healthy   bool
	startTime time.Time
	checks    map[string]CheckFunc
}

// CheckFunc is a function that performs a health check.
type CheckFunc func() error

// Status represents the health status response.
type Status struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Uptime    string            `json:"uptime"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{
		healthy:   true,
		ready:     false,
		startTime: time.Now(),
		checks:    make(map[string]CheckFunc),
	}
}

// SetReady sets the readiness status.
func (c *Checker) SetReady(ready bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = ready
}

// SetHealthy sets the health status.
func (c *Checker) SetHealthy(healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthy = healthy
}

// IsReady returns the readiness status.
func (c *Checker) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// IsHealthy returns the health status.
func (c *Checker) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// AddCheck adds a named health check function.
func (c *Checker) AddCheck(name string, check CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
}

// RunChecks runs all health checks and returns the results.
func (c *Checker) RunChecks() map[string]error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make(map[string]error)
	for name, check := range c.checks {
		results[name] = check()
	}
	return results
}

// HealthHandler returns an HTTP handler for the health endpoint.
func (c *Checker) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		healthy := c.healthy
		c.mu.RUnlock()

		checkResults := c.RunChecks()
		checksStatus := make(map[string]string)
		allHealthy := healthy

		for name, err := range checkResults {
			if err != nil {
				checksStatus[name] = err.Error()
				allHealthy = false
			} else {
				checksStatus[name] = "ok"
			}
		}

		status := Status{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    time.Since(c.startTime).String(),
			Checks:    checksStatus,
		}

		if allHealthy {
			status.Status = "healthy"
			w.WriteHeader(http.StatusOK)
		} else {
			status.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

// ReadyHandler returns an HTTP handler for the readiness endpoint.
func (c *Checker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		ready := c.ready
		c.mu.RUnlock()

		status := Status{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    time.Since(c.startTime).String(),
		}

		if ready {
			status.Status = "ready"
			w.WriteHeader(http.StatusOK)
		} else {
			status.Status = "not ready"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

// LiveHandler returns an HTTP handler for the liveness endpoint.
func (c *Checker) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := Status{
			Status:    "alive",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    time.Since(c.startTime).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}
