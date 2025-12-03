package rebalancer

import (
	"context"
	"fmt"
	"time"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Executor executes rebalancing plans by provisioning and draining nodes
type Executor struct {
	kubeClient  kubernetes.Interface
	vpsieClient *client.Client
	config      *ExecutorConfig
}

// NewExecutor creates a new rebalance executor
func NewExecutor(kubeClient kubernetes.Interface, vpsieClient *client.Client, config *ExecutorConfig) *Executor {
	if config == nil {
		config = &ExecutorConfig{
			DrainTimeout:        5 * time.Minute,
			ProvisionTimeout:    10 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          3,
		}
	}

	return &Executor{
		kubeClient:  kubeClient,
		vpsieClient: vpsieClient,
		config:      config,
	}
}

// ExecuteRebalance executes a complete rebalancing plan
func (e *Executor) ExecuteRebalance(ctx context.Context, plan *RebalancePlan) (*RebalanceResult, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting rebalance execution",
		"planID", plan.ID,
		"strategy", plan.Strategy,
		"batches", len(plan.Batches))

	result := &RebalanceResult{
		PlanID:          plan.ID,
		Status:          StatusInProgress,
		NodesRebalanced: 0,
		NodesFailed:     0,
		Errors:          make([]error, 0),
	}

	state := &ExecutionState{
		PlanID:           plan.ID,
		Status:           StatusInProgress,
		CurrentBatch:     0,
		CompletedNodes:   make([]string, 0),
		FailedNodes:      make([]NodeFailure, 0),
		ProvisionedNodes: make([]string, 0),
		StartedAt:        time.Now(),
	}

	startTime := time.Now()

	// Execute each batch in order
	for i, batch := range plan.Batches {
		logger.Info("Executing batch",
			"batchNumber", batch.BatchNumber,
			"nodes", len(batch.Nodes))

		state.CurrentBatch = i

		// Execute batch based on strategy
		batchResult, err := e.executeBatch(ctx, plan, &batch, state)
		if err != nil {
			logger.Error(err, "Batch execution failed", "batchNumber", batch.BatchNumber)

			// Attempt rollback
			if plan.RollbackPlan != nil && plan.RollbackPlan.AutoRollback {
				logger.Info("Initiating automatic rollback")
				rollbackErr := e.Rollback(ctx, plan, state)
				if rollbackErr != nil {
					logger.Error(rollbackErr, "Rollback failed")
					result.Errors = append(result.Errors, rollbackErr)
				}
			}

			result.Status = StatusFailed
			result.Errors = append(result.Errors, err)
			result.Duration = time.Since(startTime)
			return result, err
		}

		// Update results
		result.NodesRebalanced += batchResult.NodesRebalanced
		result.NodesFailed += batchResult.NodesFailed
		state.CompletedNodes = append(state.CompletedNodes, batchResult.CompletedNodes...)
		state.FailedNodes = append(state.FailedNodes, batchResult.FailedNodes...)
	}

	// Success
	completedAt := time.Now()
	state.CompletedAt = &completedAt
	state.Status = StatusCompleted

	result.Status = StatusCompleted
	result.Duration = time.Since(startTime)
	result.SavingsRealized = plan.Optimization.MonthlySavings

	logger.Info("Rebalance execution completed",
		"planID", plan.ID,
		"nodesRebalanced", result.NodesRebalanced,
		"duration", result.Duration)

	return result, nil
}

// executeBatch executes a single batch of node replacements
func (e *Executor) executeBatch(ctx context.Context, plan *RebalancePlan, batch *NodeBatch, state *ExecutionState) (*batchResult, error) {
	logger := log.FromContext(ctx)

	switch plan.Strategy {
	case StrategyRolling:
		return e.executeRollingBatch(ctx, plan, batch, state)
	case StrategySurge:
		return e.executeSurgeBatch(ctx, plan, batch, state)
	case StrategyBlueGreen:
		return e.executeBlueGreenBatch(ctx, plan, batch, state)
	default:
		logger.Error(fmt.Errorf("unknown strategy"), "Strategy not supported", "strategy", plan.Strategy)
		return e.executeRollingBatch(ctx, plan, batch, state) // Fallback to rolling
	}
}

// executeRollingBatch executes rolling replacement (one-by-one or small batches)
func (e *Executor) executeRollingBatch(ctx context.Context, plan *RebalancePlan, batch *NodeBatch, state *ExecutionState) (*batchResult, error) {
	logger := log.FromContext(ctx)
	result := &batchResult{
		CompletedNodes: make([]string, 0),
		FailedNodes:    make([]NodeFailure, 0),
	}

	// For each node in batch: provision new, drain old, terminate old
	for _, candidate := range batch.Nodes {
		logger.Info("Replacing node",
			"nodeName", candidate.NodeName,
			"currentOffering", candidate.CurrentOffering,
			"targetOffering", candidate.TargetOffering)

		// Step 1: Provision new node
		newNode, err := e.provisionNewNode(ctx, plan, &candidate)
		if err != nil {
			logger.Error(err, "Failed to provision new node", "nodeName", candidate.NodeName)
			result.FailedNodes = append(result.FailedNodes, NodeFailure{
				NodeName:  candidate.NodeName,
				Operation: "provision",
				Error:     err,
				Timestamp: time.Now(),
			})
			result.NodesFailed++
			continue
		}

		state.ProvisionedNodes = append(state.ProvisionedNodes, newNode.Name)

		// Step 2: Wait for new node to be ready
		err = e.waitForNodeReady(ctx, newNode)
		if err != nil {
			logger.Error(err, "New node failed to become ready", "nodeName", newNode.Name)
			// Terminate failed node
			_ = e.TerminateNode(ctx, newNode)
			result.FailedNodes = append(result.FailedNodes, NodeFailure{
				NodeName:  candidate.NodeName,
				Operation: "node_ready",
				Error:     err,
				Timestamp: time.Now(),
			})
			result.NodesFailed++
			continue
		}

		// Step 3: Drain old node
		oldNode := &Node{Name: candidate.NodeName}
		err = e.DrainNode(ctx, oldNode)
		if err != nil {
			logger.Error(err, "Failed to drain old node", "nodeName", candidate.NodeName)
			result.FailedNodes = append(result.FailedNodes, NodeFailure{
				NodeName:  candidate.NodeName,
				Operation: "drain",
				Error:     err,
				Timestamp: time.Now(),
			})
			result.NodesFailed++
			// Don't continue - leave both nodes running for manual intervention
			continue
		}

		// Step 4: Terminate old node
		err = e.TerminateNode(ctx, oldNode)
		if err != nil {
			logger.Error(err, "Failed to terminate old node", "nodeName", candidate.NodeName)
			result.FailedNodes = append(result.FailedNodes, NodeFailure{
				NodeName:  candidate.NodeName,
				Operation: "terminate",
				Error:     err,
				Timestamp: time.Now(),
			})
			result.NodesFailed++
			continue
		}

		result.CompletedNodes = append(result.CompletedNodes, candidate.NodeName)
		result.NodesRebalanced++
		logger.Info("Node successfully replaced", "oldNode", candidate.NodeName, "newNode", newNode.Name)
	}

	return result, nil
}

// executeSurgeBatch executes surge replacement (provision all, then drain all)
func (e *Executor) executeSurgeBatch(ctx context.Context, plan *RebalancePlan, batch *NodeBatch, state *ExecutionState) (*batchResult, error) {
	logger := log.FromContext(ctx)
	result := &batchResult{
		CompletedNodes: make([]string, 0),
		FailedNodes:    make([]NodeFailure, 0),
	}

	newNodes := make([]*Node, 0)

	// Phase 1: Provision all new nodes
	logger.Info("Surge strategy: provisioning all new nodes", "count", len(batch.Nodes))
	for _, candidate := range batch.Nodes {
		newNode, err := e.provisionNewNode(ctx, plan, &candidate)
		if err != nil {
			logger.Error(err, "Failed to provision new node", "nodeName", candidate.NodeName)
			result.FailedNodes = append(result.FailedNodes, NodeFailure{
				NodeName:  candidate.NodeName,
				Operation: "provision",
				Error:     err,
				Timestamp: time.Now(),
			})
			result.NodesFailed++
			continue
		}
		newNodes = append(newNodes, newNode)
		state.ProvisionedNodes = append(state.ProvisionedNodes, newNode.Name)
	}

	// Wait for all new nodes to be ready
	for _, newNode := range newNodes {
		err := e.waitForNodeReady(ctx, newNode)
		if err != nil {
			logger.Error(err, "New node failed to become ready", "nodeName", newNode.Name)
			result.NodesFailed++
		}
	}

	// Phase 2: Drain and terminate all old nodes
	logger.Info("Surge strategy: draining all old nodes", "count", len(batch.Nodes))
	for _, candidate := range batch.Nodes {
		oldNode := &Node{Name: candidate.NodeName}

		err := e.DrainNode(ctx, oldNode)
		if err != nil {
			logger.Error(err, "Failed to drain old node", "nodeName", candidate.NodeName)
			result.NodesFailed++
			continue
		}

		err = e.TerminateNode(ctx, oldNode)
		if err != nil {
			logger.Error(err, "Failed to terminate old node", "nodeName", candidate.NodeName)
			result.NodesFailed++
			continue
		}

		result.CompletedNodes = append(result.CompletedNodes, candidate.NodeName)
		result.NodesRebalanced++
	}

	return result, nil
}

// executeBlueGreenBatch executes blue-green replacement
func (e *Executor) executeBlueGreenBatch(ctx context.Context, plan *RebalancePlan, batch *NodeBatch, state *ExecutionState) (*batchResult, error) {
	// Blue-green is similar to surge but with more explicit phases
	return e.executeSurgeBatch(ctx, plan, batch, state)
}

// ProvisionNode provisions a new node with the target instance type
func (e *Executor) provisionNewNode(ctx context.Context, plan *RebalancePlan, candidate *CandidateNode) (*Node, error) {
	logger := log.FromContext(ctx)
	logger.Info("Provisioning new node",
		"offering", candidate.TargetOffering,
		"nodeGroup", plan.NodeGroupName)

	// Create node spec (placeholder for future VPSie API integration)
	// TODO: Use spec to provision actual VPS instance via VPSie API
	_ = &NodeSpec{
		NodeGroupName: plan.NodeGroupName,
		Namespace:     plan.Namespace,
		OfferingID:    candidate.TargetOffering,
		// Other fields would be populated from NodeGroup spec
	}

	// This is a placeholder - actual implementation would call VPSie API
	// to provision a new VPS instance
	newNode := &Node{
		Name:       fmt.Sprintf("%s-new-%d", candidate.NodeName, time.Now().Unix()),
		OfferingID: candidate.TargetOffering,
		Status:     corev1.NodeReady, // Use NodeReady as default condition type
	}

	logger.Info("Node provisioned", "nodeName", newNode.Name)
	return newNode, nil
}

// DrainNode safely drains workloads from a node
func (e *Executor) DrainNode(ctx context.Context, node *Node) error {
	logger := log.FromContext(ctx)
	logger.Info("Draining node", "nodeName", node.Name)

	// Get the Kubernetes node
	k8sNode, err := e.kubeClient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", node.Name, err)
	}

	// Cordon the node
	k8sNode.Spec.Unschedulable = true
	_, err = e.kubeClient.CoreV1().Nodes().Update(ctx, k8sNode, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %w", node.Name, err)
	}

	logger.Info("Node cordoned", "nodeName", node.Name)

	// Get pods on the node
	pods, err := e.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", node.Name, err)
	}

	// Evict each pod
	for _, pod := range pods.Items {
		// Skip DaemonSet pods
		if e.isDaemonSetPod(&pod) {
			continue
		}

		// Skip mirror pods
		if e.isMirrorPod(&pod) {
			continue
		}

		logger.Info("Evicting pod", "pod", pod.Name, "namespace", pod.Namespace)

		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}

		err = e.kubeClient.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
		if err != nil {
			logger.Error(err, "Failed to evict pod", "pod", pod.Name)
			// Continue with other pods
		}
	}

	// Wait for pods to be evicted (with timeout)
	// PollUntilContextTimeout creates its own timeout context internally,
	// but respects parent context cancellation for graceful shutdown
	err = wait.PollUntilContextTimeout(ctx, e.config.HealthCheckInterval, e.config.DrainTimeout, true, func(pollCtx context.Context) (bool, error) {
		pods, err := e.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
		})
		if err != nil {
			return false, err
		}

		// Count non-DaemonSet, non-mirror pods
		count := 0
		for _, pod := range pods.Items {
			if !e.isDaemonSetPod(&pod) && !e.isMirrorPod(&pod) {
				count++
			}
		}

		if count == 0 {
			return true, nil
		}

		logger.Info("Waiting for pods to drain", "nodeName", node.Name, "remainingPods", count)
		return false, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for node %s to drain: %w", node.Name, err)
	}

	logger.Info("Node drained successfully", "nodeName", node.Name)
	return nil
}

// TerminateNode terminates an old node after draining
func (e *Executor) TerminateNode(ctx context.Context, node *Node) error {
	logger := log.FromContext(ctx)
	logger.Info("Terminating node", "nodeName", node.Name)

	// Delete from Kubernetes
	err := e.kubeClient.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete node %s from Kubernetes: %w", node.Name, err)
	}

	// Terminate VPS instance
	if node.VPSID > 0 {
		logger.Info("Terminating VPS instance", "vpsID", node.VPSID)
		// TODO: Implement VPSie API call with context propagation
		// Example: if err := e.vpsieClient.DeleteVPS(ctx, node.VPSID); err != nil {
		//     return fmt.Errorf("failed to delete VPS %d: %w", node.VPSID, err)
		// }
	}

	logger.Info("Node terminated successfully", "nodeName", node.Name)
	return nil
}

// Rollback reverts a failed rebalancing operation
func (e *Executor) Rollback(ctx context.Context, plan *RebalancePlan, state *ExecutionState) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting rollback", "planID", plan.ID)

	state.Status = StatusRollingBack

	if plan.RollbackPlan == nil {
		return fmt.Errorf("no rollback plan available")
	}

	// Execute rollback steps in order
	for _, step := range plan.RollbackPlan.Steps {
		logger.Info("Executing rollback step", "step", step.Order, "description", step.Description)

		switch step.Action {
		case "pause_execution":
			state.Status = StatusPaused
		case "uncordon_old_nodes":
			// Uncordon any cordoned old nodes
			// Implementation would iterate through state and uncordon nodes
		case "terminate_new_nodes":
			// Terminate newly provisioned nodes
			for _, nodeName := range state.ProvisionedNodes {
				node := &Node{Name: nodeName}
				err := e.TerminateNode(ctx, node)
				if err != nil {
					logger.Error(err, "Failed to terminate node during rollback", "nodeName", nodeName)
				}
			}
		case "verify_workloads":
			// Verify workloads are running
			// Implementation would check pod status
		case "update_status":
			state.Status = StatusFailed
		}
	}

	logger.Info("Rollback completed", "planID", plan.ID)
	return nil
}

// Helper functions

func (e *Executor) waitForNodeReady(ctx context.Context, node *Node) error {
	logger := log.FromContext(ctx)
	logger.Info("Waiting for node to be ready", "nodeName", node.Name)

	// PollUntilContextTimeout creates its own timeout context internally
	err := wait.PollUntilContextTimeout(ctx, e.config.HealthCheckInterval, e.config.ProvisionTimeout, true, func(pollCtx context.Context) (bool, error) {
		k8sNode, err := e.kubeClient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			return false, nil // Node not yet registered
		}

		for _, condition := range k8sNode.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("timeout waiting for node %s to be ready: %w", node.Name, err)
	}

	logger.Info("Node is ready", "nodeName", node.Name)
	return nil
}

func (e *Executor) isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func (e *Executor) isMirrorPod(pod *corev1.Pod) bool {
	_, isMirror := pod.Annotations[corev1.MirrorPodAnnotationKey]
	return isMirror
}

// batchResult contains the results of executing a single batch
type batchResult struct {
	NodesRebalanced int32
	NodesFailed     int32
	CompletedNodes  []string
	FailedNodes     []NodeFailure
}
