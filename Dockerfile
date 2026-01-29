# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd ./cmd
COPY pkg ./pkg
COPY internal ./internal

# Build the binary for the native architecture
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s" \
    -o /nameserver-switcher \
    ./cmd/nameserver-switcher

# Runtime stage
FROM alpine:3.23.2 AS runtime-stage-prepare

# Install ca-certificates for HTTPS and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S switcher && adduser -S switcher -G switcher

# Copy binary from builder
COPY --from=builder --chown=switcher:switcher /nameserver-switcher /usr/local/bin/nameserver-switcher

# Switch to non-root user
USER switcher

# Expose ports
# DNS (UDP and TCP)
EXPOSE 5353/udp
EXPOSE 5353/tcp
# gRPC
EXPOSE 5354
# HTTP (health/metrics)
EXPOSE 8080

# Default environment variables
ENV DNS_LISTEN_ADDR=0.0.0.0 \
    DNS_PORT=5353 \
    GRPC_LISTEN_ADDR=0.0.0.0 \
    GRPC_PORT=5354 \
    HTTP_LISTEN_ADDR=0.0.0.0 \
    HTTP_PORT=8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/healthz || exit 1

# Run the binary
ENTRYPOINT []
CMD ["nameserver-switcher"]
