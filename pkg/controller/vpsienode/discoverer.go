package vpsienode

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// DiscoveryTimeout is the maximum time to wait for VPS discovery
const DiscoveryTimeout = 15 * time.Minute

// Discoverer handles VPS and K8s node discovery for async provisioning
type Discoverer struct {
	vpsieClient VPSieClientInterface
	k8sClient   client.Client
	logger      *zap.Logger
}

// NewDiscoverer creates a new Discoverer
func NewDiscoverer(vpsieClient VPSieClientInterface, k8sClient client.Client, logger *zap.Logger) *Discoverer {
	return &Discoverer{
		vpsieClient: vpsieClient,
		k8sClient:   k8sClient,
		logger:      logger.Named("discoverer"),
	}
}

// DiscoverVPSID attempts to discover the VPS ID for a VPSieNode
// that was created via async provisioning (creation-requested=true, VPSieInstanceID=0)
//
// Strategy:
// 1. List all VMs from VPSie API
// 2. Filter to running VMs
// 3. Sort by creation time (newest first)
// 4. Match by hostname pattern (VPSieNode name as prefix)
// 5. Fallback: Match by IP if K8s node exists with matching IP
//
// Returns:
//   - *vpsieclient.VPS: Discovered VPS (nil if not found)
//   - bool: Whether discovery timed out
//   - error: API errors
func (d *Discoverer) DiscoverVPSID(ctx context.Context, vn *v1alpha1.VPSieNode) (*vpsieclient.VPS, bool, error) {
	logger := d.logger.With(
		zap.String("vpsienode", vn.Name),
		zap.String("cluster", vn.Spec.ResourceIdentifier),
		zap.Int("groupID", vn.Spec.VPSieGroupID),
	)

	// Check timeout
	if vn.Status.CreatedAt != nil {
		elapsed := time.Since(vn.Status.CreatedAt.Time)
		if elapsed > DiscoveryTimeout {
			logger.Warn("VPS discovery timeout exceeded",
				zap.Duration("elapsed", elapsed),
				zap.Duration("timeout", DiscoveryTimeout),
			)
			return nil, true, nil
		}
	}

	// List all VMs from VPSie API
	allVMs, err := d.vpsieClient.ListVMs(ctx)
	if err != nil {
		logger.Error("Failed to list VMs for discovery", zap.Error(err))
		return nil, false, fmt.Errorf("failed to list VMs: %w", err)
	}

	// Count VMs by status for debugging
	statusCounts := make(map[string]int)
	for _, vm := range allVMs {
		statusCounts[vm.Status]++
	}
	logger.Info("VPS discovery: listed VMs from API",
		zap.Int("totalVMs", len(allVMs)),
		zap.Any("statusCounts", statusCounts),
	)

	// Filter VMs to running status (candidates for discovery)
	// Also include VMs with empty status since VPSie API may not populate status field
	var candidates []vpsieclient.VPS
	for _, vm := range allVMs {
		// Include running VMs or VMs with empty status (API may not populate)
		if vm.Status == "running" || vm.Status == "" {
			candidates = append(candidates, vm)
		}
	}

	if len(candidates) == 0 {
		logger.Info("VPS discovery: no candidate VMs found for discovery",
			zap.Int("totalVMs", len(allVMs)),
		)
		return nil, false, nil
	}

	logger.Info("VPS discovery: found candidate VMs",
		zap.Int("candidates", len(candidates)),
	)

	// Sort by creation time (newest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})

	// Find VPS that matches our VPSieNode
	// Strategy 3 (primary for VPSie K8s service): Find unclaimed K8s nodes
	// VPSie K8s managed clusters don't expose individual node VPS IDs through the API
	// So we match K8s nodes directly without requiring a VPS ID
	unclaimedNode, err := d.findUnclaimedK8sNode(ctx, vn)
	if err == nil && unclaimedNode != nil {
		nodeIP := d.getNodeIP(unclaimedNode)
		if nodeIP != "" {
			// Try to find VM with this IP for completeness
			for i := range candidates {
				vm := &candidates[i]
				if vm.IPAddress == nodeIP {
					logger.Info("Discovered VPS by unclaimed K8s node IP",
						zap.Int("vpsID", vm.ID),
						zap.String("ip", vm.IPAddress),
						zap.String("k8sNode", unclaimedNode.Name),
						zap.String("hostname", vm.Hostname),
					)
					return vm, false, nil
				}
			}
			// VPSie K8s-managed nodes don't appear in ListVMs
			// Create a synthetic VPS entry with just the K8s node info
			logger.Info("Discovered K8s node (VPSie K8s-managed, no VPS ID available)",
				zap.String("k8sNode", unclaimedNode.Name),
				zap.String("nodeIP", nodeIP),
			)
			return &vpsieclient.VPS{
				ID:        0, // VPS ID not available for K8s-managed nodes
				Hostname:  unclaimedNode.Name,
				IPAddress: nodeIP,
				Status:    "running", // K8s node exists so it's running
			}, false, nil
		}
	}

	for i := range candidates {
		vm := &candidates[i]

		// Strategy 1: Match by hostname pattern (VPSieNode name should be prefix)
		if matchesHostnamePattern(vn.Name, vm.Hostname) {
			logger.Info("Discovered VPS by hostname pattern",
				zap.Int("vpsID", vm.ID),
				zap.String("hostname", vm.Hostname),
			)
			return vm, false, nil
		}

		// Strategy 2: Match by IP if we can correlate with K8s node
		if vm.IPAddress != "" {
			k8sNode, err := d.findK8sNodeByIP(ctx, vm.IPAddress)
			if err == nil && k8sNode != nil {
				// Check if this K8s node is not already claimed by another VPSieNode
				if !d.isNodeClaimedByOther(k8sNode, vn) {
					logger.Info("Discovered VPS by IP matching K8s node",
						zap.Int("vpsID", vm.ID),
						zap.String("ip", vm.IPAddress),
						zap.String("k8sNode", k8sNode.Name),
					)
					return vm, false, nil
				}
			}
		}
	}

	logger.Info("VPS discovery: no matching VPS found",
		zap.Int("candidateCount", len(candidates)),
	)
	return nil, false, nil
}

// matchesHostnamePattern checks if the VPS hostname matches the VPSieNode name pattern
// VPSie typically appends a suffix to the node name when creating nodes
// e.g., VPSieNode "my-nodegroup-abc123" might create VPS "my-nodegroup-abc123-k8s-worker"
func matchesHostnamePattern(vpsieNodeName, vpsHostname string) bool {
	// Normalize both names for comparison
	vpsieNodeName = strings.ToLower(vpsieNodeName)
	vpsHostname = strings.ToLower(vpsHostname)

	// VPSie may use the VPSieNode name as a prefix for the hostname
	if len(vpsHostname) >= len(vpsieNodeName) {
		return strings.HasPrefix(vpsHostname, vpsieNodeName)
	}
	return false
}

// findK8sNodeByIP finds a Kubernetes node by its IP address
func (d *Discoverer) findK8sNodeByIP(ctx context.Context, ip string) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := d.k8sClient.List(ctx, nodeList); err != nil {
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

	return nil, nil
}

// isNodeClaimedByOther checks if a K8s node is already associated with another VPSieNode
// by checking the autoscaler.vpsie.com/vpsienode label
func (d *Discoverer) isNodeClaimedByOther(node *corev1.Node, currentVN *v1alpha1.VPSieNode) bool {
	// Check if node has VPSieNode label pointing to a different VPSieNode
	if vnLabel, ok := node.Labels["autoscaler.vpsie.com/vpsienode"]; ok {
		if vnLabel != "" && vnLabel != currentVN.Name {
			return true
		}
	}
	return false
}

// findUnclaimedK8sNode finds K8s nodes that recently joined and aren't claimed by any VPSieNode
// Returns the newest unclaimed worker node
func (d *Discoverer) findUnclaimedK8sNode(ctx context.Context, vn *v1alpha1.VPSieNode) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := d.k8sClient.List(ctx, nodeList); err != nil {
		return nil, err
	}

	// Find nodes that:
	// 1. Are not control-plane nodes
	// 2. Don't have the autoscaler.vpsie.com/vpsienode label (unclaimed)
	// 3. Were created after the VPSieNode was created (approximately)
	var unclaimed []*corev1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// Skip control-plane nodes
		if _, isControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; isControlPlane {
			continue
		}
		if _, isMaster := node.Labels["node-role.kubernetes.io/master"]; isMaster {
			continue
		}

		// Skip nodes already claimed by the autoscaler
		if _, claimed := node.Labels["autoscaler.vpsie.com/vpsienode"]; claimed {
			continue
		}

		// Only consider nodes created recently (within 30 minutes)
		// VPSie K8s-managed nodes may have been created by previous VPSieNode requests
		// that were deleted, so we can't rely on strict timing relative to current VPSieNode
		nodeAge := time.Since(node.CreationTimestamp.Time)
		if nodeAge > 30*time.Minute {
			continue // Node is too old to be recently provisioned
		}

		unclaimed = append(unclaimed, node)
	}

	if len(unclaimed) == 0 {
		return nil, nil
	}

	// Return the newest unclaimed node (most likely to be ours)
	var newest *corev1.Node
	for _, node := range unclaimed {
		if newest == nil || node.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = node
		}
	}

	d.logger.Info("Found unclaimed K8s node",
		zap.String("nodeName", newest.Name),
		zap.Time("nodeCreated", newest.CreationTimestamp.Time),
	)

	return newest, nil
}

// getNodeIP returns the internal IP of a K8s node
func (d *Discoverer) getNodeIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	// Fallback to external IP if no internal IP
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			return addr.Address
		}
	}
	return ""
}
