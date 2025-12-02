# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0-alpha] - 2025-12-03

### Production Readiness Release ✅

This release focuses on production-ready improvements including concurrency safety, memory management, enhanced observability, and configuration flexibility.

### Fixed

#### Concurrency & Thread Safety
- Fixed status update race conditions by using Patch API with optimistic locking
  - `pkg/controller/nodegroup/reconciler.go`: All status updates now use `Status().Patch()` with conflict handling
  - Prevents concurrent update conflicts with automatic retry on conflict
- Fixed unsafe pointer returns in utilization tracker
  - `pkg/scaler/utilization.go`: `GetNodeUtilization()` and `GetUnderutilizedNodes()` now return deep copies
  - Prevents external modification of internal state
- Fixed context cancellation handling in drain operations
  - `pkg/scaler/drain.go`: All cleanup operations use `context.Background()` with timeout
  - Ensures cleanup completes even when parent context is cancelled

#### Memory Management
- Fixed memory leak in node utilization tracking
  - `pkg/scaler/utilization.go`: Added automatic garbage collection for deleted nodes
  - Prevents unbounded memory growth in long-running deployments
- Fixed context leak in metrics collection loop
  - `pkg/controller/manager.go`: Context cancellation now happens immediately, not deferred
  - Prevents context accumulation in long-running goroutines

#### Reliability
- Added goroutine timeout protection in metrics collection
  - `pkg/controller/manager.go`: 45-second timeout prevents API hang from blocking metrics
  - Timeout is less than collection interval (60s) to avoid overlap

### Added

#### Enhanced Observability
- Added 4 new Prometheus metrics for production monitoring (`pkg/metrics/metrics.go`)
  - `vpsie_autoscaler_scale_down_blocked_total{nodegroup,namespace,reason}` - Track scale-down operations blocked by safety checks (PDB, affinity, capacity, cooldown)
  - `vpsie_autoscaler_safety_check_failures_total{check_type,nodegroup,namespace}` - Monitor safety check failures by type
  - `vpsie_autoscaler_node_drain_duration_seconds{nodegroup,namespace,result}` - Track drain performance (success, timeout, error)
  - `vpsie_autoscaler_node_drain_pods_evicted{nodegroup,namespace}` - Monitor pod eviction counts during drain
- All new metrics properly registered in `RegisterMetrics()` and `ResetMetrics()`
- Follows Prometheus naming conventions (snake_case, proper suffixes)

#### Configuration Flexibility
- Added cloud-init template configuration support (`pkg/controller/options.go`)
  - New `CloudInitTemplate` field in controller Options
  - Allows custom cloud-init scripts for node provisioning
- Added SSH key configuration with per-node override support
  - New `SSHKeyIDs` field in controller Options for global SSH keys
  - `pkg/controller/vpsienode/provisioner.go`: Added `getSSHKeyIDs()` helper
  - Prefers spec-level SSH keys (per-node override), falls back to global config
  - Enables both cluster-wide and node-specific SSH key injection

### Changed

- Updated Prometheus metrics count from 22 to 26 total metrics
- Enhanced metrics collection with timeout protection
- Improved context handling patterns across codebase

### Added

#### Observability Framework
- Comprehensive Prometheus metrics package (`pkg/metrics/`)
  - 22 metrics across 7 categories (NodeGroup, VPSieNode, Controller, API, Scaling, Pods, Events)
  - Histograms for duration tracking (reconciliation, API requests, node provisioning/termination)
  - Counters for operations and errors
  - Gauges for current state
  - Helper functions in `recorder.go` for easy metric recording
- Structured logging package (`pkg/logging/`)
  - zap logger integration with structured fields
  - Request ID tracking (UUID v4) for distributed tracing
  - Comprehensive logging functions for scaling, API, nodes, phases, reconciliation
  - ISO8601 time encoding and caller info
- Kubernetes event emitter (`pkg/events/`)
  - 20+ event types for NodeGroup and VPSieNode lifecycle
  - Automatic metrics recording for all emitted events
  - Normal and Warning event types
- VPSie API client observability integration
  - Metrics tracking for all API methods (GET, POST, PUT, DELETE)
  - Request duration histograms (10ms to 40s buckets)
  - Error categorization (unauthorized, forbidden, rate_limited, etc.)
  - Debug logging for API calls and responses
  - Error logging with full context
- Comprehensive observability documentation (OBSERVABILITY.md - 409 lines)
  - All metrics documented with labels and purposes
  - Sample Prometheus queries and alert rules
  - Grafana dashboard recommendations
  - Integration guide and best practices

### Phase 2: Controller Implementation (Planned)
- Controller manager with leader election
- NodeGroup controller with reconciliation loop
- VPSieNode controller with lifecycle management
- Event-driven scale-up logic
- Safe scale-down with utilization monitoring
- Integration of observability framework into controllers

## [0.1.0-alpha] - 2025-10-12

### Phase 1: Foundation Complete ✅

This release establishes the complete foundation for the VPSie Kubernetes Autoscaler, including API client, CRDs, comprehensive testing, and CI/CD infrastructure.

### Added

#### VPSie API Client
- VPSie custom authentication with automatic token refresh ([commit b0b3a73](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/b0b3a73))
- Thread-safe token management with mutex protection
- VM lifecycle operations: List, Create, Get, Delete
- Comprehensive error handling with wrapped errors
- IsNotFound, IsUnauthorized, IsRateLimited error helpers
- 36 tests with 85.3% coverage

#### Custom Resource Definitions (CRDs)
- NodeGroup CRD for managing logical node groups ([commit e853900](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/e853900))
  - Scaling policies (min/max nodes, CPU/memory thresholds)
  - Node labels and taints support
  - SSH keys and VPSie tags
  - Status tracking with conditions
- VPSieNode CRD for tracking individual VPS instances
  - 8 lifecycle phases: Pending → Provisioning → Provisioned → Joining → Ready → Terminating → Deleting → Failed
  - Resource allocation tracking (CPU, Memory, Disk, Bandwidth)
  - Detailed timestamps for each lifecycle phase
- Full OpenAPI v3 validation with kubebuilder markers
- Custom printer columns for kubectl output
- 38 CRD tests with 100% pass rate

#### Documentation
- Product Requirements Document (PRD) - 429 lines ([commit 8caace8](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/8caace8))
- API Reference (docs/API.md) - 497 lines
- Development Guide (docs/DEVELOPMENT.md) - 505 lines
- OAuth Migration Guide (OAUTH_MIGRATION.md) - 315 lines ([commit 7c57f06](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/7c57f06))
- Implementation roadmap (NEXT_STEPS.md) - 630 lines
- Example configurations (general-purpose, high-memory, spot-instances)

#### CI/CD & Infrastructure
- GitHub Actions CI workflow ([commit f841ebc](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/f841ebc))
  - Automated testing with race detector and coverage reporting
  - golangci-lint with Go 1.22 compatibility
  - Binary build verification
  - CRD manifest freshness verification
- Docker multi-arch builds (linux/amd64, linux/arm64)
  - Automated builds on push to main and version tags
  - Published to ghcr.io/vpsie/vpsie-k8s-autoscaler
  - Version information injection via ldflags
  - Distroless base image for minimal attack surface
- Makefile with comprehensive targets
  - Build, test, lint, generate, docker operations
  - kind cluster management
  - Helm package and install commands

### Fixed

#### CI/CD Issues
- Fixed Go version mismatch in go.mod (1.25.2 → 1.22) ([commit 5ac8d0f](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/5ac8d0f))
- Added dependency download step to lint job ([commit 9c0b357](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/9c0b357))
- Removed duplicate ci.yaml workflow file ([commit 15524f1](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/15524f1))
- Fixed Go 1.22 covdata warning in test execution ([commit 3a9b6b3](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/3a9b6b3))

#### VPSie API Client
- Fixed error unwrapping with errors.As() instead of type assertions ([commit 9af4774](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/9af4774))
- Updated API endpoints from /vms to /vm (singular) ([commit a5172b8](https://github.com/vpsieinc/vpsie-k8s-autoscaler/commit/a5172b8))
- Changed VPS ID types from string to int
- Updated VPS struct field mappings to match VPSie API response format

### Changed

#### Authentication
- Replaced OAuth 2.0 with VPSie custom authentication
- Endpoint changed from `/oauth/token` to `/auth/from/api`
- Request format changed from JSON to form-urlencoded
- Response structure now uses nested accessToken object with RFC3339 timestamps
- Proactive token refresh 5 minutes before expiration
- Reactive token refresh on 401 Unauthorized with single retry

#### Kubernetes Secret Format (Breaking Change)
**Old format (no longer supported):**
```yaml
stringData:
  token: "long-lived-api-token"
```

**New format (required):**
```yaml
stringData:
  clientId: "your-client-id"
  clientSecret: "your-client-secret"
```

See [OAUTH_MIGRATION.md](OAUTH_MIGRATION.md) for migration guide.

### Test Coverage

- **Total Tests:** 72 passing, 2 skipped (integration tests)
- **Coverage:** 81.5% overall
  - VPSie client: 85.3%
  - CRD types: 59.4% (generated code has lower coverage)
- **Zero Failures:** All tests passing in CI/CD pipeline

### Docker Images

Published to GitHub Container Registry:
- `ghcr.io/vpsie/vpsie-k8s-autoscaler:latest`
- `ghcr.io/vpsie/vpsie-k8s-autoscaler:main`
- `ghcr.io/vpsie/vpsie-k8s-autoscaler:v0.1.0-alpha` (planned)

Multi-architecture support:
- linux/amd64
- linux/arm64

### Project Statistics

- **Total Lines of Code:** ~8,500+ lines (excluding generated code)
- **Documentation:** ~2,500+ lines
- **Test Code:** ~2,000+ lines
- **Commits:** 15+ commits in foundation phase
- **Contributors:** 1 (with Claude Code assistance)

## Next Steps

See [NEXT_STEPS.md](NEXT_STEPS.md) for detailed implementation roadmap.

**Phase 2 (Weeks 1-4):** Controller Implementation
- Priority 1: Controller scaffold with manager setup
- Priority 2: NodeGroup controller with reconciliation
- Priority 3: VPSieNode controller with lifecycle management

**Phase 3 (Weeks 5-7):** Scaling Logic
- Priority 4: Event-driven scale-up
- Priority 5: Safe scale-down with utilization monitoring

**Phase 4 (Weeks 8-9):** Deployment & Operations
- Priority 6: Deployment manifests (RBAC, Deployment, Service)
- Priority 7: Observability (Prometheus metrics, structured logging)
- Priority 8: Complete documentation

**Phase 5 (Week 10+):** Polish & Release
- Priority 9: Edge cases & reliability
- Priority 10: Release preparation (versioning, CI/CD, security)

---

## Release Links

- [GitHub Repository](https://github.com/vpsieinc/vpsie-k8s-autoscaler)
- [Container Images](https://github.com/vpsieinc/vpsie-k8s-autoscaler/pkgs/container/vpsie-k8s-autoscaler)
- [GitHub Actions](https://github.com/vpsieinc/vpsie-k8s-autoscaler/actions)

## Contributors

- [@keresztespeter](https://github.com/keresztespeter) - Initial implementation with Claude Code assistance

---

**Note:** This project is in early alpha development. The API may change between releases.
