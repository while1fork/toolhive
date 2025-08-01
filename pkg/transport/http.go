package transport

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/stacklok/toolhive/pkg/container"
	rt "github.com/stacklok/toolhive/pkg/container/runtime"
	"github.com/stacklok/toolhive/pkg/ignore"
	"github.com/stacklok/toolhive/pkg/logger"
	"github.com/stacklok/toolhive/pkg/permissions"
	"github.com/stacklok/toolhive/pkg/transport/errors"
	"github.com/stacklok/toolhive/pkg/transport/proxy/transparent"
	"github.com/stacklok/toolhive/pkg/transport/types"
)

const (
	// LocalhostName is the standard hostname for localhost
	LocalhostName = "localhost"
	// LocalhostIPv4 is the standard IPv4 address for localhost
	LocalhostIPv4 = "127.0.0.1"
)

// HTTPTransport implements the Transport interface using Server-Sent/Streamable Events.
type HTTPTransport struct {
	transportType     types.TransportType
	host              string
	proxyPort         int
	targetPort        int
	targetHost        string
	containerName     string
	deployer          rt.Deployer
	debug             bool
	middlewares       []types.Middleware
	prometheusHandler http.Handler

	// Mutex for protecting shared state
	mutex sync.Mutex

	// Transparent proxy
	proxy types.Proxy

	// Shutdown channel
	shutdownCh chan struct{}

	// Container monitor
	monitor rt.Monitor
	errorCh <-chan error
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(
	transportType types.TransportType,
	host string,
	proxyPort int,
	targetPort int,
	deployer rt.Deployer,
	debug bool,
	targetHost string,
	prometheusHandler http.Handler,
	middlewares ...types.Middleware,
) *HTTPTransport {
	if host == "" {
		host = LocalhostIPv4
	}

	// If targetHost is not specified, default to localhost
	if targetHost == "" {
		targetHost = LocalhostIPv4
	}

	return &HTTPTransport{
		transportType:     transportType,
		host:              host,
		proxyPort:         proxyPort,
		middlewares:       middlewares,
		targetPort:        targetPort,
		targetHost:        targetHost,
		deployer:          deployer,
		debug:             debug,
		prometheusHandler: prometheusHandler,
		shutdownCh:        make(chan struct{}),
	}
}

// Mode returns the transport mode.
func (t *HTTPTransport) Mode() types.TransportType {
	return t.transportType
}

// ProxyPort returns the proxy port used by the transport.
func (t *HTTPTransport) ProxyPort() int {
	return t.proxyPort
}

var transportEnvMap = map[types.TransportType]string{
	types.TransportTypeSSE:            "sse",
	types.TransportTypeStreamableHTTP: "streamable-http",
}

// Setup prepares the transport for use.
func (t *HTTPTransport) Setup(ctx context.Context, runtime rt.Deployer, containerName string, image string, cmdArgs []string,
	envVars, labels map[string]string, permissionProfile *permissions.Profile, k8sPodTemplatePatch string,
	isolateNetwork bool, ignoreConfig *ignore.Config) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.deployer = runtime
	t.containerName = containerName

	env, ok := transportEnvMap[t.transportType]
	if !ok {
		return fmt.Errorf("unsupported transport type: %s", t.transportType)
	}
	envVars["MCP_TRANSPORT"] = env

	// Use the target port for the container's environment variables
	envVars["MCP_PORT"] = fmt.Sprintf("%d", t.targetPort)
	envVars["FASTMCP_PORT"] = fmt.Sprintf("%d", t.targetPort)
	envVars["MCP_HOST"] = t.targetHost

	// Create workload options
	containerOptions := rt.NewDeployWorkloadOptions()
	containerOptions.K8sPodTemplatePatch = k8sPodTemplatePatch
	containerOptions.IgnoreConfig = ignoreConfig

	// Expose the target port in the container
	containerPortStr := fmt.Sprintf("%d/tcp", t.targetPort)
	containerOptions.ExposedPorts[containerPortStr] = struct{}{}

	// Create host port bindings (configurable through the --host flag)
	portBindings := []rt.PortBinding{
		{
			HostIP:   t.host,
			HostPort: fmt.Sprintf("%d", t.targetPort),
		},
	}

	// Check if IPv6 is available and add IPv6 localhost binding (commented out for now)
	//if networking.IsIPv6Available() {
	//	portBindings = append(portBindings, rt.PortBinding{
	//		HostIP:   "::1", // IPv6 localhost
	//		HostPort: fmt.Sprintf("%d", t.targetPort),
	//	})
	//}

	// Set the port bindings
	containerOptions.PortBindings[containerPortStr] = portBindings

	// For SSE transport, we don't need to attach stdio
	containerOptions.AttachStdio = false

	// Create the container
	logger.Infof("Deploying workload %s from image %s...", containerName, image)
	exposedPort, err := t.deployer.DeployWorkload(
		ctx,
		image,
		containerName,
		cmdArgs,
		envVars,
		labels,
		permissionProfile,
		t.Mode().String(), // Use the transport type as the mode
		containerOptions,
		isolateNetwork,
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}
	logger.Infof("Container created: %s", containerName)

	if (t.Mode() == types.TransportTypeSSE || t.Mode() == types.TransportTypeStreamableHTTP) && rt.IsKubernetesRuntime() {
		// If the SSEHeadlessServiceName is set, use it as the target host
		// This is useful for Kubernetes deployments where the workload is
		// exposed as a headless service.
		if containerOptions.SSEHeadlessServiceName != "" {
			t.targetHost = containerOptions.SSEHeadlessServiceName
		}
	}

	// we don't want to override the targetPort in a Kubernetes deployment. Because
	// by default the Kubernetes container deployer returns `0` for the exposedPort
	// therefore causing the "target port not set" error when it is assigned to the targetPort.
	// Issues:
	// - https://github.com/stacklok/toolhive/issues/902
	// - https://github.com/stacklok/toolhive/issues/924
	if !rt.IsKubernetesRuntime() {
		// also override the exposed port, in case we need it via ingress
		t.targetPort = exposedPort
	}

	return nil
}

// Start initializes the transport and begins processing messages.
// The transport is responsible for starting the container.
func (t *HTTPTransport) Start(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.containerName == "" {
		return errors.ErrContainerNameNotSet
	}

	if t.deployer == nil {
		return fmt.Errorf("container deployer not set")
	}

	// Create and start the transparent proxy
	// The SSE transport forwards requests from the host port to the container's target port
	// In a Docker bridge network, we need to use the specified target host
	// We ignore containerIP even if it's set, as it's not directly accessible from the host
	targetHost := t.targetHost

	// Check if target port is set
	if t.targetPort <= 0 {
		return fmt.Errorf("target port not set for HTTP transport")
	}

	// Use the target port for the container
	containerPort := t.targetPort
	targetURI := fmt.Sprintf("http://%s:%d", targetHost, containerPort)
	logger.Infof("Setting up transparent proxy to forward from host port %d to %s",
		t.proxyPort, targetURI)

	// Create the transparent proxy with middlewares (enable healthcheck for MCP servers)
	t.proxy = transparent.NewTransparentProxy(
		t.host, t.proxyPort, t.containerName, targetURI, t.prometheusHandler, true, t.middlewares...)
	if err := t.proxy.Start(ctx); err != nil {
		return err
	}

	logger.Infof("HTTP transport started for container %s on port %d", t.containerName, t.proxyPort)

	// Create a container monitor
	monitorRuntime, err := container.NewFactory().Create(ctx)
	if err != nil {
		return fmt.Errorf("failed to create container monitor: %v", err)
	}
	t.monitor = container.NewMonitor(monitorRuntime, t.containerName)

	// Start monitoring the container
	t.errorCh, err = t.monitor.StartMonitoring(ctx)
	if err != nil {
		return fmt.Errorf("failed to start container monitoring: %v", err)
	}

	// Start a goroutine to handle container exit
	go t.handleContainerExit(ctx)

	return nil
}

// Stop gracefully shuts down the transport and the container.
func (t *HTTPTransport) Stop(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Signal shutdown
	close(t.shutdownCh)

	// Stop the monitor if it's running
	if t.monitor != nil {
		t.monitor.StopMonitoring()
		t.monitor = nil
	}

	// Stop the transparent proxy
	if t.proxy != nil {
		if err := t.proxy.Stop(ctx); err != nil {
			logger.Warnf("Warning: Failed to stop proxy: %v", err)
		}
	}

	// Stop the container if deployer is available
	if t.deployer != nil && t.containerName != "" {
		if err := t.deployer.StopWorkload(ctx, t.containerName); err != nil {
			return fmt.Errorf("failed to stop workload: %w", err)
		}
	}

	return nil
}

// handleContainerExit handles container exit events.
func (t *HTTPTransport) handleContainerExit(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case err := <-t.errorCh:
		logger.Infof("Container %s exited: %v", t.containerName, err)
		// Stop the transport when the container exits
		if stopErr := t.Stop(ctx); stopErr != nil {
			logger.Errorf("Error stopping transport after container exit: %v", stopErr)
		}
	}
}

// IsRunning checks if the transport is currently running.
func (t *HTTPTransport) IsRunning(_ context.Context) (bool, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Check if the shutdown channel is closed
	select {
	case <-t.shutdownCh:
		return false, nil
	default:
		return true, nil
	}
}
