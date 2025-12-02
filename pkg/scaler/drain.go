package scaler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// Retry configuration for pod eviction
	EvictionRetryInterval = 5 * time.Second
	MaxEvictionRetries    = 12 // Total 1 minute with 5s intervals

	// Annotations
	DrainStartTimeAnnotation = "autoscaler.vpsie.com/drain-start-time"
	DrainStatusAnnotation    = "autoscaler.vpsie.com/drain-status"
)

// DrainNode safely drains a node by evicting all pods.
//
// Cleanup Policy:
// - On PDB validation failure: Uncordons the node (rollback)
// - On eviction timeout: Leaves node cordoned (pods still terminating)
// - On eviction failure: Uncordons only if not a timeout (allows retry)
// - On successful drain: Leaves node cordoned for controller deletion
//
// This ensures that nodes remain cordoned when pods are actively terminating,
// preventing new pods from scheduling on nodes that are being removed.
func (s *ScaleDownManager) DrainNode(ctx context.Context, node *corev1.Node) error {
	s.logger.Info("starting node drain",
		zap.String("node", node.Name))

	// Step 1: Cordon the node
	if err := s.cordonNode(ctx, node); err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	// Step 2: Get all pods on the node
	pods, err := s.getNodePods(ctx, node.Name)
	if err != nil {
		// Uncordon on failure using fresh context (don't propagate cancellation)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = s.uncordonNode(cleanupCtx, node)
		return fmt.Errorf("failed to get pods: %w", err)
	}

	// Step 3: Filter pods that need eviction
	podsToEvict := s.filterPodsForEviction(pods)

	if len(podsToEvict) == 0 {
		s.logger.Info("no pods to evict",
			zap.String("node", node.Name))
		return nil
	}

	s.logger.Info("evicting pods from node",
		zap.String("node", node.Name),
		zap.Int("podCount", len(podsToEvict)))

	// Step 4: Check PodDisruptionBudgets
	if err := s.ValidatePodDisruptionBudgets(ctx, podsToEvict); err != nil {
		// Uncordon on failure using fresh context
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = s.uncordonNode(cleanupCtx, node)
		return fmt.Errorf("PDB validation failed: %w", err)
	}

	// Step 5: Annotate node with drain start time
	if err := s.annotateNodeDrainStart(ctx, node); err != nil {
		s.logger.Warn("failed to annotate drain start",
			zap.Error(err))
	}

	// Step 6: Evict pods
	// Use detached context for drain to allow completion even if parent is cancelled
	// This ensures graceful drain during controller shutdown
	drainCtx, drainCancel := context.WithTimeout(context.Background(), s.config.DrainTimeout)
	defer drainCancel()

	// Monitor parent context cancellation in parallel
	parentCancelled := false
	done := make(chan error, 1)

	go func() {
		done <- s.evictPods(drainCtx, podsToEvict)
	}()

	var evictErr error
	select {
	case evictErr = <-done:
		// Eviction completed normally
	case <-ctx.Done():
		// Parent context cancelled (e.g., controller shutdown)
		parentCancelled = true
		s.logger.Warn("drain operation detached due to parent cancellation, continuing in background",
			zap.String("node", node.Name))
		// Wait for eviction to complete with drain timeout
		evictErr = <-done
	}

	if evictErr != nil {
		// Use fresh context for cleanup operations
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		// Mark drain as failed
		_ = s.annotateNodeDrainStatus(cleanupCtx, node, "failed")

		// Only uncordon on non-timeout failures
		// If it's a timeout, pods are still being evicted, so leave node cordoned
		// If it's a PDB violation or other error, rollback by uncordoning
		if drainCtx.Err() != context.DeadlineExceeded && drainCtx.Err() != context.Canceled && !parentCancelled {
			s.logger.Info("uncordoning node due to eviction failure",
				zap.String("node", node.Name),
				zap.Error(evictErr))
			_ = s.uncordonNode(cleanupCtx, node)
		} else {
			s.logger.Info("leaving node cordoned - eviction in progress or parent cancelled",
				zap.String("node", node.Name))
		}

		if parentCancelled {
			return fmt.Errorf("drain detached (parent cancelled), eviction result: %w", evictErr)
		}
		return fmt.Errorf("failed to evict pods: %w", evictErr)
	}

	// Step 7: Wait for pod termination
	// Use drain context (detached) for termination wait as well
	if err := s.waitForPodTermination(drainCtx, node.Name, podsToEvict); err != nil {
		// Use fresh context for cleanup
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = s.annotateNodeDrainStatus(cleanupCtx, node, "timeout")

		// Don't uncordon here - pods are being terminated
		if parentCancelled {
			return fmt.Errorf("drain detached (parent cancelled), termination wait failed: %w", err)
		}
		return fmt.Errorf("timeout waiting for pods to terminate: %w", err)
	}

	// Step 8: Verify successful migration
	// Use drain context for final verification
	remainingPods, err := s.getNodePods(drainCtx, node.Name)
	if err != nil {
		if parentCancelled {
			return fmt.Errorf("drain detached (parent cancelled), migration verification failed: %w", err)
		}
		return fmt.Errorf("failed to verify pod migration: %w", err)
	}

	// Filter out DaemonSets and static pods
	activePods := s.filterPodsForEviction(remainingPods)
	if len(activePods) > 0 {
		if parentCancelled {
			return fmt.Errorf("drain detached (parent cancelled), %d pods still running", len(activePods))
		}
		return fmt.Errorf("drain incomplete: %d pods still running", len(activePods))
	}

	// Mark drain as complete using fresh context
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()
	_ = s.annotateNodeDrainStatus(cleanupCtx, node, "complete")

	s.logger.Info("node drain completed successfully",
		zap.String("node", node.Name),
		zap.Int("podsEvicted", len(podsToEvict)),
		zap.Bool("detached", parentCancelled))

	return nil
}

// cordonNode marks a node as unschedulable
func (s *ScaleDownManager) cordonNode(ctx context.Context, node *corev1.Node) error {
	if node.Spec.Unschedulable {
		return nil // Already cordoned
	}

	nodeCopy := node.DeepCopy()
	nodeCopy.Spec.Unschedulable = true

	_, err := s.client.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	s.logger.Info("node cordoned",
		zap.String("node", node.Name))
	return nil
}

// uncordonNode marks a node as schedulable (rollback)
func (s *ScaleDownManager) uncordonNode(ctx context.Context, node *corev1.Node) error {
	if !node.Spec.Unschedulable {
		return nil // Already uncordoned
	}

	nodeCopy := node.DeepCopy()
	nodeCopy.Spec.Unschedulable = false

	_, err := s.client.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to uncordon node: %w", err)
	}

	s.logger.Info("node uncordoned (rollback)",
		zap.String("node", node.Name))
	return nil
}

// filterPodsForEviction filters pods that need eviction (excludes DaemonSets, static pods)
func (s *ScaleDownManager) filterPodsForEviction(pods []*corev1.Pod) []*corev1.Pod {
	var filtered []*corev1.Pod

	for _, pod := range pods {
		// Skip pods in terminal states
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		// Skip DaemonSet pods
		if isDaemonSetPod(pod) {
			s.logger.Debug("skipping DaemonSet pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace))
			continue
		}

		// Skip static pods (mirror pods)
		if isStaticPod(pod) {
			s.logger.Debug("skipping static pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace))
			continue
		}

		filtered = append(filtered, pod)
	}

	return filtered
}

// evictPods evicts all pods with retries
func (s *ScaleDownManager) evictPods(ctx context.Context, pods []*corev1.Pod) error {
	gracePeriod := int64(s.config.EvictionGracePeriod)

	for _, pod := range pods {
		if err := s.evictPodWithRetry(ctx, pod, gracePeriod); err != nil {
			return fmt.Errorf("failed to evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	return nil
}

// evictPodWithRetry evicts a single pod with retry logic
func (s *ScaleDownManager) evictPodWithRetry(ctx context.Context, pod *corev1.Pod, gracePeriod int64) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		},
	}

	var lastErr error

	for attempt := 0; attempt < MaxEvictionRetries; attempt++ {
		err := s.client.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)

		if err == nil {
			s.logger.Info("pod evicted",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
				zap.Int("attempt", attempt+1))
			return nil
		}

		// If pod is already gone, consider it successful
		if apierrors.IsNotFound(err) {
			return nil
		}

		// If eviction is not allowed due to PDB, fail immediately
		if apierrors.IsTooManyRequests(err) {
			return fmt.Errorf("eviction blocked by PodDisruptionBudget: %w", err)
		}

		lastErr = err

		s.logger.Warn("eviction attempt failed",
			zap.String("pod", pod.Name),
			zap.String("namespace", pod.Namespace),
			zap.Int("attempt", attempt+1),
			zap.Error(err))

		// Wait before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(EvictionRetryInterval):
		}
	}

	return fmt.Errorf("max eviction retries exceeded: %w", lastErr)
}

// waitForPodTermination waits for all evicted pods to terminate
func (s *ScaleDownManager) waitForPodTermination(ctx context.Context, nodeName string, evictedPods []*corev1.Pod) error {
	// Create map of evicted pod UIDs for tracking
	evictedUIDs := make(map[string]bool)
	for _, pod := range evictedPods {
		evictedUIDs[string(pod.UID)] = true
	}

	s.logger.Info("waiting for pod termination",
		zap.String("node", nodeName),
		zap.Int("podCount", len(evictedPods)))

	// Poll for pod termination
	err := wait.PollImmediate(5*time.Second, s.config.DrainTimeout, func() (bool, error) {
		// Get current pods on node
		currentPods, err := s.getNodePods(ctx, nodeName)
		if err != nil {
			return false, err
		}

		// Count how many evicted pods are still running
		stillRunning := 0
		for _, pod := range currentPods {
			if evictedUIDs[string(pod.UID)] {
				stillRunning++
				s.logger.Debug("pod still running",
					zap.String("pod", pod.Name),
					zap.String("namespace", pod.Namespace),
					zap.String("phase", string(pod.Status.Phase)))
			}
		}

		if stillRunning == 0 {
			s.logger.Info("all pods terminated",
				zap.String("node", nodeName))
			return true, nil
		}

		s.logger.Debug("waiting for pods to terminate",
			zap.String("node", nodeName),
			zap.Int("remaining", stillRunning))

		return false, nil
	})

	return err
}

// annotateNodeDrainStart adds drain start time annotation
func (s *ScaleDownManager) annotateNodeDrainStart(ctx context.Context, node *corev1.Node) error {
	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	nodeCopy.Annotations[DrainStartTimeAnnotation] = time.Now().Format(time.RFC3339)
	nodeCopy.Annotations[DrainStatusAnnotation] = "draining"

	_, err := s.client.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	return err
}

// annotateNodeDrainStatus updates drain status annotation
func (s *ScaleDownManager) annotateNodeDrainStatus(ctx context.Context, node *corev1.Node, status string) error {
	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	nodeCopy.Annotations[DrainStatusAnnotation] = status

	_, err := s.client.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	return err
}

// Helper functions

func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func isStaticPod(pod *corev1.Pod) bool {
	// Static pods have a node name in the OwnerReferences
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "Node" {
			return true
		}
	}

	// Alternative check: static pods have specific annotations
	if _, exists := pod.Annotations["kubernetes.io/config.source"]; exists {
		return true
	}

	// Static pods also have this annotation
	if _, exists := pod.Annotations["kubernetes.io/config.mirror"]; exists {
		return true
	}

	return false
}

// IsNodeDraining checks if a node is currently being drained
func IsNodeDraining(node *corev1.Node) bool {
	if node.Annotations == nil {
		return false
	}

	status, exists := node.Annotations[DrainStatusAnnotation]
	return exists && status == "draining"
}

// GetNodeDrainDuration returns how long a node has been draining
func GetNodeDrainDuration(node *corev1.Node) time.Duration {
	if node.Annotations == nil {
		return 0
	}

	startTimeStr, exists := node.Annotations[DrainStartTimeAnnotation]
	if !exists {
		return 0
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return 0
	}

	return time.Since(startTime)
}
