# VPSie Kubernetes Autoscaler Architecture

## Overview

The VPSie Kubernetes Autoscaler is a Kubernetes controller that automatically manages cluster nodes by provisioning and deprovisioning VPSie VPS instances based on workload demands and cost optimization goals.

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐     │
│  │    NodeGroup     │     │    NodeGroup     │     │    NodeGroup     │     │
│  │   (CRD)          │     │   (CRD)          │     │   (CRD)          │     │
│  │                  │     │                  │     │                  │     │
│  │  minNodes: 2     │     │  minNodes: 1     │     │  minNodes: 3     │     │
│  │  maxNodes: 10    │     │  maxNodes: 5     │     │  maxNodes: 20    │     │
│  └────────┬─────────┘     └────────┬─────────┘     └────────┬─────────┘     │
│           │                        │                        │               │
│           └────────────────────────┼────────────────────────┘               │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    VPSie Autoscaler Controller                       │   │
│  │                                                                      │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐      │   │
│  │  │ NodeGroup       │  │ VPSieNode       │  │ Pod Watcher     │      │   │
│  │  │ Controller      │  │ Controller      │  │                 │      │   │
│  │  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘      │   │
│  │           │                    │                    │               │   │
│  │           └────────────────────┼────────────────────┘               │   │
│  │                                │                                    │   │
│  │  ┌─────────────────────────────▼──────────────────────────────┐    │   │
│  │  │                    Core Components                          │    │   │
│  │  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐           │    │   │
│  │  │  │   Scaler    │ │ Rebalancer  │ │ Cost        │           │    │   │
│  │  │  │             │ │             │ │ Calculator  │           │    │   │
│  │  │  └─────────────┘ └─────────────┘ └─────────────┘           │    │   │
│  │  └────────────────────────────┬───────────────────────────────┘    │   │
│  │                               │                                     │   │
│  │  ┌────────────────────────────▼───────────────────────────────┐    │   │
│  │  │                    VPSie API Client                         │    │   │
│  │  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │    │   │
│  │  │  │ Rate Limiter │ │ Circuit      │ │ OAuth        │        │    │   │
│  │  │  │              │ │ Breaker      │ │ Handler      │        │    │   │
│  │  │  └──────────────┘ └──────────────┘ └──────────────┘        │    │   │
│  │  └────────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                         VPSieNode Resources                         │    │
│  │   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐ │    │
│  │   │VPSieNode│  │VPSieNode│  │VPSieNode│  │VPSieNode│  │VPSieNode│ │    │
│  │   │ node-1  │  │ node-2  │  │ node-3  │  │ node-4  │  │ node-5  │ │    │
│  │   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘ │    │
│  └────────┼────────────┼────────────┼────────────┼────────────┼──────┘    │
│           │            │            │            │            │            │
└───────────┼────────────┼────────────┼────────────┼────────────┼────────────┘
            │            │            │            │            │
            ▼            ▼            ▼            ▼            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              VPSie Cloud API                                 │
│                                                                              │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│   │     VM      │  │     VM      │  │     VM      │  │     VM      │       │
│   │   node-1    │  │   node-2    │  │   node-3    │  │   node-4    │       │
│   └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Component Overview

### Custom Resource Definitions (CRDs)

#### NodeGroup
Defines a pool of nodes with autoscaling configuration:
- `minNodes` / `maxNodes`: Scaling boundaries
- `offeringIDs`: Allowed VPSie instance types
- `datacenterID`: Target datacenter
- `osImageID`: Operating system image
- `kubernetesVersion`: K8s version for nodes

#### VPSieNode
Represents a single VPSie VM managed by the autoscaler:
- Tracks VM provisioning status
- Contains VPSie instance ID
- Managed by owning NodeGroup

### Controllers

#### NodeGroup Controller
Primary reconciliation loop for NodeGroups:
- Monitors desired vs current node count
- Triggers scale-up/scale-down operations
- Manages VPSieNode lifecycle
- Handles finalizers for cleanup

#### VPSieNode Controller
Manages individual VM lifecycle:
- Provisions new VMs via VPSie API
- Monitors provisioning progress
- Handles VM termination
- Updates status based on VM state

### Core Components

#### Scaler (`pkg/scaler/`)
Handles scaling decisions:
- **ScaleDownManager**: Identifies underutilized nodes
- **PolicyEngine**: Validates scale-down safety
- **DrainManager**: Handles pod eviction

#### Rebalancer (`pkg/rebalancer/`)
Optimizes node allocation:
- **Analyzer**: Identifies rebalance candidates
- **Planner**: Creates migration plans
- **Executor**: Executes node replacements

#### Cost Calculator (`pkg/vpsie/cost/`)
Provides cost analysis:
- Calculates hourly/monthly costs
- Compares offering prices
- Estimates savings from optimization

### VPSie API Client (`pkg/vpsie/client/`)
Handles all VPSie API communication:
- **OAuth Handler**: Token management and refresh
- **Rate Limiter**: Prevents API quota exhaustion
- **Circuit Breaker**: Fault tolerance for API failures
- **Retry Logic**: Exponential backoff with jitter

## Data Flow

### Scale-Up Flow

```
1. Pending pods detected
         │
         ▼
2. NodeGroup Controller evaluates
         │
         ▼
3. Creates VPSieNode resource
         │
         ▼
4. VPSieNode Controller provisions VM
         │
         ▼
5. VPSie API creates VM
         │
         ▼
6. VM joins cluster via kubelet
         │
         ▼
7. VPSieNode status updated to Ready
         │
         ▼
8. Pending pods scheduled
```

### Scale-Down Flow

```
1. Underutilization detected
         │
         ▼
2. ScaleDownManager selects candidates
         │
         ▼
3. PolicyEngine validates safety
         │
         ├──[Blocked]──► Log reason, skip
         │
         ▼
4. DrainManager evicts pods
         │
         ▼
5. NodeGroup Controller deletes VPSieNode
         │
         ▼
6. VPSieNode finalizer terminates VM
         │
         ▼
7. K8s node removed from cluster
```

## Observability

### Metrics (Prometheus)
- `nodegroup_*`: NodeGroup state metrics
- `vpsienode_*`: VPSieNode lifecycle metrics
- `vpsie_api_*`: API client metrics
- `controller_*`: Controller performance metrics
- `scale_*`: Scaling operation metrics
- `rebalancer_*`: Rebalancer operation metrics

### Logging (Zap)
- Structured JSON logs
- Correlation IDs for request tracing
- Debug, Info, Warn, Error levels

### Audit Logging
- Security-relevant events
- Scaling decisions
- API operations
- Configuration changes

## Security

### Authentication
- OAuth2 client credentials for VPSie API
- Kubernetes RBAC for controller operations

### Secrets Management
- VPSie credentials stored in Kubernetes Secret
- Automatic credential rotation support
- No credentials in logs or metrics

### Network Security
- TLS 1.2+ for VPSie API communication
- Strong cipher suites enforced
- NetworkPolicy support in Helm chart

## High Availability

### Leader Election
- Uses Kubernetes lease-based leader election
- Only one active controller instance
- Automatic failover on leader failure

### Circuit Breaker
- Prevents cascading failures to VPSie API
- Automatic recovery via half-open state
- Configurable thresholds

### Graceful Shutdown
- Completes in-flight operations
- Releases leader lease
- Closes API connections cleanly
