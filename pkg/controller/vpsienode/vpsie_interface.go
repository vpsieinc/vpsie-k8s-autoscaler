package vpsienode

import (
	"context"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// VPSieClientInterface defines the interface for VPSie API operations
type VPSieClientInterface interface {
	CreateVM(ctx context.Context, req vpsieclient.CreateVPSRequest) (*vpsieclient.VPS, error)
	GetVM(ctx context.Context, id int) (*vpsieclient.VPS, error)
	DeleteVM(ctx context.Context, id int) error
	ListVMs(ctx context.Context) ([]vpsieclient.VPS, error)
	GetBaseURL() string
	// AddK8sNode adds a node to a VPSie managed Kubernetes cluster (uses /apps/v2 API)
	AddK8sNode(ctx context.Context, req vpsieclient.AddK8sNodeRequest) (*vpsieclient.VPS, error)
	// AddK8sSlaveToGroup adds a slave node to a specific node group in a VPSie K8s cluster
	// Uses endpoint: POST /k8s/cluster/byId/{clusterIdentifier}/add/slave/group/{groupID}
	AddK8sSlaveToGroup(ctx context.Context, clusterIdentifier string, groupID int) (*vpsieclient.VPS, error)
	// ListK8sNodeGroups lists all node groups for a VPSie managed Kubernetes cluster
	// Returns the node groups with their numeric IDs and node counts
	// Used by Discoverer to find VPS nodes created via async provisioning
	ListK8sNodeGroups(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error)
	// DeleteK8sNode deletes a node from a VPSie managed Kubernetes cluster
	// Uses the K8s-specific deletion API: DELETE /k8s/cluster/byId/{clusterIdentifier}/delete/slave
	DeleteK8sNode(ctx context.Context, clusterIdentifier, nodeIdentifier string) error
	// FindK8sNodeIdentifier looks up a node's identifier by hostname in a cluster
	// Used during node deletion when VPSieNodeIdentifier is not set
	FindK8sNodeIdentifier(ctx context.Context, clusterIdentifier, hostname string) (string, error)
}

// Ensure vpsieclient.Client implements VPSieClientInterface
var _ VPSieClientInterface = (*vpsieclient.Client)(nil)

// Ensure MockVPSieClient implements VPSieClientInterface
var _ VPSieClientInterface = (*MockVPSieClient)(nil)
