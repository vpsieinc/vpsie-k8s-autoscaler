package vpsienode

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// DiscovererConfig holds configuration for the Discoverer
type DiscovererConfig struct {
	// DiscoveryTimeout is the maximum time to wait for VPS discovery
	// Default: 15 minutes
	DiscoveryTimeout time.Duration

	// MaxNodeAge is the maximum age of unclaimed nodes to consider
	// Default: 30 minutes
	MaxNodeAge time.Duration
}

// DefaultDiscovererConfig returns the default configuration
func DefaultDiscovererConfig() *DiscovererConfig {
	return &DiscovererConfig{
		DiscoveryTimeout: 15 * time.Minute,
		MaxNodeAge:       30 * time.Minute,
	}
}

// DiscoveryTimeout is the maximum time to wait for VPS discovery
// Deprecated: Use DiscovererConfig.DiscoveryTimeout instead
const DiscoveryTimeout = 15 * time.Minute

// Discoverer handles VPS and K8s node discovery for async provisioning
type Discoverer struct {
	vpsieClient VPSieClientInterface
	k8sClient   client.Client
	logger      *zap.Logger
	config      *DiscovererConfig
}

// NewDiscoverer creates a new Discoverer with default configuration
func NewDiscoverer(vpsieClient VPSieClientInterface, k8sClient client.Client, logger *zap.Logger) *Discoverer {
	return NewDiscovererWithConfig(vpsieClient, k8sClient, logger, nil)
}

// NewDiscovererWithConfig creates a new Discoverer with custom configuration
func NewDiscovererWithConfig(vpsieClient VPSieClientInterface, k8sClient client.Client, logger *zap.Logger, config *DiscovererConfig) *Discoverer {
	if config == nil {
		config = DefaultDiscovererConfig()
	}
	return &Discoverer{
		vpsieClient: vpsieClient,
		k8sClient:   k8sClient,
		logger:      logger.Named("discoverer"),
		config:      config,
	}
}

// DiscoverVPSID attempts to discover the VPS ID for a VPSieNode
// that was created via async provisioning (creation-requested=true, VPSieInstanceID=0)
//
// Discovery Strategy Priority:
//
// Strategy 1 - Unclaimed K8s Node (Primary):
//
//	VPSie K8s managed clusters don't expose individual node VPS IDs through their API.
//	This strategy finds recently joined K8s nodes that aren't claimed by any VPSieNode
//	and attempts to claim them via optimistic locking. This is tried first because it's
//	the most common case for VPSie-managed Kubernetes clusters.
//
// Strategy 2 - Hostname Pattern Matching:
//
//	When VPS IDs are available (non-K8s-managed VMs), we match by hostname pattern.
//	VPSie typically creates VMs with hostnames that include the VPSieNode name as a prefix.
//	This is reliable when the VPSie API returns VM information.
//
// Strategy 3 - IP Address Correlation:
//
//	As a fallback, we correlate VM IP addresses with K8s node IPs. This handles cases
//	where hostname patterns don't match due to custom naming conventions.
//
// Returns:
//   - *vpsieclient.VPS: Discovered VPS (nil if not found)
//   - bool: Whether discovery timed out
//   - error: API errors or context cancellation
func (d *Discoverer) DiscoverVPSID(ctx context.Context, vn *v1alpha1.VPSieNode) (*vpsieclient.VPS, bool, error) {
	startTime := time.Now()
	logger := d.logger.With(
		zap.String("vpsienode", vn.Name),
		zap.String("cluster", vn.Spec.ResourceIdentifier),
		zap.Int("groupID", vn.Spec.VPSieGroupID),
	)

	// Check timeout using configurable value
	timeout := d.config.DiscoveryTimeout
	if vn.Status.CreatedAt != nil {
		elapsed := time.Since(vn.Status.CreatedAt.Time)
		if elapsed > timeout {
			logger.Warn("VPS discovery timeout exceeded",
				zap.Duration("elapsed", elapsed),
				zap.Duration("timeout", timeout),
			)
			metrics.VPSieNodeDiscoveryFailuresTotal.WithLabelValues("timeout").Inc()
			return nil, true, nil
		}
	}

	// Defer metrics recording
	defer func() {
		metrics.VPSieNodeDiscoveryDuration.Observe(time.Since(startTime).Seconds())
	}()

	// List all VMs from VPSie API
	allVMs, err := d.vpsieClient.ListVMs(ctx)
	if err != nil {
		logger.Error("Failed to list VMs for discovery", zap.Error(err))
		metrics.VPSieNodeDiscoveryFailuresTotal.WithLabelValues("api_error").Inc()
		return nil, false, fmt.Errorf("failed to list VMs: %w", err)
	}

	// Count VMs by status for debugging
	statusCounts := make(map[string]int)
	for _, vm := range allVMs {
		statusCounts[vm.Status]++
	}
	logger.Debug("VPS discovery: listed VMs from API",
		zap.Int("totalVMs", len(allVMs)),
		zap.Any("statusCounts", statusCounts),
	)

	// Cache K8s node list once for the entire discovery operation
	// This reduces API calls from O(n) to O(1) where n = number of VM candidates
	nodeList := &corev1.NodeList{}
	if err := d.k8sClient.List(ctx, nodeList); err != nil {
		logger.Error("Failed to list K8s nodes for discovery", zap.Error(err))
		metrics.VPSieNodeDiscoveryFailuresTotal.WithLabelValues("api_error").Inc()
		return nil, false, fmt.Errorf("failed to list K8s nodes: %w", err)
	}

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
		logger.Debug("VPS discovery: no candidate VMs found for discovery",
			zap.Int("totalVMs", len(allVMs)),
		)
		return nil, false, nil
	}

	logger.Debug("VPS discovery: found candidate VMs",
		zap.Int("candidates", len(candidates)),
	)

	// Sort by creation time (newest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})

	// Find VPS that matches our VPSieNode
	// Strategy 1 (primary for VPSie K8s service): Find unclaimed K8s nodes
	// VPSie K8s managed clusters don't expose individual node VPS IDs through the API
	// So we match K8s nodes directly without requiring a VPS ID
	unclaimedNode := d.findUnclaimedK8sNodeFromList(nodeList, vn)
	if unclaimedNode != nil {
		nodeIP := d.getNodeIP(unclaimedNode)
		if nodeIP != "" {
			// Attempt to claim the node before returning it to prevent race conditions.
			// If claim fails due to conflict, another VPSieNode got it first.
			if claimErr := d.claimNode(ctx, unclaimedNode, vn); claimErr != nil {
				if apierrors.IsConflict(claimErr) {
					// Another VPSieNode claimed this node, continue searching
					logger.Debug("Node was claimed by another VPSieNode, continuing search",
						zap.String("nodeName", unclaimedNode.Name),
					)
				} else {
					logger.Warn("Failed to claim node", zap.Error(claimErr))
				}
				// Don't return this node, fall through to other strategies
			} else {
				// Successfully claimed the node
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
						metrics.VPSieNodeDiscoveryStrategyUsed.WithLabelValues("unclaimed_k8s_node_ip").Inc()
						return vm, false, nil
					}
				}
				// VPSie K8s-managed nodes don't appear in ListVMs
				// Create a synthetic VPS entry with just the K8s node info
				logger.Info("Discovered K8s node (VPSie K8s-managed, no VPS ID available)",
					zap.String("k8sNode", unclaimedNode.Name),
					zap.String("nodeIP", nodeIP),
				)
				metrics.VPSieNodeDiscoveryStrategyUsed.WithLabelValues("unclaimed_k8s_node").Inc()
				return &vpsieclient.VPS{
					ID:        0, // VPS ID not available for K8s-managed nodes
					Hostname:  unclaimedNode.Name,
					IPAddress: nodeIP,
					Status:    "running", // K8s node exists so it's running
				}, false, nil
			}
		}
	}

	for i := range candidates {
		// Check for context cancellation to support graceful shutdown
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}

		vm := &candidates[i]

		// Strategy 2: Match by hostname pattern (VPSieNode name should be prefix)
		if matchesHostnamePattern(vn.Name, vm.Hostname) {
			logger.Info("Discovered VPS by hostname pattern",
				zap.Int("vpsID", vm.ID),
				zap.String("hostname", vm.Hostname),
			)
			metrics.VPSieNodeDiscoveryStrategyUsed.WithLabelValues("hostname_pattern").Inc()
			return vm, false, nil
		}

		// Strategy 3: Match by IP if we can correlate with K8s node (using cached list)
		if vm.IPAddress != "" {
			k8sNode := d.findK8sNodeByIPFromList(nodeList, vm.IPAddress)
			if k8sNode != nil {
				// Check if this K8s node is not already claimed by another VPSieNode
				if !d.isNodeClaimedByOther(k8sNode, vn) {
					logger.Info("Discovered VPS by IP matching K8s node",
						zap.Int("vpsID", vm.ID),
						zap.String("ip", vm.IPAddress),
						zap.String("k8sNode", k8sNode.Name),
					)
					metrics.VPSieNodeDiscoveryStrategyUsed.WithLabelValues("ip_matching").Inc()
					return vm, false, nil
				}
			}
		}
	}

	logger.Debug("VPS discovery: no matching VPS found",
		zap.Int("candidateCount", len(candidates)),
	)
	metrics.VPSieNodeDiscoveryFailuresTotal.WithLabelValues("not_found").Inc()
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
// Deprecated: Use findK8sNodeByIPFromList with cached node list for better performance
func (d *Discoverer) findK8sNodeByIP(ctx context.Context, ip string) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := d.k8sClient.List(ctx, nodeList); err != nil {
		return nil, err
	}
	return d.findK8sNodeByIPFromList(nodeList, ip), nil
}

// findK8sNodeByIPFromList finds a Kubernetes node by its IP address using a cached node list.
// This avoids repeated API calls when searching multiple IPs.
func (d *Discoverer) findK8sNodeByIPFromList(nodeList *corev1.NodeList, ip string) *corev1.Node {
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				if addr.Address == ip {
					return node
				}
			}
		}
	}
	return nil
}

// isNodeClaimedByOther checks if a K8s node is already associated with another VPSieNode
// by checking the autoscaler.vpsie.com/vpsienode label
func (d *Discoverer) isNodeClaimedByOther(node *corev1.Node, currentVN *v1alpha1.VPSieNode) bool {
	// Check if node has VPSieNode label pointing to a different VPSieNode
	if vnLabel, ok := node.Labels[v1alpha1.VPSieNodeLabelKey]; ok {
		if vnLabel != "" && vnLabel != currentVN.Name {
			return true
		}
	}
	return false
}

// findUnclaimedK8sNodeFromList finds K8s nodes that recently joined and aren't claimed by any VPSieNode.
// Returns the newest unclaimed worker node using a cached node list.
func (d *Discoverer) findUnclaimedK8sNodeFromList(nodeList *corev1.NodeList, vn *v1alpha1.VPSieNode) *corev1.Node {
	// Find nodes that:
	// 1. Are not control-plane nodes
	// 2. Don't have the autoscaler.vpsie.com/vpsienode label (unclaimed)
	// 3. Were created recently (configurable via MaxNodeAge)
	var unclaimed []*corev1.Node
	maxAge := d.config.MaxNodeAge
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
		if _, claimed := node.Labels[v1alpha1.VPSieNodeLabelKey]; claimed {
			continue
		}

		// Only consider nodes created recently (configurable via MaxNodeAge)
		// VPSie K8s-managed nodes may have been created by previous VPSieNode requests
		// that were deleted, so we can't rely on strict timing relative to current VPSieNode
		nodeAge := time.Since(node.CreationTimestamp.Time)
		if nodeAge > maxAge {
			continue // Node is too old to be recently provisioned
		}

		unclaimed = append(unclaimed, node)
	}

	if len(unclaimed) == 0 {
		return nil
	}

	// Return the newest unclaimed node (most likely to be ours)
	var newest *corev1.Node
	for _, node := range unclaimed {
		if newest == nil || node.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = node
		}
	}

	d.logger.Debug("Found unclaimed K8s node",
		zap.String("nodeName", newest.Name),
		zap.Time("nodeCreated", newest.CreationTimestamp.Time),
	)

	return newest
}

// claimNode attempts to claim a K8s node for a VPSieNode by adding the VPSieNode label.
// This uses optimistic locking to handle race conditions - if another VPSieNode
// claims the node first, this will return a conflict error.
func (d *Discoverer) claimNode(ctx context.Context, node *corev1.Node, vn *v1alpha1.VPSieNode) error {
	// Create a patch to add the VPSieNode label
	original := node.DeepCopy()

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels[v1alpha1.VPSieNodeLabelKey] = vn.Name
	node.Labels[v1alpha1.NodeGroupLabelKey] = vn.Spec.NodeGroupName

	// Use patch with optimistic locking to prevent race conditions
	patch := client.MergeFrom(original)
	if err := d.k8sClient.Patch(ctx, node, patch); err != nil {
		if apierrors.IsConflict(err) {
			d.logger.Info("Node claim conflict - another VPSieNode may have claimed this node",
				zap.String("nodeName", node.Name),
				zap.String("vpsienode", vn.Name),
			)
			metrics.VPSieNodeDiscoveryFailuresTotal.WithLabelValues("claim_conflict").Inc()
		}
		return err
	}

	d.logger.Info("Successfully claimed K8s node",
		zap.String("nodeName", node.Name),
		zap.String("vpsienode", vn.Name),
		zap.String("nodegroup", vn.Spec.NodeGroupName),
	)

	return nil
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
