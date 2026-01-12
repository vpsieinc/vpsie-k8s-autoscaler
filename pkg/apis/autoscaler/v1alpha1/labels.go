package v1alpha1

// Label and annotation keys used for NodeGroup and VPSieNode management.
// These constants are defined here in the API types package to avoid circular
// dependencies between controller and event packages.
const (
	// ManagedLabelKey is the label key used to mark NodeGroups as managed by the autoscaler.
	// Only NodeGroups with this label set to ManagedLabelValue will be processed.
	ManagedLabelKey = "autoscaler.vpsie.com/managed"

	// ManagedLabelValue is the expected value for the managed label.
	// NodeGroups must have ManagedLabelKey set to this value to be managed.
	ManagedLabelValue = "true"

	// NodeGroupLabelKey is the label key used to identify which NodeGroup a resource belongs to.
	// This is applied to VPSieNodes and K8s nodes to associate them with their parent NodeGroup.
	NodeGroupLabelKey = "autoscaler.vpsie.com/nodegroup"

	// VPSieNodeLabelKey is the label key used to associate K8s nodes with their VPSieNode CR.
	VPSieNodeLabelKey = "autoscaler.vpsie.com/vpsienode"

	// DatacenterLabelKey is the label key for the datacenter ID.
	DatacenterLabelKey = "autoscaler.vpsie.com/datacenter"

	// OfferingLabelKey is the label key for the VPSie offering/instance type ID.
	OfferingLabelKey = "autoscaler.vpsie.com/offering"

	// VPSIDAnnotationKey is the annotation key for the VPSie VPS ID.
	VPSIDAnnotationKey = "autoscaler.vpsie.com/vps-id"

	// CreationRequestedAnnotation is the annotation key to trigger async VPS discovery.
	CreationRequestedAnnotation = "autoscaler.vpsie.com/creation-requested"

	// CreationReasonAnnotationKey is the annotation key for tracking why a node was created.
	// Values: "metrics" (scale-up due to resource metrics), "manual" (manually created),
	// "rebalance" (created during rebalancing), "initial" (initial nodegroup setup)
	CreationReasonAnnotationKey = "autoscaler.vpsie.com/creation-reason"

	// CreationReasonMetrics indicates the node was created due to metrics-based scale-up
	CreationReasonMetrics = "metrics"

	// CreationReasonManual indicates the node was manually created or pre-existing
	CreationReasonManual = "manual"

	// CreationReasonRebalance indicates the node was created during rebalancing
	CreationReasonRebalance = "rebalance"

	// CreationReasonInitial indicates the node was created during initial nodegroup setup
	CreationReasonInitial = "initial"
)

// IsManagedNodeGroup checks if the NodeGroup has the managed label set to "true".
// Returns false if the NodeGroup is nil, has nil labels, missing managed label, or
// the label is set to any value other than "true".
func IsManagedNodeGroup(ng *NodeGroup) bool {
	if ng == nil || ng.Labels == nil {
		return false
	}
	return ng.Labels[ManagedLabelKey] == ManagedLabelValue
}

// SetNodeGroupManaged adds the managed label to a NodeGroup.
// This function is idempotent - calling it multiple times has the same effect as calling it once.
// If the NodeGroup has nil labels, a new labels map is created.
// Does nothing if ng is nil.
func SetNodeGroupManaged(ng *NodeGroup) {
	if ng == nil {
		return
	}
	if ng.Labels == nil {
		ng.Labels = make(map[string]string)
	}
	ng.Labels[ManagedLabelKey] = ManagedLabelValue
}
