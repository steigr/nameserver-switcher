package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChecker(t *testing.T) {
	c := NewChecker()

	assert.NotNil(t, c)
	assert.True(t, c.IsHealthy())
	assert.False(t, c.IsReady())
}

func TestChecker_SetReady(t *testing.T) {
	c := NewChecker()

	assert.False(t, c.IsReady())

	c.SetReady(true)
	assert.True(t, c.IsReady())

	c.SetReady(false)
	assert.False(t, c.IsReady())
}

func TestChecker_SetHealthy(t *testing.T) {
	c := NewChecker()

	assert.True(t, c.IsHealthy())

	c.SetHealthy(false)
	assert.False(t, c.IsHealthy())

	c.SetHealthy(true)
	assert.True(t, c.IsHealthy())
}

func TestChecker_AddCheck(t *testing.T) {
	c := NewChecker()

	c.AddCheck("test", func() error {
		return nil
	})

	results := c.RunChecks()
	assert.Len(t, results, 1)
	assert.NoError(t, results["test"])
}

func TestChecker_RunChecks(t *testing.T) {
	c := NewChecker()

	c.AddCheck("passing", func() error {
		return nil
	})
	c.AddCheck("failing", func() error {
		return errors.New("check failed")
	})

	results := c.RunChecks()
	assert.Len(t, results, 2)
	assert.NoError(t, results["passing"])
	assert.Error(t, results["failing"])
}

func TestChecker_HealthHandler_Healthy(t *testing.T) {
	c := NewChecker()
	c.SetHealthy(true)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	c.HealthHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "healthy", status.Status)
}

func TestChecker_HealthHandler_Unhealthy(t *testing.T) {
	c := NewChecker()
	c.SetHealthy(false)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	c.HealthHandler()(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", status.Status)
}

func TestChecker_HealthHandler_WithFailingCheck(t *testing.T) {
	c := NewChecker()
	c.SetHealthy(true)
	c.AddCheck("failing", func() error {
		return errors.New("something went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	c.HealthHandler()(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", status.Status)
	assert.Equal(t, "something went wrong", status.Checks["failing"])
}

func TestChecker_HealthHandler_WithPassingCheck(t *testing.T) {
	c := NewChecker()
	c.SetHealthy(true)
	c.AddCheck("passing", func() error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	c.HealthHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "healthy", status.Status)
	assert.Equal(t, "ok", status.Checks["passing"])
}

func TestChecker_ReadyHandler_Ready(t *testing.T) {
	c := NewChecker()
	c.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	c.ReadyHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "ready", status.Status)
}

func TestChecker_ReadyHandler_NotReady(t *testing.T) {
	c := NewChecker()
	c.SetReady(false)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	c.ReadyHandler()(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "not ready", status.Status)
}

func TestChecker_LiveHandler(t *testing.T) {
	c := NewChecker()

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	c.LiveHandler()(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status Status
	err := json.NewDecoder(rec.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "alive", status.Status)
}
