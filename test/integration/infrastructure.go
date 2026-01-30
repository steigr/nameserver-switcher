// Package integration provides integration testing infrastructure using testcontainers-go.
package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMode defines the mode of integration testing.
type TestMode string

const (
	// DNSMode tests with CoreDNS using the forward plugin (DNS forwarding).
	DNSMode TestMode = "dns"
	// GRPCMode tests with CoreDNS using the grpc plugin (gRPC forwarding).
	GRPCMode TestMode = "grpc"
)

// Infrastructure holds all the test containers.
type Infrastructure struct {
	Mode             TestMode
	Network          *testcontainers.DockerNetwork
	DNSMasqSystem    testcontainers.Container
	DNSMasqExplicit  testcontainers.Container
	NameserverSwitch testcontainers.Container
	CoreDNS          testcontainers.Container

	// Additional DNS servers for testing different resolver scenarios
	DNSMasqPassthrough     testcontainers.Container
	DNSMasqNoCnameResponse testcontainers.Container
	DNSMasqNoCnameMatch    testcontainers.Container

	// Resolved addresses
	DNSMasqSystemAddr          string
	DNSMasqExplicitAddr        string
	DNSMasqPassthroughAddr     string
	DNSMasqNoCnameResponseAddr string
	DNSMasqNoCnameMatchAddr    string
	NameserverSwitchAddr       string
	CoreDNSAddr                string
}

// projectRoot returns the project root directory.
func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// Setup creates and starts all test containers for the given mode.
func Setup(ctx context.Context, mode TestMode) (*Infrastructure, error) {
	infra := &Infrastructure{Mode: mode}

	// Create network
	net, err := network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	infra.Network = net

	networkName := net.Name

	// Start dnsmasq-system
	infra.DNSMasqSystem, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-system"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// foo.example.com CNAME to bar-match.example.com
				"--cname=foo.example.com,bar-match.example.com",
				// hello.example.com CNAME to bar-nomatch.example.com
				"--cname=hello.example.com,bar-nomatch.example.com",
				// bar-match.example.com returns 127.0.0.99 on system resolver (to prove routing)
				"--address=/bar-match.example.com/127.0.0.99",
				// bar-nomatch.example.com returns 127.0.0.3
				"--address=/bar-nomatch.example.com/127.0.0.3",
				// direct.example.com has NO CNAME, direct A record - returns 127.0.0.4 on system
				"--address=/direct.example.com/127.0.0.4",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-system: %w", err)
	}

	// Get dnsmasq-system internal IP
	dnsmasqSystemIP, err := infra.DNSMasqSystem.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-system IP: %w", err)
	}
	infra.DNSMasqSystemAddr = dnsmasqSystemIP + ":53"

	// Start dnsmasq-explicit
	infra.DNSMasqExplicit, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-explicit"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// foo.example.com CNAME to bar-match.example.com
				"--cname=foo.example.com,bar-match.example.com",
				// bar-match.example.com returns 127.0.0.2 on explicit resolver
				"--address=/bar-match.example.com/127.0.0.2",
				// direct.example.com has NO CNAME, direct A record - returns 127.0.0.5 on explicit
				// If system resolver is used correctly, we should get 127.0.0.4 (not 127.0.0.5)
				"--address=/direct.example.com/127.0.0.5",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-explicit: %w", err)
	}

	// Get dnsmasq-explicit internal IP
	dnsmasqExplicitIP, err := infra.DNSMasqExplicit.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-explicit IP: %w", err)
	}
	infra.DNSMasqExplicitAddr = dnsmasqExplicitIP + ":53"

	// Build and start nameserver-switcher
	infra.NameserverSwitch, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    projectRoot(),
				Dockerfile: "Dockerfile",
			},
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"nameserver-switcher"},
			},
			Env: map[string]string{
				"REQUEST_PATTERNS": `.*\.example\.com$`,
				"CNAME_PATTERNS":   `.*-match\.example\.com$`,
				// Use DNS hostnames so container restarts work
				"REQUEST_RESOLVER":  "dnsmasq-system:53",
				"EXPLICIT_RESOLVER": "dnsmasq-explicit:53",
				"DNS_PORT":          "5353",
				"GRPC_PORT":         "5354",
				"HTTP_PORT":         "8080",
			},
			ExposedPorts: []string{"5353/udp", "5353/tcp", "5354/tcp", "8080/tcp"},
			WaitingFor:   wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start nameserver-switcher: %w", err)
	}

	// Get nameserver-switcher internal IP
	nsSwitchIP, err := infra.NameserverSwitch.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get nameserver-switcher IP: %w", err)
	}
	infra.NameserverSwitchAddr = nsSwitchIP

	// Create CoreDNS Corefile based on mode
	var corefile string
	switch mode {
	case DNSMode:
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    forward . %s:5353 {
        max_fails 0
    }
}
`, nsSwitchIP)
	case GRPCMode:
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    grpc . %s:5354
}
`, nsSwitchIP)
	}

	// Start CoreDNS
	infra.CoreDNS, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "coredns/coredns:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"coredns"},
			},
			Cmd: []string{"-conf", "/etc/coredns/Corefile"},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            stringReader(corefile),
					ContainerFilePath: "/etc/coredns/Corefile",
					FileMode:          0644,
				},
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("CoreDNS").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start CoreDNS: %w", err)
	}

	// Get CoreDNS internal IP
	corednsIP, err := infra.CoreDNS.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get CoreDNS IP: %w", err)
	}
	infra.CoreDNSAddr = corednsIP

	return infra, nil
}

// SetupWithSeparateResolvers creates infrastructure with separate DNS servers for each resolver type.
// This allows testing that each fallback scenario uses the correct resolver.
func SetupWithSeparateResolvers(ctx context.Context, mode TestMode) (*Infrastructure, error) {
	infra := &Infrastructure{Mode: mode}

	// Create network
	net, err := network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	infra.Network = net

	networkName := net.Name

	// Start dnsmasq-passthrough (for unmatched requests)
	// Returns unique IP 127.0.0.10 for unmatched.example.org
	infra.DNSMasqPassthrough, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-passthrough"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// Return unique IP for passthrough
				"--address=/unmatched.example.org/127.0.0.10",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-passthrough: %w", err)
	}

	dnsmasqPassthroughIP, err := infra.DNSMasqPassthrough.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-passthrough IP: %w", err)
	}
	infra.DNSMasqPassthroughAddr = dnsmasqPassthroughIP + ":53"

	// Start dnsmasq-no-cname-response (for requests without CNAME in response)
	// Returns unique IP 127.0.0.20 for direct.example.com
	infra.DNSMasqNoCnameResponse, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-no-cname-response"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// Return unique IP for no-cname-response
				"--address=/direct.example.com/127.0.0.20",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-no-cname-response: %w", err)
	}

	dnsmasqNoCnameResponseIP, err := infra.DNSMasqNoCnameResponse.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-no-cname-response IP: %w", err)
	}
	infra.DNSMasqNoCnameResponseAddr = dnsmasqNoCnameResponseIP + ":53"

	// Start dnsmasq-no-cname-match (for CNAME responses that don't match pattern)
	// Returns unique IP 127.0.0.30 for hello.example.com
	infra.DNSMasqNoCnameMatch, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-no-cname-match"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// Return unique IP for no-cname-match
				"--address=/hello.example.com/127.0.0.30",
				"--address=/bar-nomatch.example.com/127.0.0.31",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-no-cname-match: %w", err)
	}

	dnsmasqNoCnameMatchIP, err := infra.DNSMasqNoCnameMatch.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-no-cname-match IP: %w", err)
	}
	infra.DNSMasqNoCnameMatchAddr = dnsmasqNoCnameMatchIP + ":53"

	// Start dnsmasq-explicit (for matched CNAME patterns)
	infra.DNSMasqExplicit, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-explicit"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				// foo.example.com CNAME to bar-match.example.com (matches CNAME pattern)
				"--cname=foo.example.com,bar-match.example.com",
				// hello.example.com CNAME to bar-nomatch.example.com (doesn't match CNAME pattern)
				"--cname=hello.example.com,bar-nomatch.example.com",
				// direct.example.com has NO CNAME, direct A record
				"--address=/direct.example.com/127.0.0.5",
				// bar-match.example.com returns 127.0.0.2 on explicit resolver
				"--address=/bar-match.example.com/127.0.0.2",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-explicit: %w", err)
	}

	dnsmasqExplicitIP, err := infra.DNSMasqExplicit.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-explicit IP: %w", err)
	}
	infra.DNSMasqExplicitAddr = dnsmasqExplicitIP + ":53"

	// Build and start nameserver-switcher with separate resolvers
	infra.NameserverSwitch, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    projectRoot(),
				Dockerfile: "Dockerfile",
			},
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"nameserver-switcher"},
			},
			Env: map[string]string{
				"REQUEST_PATTERNS": `.*\.example\.com$`,
				"CNAME_PATTERNS":   `.*-match\.example\.com$`,
				// Use separate resolvers for each scenario
				"EXPLICIT_RESOLVER":          "dnsmasq-explicit:53",
				"PASSTHROUGH_RESOLVER":       "dnsmasq-passthrough:53",
				"NO_CNAME_RESPONSE_RESOLVER": "dnsmasq-no-cname-response:53",
				"NO_CNAME_MATCH_RESOLVER":    "dnsmasq-no-cname-match:53",
				"DNS_PORT":                   "5353",
				"GRPC_PORT":                  "5354",
				"HTTP_PORT":                  "8080",
			},
			ExposedPorts: []string{"5353/udp", "5353/tcp", "5354/tcp", "8080/tcp"},
			WaitingFor:   wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start nameserver-switcher: %w", err)
	}

	nsSwitchIP, err := infra.NameserverSwitch.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get nameserver-switcher IP: %w", err)
	}
	infra.NameserverSwitchAddr = nsSwitchIP

	// Create CoreDNS Corefile based on mode
	var corefile string
	switch mode {
	case DNSMode:
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    forward . %s:5353 {
        max_fails 0
    }
}
`, nsSwitchIP)
	case GRPCMode:
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    grpc . %s:5354
}
`, nsSwitchIP)
	}

	// Start CoreDNS
	infra.CoreDNS, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "coredns/coredns:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"coredns"},
			},
			Cmd: []string{"-conf", "/etc/coredns/Corefile"},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            stringReader(corefile),
					ContainerFilePath: "/etc/coredns/Corefile",
					FileMode:          0644,
				},
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("CoreDNS").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start CoreDNS: %w", err)
	}

	corednsIPSep, err := infra.CoreDNS.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get CoreDNS IP: %w", err)
	}
	infra.CoreDNSAddr = corednsIPSep

	return infra, nil
}

// GetDNSMasqPassthroughInternalAddr returns the internal network address for dnsmasq-passthrough.
func (i *Infrastructure) GetDNSMasqPassthroughInternalAddr() string {
	return i.DNSMasqPassthroughAddr
}

// GetDNSMasqNoCnameResponseInternalAddr returns the internal network address for dnsmasq-no-cname-response.
func (i *Infrastructure) GetDNSMasqNoCnameResponseInternalAddr() string {
	return i.DNSMasqNoCnameResponseAddr
}

// GetDNSMasqNoCnameMatchInternalAddr returns the internal network address for dnsmasq-no-cname-match.
func (i *Infrastructure) GetDNSMasqNoCnameMatchInternalAddr() string {
	return i.DNSMasqNoCnameMatchAddr
}

// Teardown stops and removes all test containers.
func (i *Infrastructure) Teardown(ctx context.Context) {
	if i.CoreDNS != nil {
		_ = i.CoreDNS.Terminate(ctx)
	}
	if i.NameserverSwitch != nil {
		_ = i.NameserverSwitch.Terminate(ctx)
	}
	if i.DNSMasqExplicit != nil {
		_ = i.DNSMasqExplicit.Terminate(ctx)
	}
	if i.DNSMasqSystem != nil {
		_ = i.DNSMasqSystem.Terminate(ctx)
	}
	if i.DNSMasqPassthrough != nil {
		_ = i.DNSMasqPassthrough.Terminate(ctx)
	}
	if i.DNSMasqNoCnameResponse != nil {
		_ = i.DNSMasqNoCnameResponse.Terminate(ctx)
	}
	if i.DNSMasqNoCnameMatch != nil {
		_ = i.DNSMasqNoCnameMatch.Terminate(ctx)
	}
	if i.Network != nil {
		_ = i.Network.Remove(ctx)
	}
}

// GetCoreDNSHostPort returns the host:port for CoreDNS (for external access).
func (i *Infrastructure) GetCoreDNSHostPort(ctx context.Context) (string, error) {
	host, err := i.CoreDNS.Host(ctx)
	if err != nil {
		return "", err
	}
	port, err := i.CoreDNS.MappedPort(ctx, "53/udp")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", host, port.Port()), nil
}

// GetNameserverSwitchDNSHostPort returns the host:port for nameserver-switcher DNS (for external access).
func (i *Infrastructure) GetNameserverSwitchDNSHostPort(ctx context.Context) (string, error) {
	host, err := i.NameserverSwitch.Host(ctx)
	if err != nil {
		return "", err
	}
	port, err := i.NameserverSwitch.MappedPort(ctx, "5353/udp")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", host, port.Port()), nil
}

// GetDNSMasqSystemInternalAddr returns the internal network address for dnsmasq-system.
func (i *Infrastructure) GetDNSMasqSystemInternalAddr() string {
	return i.DNSMasqSystemAddr
}

// GetDNSMasqExplicitInternalAddr returns the internal network address for dnsmasq-explicit.
func (i *Infrastructure) GetDNSMasqExplicitInternalAddr() string {
	return i.DNSMasqExplicitAddr
}

// RunDNSQuery runs a DNS query from within the Docker network using a helper container.
func (i *Infrastructure) RunDNSQuery(ctx context.Context, serverAddr, domain string) (string, error) {
	networkName := i.Network.Name

	// Parse server address to separate host and port
	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil {
		// If no port, assume 53
		host = serverAddr
		port = "53"
	}

	// Build dig command with proper syntax: dig @server -p port domain
	digCmd := fmt.Sprintf("apk add --no-cache bind-tools > /dev/null 2>&1 && dig @%s -p %s %s A +short", host, port, domain)

	// Use alpine with bind-tools for dig
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      "alpine:latest",
			Networks:   []string{networkName},
			Cmd:        []string{"sh", "-c", digCmd},
			WaitingFor: wait.ForExit().WithExitTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create query container: %w", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	// Get the logs (output)
	logs, err := container.Logs(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer func() { _ = logs.Close() }()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, logs)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}

// stringReader creates an io.Reader from a string.
func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

// RestartDNSMasqSystemWithConfig stops and restarts dnsmasq-system with new configuration.
func (i *Infrastructure) RestartDNSMasqSystemWithConfig(ctx context.Context, cmd []string) error {
	// Stop the existing container
	if err := i.DNSMasqSystem.Stop(ctx, nil); err != nil {
		return fmt.Errorf("failed to stop dnsmasq-system: %w", err)
	}

	// Terminate the old container
	if err := i.DNSMasqSystem.Terminate(ctx); err != nil {
		return fmt.Errorf("failed to terminate dnsmasq-system: %w", err)
	}

	networkName := i.Network.Name

	// Start a new dnsmasq-system with the new configuration
	var err error
	i.DNSMasqSystem, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-system"},
			},
			Cmd:          cmd,
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return fmt.Errorf("failed to start new dnsmasq-system: %w", err)
	}

	// Update the internal address
	dnsmasqSystemIP, err := i.DNSMasqSystem.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get dnsmasq-system IP: %w", err)
	}
	i.DNSMasqSystemAddr = dnsmasqSystemIP + ":53"

	return nil
}

// StopNameserverSwitcher stops the nameserver-switcher container.
func (i *Infrastructure) StopNameserverSwitcher(ctx context.Context) error {
	if err := i.NameserverSwitch.Stop(ctx, nil); err != nil {
		return fmt.Errorf("failed to stop nameserver-switcher: %w", err)
	}
	return nil
}

// StopDNSMasqExplicit stops the dnsmasq-explicit container.
func (i *Infrastructure) StopDNSMasqExplicit(ctx context.Context) error {
	if err := i.DNSMasqExplicit.Stop(ctx, nil); err != nil {
		return fmt.Errorf("failed to stop dnsmasq-explicit: %w", err)
	}
	return nil
}

// SetupWithFallback creates infrastructure with CoreDNS configured to fallthrough to system resolver.
func SetupWithFallback(ctx context.Context, mode TestMode) (*Infrastructure, error) {
	infra := &Infrastructure{Mode: mode}

	// Create network
	net, err := network.New(ctx, network.WithDriver("bridge"))
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	infra.Network = net

	networkName := net.Name

	// Start dnsmasq-system (will be used as fallback)
	infra.DNSMasqSystem, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-system"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				"--cname=foo.example.com,bar-match.example.com",
				"--cname=hello.example.com,bar-nomatch.example.com",
				"--address=/bar-match.example.com/127.0.0.99",
				"--address=/bar-nomatch.example.com/127.0.0.3",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-system: %w", err)
	}

	dnsmasqSystemIP, err := infra.DNSMasqSystem.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-system IP: %w", err)
	}
	infra.DNSMasqSystemAddr = dnsmasqSystemIP + ":53"

	// Start dnsmasq-explicit
	infra.DNSMasqExplicit, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "4km3/dnsmasq:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"dnsmasq-explicit"},
			},
			Cmd: []string{
				"--keep-in-foreground",
				"--log-facility=-",
				"--no-resolv",
				"--no-poll",
				"--cname=foo.example.com,bar-match.example.com",
				"--address=/bar-match.example.com/127.0.0.2",
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("started").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start dnsmasq-explicit: %w", err)
	}

	dnsmasqExplicitIP, err := infra.DNSMasqExplicit.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get dnsmasq-explicit IP: %w", err)
	}
	infra.DNSMasqExplicitAddr = dnsmasqExplicitIP + ":53"

	// Build and start nameserver-switcher
	infra.NameserverSwitch, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    projectRoot(),
				Dockerfile: "Dockerfile",
			},
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"nameserver-switcher"},
			},
			Env: map[string]string{
				"REQUEST_PATTERNS": `.*\.example\.com$`,
				"CNAME_PATTERNS":   `.*-match\.example\.com$`,
				// Use DNS hostnames so container restarts work
				"REQUEST_RESOLVER":  "dnsmasq-system:53",
				"EXPLICIT_RESOLVER": "dnsmasq-explicit:53",
				"DNS_PORT":          "5353",
				"GRPC_PORT":         "5354",
				"HTTP_PORT":         "8080",
			},
			ExposedPorts: []string{"5353/udp", "5353/tcp", "5354/tcp", "8080/tcp"},
			WaitingFor:   wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start nameserver-switcher: %w", err)
	}

	nsSwitchIP, err := infra.NameserverSwitch.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get nameserver-switcher IP: %w", err)
	}
	infra.NameserverSwitchAddr = nsSwitchIP

	// Create CoreDNS Corefile with fallthrough to system resolver
	var corefile string
	switch mode {
	case DNSMode:
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    forward . %s:5353 %s {
        max_fails 0
        policy sequential
    }
}
`, nsSwitchIP, dnsmasqSystemIP)
	case GRPCMode:
		// gRPC plugin with fallback forward
		corefile = fmt.Sprintf(`.:53 {
    log
    errors
    grpc . %s:5354
    forward . %s {
        max_fails 0
    }
}
`, nsSwitchIP, dnsmasqSystemIP)
	}

	// Start CoreDNS
	infra.CoreDNS, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "coredns/coredns:latest",
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"coredns"},
			},
			Cmd: []string{"-conf", "/etc/coredns/Corefile"},
			Files: []testcontainers.ContainerFile{
				{
					Reader:            stringReader(corefile),
					ContainerFilePath: "/etc/coredns/Corefile",
					FileMode:          0644,
				},
			},
			ExposedPorts: []string{"53/udp", "53/tcp"},
			WaitingFor:   wait.ForLog("CoreDNS").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to start CoreDNS: %w", err)
	}

	corednsIP, err := infra.CoreDNS.ContainerIP(ctx)
	if err != nil {
		infra.Teardown(ctx)
		return nil, fmt.Errorf("failed to get CoreDNS IP: %w", err)
	}
	infra.CoreDNSAddr = corednsIP

	return infra, nil
}
