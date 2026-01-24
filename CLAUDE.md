# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VPSie Kubernetes Node Autoscaler - An intelligent Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform. The autoscaler automatically scales cluster nodes based on workload demands, optimizes costs by selecting appropriate instance types, and continuously rebalances nodes for best price/performance.

## Working Guidelines

1. First think through the problem, read the codebase for relevant files.
2. Before you make any major changes, check in with me and I will verify the plan.
3. Please every step of the way just give me a high level explanation of what changes you made.
4. Make every task and code change you do as simple as possible. We want to avoid making any massive or complex changes. Every change should impact as little code as possible. Everything is about simplicity.
5. Maintain a documentation file that describes how the architecture of the app works inside and out.
6. Never speculate about code you have not opened. If the user references a specific file, you MUST read the file before answering. Make sure to investigate and read relevant files BEFORE answering questions about the codebase.

For detailed architecture diagrams (scale-up/down workflows, VPSieNode state machine, CRD relationships), see `docs/ARCHITECTURE.md`.

## Build and Development Commands

```bash
# Build
make build                    # Build controller binary to bin/vpsie-autoscaler
make clean                    # Clean build artifacts

# Test
make test                     # Run unit tests with race detection
make lint                     # Run golangci-lint
make fmt                      # Format code (gofmt + goimports)

# Run specific tests
go test ./pkg/scaler -run TestScaleDownManager_IdentifyUnderutilizedNodes -v
go test ./pkg/rebalancer/... -v
go test -race ./pkg/controller/nodegroup -v

# Integration tests (require cluster or mock)
make test-integration-basic   # CRUD tests only (fast)
make test-integration-all     # Complete integration suite
go test -tags=integration ./test/integration -run TestNodeGroup_CRUD -v

# Code generation (REQUIRED after modifying CRD types)
make generate                 # Generate DeepCopy methods + CRD manifests

# Local development
make run                      # Run controller against current kubectl context
make kind-create              # Create local kind cluster
make kind-delete              # Delete kind cluster
```

## Architecture

### Component Overview

```
cmd/controller/       Main Kubernetes controller binary with CLI (cobra)
pkg/
├── apis/autoscaler/v1alpha1/  CRD definitions (NodeGroup, VPSieNode, labels)
├── controller/
│   ├── nodegroup/    NodeGroup reconciler - main orchestration loop
│   └── vpsienode/    VPSieNode controller - VPS lifecycle management
├── scaler/           Scale-down logic (utilization analysis, draining, 6 safety checks)
├── rebalancer/       Cost optimization (analyzer, planner, executor)
├── vpsie/
│   ├── client/       VPSie API v2 client (OAuth, rate limiting, circuit breaker)
│   └── cost/         Cost calculation and optimization
├── metrics/          Prometheus metrics with label sanitization
├── events/           Kubernetes event management and scale-up triggers
├── webhook/          Validation webhooks (TLS 1.3)
├── tracing/          Sentry integration for error tracking and performance
├── audit/            Audit logging for compliance
└── utils/            Shared utilities (node helpers)
```

### Critical Data Flow

1. **Scale-Up Path:** Unschedulable pods → `EventWatcher` → `ScaleUpController` → `NodeGroupReconciler` → VPSie API (provision VM) → VPSieNode CR → Node joins cluster

2. **Scale-Down Path:** `ScaleDownManager` identifies underutilized nodes → `PolicyEngine` validates 6 safety checks → drains node → `NodeGroupReconciler` terminates VPSie VM

3. **Rebalancing Path:** `Analyzer` (5 safety checks) → `Planner` (migration strategy) → `Executor` (cordon, drain, provision, rollback)

### Key Design Decisions

- **Controller Separation:** ScaleDownManager handles node identification/draining, NodeGroupReconciler handles VM termination. This prevents race conditions.
- **Scale-Down Safety (6 checks):** No local storage pods, pods can be rescheduled, no critical system pods, no anti-affinity violations, cluster has capacity, node not protected by annotation
- **Rebalancer Safety (5 checks):** Cluster health, PDB compliance, local storage detection, maintenance windows, cooldowns
- **VPSie Client:** OAuth with auto-refresh, rate limiting (100 req/min default), circuit breaker for fault tolerance
- **Max 1 node per scale-down operation:** Prevents aggressive scale-down
- **TTL for Failed VPSieNodes:** Automatic cleanup of stuck resources (30min default)

## VPSie API Integration

The client reads OAuth credentials from Kubernetes secret `vpsie-secret` in `kube-system`:
- `clientId`: VPSie OAuth client ID
- `clientSecret`: VPSie OAuth client secret
- `url`: API endpoint (optional, defaults to https://api.vpsie.com/v2)

```go
// Create client from K8s secret
client, err := client.NewClient(ctx, clientset, &client.ClientOptions{
    SecretName:      "vpsie-secret",
    SecretNamespace: "kube-system",
    RateLimit:       100,  // requests per minute
    Timeout:         30 * time.Second,
})

// Typed error handling
if client.IsNotFound(err) { /* 404 */ }
if client.IsRateLimited(err) { /* 429 */ }
if apiErr, ok := err.(*client.APIError); ok {
    log.Error(apiErr.RequestID, apiErr.Message)
}
```

## Code Generation Workflow

After modifying CRD types in `pkg/apis/autoscaler/v1alpha1/`:

```bash
make generate  # Generates zz_generated.deepcopy.go + deploy/crds/*.yaml
```

If `controller-gen` is missing:
```bash
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
```

## Testing Patterns

- Unit tests: `*_test.go` alongside code
- Integration tests: `//go:build integration` tag
- Performance tests: `//go:build performance` tag
- E2E tests: `//go:build e2e` tag
- Use table-driven tests; mock VPSie API and Kubernetes API

## Key Files for Common Tasks

| Task | Files |
|------|-------|
| Add NodeGroup field | `pkg/apis/autoscaler/v1alpha1/nodegroup_types.go`, then `make generate` |
| Add/modify labels | `pkg/apis/autoscaler/v1alpha1/labels.go` |
| Modify scaling logic | `pkg/scaler/scaler.go`, `pkg/scaler/policies.go`, `pkg/scaler/safety.go` |
| Modify rebalancing | `pkg/rebalancer/analyzer.go`, `planner.go`, `executor.go` |
| Add metrics | `pkg/metrics/metrics.go` (use `sanitize.go` for labels) |
| VPSie API changes | `pkg/vpsie/client/client.go`, `types.go`, `errors.go` |
| Webhook validation | `pkg/webhook/server.go`, `nodegroup_validator.go`, `vpsienode_validator.go` |
| Controller CLI flags | `cmd/controller/main.go`, `pkg/controller/options.go` |
| Error tracking | `pkg/tracing/sentry.go` |
| Node utilities | `pkg/utils/node.go` |

## Deployment

```bash
# Create VPSie credentials secret first
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='your-client-id' \
  --from-literal=clientSecret='your-client-secret' \
  -n kube-system

# Deploy via Helm (recommended)
make helm-install

# Or via kubectl
kubectl apply -f deploy/crds/
kubectl apply -f deploy/manifests/
```

## Labels and Annotations

Key labels/annotations defined in `pkg/apis/autoscaler/v1alpha1/labels.go`:

| Key | Purpose |
|-----|---------|
| `autoscaler.vpsie.com/managed=true` | Required for NodeGroup to be managed |
| `autoscaler.vpsie.com/nodegroup` | Associates VPSieNode/K8s node with parent NodeGroup |
| `autoscaler.vpsie.com/vpsienode` | Associates K8s node with VPSieNode CR |
| `autoscaler.vpsie.com/creation-reason` | Tracks why node was created: `metrics`, `manual`, `rebalance`, `initial` |
| `autoscaler.vpsie.com/vps-id` (annotation) | VPSie VPS instance ID |

## Important Notes

- **Metrics sanitization:** All Prometheus labels are sanitized via `pkg/metrics/sanitize.go` (max 100 chars, special chars → underscores)
- **Cloud-init removed:** Node configuration handled by VPSie API via QEMU agent. Fields `spec.userData`, `spec.cloudInitTemplate*` no longer exist.
- **TLS 1.3 required:** Webhook server enforces TLS 1.3 minimum
- **Sentry integration:** Optional error tracking/tracing via `pkg/tracing/sentry.go`. Configure with `--sentry-dsn` or `SENTRY_DSN` env var.
