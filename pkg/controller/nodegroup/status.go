package nodegroup

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// Re-export label constants from v1alpha1 for backward compatibility and convenience.
// These are the canonical definitions, re-exported for use within the controller package.
const (
	// ManagedLabelKey is the label key used to mark NodeGroups as managed by the autoscaler.
	ManagedLabelKey = v1alpha1.ManagedLabelKey

	// ManagedLabelValue is the expected value for the managed label.
	ManagedLabelValue = v1alpha1.ManagedLabelValue

	// NodeGroupNameLabelKey is the label key used to identify which NodeGroup a resource belongs to.
	NodeGroupNameLabelKey = v1alpha1.NodeGroupLabelKey
)

// IsManagedNodeGroup checks if the NodeGroup has the managed label set to "true".
// Delegates to v1alpha1.IsManagedNodeGroup for consistent behavior.
func IsManagedNodeGroup(ng *v1alpha1.NodeGroup) bool {
	return v1alpha1.IsManagedNodeGroup(ng)
}

// SetNodeGroupManaged adds the managed label to a NodeGroup.
// Delegates to v1alpha1.SetNodeGroupManaged for consistent behavior.
func SetNodeGroupManaged(ng *v1alpha1.NodeGroup) {
	v1alpha1.SetNodeGroupManaged(ng)
}

// ManagedLabelSelector returns a client.MatchingLabels selector that can be used
// to filter NodeGroups to only those managed by the autoscaler.
// This is useful for List operations that should only return managed NodeGroups.
func ManagedLabelSelector() client.MatchingLabels {
	return client.MatchingLabels{
		ManagedLabelKey: ManagedLabelValue,
	}
}

// UpdateNodeGroupStatus updates the NodeGroup status based on the current state of VPSieNodes
func UpdateNodeGroupStatus(ctx context.Context, c client.Client, ng *v1alpha1.NodeGroup, vpsieNodes []v1alpha1.VPSieNode) error {
	// Calculate node counts
	currentNodes := int32(len(vpsieNodes))
	readyNodes := int32(0)

	// Build nodes list for status
	var nodes []v1alpha1.NodeInfo
	for _, vn := range vpsieNodes {
		// Count ready nodes
		if vn.Status.Phase == v1alpha1.VPSieNodePhaseReady {
			readyNodes++
		}

		// Build node info
		nodeInfo := v1alpha1.NodeInfo{
			NodeName:     vn.Status.NodeName,
			VPSID:        vn.Spec.VPSieInstanceID,
			InstanceType: vn.Spec.InstanceType,
			Status:       string(vn.Status.Phase),
			IPAddress:    vn.Spec.IPAddress,
		}

		if vn.Status.CreatedAt != nil {
			nodeInfo.CreatedAt = vn.Status.CreatedAt
		}

		if vn.Status.ReadyAt != nil {
			nodeInfo.ReadyAt = vn.Status.ReadyAt
		}

		nodes = append(nodes, nodeInfo)
	}

	// Update status fields
	ng.Status.CurrentNodes = currentNodes
	ng.Status.ReadyNodes = readyNodes
	ng.Status.Nodes = nodes
	ng.Status.ObservedGeneration = ng.Generation

	// Set desired nodes if not set
	if ng.Status.DesiredNodes == 0 {
		ng.Status.DesiredNodes = ng.Spec.MinNodes
	}

	return nil
}

// SetDesiredNodes sets the desired node count and updates the last scale time
func SetDesiredNodes(ng *v1alpha1.NodeGroup, desired int32) {
	if ng.Status.DesiredNodes != desired {
		now := metav1.Now()
		ng.Status.DesiredNodes = desired
		ng.Status.LastScaleTime = &now

		// Track scale up vs scale down
		if desired > ng.Status.CurrentNodes {
			ng.Status.LastScaleUpTime = &now
		} else if desired < ng.Status.CurrentNodes {
			ng.Status.LastScaleDownTime = &now
		}
	}
}

// CalculateDesiredNodes calculates the desired number of nodes based on spec constraints
func CalculateDesiredNodes(ng *v1alpha1.NodeGroup) int32 {
	desired := ng.Status.DesiredNodes

	// If not set, start with minimum
	if desired == 0 {
		desired = ng.Spec.MinNodes
	}

	// Ensure desired is within min/max bounds
	if desired < ng.Spec.MinNodes {
		desired = ng.Spec.MinNodes
	}

	if desired > ng.Spec.MaxNodes {
		desired = ng.Spec.MaxNodes
	}

	return desired
}

// NeedsScaleUp returns true if the NodeGroup needs to scale up
func NeedsScaleUp(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.CurrentNodes < ng.Status.DesiredNodes &&
		ng.Status.CurrentNodes < ng.Spec.MaxNodes
}

// NeedsScaleDown returns true if the NodeGroup needs to scale down
func NeedsScaleDown(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.CurrentNodes > ng.Status.DesiredNodes &&
		ng.Status.CurrentNodes > ng.Spec.MinNodes
}

// CanScaleUp returns true if the NodeGroup can scale up
func CanScaleUp(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.CurrentNodes < ng.Spec.MaxNodes
}

// CanScaleDown returns true if the NodeGroup can scale down
func CanScaleDown(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.CurrentNodes > ng.Spec.MinNodes
}

// CalculateNodesToAdd returns the number of nodes to add during scale-up
func CalculateNodesToAdd(ng *v1alpha1.NodeGroup) int32 {
	needed := ng.Status.DesiredNodes - ng.Status.CurrentNodes
	canAdd := ng.Spec.MaxNodes - ng.Status.CurrentNodes

	if needed > canAdd {
		return canAdd
	}

	return needed
}

// CalculateNodesToRemove returns the number of nodes to remove during scale-down
func CalculateNodesToRemove(ng *v1alpha1.NodeGroup) int32 {
	excess := ng.Status.CurrentNodes - ng.Status.DesiredNodes
	canRemove := ng.Status.CurrentNodes - ng.Spec.MinNodes

	if excess > canRemove {
		return canRemove
	}

	return excess
}

// ValidateNodeGroupSpec validates the NodeGroup spec and returns an error if invalid
func ValidateNodeGroupSpec(ng *v1alpha1.NodeGroup) error {
	if ng.Spec.MinNodes < 0 {
		return fmt.Errorf("minNodes must be >= 0, got %d", ng.Spec.MinNodes)
	}

	if ng.Spec.MaxNodes < 1 {
		return fmt.Errorf("maxNodes must be >= 1, got %d", ng.Spec.MaxNodes)
	}

	if ng.Spec.MinNodes > ng.Spec.MaxNodes {
		return fmt.Errorf("minNodes (%d) must be <= maxNodes (%d)", ng.Spec.MinNodes, ng.Spec.MaxNodes)
	}

	if ng.Spec.DatacenterID == "" {
		return fmt.Errorf("datacenterID is required")
	}

	if len(ng.Spec.OfferingIDs) == 0 {
		return fmt.Errorf("at least one offeringID is required")
	}

	// OSImageID is optional - VPSie API will automatically select an appropriate OS image

	if ng.Spec.KubernetesVersion == "" {
		return fmt.Errorf("kubernetesVersion is required")
	}

	// Validate version format (also validated by kubebuilder pattern, but check here for clarity)
	if _, err := ParseVersion(ng.Spec.KubernetesVersion); err != nil {
		return fmt.Errorf("invalid kubernetesVersion format: %w", err)
	}

	return nil
}

// IsScaling returns true if the NodeGroup is currently scaling
func IsScaling(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.CurrentNodes != ng.Status.DesiredNodes
}

// IsReady returns true if all desired nodes are ready
func IsReady(ng *v1alpha1.NodeGroup) bool {
	return ng.Status.ReadyNodes == ng.Status.DesiredNodes &&
		ng.Status.CurrentNodes == ng.Status.DesiredNodes
}

// GetNodeGroupNameLabel returns the label key for NodeGroup name.
// Deprecated: Use NodeGroupNameLabelKey constant directly instead.
func GetNodeGroupNameLabel() string {
	return NodeGroupNameLabelKey
}

// GetNodeGroupLabels returns labels to apply to VPSieNodes.
// This includes both the NodeGroup name label and the managed label,
// ensuring that all VPSieNodes created by a managed NodeGroup are properly labeled.
func GetNodeGroupLabels(ng *v1alpha1.NodeGroup) map[string]string {
	labels := make(map[string]string)
	labels[NodeGroupNameLabelKey] = ng.Name
	labels[ManagedLabelKey] = ManagedLabelValue
	return labels
}

// ShouldReconcile returns true if the NodeGroup should be reconciled
func ShouldReconcile(ng *v1alpha1.NodeGroup) bool {
	// Always reconcile if there's a difference between current and desired
	if ng.Status.CurrentNodes != ng.Status.DesiredNodes {
		return true
	}

	// Reconcile if not all nodes are ready
	if ng.Status.ReadyNodes < ng.Status.CurrentNodes {
		return true
	}

	// Reconcile if status is outdated
	if ng.Status.ObservedGeneration != ng.Generation {
		return true
	}

	return false
}

// HasNodesInTransition checks if any VPSieNodes are in a transitional state
// (not yet Ready or Failed). Returns true if there are nodes being provisioned
// or joining the cluster.
func HasNodesInTransition(vpsieNodes []v1alpha1.VPSieNode) bool {
	for _, vn := range vpsieNodes {
		switch vn.Status.Phase {
		case v1alpha1.VPSieNodePhasePending,
			v1alpha1.VPSieNodePhaseProvisioning,
			v1alpha1.VPSieNodePhaseProvisioned,
			v1alpha1.VPSieNodePhaseJoining:
			// Node is in a transitional state - not yet Ready
			return true
		}
	}
	return false
}

// CountNodesInTransition returns the number of VPSieNodes in transitional states
func CountNodesInTransition(vpsieNodes []v1alpha1.VPSieNode) int {
	count := 0
	for _, vn := range vpsieNodes {
		switch vn.Status.Phase {
		case v1alpha1.VPSieNodePhasePending,
			v1alpha1.VPSieNodePhaseProvisioning,
			v1alpha1.VPSieNodePhaseProvisioned,
			v1alpha1.VPSieNodePhaseJoining:
			count++
		}
	}
	return count
}
