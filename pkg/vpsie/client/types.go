package client

import (
	"time"
)

// VPS represents a Virtual Private Server instance from VPSie API
type VPS struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Hostname     string    `json:"hostname"`
	Status       string    `json:"status"` // running, stopped, suspended, etc.
	CPU          int       `json:"cpu"`    // Number of CPU cores
	RAM          int       `json:"ram"`    // RAM in MB
	Disk         int       `json:"disk"`   // Disk size in GB
	Bandwidth    int       `json:"bandwidth"`
	IPAddress    string    `json:"ip_address"`
	IPv6Address  string    `json:"ipv6_address"`
	OfferingID   string    `json:"offering_id"`
	DatacenterID string    `json:"datacenter_id"`
	OSName       string    `json:"os_name"`
	OSVersion    string    `json:"os_version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Tags         []string  `json:"tags"`
	Notes        string    `json:"notes"`
}

// Offering represents a VPSie instance type/plan
type Offering struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CPU          int     `json:"cpu"`           // Number of CPU cores
	RAM          int     `json:"ram"`           // RAM in MB
	Disk         int     `json:"disk"`          // Disk size in GB
	Bandwidth    int     `json:"bandwidth"`     // Bandwidth in GB
	Price        float64 `json:"price"`         // Monthly price in USD
	HourlyPrice  float64 `json:"hourly_price"`  // Hourly price in USD
	Available    bool    `json:"available"`     // Whether offering is available
	DatacenterID string  `json:"datacenter_id"` // Datacenter where offering is available
	Category     string  `json:"category"`      // e.g., "standard", "optimized", "high-memory"
}

// Datacenter represents a VPSie datacenter/region
type Datacenter struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code"`        // e.g., "us-east", "eu-west"
	Country    string `json:"country"`     // e.g., "United States", "Germany"
	City       string `json:"city"`        // e.g., "New York", "Frankfurt"
	Continent  string `json:"continent"`   // e.g., "North America", "Europe"
	Available  bool   `json:"available"`   // Whether datacenter accepts new VMs
	FeaturedDC bool   `json:"featured_dc"` // Whether this is a featured datacenter
}

// OSImage represents an operating system image
type OSImage struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Distribution string `json:"distribution"` // e.g., "ubuntu", "debian", "centos"
	Architecture string `json:"architecture"` // e.g., "x86_64"
	MinDisk      int    `json:"min_disk"`     // Minimum disk size in GB
	Available    bool   `json:"available"`
}

// CreateVPSRequest represents a request to create a new VPS
type CreateVPSRequest struct {
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	OfferingID   string   `json:"offering_id"`
	DatacenterID string   `json:"datacenter_id"`
	OSImageID    string   `json:"os_image_id"`
	SSHKeyIDs    []string `json:"ssh_key_ids,omitempty"`
	Password     string   `json:"password,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	UserData     string   `json:"user_data,omitempty"` // Cloud-init user data
}

// UpdateVPSRequest represents a request to update a VPS
type UpdateVPSRequest struct {
	Name  string   `json:"name,omitempty"`
	Notes string   `json:"notes,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

// VPSAction represents an action to perform on a VPS
type VPSAction struct {
	Action string `json:"action"` // start, stop, restart, rebuild, etc.
}

// ListVPSResponse represents the response from listing VPSs
type ListVPSResponse struct {
	Data       []VPS      `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// ListOfferingsResponse represents the response from listing offerings
type ListOfferingsResponse struct {
	Data       []Offering `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// ListDatacentersResponse represents the response from listing datacenters
type ListDatacentersResponse struct {
	Data       []Datacenter `json:"data"`
	Pagination Pagination   `json:"pagination"`
}

// ListOSImagesResponse represents the response from listing OS images
type ListOSImagesResponse struct {
	Data       []OSImage  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Pagination represents pagination information in API responses
type Pagination struct {
	Total       int `json:"total"`
	Count       int `json:"count"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	TotalPages  int `json:"total_pages"`
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// ListOptions represents options for listing resources
type ListOptions struct {
	Page    int    // Page number for pagination
	PerPage int    // Number of items per page
	Query   string // Search query
	SortBy  string // Field to sort by
	Order   string // Sort order: "asc" or "desc"
}

// SSHKey represents an SSH key stored in VPSie
type SSHKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"public_key"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateSSHKeyRequest represents a request to create an SSH key
type CreateSSHKeyRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

// Snapshot represents a VPS snapshot
type Snapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	VPSID       string    `json:"vps_id"`
	Size        int       `json:"size"`   // Size in GB
	Status      string    `json:"status"` // creating, available, deleting
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// CreateSnapshotRequest represents a request to create a snapshot
type CreateSnapshotRequest struct {
	Name  string `json:"name"`
	VPSID string `json:"vps_id"`
}

// RestoreSnapshotRequest represents a request to restore a snapshot
type RestoreSnapshotRequest struct {
	SnapshotID string `json:"snapshot_id"`
}

// VPSMetrics represents metrics for a VPS
type VPSMetrics struct {
	VPSID      string    `json:"vps_id"`
	CPUUsage   float64   `json:"cpu_usage"`   // CPU usage percentage
	RAMUsage   float64   `json:"ram_usage"`   // RAM usage percentage
	DiskUsage  float64   `json:"disk_usage"`  // Disk usage percentage
	NetworkIn  int64     `json:"network_in"`  // Network in bytes
	NetworkOut int64     `json:"network_out"` // Network out bytes
	Timestamp  time.Time `json:"timestamp"`
}
