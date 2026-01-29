# Logging Examples

This document demonstrates the logging capabilities of nameserver-switcher.

## Example 1: Normal Logging (Requests and Responses)

Start the server with default logging:

```bash
./nameserver-switcher \
  --request-patterns=".*\.example\.com$" \
  --cname-patterns=".*\.cdn\..*" \
  --request-resolver="8.8.8.8:53" \
  --explicit-resolver="1.1.1.1:53"
```

Make a DNS query:

```bash
dig @localhost -p 5353 test.example.com A
```

**Expected log output:**

```
[REQUEST] protocol=udp type=A name=test.example.com. from=127.0.0.1:54321
[RESPONSE] name=test.example.com. rcode=NOERROR answers=1 resolver=8.8.8.8:53 duration=15.234ms
```

## Example 2: Debug Logging (Pattern Matching Details)

Start with debug mode enabled:

```bash
./nameserver-switcher \
  --debug \
  --request-patterns=".*\.example\.com$" \
  --cname-patterns=".*\.cdn\..*" \
  --request-resolver="8.8.8.8:53" \
  --explicit-resolver="1.1.1.1:53"
```

Make a DNS query that triggers pattern matching:

```bash
dig @localhost -p 5353 www.example.com A
```

**Expected log output:**

```
[REQUEST] protocol=udp type=A name=www.example.com. from=127.0.0.1:54321
[DEBUG] REQUEST_PATTERN matched: pattern=".*\\.example\\.com$" request="www.example.com"
[DEBUG] Queried nameserver: 8.8.8.8:53
[RESPONSE] name=www.example.com. rcode=NOERROR answers=1 resolver=8.8.8.8:53 duration=15.234ms
[DEBUG] Full response: ;; opcode: QUERY, status: NOERROR, id: 12345
;; flags: qr rd; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;www.example.com.               IN      A

;; ANSWER SECTION:
www.example.com.        3600    IN      CNAME   www.cdn.example.net.
```

If the CNAME matches the pattern:

```
[DEBUG] CNAME_PATTERN matched: pattern=".*\\.cdn\\..*" cname="www.cdn.example.net"
[DEBUG] Queried nameserver: 1.1.1.1:53
```

## Example 3: Minimal Logging (Errors Only)

Disable request and response logging:

```bash
./nameserver-switcher \
  --log-requests=false \
  --log-responses=false \
  --request-patterns=".*\.example\.com$"
```

Only errors will be logged:

```
Error routing request: no question in request
```

## Example 4: Environment Variables

```bash
export DEBUG=true
export LOG_REQUESTS=true
export LOG_RESPONSES=true
export REQUEST_PATTERNS=".*\.example\.com$
.*\.test\.org$"
export CNAME_PATTERNS=".*\.cdn\..*"
export REQUEST_RESOLVER="8.8.8.8:53"
export EXPLICIT_RESOLVER="1.1.1.1:53"

./nameserver-switcher
```

## Example 5: Docker Logs

When running in Docker, view logs with:

```bash
docker logs -f nameserver-switcher
```

## Example 6: Kubernetes Logs

View logs in Kubernetes:

```bash
kubectl logs -f deployment/nameserver-switcher

# With debug logging
kubectl set env deployment/nameserver-switcher DEBUG=true

# Follow logs
kubectl logs -f deployment/nameserver-switcher
```

## Log Format Reference

### Request Log Format

```
[REQUEST] protocol=<udp|tcp|grpc> type=<A|AAAA|CNAME|...> name=<query-name> [from=<client-addr>]
```

### Response Log Format

```
[RESPONSE] name=<query-name> rcode=<NOERROR|NXDOMAIN|SERVFAIL|...> answers=<count> resolver=<resolver-used> duration=<ms>
```

### Debug Log Formats

**Pattern match:**
```
[DEBUG] REQUEST_PATTERN matched: pattern="<regex>" request="<query-name>"
```

**CNAME match:**
```
[DEBUG] CNAME_PATTERN matched: pattern="<regex>" cname="<cname-value>"
```

**Resolver used:**
```
[DEBUG] Queried nameserver: <resolver-address>
```

**Full response:**
```
[DEBUG] Full response: <dns-message-text>
```
