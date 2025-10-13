# Observability Implementation

This document describes the comprehensive observability implementation for the VPSie Kubernetes Autoscaler.

## Overview

The autoscaler provides three pillars of observability:

1. **Metrics** - Prometheus metrics for quantitative monitoring
2. **Logging** - Structured logging with zap for debugging and auditing
3. **Events** - Kubernetes events for user-visible state changes

## Metrics

All metrics are exposed via Prometheus and use the `vpsie_autoscaler` namespace.

### NodeGroup Metrics

Track the state and configuration of NodeGroups:

- `vpsie_autoscaler_nodegroup_desired_nodes{nodegroup, namespace}` - Desired number of nodes
- `vpsie_autoscaler_nodegroup_current_nodes{nodegroup, namespace}` - Current number of nodes
- `vpsie_autoscaler_nodegroup_ready_nodes{nodegroup, namespace}` - Number of ready nodes
- `vpsie_autoscaler_nodegroup_min_nodes{nodegroup, namespace}` - Minimum nodes configuration
- `vpsie_autoscaler_nodegroup_max_nodes{nodegroup, namespace}` - Maximum nodes configuration

### VPSieNode Metrics

Track the lifecycle and phases of individual nodes:

- `vpsie_autoscaler_vpsienode_phase{phase, nodegroup, namespace}` - Number of nodes in each phase
- `vpsie_autoscaler_vpsienode_phase_transitions_total{from_phase, to_phase, nodegroup, namespace}` - Phase transition counts
- `vpsie_autoscaler_node_provisioning_duration_seconds{nodegroup, namespace}` - Time to provision a node (histogram)
- `vpsie_autoscaler_node_termination_duration_seconds{nodegroup, namespace}` - Time to terminate a node (histogram)

**Phases tracked:**
- Pending
- Provisioning
- Provisioned
- Joining
- Ready
- Terminating
- Deleting
- Failed

### Controller Metrics

Track controller reconciliation performance:

- `vpsie_autoscaler_controller_reconcile_duration_seconds{controller}` - Reconciliation duration (histogram, 1ms to 16s buckets)
- `vpsie_autoscaler_controller_reconcile_errors_total{controller, error_type}` - Error counts by type
- `vpsie_autoscaler_controller_reconcile_total{controller, result}` - Total reconciliations by result (success, error, requeue)

**Controllers:**
- `nodegroup`
- `vpsienode`

### VPSie API Metrics

Track API client performance and errors:

- `vpsie_autoscaler_vpsie_api_requests_total{method, status}` - Total API requests by method and status code
- `vpsie_autoscaler_vpsie_api_request_duration_seconds{method}` - API request duration (histogram, 10ms to 40s buckets)
- `vpsie_autoscaler_vpsie_api_errors_total{method, error_type}` - API errors by type

**Methods tracked:**
- GET
- POST
- PUT
- DELETE

**Error types:**
- unauthorized
- forbidden
- not_found
- rate_limited
- server_error
- client_error
- request_failed

### Scaling Metrics

Track scale-up and scale-down operations:

- `vpsie_autoscaler_scale_up_total{nodegroup, namespace}` - Total scale-up operations
- `vpsie_autoscaler_scale_down_total{nodegroup, namespace}` - Total scale-down operations
- `vpsie_autoscaler_scale_up_nodes_added{nodegroup, namespace}` - Number of nodes added per operation (histogram, 1-10 buckets)
- `vpsie_autoscaler_scale_down_nodes_removed{nodegroup, namespace}` - Number of nodes removed per operation (histogram, 1-10 buckets)

### Unschedulable Pod Metrics

Track pod scheduling issues:

- `vpsie_autoscaler_unschedulable_pods_total{constraint, namespace}` - Total unschedulable pods detected
- `vpsie_autoscaler_pending_pods_current{namespace}` - Current number of pending unschedulable pods

**Constraints:**
- cpu
- memory
- pods

### Event Emission Metrics

Track Kubernetes events emitted:

- `vpsie_autoscaler_events_emitted_total{event_type, reason, object_kind}` - Total events emitted

## Logging

### Log Levels

- **Debug** - API calls/responses, reconciliation start/complete
- **Info** - Scaling decisions, phase transitions, node lifecycle events
- **Error** - Failures and errors

### Structured Fields

All logs use structured fields for easy parsing and filtering:

- `requestID` - Unique request identifier
- `controller` - Controller name
- `nodeGroup` - NodeGroup name
- `namespace` - Kubernetes namespace
- `object` - Object name (pod, node, etc.)
- `action` - Action being performed
- `phase` - Current phase
- `duration` - Operation duration
- `reason` - Reason for action/error

### Key Log Functions

#### Scaling Decisions

```go
logging.LogScaleUpDecision(logger, nodeGroup, namespace, currentNodes, desiredNodes, nodesAdded, reason)
logging.LogScaleDownDecision(logger, nodeGroup, namespace, currentNodes, desiredNodes, nodesRemoved, reason)
```

#### API Calls

```go
logging.LogAPICall(logger, method, endpoint, requestID)
logging.LogAPIResponse(logger, method, endpoint, statusCode, duration, requestID)
logging.LogAPIError(logger, method, endpoint, statusCode, err, requestID)
```

#### Node Lifecycle

```go
logging.LogNodeProvisioningStart(logger, nodeName, nodeGroup, namespace, instanceType)
logging.LogNodeProvisioningComplete(logger, nodeName, nodeGroup, namespace, duration)
logging.LogNodeProvisioningFailed(logger, nodeName, nodeGroup, namespace, err, reason)

logging.LogNodeTerminationStart(logger, nodeName, nodeGroup, namespace)
logging.LogNodeTerminationComplete(logger, nodeName, nodeGroup, namespace, duration)
logging.LogNodeTerminationFailed(logger, nodeName, nodeGroup, namespace, err, reason)
```

#### Phase Transitions

```go
logging.LogPhaseTransition(logger, nodeName, nodeGroup, namespace, fromPhase, toPhase, reason)
```

#### Reconciliation

```go
logging.LogReconciliationStart(logger, controller, objectName, namespace)
logging.LogReconciliationComplete(logger, controller, objectName, namespace, duration, result)
logging.LogReconciliationError(logger, controller, objectName, namespace, err, errorType)
```

### Request ID Tracking

All operations can be traced using request IDs:

```go
// Add request ID to context
ctx = logging.WithRequestID(ctx)

// Get request ID from context
requestID := logging.GetRequestID(ctx)

// Add request ID to logger
logger = logging.WithRequestIDField(ctx, logger)
```

## Kubernetes Events

Events are emitted for all significant state changes and are visible in `kubectl describe` output.

### Event Types

- **Normal** - Successful operations and state changes
- **Warning** - Failures and error conditions

### NodeGroup Events

**Scale-Up:**
- `ScaleUpTriggered` - Scale-up operation started
- `ScaleUpCompleted` - Scale-up operation completed successfully
- `ScaleUpFailed` - Scale-up operation failed

**Scale-Down:**
- `ScaleDownTriggered` - Scale-down operation started
- `ScaleDownCompleted` - Scale-down operation completed successfully
- `ScaleDownFailed` - Scale-down operation failed

**General:**
- `NodeGroupUpdated` - NodeGroup configuration changed
- `NodeGroupError` - NodeGroup encountered an error

### VPSieNode Events

**Provisioning:**
- `NodeProvisioning` - Node provisioning started
- `NodeProvisioned` - Node provisioning completed
- `NodeProvisioningFailed` - Node provisioning failed
- `VPSCreated` - VPS created successfully
- `VPSCreateFailed` - VPS creation failed

**Joining:**
- `NodeJoining` - Node joining cluster
- `NodeReady` - Node became ready
- `NodeJoinFailed` - Node join failed

**Termination:**
- `NodeTerminating` - Node termination started
- `NodeTerminated` - Node termination completed
- `NodeTerminationFailed` - Node termination failed
- `NodeDraining` - Node draining started
- `NodeDrained` - Node draining completed
- `NodeDrainFailed` - Node drain failed
- `VPSDeleted` - VPS deleted successfully
- `VPSDeleteFailed` - VPS deletion failed

**Scheduling:**
- `UnschedulablePods` - Unschedulable pods detected

### Event Emission Usage

```go
// Create emitter
emitter := events.NewEventEmitter(clientset, scheme)

// Emit events
emitter.EmitScaleUpTriggered(nodeGroup, currentNodes, desiredNodes, reason)
emitter.EmitNodeProvisioning(vpsieNode, instanceType)
emitter.EmitNodeReady(vpsieNode, nodeName)
emitter.EmitVPSCreated(vpsieNode, vpsID)
```

## Integration

### VPSie API Client

The VPSie API client automatically tracks:

✅ **Metrics:**
- API request duration for all methods (GET, POST, PUT, DELETE)
- API request counts by status code
- API errors by type

✅ **Logging:**
- Debug logs for all API calls (method, endpoint, request ID)
- Debug logs for all API responses (status code, duration, request ID)
- Error logs for API failures (status code, error message, request ID)

### Controllers (To Be Integrated)

**NodeGroup Controller** - Will track:
- Reconciliation duration and errors
- NodeGroup state metrics
- Scale-up/down operations
- Events for scaling operations

**VPSieNode Controller** - Will track:
- Reconciliation duration and errors
- Phase distribution and transitions
- Node provisioning/termination duration
- Events for node lifecycle

**Event Watcher** - Will track:
- Unschedulable pod detection
- Pending pod counts
- Events for scheduling issues

## Prometheus Integration

### Metrics Endpoint

Metrics are exposed on the `/metrics` endpoint (default port TBD by controller configuration).

### Sample Prometheus Queries

**NodeGroup capacity utilization:**
```promql
vpsie_autoscaler_nodegroup_current_nodes / vpsie_autoscaler_nodegroup_max_nodes
```

**Average node provisioning time:**
```promql
rate(vpsie_autoscaler_node_provisioning_duration_seconds_sum[5m])
/
rate(vpsie_autoscaler_node_provisioning_duration_seconds_count[5m])
```

**API error rate:**
```promql
rate(vpsie_autoscaler_vpsie_api_errors_total[5m])
```

**Scale-up operations per hour:**
```promql
increase(vpsie_autoscaler_scale_up_total[1h])
```

**Controller reconciliation P95:**
```promql
histogram_quantile(0.95, rate(vpsie_autoscaler_controller_reconcile_duration_seconds_bucket[5m]))
```

### Sample Alerts

**High API error rate:**
```yaml
alert: HighVPSieAPIErrorRate
expr: rate(vpsie_autoscaler_vpsie_api_errors_total[5m]) > 0.1
annotations:
  summary: High VPSie API error rate detected
```

**NodeGroup at max capacity:**
```yaml
alert: NodeGroupAtMaxCapacity
expr: vpsie_autoscaler_nodegroup_current_nodes >= vpsie_autoscaler_nodegroup_max_nodes
annotations:
  summary: NodeGroup {{ $labels.nodegroup }} is at maximum capacity
```

**Slow node provisioning:**
```yaml
alert: SlowNodeProvisioning
expr: histogram_quantile(0.95, rate(vpsie_autoscaler_node_provisioning_duration_seconds_bucket[15m])) > 600
annotations:
  summary: Node provisioning is taking longer than 10 minutes
```

## Grafana Dashboard

A sample Grafana dashboard can be found at `deploy/grafana/autoscaler-dashboard.json` (to be created).

### Recommended Panels

1. **NodeGroup Overview** - Current/desired/ready nodes per NodeGroup
2. **Scaling Activity** - Scale-up/down operations over time
3. **Node Provisioning** - Provisioning duration heatmap
4. **API Health** - Request rate, error rate, latency
5. **Controller Performance** - Reconciliation duration and error rate
6. **Phase Distribution** - VPSieNode phases over time
7. **Unschedulable Pods** - Pending pod count and constraints

## Best Practices

1. **Always use request IDs** - Add request ID to context for all operations
2. **Log decisions, not actions** - Log why something happened, not what happened (that's in metrics)
3. **Use appropriate log levels** - Debug for tracing, Info for decisions, Error for failures
4. **Emit events for user-visible changes** - Users should see important state changes in kubectl describe
5. **Monitor P95/P99 latencies** - Don't just look at averages
6. **Set up alerts** - Proactive monitoring prevents incidents
7. **Use structured logging** - Makes log parsing and filtering much easier

## Troubleshooting

### No metrics appearing

1. Check metrics endpoint is accessible: `curl http://localhost:8080/metrics`
2. Verify Prometheus scrape config includes autoscaler pods
3. Check controller logs for metrics registration errors

### Missing logs

1. Verify log level configuration (set to debug for verbose logging)
2. Check logger initialization in controller startup
3. Ensure logger is passed to all components

### Events not showing up

1. Check RBAC permissions for event creation
2. Verify EventEmitter is initialized with correct scheme
3. Look for event emission errors in controller logs

## Performance Considerations

- Metrics have minimal overhead (~microseconds per observation)
- Debug logging can be disabled in production for performance
- Event emission is asynchronous and non-blocking
- Request ID generation uses UUID v4 (cryptographically random)

## Future Enhancements

- [ ] Distributed tracing with OpenTelemetry
- [ ] Log aggregation with ELK/Loki
- [ ] Custom metrics endpoint port configuration
- [ ] Metric label cardinality limits
- [ ] Grafana dashboard templates
- [ ] Alert rule templates for Prometheus
- [ ] Performance benchmarks for observability overhead
