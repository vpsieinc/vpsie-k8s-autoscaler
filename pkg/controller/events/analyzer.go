package events

import (
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// ResourceDeficit represents the total resource deficit from pending pods
type ResourceDeficit struct {
	CPU    resource.Quantity
	Memory resource.Quantity
	Pods   int
}

// PodResourceRequest represents the resources requested by a pod
type PodResourceRequest struct {
	Pod    *corev1.Pod
	CPU    resource.Quantity
	Memory resource.Quantity
}

// NodeGroupMatch represents a NodeGroup that can satisfy pending pods
type NodeGroupMatch struct {
	NodeGroup    *v1alpha1.NodeGroup
	MatchingPods []*corev1.Pod
	Deficit      ResourceDeficit
	Score        int // Higher score = better match
}

// ResourceAnalyzer analyzes resource deficits and matches them to NodeGroups
type ResourceAnalyzer struct {
	logger *zap.Logger
}

// NewResourceAnalyzer creates a new resource analyzer
func NewResourceAnalyzer(logger *zap.Logger) *ResourceAnalyzer {
	return &ResourceAnalyzer{
		logger: logger.Named("resource-analyzer"),
	}
}

// CalculateDeficit calculates the total resource deficit from scheduling events
func (a *ResourceAnalyzer) CalculateDeficit(events []SchedulingEvent) ResourceDeficit {
	deficit := ResourceDeficit{
		CPU:    resource.Quantity{},
		Memory: resource.Quantity{},
		Pods:   0,
	}

	// Track unique pods to avoid double-counting
	seenPods := make(map[string]bool)

	for _, event := range events {
		podKey := fmt.Sprintf("%s/%s", event.Pod.Namespace, event.Pod.Name)
		if seenPods[podKey] {
			continue
		}
		seenPods[podKey] = true

		// Calculate pod resource requests
		podResources := a.CalculatePodResources(event.Pod)

		// Add to deficit
		deficit.CPU.Add(podResources.CPU)
		deficit.Memory.Add(podResources.Memory)
		deficit.Pods++
	}

	a.logger.Debug("Calculated resource deficit",
		zap.String("cpu", deficit.CPU.String()),
		zap.String("memory", deficit.Memory.String()),
		zap.Int("pods", deficit.Pods),
	)

	return deficit
}

// CalculatePodResources calculates the total resource requests for a pod
func (a *ResourceAnalyzer) CalculatePodResources(pod *corev1.Pod) PodResourceRequest {
	totalCPU := resource.Quantity{}
	totalMemory := resource.Quantity{}

	for _, container := range pod.Spec.Containers {
		if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			totalCPU.Add(cpu)
		}
		if memory, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			totalMemory.Add(memory)
		}
	}

	// Include init containers (they run sequentially, so use max, not sum)
	for _, container := range pod.Spec.InitContainers {
		if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			if cpu.Cmp(totalCPU) > 0 {
				totalCPU = cpu
			}
		}
		if memory, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			if memory.Cmp(totalMemory) > 0 {
				totalMemory = memory
			}
		}
	}

	return PodResourceRequest{
		Pod:    pod,
		CPU:    totalCPU,
		Memory: totalMemory,
	}
}

// FindMatchingNodeGroups finds NodeGroups that can satisfy the pending pods
func (a *ResourceAnalyzer) FindMatchingNodeGroups(
	pendingPods []corev1.Pod,
	nodeGroups []v1alpha1.NodeGroup,
) []NodeGroupMatch {
	matches := make([]NodeGroupMatch, 0)

	for _, ng := range nodeGroups {
		match := a.matchNodeGroup(&ng, pendingPods)
		if match != nil && len(match.MatchingPods) > 0 {
			matches = append(matches, *match)
		}
	}

	// Sort by score (higher is better)
	sortNodeGroupMatches(matches)

	a.logger.Debug("Found NodeGroup matches",
		zap.Int("matchCount", len(matches)),
	)

	return matches
}

// matchNodeGroup checks if a NodeGroup can satisfy any pending pods
func (a *ResourceAnalyzer) matchNodeGroup(
	ng *v1alpha1.NodeGroup,
	pendingPods []corev1.Pod,
) *NodeGroupMatch {
	matchingPods := make([]*corev1.Pod, 0)

	for i := range pendingPods {
		pod := &pendingPods[i]
		if a.podMatchesNodeGroup(pod, ng) {
			matchingPods = append(matchingPods, pod)
		}
	}

	if len(matchingPods) == 0 {
		return nil
	}

	// Calculate deficit for matching pods
	deficit := ResourceDeficit{
		CPU:    resource.Quantity{},
		Memory: resource.Quantity{},
		Pods:   len(matchingPods),
	}

	for _, pod := range matchingPods {
		podRes := a.CalculatePodResources(pod)
		deficit.CPU.Add(podRes.CPU)
		deficit.Memory.Add(podRes.Memory)
	}

	// Calculate match score
	score := a.calculateMatchScore(ng, matchingPods, deficit)

	return &NodeGroupMatch{
		NodeGroup:    ng,
		MatchingPods: matchingPods,
		Deficit:      deficit,
		Score:        score,
	}
}

// podMatchesNodeGroup checks if a pod can be scheduled on a NodeGroup
func (a *ResourceAnalyzer) podMatchesNodeGroup(
	pod *corev1.Pod,
	ng *v1alpha1.NodeGroup,
) bool {
	// Check node selector
	if len(pod.Spec.NodeSelector) > 0 {
		// Pod has node selector requirements
		if len(ng.Spec.Labels) == 0 {
			// NodeGroup has no labels - it's a generic NodeGroup that accepts any pod
			// This is acceptable as a fallback
			return true
		}
		// Check if all pod's node selector requirements are met by NodeGroup labels
		for key, value := range pod.Spec.NodeSelector {
			if ngValue, exists := ng.Spec.Labels[key]; !exists || ngValue != value {
				return false
			}
		}
	} else {
		// Pod has no node selector - only match generic NodeGroups (no labels)
		if len(ng.Spec.Labels) > 0 {
			return false
		}
	}

	// Check taints/tolerations
	if len(ng.Spec.Taints) > 0 {
		if !a.podToleratesNodeGroupTaints(pod, ng.Spec.Taints) {
			return false
		}
	}

	return true
}

// podToleratesNodeGroupTaints checks if a pod tolerates all NodeGroup taints
func (a *ResourceAnalyzer) podToleratesNodeGroupTaints(
	pod *corev1.Pod,
	taints []corev1.Taint,
) bool {
	for _, taint := range taints {
		if !a.podToleratesTolerates(pod.Spec.Tolerations, &taint) {
			return false
		}
	}
	return true
}

// podToleratesTolerates checks if a toleration matches a taint
func (a *ResourceAnalyzer) podToleratesTolerates(
	tolerations []corev1.Toleration,
	taint *corev1.Taint,
) bool {
	for _, toleration := range tolerations {
		if toleration.ToleratesTaint(taint) {
			return true
		}
	}
	return false
}

// calculateMatchScore calculates a score for how well a NodeGroup matches the demand
func (a *ResourceAnalyzer) calculateMatchScore(
	ng *v1alpha1.NodeGroup,
	matchingPods []*corev1.Pod,
	deficit ResourceDeficit,
) int {
	score := 0

	// More matching pods = higher score
	score += len(matchingPods) * 100

	// Prefer NodeGroups with capacity to scale
	availableCapacity := ng.Spec.MaxNodes - ng.Status.DesiredNodes
	if availableCapacity > 0 {
		score += int(availableCapacity) * 50
	}

	// Prefer NodeGroups that are not at max capacity
	if ng.Status.DesiredNodes < ng.Spec.MaxNodes {
		score += 200
	}

	// Prefer NodeGroups with PreferredInstanceType set
	if ng.Spec.PreferredInstanceType != "" {
		score += 100
	}

	return score
}

// sortNodeGroupMatches sorts matches by score (higher first)
func sortNodeGroupMatches(matches []NodeGroupMatch) {
	// Simple bubble sort (fine for small lists)
	n := len(matches)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if matches[j].Score < matches[j+1].Score {
				matches[j], matches[j+1] = matches[j+1], matches[j]
			}
		}
	}
}

// EstimateNodesNeeded estimates how many nodes are needed to satisfy the deficit
func (a *ResourceAnalyzer) EstimateNodesNeeded(
	deficit ResourceDeficit,
	instanceType v1alpha1.InstanceTypeInfo,
) int {
	if deficit.Pods == 0 {
		return 0
	}

	// Calculate based on CPU
	cpuMillis := deficit.CPU.MilliValue()
	instanceCPUMillis := int64(instanceType.CPU) * 1000
	nodesByCPU := (cpuMillis + instanceCPUMillis - 1) / instanceCPUMillis // Ceiling division

	// Calculate based on memory
	memoryBytes := deficit.Memory.Value()
	instanceMemoryBytes := int64(instanceType.MemoryMB) * 1024 * 1024
	nodesByMemory := (memoryBytes + instanceMemoryBytes - 1) / instanceMemoryBytes

	// Calculate based on pod count (assume 110 pods per node max)
	maxPodsPerNode := int64(110)
	nodesByPods := (int64(deficit.Pods) + maxPodsPerNode - 1) / maxPodsPerNode

	// Take the maximum
	nodesNeeded := nodesByCPU
	if nodesByMemory > nodesNeeded {
		nodesNeeded = nodesByMemory
	}
	if nodesByPods > nodesNeeded {
		nodesNeeded = nodesByPods
	}

	// Return at least 1 if there's any deficit
	if nodesNeeded < 1 {
		nodesNeeded = 1
	}

	a.logger.Debug("Estimated nodes needed",
		zap.Int64("nodesByCPU", nodesByCPU),
		zap.Int64("nodesByMemory", nodesByMemory),
		zap.Int64("nodesByPods", nodesByPods),
		zap.Int64("nodesNeeded", nodesNeeded),
	)

	return int(nodesNeeded)
}

// SelectInstanceType selects the optimal instance type for a NodeGroup
func (a *ResourceAnalyzer) SelectInstanceType(
	ng *v1alpha1.NodeGroup,
	deficit ResourceDeficit,
) (string, error) {
	// If PreferredInstanceType is set and available, use it
	if ng.Spec.PreferredInstanceType != "" {
		for _, offering := range ng.Spec.OfferingIDs {
			if offering == ng.Spec.PreferredInstanceType {
				a.logger.Debug("Using preferred instance type",
					zap.String("nodeGroup", ng.Name),
					zap.String("instanceType", offering),
				)
				return offering, nil
			}
		}
	}

	// Otherwise, select the first available offering
	// TODO: In future, implement cost-aware selection based on deficit
	if len(ng.Spec.OfferingIDs) > 0 {
		selected := ng.Spec.OfferingIDs[0]
		a.logger.Debug("Selected instance type",
			zap.String("nodeGroup", ng.Name),
			zap.String("instanceType", selected),
		)
		return selected, nil
	}

	return "", fmt.Errorf("no instance types available for NodeGroup %s", ng.Name)
}
