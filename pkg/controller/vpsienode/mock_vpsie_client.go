package vpsienode

import (
	"context"
	"fmt"
	"sync"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// MockVPSieClient is a mock implementation of the VPSie client for testing
type MockVPSieClient struct {
	mu sync.RWMutex

	// VMs stores the mocked VMs by ID
	VMs map[int]*vpsieclient.VPS

	// NextID is the next VPS ID to assign
	NextID int

	// CreateVMFunc allows custom behavior for CreateVM
	CreateVMFunc func(ctx context.Context, req vpsieclient.CreateVPSRequest) (*vpsieclient.VPS, error)

	// GetVMFunc allows custom behavior for GetVM
	GetVMFunc func(ctx context.Context, id int) (*vpsieclient.VPS, error)

	// DeleteVMFunc allows custom behavior for DeleteVM
	DeleteVMFunc func(ctx context.Context, id int) error

	// ListVMsFunc allows custom behavior for ListVMs
	ListVMsFunc func(ctx context.Context) ([]vpsieclient.VPS, error)

	// CallCounts tracks how many times each method was called
	CallCounts map[string]int
}

// NewMockVPSieClient creates a new mock VPSie client
func NewMockVPSieClient() *MockVPSieClient {
	return &MockVPSieClient{
		VMs:        make(map[int]*vpsieclient.VPS),
		NextID:     1000,
		CallCounts: make(map[string]int),
	}
}

// CreateVM creates a mock VPS
func (m *MockVPSieClient) CreateVM(ctx context.Context, req vpsieclient.CreateVPSRequest) (*vpsieclient.VPS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCounts["CreateVM"]++

	// Use custom function if provided
	if m.CreateVMFunc != nil {
		return m.CreateVMFunc(ctx, req)
	}

	// Create a new VPS
	vps := &vpsieclient.VPS{
		ID:           m.NextID,
		Name:         req.Name,
		Hostname:     req.Hostname,
		Status:       "provisioning", // Start in provisioning state
		CPU:          4,
		RAM:          8192,
		Disk:         80,
		Bandwidth:    1000,
		IPAddress:    fmt.Sprintf("10.0.0.%d", m.NextID%256),
		IPv6Address:  fmt.Sprintf("2001:db8::%d", m.NextID),
		OfferingID:   parseOfferingID(req.OfferingID),
		DatacenterID: parseDatacenterID(req.DatacenterID),
		OSName:       "Ubuntu",
		OSVersion:    "22.04",
		Tags:         req.Tags,
		Notes:        req.Notes,
	}

	m.VMs[m.NextID] = vps
	m.NextID++

	return vps, nil
}

// GetVM gets a mock VPS by ID
func (m *MockVPSieClient) GetVM(ctx context.Context, id int) (*vpsieclient.VPS, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.CallCounts["GetVM"]++

	// Use custom function if provided
	if m.GetVMFunc != nil {
		return m.GetVMFunc(ctx, id)
	}

	vps, exists := m.VMs[id]
	if !exists {
		return nil, &vpsieclient.APIError{
			StatusCode: 404,
			Message:    "VPS not found",
		}
	}

	// Return a copy to prevent test contamination
	vpsCopy := *vps
	return &vpsCopy, nil
}

// DeleteVM deletes a mock VPS
func (m *MockVPSieClient) DeleteVM(ctx context.Context, id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCounts["DeleteVM"]++

	// Use custom function if provided
	if m.DeleteVMFunc != nil {
		return m.DeleteVMFunc(ctx, id)
	}

	if _, exists := m.VMs[id]; !exists {
		return &vpsieclient.APIError{
			StatusCode: 404,
			Message:    "VPS not found",
		}
	}

	delete(m.VMs, id)
	return nil
}

// ListVMs lists all mock VPSs
func (m *MockVPSieClient) ListVMs(ctx context.Context) ([]vpsieclient.VPS, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.CallCounts["ListVMs"]++

	// Use custom function if provided
	if m.ListVMsFunc != nil {
		return m.ListVMsFunc(ctx)
	}

	vms := make([]vpsieclient.VPS, 0, len(m.VMs))
	for _, vps := range m.VMs {
		vms = append(vms, *vps)
	}

	return vms, nil
}

// UpdateVMStatus updates the status of a mock VPS
func (m *MockVPSieClient) UpdateVMStatus(id int, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vps, exists := m.VMs[id]
	if !exists {
		return fmt.Errorf("VPS %d not found", id)
	}

	vps.Status = status
	return nil
}

// GetCallCount returns the number of times a method was called
func (m *MockVPSieClient) GetCallCount(method string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.CallCounts[method]
}

// ResetCallCounts resets all call counts
func (m *MockVPSieClient) ResetCallCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCounts = make(map[string]int)
}

// GetBaseURL returns a mock base URL
func (m *MockVPSieClient) GetBaseURL() string {
	return "https://mock.vpsie.com/v2"
}

// AddK8sNode adds a mock K8s node (delegates to CreateVM-like behavior)
func (m *MockVPSieClient) AddK8sNode(ctx context.Context, req vpsieclient.AddK8sNodeRequest) (*vpsieclient.VPS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCounts["AddK8sNode"]++

	// Create a new VPS representing the K8s node
	vps := &vpsieclient.VPS{
		ID:           m.NextID,
		Name:         req.Hostname,
		Hostname:     req.Hostname,
		Status:       "provisioning", // Start in provisioning state
		CPU:          4,
		RAM:          8192,
		Disk:         80,
		Bandwidth:    1000,
		IPAddress:    fmt.Sprintf("10.0.0.%d", m.NextID%256),
		IPv6Address:  fmt.Sprintf("2001:db8::%d", m.NextID),
		OfferingID:   1, // Default value
		DatacenterID: parseDatacenterID(req.DatacenterID),
		OSName:       "Ubuntu",
		OSVersion:    "22.04",
		Tags:         []string{"kubernetes", "autoscaler"},
		Notes:        fmt.Sprintf("K8s node for cluster %s", req.ResourceIdentifier),
	}

	m.VMs[m.NextID] = vps
	m.NextID++

	return vps, nil
}

// AddK8sSlaveToGroup adds a mock K8s slave node to a specific group
func (m *MockVPSieClient) AddK8sSlaveToGroup(ctx context.Context, clusterIdentifier string, groupID int) (*vpsieclient.VPS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCounts["AddK8sSlaveToGroup"]++

	// Create a new VPS representing the K8s slave node
	vps := &vpsieclient.VPS{
		ID:           m.NextID,
		Name:         fmt.Sprintf("k8s-slave-%d", m.NextID),
		Hostname:     fmt.Sprintf("k8s-slave-%d", m.NextID),
		Status:       "provisioning", // Start in provisioning state
		CPU:          4,
		RAM:          8192,
		Disk:         80,
		Bandwidth:    1000,
		IPAddress:    fmt.Sprintf("10.0.0.%d", m.NextID%256),
		IPv6Address:  fmt.Sprintf("2001:db8::%d", m.NextID),
		OfferingID:   groupID, // Use group ID as offering ID for mock
		DatacenterID: 1,
		OSName:       "Ubuntu",
		OSVersion:    "22.04",
		Tags:         []string{"kubernetes", "autoscaler"},
		Notes:        fmt.Sprintf("K8s slave node for cluster %s, group %d", clusterIdentifier, groupID),
	}

	m.VMs[m.NextID] = vps
	m.NextID++

	return vps, nil
}

// Helper functions to parse string IDs to int
func parseOfferingID(id string) int {
	// In real implementation, this would parse the string ID
	// For mock, just return a fixed value
	return 1
}

func parseDatacenterID(id string) int {
	// In real implementation, this would parse the string ID
	// For mock, just return a fixed value
	return 1
}
