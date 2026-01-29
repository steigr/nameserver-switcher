# Logging Implementation Summary

## Overview

Comprehensive logging has been added to the nameserver-switcher project with configurable log levels and detailed information about DNS request processing.

## Changes Made

### 1. Configuration (`internal/config/config.go`)

Added three new configuration options:

- **`Debug`** (bool): Enable debug logging
- **`LogRequests`** (bool): Log all DNS requests (default: true)
- **`LogResponses`** (bool): Log all DNS responses (default: true)

These can be configured via:
- Command-line flags: `--debug`, `--log-requests`, `--log-responses`
- Environment variables: `DEBUG`, `LOG_REQUESTS`, `LOG_RESPONSES`

### 2. DNS Server (`internal/dns/server.go`)

Updated the DNS server to include logging throughout request processing:

- Added `Config` field to Server struct
- Updated `ServerConfig` to include `Config *config.Config`
- Modified `handleRequest()` to log:
  - **Request logging**: Protocol, query type, query name, client address
  - **Response logging**: Query name, response code, answer count, resolver used, duration
  - **Debug logging**: Pattern matches, CNAME matches, queried nameserver, full DNS response

### 3. gRPC Server (`internal/grpc/server.go`)

Updated the gRPC server's `Query()` method to include similar logging:

- Request logging with protocol=grpc
- Response logging with resolver and duration information
- Debug logging for pattern matching and full responses

### 4. Main Application (`cmd/nameserver-switcher/main.go`)

Updated `NewApp()` to pass the configuration to the DNS server:

```go
dnsServer := dnsserver.NewServer(dnsserver.ServerConfig{
    Addr:    cfg.DNSListenAddr,
    Port:    cfg.DNSPort,
    Router:  router,
    Metrics: m,
    Config:  cfg,  // Added config
})
```

## Logging Formats

### Normal Mode (LOG_REQUESTS=true, LOG_RESPONSES=true)

**Request:**
```
[REQUEST] protocol=udp type=A name=example.com. from=127.0.0.1:54321
```

**Response:**
```
[RESPONSE] name=example.com. rcode=NOERROR answers=1 resolver=8.8.8.8:53 duration=15.234ms
```

### Debug Mode (DEBUG=true)

**Pattern Matching:**
```
[DEBUG] REQUEST_PATTERN matched: pattern=".*\\.example\\.com$" request="test.example.com."
```

**CNAME Matching:**
```
[DEBUG] CNAME_PATTERN matched: pattern=".*\\.cdn\\..*" cname="www.cdn.example.net"
```

**Resolver Information:**
```
[DEBUG] Queried nameserver: 1.1.1.1:53
```

**Full Response:**
```
[DEBUG] Full response: ;; opcode: QUERY, status: NOERROR, id: 12345
;; flags: qr rd ra; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0
...
```

## Usage Examples

### Enable Full Debug Logging

**Command line:**
```bash
./nameserver-switcher --debug --log-requests --log-responses
```

**Environment variables:**
```bash
export DEBUG=true
export LOG_REQUESTS=true
export LOG_RESPONSES=true
./nameserver-switcher
```

### Minimal Logging (Errors Only)

```bash
./nameserver-switcher --log-requests=false --log-responses=false
```

### Normal Logging (Default)

```bash
./nameserver-switcher
# LOG_REQUESTS=true and LOG_RESPONSES=true by default
```

## Documentation

Created comprehensive documentation:

- **README.md**: Updated with logging configuration options
- **docs/LOGGING.md**: Detailed logging examples and format reference
- **test-logging.sh**: Script to demonstrate logging functionality
- **cmd/nameserver-switcher/logging_test.go**: Tests for logging functionality

## Testing

Added comprehensive tests:

- `TestLogging_NormalMode`: Tests request/response logging
- `TestLogging_DebugMode`: Tests debug logging with pattern matching
- `TestLogging_Disabled`: Tests that logging can be disabled

All tests pass successfully.

## Benefits

1. **Visibility**: Full visibility into DNS request processing
2. **Debugging**: Detailed debug information for troubleshooting
3. **Production Ready**: Configurable logging levels for different environments
4. **Performance**: Log output can be disabled to minimize overhead
5. **Pattern Inspection**: See exactly which patterns match and when
6. **Resolver Tracking**: Know which resolver was used for each request

## Future Enhancements

Potential improvements:

- Structured logging (JSON format) for log aggregation
- Different log levels (TRACE, INFO, WARN, ERROR)
- Log rotation configuration
- Request ID tracking for correlation
- Sampling for high-volume environments
