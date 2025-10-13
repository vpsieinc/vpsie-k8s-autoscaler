# VPSie Kubernetes Node Autoscaler

[![CI](https://github.com/vpsie/vpsie-k8s-autoscaler/actions/workflows/ci.yml/badge.svg)](https://github.com/vpsie/vpsie-k8s-autoscaler/actions/workflows/ci.yml)
[![Docker](https://github.com/vpsie/vpsie-k8s-autoscaler/actions/workflows/docker.yml/badge.svg)](https://github.com/vpsie/vpsie-k8s-autoscaler/actions/workflows/docker.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/vpsie/vpsie-k8s-autoscaler)](https://goreportcard.com/report/github.com/vpsie/vpsie-k8s-autoscaler)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

Event-driven Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform.

## 🚧 Project Status

**Current Phase:** Foundation Complete, Controller Implementation Ready (v0.1.0-alpha)

✅ **Completed:**
- VPSie API client with custom authentication and automatic token refresh
- Custom Resource Definitions (NodeGroup, VPSieNode) with OpenAPI v3 validation
- Comprehensive test coverage (72 tests, 81.5% coverage)
- CRD manifests and example configurations
- Complete documentation (PRD, API reference, development guide, observability guide)
- Docker multi-arch images (amd64, arm64) published to ghcr.io
- CI/CD pipeline with automated testing, linting, and image builds
- GitHub Actions workflows fully configured and passing
- Comprehensive observability framework (metrics, logging, events)

🚧 **Ready to Start:**
- Phase 2: Controller implementation (see [NEXT_STEPS.md](NEXT_STEPS.md))
  - Controller scaffold with manager setup
  - NodeGroup controller with reconciliation loop
  - VPSieNode controller with lifecycle management

📋 **Planned (Phases 3-5):**
- Event-driven autoscaling logic
- Scale-down with utilization monitoring
- Cost optimization engine
- Helm charts and deployment manifests
- Prometheus metrics and observability

## 📦 Container Images

Docker images are automatically built and published to GitHub Container Registry:

```bash
# Pull the latest image
docker pull ghcr.io/vpsie/vpsie-k8s-autoscaler:latest

# Pull a specific version
docker pull ghcr.io/vpsie/vpsie-k8s-autoscaler:v0.1.0

# Pull from main branch
docker pull ghcr.io/vpsie/vpsie-k8s-autoscaler:main
```

**Available tags:**
- `latest` - Latest stable release from main branch
- `v*` - Semantic version tags (e.g., `v0.1.0`, `v0.2.0`)
- `main` - Latest commit from main branch
- `main-<sha>` - Specific commit from main branch

**Supported architectures:**
- `linux/amd64`
- `linux/arm64`

## Recent Updates

### October 2025 - Foundation Phase Complete ✅
- **Fixed CI/CD Pipeline**: Resolved Go version mismatch and duplicate workflow files
- **Authentication**: Implemented VPSie custom authentication with automatic token refresh
- **Multi-arch Docker Images**: Automated builds for linux/amd64 and linux/arm64
- **Documentation**: Complete API reference, development guide, observability guide, and migration docs
- **Test Coverage**: 72 passing tests with 81.5% coverage, zero failures
- **Observability Framework**: Comprehensive metrics (22 Prometheus metrics), structured logging (zap), and Kubernetes events

All CI/CD workflows are now passing, and the project is ready for Phase 2 controller implementation.

## Features

### Implemented ✅
- 🔐 **VPSie API Integration:** Custom authentication with automatic token refresh (RFC3339 expiry)
- 📦 **Custom Resources:** NodeGroup and VPSieNode CRDs for declarative management
- 🧪 **Comprehensive Testing:** 72 tests with 81.5% coverage, fully automated in CI
- 📝 **Full OpenAPI Validation:** Kubernetes-native validation with kubebuilder markers
- 🐳 **Container Images:** Multi-arch Docker images published to ghcr.io
- 🔄 **CI/CD Pipeline:** Automated testing, linting, building, and image publishing
- 📊 **Observability Framework:** 22 Prometheus metrics, structured logging (zap), Kubernetes events

### Planned 🚧
- 🚀 **Event-Driven Scaling:** React to pod scheduling failures and resource shortages
- 💰 **Cost Optimization:** Select most cost-effective VPSie instance types
- 🔄 **Lifecycle Management:** Complete VPS provisioning, joining, and termination
- 📊 **Node Groups:** Organize nodes with different scaling policies
- 📈 **Prometheus Metrics:** Comprehensive observability

## Quick Start

### Prerequisites

- Kubernetes cluster 1.24+
- VPSie account with API credentials
- kubectl configured
- Go 1.22+ (for development)

### Install CRDs

```bash
# Clone the repository
git clone https://github.com/vpsie/vpsie-k8s-autoscaler.git
cd vpsie-k8s-autoscaler

# Install Custom Resource Definitions
kubectl apply -f deploy/crds/

# Verify CRD installation
kubectl get crds | grep autoscaler.vpsie.com
```

### Create VPSie Credentials Secret

```bash
# Create secret with VPSie OAuth credentials
kubectl create secret generic vpsie-secret \
  --namespace=kube-system \
  --from-literal=clientId='your-client-id' \
  --from-literal=clientSecret='your-client-secret'
```

### Create a NodeGroup

```bash
# Apply example NodeGroup configuration
kubectl apply -f deploy/examples/nodegroup-general-purpose.yaml

# View NodeGroups
kubectl get nodegroups -n kube-system
kubectl get ng -n kube-system  # short name
```

**Example NodeGroup:**
```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: general-purpose
  namespace: kube-system
spec:
  minNodes: 2
  maxNodes: 10
  datacenterID: "dc-us-east-1"
  offeringIDs:
    - "small-2cpu-4gb"
    - "medium-4cpu-8gb"
  osImageID: "ubuntu-22.04-lts"
  scaleUpPolicy:
    enabled: true
    stabilizationWindowSeconds: 60
    cpuThreshold: 80
    memoryThreshold: 80
  scaleDownPolicy:
    enabled: true
    stabilizationWindowSeconds: 600
    cpuThreshold: 50
    memoryThreshold: 50
```

## Development

### Build and Test

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run linters
golangci-lint run

# Build controller binary
make build

# Generate CRD manifests (after modifying types)
make generate
```

### Project Structure

```
vpsie-k8s-autoscaler/
├── cmd/
│   └── controller/          # Controller binary entry point (planned)
├── pkg/
│   ├── apis/
│   │   └── autoscaler/v1alpha1/  # CRD type definitions
│   ├── vpsie/
│   │   └── client/          # VPSie API client
│   └── log/                 # Logging utilities
├── deploy/
│   ├── crds/                # CRD manifests
│   └── examples/            # Example configurations
└── docs/
    ├── PRD.md               # Product Requirements Document
    └── NEXT_STEPS.md        # Development roadmap
```

## Documentation

- **[NEXT_STEPS.md](NEXT_STEPS.md)** - Implementation roadmap and next steps
- **[OBSERVABILITY.md](OBSERVABILITY.md)** - Metrics, logging, and events guide
- **[Product Requirements Document](docs/PRD.md)** - Complete requirements and architecture
- **[CRD Examples](deploy/examples/)** - NodeGroup and VPSieNode examples
- **[API Client Documentation](pkg/vpsie/client/)** - VPSie API integration

## Contributing

This project is in early development. Contributions are welcome!

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

Apache License 2.0
