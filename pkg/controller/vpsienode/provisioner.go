package vpsienode

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Provisioner handles VPS provisioning operations
type Provisioner struct {
	vpsieClient VPSieClientInterface
	// Cloud-init template for node bootstrapping
	cloudInitTemplate string
	// SSH key IDs to inject into VPS
	sshKeyIDs []string
}

// NewProvisioner creates a new Provisioner
func NewProvisioner(vpsieClient VPSieClientInterface, cloudInitTemplate string, sshKeyIDs []string) *Provisioner {
	return &Provisioner{
		vpsieClient:       vpsieClient,
		cloudInitTemplate: cloudInitTemplate,
		sshKeyIDs:         sshKeyIDs,
	}
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

// createVPS creates a new VPS instance via VPSie API
func (p *Provisioner) createVPS(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Creating new VPS",
		zap.String("vpsienode", vn.Name),
		zap.String("instanceType", vn.Spec.InstanceType),
		zap.String("datacenter", vn.Spec.DatacenterID),
	)

	// Generate hostname
	hostname := p.generateHostname(vn)

	// Create VPS request
	req := vpsieclient.CreateVPSRequest{
		Name:         vn.Name,
		Hostname:     hostname,
		OfferingID:   vn.Spec.InstanceType,
		DatacenterID: vn.Spec.DatacenterID,
		OSImageID:    vn.Spec.OSImageID,
		SSHKeyIDs:    p.getSSHKeyIDs(vn),
		UserData:     vn.Spec.UserData,
		Tags:         []string{"kubernetes", "autoscaler", vn.Spec.NodeGroupName},
		Notes:        fmt.Sprintf("Managed by VPSie Kubernetes Autoscaler - NodeGroup: %s", vn.Spec.NodeGroupName),
	}

	// Call VPSie API to create VPS
	vps, err := p.vpsieClient.CreateVM(ctx, req)
	if err != nil {
		logger.Error("Failed to create VPS via VPSie API",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("failed to create VPS: %w", err)
	}

	logger.Info("VPS created successfully",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vps.ID),
		zap.String("hostname", vps.Hostname),
	)

	// Update VPSieNode spec with VPS information
	vn.Spec.VPSieInstanceID = vps.ID
	vn.Spec.IPAddress = vps.IPAddress
	vn.Spec.IPv6Address = vps.IPv6Address
	if vn.Spec.NodeName == "" {
		vn.Spec.NodeName = hostname
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
func (p *Provisioner) Delete(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) error {
	if vn.Spec.VPSieInstanceID == 0 {
		logger.Info("No VPS ID set, skipping deletion",
			zap.String("vpsienode", vn.Name),
		)
		return nil
	}

	logger.Info("Deleting VPS",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
	)

	// Delete VPS via VPSie API
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

// generateHostname generates a hostname for the VPS
func (p *Provisioner) generateHostname(vn *v1alpha1.VPSieNode) string {
	if vn.Spec.NodeName != "" {
		return vn.Spec.NodeName
	}
	// Generate hostname from VPSieNode name
	// Kubernetes node names must be lowercase and can contain dashes
	return vn.Name
}

// generateCloudInit generates cloud-init user data for node bootstrapping
func (p *Provisioner) generateCloudInit(vn *v1alpha1.VPSieNode) string {
	// If a template is provided, use it
	if p.cloudInitTemplate != "" {
		// TODO: Replace template variables with actual values
		// For now, return the template as-is
		return p.cloudInitTemplate
	}

	// Generate basic cloud-init that sets hostname and prepares for kubeadm
	hostname := p.generateHostname(vn)
	return fmt.Sprintf(`#cloud-config
hostname: %s
fqdn: %s
manage_etc_hosts: true

packages:
  - curl
  - apt-transport-https
  - ca-certificates
  - gnupg

runcmd:
  - echo "VPSie Kubernetes Node - %s" > /etc/motd
  - echo "NodeGroup: %s" >> /etc/motd
  - echo "VPSieNode: %s" >> /etc/motd
  - echo "Node provisioned at: %s" >> /etc/motd
`, hostname, hostname, vn.Name, vn.Spec.NodeGroupName, vn.Name, time.Now().Format(time.RFC3339))
}

// getSSHKeyIDs returns SSH key IDs to use for VPS provisioning
// Prefers spec-level SSH keys (per-node override), falls back to provisioner-level (global config)
func (p *Provisioner) getSSHKeyIDs(vn *v1alpha1.VPSieNode) []string {
	// If VPSieNode spec has SSH keys defined, use them (per-node override)
	if len(vn.Spec.SSHKeyIDs) > 0 {
		return vn.Spec.SSHKeyIDs
	}
	// Fall back to provisioner-level SSH keys (global configuration from controller options)
	return p.sshKeyIDs
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
