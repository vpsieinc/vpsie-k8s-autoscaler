package scaler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	// Default scale-down thresholds
	DefaultCPUThreshold      = 50.0 // Scale down if CPU < 50%
	DefaultMemoryThreshold   = 50.0 // Scale down if Memory < 50%
	DefaultObservationWindow = 10 * time.Minute
	DefaultCooldownPeriod    = 10 * time.Minute

	// Annotations for node protection
	ProtectedNodeAnnotation = "autoscaler.vpsie.com/protected"
	ScaleDownDisabledLabel  = "autoscaler.vpsie.com/scale-down-disabled"
)

// ScaleDownManager manages node scale-down operations.
//
// Architecture Note:
// The ScaleDownManager is responsible for IDENTIFYING and PREPARING nodes
// for scale-down, but NOT for deleting nodes from Kubernetes. This separation
// of concerns ensures:
//
//  1. Safe Pod Eviction: The manager drains nodes by cordoning and evicting pods,
//     ensuring workloads are safely migrated before removal.
//
//  2. Controller Responsibility: The NodeGroup controller is responsible for the
//     actual node deletion after verifying the node is fully drained and the
//     corresponding VPSie VM is terminated.
//
//  3. Clean State Management: This prevents race conditions between node draining
//     and VM termination, ensuring consistent state in both Kubernetes and VPSie.
//
// Workflow:
// - ScaleDownManager: Identifies underutilized nodes → Drains pods → Marks ready for deletion
// - NodeGroup Controller: Terminates VPSie VM → Deletes Kubernetes Node object
type ScaleDownManager struct {
	client        kubernetes.Interface
	metricsClient metricsv1beta1.Interface
	logger        *zap.SugaredLogger

	// Node utilization tracking
	nodeUtilization map[string]*NodeUtilization
	utilizationLock sync.RWMutex

	// Configuration
	config *Config

	// State tracking
	lastScaleDown map[string]time.Time // nodegroup -> last scale down time
	scaleDownLock sync.RWMutex

	// Policy engine
	policyEngine *PolicyEngine
}

// Config holds configuration for scale-down operations
type Config struct {
	CPUThreshold              float64
	MemoryThreshold           float64
	ObservationWindow         time.Duration
	CooldownPeriod            time.Duration
	MaxNodesPerScaleDown      int
	EnablePodDisruptionBudget bool
	DrainTimeout              time.Duration
	EvictionGracePeriod       int32
}

// NodeUtilization tracks resource utilization for a node
type NodeUtilization struct {
	NodeName          string
	CPUUtilization    float64 // percentage (0-100)
	MemoryUtilization float64 // percentage (0-100)
	Samples           []UtilizationSample
	LastUpdated       time.Time
	IsUnderutilized   bool
}

// UtilizationSample represents a point-in-time utilization measurement
type UtilizationSample struct {
	Timestamp         time.Time
	CPUUtilization    float64
	MemoryUtilization float64
}

// ScaleDownCandidate represents a node that can be scaled down
type ScaleDownCandidate struct {
	Node         *corev1.Node
	Utilization  *NodeUtilization
	Pods         []*corev1.Pod
	SafeToRemove bool
	Reason       string
	Priority     int // Lower priority nodes removed first
}

// NewScaleDownManager creates a new ScaleDownManager
func NewScaleDownManager(
	client kubernetes.Interface,
	metricsClient metricsv1beta1.Interface,
	logger *zap.Logger,
	config *Config,
) *ScaleDownManager {
	if config == nil {
		config = DefaultConfig()
	}

	return &ScaleDownManager{
		client:          client,
		metricsClient:   metricsClient,
		logger:          logger.Sugar(),
		nodeUtilization: make(map[string]*NodeUtilization),
		config:          config,
		lastScaleDown:   make(map[string]time.Time),
		policyEngine:    NewPolicyEngine(logger.Sugar(), config),
	}
}

// DefaultConfig returns default scale-down configuration
func DefaultConfig() *Config {
	return &Config{
		CPUThreshold:              DefaultCPUThreshold,
		MemoryThreshold:           DefaultMemoryThreshold,
		ObservationWindow:         DefaultObservationWindow,
		CooldownPeriod:            DefaultCooldownPeriod,
		MaxNodesPerScaleDown:      5,
		EnablePodDisruptionBudget: true,
		DrainTimeout:              5 * time.Minute,
		EvictionGracePeriod:       30,
	}
}

// IdentifyUnderutilizedNodes finds nodes with low utilization.
// Only processes NodeGroups that have the managed label (autoscaler.vpsie.com/managed=true).
func (s *ScaleDownManager) IdentifyUnderutilizedNodes(
	ctx context.Context,
	nodeGroup *autoscalerv1alpha1.NodeGroup,
) ([]*ScaleDownCandidate, error) {
	// Log with correlation ID if available
	requestID := logging.GetRequestID(ctx)
	if requestID != "" {
		s.logger.Infow("identifying underutilized nodes",
			"nodeGroup", nodeGroup.Name,
			"requestID", requestID)
	}

	// NodeGroup isolation: Defensive check to ensure only managed NodeGroups are processed
	if !autoscalerv1alpha1.IsManagedNodeGroup(nodeGroup) {
		s.logger.Debugw("skipping unmanaged NodeGroup in scale-down",
			"nodeGroup", nodeGroup.Name,
			"labels", nodeGroup.Labels)
		return nil, nil
	}

	// Get all nodes in this NodeGroup
	nodes, err := s.getNodeGroupNodes(ctx, nodeGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var candidates []*ScaleDownCandidate

	for _, node := range nodes {
		// Skip protected nodes
		if s.isNodeProtected(node) {
			s.logger.Info("skipping protected node",
				"node", node.Name,
				"nodeGroup", nodeGroup.Name)
			// Note: This is during candidate identification, not a block decision
			// Blocking metrics are recorded in CanScaleDown where the final decision is made
			continue
		}

		// Skip nodes not created due to metrics-based scaling
		// Only scale down nodes that were created by the autoscaler due to resource metrics
		creationReason := node.Annotations[autoscalerv1alpha1.CreationReasonAnnotationKey]
		if creationReason != "" && creationReason != autoscalerv1alpha1.CreationReasonMetrics {
			s.logger.Info("skipping node not created due to metrics",
				"node", node.Name,
				"nodeGroup", nodeGroup.Name,
				"creationReason", creationReason)
			continue
		}

		// Get utilization data and create a safe copy
		// CRITICAL: We perform a DEEP COPY of the utilization data to prevent race conditions.
		//
		// Race Condition Prevention:
		// The nodeUtilization map stores pointers to NodeUtilization structs, which contain
		// a Samples slice. If we only performed a shallow copy, the slice header would be
		// copied but the underlying array would be shared. This creates a race condition:
		//
		// Thread A (IdentifyUnderutilizedNodes): Reads len(Samples) = 5
		// Thread B (UpdateNodeUtilization):      Appends new sample, Samples now points to new array
		// Thread A: Calls copy() on old array   -> DATA CORRUPTION or PANIC
		//
		// Solution: We hold the RLock during the ENTIRE copy operation, including:
		// 1. Reading the struct fields
		// 2. Creating the new Samples slice
		// 3. Copying all sample values
		//
		// This ensures the data cannot change while we're copying it.
		s.utilizationLock.RLock()
		utilization, exists := s.nodeUtilization[node.Name]
		if !exists || !utilization.IsUnderutilized {
			s.utilizationLock.RUnlock()
			continue
		}

		// Create a deep copy while holding the lock to prevent races
		// We must complete the entire copy atomically before releasing the lock
		utilizationCopy := &NodeUtilization{
			NodeName:          utilization.NodeName,
			CPUUtilization:    utilization.CPUUtilization,
			MemoryUtilization: utilization.MemoryUtilization,
			IsUnderutilized:   utilization.IsUnderutilized,
			LastUpdated:       utilization.LastUpdated,
			Samples:           make([]UtilizationSample, len(utilization.Samples)),
		}
		// Copy all samples while still holding the lock
		// This creates a new backing array, preventing shared references
		copy(utilizationCopy.Samples, utilization.Samples)
		// Only release lock after copy is complete
		s.utilizationLock.RUnlock()

		// Check if node has been underutilized for observation window
		if !s.hasBeenUnderutilizedForWindow(utilizationCopy) {
			continue
		}

		// Get pods on the node
		pods, err := s.getNodePods(ctx, node.Name)
		if err != nil {
			s.logger.Error("failed to get pods for node",
				"node", node.Name,
				"error", err)
			continue
		}

		candidate := &ScaleDownCandidate{
			Node:        node,
			Utilization: utilizationCopy,
			Pods:        pods,
			Priority:    s.calculatePriority(utilizationCopy, pods),
		}

		candidates = append(candidates, candidate)
	}

	// Sort candidates by priority (lower first)
	sortCandidatesByPriority(candidates)

	return candidates, nil
}

// CanScaleDown determines if a node can be safely scaled down
func (s *ScaleDownManager) CanScaleDown(
	ctx context.Context,
	nodeGroup *autoscalerv1alpha1.NodeGroup,
	node *corev1.Node,
) (bool, string, error) {
	// Check cooldown period
	if !s.isOutsideCooldownPeriod(nodeGroup.Name) {
		// Record scale-down blocked due to cooldown
		metrics.ScaleDownBlockedTotal.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
			"cooldown",
		).Inc()
		return false, "within cooldown period", nil
	}

	// Check minimum nodes constraint
	currentNodes := len(nodeGroup.Status.Nodes)
	if currentNodes <= int(nodeGroup.Spec.MinNodes) {
		// Record scale-down blocked due to minimum nodes constraint
		metrics.ScaleDownBlockedTotal.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
			"min_nodes",
		).Inc()
		return false, "at minimum nodes", nil
	}

	// Get pods on node
	pods, err := s.getNodePods(ctx, node.Name)
	if err != nil {
		return false, "", fmt.Errorf("failed to get pods: %w", err)
	}

	// Run safety checks
	safe, reason, err := s.IsSafeToRemove(ctx, node, pods)
	if err != nil {
		return false, "", fmt.Errorf("safety check failed: %w", err)
	}

	if !safe {
		// Record scale-down blocked by safety checks
		// Determine the specific reason for blocking
		blockReason := "safety_check"
		if strings.Contains(reason, "local storage") {
			blockReason = "local_storage"
		} else if strings.Contains(reason, "capacity") || strings.Contains(reason, "utilization") {
			blockReason = "capacity"
		} else if strings.Contains(reason, "anti-affinity") {
			blockReason = "affinity"
		} else if strings.Contains(reason, "protected") {
			blockReason = "protected_node"
		} else if strings.Contains(reason, "PDB") || strings.Contains(reason, "disruptions") {
			blockReason = "pdb"
		}

		metrics.ScaleDownBlockedTotal.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
			blockReason,
		).Inc()
		return false, reason, nil
	}

	// Check policy constraints
	if !s.policyEngine.AllowScaleDown(ctx, nodeGroup, node) {
		// Record scale-down blocked by policy constraint
		metrics.ScaleDownBlockedTotal.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
			"policy_constraint",
		).Inc()
		return false, "policy constraint", nil
	}

	return true, "safe to remove", nil
}

// ScaleDown performs scale-down operation on selected nodes by draining them.
//
// This function:
// - Validates each candidate node is still safe to remove
// - Cordons the node to prevent new pod scheduling
// - Evicts all pods (respecting PodDisruptionBudgets)
// - Waits for pod termination
// - Leaves the node cordoned and drained for the controller to delete
//
// Important: This function does NOT delete the Kubernetes node or terminate
// the VPSie VM. The NodeGroup controller is responsible for those operations
// after verifying the drain completed successfully.
func (s *ScaleDownManager) ScaleDown(
	ctx context.Context,
	nodeGroup *autoscalerv1alpha1.NodeGroup,
	candidates []*ScaleDownCandidate,
) error {
	// Limit number of nodes to scale down at once
	maxNodes := s.config.MaxNodesPerScaleDown
	if len(candidates) > maxNodes {
		candidates = candidates[:maxNodes]
	}

	// Log with correlation ID for tracing
	requestID := logging.GetRequestID(ctx)
	s.logger.Infow("initiating scale-down",
		"nodeGroup", nodeGroup.Name,
		"candidates", len(candidates),
		"requestID", requestID)

	var errors []error
	successCount := 0

	for _, candidate := range candidates {
		// Double-check safety before draining
		canScale, reason, err := s.CanScaleDown(ctx, nodeGroup, candidate.Node)
		if err != nil {
			errors = append(errors, fmt.Errorf("pre-drain check failed for %s: %w", candidate.Node.Name, err))
			continue
		}

		if !canScale {
			s.logger.Info("skipping node - cannot scale down",
				"node", candidate.Node.Name,
				"reason", reason)
			// Metrics already recorded in CanScaleDown function
			continue
		}

		// Drain the node
		if err := s.DrainNode(ctx, candidate.Node); err != nil {
			errors = append(errors, fmt.Errorf("failed to drain node %s: %w", candidate.Node.Name, err))

			// Record drain failure metric
			metrics.ScaleDownErrorsTotal.WithLabelValues(
				nodeGroup.Name,
				nodeGroup.Namespace,
				"drain_failed",
			).Inc()
			continue
		}

		s.logger.Info("node drained successfully - ready for VPSieNode deletion",
			"node", candidate.Node.Name,
			"nodeGroup", nodeGroup.Name)

		// NOTE: We do NOT delete the Kubernetes node here.
		// The proper flow is:
		// 1. ScaleDownManager drains the node (done above)
		// 2. NodeGroupReconciler deletes the VPSieNode CR
		// 3. VPSieNode controller terminates the VM and deletes the K8s node
		//
		// This ensures proper cleanup of VPSie resources and consistent state.

		successCount++

		// Record metric
		metrics.ScaleDownNodesRemoved.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
		).Observe(1)
	}

	// Update last scale-down time
	if successCount > 0 {
		s.scaleDownLock.Lock()
		s.lastScaleDown[nodeGroup.Name] = time.Now()
		s.scaleDownLock.Unlock()

		// Record total scale-down event
		metrics.ScaleDownTotal.WithLabelValues(
			nodeGroup.Name,
			nodeGroup.Namespace,
		).Inc()
	}

	if len(errors) > 0 {
		return fmt.Errorf("scale-down completed with %d errors (succeeded: %d): %v", len(errors), successCount, errors)
	}

	return nil
}

// Helper functions

func (s *ScaleDownManager) getNodeGroupNodes(
	ctx context.Context,
	nodeGroup *autoscalerv1alpha1.NodeGroup,
) ([]*corev1.Node, error) {
	// List all nodes with NodeGroup label using centralized constants
	labelSelector := fmt.Sprintf("%s=%s", autoscalerv1alpha1.NodeGroupLabelKey, nodeGroup.Name)
	nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes[i] = &nodeList.Items[i]
	}

	return nodes, nil
}

func (s *ScaleDownManager) getNodePods(ctx context.Context, nodeName string) ([]*corev1.Pod, error) {
	podList, err := s.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, err
	}

	pods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		// Skip pods in terminal states
		if podList.Items[i].Status.Phase == corev1.PodSucceeded ||
			podList.Items[i].Status.Phase == corev1.PodFailed {
			continue
		}
		pods = append(pods, &podList.Items[i])
	}

	return pods, nil
}

func (s *ScaleDownManager) isNodeProtected(node *corev1.Node) bool {
	// Check annotation
	if val, exists := node.Annotations[ProtectedNodeAnnotation]; exists && val == "true" {
		return true
	}

	// Check label
	if val, exists := node.Labels[ScaleDownDisabledLabel]; exists && val == "true" {
		return true
	}

	return false
}

func (s *ScaleDownManager) hasBeenUnderutilizedForWindow(utilization *NodeUtilization) bool {
	if len(utilization.Samples) == 0 {
		return false
	}

	// Check if utilization data is stale (no updates in last 5 minutes)
	// Stale data should not be used for scale-down decisions
	const maxStaleness = 5 * time.Minute
	now := time.Now()
	if now.Sub(utilization.LastUpdated) > maxStaleness {
		s.logger.Warn("utilization data is stale, skipping scale-down",
			zap.String("node", utilization.NodeName),
			zap.Duration("staleness", now.Sub(utilization.LastUpdated)),
			zap.Duration("maxAllowed", maxStaleness))
		return false
	}

	// Check if all samples within observation window are underutilized
	windowStart := now.Add(-s.config.ObservationWindow)

	underutilizedCount := 0
	totalSamples := 0

	for _, sample := range utilization.Samples {
		if sample.Timestamp.Before(windowStart) {
			continue
		}

		totalSamples++
		if sample.CPUUtilization < s.config.CPUThreshold &&
			sample.MemoryUtilization < s.config.MemoryThreshold {
			underutilizedCount++
		}
	}

	// Require at least 80% of samples to be underutilized
	if totalSamples == 0 {
		return false
	}

	return float64(underutilizedCount)/float64(totalSamples) >= 0.8
}

func (s *ScaleDownManager) isOutsideCooldownPeriod(nodeGroupName string) bool {
	s.scaleDownLock.RLock()
	lastScaleDown, exists := s.lastScaleDown[nodeGroupName]
	s.scaleDownLock.RUnlock()

	if !exists {
		return true
	}

	return time.Since(lastScaleDown) >= s.config.CooldownPeriod
}

func (s *ScaleDownManager) calculatePriority(utilization *NodeUtilization, pods []*corev1.Pod) int {
	// Lower priority = removed first
	priority := 0

	// Prefer nodes with lower utilization
	avgUtilization := (utilization.CPUUtilization + utilization.MemoryUtilization) / 2
	priority += int(avgUtilization * 10) // 0-1000

	// Prefer nodes with fewer pods
	priority += len(pods) * 100

	// Prefer nodes with fewer system pods
	systemPodCount := 0
	for _, pod := range pods {
		if pod.Namespace == "kube-system" {
			systemPodCount++
		}
	}
	priority += systemPodCount * 500

	return priority
}

func sortCandidatesByPriority(candidates []*ScaleDownCandidate) {
	// Sort by priority (lower priority first) using efficient O(n log n) algorithm
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority < candidates[j].Priority
	})
}

// ValidatePodDisruptionBudgets checks if PDBs allow pod eviction
func (s *ScaleDownManager) ValidatePodDisruptionBudgets(
	ctx context.Context,
	pods []*corev1.Pod,
) error {
	if !s.config.EnablePodDisruptionBudget {
		return nil
	}

	// Group pods by namespace
	podsByNamespace := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		podsByNamespace[pod.Namespace] = append(podsByNamespace[pod.Namespace], pod)
	}

	// Check PDBs for each namespace
	for namespace, nsPods := range podsByNamespace {
		pdbList, err := s.client.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list PDBs: %w", err)
		}

		for _, pdb := range pdbList.Items {
			if err := s.validatePDB(ctx, &pdb, nsPods); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *ScaleDownManager) validatePDB(
	ctx context.Context,
	pdb *policyv1.PodDisruptionBudget,
	pods []*corev1.Pod,
) error {
	// Match pods to PDB selector
	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return fmt.Errorf("invalid PDB selector: %w", err)
	}

	matchingPods := 0
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			matchingPods++
		}
	}

	if matchingPods == 0 {
		return nil
	}

	// Check if eviction would violate PDB
	if pdb.Status.DisruptionsAllowed < 1 {
		// Note: PDB violations are also tracked as SafetyCheckFailuresTotal
		// in the IsSafeToRemove function when this error is returned
		return fmt.Errorf("PDB %s/%s does not allow disruptions", pdb.Namespace, pdb.Name)
	}

	return nil
}
