# VPSie Kubernetes Node Autoscaler

Intelligent Kubernetes node autoscaler that dynamically provisions and optimizes nodes using the VPSie cloud platform.

## Features

- 🚀 **Dynamic Scaling:** Automatically scales cluster nodes based on workload demands
- 💰 **Cost Optimization:** Selects the most cost-effective VPSie instance types
- 🔄 **Node Rebalancing:** Continuously optimizes node selection for best price/performance
- 📊 **Node Groups:** Organize nodes into logical groups with different policies
- 🔐 **OAuth2 Integration:** Secure authentication with VPSie API
- 📈 **Prometheus Metrics:** Comprehensive observability

## Quick Start

### Prerequisites

- Kubernetes cluster 1.24+
- VPSie account with API access
- kubectl configured
- Helm 3.0+

### Installation

```bash
# Add Helm repository
helm repo add vpsie-autoscaler https://charts.vpsie.io
helm repo update

# Install the autoscaler (uses existing vpsie-secret)
helm install vpsie-autoscaler vpsie-autoscaler/vpsie-k8s-autoscaler \
  --namespace kube-system \
  --set cluster.name=production

# Note: The autoscaler automatically uses the vpsie-secret in kube-system
```

### Create a Node Group

```bash
kubectl apply -f - <<EOFYAML
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: general-workload
  namespace: kube-system
spec:
  minNodes: 2
  maxNodes: 10
  targetResources:
    cpu: "2-4"
    memory: "4-8Gi"
    disk: "50Gi"
  datacenter:
    region: "us-east"
EOFYAML
```

## Development

### Setup Development Environment

```bash
# Run the setup script
curl -fsSL https://raw.githubusercontent.com/vpsie/k8s-autoscaler/main/scripts/setup.sh | bash
```

### Build and Test

```bash
# Build
make build

# Run tests
make test

# Run linters
make lint

# Create local cluster
make kind-create

# Deploy locally
make deploy
```

## Documentation

- [Product Requirements Document](docs/PRD.md)
- [Architecture Overview](docs/ARCHITECTURE.md)
- [API Reference](docs/API.md)
- [Configuration Guide](docs/CONFIGURATION.md)

## License

Apache License 2.0
