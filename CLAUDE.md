# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VPSie Kubernetes Node Autoscaler - An intelligent Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform. The autoscaler automatically scales cluster nodes based on workload demands, optimizes costs by selecting appropriate instance types, and continuously rebalances nodes for best price/performance.

**Current Version:** v0.6.0 (Phase 5+ Complete - Cost Optimization, Node Rebalancer, Security Hardening)

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
6. **Metrics** (`pkg/metrics/`) - Prometheus metrics for observability with label sanitization

### Custom Resource Definitions (CRDs)

The autoscaler introduces a `NodeGroup` CRD (`autoscaler.vpsie.io/v1alpha1`) which defines:
- Minimum and maximum node counts
- Target resource requirements (CPU, memory, disk)
- Datacenter/region preferences
- Scaling policies and thresholds

### VPSie API Integration

The client reads OAuth credentials from a Kubernetes secret named `vpsie-secret` in the `kube-system` namespace containing:
- `clientId`: VPSie OAuth client ID
- `clientSecret`: VPSie OAuth client secret
- `url`: VPSie API endpoint (optional, defaults to https://api.vpsie.com/v2)

The client implements:
- OAuth authentication with automatic token refresh (RFC3339 expiry tracking)
- Automatic rate limiting (100 requests/minute by default)
- Circuit breaker for fault tolerance (prevents cascading failures to VPSie API)
- Request retries with exponential backoff
- Proper error handling with typed errors
- Thread-safe credential updates for rotation
- TLS validation (configurable, defaults to enabled)

### Key Package Structure

```
pkg/
├── apis/autoscaler/v1alpha1/  # CRD definitions and validation
│   ├── types.go               # NodeGroup and VPSieNode type definitions
│   └── validation.go          # OpenAPI v3 validation rules
├── controller/                # Kubernetes controllers
│   ├── nodegroup/             # NodeGroup controller (main reconciliation loop)
│   └── vpsienode/             # VPSieNode controller (VPS lifecycle)
├── scaler/                    # Scaling logic
│   ├── scaler.go              # ScaleDownManager for node utilization analysis
│   ├── policy.go              # PolicyEngine for scale-down safety checks
│   └── drain.go               # Node draining and pod eviction
├── rebalancer/                # Cost optimization and node rebalancing
│   ├── analyzer.go            # Identifies rebalance candidates with safety checks
│   ├── planner.go             # Creates migration plans (rolling, surge, blue-green)
│   └── executor.go            # Executes node replacement operations
├── vpsie/                     # VPSie cloud integration
│   ├── client/                # VPSie API v2 client with rate limiting
│   │   ├── client.go          # Main client with K8s secret integration
│   │   ├── types.go           # API request/response types
│   │   └── errors.go          # Custom error types
│   └── cost/                  # Cost calculation and optimization
│       ├── calculator.go      # Offering cost calculation with caching
│       └── optimizer.go       # Right-sizing and savings recommendations
├── metrics/                   # Prometheus metrics collection
│   ├── metrics.go             # Core metrics definitions and registration
│   ├── sanitize.go            # Label sanitization for security (v0.6.0+)
│   └── sanitize_test.go       # Sanitization test suite
├── events/                    # Kubernetes event management
└── webhook/                   # Validation webhooks

cmd/
├── controller/                # Main controller binary with CLI
└── webhook/                   # Webhook server binary
```

### Critical Architecture Notes

1. **Controller Separation of Concerns:**
   - `ScaleDownManager` (pkg/scaler/) identifies and drains nodes
   - `NodeGroupReconciler` (pkg/controller/nodegroup/) handles VPSie VM termination and K8s node deletion
   - This prevents race conditions between draining and VM termination

2. **Rebalancer Safety:**
   - `Analyzer` performs 5 safety checks (cluster health, PDB, local storage, maintenance windows, cooldowns)
   - `Planner` creates strategic migration plans with rollback support
   - `Executor` handles cordon, drain, provision, and rollback operations
   - All operations respect PodDisruptionBudgets

3. **VPSie Client:**
   - Reads OAuth credentials (clientId, clientSecret) from K8s secret `vpsie-secret` in `kube-system` namespace
   - Implements automatic rate limiting (100 req/min default)
   - Circuit breaker for fault tolerance (prevents cascading failures)
   - Automatic token refresh with RFC3339 expiry tracking
   - Thread-safe credential updates for rotation
   - Uses typed errors for better error handling

## Development Guidelines

### Dependencies

The project uses Go 1.24+ and key dependencies include:
- Kubernetes client-go v0.28.4
- controller-runtime v0.16.3
- Prometheus client_golang v1.17.0
- k8s.io/metrics v0.28.4 (for node utilization metrics)
- Cobra for CLI
- zap for structured logging
- golang.org/x/time for rate limiting

### Testing Patterns

When writing tests:
- Unit tests go in `*_test.go` files alongside the code
- Integration tests use build tag `//go:build integration` (or legacy `// +build integration`)
- Performance tests use build tag `//go:build performance`
- E2E tests use build tag `//go:build e2e`
- Use table-driven tests for multiple test cases
- Mock external dependencies (VPSie API, Kubernetes API)

### Running Individual Tests

```bash
# Run a specific test by name
go test ./pkg/scaler -run TestScaleDownManager_IdentifyUnderutilizedNodes -v

# Run tests in a specific package
go test ./pkg/rebalancer/... -v

# Run tests with race detector
go test -race ./pkg/controller/nodegroup -v

# Run a single integration test
go test -tags=integration ./test/integration -run TestNodeGroup_CRUD -v
```

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
  --from-literal=clientId='your-client-id' \
  --from-literal=clientSecret='your-client-secret' \
  -n kube-system
```

Note: The VPSie client uses OAuth authentication with `clientId` and `clientSecret`, not a simple API token.

## Important Code Generation

After modifying CRD types in `pkg/apis/autoscaler/v1alpha1/`:
```bash
# Regenerate DeepCopy methods and CRD manifests
make generate

# Verify generated CRDs
ls -la deploy/crds/
```

The `controller-gen` tool generates:
- `zz_generated.deepcopy.go` - DeepCopy methods for CRD types
- `deploy/crds/*.yaml` - OpenAPI v3 validated CRD manifests

**Note:** If `controller-gen` is not installed:
```bash
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
```

## Recent Breaking Changes (v0.6.0)

### Cloud-Init Removal

Node configuration is now handled entirely by VPSie API via QEMU agent. **All cloud-init related fields have been removed** from the NodeGroup CRD:

**Removed Fields:**
- `spec.userData` (deprecated)
- `spec.cloudInitTemplate`
- `spec.cloudInitTemplateRef`
- `spec.cloudInitVariables`

**Why:** VPSie API handles all node configuration based on `osImageID` and `kubernetesVersion`, making cloud-init unnecessary and removing potential security vulnerabilities from template injection.

### Metrics Label Sanitization (v0.6.0)

All Prometheus metric labels are now automatically sanitized to prevent cardinality explosion and injection attacks:
- Special characters replaced with underscores
- Maximum length enforced (100 characters)
- See `pkg/metrics/sanitize.go` for implementation details
