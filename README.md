# VPSie Kubernetes Node Autoscaler

Event-driven Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform.

## 🚧 Project Status

**Current Phase:** Foundation Implementation (v0.1.0-alpha)

✅ **Completed:**
- VPSie API client with OAuth 2.0 authentication
- Custom Resource Definitions (NodeGroup, VPSieNode)
- Comprehensive test coverage (81.5%)
- CRD manifests and example configurations
- Product Requirements Document

🚧 **In Progress:**
- Controller implementation (see [NEXT_STEPS.md](NEXT_STEPS.md))

📋 **Planned:**
- Event-driven autoscaling logic
- Cost optimization engine
- Helm charts and deployment manifests

## Features

### Implemented ✅
- 🔐 **VPSie API Integration:** OAuth 2.0 client with automatic token refresh
- 📦 **Custom Resources:** NodeGroup and VPSieNode CRDs for declarative management
- 🧪 **Comprehensive Testing:** 74 tests with 81.5% coverage
- 📝 **Full OpenAPI Validation:** Kubernetes-native validation with kubebuilder markers

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
