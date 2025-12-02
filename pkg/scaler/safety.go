package scaler

import (
	"context"
	"fmt"
	"strings"

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

	// Check 1: Node has no pods with local storage
	if hasLocalStorage, reason := s.hasPodsWithLocalStorage(ctx, pods); hasLocalStorage {
		return false, reason, nil
	}

	// Check 2: All pods can be scheduled elsewhere
	if canSchedule, reason, err := s.canPodsBeRescheduled(ctx, pods); err != nil {
		return false, "", err
	} else if !canSchedule {
		return false, reason, nil
	}

	// Check 3: System pods have alternatives
	if hasUniqueSystem, reason := s.hasUniqueSystemPods(pods); hasUniqueSystem {
		return false, reason, nil
	}

	// Check 4: No pod anti-affinity violations
	if hasViolation, reason, err := s.hasAntiAffinityViolations(ctx, pods); err != nil {
		return false, "", err
	} else if hasViolation {
		return false, reason, nil
	}

	// Check 5: Cluster has sufficient capacity after removal
	if insufficient, reason, err := s.hasInsufficientCapacity(ctx, node, pods); err != nil {
		return false, "", err
	} else if insufficient {
		return false, reason, nil
	}

	// Check 6: Node is not annotated as protected
	if s.isNodeProtected(node) {
		return false, "node is protected", nil
	}

	s.logger.Debug("all safety checks passed", "node", node.Name)
	return true, "safe to remove", nil
}

// hasPodsWithLocalStorage checks if any pods use local storage volumes
func (s *ScaleDownManager) hasPodsWithLocalStorage(ctx context.Context, pods []*corev1.Pod) (bool, string) {
	for _, pod := range pods {
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
