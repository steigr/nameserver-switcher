.PHONY: all build test lint clean docker helm integration-test integration-test-grpc integration-test-dns

# Variables
BINARY_NAME=nameserver-switcher
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-w -s -X main.version=$(VERSION)"
DOCKER_IMAGE=ghcr.io/steigr/nameserver-switcher
DOCKER_TAG?=$(VERSION)

all: build

# Build the binary
build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/nameserver-switcher

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/nameserver-switcher
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/nameserver-switcher
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/nameserver-switcher
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/nameserver-switcher

# Run tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Build Docker image
docker:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Push Docker image
docker-push: docker
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

# Build multi-arch Docker image
docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .

# Helm lint
helm-lint:
	helm lint charts/nameserver-switcher

# Helm package
helm-package:
	helm package charts/nameserver-switcher

# Generate protobuf code (requires protoc and plugins)
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		pkg/api/v1/switcher.proto

# Run the application locally
run: build
	./bin/$(BINARY_NAME)

# Run with example configuration
run-example: build
	./bin/$(BINARY_NAME) \
		--request-patterns=".*\.example\.com$$" \
		--cname-patterns=".*\.cdn\.com$$" \
		--request-resolver="8.8.8.8:53" \
		--explicit-resolver="1.1.1.1:53"

# Install development dependencies
dev-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  tidy         - Tidy dependencies"
	@echo "  clean        - Clean build artifacts"
	@echo "  docker       - Build Docker image"
	@echo "  docker-push  - Push Docker image"
	@echo "  helm-lint    - Lint Helm chart"
	@echo "  helm-package - Package Helm chart"
	@echo "  proto        - Generate protobuf code"
	@echo "  run          - Run the application locally"
	@echo "  run-example  - Run with example configuration"
	@echo "  dev-deps     - Install development dependencies"
	@echo "  integration-test       - Run all integration tests"
	@echo "  integration-test-dns   - Run DNS mode integration tests"
	@echo "  integration-test-grpc  - Run gRPC mode integration tests"
	@echo "  integration-test-junit - Run integration tests with JUnit XML output"
	@echo "  test-junit   - Run unit tests with JUnit XML output"

# Run all integration tests using testcontainers-go
integration-test:
	go test -tags=integration -v -timeout 10m ./test/integration/...

# Run DNS mode integration tests using testcontainers-go
integration-test-dns:
	go test -tags=integration -v -timeout 10m -run TestDNSModeSuite ./test/integration/...

# Run gRPC mode integration tests using testcontainers-go
integration-test-grpc:
	go test -tags=integration -v -timeout 10m -run TestGRPCModeSuite ./test/integration/...

# Run integration tests with JUnit XML output
integration-test-junit:
	@mkdir -p test-results
	go run gotest.tools/gotestsum@latest --junitfile test-results/integration-tests.xml -- -tags=integration -timeout 10m ./test/integration/...

# Run all tests with JUnit XML output
test-junit:
	@mkdir -p test-results
	go run gotest.tools/gotestsum@latest --junitfile test-results/unit-tests.xml -- -v -race -coverprofile=coverage.out ./internal/... ./pkg/...
