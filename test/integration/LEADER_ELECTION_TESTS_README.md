# Leader Election Integration Tests

## Overview

This document describes the comprehensive leader election integration tests for the VPSie Kubernetes Node Autoscaler controller, ensuring high availability and proper failover capabilities.

## Test Coverage

### 1. TestLeaderElection_Integration

Tests basic leader election functionality with multiple controller instances.

**Features Tested:**
- Multi-instance controller startup with leader election
- Single leader guarantee
- Leader identification via health endpoints
- Work distribution (only leader reconciles)
- Leader election metrics exposure

**Test Flow:**
1. Start mock VPSie server
2. Create VPSie secret for authentication
3. Start 3 controller instances with same leader-election-id
4. Wait for leader election to complete (10 seconds)
5. Verify exactly one controller becomes leader
6. Check health endpoints reflect leadership status
7. Create NodeGroup and verify only leader reconciles
8. Verify leader election metrics are exposed

**Key Validations:**

#### Single Leader
```go
// Exactly one controller should be leader
leader, leaderCount := identifyLeader(controllers)
assert.Equal(t, 1, leaderCount)
```

#### Health Status
- Leader: `/readyz` returns 200 OK
- Non-leaders: `/readyz` may return 503 (implementation dependent)

#### Work Distribution
- Only leader's reconciliation counter increases
- Only leader creates VPSieNodes
- Non-leaders remain idle

**Port Allocation:**
| Controller | Metrics Port | Health Port |
|------------|-------------|-------------|
| Instance 0 | 19000 | 19100 |
| Instance 1 | 19001 | 19101 |
| Instance 2 | 19002 | 19102 |

### 2. TestLeaderElection_Handoff

Tests leader handoff when the current leader stops.

**Features Tested:**
- Graceful leader transition
- New leader election within 15 seconds
- Work continuity during handoff
- No resource loss

**Test Flow:**
1. Start 2 controllers with leader election
2. Identify initial leader
3. Create NodeGroup and verify initial reconciliation
4. Stop current leader controller
5. Verify remaining controller becomes leader
6. Update NodeGroup to trigger new reconciliation
7. Verify new leader takes over work
8. Confirm no work is lost during transition

**Key Validations:**

#### Leader Transition
```go
// Remaining controller becomes leader within 15 seconds
require.Eventually(t, func() bool {
    return isControllerLeader(remainingController)
}, 15*time.Second, 1*time.Second)
```

#### Work Continuity
- Initial VPSieNodes remain intact
- New leader processes updates
- NodeGroup status remains consistent
- All resources have valid specs

**Handoff Timeline:**
| Time | Event |
|------|-------|
| T+0s | Initial leader identified |
| T+5s | NodeGroup created, VPSieNodes provisioned |
| T+10s | Leader stopped |
| T+15s | New leader elected |
| T+20s | New leader reconciling |

### 3. TestLeaderElection_SplitBrain

Tests split-brain prevention mechanisms.

**Features Tested:**
- Kubernetes Lease-based election
- Single lease holder guarantee
- Lease renewal mechanisms
- Split-brain prevention

**Note:** This test is currently skipped as it requires network manipulation capabilities. In production environments, it would test:
- Network partition simulation
- Lease expiration handling
- Leader convergence after partition healing

**Lease Validation:**
```go
// Verify lease exists and has proper configuration
assert.NotNil(t, lease.Spec.HolderIdentity)
assert.NotNil(t, lease.Spec.LeaseDurationSeconds)
assert.NotNil(t, lease.Spec.RenewTime)
```

**Lease Properties:**
- Duration: Typically 15-30 seconds
- Renew Interval: Duration/2
- Retry Period: 2 seconds

## Leader Election Mechanism

### Kubernetes Lease Objects

The controller uses Kubernetes Lease objects for leader election:

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: test-leader-xxxxx
  namespace: vpsie-autoscaler-test
spec:
  holderIdentity: controller-instance-id
  leaseDurationSeconds: 15
  renewTime: 2024-01-20T10:30:00Z
  acquireTime: 2024-01-20T10:29:45Z
```

### Election Process

1. **Initial Election:**
   - All controllers attempt to create/acquire lease
   - First successful controller becomes leader
   - Others become followers

2. **Lease Renewal:**
   - Leader renews lease every `leaseDuration/2` seconds
   - Prevents lease expiration during normal operation

3. **Leader Failure:**
   - Leader stops renewing lease
   - Lease expires after `leaseDurationSeconds`
   - Followers detect expiration and compete for leadership

4. **New Leader Election:**
   - Followers attempt to update lease with their identity
   - First successful update becomes new leader
   - Process typically completes within 15-30 seconds

## Helper Functions

### Controller Management

```go
// Start multiple controllers with leader election
controllers, err := startMultipleControllersWithLeaderElection(
    count,
    secretName,
    secretNamespace,
    leaderElectionID,
)

// Stop all controllers
stopAllControllers(controllers)

// Identify current leader
leader, leaderCount := identifyLeader(controllers)

// Check if specific controller is leader
isLeader := isControllerLeader(controller)
```

### Port Management

Each controller instance requires unique ports:

```go
baseMetricsPort := 19000
baseHealthPort := 19100

for i := 0; i < count; i++ {
    metricsPort := baseMetricsPort + i
    healthPort := baseHealthPort + i
    // Start controller with these ports
}
```

## Running the Tests

### Run All Leader Election Tests
```bash
go test -v -tags=integration ./test/integration \
  -run "TestLeaderElection" -timeout 300s
```

### Run Individual Tests
```bash
# Basic leader election
go test -v -tags=integration ./test/integration \
  -run TestLeaderElection_Integration -timeout 120s

# Leader handoff
go test -v -tags=integration ./test/integration \
  -run TestLeaderElection_Handoff -timeout 120s

# Split-brain prevention (if enabled)
go test -v -tags=integration ./test/integration \
  -run TestLeaderElection_SplitBrain -timeout 180s
```

### Debug Mode
```bash
# Enable debug logging for leader election
LEADER_ELECTION_DEBUG=true go test -v -tags=integration \
  ./test/integration -run TestLeaderElection
```

## Configuration

### Leader Election Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| --leader-election | false | Enable leader election |
| --leader-election-id | "" | Unique election ID |
| --leader-election-namespace | "" | Namespace for lease object |
| --leader-election-lease-duration | 15s | Lease duration |
| --leader-election-renew-deadline | 10s | Renew deadline |
| --leader-election-retry-period | 2s | Retry period |

### Test Configuration

```go
// Unique leader election ID per test
leaderElectionID := "test-leader-" + fmt.Sprintf("%d", time.Now().Unix())

// Common test timeouts
leaderElectionTimeout := 10 * time.Second
handoffTimeout := 15 * time.Second
reconciliationWait := 10 * time.Second
```

## Metrics

### Leader Election Metrics

| Metric | Description |
|--------|-------------|
| leader_election_master_status | 1 if leader, 0 if follower |
| leader_election_transitions_total | Number of leader transitions |
| leader_election_lease_duration_seconds | Current lease duration |

### Controller Metrics During Election

Leaders show active metrics:
- `vpsie_autoscaler_controller_reconcile_total` > 0
- `vpsie_autoscaler_nodegroup_current_nodes` updated

Followers show idle metrics:
- `vpsie_autoscaler_controller_reconcile_total` = 0
- No resource updates

## Troubleshooting

### Common Issues

#### 1. Multiple Leaders Detected

**Symptoms:**
- More than one controller shows as leader
- Duplicate resource creation

**Solutions:**
- Verify all controllers use same leader-election-id
- Check lease object in Kubernetes
- Ensure network connectivity between controllers

#### 2. No Leader Elected

**Symptoms:**
- No controller becomes leader
- All controllers remain idle

**Solutions:**
- Check RBAC permissions for Lease objects
- Verify namespace exists
- Check controller logs for election errors

#### 3. Slow Leader Handoff

**Symptoms:**
- Takes > 30 seconds for new leader
- Extended downtime during transition

**Solutions:**
- Reduce lease duration
- Check controller health
- Verify Kubernetes API server responsiveness

#### 4. Leader Flapping

**Symptoms:**
- Leadership changes frequently
- Unstable reconciliation

**Solutions:**
- Increase lease duration
- Check for network issues
- Verify controller stability

### Debug Commands

```bash
# List lease objects
kubectl get leases -n vpsie-autoscaler-test

# Describe specific lease
kubectl describe lease test-leader-xxxxx -n vpsie-autoscaler-test

# Watch lease updates
kubectl get leases -n vpsie-autoscaler-test -w

# Check controller logs
kubectl logs <controller-pod> | grep -i "leader"

# Monitor metrics
curl http://localhost:19000/metrics | grep leader_election
```

## Best Practices

### 1. Unique Election IDs
```go
// Use timestamp to ensure uniqueness across test runs
leaderElectionID := fmt.Sprintf("test-leader-%d", time.Now().Unix())
```

### 2. Proper Cleanup
```go
// Always defer cleanup
defer stopAllControllers(controllers)
```

### 3. Wait for Stabilization
```go
// Allow time for election to complete
time.Sleep(10 * time.Second)
```

### 4. Verify Before and After
```go
// Check initial state
initialLeader, _ := identifyLeader(controllers)

// Perform action
stopLeader(initialLeader)

// Verify new state
newLeader, _ := identifyLeader(controllers)
assert.NotEqual(t, initialLeader, newLeader)
```

## High Availability Considerations

### Production Deployment

For production HA setup:

1. **Minimum 3 Controllers**: Provides quorum and tolerates 1 failure
2. **Spread Across Nodes**: Use anti-affinity rules
3. **Resource Limits**: Prevent resource starvation
4. **Health Probes**: Ensure quick failure detection

### Example HA Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vpsie-autoscaler
spec:
  replicas: 3
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: vpsie-autoscaler
            topologyKey: kubernetes.io/hostname
      containers:
      - name: controller
        args:
        - --leader-election=true
        - --leader-election-id=vpsie-autoscaler
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Leader Election Tests

on:
  push:
    branches: [main]
  pull_request:

jobs:
  leader-election:
    runs-on: ubuntu-latest
    timeout-minutes: 15

    steps:
    - uses: actions/checkout@v3

    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.25'

    - name: Create kind cluster
      run: |
        kind create cluster
        kubectl apply -f deploy/crds/

    - name: Build controller
      run: make build

    - name: Run leader election tests
      run: |
        go test -v -tags=integration ./test/integration \
          -run TestLeaderElection \
          -timeout 300s

    - name: Collect logs on failure
      if: failure()
      run: |
        kubectl get leases -A
        kubectl logs -l app=vpsie-autoscaler --tail=100
```

## Related Documentation

- [Controller Integration Tests](CONTROLLER_TESTS_README.md)
- [Shutdown Tests](SHUTDOWN_TESTS_README.md)
- [Test Helpers](test_helpers.go)
- [Kubernetes Leader Election](https://kubernetes.io/blog/2016/01/simple-leader-election-with-kubernetes/)
- [Controller Runtime Leader Election](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/leaderelection)