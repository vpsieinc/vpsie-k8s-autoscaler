# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VPSie Kubernetes Node Autoscaler - An intelligent Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform. The autoscaler automatically scales cluster nodes based on workload demands, optimizes costs by selecting appropriate instance types, and continuously rebalances nodes for best price/performance.

## Build and Development Commands

### Building
```bash
# Build the controller binary
make build

# Clean build artifacts
make clean

# Build and test
make all
```

### Testing
```bash
# Run unit tests
make test

# Run unit tests with coverage report
make coverage

# Run integration tests (requires cluster)
make test-integration

# Run specific integration test suites
make test-integration-basic          # CRUD tests only (fast)
make test-integration-runtime        # Health, metrics, reconciliation
make test-integration-shutdown       # Graceful shutdown tests
make test-integration-leader         # Leader election tests
make test-integration-scale          # Scaling tests
make test-integration-all            # Complete integration suite

# Run performance tests (requires cluster)
make test-integration-performance    # Load tests (100 NodeGroups, high churn, large scale)
make test-performance-benchmarks     # Benchmarks only

# Run integration tests with coverage
make test-coverage-integration

# Run end-to-end tests (requires cluster)
make test-e2e

# Run linters
make lint

# Format code
make fmt

# Run go vet
make vet
```

### Code Generation
```bash
# Generate CRDs, clients, and mocks
make generate

# Generate Kubernetes manifests only
make manifests
```

### Local Development with kind
```bash
# Create a local kind cluster
make kind-create

# Delete the kind cluster
make kind-delete

# Load Docker image into kind
make kind-load

# Deploy to local cluster
make deploy

# Remove from cluster
make undeploy
```

### Running Locally
```bash
# Run controller locally (connects to current kubectl context)
make run
```

### Docker
```bash
# Build Docker image
make docker-build

# Push Docker image
make docker-push
```

### Helm
```bash
# Package Helm chart
make helm-package

# Install via Helm
make helm-install

# Upgrade Helm release
make helm-upgrade

# Uninstall Helm release
make helm-uninstall
```

## Architecture

### High-Level Components

1. **Controller** (`cmd/controller/`) - Main Kubernetes controller that reconciles NodeGroup resources
2. **API Client** (`pkg/vpsie/client/`) - VPSie API v2 client with rate limiting and Kubernetes secret integration
3. **Scaler** (`pkg/scaler/`) - Core scaling logic that determines when to scale up/down
4. **Rebalancer** (`pkg/rebalancer/`) - Node rebalancing logic for cost optimization
5. **Cost Calculator** (`pkg/vpsie/cost/`) - Analyzes and compares VPSie instance type costs
6. **Metrics** (`pkg/metrics/`) - Prometheus metrics for observability

### Custom Resource Definitions (CRDs)

The autoscaler introduces a `NodeGroup` CRD (`autoscaler.vpsie.io/v1alpha1`) which defines:
- Minimum and maximum node counts
- Target resource requirements (CPU, memory, disk)
- Datacenter/region preferences
- Scaling policies and thresholds

### VPSie API Integration

The client reads credentials from a Kubernetes secret named `vpsie-secret` in the `kube-system` namespace containing:
- `token`: Base64-encoded VPSie API key
- `url`: Base64-encoded VPSie API endpoint (optional, defaults to https://api.vpsie.com/v2)

The client implements:
- Automatic rate limiting (100 requests/minute by default)
- Request retries with exponential backoff
- Proper error handling with typed errors
- Thread-safe credential updates for rotation

### Key Package Structure

- `pkg/vpsie/client/` - VPSie API client implementation
  - `client.go` - Main client with Kubernetes secret integration and rate limiting
  - `types.go` - API request/response types for VPSie API v2
  - `errors.go` - Custom error types (APIError, SecretError, ConfigError)

- `pkg/apis/autoscaler/v1alpha1/` - NodeGroup CRD types and validation

- `pkg/controller/nodegroup/` - NodeGroup controller reconciliation logic

- `internal/config/` - Configuration management
- `internal/logging/` - Structured logging utilities

## Development Guidelines

### Dependencies

The project uses Go 1.25.2 and key dependencies include:
- Kubernetes client-go v0.28.0
- controller-runtime v0.16.0
- Prometheus client
- Cobra for CLI
- Viper for configuration

### Testing Patterns

When writing tests:
- Unit tests go in `*_test.go` files alongside the code
- Integration tests use build tag `// +build integration`
- E2E tests use build tag `// +build e2e`
- Use table-driven tests for multiple test cases
- Mock external dependencies (VPSie API, Kubernetes API)

### VPSie API Client Usage

```go
// Create client from Kubernetes secret
client, err := client.NewClient(ctx, clientset, &client.ClientOptions{
    SecretName: "vpsie-secret",
    SecretNamespace: "kube-system",
    RateLimit: 100, // requests per minute
})

// Or create client with explicit credentials (for testing)
client, err := client.NewClientWithCredentials(
    "https://api.vpsie.com/v2",
    "your-api-token",
    nil,
)
```

### Error Handling

The VPSie client uses typed errors:
```go
if err != nil {
    if client.IsNotFound(err) {
        // Handle 404
    } else if client.IsRateLimited(err) {
        // Handle 429
    } else if apiErr, ok := err.(*client.APIError); ok {
        // Handle other API errors
        log.Error(apiErr.RequestID, apiErr.Message)
    }
}
```

### Logging

Use structured logging with fields:
```go
log.Info("scaling node group",
    "nodeGroup", nodeGroup.Name,
    "currentNodes", currentCount,
    "desiredNodes", desiredCount,
)
```

## Configuration

Configuration is managed through:
1. `config/config.example.yaml` - Example configuration file
2. Environment variables
3. Kubernetes secrets for sensitive data (VPSie credentials)

## Deployment

The autoscaler can be deployed via:
1. **Helm** (recommended) - `make helm-install`
2. **kubectl** - `kubectl apply -f deploy/manifests/`
3. **Local development** - `make run` (connects to current context)

Required Kubernetes secret before deployment:
```bash
kubectl create secret generic vpsie-secret \
  --from-literal=token='your-vpsie-api-token' \
  --from-literal=url='https://api.vpsie.com/v2' \
  -n kube-system
```
