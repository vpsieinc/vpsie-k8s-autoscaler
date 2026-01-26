package vpsienode

import (
	"context"
	"fmt"
	"strconv"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Provisioner handles VPS provisioning operations
type Provisioner struct {
	vpsieClient VPSieClientInterface
	// SSH key IDs to inject into VPS
	sshKeyIDs []string
	// discoverer handles VPS ID discovery for async provisioning
	discoverer *Discoverer
}

// NewProvisioner creates a new Provisioner
func NewProvisioner(vpsieClient VPSieClientInterface, sshKeyIDs []string) *Provisioner {
	return &Provisioner{
		vpsieClient: vpsieClient,
		sshKeyIDs:   sshKeyIDs,
	}
}

// SetDiscoverer sets the discoverer for async VPS ID discovery
func (p *Provisioner) SetDiscoverer(d *Discoverer) {
	p.discoverer = d
}

// Provision creates a VPS and transitions through provisioning phases
func (p *Provisioner) Provision(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	// Check if VPS ID is already set (VPS was created previously)
	if vn.Spec.VPSieInstanceID != 0 {
		// VPS already exists, check its status
		return p.checkVPSStatus(ctx, vn, logger)
	}

	// Create a new VPS
	return p.createVPS(ctx, vn, logger)
}

// AnnotationCreationRequested is set when node creation has been requested but ID is not yet known
const AnnotationCreationRequested = "autoscaler.vpsie.com/creation-requested"

// createVPS creates a new VPS instance via VPSie Kubernetes API
func (p *Provisioner) createVPS(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	// Check if creation was already requested (to avoid duplicate API calls)
	if vn.Annotations != nil && vn.Annotations[AnnotationCreationRequested] == "true" {
		logger.Info("Node creation already requested, attempting discovery",
			zap.String("vpsienode", vn.Name),
		)

		// Attempt to discover the VPS ID
		if p.discoverer != nil {
			vps, timedOut, err := p.discoverer.DiscoverVPSID(ctx, vn)
			if err != nil {
				logger.Warn("VPS discovery failed", zap.Error(err))
				// Continue - will retry on next reconcile
			} else if timedOut {
				// Discovery timeout - mark as failed
				logger.Error("VPS discovery timeout exceeded",
					zap.String("vpsienode", vn.Name),
				)
				return ctrl.Result{}, fmt.Errorf("VPS discovery timeout exceeded")
			} else if vps != nil {
				// Discovery successful - update VPSieNode
				logger.Info("VPS discovered successfully",
					zap.Int("vpsID", vps.ID),
					zap.String("hostname", vps.Hostname),
					zap.String("ip", vps.IPAddress),
				)

				// Update VPSieNode with discovered VPS information
				vn.Spec.VPSieInstanceID = vps.ID
				vn.Spec.IPAddress = vps.IPAddress
				vn.Spec.IPv6Address = vps.IPv6Address
				if vn.Spec.NodeName == "" && vps.Hostname != "" {
					vn.Spec.NodeName = vps.Hostname
				}
				vn.Status.Hostname = vps.Hostname
				vn.Status.VPSieStatus = vps.Status
				vn.Status.Resources = v1alpha1.NodeResources{
					CPU:      vps.CPU,
					MemoryMB: vps.RAM,
					DiskGB:   vps.Disk,
				}

				// For K8s-managed nodes (VPS ID=0 but IP exists), skip VPS status check
				// The K8s node exists and is running, so go directly to Provisioned
				if vps.ID == 0 && vps.IPAddress != "" {
					logger.Info("K8s-managed node discovered, transitioning to Provisioned",
						zap.String("vpsienode", vn.Name),
						zap.String("hostname", vps.Hostname),
						zap.String("ip", vps.IPAddress),
					)
					// Set NodeName from discovered hostname for the Joining phase to find it
					if vn.Status.NodeName == "" && vps.Hostname != "" {
						vn.Status.NodeName = vps.Hostname
					}
					SetPhase(vn, v1alpha1.VPSieNodePhaseProvisioned, ReasonProvisioned, "K8s node is running")
					SetVPSReadyCondition(vn, true, ReasonProvisioned, "K8s node is running")
					now := metav1.Now()
					if vn.Status.CreatedAt == nil {
						vn.Status.CreatedAt = &now
					}
					vn.Status.ProvisionedAt = &now
					return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
				}

				// Continue with normal provisioning flow for VMs with VPS ID
				return p.checkVPSStatus(ctx, vn, logger)
			}
		}

		// VPS not discovered yet, keep waiting
		return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
	}

	logger.Info("Creating new Kubernetes node via VPSie API",
		zap.String("vpsienode", vn.Name),
		zap.String("instanceType", vn.Spec.InstanceType),
		zap.String("datacenter", vn.Spec.DatacenterID),
		zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
		zap.String("project", vn.Spec.Project),
		zap.Int("groupID", vn.Spec.VPSieGroupID),
	)

	// Validate that we have the numeric group ID
	if vn.Spec.VPSieGroupID == 0 {
		logger.Error("VPSieGroupID is required to add node to cluster",
			zap.String("vpsienode", vn.Name),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("VPSieGroupID is required")
	}

	// Call VPSie Kubernetes API to add slave node to the specific group
	// Uses the endpoint: POST /k8s/cluster/byId/{clusterIdentifier}/add/slave/group/{groupID}
	vps, err := p.vpsieClient.AddK8sSlaveToGroup(ctx, vn.Spec.ResourceIdentifier, vn.Spec.VPSieGroupID)
	if err != nil {
		logger.Error("Failed to create K8s node via VPSie API",
			zap.String("vpsienode", vn.Name),
			zap.Int("groupID", vn.Spec.VPSieGroupID),
			zap.Error(err),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("failed to create VPS: %w", err)
	}

	logger.Info("K8s node creation request accepted",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vps.ID),
		zap.String("hostname", vps.Hostname),
		zap.String("status", vps.Status),
	)

	// If API returned ID=0, it means the request was accepted but node creation is async
	// Set an annotation to track that we've requested creation
	if vps.ID == 0 {
		logger.Info("Node creation requested but ID not yet assigned (async provisioning)",
			zap.String("vpsienode", vn.Name),
			zap.String("vpsieNodeIdentifier", vps.Identifier),
		)
		if vn.Annotations == nil {
			vn.Annotations = make(map[string]string)
		}
		vn.Annotations[AnnotationCreationRequested] = "true"
		// Store the VPSie node identifier if returned (for K8s API deletion)
		if vps.Identifier != "" {
			vn.Spec.VPSieNodeIdentifier = vps.Identifier
		}
		vn.Status.VPSieStatus = "provisioning"
		SetVPSReadyCondition(vn, false, ReasonProvisioning, "Node creation requested, waiting for VPSie to provision")
		now := metav1.Now()
		vn.Status.CreatedAt = &now
		return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
	}

	// Update VPSieNode spec with VPS information
	vn.Spec.VPSieInstanceID = vps.ID
	vn.Spec.IPAddress = vps.IPAddress
	vn.Spec.IPv6Address = vps.IPv6Address
	// Store the VPSie node identifier if returned (for K8s API deletion)
	if vps.Identifier != "" {
		vn.Spec.VPSieNodeIdentifier = vps.Identifier
	}
	if vn.Spec.NodeName == "" {
		if vps.Hostname != "" {
			vn.Spec.NodeName = vps.Hostname
		} else {
			vn.Spec.NodeName = p.generateHostname(vn)
		}
	}

	// Update status
	vn.Status.Hostname = vps.Hostname
	vn.Status.VPSieStatus = vps.Status
	vn.Status.Resources = v1alpha1.NodeResources{
		CPU:      vps.CPU,
		MemoryMB: vps.RAM,
		DiskGB:   vps.Disk,
	}
	now := metav1.Now()
	vn.Status.CreatedAt = &now

	// Set VPS ready condition to false initially
	SetVPSReadyCondition(vn, false, ReasonProvisioning, "VPS is being provisioned")

	// Requeue to check VPS status
	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// checkVPSStatus checks the VPS status and transitions to Provisioned when ready
func (p *Provisioner) checkVPSStatus(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Debug("Checking VPS status",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
	)

	// Get VPS from VPSie API
	vps, err := p.vpsieClient.GetVM(ctx, vn.Spec.VPSieInstanceID)
	if err != nil {
		logger.Error("Failed to get VPS status",
			zap.String("vpsienode", vn.Name),
			zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			zap.Error(err),
		)

		// Check if VPS was not found (deleted externally)
		if vpsieclient.IsNotFound(err) {
			logger.Warn("VPS not found, may have been deleted externally",
				zap.String("vpsienode", vn.Name),
				zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			)
			SetPhase(vn, v1alpha1.VPSieNodePhaseFailed, ReasonFailed, "VPS not found")
			RecordError(vn, ReasonVPSieAPIError, "VPS not found")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("failed to get VPS status: %w", err)
	}

	// Update status with latest VPS information
	vn.Status.VPSieStatus = vps.Status
	vn.Status.Hostname = vps.Hostname
	vn.Spec.IPAddress = vps.IPAddress
	vn.Spec.IPv6Address = vps.IPv6Address
	vn.Status.Resources = v1alpha1.NodeResources{
		CPU:      vps.CPU,
		MemoryMB: vps.RAM,
		DiskGB:   vps.Disk,
	}

	logger.Debug("VPS status",
		zap.String("vpsienode", vn.Name),
		zap.String("status", vps.Status),
	)

	// Check if VPS is running
	if vps.Status == "running" {
		logger.Info("VPS is now running",
			zap.String("vpsienode", vn.Name),
			zap.Int("vpsID", vps.ID),
		)

		// Transition to Provisioned phase
		SetPhase(vn, v1alpha1.VPSieNodePhaseProvisioned, ReasonProvisioned, "VPS is running")
		SetVPSReadyCondition(vn, true, ReasonProvisioned, "VPS is running")
		now := metav1.Now()
		vn.Status.ProvisionedAt = &now

		// Requeue to start node joining process
		return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
	}

	// VPS is not running yet, keep polling
	logger.Debug("VPS is not running yet, continuing to poll",
		zap.String("vpsienode", vn.Name),
		zap.String("status", vps.Status),
	)

	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// Delete deletes the VPS from VPSie
// Uses the K8s-specific deletion API when ResourceIdentifier and VPSieNodeIdentifier are available
func (p *Provisioner) Delete(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) error {
	nodeIdentifier := vn.Spec.VPSieNodeIdentifier

	// If we have ResourceIdentifier but no VPSieNodeIdentifier, try to look it up by hostname
	if vn.Spec.ResourceIdentifier != "" && nodeIdentifier == "" {
		hostname := vn.Status.Hostname
		if hostname == "" {
			hostname = vn.Spec.NodeName
		}
		if hostname == "" {
			hostname = vn.Status.NodeName
		}

		if hostname != "" {
			logger.Info("Looking up K8s node identifier by hostname",
				zap.String("vpsienode", vn.Name),
				zap.String("hostname", hostname),
				zap.String("clusterIdentifier", vn.Spec.ResourceIdentifier),
			)

			lookedUpID, err := p.vpsieClient.FindK8sNodeIdentifier(ctx, vn.Spec.ResourceIdentifier, hostname)
			if err != nil {
				logger.Warn("Failed to look up node identifier, will try other deletion methods",
					zap.String("vpsienode", vn.Name),
					zap.String("vpsieNodeIdentifier", vn.Spec.VPSieNodeIdentifier),
					zap.String("statusHostname", vn.Status.Hostname),
					zap.String("specNodeName", vn.Spec.NodeName),
					zap.String("statusNodeName", vn.Status.NodeName),
					zap.String("hostnameUsed", hostname),
					zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
					zap.Int("vpsieInstanceID", vn.Spec.VPSieInstanceID),
					zap.Error(err),
				)
			} else if lookedUpID != "" {
				logger.Info("Found node identifier via cluster info lookup",
					zap.String("vpsienode", vn.Name),
					zap.String("nodeIdentifier", lookedUpID),
				)
				nodeIdentifier = lookedUpID
			} else {
				// Node was not found in cluster info - this means it's already deleted from VPSie
				// We successfully queried the K8s cluster API, the cluster exists, but the node doesn't
				// This is the expected state after a manual deletion or if the node was never created
				logger.Info("Node not found in cluster info, treating as already deleted",
					zap.String("vpsienode", vn.Name),
					zap.String("hostnameSearched", hostname),
					zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
				)
				// Return nil to indicate success - the VPS doesn't exist so there's nothing to delete
				return nil
			}
		}
	}

	// Try K8s-specific deletion API if we have the required identifiers
	if vn.Spec.ResourceIdentifier != "" && nodeIdentifier != "" {
		logger.Info("Deleting K8s node via cluster API",
			zap.String("vpsienode", vn.Name),
			zap.String("clusterIdentifier", vn.Spec.ResourceIdentifier),
			zap.String("nodeIdentifier", nodeIdentifier),
		)

		err := p.vpsieClient.DeleteK8sNode(ctx, vn.Spec.ResourceIdentifier, nodeIdentifier)
		if err != nil {
			// If node not found, consider it already deleted
			if vpsieclient.IsNotFound(err) {
				logger.Info("K8s node not found, already deleted",
					zap.String("vpsienode", vn.Name),
					zap.String("nodeIdentifier", nodeIdentifier),
				)
				return nil
			}

			logger.Error("Failed to delete K8s node via cluster API",
				zap.String("vpsienode", vn.Name),
				zap.String("nodeIdentifier", nodeIdentifier),
				zap.Error(err),
			)
			return fmt.Errorf("failed to delete K8s node: %w", err)
		}

		logger.Info("K8s node deleted successfully via cluster API",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeIdentifier", nodeIdentifier),
		)

		now := metav1.Now()
		vn.Status.DeletedAt = &now
		return nil
	}

	// Fallback to regular VM deletion if K8s identifiers are not available
	if vn.Spec.VPSieInstanceID == 0 {
		logger.Warn("No VPS ID or K8s node identifier available for deletion",
			zap.String("vpsienode", vn.Name),
			zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
			zap.String("hostname", vn.Status.Hostname),
		)
		// Don't silently skip - return error so the caller knows deletion didn't happen
		return fmt.Errorf("cannot delete VPS: no VPSieInstanceID or VPSieNodeIdentifier available (hostname: %s)", vn.Status.Hostname)
	}

	logger.Info("Deleting VPS via VM API (fallback)",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
	)

	// Delete VPS via VPSie VM API
	err := p.vpsieClient.DeleteVM(ctx, vn.Spec.VPSieInstanceID)
	if err != nil {
		// If VPS not found, consider it already deleted
		if vpsieclient.IsNotFound(err) {
			logger.Info("VPS not found, already deleted",
				zap.String("vpsienode", vn.Name),
				zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			)
			return nil
		}

		logger.Error("Failed to delete VPS",
			zap.String("vpsienode", vn.Name),
			zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete VPS: %w", err)
	}

	logger.Info("VPS deleted successfully",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
	)

	now := metav1.Now()
	vn.Status.DeletedAt = &now

	return nil
}

// generateHostname generates a hostname for the VPS.
// This function is used as a fallback when updating VPSieNode spec after VPS creation,
// specifically when both vn.Spec.NodeName is empty AND the VPSie API does not return
// a hostname (vps.Hostname is empty). In practice, VPSie API usually provides the
// hostname, but this fallback ensures we always have a valid NodeName set.
// See UpdateVPSieNodeFromVPS where this is called.
func (p *Provisioner) generateHostname(vn *v1alpha1.VPSieNode) string {
	if vn.Spec.NodeName != "" {
		return vn.Spec.NodeName
	}
	// Generate hostname from VPSieNode name
	// Kubernetes node names must be lowercase and can contain dashes
	return vn.Name
}

// GetVPS gets the VPS for a VPSieNode
func (p *Provisioner) GetVPS(ctx context.Context, vn *v1alpha1.VPSieNode) (*vpsieclient.VPS, error) {
	if vn.Spec.VPSieInstanceID == 0 {
		return nil, fmt.Errorf("VPS ID not set")
	}

	return p.vpsieClient.GetVM(ctx, vn.Spec.VPSieInstanceID)
}

// ListVPSByTag lists VPSs by tag
func (p *Provisioner) ListVPSByTag(ctx context.Context, tag string) ([]vpsieclient.VPS, error) {
	// TODO: Implement tag-based filtering once VPSie API supports it
	// For now, list all VMs and filter client-side
	vms, err := p.vpsieClient.ListVMs(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []vpsieclient.VPS
	for _, vm := range vms {
		for _, vmTag := range vm.Tags {
			if vmTag == tag {
				filtered = append(filtered, vm)
				break
			}
		}
	}

	return filtered, nil
}

// ParseVPSIDFromString parses a VPS ID from a string
func ParseVPSIDFromString(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid VPS ID: %w", err)
	}
	return id, nil
}
