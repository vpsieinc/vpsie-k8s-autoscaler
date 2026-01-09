# Contributing to VPSie Kubernetes Autoscaler

Thank you for your interest in contributing to the VPSie Kubernetes Autoscaler! This guide will help you get started.

## Table of Contents

- [Development Setup](#development-setup)
- [Code Organization](#code-organization)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Code Style](#code-style)

## Development Setup

### Prerequisites

- Go 1.24 or later
- Docker (for building images)
- kubectl configured with a Kubernetes cluster
- [kind](https://kind.sigs.k8s.io/) (for local development)
- Make

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/vpsie/vpsie-k8s-autoscaler.git
cd vpsie-k8s-autoscaler

# Install dependencies
go mod download

# Build the binary
make build

# Run tests
make test
```

### Local Development with kind

```bash
# Create a local kind cluster
make kind-create

# Build and load Docker image
make docker-build
make kind-load

# Deploy CRDs
kubectl apply -f deploy/crds/

# Create VPSie secret (use test credentials)
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='test-client-id' \
  --from-literal=clientSecret='test-client-secret' \
  -n kube-system

# Run controller locally
make run
```

### IDE Setup

#### VS Code

Recommended extensions:
- Go (golang.go)
- YAML (redhat.vscode-yaml)
- Kubernetes (ms-kubernetes-tools.vscode-kubernetes-tools)

#### GoLand

The project includes standard Go module configuration and should work out of the box.

## Code Organization

```
vpsie-k8s-autoscaler/
├── cmd/
│   ├── controller/          # Main controller binary
│   └── webhook/             # Webhook server binary
├── pkg/
│   ├── apis/autoscaler/v1alpha1/  # CRD types
│   ├── controller/
│   │   ├── nodegroup/       # NodeGroup controller
│   │   └── vpsienode/       # VPSieNode controller
│   ├── scaler/              # Scaling logic
│   ├── rebalancer/          # Cost optimization
│   ├── vpsie/
│   │   ├── client/          # VPSie API client
│   │   └── cost/            # Cost calculation
│   ├── metrics/             # Prometheus metrics
│   ├── audit/               # Audit logging
│   └── webhook/             # Validation webhooks
├── internal/
│   └── logging/             # Internal logging utilities
├── deploy/
│   ├── crds/                # CRD manifests
│   ├── manifests/           # Kubernetes manifests
│   └── helm/                # Helm chart
├── test/
│   ├── e2e/                 # End-to-end tests
│   ├── integration/         # Integration tests
│   └── chaos/               # Chaos engineering tests
└── docs/
    ├── architecture/        # Architecture docs
    ├── runbooks/            # Operational runbooks
    └── operations/          # Operations guides
```

## Making Changes

### Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation only
- `refactor/` - Code refactoring
- `test/` - Test improvements

Example: `feature/add-spot-instance-support`

### Commit Messages

Follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Example:
```
feat(scaler): add support for custom utilization thresholds

- Allow configuring CPU/memory thresholds per NodeGroup
- Add validation for threshold values
- Update documentation

Fixes #123
```

### Code Generation

After modifying CRD types in `pkg/apis/autoscaler/v1alpha1/`:

```bash
# Generate DeepCopy methods and CRD manifests
make generate

# Verify generated files
git diff deploy/crds/
```

## Testing

### Unit Tests

```bash
# Run all unit tests
make test

# Run tests for a specific package
go test -v ./pkg/scaler/...

# Run a specific test
go test -v ./pkg/controller/nodegroup -run TestReconcile

# Run with race detector
go test -race ./pkg/...

# Run with coverage
make coverage
```

### Integration Tests

```bash
# Run integration tests (requires cluster)
make test-integration

# Run specific integration test suite
make test-integration-basic      # CRUD tests
make test-integration-runtime    # Health, metrics
make test-integration-scale      # Scaling tests
```

### End-to-End Tests

```bash
# Run E2E tests
make test-e2e
```

### Writing Tests

- Use table-driven tests for multiple cases
- Mock external dependencies (VPSie API, K8s API)
- Add `//go:build integration` tag for integration tests
- Test error paths, not just happy paths

Example:
```go
func TestScaleDown(t *testing.T) {
    tests := []struct {
        name          string
        currentNodes  int32
        desiredNodes  int32
        wantScaleDown bool
    }{
        {
            name:          "scale down needed",
            currentNodes:  5,
            desiredNodes:  3,
            wantScaleDown: true,
        },
        {
            name:          "no scale down needed",
            currentNodes:  3,
            desiredNodes:  5,
            wantScaleDown: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ng := &v1alpha1.NodeGroup{
                Status: v1alpha1.NodeGroupStatus{
                    CurrentNodes: tt.currentNodes,
                    DesiredNodes: tt.desiredNodes,
                },
            }
            got := NeedsScaleDown(ng)
            if got != tt.wantScaleDown {
                t.Errorf("NeedsScaleDown() = %v, want %v", got, tt.wantScaleDown)
            }
        })
    }
}
```

## Submitting Changes

### Pull Request Process

1. **Fork and create branch**
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make changes and test**
   ```bash
   make test
   make lint
   ```

3. **Commit with clear messages**
   ```bash
   git commit -m "feat(scaler): add feature description"
   ```

4. **Push and create PR**
   ```bash
   git push origin feature/my-feature
   ```

5. **PR Requirements**
   - Describe what changes and why
   - Link related issues
   - Include test results
   - Update documentation if needed

### PR Checklist

- [ ] Tests pass (`make test`)
- [ ] Linting passes (`make lint`)
- [ ] Documentation updated
- [ ] CRD changes regenerated (`make generate`)
- [ ] Commit messages follow convention
- [ ] No sensitive data in commits

## Code Style

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Use `golangci-lint` for linting

```bash
# Format code
make fmt

# Run linter
make lint
```

### Logging

Use structured logging with zap:

```go
logger.Info("scaling node group",
    zap.String("nodeGroup", ng.Name),
    zap.Int32("current", ng.Status.CurrentNodes),
    zap.Int32("desired", ng.Status.DesiredNodes),
)
```

### Error Handling

- Return typed errors when possible
- Wrap errors with context
- Don't log errors at multiple levels

```go
// Good
if err != nil {
    return fmt.Errorf("failed to scale node group %s: %w", ng.Name, err)
}

// Avoid
if err != nil {
    logger.Error("failed", zap.Error(err))
    return err // Double logging when caller also logs
}
```

### Metrics

- Use `pkg/metrics/sanitize.go` for label sanitization
- Follow Prometheus naming conventions
- Add help text for all metrics

```go
myMetric = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "vpsie_autoscaler",
        Name:      "my_operation_total",
        Help:      "Total number of my operations",
    },
    []string{"nodegroup", "result"},
)
```

## Getting Help

- Open an issue for bugs or feature requests
- Join discussions in GitHub Discussions
- Review existing documentation in `/docs`

## License

By contributing, you agree that your contributions will be licensed under the project's license.
