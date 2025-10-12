# Development Guide

## Prerequisites

- **Go 1.22+** - Programming language
- **kubectl** - Kubernetes command-line tool
- **kind** or **minikube** - Local Kubernetes cluster (for testing)
- **controller-gen** - CRD and code generation tool
- **golangci-lint** - Go linter

## Setting Up Development Environment

### 1. Clone the Repository

```bash
git clone https://github.com/vpsie/vpsie-k8s-autoscaler.git
cd vpsie-k8s-autoscaler
```

### 2. Install Dependencies

```bash
# Install Go dependencies
go mod download

# Install development tools
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 3. Verify Installation

```bash
# Run tests
go test ./...

# Run linters
golangci-lint run

# Generate CRD manifests
make generate
```

## Project Structure

```
vpsie-k8s-autoscaler/
├── cmd/
│   └── controller/              # Controller binary entry point
│       └── main.go
├── pkg/
│   ├── apis/
│   │   └── autoscaler/v1alpha1/ # CRD type definitions
│   │       ├── doc.go           # Package documentation
│   │       ├── groupversion_info.go
│   │       ├── nodegroup_types.go
│   │       ├── vpsienode_types.go
│   │       ├── *_test.go        # Unit tests
│   │       └── zz_generated.deepcopy.go
│   ├── controller/              # Controller implementations (planned)
│   │   ├── manager.go
│   │   ├── nodegroup/
│   │   └── vpsienode/
│   ├── vpsie/
│   │   └── client/              # VPSie API client
│   │       ├── client.go
│   │       ├── types.go
│   │       ├── errors.go
│   │       └── client_test.go
│   └── log/                     # Logging utilities
│       └── logger.go
├── deploy/
│   ├── crds/                    # CRD manifests
│   │   ├── autoscaler.vpsie.com_nodegroups.yaml
│   │   └── autoscaler.vpsie.com_vpsienodes.yaml
│   └── examples/                # Example configurations
│       ├── nodegroup-*.yaml
│       └── vpsienode-example.yaml
├── docs/                        # Documentation
│   ├── PRD.md
│   ├── DEVELOPMENT.md
│   └── API.md
├── test/                        # Integration and E2E tests (planned)
│   ├── integration/
│   └── e2e/
├── Makefile                     # Build and development commands
├── go.mod                       # Go module definition
└── README.md
```

## Development Workflow

### Making Changes to CRD Types

1. **Edit type definitions** in `pkg/apis/autoscaler/v1alpha1/*.go`
2. **Add kubebuilder markers** for validation, defaults, etc.
3. **Regenerate code and manifests:**
   ```bash
   make generate
   ```
4. **Update tests** to cover new fields/validation
5. **Run tests:**
   ```bash
   go test ./pkg/apis/autoscaler/v1alpha1/...
   ```

### Adding New Controller Logic

1. **Create controller package** under `pkg/controller/`
2. **Implement reconciliation logic** following controller-runtime patterns
3. **Write unit tests** with fake clients
4. **Test with local cluster:**
   ```bash
   # Start local cluster
   kind create cluster --name vpsie-autoscaler

   # Install CRDs
   kubectl apply -f deploy/crds/

   # Build and run controller locally
   make build
   ./bin/vpsie-autoscaler --kubeconfig ~/.kube/config
   ```

### Testing Strategy

#### Unit Tests
- Test individual functions and methods
- Mock external dependencies (Kubernetes API, VPSie API)
- Fast execution (<1 second per test)

**Example:**
```go
func TestNodeGroupValidation(t *testing.T) {
    ng := &NodeGroup{
        Spec: NodeGroupSpec{
            MinNodes: 2,
            MaxNodes: 10,
        },
    }
    // Test validation logic
}
```

#### Integration Tests
- Test multiple components together
- Use fake Kubernetes API server (envtest)
- Test reconciliation loops

**Example:**
```go
func TestNodeGroupController(t *testing.T) {
    // Create fake Kubernetes client
    scheme := runtime.NewScheme()
    v1alpha1.AddToScheme(scheme)
    client := fake.NewClientBuilder().WithScheme(scheme).Build()

    // Create controller with fake client
    reconciler := &NodeGroupReconciler{
        Client: client,
        Scheme: scheme,
    }

    // Test reconciliation
    req := reconcile.Request{NamespacedName: types.NamespacedName{
        Namespace: "kube-system",
        Name:      "test-nodegroup",
    }}
    result, err := reconciler.Reconcile(context.Background(), req)
    // Assert expected behavior
}
```

#### E2E Tests (Planned)
- Test complete system behavior
- Use real Kubernetes cluster (kind)
- Mock VPSie API for reproducibility

### Code Generation

The project uses code generation for:

1. **DeepCopy methods** (required for Kubernetes API types)
2. **CRD manifests** (OpenAPI v3 schemas)

**Generate all:**
```bash
make generate
```

**Manual generation:**
```bash
# Generate DeepCopy methods
controller-gen object paths="./pkg/apis/autoscaler/v1alpha1/..."

# Generate CRD manifests
controller-gen crd paths="./pkg/apis/autoscaler/v1alpha1/..." \
  output:crd:dir=./deploy/crds
```

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test ./pkg/vpsie/client/...

# Specific test
go test -v -run TestNodeGroup_Creation ./pkg/apis/autoscaler/v1alpha1/
```

### Linting

```bash
# Run all linters
golangci-lint run

# Auto-fix issues
golangci-lint run --fix

# Specific linters
golangci-lint run --disable-all --enable=govet,errcheck,staticcheck
```

### Pre-Commit Hooks

The project uses git hooks for code quality:

**.git/hooks/pre-commit:**
```bash
#!/bin/bash
set -e

echo "Running pre-commit hooks..."

# Format code
echo "Formatting code..."
goimports -w $(find . -name "*.go" -not -path "./vendor/*")

# Run linters
echo "Running linters..."
golangci-lint run

# Run tests
echo "Running tests..."
go test ./...

echo "Pre-commit checks passed!"
```

### Building

```bash
# Build controller binary
make build

# Build with version information
VERSION=v0.1.0 COMMIT=$(git rev-parse HEAD) DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  make build

# Cross-compile
GOOS=linux GOARCH=amd64 make build
```

### Debugging

#### Local Debugging

1. **Run controller locally:**
   ```bash
   go run cmd/controller/main.go \
     --kubeconfig ~/.kube/config \
     --log-level debug
   ```

2. **Use VS Code debugger:**
   ```json
   {
     "version": "0.2.0",
     "configurations": [
       {
         "name": "Debug Controller",
         "type": "go",
         "request": "launch",
         "mode": "debug",
         "program": "${workspaceFolder}/cmd/controller",
         "args": [
           "--kubeconfig", "${env:HOME}/.kube/config",
           "--log-level", "debug"
         ]
       }
     ]
   }
   ```

#### Debugging in Cluster

1. **Enable verbose logging:**
   ```bash
   kubectl set env deployment/vpsie-autoscaler LOG_LEVEL=debug -n kube-system
   ```

2. **View logs:**
   ```bash
   kubectl logs -f deployment/vpsie-autoscaler -n kube-system
   ```

3. **Debug with port-forward:**
   ```bash
   # Forward health/metrics ports
   kubectl port-forward deployment/vpsie-autoscaler 8080:8080 8081:8081

   # Access metrics
   curl localhost:8080/metrics

   # Check health
   curl localhost:8081/healthz
   ```

## Common Tasks

### Adding a New Field to NodeGroup

1. **Update type definition:**
   ```go
   // pkg/apis/autoscaler/v1alpha1/nodegroup_types.go
   type NodeGroupSpec struct {
       // ... existing fields ...

       // NewField is a description of the new field
       // +kubebuilder:validation:Optional
       // +kubebuilder:default="default-value"
       NewField string `json:"newField,omitempty"`
   }
   ```

2. **Regenerate code:**
   ```bash
   make generate
   ```

3. **Update tests:**
   ```go
   func TestNodeGroup_NewField(t *testing.T) {
       ng := &NodeGroup{
           Spec: NodeGroupSpec{
               NewField: "test-value",
           },
       }
       assert.Equal(t, "test-value", ng.Spec.NewField)
   }
   ```

4. **Update examples:**
   ```yaml
   # deploy/examples/nodegroup-example.yaml
   spec:
     newField: "example-value"
   ```

### Adding a New VPSie API Method

1. **Add method to client:**
   ```go
   // pkg/vpsie/client/client.go
   func (c *Client) NewMethod(ctx context.Context, params SomeParams) (*Result, error) {
       // Implementation
   }
   ```

2. **Add types if needed:**
   ```go
   // pkg/vpsie/client/types.go
   type SomeParams struct {
       Field1 string `json:"field1"`
       Field2 int    `json:"field2"`
   }
   ```

3. **Write tests:**
   ```go
   // pkg/vpsie/client/client_test.go
   func TestClient_NewMethod(t *testing.T) {
       server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
           // Mock response
           json.NewEncoder(w).Encode(Result{})
       })
       defer server.Close()

       client, _ := NewClientWithCredentials(server.URL, "id", "secret")
       result, err := client.NewMethod(context.Background(), SomeParams{})

       assert.NoError(t, err)
       assert.NotNil(t, result)
   }
   ```

## Best Practices

### Code Style

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use meaningful variable names
- Keep functions small and focused
- Write self-documenting code with comments for complex logic

### Testing

- Write tests before implementation (TDD)
- Test edge cases and error conditions
- Mock external dependencies
- Aim for 80%+ code coverage

### Git Workflow

- Create feature branches from `main`
- Write descriptive commit messages
- Keep commits focused (one logical change per commit)
- Squash commits before merging

**Commit message format:**
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `chore`

**Example:**
```
feat(controller): Add NodeGroup reconciliation logic

Implement the core reconciliation loop for NodeGroup resources.
This includes creating VPSieNode resources based on desired count
and updating status with current state.

Closes #123
```

### Kubernetes Best Practices

- Use finalizers for cleanup logic
- Update status separately from spec
- Use conditions for communicating state
- Implement proper RBAC with minimal permissions
- Add resource limits to deployments

## Troubleshooting

### Tests Failing

```bash
# Clean test cache
go clean -testcache

# Run with verbose output
go test -v ./...

# Run specific test with more details
go test -v -run TestName ./pkg/...
```

### CRD Generation Not Working

```bash
# Ensure controller-gen is installed
which controller-gen

# Regenerate with verbose output
controller-gen -v crd paths="./pkg/apis/..." output:crd:dir=./deploy/crds
```

### Import Errors

```bash
# Sync dependencies
go mod tidy
go mod download

# Verify go.mod
go mod verify
```

## Resources

- **Go Documentation:** https://go.dev/doc/
- **Kubernetes API Conventions:** https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **controller-runtime Documentation:** https://pkg.go.dev/sigs.k8s.io/controller-runtime
- **Kubebuilder Book:** https://book.kubebuilder.io/
- **VPSie API Docs:** https://api-docs.vpsie.com/

## Getting Help

- **GitHub Issues:** https://github.com/vpsie/vpsie-k8s-autoscaler/issues
- **Discussions:** https://github.com/vpsie/vpsie-k8s-autoscaler/discussions
- **VPSie Support:** support@vpsie.com
