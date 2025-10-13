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
}

// Ensure vpsieclient.Client implements VPSieClientInterface
var _ VPSieClientInterface = (*vpsieclient.Client)(nil)

// Ensure MockVPSieClient implements VPSieClientInterface
var _ VPSieClientInterface = (*MockVPSieClient)(nil)
