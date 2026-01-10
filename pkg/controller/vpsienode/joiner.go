package vpsienode

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// Joiner handles node joining operations
type Joiner struct {
	client      client.Client
	provisioner *Provisioner
}

// NewJoiner creates a new Joiner
func NewJoiner(client client.Client, provisioner *Provisioner) *Joiner {
	return &Joiner{
		client:      client,
		provisioner: provisioner,
	}
}

// PrepareJoin prepares the node for joining the Kubernetes cluster
func (j *Joiner) PrepareJoin(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) error {
	logger.Info("Preparing node for joining",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", vn.Spec.NodeName),
	)

	// At this point, the VPS is running and cloud-init should be executing
	// The cloud-init user data should contain kubeadm join commands
	// We just need to wait for the node to appear in Kubernetes

	return nil
}

// CheckJoinStatus checks if the node has joined the Kubernetes cluster
func (j *Joiner) CheckJoinStatus(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Debug("Checking node join status",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", vn.Spec.NodeName),
	)

	// Find the Kubernetes Node by various methods
	node, err := j.findKubernetesNode(ctx, vn, logger)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("Node not found in Kubernetes yet",
				zap.String("vpsienode", vn.Name),
				zap.String("nodeName", vn.Spec.NodeName),
			)
			// Node hasn't joined yet, keep waiting
			return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
		}

		logger.Error("Failed to check for Kubernetes node",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	// Node found! It has joined the cluster
	logger.Info("Node has joined the Kubernetes cluster",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", node.Name),
	)

	// Update VPSieNode status
	vn.Status.NodeName = node.Name
	if vn.Spec.NodeName == "" {
		vn.Spec.NodeName = node.Name
	}

	// Apply labels and taints from NodeGroup
	if err := j.applyNodeConfiguration(ctx, vn, node, logger); err != nil {
		logger.Error("Failed to apply node configuration",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", node.Name),
			zap.Error(err),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	// Check if node is ready
	nodeReady := j.isNodeReady(node)
	if nodeReady {
		logger.Info("Node is ready",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", node.Name),
		)

		// Transition to Ready phase
		SetPhase(vn, v1alpha1.VPSieNodePhaseReady, ReasonReady, "Node is ready")
		SetNodeJoinedCondition(vn, true, ReasonJoined, "Node has joined the cluster")
		SetNodeReadyCondition(vn, true, ReasonReady, "Node is ready")

		now := metav1.Now()
		if vn.Status.JoinedAt == nil {
			vn.Status.JoinedAt = &now
		}
		if vn.Status.ReadyAt == nil {
			vn.Status.ReadyAt = &now
		}

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	// Node has joined but not ready yet
	logger.Debug("Node has joined but is not ready yet",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", node.Name),
	)

	SetNodeJoinedCondition(vn, true, ReasonJoined, "Node has joined but not ready")
	SetNodeReadyCondition(vn, false, ReasonJoining, "Node is not ready yet")

	now := metav1.Now()
	if vn.Status.JoinedAt == nil {
		vn.Status.JoinedAt = &now
	}

	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// MonitorNode monitors the health of a ready node
func (j *Joiner) MonitorNode(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Debug("Monitoring node health",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", vn.Status.NodeName),
	)

	// Get the Kubernetes Node
	node := &corev1.Node{}
	err := j.client.Get(ctx, types.NamespacedName{Name: vn.Status.NodeName}, node)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Warn("Node disappeared from Kubernetes",
				zap.String("vpsienode", vn.Name),
				zap.String("nodeName", vn.Status.NodeName),
			)

			// Node disappeared, might have been deleted
			// TODO: Decide whether to recreate or mark as failed
			SetNodeReadyCondition(vn, false, ReasonNodeNotFound, "Node not found in Kubernetes")
			return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
		}

		logger.Error("Failed to get Kubernetes node",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", vn.Status.NodeName),
			zap.Error(err),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	// Check node readiness
	nodeReady := j.isNodeReady(node)
	if !nodeReady {
		logger.Warn("Node is no longer ready",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", node.Name),
		)
		SetNodeReadyCondition(vn, false, "NotReady", "Node is not ready")
	} else {
		SetNodeReadyCondition(vn, true, ReasonReady, "Node is ready")
	}

	return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
}

// findKubernetesNode finds the Kubernetes Node corresponding to the VPSieNode
// Uses IP-first matching strategy for reliability in async provisioning scenarios
func (j *Joiner) findKubernetesNode(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (*corev1.Node, error) {
	// Strategy 1: Try finding by IP address first (most reliable)
	// IP addresses are stable identifiers in cloud environments
	if vn.Spec.IPAddress != "" {
		node, err := j.findNodeByIP(ctx, vn.Spec.IPAddress)
		if err == nil && node != nil {
			logger.Debug("Found node by IP address",
				zap.String("ip", vn.Spec.IPAddress),
				zap.String("nodeName", node.Name),
			)
			return node, nil
		}
		if err != nil && !errors.IsNotFound(err) {
			logger.Debug("Error finding node by IP", zap.Error(err))
		}
	}

	// Strategy 2: Try finding by exact node name
	if vn.Spec.NodeName != "" {
		node := &corev1.Node{}
		err := j.client.Get(ctx, types.NamespacedName{Name: vn.Spec.NodeName}, node)
		if err == nil {
			logger.Debug("Found node by name",
				zap.String("nodeName", vn.Spec.NodeName),
			)
			return node, nil
		}
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	// Strategy 3: Try finding by hostname (fallback)
	if vn.Status.Hostname != "" {
		node, err := j.findNodeByHostname(ctx, vn.Status.Hostname)
		if err == nil && node != nil {
			logger.Debug("Found node by hostname",
				zap.String("hostname", vn.Status.Hostname),
				zap.String("nodeName", node.Name),
			)
			return node, nil
		}
		if err != nil && !errors.IsNotFound(err) {
			logger.Debug("Error finding node by hostname", zap.Error(err))
		}
	}

	return nil, errors.NewNotFound(corev1.Resource("node"), vn.Spec.NodeName)
}

// findNodeByIP finds a node by its IP address
func (j *Joiner) findNodeByIP(ctx context.Context, ip string) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := j.client.List(ctx, nodeList); err != nil {
		return nil, err
	}

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				if addr.Address == ip {
					return node, nil
				}
			}
		}
	}

	return nil, errors.NewNotFound(corev1.Resource("node"), ip)
}

// findNodeByHostname finds a node by its hostname
func (j *Joiner) findNodeByHostname(ctx context.Context, hostname string) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := j.client.List(ctx, nodeList); err != nil {
		return nil, err
	}

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeHostName {
				if addr.Address == hostname {
					return node, nil
				}
			}
		}
		// Also check node name as a fallback
		if node.Name == hostname {
			return node, nil
		}
	}

	return nil, errors.NewNotFound(corev1.Resource("node"), hostname)
}

// isNodeReady checks if a Kubernetes Node is ready
func (j *Joiner) isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// applyNodeConfiguration applies labels and taints from NodeGroup to the Kubernetes Node
func (j *Joiner) applyNodeConfiguration(ctx context.Context, vn *v1alpha1.VPSieNode, node *corev1.Node, logger *zap.Logger) error {
	logger.Info("Applying node configuration",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", node.Name),
	)

	// Get the NodeGroup to retrieve labels and taints
	// TODO: Implement label and taint application from NodeGroup spec
	// For now, just add basic labels

	updated := false

	// Add VPSie autoscaler labels
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	requiredLabels := map[string]string{
		v1alpha1.ManagedLabelKey:    v1alpha1.ManagedLabelValue,
		v1alpha1.NodeGroupLabelKey:  vn.Spec.NodeGroupName,
		v1alpha1.VPSieNodeLabelKey:  vn.Name,
		v1alpha1.DatacenterLabelKey: vn.Spec.DatacenterID,
	}

	for key, value := range requiredLabels {
		if node.Labels[key] != value {
			node.Labels[key] = value
			updated = true
		}
	}

	if updated {
		logger.Info("Updating node labels",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", node.Name),
		)
		if err := j.client.Update(ctx, node); err != nil {
			return fmt.Errorf("failed to update node labels: %w", err)
		}
	}

	return nil
}

// GetNode gets the Kubernetes Node for a VPSieNode
func (j *Joiner) GetNode(ctx context.Context, vn *v1alpha1.VPSieNode) (*corev1.Node, error) {
	if vn.Status.NodeName == "" {
		return nil, fmt.Errorf("node name not set")
	}

	node := &corev1.Node{}
	err := j.client.Get(ctx, types.NamespacedName{Name: vn.Status.NodeName}, node)
	if err != nil {
		return nil, err
	}

	return node, nil
}
