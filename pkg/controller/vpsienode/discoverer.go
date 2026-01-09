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

	// Filter VMs to running status (candidates for discovery)
	var candidates []vpsieclient.VPS
	for _, vm := range allVMs {
		// Only consider running VMs
		if vm.Status == "running" {
			candidates = append(candidates, vm)
		}
	}

	if len(candidates) == 0 {
		logger.Debug("No candidate VMs found for discovery")
		return nil, false, nil
	}

	// Sort by creation time (newest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})

	// Find VPS that matches our VPSieNode
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

	logger.Debug("No matching VPS found in discovery",
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
