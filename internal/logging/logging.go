// Package logging provides structured logging with support for JSON and text formats using zap.
package logging

import (
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// Format represents the log output format.
type Format string

const (
	// FormatText outputs logs in human-readable text format.
	FormatText Format = "text"
	// FormatJSON outputs logs in JSON format.
	FormatJSON Format = "json"
)

// Level represents the log level.
type Level string

const (
	// LevelDebug is for debug messages.
	LevelDebug Level = "DEBUG"
	// LevelInfo is for informational messages.
	LevelInfo Level = "INFO"
	// LevelWarn is for warning messages.
	LevelWarn Level = "WARN"
	// LevelError is for error messages.
	LevelError Level = "ERROR"
)

// Logger provides structured logging capabilities using zap.
type Logger struct {
	mu        sync.RWMutex
	zap       *zap.Logger
	sugar     *zap.SugaredLogger
	format    Format
	debug     bool
	component string
	output    zapcore.WriteSyncer
}

// Config holds logger configuration.
type Config struct {
	// Output is the writer for log output (default: os.Stderr).
	Output zapcore.WriteSyncer
	// Format is the log format: "text" or "json" (default: "text").
	Format Format
	// Debug enables debug level logging.
	Debug bool
	// Component is an optional component name to include in logs.
	Component string
}

// LogEntry represents a structured log entry (for test compatibility).
type LogEntry struct {
	// TimeMillis is the timestamp in milliseconds since Unix epoch.
	TimeMillis int64 `json:"timemillis"`
	// Time is the human-readable timestamp (ISO 8601).
	Time string `json:"time"`
	// Level is the log level.
	Level Level `json:"level"`
	// Message is the log message.
	Message string `json:"message"`
	// Component is the optional component name.
	Component string `json:"component,omitempty"`
	// Fields contains additional structured fields.
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// defaultLogger is the global logger instance.
var defaultLogger = NewLogger(Config{})
var defaultLoggerMu sync.RWMutex

// customTimeEncoder encodes time as RFC3339Nano for the "time" field.
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.RFC3339Nano))
}

// customTextTimeEncoder encodes time in a human-readable format for text output.
func customTextTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006/01/02 15:04:05.000"))
}

// customLevelEncoder encodes levels in uppercase.
func customLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + l.CapitalString() + "]")
}

// customJSONLevelEncoder encodes levels in uppercase for JSON.
func customJSONLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(l.CapitalString())
}

// NewLogger creates a new logger with the given configuration.
func NewLogger(cfg Config) *Logger {
	output := cfg.Output
	if output == nil {
		output = zapcore.AddSync(os.Stderr)
	}

	format := cfg.Format
	if format == "" {
		format = FormatText
	}

	level := zapcore.InfoLevel
	if cfg.Debug {
		level = zapcore.DebugLevel
	}

	var encoder zapcore.Encoder
	if format == FormatJSON {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    customJSONLevelEncoder,
			EncodeTime:     customTimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		encoder = newCustomJSONEncoder(encoderConfig)
	} else {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:          "time",
			LevelKey:         "level",
			NameKey:          "logger",
			CallerKey:        "",
			FunctionKey:      zapcore.OmitKey,
			MessageKey:       "message",
			StacktraceKey:    "",
			LineEnding:       zapcore.DefaultLineEnding,
			EncodeLevel:      customLevelEncoder,
			EncodeTime:       customTextTimeEncoder,
			EncodeDuration:   zapcore.MillisDurationEncoder,
			EncodeCaller:     zapcore.ShortCallerEncoder,
			ConsoleSeparator: " ",
		}
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(encoder, output, level)
	zapLogger := zap.New(core)

	if cfg.Component != "" {
		zapLogger = zapLogger.With(zap.String("component", cfg.Component))
	}

	return &Logger{
		zap:       zapLogger,
		sugar:     zapLogger.Sugar(),
		format:    format,
		debug:     cfg.Debug,
		component: cfg.Component,
		output:    output,
	}
}

// customJSONEncoder wraps the JSON encoder to add timemillis field.
type customJSONEncoder struct {
	zapcore.Encoder
	config zapcore.EncoderConfig
}

func newCustomJSONEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &customJSONEncoder{
		Encoder: zapcore.NewJSONEncoder(cfg),
		config:  cfg,
	}
}

func (e *customJSONEncoder) Clone() zapcore.Encoder {
	return &customJSONEncoder{
		Encoder: e.Encoder.Clone(),
		config:  e.config,
	}
}

func (e *customJSONEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// Add timemillis field
	fields = append(fields, zap.Int64("timemillis", entry.Time.UnixMilli()))
	return e.Encoder.EncodeEntry(entry, fields)
}

// SetDefault sets the default global logger.
func SetDefault(l *Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = l
}

// Default returns the default global logger.
func Default() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// WithComponent returns a new logger with the specified component name.
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newZap := l.zap.With(zap.String("component", component))
	return &Logger{
		zap:       newZap,
		sugar:     newZap.Sugar(),
		format:    l.format,
		debug:     l.debug,
		component: component,
		output:    l.output,
	}
}

// SetDebug enables or disables debug logging.
func (l *Logger) SetDebug(debug bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug = debug

	// Rebuild the logger with new level
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	var encoder zapcore.Encoder
	if l.format == FormatJSON {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    customJSONLevelEncoder,
			EncodeTime:     customTimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		encoder = newCustomJSONEncoder(encoderConfig)
	} else {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:          "time",
			LevelKey:         "level",
			NameKey:          "logger",
			CallerKey:        "",
			FunctionKey:      zapcore.OmitKey,
			MessageKey:       "message",
			StacktraceKey:    "",
			LineEnding:       zapcore.DefaultLineEnding,
			EncodeLevel:      customLevelEncoder,
			EncodeTime:       customTextTimeEncoder,
			EncodeDuration:   zapcore.MillisDurationEncoder,
			EncodeCaller:     zapcore.ShortCallerEncoder,
			ConsoleSeparator: " ",
		}
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(encoder, l.output, level)
	l.zap = zap.New(core)
	if l.component != "" {
		l.zap = l.zap.With(zap.String("component", l.component))
	}
	l.sugar = l.zap.Sugar()
}

// SetFormat sets the log format.
func (l *Logger) SetFormat(format Format) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format

	level := zapcore.InfoLevel
	if l.debug {
		level = zapcore.DebugLevel
	}

	var encoder zapcore.Encoder
	if format == FormatJSON {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    customJSONLevelEncoder,
			EncodeTime:     customTimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		encoder = newCustomJSONEncoder(encoderConfig)
	} else {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:          "time",
			LevelKey:         "level",
			NameKey:          "logger",
			CallerKey:        "",
			FunctionKey:      zapcore.OmitKey,
			MessageKey:       "message",
			StacktraceKey:    "",
			LineEnding:       zapcore.DefaultLineEnding,
			EncodeLevel:      customLevelEncoder,
			EncodeTime:       customTextTimeEncoder,
			EncodeDuration:   zapcore.MillisDurationEncoder,
			EncodeCaller:     zapcore.ShortCallerEncoder,
			ConsoleSeparator: " ",
		}
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(encoder, l.output, level)
	l.zap = zap.New(core)
	if l.component != "" {
		l.zap = l.zap.With(zap.String("component", l.component))
	}
	l.sugar = l.zap.Sugar()
}

// fieldsToZapFields converts a map to zap fields.
func fieldsToZapFields(fields map[string]interface{}) []zap.Field {
	if fields == nil {
		return nil
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return zapFields
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.debug {
		return
	}
	if len(fields) > 0 {
		l.zap.Debug(msg, fieldsToZapFields(fields[0])...)
	} else {
		l.zap.Debug(msg)
	}
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(fields) > 0 {
		l.zap.Info(msg, fieldsToZapFields(fields[0])...)
	} else {
		l.zap.Info(msg)
	}
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(fields) > 0 {
		l.zap.Warn(msg, fieldsToZapFields(fields[0])...)
	} else {
		l.zap.Warn(msg)
	}
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(fields) > 0 {
		l.zap.Error(msg, fieldsToZapFields(fields[0])...)
	} else {
		l.zap.Error(msg)
	}
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.debug {
		return
	}
	l.sugar.Debugf(format, args...)
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.sugar.Infof(format, args...)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.sugar.Warnf(format, args...)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.sugar.Errorf(format, args...)
}

// --- Package-level functions using the default logger ---

// Debug logs a debug message using the default logger.
func Debug(msg string, fields ...map[string]interface{}) {
	Default().Debug(msg, fields...)
}

// Info logs an info message using the default logger.
func Info(msg string, fields ...map[string]interface{}) {
	Default().Info(msg, fields...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, fields ...map[string]interface{}) {
	Default().Warn(msg, fields...)
}

// Error logs an error message using the default logger.
func Error(msg string, fields ...map[string]interface{}) {
	Default().Error(msg, fields...)
}

// Debugf logs a formatted debug message using the default logger.
func Debugf(format string, args ...interface{}) {
	Default().Debugf(format, args...)
}

// Infof logs a formatted info message using the default logger.
func Infof(format string, args ...interface{}) {
	Default().Infof(format, args...)
}

// Warnf logs a formatted warning message using the default logger.
func Warnf(format string, args ...interface{}) {
	Default().Warnf(format, args...)
}

// Errorf logs a formatted error message using the default logger.
func Errorf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
}

// --- Specialized DNS logging functions ---

// DNSRequest represents a DNS request log entry.
type DNSRequest struct {
	Protocol string `json:"protocol"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	From     string `json:"from"`
}

// DNSResponse represents a DNS response log entry.
type DNSResponse struct {
	Name           string  `json:"name"`
	Rcode          string  `json:"rcode"`
	AnswerCount    int     `json:"answer_count"`
	Resolver       string  `json:"resolver"`
	DurationMs     float64 `json:"duration_ms"`
	RequestMatched bool    `json:"request_matched,omitempty"`
	CNAMEMatched   bool    `json:"cname_matched,omitempty"`
}

// DNSDebug represents debug information for DNS routing.
type DNSDebug struct {
	MatchedPattern string `json:"matched_pattern,omitempty"`
	Request        string `json:"request,omitempty"`
	CNAMEPattern   string `json:"cname_pattern,omitempty"`
	CNAME          string `json:"cname,omitempty"`
	Resolver       string `json:"resolver,omitempty"`
	FullResponse   string `json:"full_response,omitempty"`
}

// LogDNSRequest logs a DNS request.
func (l *Logger) LogDNSRequest(req DNSRequest) {
	fields := map[string]interface{}{
		"protocol": req.Protocol,
		"type":     req.Type,
		"name":     req.Name,
		"from":     req.From,
	}
	l.Info("DNS request received", fields)
}

// LogDNSResponse logs a DNS response.
func (l *Logger) LogDNSResponse(resp DNSResponse) {
	fields := map[string]interface{}{
		"name":         resp.Name,
		"rcode":        resp.Rcode,
		"answer_count": resp.AnswerCount,
		"resolver":     resp.Resolver,
		"duration_ms":  resp.DurationMs,
	}
	if resp.RequestMatched {
		fields["request_matched"] = true
	}
	if resp.CNAMEMatched {
		fields["cname_matched"] = true
	}
	l.Info("DNS response sent", fields)
}

// LogDNSDebug logs DNS routing debug information.
func (l *Logger) LogDNSDebug(debug DNSDebug) {
	l.mu.RLock()
	isDebug := l.debug
	l.mu.RUnlock()

	if !isDebug {
		return
	}

	if debug.MatchedPattern != "" {
		l.Debug("REQUEST_PATTERN matched", map[string]interface{}{
			"pattern": debug.MatchedPattern,
			"request": debug.Request,
		})
	}

	if debug.CNAMEPattern != "" {
		l.Debug("CNAME_PATTERN matched", map[string]interface{}{
			"pattern": debug.CNAMEPattern,
			"cname":   debug.CNAME,
		})
	}

	if debug.Resolver != "" {
		l.Debug("Queried nameserver", map[string]interface{}{
			"resolver": debug.Resolver,
		})
	}

	if debug.FullResponse != "" {
		l.Debug("Full response", map[string]interface{}{
			"response": debug.FullResponse,
		})
	}
}

// Package-level DNS logging functions

// LogDNSRequest logs a DNS request using the default logger.
func LogDNSRequest(req DNSRequest) {
	Default().LogDNSRequest(req)
}

// LogDNSResponse logs a DNS response using the default logger.
func LogDNSResponse(resp DNSResponse) {
	Default().LogDNSResponse(resp)
}

// LogDNSDebug logs DNS routing debug information using the default logger.
func LogDNSDebug(debug DNSDebug) {
	Default().LogDNSDebug(debug)
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.zap.Sync()
}

// Sync flushes any buffered log entries for the default logger.
func Sync() error {
	return Default().Sync()
}
