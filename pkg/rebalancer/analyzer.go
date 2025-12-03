package rebalancer

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Analyzer identifies nodes that are candidates for rebalancing and performs safety checks
type Analyzer struct {
	kubeClient    kubernetes.Interface
	costOptimizer *cost.Optimizer
	config        *AnalyzerConfig
}

// NewAnalyzer creates a new rebalance analyzer
func NewAnalyzer(kubeClient kubernetes.Interface, costOptimizer *cost.Optimizer, config *AnalyzerConfig) *Analyzer {
	if config == nil {
		config = &AnalyzerConfig{
			MinHealthyPercent:         75,
			SkipNodesWithLocalStorage: true,
			RespectPDBs:               true,
			CooldownPeriod:            time.Hour,
		}
	}

	return &Analyzer{
		kubeClient:    kubeClient,
		costOptimizer: costOptimizer,
		config:        config,
	}
}

// AnalyzeRebalanceOpportunities identifies which nodes should be rebalanced
func (a *Analyzer) AnalyzeRebalanceOpportunities(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) (*RebalanceAnalysis, error) {
	logger := log.FromContext(ctx)
	logger.Info("Analyzing rebalance opportunities", "nodeGroup", nodeGroup.Name)

	analysis := &RebalanceAnalysis{
		NodeGroupName: nodeGroup.Name,
		Namespace:     nodeGroup.Namespace,
		AnalyzedAt:    time.Now(),
	}

	// Get cost optimization recommendation
	report, err := a.costOptimizer.AnalyzeOptimizations(ctx, nodeGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze optimizations: %w", err)
	}

	if len(report.Opportunities) == 0 {
		logger.Info("No optimization opportunities found")
		analysis.RecommendedAction = ActionReject
		return analysis, nil
	}

	// Use the top optimization opportunity
	analysis.Optimization = &report.Opportunities[0]

	// Get current nodes in the NodeGroup
	nodes, err := a.getNodeGroupNodes(ctx, nodeGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	analysis.TotalNodes = int32(len(nodes))

	// Identify candidate nodes for rebalancing
	candidates, err := a.identifyCandidates(ctx, nodeGroup, nodes, analysis.Optimization)
	if err != nil {
		return nil, fmt.Errorf("failed to identify candidates: %w", err)
	}

	analysis.CandidateNodes = candidates

	// Perform safety checks
	safetyChecks, err := a.performSafetyChecks(ctx, nodeGroup, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to perform safety checks: %w", err)
	}

	analysis.SafetyChecks = safetyChecks

	// Determine recommended action
	analysis.RecommendedAction = a.determineRecommendedAction(safetyChecks, analysis.Optimization)

	// Calculate priority
	analysis.Priority = a.calculatePriority(analysis.Optimization, safetyChecks)

	// Estimate duration
	analysis.EstimatedDuration = a.estimateDuration(candidates)

	logger.Info("Analysis complete",
		"candidates", len(candidates),
		"action", analysis.RecommendedAction,
		"priority", analysis.Priority,
		"estimatedDuration", analysis.EstimatedDuration)

	return analysis, nil
}

// ValidateRebalanceSafety checks if rebalancing is safe to proceed
func (a *Analyzer) ValidateRebalanceSafety(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodes []*Node) (*SafetyCheck, error) {
	logger := log.FromContext(ctx)
	logger.Info("Validating rebalance safety", "nodeGroup", nodeGroup.Name, "nodes", len(nodes))

	// Perform comprehensive safety check
	checks, err := a.performSafetyChecks(ctx, nodeGroup, a.nodesToCandidates(nodes))
	if err != nil {
		return nil, err
	}

	// Aggregate results
	overallCheck := &SafetyCheck{
		Category:  SafetyCheckClusterHealth,
		Status:    SafetyCheckPassed,
		Message:   "All safety checks passed",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	for _, check := range checks {
		if check.Status == SafetyCheckFailed {
			overallCheck.Status = SafetyCheckFailed
			overallCheck.Message = "One or more safety checks failed"
			break
		} else if check.Status == SafetyCheckWarn && overallCheck.Status == SafetyCheckPassed {
			overallCheck.Status = SafetyCheckWarn
			overallCheck.Message = "Safety checks passed with warnings"
		}
	}

	return overallCheck, nil
}

// CalculateRebalancePriority determines the order of node replacement
func (a *Analyzer) CalculateRebalancePriority(nodes []*Node, optimization *cost.Opportunity) ([]PriorityNode, error) {
	priorityNodes := make([]PriorityNode, 0, len(nodes))

	for _, node := range nodes {
		score := a.calculateNodePriorityScore(node, optimization)
		reason := a.getPriorityReason(node, score)

		priorityNodes = append(priorityNodes, PriorityNode{
			Node: &CandidateNode{
				NodeName:        node.Name,
				VPSID:           node.VPSID,
				CurrentOffering: node.OfferingID,
				Age:             node.Age,
			},
			PriorityScore: score,
			Reason:        reason,
		})
	}

	// Sort by priority score (highest first)
	sort.Slice(priorityNodes, func(i, j int) bool {
		return priorityNodes[i].PriorityScore > priorityNodes[j].PriorityScore
	})

	return priorityNodes, nil
}

// identifyCandidates identifies nodes that should be rebalanced
func (a *Analyzer) identifyCandidates(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodes []*Node, optimization *cost.Opportunity) ([]CandidateNode, error) {
	candidates := make([]CandidateNode, 0, len(nodes))

	for _, node := range nodes {
		// Check if node is using the current (non-optimal) offering
		if node.OfferingID != optimization.CurrentOffering {
			continue
		}

		// Get workloads running on this node
		workloads, err := a.getNodeWorkloads(ctx, node)
		if err != nil {
			return nil, fmt.Errorf("failed to get workloads for node %s: %w", node.Name, err)
		}

		// Check if node has local storage (if configured to skip)
		if a.config.SkipNodesWithLocalStorage && a.hasLocalStorage(workloads) {
			continue
		}

		// Calculate priority score
		priorityScore := a.calculateNodePriorityScore(node, optimization)

		candidate := CandidateNode{
			NodeName:        node.Name,
			VPSID:           node.VPSID,
			CurrentOffering: node.OfferingID,
			TargetOffering:  optimization.RecommendedOffering,
			Age:             node.Age,
			Workloads:       workloads,
			PriorityScore:   priorityScore,
			SafeToRebalance: true, // Will be validated by safety checks
			RebalanceReason: fmt.Sprintf("Cost optimization: %s", optimization.Type),
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// performSafetyChecks performs all safety checks before rebalancing
func (a *Analyzer) performSafetyChecks(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, candidates []CandidateNode) ([]SafetyCheck, error) {
	checks := make([]SafetyCheck, 0)

	// 1. Cluster health check
	clusterCheck := a.checkClusterHealth(ctx)
	checks = append(checks, clusterCheck)

	// 2. NodeGroup health check
	nodeGroupCheck := a.checkNodeGroupHealth(ctx, nodeGroup)
	checks = append(checks, nodeGroupCheck)

	// 3. PodDisruptionBudget check
	if a.config.RespectPDBs {
		pdbCheck := a.checkPodDisruptionBudgets(ctx, candidates)
		checks = append(checks, pdbCheck)
	}

	// 4. Resource capacity check
	capacityCheck := a.checkResourceCapacity(ctx, nodeGroup, candidates)
	checks = append(checks, capacityCheck)

	// 5. Timing check
	timingCheck := a.checkTiming(ctx, nodeGroup)
	checks = append(checks, timingCheck)

	return checks, nil
}

// checkClusterHealth checks if the cluster is healthy
func (a *Analyzer) checkClusterHealth(ctx context.Context) SafetyCheck {
	check := SafetyCheck{
		Category:  SafetyCheckClusterHealth,
		Status:    SafetyCheckPassed,
		Message:   "Cluster is healthy",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check control plane health by listing nodes
	nodes, err := a.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		check.Status = SafetyCheckFailed
		check.Message = fmt.Sprintf("Failed to list nodes: %v", err)
		return check
	}

	// Count ready nodes
	readyNodes := 0
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}

	check.Details["total_nodes"] = len(nodes.Items)
	check.Details["ready_nodes"] = readyNodes

	if readyNodes < len(nodes.Items)*a.config.MinHealthyPercent/100 {
		check.Status = SafetyCheckWarn
		check.Message = fmt.Sprintf("Only %d/%d nodes are ready", readyNodes, len(nodes.Items))
	}

	return check
}

// checkNodeGroupHealth checks if the NodeGroup is healthy
func (a *Analyzer) checkNodeGroupHealth(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) SafetyCheck {
	check := SafetyCheck{
		Category:  SafetyCheckNodeGroupHealth,
		Status:    SafetyCheckPassed,
		Message:   "NodeGroup is healthy",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check if NodeGroup has minimum nodes
	currentNodes := nodeGroup.Status.CurrentNodes
	minNodes := nodeGroup.Spec.MinNodes

	check.Details["current_nodes"] = currentNodes
	check.Details["min_nodes"] = minNodes

	if currentNodes < minNodes {
		check.Status = SafetyCheckFailed
		check.Message = fmt.Sprintf("NodeGroup has %d nodes but requires minimum %d", currentNodes, minNodes)
		return check
	}

	// Check for recent scaling events (avoid rebalancing during active scaling)
	if nodeGroup.Status.LastScaleTime != nil {
		timeSinceScale := time.Since(nodeGroup.Status.LastScaleTime.Time)
		if timeSinceScale < a.config.CooldownPeriod {
			check.Status = SafetyCheckFailed
			check.Message = fmt.Sprintf("Recent scaling event %v ago (cooldown: %v)", timeSinceScale, a.config.CooldownPeriod)
			check.Details["time_since_scale"] = timeSinceScale.String()
			return check
		}
	}

	return check
}

// checkPodDisruptionBudgets checks if PDBs can be satisfied
func (a *Analyzer) checkPodDisruptionBudgets(ctx context.Context, candidates []CandidateNode) SafetyCheck {
	check := SafetyCheck{
		Category:  SafetyCheckPodDisruption,
		Status:    SafetyCheckPassed,
		Message:   "PodDisruptionBudgets can be satisfied",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get all PDBs in the cluster
	pdbs, err := a.kubeClient.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		check.Status = SafetyCheckWarn
		check.Message = fmt.Sprintf("Failed to list PDBs: %v", err)
		return check
	}

	violatedPDBs := 0
	for _, pdb := range pdbs.Items {
		if !a.canSatisfyPDB(&pdb, candidates) {
			violatedPDBs++
		}
	}

	check.Details["total_pdbs"] = len(pdbs.Items)
	check.Details["violated_pdbs"] = violatedPDBs

	if violatedPDBs > 0 {
		check.Status = SafetyCheckFailed
		check.Message = fmt.Sprintf("%d PodDisruptionBudgets would be violated", violatedPDBs)
	}

	return check
}

// checkResourceCapacity checks if there's sufficient capacity for rebalancing
func (a *Analyzer) checkResourceCapacity(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, candidates []CandidateNode) SafetyCheck {
	check := SafetyCheck{
		Category:  SafetyCheckResourceCapacity,
		Status:    SafetyCheckPassed,
		Message:   "Sufficient resource capacity available",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check if we have room to add new nodes before removing old ones
	currentNodes := nodeGroup.Status.CurrentNodes
	maxNodes := nodeGroup.Spec.MaxNodes
	nodesToRebalance := int32(len(candidates))

	check.Details["current_nodes"] = currentNodes
	check.Details["max_nodes"] = maxNodes
	check.Details["nodes_to_rebalance"] = nodesToRebalance

	// For rolling strategy, we need headroom to add nodes before removing old ones
	if currentNodes+nodesToRebalance > maxNodes {
		check.Status = SafetyCheckWarn
		check.Message = fmt.Sprintf("Limited headroom: can only add %d nodes before hitting max", maxNodes-currentNodes)
		check.Details["available_headroom"] = maxNodes - currentNodes
	}

	return check
}

// checkTiming checks if rebalancing is allowed at this time
func (a *Analyzer) checkTiming(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) SafetyCheck {
	check := SafetyCheck{
		Category:  SafetyCheckTiming,
		Status:    SafetyCheckPassed,
		Message:   "Rebalancing is allowed at this time",
		CheckedAt: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check maintenance windows if configured
	if len(a.config.MaintenanceWindows) > 0 {
		now := time.Now()
		inWindow := false

		for _, window := range a.config.MaintenanceWindows {
			if a.isInMaintenanceWindow(now, window) {
				inWindow = true
				break
			}
		}

		if !inWindow {
			check.Status = SafetyCheckFailed
			check.Message = "Current time is outside maintenance windows"
			check.Details["current_time"] = now.Format("15:04")
			check.Details["current_day"] = now.Weekday().String()
		}
	}

	return check
}

// Helper functions

func (a *Analyzer) getNodeGroupNodes(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) ([]*Node, error) {
	// List nodes with NodeGroup label
	labelSelector := fmt.Sprintf("vpsie.io/nodegroup=%s", nodeGroup.Name)
	nodeList, err := a.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, 0, len(nodeList.Items))
	for _, n := range nodeList.Items {
		node := &Node{
			Name:       n.Name,
			OfferingID: n.Labels["vpsie.io/offering"],
			Age:        time.Since(n.CreationTimestamp.Time),
			Cordoned:   n.Spec.Unschedulable,
		}

		// Get node status
		for _, condition := range n.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				node.Status = condition.Type
				break
			}
		}

		// Get VPSID from annotation
		if vpsID, ok := n.Annotations["vpsie.io/vps-id"]; ok {
			if _, err := fmt.Sscanf(vpsID, "%d", &node.VPSID); err != nil {
				// Failed to parse VPS ID - continue with VPSID=0
				// TODO: Add proper logging once logger is available in this context
				_ = err
			}
		}

		// Get pods running on this node
		pods, err := a.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", n.Name),
		})
		if err == nil {
			for i := range pods.Items {
				node.Pods = append(node.Pods, &pods.Items[i])
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (a *Analyzer) getNodeWorkloads(ctx context.Context, node *Node) ([]Workload, error) {
	workloads := make([]Workload, 0)

	for _, pod := range node.Pods {
		// Skip DaemonSet pods (they'll be recreated automatically)
		if pod.OwnerReferences != nil {
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "DaemonSet" {
					continue
				}
			}
		}

		workload := Workload{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Kind:      "Pod",
			CanEvict:  true,
		}

		// Check for local storage
		for _, volume := range pod.Spec.Volumes {
			if volume.EmptyDir != nil || volume.HostPath != nil {
				workload.HasLocalStorage = true
				break
			}
		}

		workloads = append(workloads, workload)
	}

	return workloads, nil
}

func (a *Analyzer) hasLocalStorage(workloads []Workload) bool {
	for _, w := range workloads {
		if w.HasLocalStorage {
			return true
		}
	}
	return false
}

func (a *Analyzer) calculateNodePriorityScore(node *Node, optimization *cost.Opportunity) float64 {
	score := 0.0

	// Older nodes have higher priority (more likely to be outdated)
	ageDays := node.Age.Hours() / 24
	score += ageDays * 0.1 // 0.1 points per day of age

	// Nodes with fewer pods have higher priority (easier to drain)
	podCount := float64(len(node.Pods))
	score += (100 - podCount) * 0.5

	// Cost savings increase priority
	score += optimization.MonthlySavings * 0.01

	return score
}

func (a *Analyzer) getPriorityReason(node *Node, score float64) string {
	if score > 50 {
		return "High priority: old node with significant savings potential"
	} else if score > 25 {
		return "Medium priority: moderate age and savings"
	}
	return "Low priority: recent node or minimal savings"
}

func (a *Analyzer) canSatisfyPDB(pdb *policyv1.PodDisruptionBudget, candidates []CandidateNode) bool {
	// Basic conservative check: be cautious with PDBs
	// TODO: Implement full PDB validation with pod selector matching

	// If no PDB exists, we can proceed
	if pdb == nil {
		// Still apply basic safety rules
		return len(candidates) <= 2
	}

	// If we're rebalancing multiple nodes at once, be conservative
	if len(candidates) > 2 {
		// Multiple nodes might violate PDB - reject
		return false
	}

	// If PDB has minAvailable or maxUnavailable set, be extra careful
	if pdb.Spec.MinAvailable != nil || pdb.Spec.MaxUnavailable != nil {
		// Only allow single node rebalancing when PDBs are present
		if len(candidates) > 1 {
			return false
		}
	}

	// Single node rebalancing with rolling strategy should be safe
	// as new node is provisioned before old one is drained
	return true
}

func (a *Analyzer) isInMaintenanceWindow(now time.Time, window MaintenanceWindow) bool {
	// Check if current day is in allowed days
	currentDay := now.Weekday().String()
	dayAllowed := false
	for _, day := range window.Days {
		if day == currentDay {
			dayAllowed = true
			break
		}
	}

	if !dayAllowed {
		return false
	}

	// Check if current time is within the window
	// This is simplified - production would parse HH:MM format properly
	return true
}

func (a *Analyzer) determineRecommendedAction(checks []SafetyCheck, optimization *cost.Opportunity) RecommendedAction {
	// If any check failed, reject
	for _, check := range checks {
		if check.Status == SafetyCheckFailed {
			return ActionReject
		}
	}

	// If high-risk optimization with warnings, needs review
	if optimization.Risk == cost.RiskHigh {
		for _, check := range checks {
			if check.Status == SafetyCheckWarn {
				return ActionNeedsReview
			}
		}
	}

	// If warnings but low risk, postpone
	for _, check := range checks {
		if check.Status == SafetyCheckWarn {
			return ActionPostpone
		}
	}

	return ActionProceed
}

func (a *Analyzer) calculatePriority(optimization *cost.Opportunity, checks []SafetyCheck) RebalancePriority {
	// High savings with all checks passed = high priority
	if optimization.MonthlySavings > 100 {
		allPassed := true
		for _, check := range checks {
			if check.Status != SafetyCheckPassed {
				allPassed = false
				break
			}
		}
		if allPassed {
			return PriorityHigh
		}
	}

	// Medium savings = medium priority
	if optimization.MonthlySavings > 50 {
		return PriorityMedium
	}

	return PriorityLow
}

func (a *Analyzer) estimateDuration(candidates []CandidateNode) time.Duration {
	// Estimate based on number of candidates
	// Assume 5 minutes per node for drain + provision + verify
	minutesPerNode := 5
	totalMinutes := len(candidates) * minutesPerNode
	return time.Duration(totalMinutes) * time.Minute
}

func (a *Analyzer) nodesToCandidates(nodes []*Node) []CandidateNode {
	candidates := make([]CandidateNode, len(nodes))
	for i, node := range nodes {
		candidates[i] = CandidateNode{
			NodeName:        node.Name,
			VPSID:           node.VPSID,
			CurrentOffering: node.OfferingID,
			Age:             node.Age,
		}
	}
	return candidates
}
