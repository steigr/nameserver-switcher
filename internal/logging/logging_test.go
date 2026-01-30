package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// testBuffer wraps bytes.Buffer to implement zapcore.WriteSyncer
type testBuffer struct {
	*bytes.Buffer
}

func (t *testBuffer) Sync() error {
	return nil
}

func newTestBuffer() *testBuffer {
	return &testBuffer{Buffer: &bytes.Buffer{}}
}

func TestNewLogger(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		logger := NewLogger(Config{})
		assert.NotNil(t, logger)
	})

	t.Run("with output", func(t *testing.T) {
		buf := newTestBuffer()
		logger := NewLogger(Config{Output: buf})
		logger.Info("test")
		assert.Contains(t, buf.String(), "test")
	})

	t.Run("with JSON format", func(t *testing.T) {
		buf := newTestBuffer()
		logger := NewLogger(Config{Output: buf, Format: FormatJSON})
		logger.Info("test message")

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)
		assert.Equal(t, "test message", entry["message"])
		assert.Equal(t, "INFO", entry["level"])
		assert.NotNil(t, entry["timemillis"])
	})
}

func TestLogger_Levels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(l *Logger, msg string)
		level    string
		debug    bool
		expected bool
	}{
		{"debug enabled", func(l *Logger, msg string) { l.Debug(msg) }, "DEBUG", true, true},
		{"debug disabled", func(l *Logger, msg string) { l.Debug(msg) }, "DEBUG", false, false},
		{"info", func(l *Logger, msg string) { l.Info(msg) }, "INFO", false, true},
		{"warn", func(l *Logger, msg string) { l.Warn(msg) }, "WARN", false, true},
		{"error", func(l *Logger, msg string) { l.Error(msg) }, "ERROR", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newTestBuffer()
			logger := NewLogger(Config{Output: buf, Debug: tt.debug})
			tt.logFunc(logger, "test message")

			if tt.expected {
				assert.Contains(t, buf.String(), "test message")
				assert.Contains(t, buf.String(), tt.level)
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: true})

	t.Run("contains timemillis", func(t *testing.T) {
		buf.Reset()
		logger.Info("test")

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		// Verify timemillis is present and reasonable
		timemillis, ok := entry["timemillis"].(float64)
		require.True(t, ok, "timemillis should be a number")
		now := time.Now().UnixMilli()
		assert.InDelta(t, now, timemillis, 1000) // Within 1 second
	})

	t.Run("contains all fields", func(t *testing.T) {
		buf.Reset()
		logger.Info("test message", map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		})

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		assert.Equal(t, "test message", entry["message"])
		assert.Equal(t, "INFO", entry["level"])
		assert.NotEmpty(t, entry["time"])
		assert.Equal(t, "value1", entry["key1"])
		assert.Equal(t, float64(123), entry["key2"]) // JSON numbers are float64
	})

	t.Run("debug with fields", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message", map[string]interface{}{
			"pattern": ".*\\.example\\.com$",
		})

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)

		assert.Equal(t, "DEBUG", entry["level"])
		assert.Equal(t, ".*\\.example\\.com$", entry["pattern"])
	})
}

func TestLogger_TextFormat(t *testing.T) {
	buf := newTestBuffer()
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
		// Zap uses JSON-style field output in console mode
		assert.Contains(t, output, "key")
		assert.Contains(t, output, "value")
	})

	t.Run("with component", func(t *testing.T) {
		buf.Reset()
		componentLogger := logger.WithComponent("dns")
		componentLogger.Info("test")

		output := buf.String()
		assert.Contains(t, output, "dns")
	})
}

func TestLogger_WithComponent(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	componentLogger := logger.WithComponent("dns-server")
	componentLogger.Info("test")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "dns-server", entry["component"])
}

func TestLogger_FormattedFunctions(t *testing.T) {
	buf := newTestBuffer()
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
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	logger.LogDNSRequest(DNSRequest{
		Protocol: "udp",
		Type:     "A",
		Name:     "example.com.",
		From:     "127.0.0.1:12345",
	})

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DNS request received", entry["message"])
	assert.Equal(t, "udp", entry["protocol"])
	assert.Equal(t, "A", entry["type"])
	assert.Equal(t, "example.com.", entry["name"])
	assert.Equal(t, "127.0.0.1:12345", entry["from"])
}

func TestLogger_DNSResponse(t *testing.T) {
	buf := newTestBuffer()
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

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DNS response sent", entry["message"])
	assert.Equal(t, "example.com.", entry["name"])
	assert.Equal(t, "NOERROR", entry["rcode"])
	assert.Equal(t, float64(2), entry["answer_count"])
	assert.Equal(t, "explicit", entry["resolver"])
	assert.Equal(t, 12.345, entry["duration_ms"])
	assert.Equal(t, true, entry["request_matched"])
	assert.Equal(t, true, entry["cname_matched"])
}

func TestLogger_DNSDebug(t *testing.T) {
	t.Run("debug disabled", func(t *testing.T) {
		buf := newTestBuffer()
		logger := NewLogger(Config{Output: buf, Debug: false})

		logger.LogDNSDebug(DNSDebug{
			MatchedPattern: ".*",
		})

		assert.Empty(t, buf.String())
	})

	t.Run("debug enabled", func(t *testing.T) {
		buf := newTestBuffer()
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
		// Verify pattern is present
		assert.Contains(t, output, "pattern")
	})
}

func TestDefaultLogger(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf})
	SetDefault(logger)

	Info("test from default")
	assert.Contains(t, buf.String(), "test from default")

	// Restore
	SetDefault(NewLogger(Config{}))
}

func TestLogger_SetDebug(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Debug: false})

	logger.Debug("should not appear")
	assert.Empty(t, buf.String())

	logger.SetDebug(true)
	logger.Debug("should appear")
	assert.Contains(t, buf.String(), "should appear")
}

func TestLogger_SetFormat(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatText})

	logger.Info("text format")
	assert.True(t, strings.HasPrefix(buf.String(), "20")) // Starts with year

	buf.Reset()
	logger.SetFormat(FormatJSON)
	logger.Info("json format")
	assert.True(t, strings.HasPrefix(buf.String(), "{")) // Starts with JSON object
}

func TestLogger_Sync(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf})

	logger.Info("test")
	err := logger.Sync()
	assert.NoError(t, err)
}

func TestLogger_JSONTimemillis(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	beforeTime := time.Now().UnixMilli()
	logger.Info("test timemillis")
	afterTime := time.Now().UnixMilli()

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	timemillis, ok := entry["timemillis"].(float64)
	require.True(t, ok, "timemillis should be present and be a number")

	// Verify timemillis is within the expected range
	assert.GreaterOrEqual(t, int64(timemillis), beforeTime)
	assert.LessOrEqual(t, int64(timemillis), afterTime)
}

// TestConfig_OutputAcceptsWriteSyncer verifies that Config.Output accepts zapcore.WriteSyncer
func TestConfig_OutputAcceptsWriteSyncer(t *testing.T) {
	buf := newTestBuffer()
	var output zapcore.WriteSyncer = buf // Verify it implements the interface

	logger := NewLogger(Config{Output: output})
	logger.Info("test")
	assert.Contains(t, buf.String(), "test")
}

// Test package-level functions using default logger
func TestPackageLevelFunctions(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Debug: true})
	SetDefault(logger)
	defer SetDefault(NewLogger(Config{})) // Restore default

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		Debug("debug message")
		assert.Contains(t, buf.String(), "debug message")
	})

	t.Run("Debug with fields", func(t *testing.T) {
		buf.Reset()
		Debug("debug with fields", map[string]interface{}{"key": "value"})
		assert.Contains(t, buf.String(), "debug with fields")
		assert.Contains(t, buf.String(), "key")
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		Warn("warn message")
		assert.Contains(t, buf.String(), "warn message")
	})

	t.Run("Warn with fields", func(t *testing.T) {
		buf.Reset()
		Warn("warn with fields", map[string]interface{}{"key": "value"})
		assert.Contains(t, buf.String(), "warn with fields")
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		Error("error message")
		assert.Contains(t, buf.String(), "error message")
	})

	t.Run("Error with fields", func(t *testing.T) {
		buf.Reset()
		Error("error with fields", map[string]interface{}{"key": "value"})
		assert.Contains(t, buf.String(), "error with fields")
	})

	t.Run("Debugf", func(t *testing.T) {
		buf.Reset()
		Debugf("debugf %s", "message")
		assert.Contains(t, buf.String(), "debugf message")
	})

	t.Run("Infof", func(t *testing.T) {
		buf.Reset()
		Infof("infof %s", "message")
		assert.Contains(t, buf.String(), "infof message")
	})

	t.Run("Warnf", func(t *testing.T) {
		buf.Reset()
		Warnf("warnf %s", "message")
		assert.Contains(t, buf.String(), "warnf message")
	})

	t.Run("Errorf", func(t *testing.T) {
		buf.Reset()
		Errorf("errorf %s", "message")
		assert.Contains(t, buf.String(), "errorf message")
	})

	t.Run("Sync", func(t *testing.T) {
		err := Sync()
		assert.NoError(t, err)
	})
}

// Test package-level DNS logging functions
func TestPackageLevelDNSFunctions(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: true})
	SetDefault(logger)
	defer SetDefault(NewLogger(Config{})) // Restore default

	t.Run("LogDNSRequest", func(t *testing.T) {
		buf.Reset()
		LogDNSRequest(DNSRequest{
			Protocol: "tcp",
			Type:     "AAAA",
			Name:     "test.example.com.",
			From:     "192.168.1.1:12345",
		})
		assert.Contains(t, buf.String(), "DNS request received")
		assert.Contains(t, buf.String(), "tcp")
	})

	t.Run("LogDNSResponse", func(t *testing.T) {
		buf.Reset()
		LogDNSResponse(DNSResponse{
			Name:        "test.example.com.",
			Rcode:       "NXDOMAIN",
			AnswerCount: 0,
			Resolver:    "system",
			DurationMs:  5.5,
		})
		assert.Contains(t, buf.String(), "DNS response sent")
		assert.Contains(t, buf.String(), "NXDOMAIN")
	})

	t.Run("LogDNSDebug", func(t *testing.T) {
		buf.Reset()
		LogDNSDebug(DNSDebug{
			MatchedPattern: ".*test.*",
			Request:        "test.com",
		})
		assert.Contains(t, buf.String(), "REQUEST_PATTERN matched")
	})
}

// Test SetDebug with JSON format
func TestLogger_SetDebug_JSONFormat(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: false})

	logger.Debug("should not appear")
	assert.Empty(t, buf.String())

	logger.SetDebug(true)
	logger.Debug("should appear in json")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "should appear in json", entry["message"])
	assert.Equal(t, "DEBUG", entry["level"])
}

// Test SetDebug with component
func TestLogger_SetDebug_WithComponent(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Component: "test-component", Debug: false})

	logger.SetDebug(true)
	logger.Debug("debug with component")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "test-component", entry["component"])
}

// Test SetFormat with component
func TestLogger_SetFormat_WithComponent(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatText, Component: "my-component"})

	logger.Info("text mode")
	assert.Contains(t, buf.String(), "my-component")

	buf.Reset()
	logger.SetFormat(FormatJSON)
	logger.Info("json mode")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "my-component", entry["component"])
}

// Test SetFormat to text from JSON
func TestLogger_SetFormat_ToText(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	logger.Info("json format")
	assert.True(t, strings.HasPrefix(buf.String(), "{"))

	buf.Reset()
	logger.SetFormat(FormatText)
	logger.Info("text format")
	assert.True(t, strings.HasPrefix(buf.String(), "20")) // Starts with year
}

// Test SetFormat with debug enabled
func TestLogger_SetFormat_WithDebug(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatText, Debug: true})

	logger.Debug("text debug")
	assert.Contains(t, buf.String(), "text debug")

	buf.Reset()
	logger.SetFormat(FormatJSON)
	logger.Debug("json debug")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "DEBUG", entry["level"])
}

// Test NewLogger with component
func TestNewLogger_WithComponent(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Component: "dns-server"})

	logger.Info("test with component")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "dns-server", entry["component"])
}

// Test Warn and Error methods without fields
func TestLogger_WarnErrorWithoutFields(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf})

	t.Run("Warn without fields", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warning message")
		assert.Contains(t, buf.String(), "warning message")
		assert.Contains(t, buf.String(), "WARN")
	})

	t.Run("Error without fields", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message")
		assert.Contains(t, buf.String(), "error message")
		assert.Contains(t, buf.String(), "ERROR")
	})
}

// Test Warn and Error methods with fields
func TestLogger_WarnErrorWithFields(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	t.Run("Warn with fields", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warning", map[string]interface{}{"code": 123})

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)
		assert.Equal(t, "WARN", entry["level"])
		assert.Equal(t, float64(123), entry["code"])
	})

	t.Run("Error with fields", func(t *testing.T) {
		buf.Reset()
		logger.Error("error", map[string]interface{}{"code": 456})

		var entry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &entry)
		require.NoError(t, err)
		assert.Equal(t, "ERROR", entry["level"])
		assert.Equal(t, float64(456), entry["code"])
	})
}

// Test Debugf when debug is disabled
func TestLogger_Debugf_Disabled(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Debug: false})

	logger.Debugf("debug %s", "should not appear")
	assert.Empty(t, buf.String())
}

// Test fieldsToZapFields with nil
func TestFieldsToZapFields_Nil(t *testing.T) {
	result := fieldsToZapFields(nil)
	assert.Nil(t, result)
}

// Test LogDNSDebug with all field types
func TestLogger_LogDNSDebug_AllFields(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON, Debug: true})

	t.Run("with CNAME pattern", func(t *testing.T) {
		buf.Reset()
		logger.LogDNSDebug(DNSDebug{
			CNAMEPattern: ".*\\.cdn\\.com$",
			CNAME:        "www.cdn.com",
		})
		assert.Contains(t, buf.String(), "CNAME_PATTERN matched")
		assert.Contains(t, buf.String(), "cdn.com")
	})

	t.Run("with resolver", func(t *testing.T) {
		buf.Reset()
		logger.LogDNSDebug(DNSDebug{
			Resolver: "explicit",
		})
		assert.Contains(t, buf.String(), "Queried nameserver")
		assert.Contains(t, buf.String(), "explicit")
	})

	t.Run("with full response", func(t *testing.T) {
		buf.Reset()
		logger.LogDNSDebug(DNSDebug{
			FullResponse: ";; ANSWER SECTION:\nexample.com. 300 IN A 1.2.3.4",
		})
		assert.Contains(t, buf.String(), "Full response")
		assert.Contains(t, buf.String(), "ANSWER SECTION")
	})

	t.Run("with all fields", func(t *testing.T) {
		buf.Reset()
		logger.LogDNSDebug(DNSDebug{
			MatchedPattern: ".*\\.example\\.com$",
			Request:        "test.example.com",
			CNAMEPattern:   ".*\\.cdn\\.com$",
			CNAME:          "www.cdn.com",
			Resolver:       "explicit",
			FullResponse:   "full response data",
		})
		output := buf.String()
		assert.Contains(t, output, "REQUEST_PATTERN matched")
		assert.Contains(t, output, "CNAME_PATTERN matched")
		assert.Contains(t, output, "Queried nameserver")
		assert.Contains(t, output, "Full response")
	})
}

// Test DNSResponse without matched flags
func TestLogger_DNSResponse_NoMatches(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	logger.LogDNSResponse(DNSResponse{
		Name:           "example.com.",
		Rcode:          "NOERROR",
		AnswerCount:    1,
		Resolver:       "system",
		DurationMs:     5.0,
		RequestMatched: false,
		CNAMEMatched:   false,
	})

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	// These should not be present when false
	_, hasRequestMatched := entry["request_matched"]
	_, hasCnameMatched := entry["cname_matched"]
	assert.False(t, hasRequestMatched, "request_matched should not be present when false")
	assert.False(t, hasCnameMatched, "cname_matched should not be present when false")
}

// Test Clone method of customJSONEncoder
func TestCustomJSONEncoder_Clone(t *testing.T) {
	buf := newTestBuffer()
	logger := NewLogger(Config{Output: buf, Format: FormatJSON})

	// Clone is called internally when using With
	loggerWithField := logger.WithComponent("cloned")
	loggerWithField.Info("test clone")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "cloned", entry["component"])
	assert.NotNil(t, entry["timemillis"])
}
