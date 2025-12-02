package scaler

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const (
	// Maximum number of samples to keep per node
	MaxSamplesPerNode = 50

	// Interval for collecting utilization metrics
	DefaultMetricsCollectionInterval = 1 * time.Minute
)

// UpdateNodeUtilization collects and updates node utilization metrics
func (s *ScaleDownManager) UpdateNodeUtilization(ctx context.Context) error {
	// Get all nodes
	nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// Get node metrics from metrics-server
	nodeMetrics, err := s.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node metrics: %w", err)
	}

	// Create map for quick lookup
	metricsMap := make(map[string]*metricsv1beta1.NodeMetrics)
	for i := range nodeMetrics.Items {
		metricsMap[nodeMetrics.Items[i].Name] = &nodeMetrics.Items[i]
	}

	// Create map of current nodes for garbage collection
	currentNodes := make(map[string]bool)
	for i := range nodeList.Items {
		currentNodes[nodeList.Items[i].Name] = true
	}

	// Garbage collect deleted nodes from utilization map
	// This prevents unbounded memory growth over time
	s.utilizationLock.Lock()
	for nodeName := range s.nodeUtilization {
		if !currentNodes[nodeName] {
			delete(s.nodeUtilization, nodeName)
			s.logger.Debug("removed deleted node from utilization tracking", "node", nodeName)
		}
	}
	s.utilizationLock.Unlock()

	// Update utilization for each node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// Skip master nodes
		if _, isMaster := node.Labels["node-role.kubernetes.io/master"]; isMaster {
			continue
		}
		if _, isMaster := node.Labels["node-role.kubernetes.io/control-plane"]; isMaster {
			continue
		}

		metrics, exists := metricsMap[node.Name]
		if !exists {
			s.logger.Warn("no metrics found for node", "node", node.Name)
			continue
		}

		if err := s.updateNodeUtilizationMetrics(ctx, node, metrics); err != nil {
			s.logger.Error("failed to update node utilization",
				"node", node.Name,
				"error", err)
			continue
		}
	}

	return nil
}

func (s *ScaleDownManager) updateNodeUtilizationMetrics(
	ctx context.Context,
	node *corev1.Node,
	metrics *metricsv1beta1.NodeMetrics,
) error {
	// Calculate CPU utilization
	cpuUsage := metrics.Usage.Cpu().MilliValue()
	cpuCapacity := node.Status.Capacity.Cpu().MilliValue()
	cpuUtilization := float64(cpuUsage) / float64(cpuCapacity) * 100

	// Calculate memory utilization
	memUsage := metrics.Usage.Memory().Value()
	memCapacity := node.Status.Capacity.Memory().Value()
	memUtilization := float64(memUsage) / float64(memCapacity) * 100

	// Create new sample
	sample := UtilizationSample{
		Timestamp:         time.Now(),
		CPUUtilization:    cpuUtilization,
		MemoryUtilization: memUtilization,
	}

	// Update utilization tracking
	s.utilizationLock.Lock()
	defer s.utilizationLock.Unlock()

	util, exists := s.nodeUtilization[node.Name]
	if !exists {
		util = &NodeUtilization{
			NodeName: node.Name,
			Samples:  make([]UtilizationSample, 0, MaxSamplesPerNode),
		}
		s.nodeUtilization[node.Name] = util
	}

	// Add new sample
	// Clone the slice before appending to prevent race conditions with readers
	// This ensures we never modify the underlying array that might be shared
	newSamples := make([]UtilizationSample, len(util.Samples), len(util.Samples)+1)
	copy(newSamples, util.Samples)
	newSamples = append(newSamples, sample)

	// Keep only recent samples
	if len(newSamples) > MaxSamplesPerNode {
		newSamples = newSamples[len(newSamples)-MaxSamplesPerNode:]
	}

	util.Samples = newSamples

	// Calculate rolling average
	util.CPUUtilization, util.MemoryUtilization = s.calculateRollingAverage(util.Samples)
	util.LastUpdated = time.Now()

	// Determine if underutilized
	util.IsUnderutilized = util.CPUUtilization < s.config.CPUThreshold &&
		util.MemoryUtilization < s.config.MemoryThreshold

	s.logger.Debug("updated node utilization",
		"node", node.Name,
		"cpu", fmt.Sprintf("%.2f%%", util.CPUUtilization),
		"memory", fmt.Sprintf("%.2f%%", util.MemoryUtilization),
		"underutilized", util.IsUnderutilized)

	return nil
}

func (s *ScaleDownManager) calculateRollingAverage(samples []UtilizationSample) (cpu, memory float64) {
	if len(samples) == 0 {
		return 0, 0
	}

	// Calculate average over observation window
	now := time.Now()
	windowStart := now.Add(-s.config.ObservationWindow)

	var cpuSum, memSum float64
	count := 0

	for _, sample := range samples {
		if sample.Timestamp.Before(windowStart) {
			continue
		}
		cpuSum += sample.CPUUtilization
		memSum += sample.MemoryUtilization
		count++
	}

	if count == 0 {
		return 0, 0
	}

	return cpuSum / float64(count), memSum / float64(count)
}

// GetNodeUtilization returns a deep copy of utilization data for a specific node
// Returns a copy to prevent external modification of internal state
func (s *ScaleDownManager) GetNodeUtilization(nodeName string) (*NodeUtilization, bool) {
	s.utilizationLock.RLock()
	defer s.utilizationLock.RUnlock()

	util, exists := s.nodeUtilization[nodeName]
	if !exists {
		return nil, false
	}

	// Return deep copy to prevent external modification
	copy := &NodeUtilization{
		NodeName:          util.NodeName,
		CPUUtilization:    util.CPUUtilization,
		MemoryUtilization: util.MemoryUtilization,
		IsUnderutilized:   util.IsUnderutilized,
		LastUpdated:       util.LastUpdated,
		Samples:           make([]UtilizationSample, len(util.Samples)),
	}
	copySlice(copy.Samples, util.Samples)

	return copy, true
}

// GetUnderutilizedNodes returns deep copies of all nodes currently marked as underutilized
// Returns copies to prevent external modification of internal state
func (s *ScaleDownManager) GetUnderutilizedNodes() []*NodeUtilization {
	s.utilizationLock.RLock()
	defer s.utilizationLock.RUnlock()

	var underutilized []*NodeUtilization
	for _, util := range s.nodeUtilization {
		if util.IsUnderutilized {
			// Create deep copy
			copy := &NodeUtilization{
				NodeName:          util.NodeName,
				CPUUtilization:    util.CPUUtilization,
				MemoryUtilization: util.MemoryUtilization,
				IsUnderutilized:   util.IsUnderutilized,
				LastUpdated:       util.LastUpdated,
				Samples:           make([]UtilizationSample, len(util.Samples)),
			}
			copySlice(copy.Samples, util.Samples)
			underutilized = append(underutilized, copy)
		}
	}

	return underutilized
}

// copySlice is a helper to copy UtilizationSample slices
func copySlice(dst, src []UtilizationSample) {
	for i := range src {
		dst[i] = src[i]
	}
}

// CalculateResourceRequests calculates total resource requests for pods
func CalculateResourceRequests(pods []*corev1.Pod) (cpu, memory int64) {
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			if req := container.Resources.Requests.Cpu(); req != nil {
				cpu += req.MilliValue()
			}
			if req := container.Resources.Requests.Memory(); req != nil {
				memory += req.Value()
			}
		}
	}
	return cpu, memory
}

// CalculateResourceLimits calculates total resource limits for pods
func CalculateResourceLimits(pods []*corev1.Pod) (cpu, memory int64) {
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			if limit := container.Resources.Limits.Cpu(); limit != nil {
				cpu += limit.MilliValue()
			}
			if limit := container.Resources.Limits.Memory(); limit != nil {
				memory += limit.Value()
			}
		}
	}
	return cpu, memory
}

// GetNodeAllocatableResources returns node's allocatable resources
func GetNodeAllocatableResources(node *corev1.Node) (cpu, memory int64) {
	if cpuRes := node.Status.Allocatable.Cpu(); cpuRes != nil {
		cpu = cpuRes.MilliValue()
	}
	if memRes := node.Status.Allocatable.Memory(); memRes != nil {
		memory = memRes.Value()
	}
	return cpu, memory
}

// CalculateNodeUtilizationFromPods calculates utilization based on pod requests
func CalculateNodeUtilizationFromPods(node *corev1.Node, pods []*corev1.Pod) (cpuUtil, memUtil float64) {
	cpuRequests, memRequests := CalculateResourceRequests(pods)
	cpuAllocatable, memAllocatable := GetNodeAllocatableResources(node)

	if cpuAllocatable > 0 {
		cpuUtil = float64(cpuRequests) / float64(cpuAllocatable) * 100
	}

	if memAllocatable > 0 {
		memUtil = float64(memRequests) / float64(memAllocatable) * 100
	}

	return cpuUtil, memUtil
}

// PredictUtilizationAfterRemoval predicts cluster utilization after removing nodes
func (s *ScaleDownManager) PredictUtilizationAfterRemoval(
	ctx context.Context,
	nodesToRemove []*corev1.Node,
	allNodes []*corev1.Node,
) (avgCPU, avgMemory, maxCPU, maxMemory float64, err error) {
	// Create map of nodes to remove
	removeMap := make(map[string]bool)
	for _, node := range nodesToRemove {
		removeMap[node.Name] = true
	}

	// Calculate remaining capacity
	var totalCPUCapacity, totalMemCapacity int64
	var totalCPURequests, totalMemRequests int64

	for _, node := range allNodes {
		if removeMap[node.Name] {
			continue // Skip nodes being removed
		}

		// Get node capacity
		cpuCap, memCap := GetNodeAllocatableResources(node)
		totalCPUCapacity += cpuCap
		totalMemCapacity += memCap

		// Get pods on node
		pods, err := s.getNodePods(ctx, node.Name)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to get pods for node %s: %w", node.Name, err)
		}

		// Calculate requests
		cpuReq, memReq := CalculateResourceRequests(pods)
		totalCPURequests += cpuReq
		totalMemRequests += memReq
	}

	// Add requests from pods on nodes being removed (they'll be rescheduled)
	for _, node := range nodesToRemove {
		pods, err := s.getNodePods(ctx, node.Name)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to get pods for node %s: %w", node.Name, err)
		}

		cpuReq, memReq := CalculateResourceRequests(pods)
		totalCPURequests += cpuReq
		totalMemRequests += memReq
	}

	// Calculate average utilization
	if totalCPUCapacity > 0 {
		avgCPU = float64(totalCPURequests) / float64(totalCPUCapacity) * 100
	}

	if totalMemCapacity > 0 {
		avgMemory = float64(totalMemRequests) / float64(totalMemCapacity) * 100
	}

	// For max utilization, we'd need to simulate scheduling
	// For now, use a conservative estimate (assume worst-case binpacking)
	remainingNodeCount := len(allNodes) - len(nodesToRemove)
	if remainingNodeCount > 0 {
		maxCPU = avgCPU * float64(len(allNodes)) / float64(remainingNodeCount)
		maxMemory = avgMemory * float64(len(allNodes)) / float64(remainingNodeCount)
	}

	return avgCPU, avgMemory, maxCPU, maxMemory, nil
}

// FormatResourceQuantity formats a resource quantity for display
func FormatResourceQuantity(quantity resource.Quantity, resourceType string) string {
	switch resourceType {
	case "cpu":
		return fmt.Sprintf("%.2f cores", float64(quantity.MilliValue())/1000)
	case "memory":
		return fmt.Sprintf("%.2f GiB", float64(quantity.Value())/(1024*1024*1024))
	default:
		return quantity.String()
	}
}
