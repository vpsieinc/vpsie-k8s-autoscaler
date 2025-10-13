package vpsienode

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

const (
	// DefaultDrainTimeout is the default timeout for draining a node
	DefaultDrainTimeout = 10 * time.Minute

	// DefaultPodEvictionTimeout is the timeout for a single pod eviction
	DefaultPodEvictionTimeout = 2 * time.Minute

	// PodDeletionGracePeriod is the grace period for pod deletion
	PodDeletionGracePeriod = 30 * time.Second

	// PollInterval is the interval for polling pod status
	PollInterval = 5 * time.Second
)

// Drainer handles graceful node draining
type Drainer struct {
	client        client.Client
	drainTimeout  time.Duration
	evictionRetry int
}

// NewDrainer creates a new Drainer
func NewDrainer(client client.Client) *Drainer {
	return &Drainer{
		client:        client,
		drainTimeout:  DefaultDrainTimeout,
		evictionRetry: 3,
	}
}

// DrainNode gracefully drains a node before deletion
func (d *Drainer) DrainNode(ctx context.Context, nodeName string, logger *zap.Logger) error {
	logger.Info("Starting node drain",
		zap.String("node", nodeName),
		zap.Duration("timeout", d.drainTimeout),
	)

	// Step 1: Cordon the node (mark as unschedulable)
	if err := d.cordonNode(ctx, nodeName, logger); err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	// Step 2: Get all pods on the node
	pods, err := d.getPodsOnNode(ctx, nodeName, logger)
	if err != nil {
		// Try to uncordon on failure
		_ = d.uncordonNode(ctx, nodeName, logger)
		return fmt.Errorf("failed to list pods on node: %w", err)
	}

	// Filter out DaemonSet pods and already terminated pods
	podsToEvict := d.filterPodsToEvict(pods, logger)

	logger.Info("Found pods to evict",
		zap.String("node", nodeName),
		zap.Int("totalPods", len(pods)),
		zap.Int("podsToEvict", len(podsToEvict)),
	)

	// Step 3: Evict all pods
	drainCtx, cancel := context.WithTimeout(ctx, d.drainTimeout)
	defer cancel()

	if err := d.evictPods(drainCtx, podsToEvict, logger); err != nil {
		// Try to uncordon on failure
		_ = d.uncordonNode(ctx, nodeName, logger)
		return fmt.Errorf("failed to evict pods: %w", err)
	}

	logger.Info("Successfully drained node", zap.String("node", nodeName))
	return nil
}

// cordonNode marks a node as unschedulable
func (d *Drainer) cordonNode(ctx context.Context, nodeName string, logger *zap.Logger) error {
	node := &corev1.Node{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Node not found, skipping cordon", zap.String("node", nodeName))
			return nil
		}
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Check if already cordoned
	if node.Spec.Unschedulable {
		logger.Debug("Node already cordoned", zap.String("node", nodeName))
		return nil
	}

	// Mark as unschedulable
	node.Spec.Unschedulable = true
	if err := d.client.Update(ctx, node); err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	logger.Info("Successfully cordoned node", zap.String("node", nodeName))
	return nil
}

// uncordonNode marks a node as schedulable (rollback operation)
func (d *Drainer) uncordonNode(ctx context.Context, nodeName string, logger *zap.Logger) error {
	node := &corev1.Node{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Node not found, skipping uncordon", zap.String("node", nodeName))
			return nil
		}
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Check if already schedulable
	if !node.Spec.Unschedulable {
		logger.Debug("Node already uncordoned", zap.String("node", nodeName))
		return nil
	}

	// Mark as schedulable
	node.Spec.Unschedulable = false
	if err := d.client.Update(ctx, node); err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	logger.Warn("Uncordoned node after drain failure", zap.String("node", nodeName))
	return nil
}

// getPodsOnNode returns all pods running on a node
func (d *Drainer) getPodsOnNode(ctx context.Context, nodeName string, logger *zap.Logger) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}

	// List pods on the specific node
	err := d.client.List(ctx, podList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return podList.Items, nil
}

// filterPodsToEvict filters out pods that should not be evicted
func (d *Drainer) filterPodsToEvict(pods []corev1.Pod, logger *zap.Logger) []corev1.Pod {
	var podsToEvict []corev1.Pod

	for _, pod := range pods {
		// Skip pods that are already terminating or terminated
		if pod.DeletionTimestamp != nil {
			logger.Debug("Skipping pod that is already terminating",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
			)
			continue
		}

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			logger.Debug("Skipping terminated pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
				zap.String("phase", string(pod.Status.Phase)),
			)
			continue
		}

		// Skip DaemonSet pods (they are managed by DaemonSet controller)
		if isDaemonSetPod(&pod) {
			logger.Debug("Skipping DaemonSet pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
			)
			continue
		}

		// Skip static pods (managed by kubelet)
		if isStaticPod(&pod) {
			logger.Debug("Skipping static pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
			)
			continue
		}

		podsToEvict = append(podsToEvict, pod)
	}

	return podsToEvict
}

// evictPods evicts all pods from the node
func (d *Drainer) evictPods(ctx context.Context, pods []corev1.Pod, logger *zap.Logger) error {
	if len(pods) == 0 {
		logger.Info("No pods to evict")
		return nil
	}

	// Create eviction for each pod
	for _, pod := range pods {
		if err := d.evictPod(ctx, &pod, logger); err != nil {
			logger.Warn("Failed to create eviction for pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
				zap.Error(err),
			)
			// Continue with other pods even if one fails
		}
	}

	// Wait for all pods to be deleted
	return d.waitForPodsDeleted(ctx, pods, logger)
}

// evictPod creates an eviction for a single pod
func (d *Drainer) evictPod(ctx context.Context, pod *corev1.Pod, logger *zap.Logger) error {
	logger.Info("Evicting pod",
		zap.String("pod", pod.Name),
		zap.String("namespace", pod.Namespace),
	)

	// Create eviction object
	gracePeriod := int64(PodDeletionGracePeriod.Seconds())
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		},
	}

	// Try to evict the pod (respects PodDisruptionBudgets)
	err := d.client.SubResource("eviction").Create(ctx, pod, eviction)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Pod already deleted
			logger.Debug("Pod already deleted",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
			)
			return nil
		}
		if apierrors.IsTooManyRequests(err) {
			// PodDisruptionBudget prevents eviction
			logger.Warn("PodDisruptionBudget prevents eviction, will retry",
				zap.String("pod", pod.Name),
				zap.String("namespace", pod.Namespace),
			)
			return err
		}
		return fmt.Errorf("failed to create eviction: %w", err)
	}

	logger.Debug("Successfully created eviction",
		zap.String("pod", pod.Name),
		zap.String("namespace", pod.Namespace),
	)
	return nil
}

// waitForPodsDeleted waits for all pods to be deleted
func (d *Drainer) waitForPodsDeleted(ctx context.Context, pods []corev1.Pod, logger *zap.Logger) error {
	logger.Info("Waiting for pods to be deleted", zap.Int("count", len(pods)))

	return wait.PollUntilContextTimeout(ctx, PollInterval, d.drainTimeout, true, func(ctx context.Context) (bool, error) {
		remaining := 0
		for _, pod := range pods {
			currentPod := &corev1.Pod{}
			err := d.client.Get(ctx, client.ObjectKey{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			}, currentPod)

			if err != nil {
				if apierrors.IsNotFound(err) {
					// Pod is deleted
					continue
				}
				logger.Warn("Error checking pod status",
					zap.String("pod", pod.Name),
					zap.String("namespace", pod.Namespace),
					zap.Error(err),
				)
				continue
			}

			remaining++
		}

		if remaining > 0 {
			logger.Debug("Waiting for pods to terminate", zap.Int("remaining", remaining))
			return false, nil
		}

		logger.Info("All pods have been deleted")
		return true, nil
	})
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// isStaticPod checks if a pod is a static pod
func isStaticPod(pod *corev1.Pod) bool {
	// Static pods have a mirror pod annotation
	if _, exists := pod.Annotations["kubernetes.io/config.mirror"]; exists {
		return true
	}
	// Static pods also have no controller reference
	return len(pod.OwnerReferences) == 0 && pod.Namespace == "kube-system"
}

// DeleteNode deletes the Kubernetes Node object
func (d *Drainer) DeleteNode(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) error {
	nodeName := vn.Status.NodeName
	if nodeName == "" {
		nodeName = vn.Spec.NodeName
	}

	if nodeName == "" {
		logger.Info("No node name set, skipping node deletion")
		return nil
	}

	logger.Info("Deleting Kubernetes Node object", zap.String("node", nodeName))

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}

	err := d.client.Delete(ctx, node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Node not found, already deleted", zap.String("node", nodeName))
			return nil
		}
		return fmt.Errorf("failed to delete node: %w", err)
	}

	logger.Info("Successfully deleted Kubernetes Node", zap.String("node", nodeName))
	return nil
}
