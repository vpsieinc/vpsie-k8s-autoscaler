package client

import "context"

// VPSieClient defines the interface for interacting with the VPSie API
// This interface is implemented by the Client struct and can be mocked for testing
type VPSieClient interface {
	// Offering operations
	ListOfferings(ctx context.Context, opts *ListOptions) ([]Offering, error)
	GetOffering(ctx context.Context, id string) (*Offering, error)

	// VPS operations
	ListVPS(ctx context.Context, opts *ListOptions) ([]VPS, error)
	GetVPS(ctx context.Context, id int) (*VPS, error)
	CreateVPS(ctx context.Context, req *CreateVPSRequest) (*VPS, error)
	DeleteVPS(ctx context.Context, id int) error
	UpdateVPS(ctx context.Context, id int, req *UpdateVPSRequest) (*VPS, error)
	PerformVPSAction(ctx context.Context, id int, action *VPSAction) error

	// Datacenter operations
	ListDatacenters(ctx context.Context, opts *ListOptions) ([]Datacenter, error)

	// OS Image operations
	ListOSImages(ctx context.Context, opts *ListOptions) ([]OSImage, error)

	// Close cleans up client resources
	Close() error
}

// Ensure Client implements VPSieClient interface
var _ VPSieClient = (*Client)(nil)
