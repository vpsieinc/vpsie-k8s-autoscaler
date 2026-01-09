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
}

// Ensure vpsieclient.Client implements VPSieClientInterface
var _ VPSieClientInterface = (*vpsieclient.Client)(nil)

// Ensure MockVPSieClient implements VPSieClientInterface
var _ VPSieClientInterface = (*MockVPSieClient)(nil)
