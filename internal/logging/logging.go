// Package logging provides structured logging with support for JSON and text formats.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
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

// Logger provides structured logging capabilities.
type Logger struct {
	mu        sync.Mutex
	output    io.Writer
	format    Format
	debug     bool
	component string
}

// Config holds logger configuration.
type Config struct {
	// Output is the writer for log output (default: os.Stderr).
	Output io.Writer
	// Format is the log format: "text" or "json" (default: "text").
	Format Format
	// Debug enables debug level logging.
	Debug bool
	// Component is an optional component name to include in logs.
	Component string
}

// LogEntry represents a structured log entry.
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

// NewLogger creates a new logger with the given configuration.
func NewLogger(cfg Config) *Logger {
	output := cfg.Output
	if output == nil {
		output = os.Stderr
	}

	format := cfg.Format
	if format == "" {
		format = FormatText
	}

	return &Logger{
		output:    output,
		format:    format,
		debug:     cfg.Debug,
		component: cfg.Component,
	}
}

// SetDefault sets the default global logger.
func SetDefault(l *Logger) {
	defaultLogger = l
}

// Default returns the default global logger.
func Default() *Logger {
	return defaultLogger
}

// WithComponent returns a new logger with the specified component name.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		output:    l.output,
		format:    l.format,
		debug:     l.debug,
		component: component,
	}
}

// SetDebug enables or disables debug logging.
func (l *Logger) SetDebug(debug bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug = debug
}

// SetFormat sets the log format.
func (l *Logger) SetFormat(format Format) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// log writes a log entry.
func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	entry := LogEntry{
		TimeMillis: now.UnixMilli(),
		Time:       now.Format(time.RFC3339Nano),
		Level:      level,
		Message:    msg,
		Component:  l.component,
		Fields:     fields,
	}

	var output string
	if l.format == FormatJSON {
		output = l.formatJSON(entry)
	} else {
		output = l.formatText(entry)
	}

	fmt.Fprintln(l.output, output)
}

// formatJSON formats the log entry as JSON.
func (l *Logger) formatJSON(entry LogEntry) string {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"error":"failed to marshal log entry: %v"}`, err)
	}
	return string(data)
}

// formatText formats the log entry as human-readable text.
func (l *Logger) formatText(entry LogEntry) string {
	timestamp := time.UnixMilli(entry.TimeMillis).Format("2006/01/02 15:04:05.000")

	var prefix string
	if entry.Component != "" {
		prefix = fmt.Sprintf("%s [%s] [%s]", timestamp, entry.Level, entry.Component)
	} else {
		prefix = fmt.Sprintf("%s [%s]", timestamp, entry.Level)
	}

	if len(entry.Fields) == 0 {
		return fmt.Sprintf("%s %s", prefix, entry.Message)
	}

	// Format fields as key=value pairs
	fieldsStr := ""
	for k, v := range entry.Fields {
		if fieldsStr != "" {
			fieldsStr += " "
		}
		fieldsStr += fmt.Sprintf("%s=%v", k, v)
	}

	return fmt.Sprintf("%s %s %s", prefix, entry.Message, fieldsStr)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	if !l.debug {
		return
	}
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	if !l.debug {
		return
	}
	l.log(LevelDebug, fmt.Sprintf(format, args...), nil)
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, fmt.Sprintf(format, args...), nil)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, fmt.Sprintf(format, args...), nil)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...), nil)
}

// --- Package-level functions using the default logger ---

// Debug logs a debug message using the default logger.
func Debug(msg string, fields ...map[string]interface{}) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message using the default logger.
func Info(msg string, fields ...map[string]interface{}) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, fields ...map[string]interface{}) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message using the default logger.
func Error(msg string, fields ...map[string]interface{}) {
	defaultLogger.Error(msg, fields...)
}

// Debugf logs a formatted debug message using the default logger.
func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

// Infof logs a formatted info message using the default logger.
func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

// Warnf logs a formatted warning message using the default logger.
func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

// Errorf logs a formatted error message using the default logger.
func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
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
	if !l.debug {
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
	defaultLogger.LogDNSRequest(req)
}

// LogDNSResponse logs a DNS response using the default logger.
func LogDNSResponse(resp DNSResponse) {
	defaultLogger.LogDNSResponse(resp)
}

// LogDNSDebug logs DNS routing debug information using the default logger.
func LogDNSDebug(debug DNSDebug) {
	defaultLogger.LogDNSDebug(debug)
}
