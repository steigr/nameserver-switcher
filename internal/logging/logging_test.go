package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		logger := NewLogger(Config{})
		assert.NotNil(t, logger)
	})

	t.Run("with output", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewLogger(Config{Output: buf})
		logger.Info("test")
		assert.Contains(t, buf.String(), "test")
	})

	t.Run("with JSON format", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewLogger(Config{Output: buf, Format: FormatJSON})
		logger.Info("test message")

		var entry LogEntry
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)
		assert.Equal(t, "test message", entry.Message)
		assert.Equal(t, LevelInfo, entry.Level)
		assert.Greater(t, entry.TimeMillis, int64(0))
	})
}

func TestLogger_Levels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(l *Logger, msg string)
		level    Level
		debug    bool
		expected bool
	}{
		{"debug enabled", func(l *Logger, msg string) { l.Debug(msg) }, LevelDebug, true, true},
		{"debug disabled", func(l *Logger, msg string) { l.Debug(msg) }, LevelDebug, false, false},
		{"info", func(l *Logger, msg string) { l.Info(msg) }, LevelInfo, false, true},
		{"warn", func(l *Logger, msg string) { l.Warn(msg) }, LevelWarn, false, true},
		{"error", func(l *Logger, msg string) { l.Error(msg) }, LevelError, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := NewLogger(Config{Output: buf, Debug: tt.debug})
			tt.logFunc(logger, "test message")

			if tt.expected {
				assert.Contains(t, buf.String(), "test message")
				assert.Contains(t, buf.String(), string(tt.level))
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: true})

	t.Run("contains timemillis", func(t *testing.T) {
		buf.Reset()
		logger.Info("test")

		var entry LogEntry
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		// Verify timemillis is present and reasonable
		now := time.Now().UnixMilli()
		assert.InDelta(t, now, entry.TimeMillis, 1000) // Within 1 second
	})

	t.Run("contains all fields", func(t *testing.T) {
		buf.Reset()
		logger.Info("test message", map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		})

		var entry LogEntry
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		assert.Equal(t, "test message", entry.Message)
		assert.Equal(t, LevelInfo, entry.Level)
		assert.NotEmpty(t, entry.Time)
		assert.Equal(t, "value1", entry.Fields["key1"])
		assert.Equal(t, float64(123), entry.Fields["key2"]) // JSON numbers are float64
	})

	t.Run("debug with fields", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message", map[string]interface{}{
			"pattern": ".*\\.example\\.com$",
		})

		var entry LogEntry
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		assert.Equal(t, LevelDebug, entry.Level)
		assert.Equal(t, ".*\\.example\\.com$", entry.Fields["pattern"])
	})
}

func TestLogger_TextFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatText})

	t.Run("basic message", func(t *testing.T) {
		buf.Reset()
		logger.Info("test message")

		output := buf.String()
		assert.Contains(t, output, "[INFO]")
		assert.Contains(t, output, "test message")
		// Should contain timestamp with milliseconds
		assert.Regexp(t, `\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3}`, output)
	})

	t.Run("with fields", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", map[string]interface{}{
			"key": "value",
		})

		output := buf.String()
		assert.Contains(t, output, "key=value")
	})

	t.Run("with component", func(t *testing.T) {
		buf.Reset()
		componentLogger := logger.WithComponent("dns")
		componentLogger.Info("test")

		output := buf.String()
		assert.Contains(t, output, "[dns]")
	})
}

func TestLogger_WithComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	componentLogger := logger.WithComponent("dns-server")
	componentLogger.Info("test")

	var entry LogEntry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "dns-server", entry.Component)
}

func TestLogger_FormattedFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Debug: true})

	tests := []struct {
		name    string
		logFunc func()
		message string
	}{
		{"Debugf", func() { logger.Debugf("debug %s %d", "test", 123) }, "debug test 123"},
		{"Infof", func() { logger.Infof("info %s", "message") }, "info message"},
		{"Warnf", func() { logger.Warnf("warn %d", 456) }, "warn 456"},
		{"Errorf", func() { logger.Errorf("error %v", "details") }, "error details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			assert.Contains(t, buf.String(), tt.message)
		})
	}
}

func TestLogger_DNSRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	logger.LogDNSRequest(DNSRequest{
		Protocol: "udp",
		Type:     "A",
		Name:     "example.com.",
		From:     "127.0.0.1:12345",
	})

	var entry LogEntry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DNS request received", entry.Message)
	assert.Equal(t, "udp", entry.Fields["protocol"])
	assert.Equal(t, "A", entry.Fields["type"])
	assert.Equal(t, "example.com.", entry.Fields["name"])
	assert.Equal(t, "127.0.0.1:12345", entry.Fields["from"])
}

func TestLogger_DNSResponse(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	logger.LogDNSResponse(DNSResponse{
		Name:           "example.com.",
		Rcode:          "NOERROR",
		AnswerCount:    2,
		Resolver:       "explicit",
		DurationMs:     12.345,
		RequestMatched: true,
		CNAMEMatched:   true,
	})

	var entry LogEntry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DNS response sent", entry.Message)
	assert.Equal(t, "example.com.", entry.Fields["name"])
	assert.Equal(t, "NOERROR", entry.Fields["rcode"])
	assert.Equal(t, float64(2), entry.Fields["answer_count"])
	assert.Equal(t, "explicit", entry.Fields["resolver"])
	assert.Equal(t, 12.345, entry.Fields["duration_ms"])
	assert.Equal(t, true, entry.Fields["request_matched"])
	assert.Equal(t, true, entry.Fields["cname_matched"])
}

func TestLogger_DNSDebug(t *testing.T) {
	t.Run("debug disabled", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewLogger(Config{Output: buf, Debug: false})

		logger.LogDNSDebug(DNSDebug{
			MatchedPattern: ".*",
		})

		assert.Empty(t, buf.String())
	})

	t.Run("debug enabled", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: true})

		logger.LogDNSDebug(DNSDebug{
			MatchedPattern: ".*\\.example\\.com$",
			Request:        "test.example.com",
		})

		output := buf.String()
		// Should log the pattern match
		assert.Contains(t, output, "REQUEST_PATTERN matched")
		// Verify the request is present
		assert.Contains(t, output, "test.example.com")
		// Verify pattern is present (JSON double-escapes backslashes)
		assert.Contains(t, output, "pattern")
	})
}

func TestDefaultLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf})
	SetDefault(logger)

	Info("test from default")
	assert.Contains(t, buf.String(), "test from default")

	// Restore
	SetDefault(NewLogger(Config{}))
}

func TestLogger_SetDebug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Debug: false})

	logger.Debug("should not appear")
	assert.Empty(t, buf.String())

	logger.SetDebug(true)
	logger.Debug("should appear")
	assert.Contains(t, buf.String(), "should appear")
}

func TestLogger_SetFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Output: buf, Format: FormatText})

	logger.Info("text format")
	assert.True(t, strings.HasPrefix(buf.String(), "20")) // Starts with year

	buf.Reset()
	logger.SetFormat(FormatJSON)
	logger.Info("json format")
	assert.True(t, strings.HasPrefix(buf.String(), "{")) // Starts with JSON object
}
