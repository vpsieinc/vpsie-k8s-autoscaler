package scaler

import (
	"context"
	"fmt"
	"strings"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// IsSafeToRemove performs comprehensive safety checks before node removal
func (s *ScaleDownManager) IsSafeToRemove(
	ctx context.Context,
	node *corev1.Node,
	pods []*corev1.Pod,
) (bool, string, error) {
	s.logger.Debug("running safety checks for node removal", "node", node.Name)

	// Get nodegroup labels for metrics (best effort)
	nodeGroupName := node.Labels["autoscaler.vpsie.com/nodegroup"]
	nodeGroupNamespace := node.Labels["autoscaler.vpsie.com/nodegroup-namespace"]
	if nodeGroupName == "" {
		nodeGroupName = "unknown"
	}
	if nodeGroupNamespace == "" {
		nodeGroupNamespace = "unknown"
	}

	// Sanitize label values to prevent cardinality explosion
	nodeGroupName, _ = metrics.SanitizeLabel(nodeGroupName)
	nodeGroupNamespace, _ = metrics.SanitizeLabel(nodeGroupNamespace)

	// Check 1: Node has no pods with local storage
	if hasLocalStorage, reason := s.hasPodsWithLocalStorage(ctx, pods); hasLocalStorage {
		// Record safety check failure: local storage
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"local_storage",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, reason, nil
	}

	// Check 2: All pods can be scheduled elsewhere
	if canSchedule, reason, err := s.canPodsBeRescheduled(ctx, pods); err != nil {
		return false, "", err
	} else if !canSchedule {
		// Record safety check failure: rescheduling
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"rescheduling",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, reason, nil
	}

	// Check 3: System pods have alternatives
	if hasUniqueSystem, reason := s.hasUniqueSystemPods(pods); hasUniqueSystem {
		// Record safety check failure: unique system pods
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"system_pods",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, reason, nil
	}

	// Check 4: No pod anti-affinity violations
	if hasViolation, reason, err := s.hasAntiAffinityViolations(ctx, pods); err != nil {
		return false, "", err
	} else if hasViolation {
		// Record safety check failure: affinity
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"affinity",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, reason, nil
	}

	// Check 5: Cluster has sufficient capacity after removal
	if insufficient, reason, err := s.hasInsufficientCapacity(ctx, node, pods); err != nil {
		return false, "", err
	} else if insufficient {
		// Record safety check failure: capacity
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"capacity",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, reason, nil
	}

	// Check 6: Node is not annotated as protected
	if s.isNodeProtected(node) {
		// Record safety check failure: protection
		metrics.SafetyCheckFailuresTotal.WithLabelValues(
			"protection",
			nodeGroupName,
			nodeGroupNamespace,
		).Inc()
		return false, "node is protected", nil
	}

	s.logger.Debug("all safety checks passed", "node", node.Name)
	return true, "safe to remove", nil
}

// hasPodsWithLocalStorage checks if any pods use local storage volumes
func (s *ScaleDownManager) hasPodsWithLocalStorage(ctx context.Context, pods []*corev1.Pod) (bool, string) {
	for _, pod := range pods {
		// Skip DaemonSet pods from system namespaces - they will be recreated on other nodes
		// and their EmptyDir data is typically ephemeral (logs, caches, etc.)
		if s.isSkippableDaemonSetPod(pod) {
			s.logger.Debug("skipping DaemonSet pod from local storage check",
				"pod", pod.Name,
				"namespace", pod.Namespace)
			continue
		}

		for _, volume := range pod.Spec.Volumes {
			// Check for EmptyDir volumes
			if volume.EmptyDir != nil {
				// EmptyDir with Memory medium is okay (data is already in memory)
				if volume.EmptyDir.Medium != corev1.StorageMediumMemory {
					return true, fmt.Sprintf("pod %s/%s uses EmptyDir local storage", pod.Namespace, pod.Name)
				}
			}

			// Check for HostPath volumes
			if volume.HostPath != nil {
				return true, fmt.Sprintf("pod %s/%s uses HostPath local storage", pod.Namespace, pod.Name)
			}

			// Check for Local persistent volumes
			if volume.PersistentVolumeClaim != nil {
				hasLocal, err := s.isPVCLocal(ctx, pod.Namespace, volume.PersistentVolumeClaim.ClaimName)
				if err != nil {
					s.logger.Warn("failed to check PVC type",
						"pvc", volume.PersistentVolumeClaim.ClaimName,
						"error", err)
					// Treat PVC validation failures as unsafe for safety
					return true, fmt.Sprintf("pod %s/%s has PVC that couldn't be validated", pod.Namespace, pod.Name)
				}
				if hasLocal {
					return true, fmt.Sprintf("pod %s/%s uses local PersistentVolume", pod.Namespace, pod.Name)
				}
			}
		}
	}

	return false, ""
}

// isSkippableDaemonSetPod checks if a pod is a DaemonSet pod from a system namespace
// that can be safely skipped during local storage checks. DaemonSet pods will be
// automatically recreated on other nodes and typically use EmptyDir for ephemeral data.
func (s *ScaleDownManager) isSkippableDaemonSetPod(pod *corev1.Pod) bool {
	// Check if pod is owned by a DaemonSet
	isDaemonSet := false
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			isDaemonSet = true
			break
		}
	}

	if !isDaemonSet {
		return false
	}

	// System namespaces where DaemonSet pods can be safely evicted
	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
		"cilium":          true,
		"calico-system":   true,
		"flannel":         true,
		"weave-net":       true,
		"tigera-operator": true,
	}

	return systemNamespaces[pod.Namespace]
}

// isPVCLocal checks if a PVC is backed by a local volume
func (s *ScaleDownManager) isPVCLocal(ctx context.Context, namespace, pvcName string) (bool, error) {
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	if pvc.Spec.VolumeName == "" {
		return false, nil // Not bound yet
	}

	pv, err := s.client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	// Check if PV is using local storage
	return pv.Spec.Local != nil, nil
}

// canPodsBeRescheduled checks if pods can be scheduled on other nodes
func (s *ScaleDownManager) canPodsBeRescheduled(ctx context.Context, pods []*corev1.Pod) (bool, string, error) {
	// Get all nodes
	nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to list nodes: %w", err)
	}

	// Filter ready and schedulable nodes
	var availableNodes []*corev1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// Skip unschedulable nodes
		if node.Spec.Unschedulable {
			continue
		}

		// Skip nodes with issues
		if !isNodeReady(node) {
			continue
		}

		availableNodes = append(availableNodes, node)
	}

	if len(availableNodes) == 0 {
		return false, "no available nodes for rescheduling", nil
	}

	// Check resource availability for pod requirements
	totalCPURequests, totalMemRequests := CalculateResourceRequests(pods)

	// Calculate total available capacity on other nodes
	var totalAvailableCPU, totalAvailableMem int64
	for _, node := range availableNodes {
		nodePods, err := s.getNodePods(ctx, node.Name)
		if err != nil {
			continue
		}

		nodeCPURequests, nodeMemRequests := CalculateResourceRequests(nodePods)
		allocatableCPU, allocatableMem := GetNodeAllocatableResources(node)

		totalAvailableCPU += (allocatableCPU - nodeCPURequests)
		totalAvailableMem += (allocatableMem - nodeMemRequests)
	}

	// Add 20% buffer for scheduling overhead
	requiredCPU := int64(float64(totalCPURequests) * 1.2)
	requiredMem := int64(float64(totalMemRequests) * 1.2)

	if totalAvailableCPU < requiredCPU {
		return false, fmt.Sprintf("insufficient CPU capacity for rescheduling (need %dm, available %dm)",
			requiredCPU, totalAvailableCPU), nil
	}

	if totalAvailableMem < requiredMem {
		return false, fmt.Sprintf("insufficient memory capacity for rescheduling (need %d, available %d)",
			requiredMem, totalAvailableMem), nil
	}

	// Check anti-affinity node requirements
	// Count pods that need unique nodes due to hostname-based anti-affinity
	requiredUniqueNodes := countPodsRequiringUniqueNodes(pods)
	if requiredUniqueNodes > 0 && len(availableNodes) < requiredUniqueNodes {
		return false, fmt.Sprintf("insufficient nodes for pod anti-affinity (need %d unique nodes, have %d available)",
			requiredUniqueNodes, len(availableNodes)), nil
	}

	return true, "", nil
}

// hasUniqueSystemPods checks if node has unique system pods
func (s *ScaleDownManager) hasUniqueSystemPods(pods []*corev1.Pod) (bool, string) {
	// Check for critical system pods that should not be evicted
	for _, pod := range pods {
		// Skip non-system pods
		if pod.Namespace != "kube-system" {
			continue
		}

		// Check if this is a single-instance system pod
		if isSingleInstanceSystemPod(pod) {
			return true, fmt.Sprintf("node has unique system pod %s", pod.Name)
		}
	}

	return false, ""
}

// isSingleInstanceSystemPod checks if a pod is a critical single-instance system pod
func isSingleInstanceSystemPod(pod *corev1.Pod) bool {
	// These components should typically have multiple replicas
	// If we find a single instance, it might be unsafe to remove
	criticalComponents := []string{
		"kube-apiserver",
		"etcd",
		"kube-controller-manager",
		"kube-scheduler",
	}

	for _, component := range criticalComponents {
		if strings.Contains(pod.Name, component) {
			return true
		}
	}

	return false
}

// hasAntiAffinityViolations checks if removing node would violate pod anti-affinity rules
func (s *ScaleDownManager) hasAntiAffinityViolations(ctx context.Context, pods []*corev1.Pod) (bool, string, error) {
	for _, pod := range pods {
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			continue
		}

		// Check required anti-affinity rules
		for _, term := range pod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			violation, reason, err := s.checkAntiAffinityTerm(ctx, pod, &term)
			if err != nil {
				return false, "", err
			}
			if violation {
				return true, reason, nil
			}
		}
	}

	return false, "", nil
}

func (s *ScaleDownManager) checkAntiAffinityTerm(
	ctx context.Context,
	pod *corev1.Pod,
	term *corev1.PodAffinityTerm,
) (bool, string, error) {
	// Get pods matching the anti-affinity selector
	selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
	if err != nil {
		return false, "", fmt.Errorf("invalid label selector: %w", err)
	}

	// Check if rescheduling this pod would violate the anti-affinity rule
	// This is a simplified check - full scheduling simulation would be more accurate
	matchingPods, err := s.client.CoreV1().Pods(pod.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to list pods: %w", err)
	}

	// If there are matching pods and topology key is node hostname,
	// we need at least 2 nodes to satisfy anti-affinity
	if len(matchingPods.Items) > 0 && term.TopologyKey == "kubernetes.io/hostname" {
		nodes, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, "", fmt.Errorf("failed to list nodes: %w", err)
		}

		readyNodes := 0
		for i := range nodes.Items {
			if isNodeReady(&nodes.Items[i]) && !nodes.Items[i].Spec.Unschedulable {
				readyNodes++
			}
		}

		// If we only have 2 ready nodes and we're removing one, anti-affinity might be violated
		if readyNodes <= 2 {
			return true, fmt.Sprintf("insufficient nodes to satisfy anti-affinity for pod %s/%s", pod.Namespace, pod.Name), nil
		}
	}

	return false, "", nil
}

// hasInsufficientCapacity checks if cluster would have insufficient capacity after removal
func (s *ScaleDownManager) hasInsufficientCapacity(
	ctx context.Context,
	nodeToRemove *corev1.Node,
	podsToMove []*corev1.Pod,
) (bool, string, error) {
	// Get all nodes
	allNodes, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]*corev1.Node, 0)
	for i := range allNodes.Items {
		nodes = append(nodes, &allNodes.Items[i])
	}

	// Predict utilization after removal
	avgCPU, avgMem, maxCPU, maxMem, err := s.PredictUtilizationAfterRemoval(
		ctx,
		[]*corev1.Node{nodeToRemove},
		nodes,
	)
	if err != nil {
		return false, "", fmt.Errorf("failed to predict utilization: %w", err)
	}

	// Check if predicted utilization is acceptable
	const MaxAcceptableUtilization = 85.0

	if maxCPU > MaxAcceptableUtilization {
		return true, fmt.Sprintf("removal would cause excessive CPU utilization (%.1f%%)", maxCPU), nil
	}

	if maxMem > MaxAcceptableUtilization {
		return true, fmt.Sprintf("removal would cause excessive memory utilization (%.1f%%)", maxMem), nil
	}

	s.logger.Debug("capacity check passed",
		"node", nodeToRemove.Name,
		"predictedAvgCPU", fmt.Sprintf("%.1f%%", avgCPU),
		"predictedAvgMem", fmt.Sprintf("%.1f%%", avgMem))

	return false, "", nil
}

// Helper functions

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// HasPersistentVolumes checks if any pods have persistent volumes
func HasPersistentVolumes(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// GetStorageClasses returns all storage classes in the cluster
func (s *ScaleDownManager) GetStorageClasses(ctx context.Context) (*storagev1.StorageClassList, error) {
	return s.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
}

// IsPodControlledBy checks if a pod is controlled by a specific resource type
func IsPodControlledBy(pod *corev1.Pod, kind string) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == kind {
			return true
		}
	}
	return false
}

// HasNodeSelector checks if pod has node selector requirements
func HasNodeSelector(pod *corev1.Pod) bool {
	return len(pod.Spec.NodeSelector) > 0
}

// HasNodeAffinity checks if pod has node affinity requirements
func HasNodeAffinity(pod *corev1.Pod) bool {
	return pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil
}

// GetPodPriority returns the priority of a pod
func GetPodPriority(pod *corev1.Pod) int32 {
	if pod.Spec.Priority != nil {
		return *pod.Spec.Priority
	}
	return 0
}

// IsSystemCriticalPod checks if a pod is marked as system-critical
func IsSystemCriticalPod(pod *corev1.Pod) bool {
	if pod.Spec.PriorityClassName == "system-cluster-critical" ||
		pod.Spec.PriorityClassName == "system-node-critical" {
		return true
	}

	priority := GetPodPriority(pod)
	return priority >= 2000000000 // System critical priority threshold
}

// MatchesNodeSelector checks if a node matches pod's node selector
func MatchesNodeSelector(node *corev1.Node, pod *corev1.Pod) bool {
	if len(pod.Spec.NodeSelector) == 0 {
		return true
	}

	nodeLabels := labels.Set(node.Labels)
	selector := labels.SelectorFromSet(pod.Spec.NodeSelector)
	return selector.Matches(nodeLabels)
}

// tolerationMatches checks if a toleration matches a taint.
// Per Kubernetes documentation:
// - Empty key with Exists operator matches all taints (wildcard)
// - Key must match
// - Effect must match (empty toleration effect matches all effects)
// - Operator: Exists matches any value, Equal requires value match
func tolerationMatches(toleration *corev1.Toleration, taint *corev1.Taint) bool {
	// Empty key with Exists operator matches all taints
	if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
		return true
	}

	// Key must match
	if toleration.Key != taint.Key {
		return false
	}

	// Effect must match (empty toleration effect matches all effects)
	if toleration.Effect != "" && toleration.Effect != taint.Effect {
		return false
	}

	// Operator-based value matching
	switch toleration.Operator {
	case corev1.TolerationOpExists:
		// Exists operator matches any value
		return true
	case corev1.TolerationOpEqual, "":
		// Equal operator (or default) requires value match
		return toleration.Value == taint.Value
	}

	return false
}

// tolerationMatchesTaint checks if any toleration in the list matches the taint.
func tolerationMatchesTaint(tolerations []corev1.Toleration, taint *corev1.Taint) bool {
	for i := range tolerations {
		if tolerationMatches(&tolerations[i], taint) {
			return true
		}
	}
	return false
}

// tolerationsTolerateTaints checks if tolerations cover all taints with NoSchedule/NoExecute effect.
// Only hard constraints (NoSchedule, NoExecute) are checked.
// PreferNoSchedule is a soft constraint and is ignored.
// Returns true if all hard-constraint taints are tolerated, false otherwise.
func tolerationsTolerateTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
	for _, taint := range taints {
		// Only check hard constraints (NoSchedule, NoExecute)
		// PreferNoSchedule is soft - ignored for hard scheduling decisions
		if taint.Effect != corev1.TaintEffectNoSchedule &&
			taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}

		// Check if any toleration matches this taint
		if !tolerationMatchesTaint(tolerations, &taint) {
			return false
		}
	}
	return true
}

// matchNodeSelectorRequirement checks if a node satisfies a single node selector requirement.
// Supports operators: In, NotIn, Exists, DoesNotExist
// Note: Gt and Lt operators are not implemented as they are alpha features.
func matchNodeSelectorRequirement(node *corev1.Node, req *corev1.NodeSelectorRequirement) bool {
	// Handle nil labels
	if node.Labels == nil {
		// For operators that check label absence, nil labels means absent
		switch req.Operator {
		case corev1.NodeSelectorOpDoesNotExist:
			return true
		case corev1.NodeSelectorOpNotIn:
			return true
		default:
			return false
		}
	}

	value, exists := node.Labels[req.Key]

	switch req.Operator {
	case corev1.NodeSelectorOpIn:
		// Node label value must be in the requirement values list
		if !exists {
			return false
		}
		for _, v := range req.Values {
			if value == v {
				return true
			}
		}
		return false

	case corev1.NodeSelectorOpNotIn:
		// Node label value must NOT be in the requirement values list
		// If label doesn't exist, it's considered "not in" the values
		if !exists {
			return true
		}
		for _, v := range req.Values {
			if value == v {
				return false
			}
		}
		return true

	case corev1.NodeSelectorOpExists:
		// Node must have the label key (value doesn't matter)
		return exists

	case corev1.NodeSelectorOpDoesNotExist:
		// Node must NOT have the label key
		return !exists

	default:
		// Gt and Lt are not supported (alpha feature)
		return false
	}
}

// matchesNodeSelectorTerms checks if a node matches any of the node selector terms.
// Terms are ORed - matching any term is sufficient.
// Within a term, MatchExpressions are ANDed - all must match.
func matchesNodeSelectorTerms(node *corev1.Node, terms []corev1.NodeSelectorTerm) bool {
	// Empty terms matches any node
	if len(terms) == 0 {
		return true
	}

	// Terms are ORed - matching any term is sufficient
	for _, term := range terms {
		if matchesNodeSelectorTerm(node, &term) {
			return true
		}
	}
	return false
}

// matchesNodeSelectorTerm checks if a node matches a single node selector term.
// MatchExpressions within a term are ANDed - all must match.
func matchesNodeSelectorTerm(node *corev1.Node, term *corev1.NodeSelectorTerm) bool {
	// Empty MatchExpressions matches any node
	if len(term.MatchExpressions) == 0 {
		return true
	}

	// MatchExpressions are ANDed - all must match
	for i := range term.MatchExpressions {
		if !matchNodeSelectorRequirement(node, &term.MatchExpressions[i]) {
			return false
		}
	}
	return true
}

// matchesNodeAffinity checks if a pod's node affinity requirements are satisfied by a node.
// Only checks RequiredDuringSchedulingIgnoredDuringExecution (hard constraint).
// Preferred constraints are ignored for scale-down decisions - they express preferences, not requirements.
func matchesNodeAffinity(pod *corev1.Pod, node *corev1.Node) bool {
	// No affinity requirements means matches any node
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
		return true
	}

	nodeAffinity := pod.Spec.Affinity.NodeAffinity

	// Check required (hard) constraints
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		if !matchesNodeSelectorTerms(node, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) {
			return false
		}
	}

	// Preferred (soft) constraints are NOT checked for scale-down decisions
	// They express preferences, not requirements
	return true
}

// matchesPodAffinityTerm checks if an existing pod matches a pod affinity term
// considering the topology key and label selector.
// For hostname-based topology (kubernetes.io/hostname), we use a simplified check:
// if the existing pod's NodeName matches the target node's Name, they are on the same topology.
// For other topology keys (e.g., zone), we would need to look up the existing pod's node,
// which requires additional API calls. For safety, we return false for non-hostname topologies.
func matchesPodAffinityTerm(existingPod *corev1.Pod, term *corev1.PodAffinityTerm, node *corev1.Node) bool {
	// Handle nil LabelSelector - cannot match without selector
	if term.LabelSelector == nil {
		return false
	}

	// Convert LabelSelector to labels.Selector
	selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
	if err != nil {
		// Invalid selector - cannot match
		return false
	}

	// Check if existing pod's labels match the selector
	if !selector.Matches(labels.Set(existingPod.Labels)) {
		return false
	}

	// Check topology key matching
	// For kubernetes.io/hostname, we can directly compare pod's NodeName with node's Name
	if term.TopologyKey == "kubernetes.io/hostname" {
		// The existing pod is on the same topology if its NodeName matches the target node's
		// hostname label (which typically equals the node name)
		nodeHostname := node.Labels[term.TopologyKey]
		if nodeHostname == "" {
			// Node doesn't have hostname label - use node name as fallback
			nodeHostname = node.Name
		}
		return existingPod.Spec.NodeName == nodeHostname
	}

	// For other topology keys (e.g., topology.kubernetes.io/zone), we would need to:
	// 1. Get the existing pod's node
	// 2. Get the topology value from that node's labels
	// 3. Compare with the target node's topology value
	// Without the ability to look up the existing pod's node, we cannot verify
	// the topology match. For safety, return false (no match assumed).
	return false
}

// countPodsRequiringUniqueNodes counts pods that have hostname-based anti-affinity
// (RequiredDuringSchedulingIgnoredDuringExecution with kubernetes.io/hostname topology key).
// Each such pod requires its own unique node to satisfy the anti-affinity constraint.
// This function is used to determine the minimum number of nodes needed for rescheduling.
func countPodsRequiringUniqueNodes(pods []*corev1.Pod) int {
	count := 0
	for _, pod := range pods {
		if hasHostnameAntiAffinity(pod) {
			count++
		}
	}
	return count
}

// hasHostnameAntiAffinity checks if a pod has RequiredDuringSchedulingIgnoredDuringExecution
// anti-affinity with kubernetes.io/hostname topology key.
func hasHostnameAntiAffinity(pod *corev1.Pod) bool {
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
		return false
	}

	for _, term := range pod.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
		if term.TopologyKey == "kubernetes.io/hostname" {
			return true
		}
	}
	return false
}

// hasPodAntiAffinityViolation checks if scheduling pod to node would violate anti-affinity rules.
// Only checks RequiredDuringSchedulingIgnoredDuringExecution (hard constraint).
// Preferred (soft) constraints are ignored for scale-down decisions - they express preferences, not requirements.
func hasPodAntiAffinityViolation(pod *corev1.Pod, node *corev1.Node, existingPods []*corev1.Pod) bool {
	// No anti-affinity requirements means no violation possible
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
		return false
	}

	antiAffinity := pod.Spec.Affinity.PodAntiAffinity

	// Only check required (hard) anti-affinity constraints
	// Preferred (soft) constraints are ignored for scale-down decisions
	for _, term := range antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
		for _, existingPod := range existingPods {
			if matchesPodAffinityTerm(existingPod, &term, node) {
				return true // Would violate anti-affinity
			}
		}
	}

	return false
}
