# VPSie Kubernetes Autoscaler - Architecture

## Overview

The VPSie Kubernetes Autoscaler is an intelligent node autoscaler that dynamically provisions and optimizes Kubernetes nodes using the VPSie cloud platform.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         KUBERNETES CLUSTER                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │    Pods     │  │    Nodes    │  │   Secrets   │  │   Events    │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                │                │                │               │
│         └────────────────┴────────────────┴────────────────┘               │
│                                   │                                         │
│                          Kubernetes API                                     │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                      VPSIE AUTOSCALER CONTROLLER                             │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                        Controller Manager                               │ │
│  │  ┌──────────────────┐    ┌──────────────────┐    ┌────────────────┐   │ │
│  │  │   NodeGroup      │    │    VPSieNode     │    │  EventWatcher  │   │ │
│  │  │   Reconciler     │    │    Reconciler    │    │  (Scale-Up)    │   │ │
│  │  └────────┬─────────┘    └────────┬─────────┘    └───────┬────────┘   │ │
│  │           │                       │                      │            │ │
│  │           ▼                       ▼                      ▼            │ │
│  │  ┌──────────────────┐    ┌──────────────────┐    ┌────────────────┐   │ │
│  │  │  ScaleDown       │    │   State Machine  │    │  ScaleUp       │   │ │
│  │  │  Manager         │    │   (Lifecycle)    │    │  Controller    │   │ │
│  │  └────────┬─────────┘    └────────┬─────────┘    └───────┬────────┘   │ │
│  │           │                       │                      │            │ │
│  │           ▼                       ▼                      ▼            │ │
│  │  ┌──────────────────┐    ┌──────────────────┐    ┌────────────────┐   │ │
│  │  │  Safety Engine   │    │   Provisioner    │    │  Resource      │   │ │
│  │  │  (6 checks)      │    │   Joiner         │    │  Analyzer      │   │ │
│  │  │                  │    │   Drainer        │    │  (Cost-aware)  │   │ │
│  │  │                  │    │   Terminator     │    │                │   │ │
│  │  └──────────────────┘    └────────┬─────────┘    └────────────────┘   │ │
│  └───────────────────────────────────┼───────────────────────────────────┘ │
│                                      │                                      │
│  ┌───────────────────────────────────┼───────────────────────────────────┐ │
│  │                        VPSie API Client                                │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │ │
│  │  │    OAuth     │  │ Rate Limiter │  │   Circuit    │                 │ │
│  │  │   (Auto)     │  │ (100/min)    │  │   Breaker    │                 │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                 │ │
│  └───────────────────────────────────┼───────────────────────────────────┘ │
└──────────────────────────────────────┼──────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                           VPSIE CLOUD PLATFORM                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │     VMs      │  │  Offerings   │  │ Datacenters  │  │  K8s Groups  │    │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              pkg/controller/                                 │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         manager.go                                   │   │
│  │  - Bootstraps all controllers                                        │   │
│  │  - Creates VPSie client                                              │   │
│  │  - Initializes health checks                                         │   │
│  │  - Starts metrics collection                                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│              ┌─────────────────────┼─────────────────────┐                  │
│              ▼                     ▼                     ▼                  │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐       │
│  │    nodegroup/     │  │    vpsienode/     │  │     events/       │       │
│  │                   │  │                   │  │                   │       │
│  │  reconciler.go    │  │  controller.go    │  │  watcher.go       │       │
│  │  - Scale up/down  │  │  - State machine  │  │  - Pod events     │       │
│  │  - Node counting  │  │  - VPS lifecycle  │  │  - FailedSchedule │       │
│  │  - VPSieNode CRUD │  │                   │  │                   │       │
│  │                   │  │  phases.go        │  │  scaleup.go       │       │
│  │  status.go        │  │  - Pending        │  │  - Scale triggers │       │
│  │  - Condition mgmt │  │  - Provisioning   │  │  - NodeGroup match│       │
│  │                   │  │  - Ready          │  │                   │       │
│  │  conditions.go    │  │  - Terminating    │  │  analyzer.go      │       │
│  │  - Ready/Error    │  │  - Failed (TTL)   │  │  - Resource calc  │       │
│  │                   │  │                   │  │  - Cost scoring   │       │
│  └─────────┬─────────┘  │  provisioner.go   │  │                   │       │
│            │            │  - VPS creation   │  │  creator.go       │       │
│            │            │                   │  │  - Dynamic NG     │       │
│            │            │  joiner.go        │  │                   │       │
│            │            │  - K8s node join  │  └───────────────────┘       │
│            │            │                   │                               │
│            │            │  drainer.go       │                               │
│            │            │  - Pod eviction   │                               │
│            │            │                   │                               │
│            │            │  terminator.go    │                               │
│            │            │  - VPS deletion   │                               │
│            │            └─────────┬─────────┘                               │
│            │                      │                                         │
│            └──────────────────────┼─────────────────────────────────────────┤
│                                   ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                           pkg/scaler/                                │   │
│  │                                                                      │   │
│  │  scaler.go          policies.go         safety.go                   │   │
│  │  - Utilization      - Scale decisions   - 6 safety checks           │   │
│  │  - Identify nodes   - Cooldowns         - Pod rescheduling          │   │
│  │  - Max 1 node/op    - Thresholds        - Anti-affinity             │   │
│  │                                                                      │   │
│  │  drain.go                                                           │   │
│  │  - Cordon node                                                      │   │
│  │  - Evict pods                                                       │   │
│  │  - PDB compliance                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Scale-Up Workflow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│     Pod      │     │   Event      │     │   ScaleUp    │     │  NodeGroup   │
│   Pending    │────▶│   Watcher    │────▶│  Controller  │────▶│  Reconciler  │
│              │     │              │     │              │     │              │
└──────────────┘     └──────────────┘     └──────────────┘     └──────┬───────┘
                                                                       │
       ┌───────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Create     │     │  VPSieNode   │     │   VPSie      │     │     VPS      │
│  VPSieNode   │────▶│  Reconciler  │────▶│    API       │────▶│   Created    │
│     CR       │     │              │     │              │     │              │
└──────────────┘     └──────────────┘     └──────────────┘     └──────┬───────┘
                                                                       │
       ┌───────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│     VPS      │     │    Node      │     │    Node      │     │     Pod      │
│   Running    │────▶│   Joining    │────▶│    Ready     │────▶│  Scheduled   │
│              │     │              │     │              │     │              │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘

Timeline: ~2-5 minutes total
├── VPS Provisioning: 1-2 min
├── Node Joining: 30-60 sec
└── Pod Scheduling: seconds
```

---

## Scale-Down Workflow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Metrics   │     │  ScaleDown   │     │   Identify   │     │   Safety     │
│  Collection  │────▶│   Manager    │────▶│ Underutilized│────▶│   Checks     │
│   (60s)      │     │              │     │    Nodes     │     │  (6 checks)  │
└──────────────┘     └──────────────┘     └──────────────┘     └──────┬───────┘
                                                                       │
                                                            ┌──────────┴──────────┐
                                                            │                     │
                                                            ▼                     ▼
                                                     ┌──────────────┐     ┌──────────────┐
                                                     │    SAFE      │     │   BLOCKED    │
                                                     │  (proceed)   │     │   (skip)     │
                                                     └──────┬───────┘     └──────────────┘
                                                            │
       ┌────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Cordon     │     │    Evict     │     │   Delete     │     │   Delete     │
│    Node      │────▶│    Pods      │────▶│  VPSieNode   │────▶│     VPS      │
│              │     │   (w/ PDB)   │     │     CR       │     │              │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘

Safety Checks:
├── 1. No local storage pods
├── 2. Pods can be rescheduled (affinity/anti-affinity)
├── 3. No critical system pods
├── 4. No anti-affinity violations
├── 5. Cluster has capacity
└── 6. Node not protected by annotation
```

---

## VPSieNode State Machine

```
                              ┌─────────────┐
                              │   PENDING   │
                              │             │
                              └──────┬──────┘
                                     │
                    Provisioner.CreateVPS()
                                     │
                                     ▼
                              ┌─────────────┐
                              │PROVISIONING │
                              │             │
                              └──────┬──────┘
                                     │
                       VPS status = "running"
                                     │
                                     ▼
                              ┌─────────────┐
                              │ PROVISIONED │
                              │             │
                              └──────┬──────┘
                                     │
                        Joiner.RegisterNode()
                                     │
                                     ▼
                              ┌─────────────┐
                              │   JOINING   │
                              │             │
                              └──────┬──────┘
                                     │
                    Node.conditions[Ready]=True
                                     │
                                     ▼
                              ┌─────────────┐
                              │    READY    │◀─────────────────────────┐
                              │             │                          │
                              └──────┬──────┘                          │
                                     │                                 │
                           Scale-down initiated                        │
                                     │                                 │
                                     ▼                                 │
                              ┌─────────────┐                          │
                              │ TERMINATING │                          │
                              │             │                          │
                              └──────┬──────┘                          │
                                     │                                 │
                               Drain pods                              │
                                     │                                 │
                                     ▼                                 │
                              ┌─────────────┐                          │
                              │  DELETING   │                          │
                              │             │                          │
                              └──────┬──────┘                          │
                                     │                                 │
                        VPSie DELETE /servers/{id}                     │
                                     │                                 │
                                     ▼                                 │
                              ┌─────────────┐                          │
                              │   DELETED   │                          │
                              │  (removed)  │                          │
                              └─────────────┘                          │
                                                                       │
          ┌────────────────────────────────────────────────────────────┘
          │
          │  Any phase can transition to Failed:
          │
          ▼
   ┌─────────────┐
   │   FAILED    │
   │             │
   │ TTL: 30min  │──────▶ Auto-deleted after TTL
   │ (default)   │
   └─────────────┘
```

---

## CRD Relationships

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              NodeGroup CR                                    │
│                                                                              │
│  spec:                           status:                                     │
│  ├── minNodes: 2                 ├── currentNodes: 3                        │
│  ├── maxNodes: 10                ├── desiredNodes: 3                        │
│  ├── datacenterID: "dc1"         ├── readyNodes: 3                          │
│  ├── offeringIDs: [...]          ├── vpsieGroupID: 12345                    │
│  ├── kubernetesVersion: v1.28    ├── conditions: [Ready, ...]               │
│  └── scaleDownPolicy:            └── nodes: [NodeInfo, ...]                 │
│      ├── enabled: true                                                       │
│      ├── cpuThreshold: 50%                                                   │
│      └── cooldownSeconds: 600                                                │
│                                                                              │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
                                      │ owns (1:N)
                                      │
                 ┌────────────────────┼────────────────────┐
                 │                    │                    │
                 ▼                    ▼                    ▼
┌────────────────────────┐ ┌────────────────────────┐ ┌────────────────────────┐
│     VPSieNode CR #1    │ │     VPSieNode CR #2    │ │     VPSieNode CR #3    │
│                        │ │                        │ │                        │
│ spec:                  │ │ spec:                  │ │ spec:                  │
│ ├── nodeGroupName      │ │ ├── nodeGroupName      │ │ ├── nodeGroupName      │
│ ├── vpsieInstanceID    │ │ ├── vpsieInstanceID    │ │ ├── vpsieInstanceID    │
│ └── instanceType       │ │ └── instanceType       │ │ └── instanceType       │
│                        │ │                        │ │                        │
│ status:                │ │ status:                │ │ status:                │
│ ├── phase: Ready       │ │ ├── phase: Ready       │ │ ├── phase: Ready       │
│ ├── nodeName: node-1   │ │ ├── nodeName: node-2   │ │ ├── nodeName: node-3   │
│ └── vpsieStatus: run   │ │ └── vpsieStatus: run   │ │ └── vpsieStatus: run   │
│                        │ │                        │ │                        │
└───────────┬────────────┘ └───────────┬────────────┘ └───────────┬────────────┘
            │                          │                          │
            │ owns (1:1)               │ owns (1:1)               │ owns (1:1)
            │                          │                          │
            ▼                          ▼                          ▼
┌────────────────────────┐ ┌────────────────────────┐ ┌────────────────────────┐
│   Kubernetes Node #1   │ │   Kubernetes Node #2   │ │   Kubernetes Node #3   │
│                        │ │                        │ │                        │
│ labels:                │ │ labels:                │ │ labels:                │
│ └── autoscaler.vpsie.  │ │ └── autoscaler.vpsie.  │ │ └── autoscaler.vpsie.  │
│     com/nodegroup:     │ │     com/nodegroup:     │ │     com/nodegroup:     │
│     my-nodegroup       │ │     my-nodegroup       │ │     my-nodegroup       │
│                        │ │                        │ │                        │
│ status:                │ │ status:                │ │ status:                │
│ └── conditions:        │ │ └── conditions:        │ │ └── conditions:        │
│     └── Ready: True    │ │     └── Ready: True    │ │     └── Ready: True    │
└────────────────────────┘ └────────────────────────┘ └────────────────────────┘
```

---

## VPSie API Integration

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          VPSie API Client                                    │
│                        (pkg/vpsie/client/)                                   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        Request Pipeline                              │   │
│  │                                                                      │   │
│  │   HTTP Request                                                       │   │
│  │        │                                                             │   │
│  │        ▼                                                             │   │
│  │   ┌─────────────┐                                                    │   │
│  │   │ Rate Limiter│  100 requests/minute                               │   │
│  │   │   (Token    │  Blocks if exceeded                                │   │
│  │   │   Bucket)   │                                                    │   │
│  │   └──────┬──────┘                                                    │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │   ┌─────────────┐                                                    │   │
│  │   │  Circuit    │  Opens after 50% errors                            │   │
│  │   │  Breaker    │  Closes after 30s recovery                         │   │
│  │   └──────┬──────┘                                                    │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │   ┌─────────────┐                                                    │   │
│  │   │   Retry     │  3 attempts, exponential backoff                   │   │
│  │   │   Logic     │  100ms → 200ms → 400ms                             │   │
│  │   └──────┬──────┘                                                    │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │   ┌─────────────┐                                                    │   │
│  │   │    OAuth    │  Auto-refresh 5min before expiry                   │   │
│  │   │   Token     │  Credentials from K8s secret                       │   │
│  │   └──────┬──────┘                                                    │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │      TLS 1.3                                                         │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │   VPSie API Server                                                   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  API Endpoints:                                                              │
│  ├── POST   /servers              Create VPS                                │
│  ├── GET    /servers/{id}         Get VPS status                            │
│  ├── DELETE /servers/{id}         Terminate VPS                             │
│  ├── GET    /offerings            List instance types                       │
│  ├── GET    /datacenters          List regions                              │
│  └── POST   /k8s/nodegroups       Create K8s node group                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Metrics & Observability

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Observability Stack                                │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Prometheus Metrics (:8080)                      │   │
│  │                                                                      │   │
│  │  Scale-Up:                        Scale-Down:                        │   │
│  │  ├── scale_up_operations_total    ├── scale_down_operations_total   │   │
│  │  ├── nodes_created_total          ├── nodes_removed_total           │   │
│  │  ├── pending_pods_gauge           ├── safety_check_failures_total   │   │
│  │  └── scheduling_constraints       ├── node_drain_duration_seconds   │   │
│  │                                   └── node_utilization (CPU/Mem)    │   │
│  │                                                                      │   │
│  │  Node Lifecycle:                  VPSieNode TTL:                     │   │
│  │  ├── provisioning_duration_sec    └── vpsienode_ttl_deletions_total │   │
│  │  ├── joining_duration_seconds                                       │   │
│  │  └── nodegroup_size_total                                           │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Health Endpoints (:8081)                        │   │
│  │                                                                      │   │
│  │  /healthz  - Liveness probe (controller running)                    │   │
│  │  /readyz   - Readiness probe (VPSie API reachable)                  │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Structured Logging (Zap)                        │   │
│  │                                                                      │   │
│  │  Format: JSON (production) / Console (development)                  │   │
│  │  Levels: debug, info, warn, error                                   │   │
│  │  Fields: nodegroup, node, vpsId, phase, duration, requestID         │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Directory Structure

```
vpsie-k8s-autoscaler/
├── cmd/
│   └── controller/
│       └── main.go                 # Entry point, CLI flags
│
├── pkg/
│   ├── apis/autoscaler/v1alpha1/   # CRD definitions
│   │   ├── nodegroup_types.go      # NodeGroup spec/status
│   │   ├── vpsienode_types.go      # VPSieNode spec/status
│   │   └── zz_generated.deepcopy.go
│   │
│   ├── controller/
│   │   ├── manager.go              # Bootstrap all controllers
│   │   ├── options.go              # CLI configuration
│   │   ├── health.go               # Health checks
│   │   │
│   │   ├── nodegroup/              # NodeGroup controller
│   │   │   ├── reconciler.go       # Main reconciliation logic
│   │   │   ├── status.go           # Status updates
│   │   │   └── conditions.go       # Condition management
│   │   │
│   │   └── vpsienode/              # VPSieNode controller
│   │       ├── controller.go       # Main controller
│   │       ├── phases.go           # State machine
│   │       ├── provisioner.go      # VPS creation
│   │       ├── joiner.go           # Node K8s join
│   │       ├── drainer.go          # Pod eviction
│   │       └── terminator.go       # VPS deletion
│   │
│   ├── scaler/                     # Scale-down logic
│   │   ├── scaler.go               # ScaleDownManager
│   │   ├── policies.go             # PolicyEngine
│   │   ├── safety.go               # 6 safety checks
│   │   └── drain.go                # Pod eviction
│   │
│   ├── events/                     # Scale-up triggers
│   │   ├── watcher.go              # EventWatcher
│   │   ├── scaleup.go              # ScaleUpController
│   │   ├── analyzer.go             # ResourceAnalyzer
│   │   └── creator.go              # DynamicNodeGroupCreator
│   │
│   ├── vpsie/
│   │   ├── client/                 # VPSie API client
│   │   │   ├── client.go           # HTTP client
│   │   │   ├── types.go            # API types
│   │   │   └── errors.go           # Error handling
│   │   │
│   │   └── cost/                   # Cost optimization
│   │       ├── calculator.go
│   │       └── optimizer.go
│   │
│   └── metrics/                    # Prometheus metrics
│       └── metrics.go
│
└── deploy/
    ├── crds/                       # CRD manifests
    └── manifests/                  # Deployment manifests
```

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Controller Separation** | ScaleDownManager drains, NodeGroupReconciler deletes - prevents race conditions |
| **VPSieNode owns Node** | Clear ownership for cascading deletion and garbage collection |
| **EventWatcher for Scale-Up** | Faster reaction to pending pods (seconds vs minutes) |
| **Max 1 node per scale-down** | Safety: prevents aggressive scale-down |
| **TTL for Failed VPSieNodes** | Automatic cleanup of stuck resources (30min default) |
| **Multiple lookup strategies** | Ensures VPS deletion after scale-down (NodeName → SpecName → Hostname) |

---

## Configuration

```bash
vpsie-autoscaler \
  --kubeconfig /path/to/kubeconfig \
  --metrics-addr :8080 \
  --health-addr :8081 \
  --leader-election \
  --vpsie-secret-name vpsie-secret \
  --vpsie-secret-namespace kube-system \
  --default-datacenter-id us-east-1 \
  --default-offering-ids offering-1,offering-2 \
  --kubernetes-version v1.28.0 \
  --failed-vpsienode-ttl 30m \
  --log-level info \
  --log-format json
```

---

## Related Documentation

- [CLAUDE.md](../CLAUDE.md) - Development guidelines
- [API Reference](./API.md) - CRD specifications
- [Deployment Guide](./DEPLOYMENT.md) - Installation instructions
