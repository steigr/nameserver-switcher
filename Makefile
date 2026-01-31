.PHONY: all build test test-verbose test-coverage lint lint-fix fmt fmt-check vet staticcheck security-check gosec \
	tidy tidy-check clean docker helm integration-test integration-test-grpc integration-test-dns \
	quality quality-check ci-check install-tools install-golangci-lint install-staticcheck install-gosec help

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
# Note: LC_DYSYMTAB warnings on macOS are harmless linker warnings and can be ignored
test-silent:
	@echo "Running tests (linker warnings on macOS are expected and harmless)..."
	@go test -v -race -coverprofile=coverage.out ./... 2>&1 | grep -v "LC_DYSYMTAB" || true

# Run tests without filtering warnings (shows all output)
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html

## Code Quality Tools

# Run golangci-lint (comprehensive linter)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, installing..." && $(MAKE) install-golangci-lint)
	@golangci-lint run ./... || (echo "\n❌ Linting failed. Run 'make lint-fix' to auto-fix some issues." && exit 1)

# Run golangci-lint with auto-fix
lint-fix:
	@echo "Running golangci-lint with auto-fix..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, installing..." && $(MAKE) install-golangci-lint)
	golangci-lint run --fix ./...

# Format code with gofmt
fmt:
	@echo "Formatting code..."
	gofmt -w -s .

# Check if code is formatted
fmt-check:
	@echo "Checking code formatting..."
	@test -z "$$(gofmt -l -s . | tee /dev/stderr)" || (echo "\n❌ Code is not formatted. Run 'make fmt' to fix." && exit 1)

# Run go vet (built-in static analysis)
vet:
	@echo "Running go vet..."
	go vet ./...

# Run staticcheck (additional static analysis)
staticcheck:
	@echo "Running staticcheck..."
	@which staticcheck > /dev/null || (echo "staticcheck not found, installing..." && go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

# Security check with gosec
security-check:
	@echo "Running security checks with gosec..."
	@which gosec > /dev/null || (echo "gosec not found, installing..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)
	gosec -quiet ./...

# Comprehensive security scan with detailed report
gosec:
	@echo "Running comprehensive security scan..."
	@which gosec > /dev/null || (echo "gosec not found, installing..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)
	gosec -fmt=json -out=gosec-report.json ./...
	@echo "✅ Security scan complete. Report saved to gosec-report.json"

# Tidy dependencies
tidy:
	go mod tidy

# Check if dependencies are tidy
tidy-check:
	@echo "Checking if go.mod and go.sum are tidy..."
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "\n❌ go.mod or go.sum are not tidy. Run 'make tidy' and commit the changes." && exit 1)

## Comprehensive Quality Checks

# Run all quality checks (for local development)
quality: fmt vet lint staticcheck
	@echo "\n✅ All quality checks passed!"

# Run all quality checks including tests (for CI)
quality-check: fmt-check tidy-check vet lint staticcheck test
	@echo "\n✅ All quality checks and tests passed!"

# Run comprehensive CI checks (everything including security)
ci-check: fmt-check tidy-check vet lint staticcheck security-check test
	@echo "\n✅ All CI checks passed!"

## Tool Installation

# Install golangci-lint only
install-golangci-lint:
	@echo "Installing golangci-lint..."
	@command -v golangci-lint > /dev/null || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin
	@echo "✅ golangci-lint installed"

# Install staticcheck only
install-staticcheck:
	@echo "Installing staticcheck..."
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "✅ staticcheck installed"

# Install gosec only
install-gosec:
	@echo "Installing gosec..."
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@echo "✅ gosec installed"

# Install all development tools
install-tools: install-golangci-lint install-staticcheck install-gosec
	@echo "Installing protoc generators..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Installing test tools..."
	@go install gotest.tools/gotestsum@latest
	@echo "\n✅ All tools installed successfully!"

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

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build          - Build the binary"
	@echo "  build-all      - Build for multiple platforms"
	@echo "  run            - Run the application locally"
	@echo "  run-example    - Run with example configuration"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run tests (with race detector)"
	@echo "  test-verbose   - Run tests with full output"
	@echo "  test-coverage  - Generate HTML coverage report"
	@echo "  test-junit     - Run tests with JUnit XML output"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint           - Run golangci-lint"
	@echo "  lint-fix       - Run golangci-lint with auto-fix"
	@echo "  fmt            - Format code with gofmt"
	@echo "  fmt-check      - Check code formatting"
	@echo "  vet            - Run go vet"
	@echo "  staticcheck    - Run staticcheck"
	@echo "  security-check - Run gosec security scanner"
	@echo "  gosec          - Run gosec with detailed report"
	@echo "  quality        - Run all quality checks (fmt, vet, lint, staticcheck)"
	@echo "  quality-check  - Run quality checks + tests (for CI)"
	@echo "  ci-check       - Run all CI checks (quality + security + tests)"
	@echo ""
	@echo "Dependencies:"
	@echo "  tidy           - Tidy go.mod and go.sum"
	@echo "  tidy-check     - Check if dependencies are tidy"
	@echo ""
	@echo "Docker & Helm:"
	@echo "  docker         - Build Docker image"
	@echo "  docker-push    - Push Docker image"
	@echo "  docker-buildx  - Build multi-arch Docker image"
	@echo "  helm-lint      - Lint Helm chart"
	@echo "  helm-package   - Package Helm chart"
	@echo ""
	@echo "Integration Tests:"
	@echo "  integration-test       - Run all integration tests"
	@echo "  integration-test-dns   - Run DNS mode integration tests"
	@echo "  integration-test-grpc  - Run gRPC mode integration tests"
	@echo "  integration-test-junit - Run integration tests with JUnit XML"
	@echo ""
	@echo "Tools:"
	@echo "  install-tools  - Install all development tools"
	@echo "  proto          - Generate protobuf code"
	@echo "  clean          - Clean build artifacts"
	@echo ""

# Run all integration tests using testcontainers-go
integration-test:
	go test -tags=integration -coverprofile=integration-coverage.out -v -timeout 10m ./test/integration/...

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
