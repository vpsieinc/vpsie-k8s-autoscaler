package vpsienode

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Test helper functions

// newTestDiscoverer creates a Discoverer with mock dependencies
func newTestDiscoverer(t *testing.T, mockVPSie *MockVPSieClient, k8sObjs ...runtime.Object) *Discoverer {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range k8sObjs {
		builder = builder.WithRuntimeObjects(obj)
	}
	k8sClient := builder.Build()

	return NewDiscoverer(mockVPSie, k8sClient, zap.NewNop())
}

// newTestVPSieNode creates a VPSieNode for testing with common defaults
func newTestVPSieNode(name string, opts ...func(*v1alpha1.VPSieNode)) *v1alpha1.VPSieNode {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1alpha1.VPSieNodeSpec{
			ResourceIdentifier: "test-cluster",
			VPSieGroupID:       100,
		},
		Status: v1alpha1.VPSieNodeStatus{
			CreatedAt: &metav1.Time{Time: time.Now()},
		},
	}
	for _, opt := range opts {
		opt(vn)
	}
	return vn
}

// newTestK8sNode creates a K8s Node for testing
func newTestK8sNode(name, ip string, labels map[string]string) *corev1.Node {
	if labels == nil {
		labels = make(map[string]string)
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: ip},
			},
		},
	}
}

// VPSieNode option functions
func withCreatedAt(t time.Time) func(*v1alpha1.VPSieNode) {
	return func(vn *v1alpha1.VPSieNode) {
		vn.Status.CreatedAt = &metav1.Time{Time: t}
	}
}

// Discovery Success Scenarios

func TestDiscoverVPSID_Success_ByHostname(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:        12345,
				Hostname:  "my-node-k8s-worker",
				IPAddress: "10.0.0.5",
				Status:    "running",
				CreatedAt: time.Now(),
			},
		}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	require.NotNil(t, vps)
	assert.Equal(t, 12345, vps.ID)
	assert.Equal(t, "my-node-k8s-worker", vps.Hostname)
}

func TestDiscoverVPSID_Success_ByIP(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:        67890,
				Hostname:  "completely-different-name",
				IPAddress: "10.0.0.100",
				Status:    "running",
				CreatedAt: time.Now(),
			},
		}, nil
	}

	// Create K8s node with matching IP
	k8sNode := newTestK8sNode("k8s-node-1", "10.0.0.100", nil)

	discoverer := newTestDiscoverer(t, mockClient, k8sNode)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	require.NotNil(t, vps)
	assert.Equal(t, 67890, vps.ID)
	assert.Equal(t, "10.0.0.100", vps.IPAddress)
}

func TestDiscoverVPSID_MultipleCandidates_SelectsNewest(t *testing.T) {
	// Arrange
	now := time.Now()
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:        111,
				Hostname:  "my-node-old",
				Status:    "running",
				CreatedAt: now.Add(-2 * time.Hour),
			},
			{
				ID:        222,
				Hostname:  "my-node-newest",
				Status:    "running",
				CreatedAt: now,
			},
			{
				ID:        333,
				Hostname:  "my-node-middle",
				Status:    "running",
				CreatedAt: now.Add(-1 * time.Hour),
			},
		}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	require.NotNil(t, vps)
	// Should select the newest (222) because it matches hostname pattern and is newest
	assert.Equal(t, 222, vps.ID)
}

// Discovery Failure Scenarios

func TestDiscoverVPSID_Timeout(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:       12345,
				Hostname: "some-node",
				Status:   "running",
			},
		}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	// Created more than 15 minutes ago
	vn := newTestVPSieNode("my-node", withCreatedAt(time.Now().Add(-20*time.Minute)))

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.True(t, timedOut)
	assert.Nil(t, vps)
}

func TestDiscoverVPSID_NoCandidates(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	assert.Nil(t, vps)
}

func TestDiscoverVPSID_APIError(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return nil, fmt.Errorf("API connection failed")
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list VMs")
	assert.False(t, timedOut)
	assert.Nil(t, vps)
}

func TestDiscoverVPSID_SkipsNonRunningVMs(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:        111,
				Hostname:  "my-node-provisioning",
				Status:    "provisioning",
				CreatedAt: time.Now(),
			},
			{
				ID:        222,
				Hostname:  "my-node-stopped",
				Status:    "stopped",
				CreatedAt: time.Now(),
			},
		}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	assert.Nil(t, vps) // No running VMs, so no discovery
}

// Hostname Pattern Tests

func TestMatchesHostnamePattern_ExactPrefix(t *testing.T) {
	tests := []struct {
		name           string
		vpsieNodeName  string
		vpsHostname    string
		expectedResult bool
	}{
		{
			name:           "exact prefix match",
			vpsieNodeName:  "my-node",
			vpsHostname:    "my-node-k8s-worker",
			expectedResult: true,
		},
		{
			name:           "exact match",
			vpsieNodeName:  "my-node",
			vpsHostname:    "my-node",
			expectedResult: true,
		},
		{
			name:           "case insensitive",
			vpsieNodeName:  "My-Node",
			vpsHostname:    "my-node-k8s-worker",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesHostnamePattern(tt.vpsieNodeName, tt.vpsHostname)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestMatchesHostnamePattern_NoMatch(t *testing.T) {
	tests := []struct {
		name           string
		vpsieNodeName  string
		vpsHostname    string
		expectedResult bool
	}{
		{
			name:           "completely different",
			vpsieNodeName:  "my-node",
			vpsHostname:    "other-node",
			expectedResult: false,
		},
		{
			name:           "partial overlap but not prefix",
			vpsieNodeName:  "my-node",
			vpsHostname:    "test-my-node",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesHostnamePattern(tt.vpsieNodeName, tt.vpsHostname)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestMatchesHostnamePattern_ShorterHostname(t *testing.T) {
	// Hostname must be >= VPSieNode name length
	result := matchesHostnamePattern("my-long-node-name", "my-long")
	assert.False(t, result)
}

// K8s Node Matching Tests

func TestFindK8sNodeByIP_Found(t *testing.T) {
	// Arrange
	k8sNode := newTestK8sNode("test-node", "192.168.1.100", nil)
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient, k8sNode)

	// Act
	found, err := discoverer.findK8sNodeByIP(context.Background(), "192.168.1.100")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "test-node", found.Name)
}

func TestFindK8sNodeByIP_NotFound(t *testing.T) {
	// Arrange
	k8sNode := newTestK8sNode("test-node", "192.168.1.100", nil)
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient, k8sNode)

	// Act
	found, err := discoverer.findK8sNodeByIP(context.Background(), "10.0.0.1")

	// Assert
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindK8sNodeByIP_MatchesExternalIP(t *testing.T) {
	// Arrange
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.100"},
				{Type: corev1.NodeExternalIP, Address: "203.0.113.50"},
			},
		},
	}
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient, k8sNode)

	// Act
	found, err := discoverer.findK8sNodeByIP(context.Background(), "203.0.113.50")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "test-node", found.Name)
}

// Claim Checking Tests

func TestIsNodeClaimedByOther_Claimed(t *testing.T) {
	// Arrange
	k8sNode := newTestK8sNode("test-node", "10.0.0.1", map[string]string{
		"autoscaler.vpsie.com/vpsienode": "other-vpsienode",
	})
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient)

	vn := newTestVPSieNode("my-vpsienode")

	// Act
	result := discoverer.isNodeClaimedByOther(k8sNode, vn)

	// Assert
	assert.True(t, result)
}

func TestIsNodeClaimedByOther_NotClaimed(t *testing.T) {
	// Arrange - node has no VPSieNode label
	k8sNode := newTestK8sNode("test-node", "10.0.0.1", nil)
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient)

	vn := newTestVPSieNode("my-vpsienode")

	// Act
	result := discoverer.isNodeClaimedByOther(k8sNode, vn)

	// Assert
	assert.False(t, result)
}

func TestIsNodeClaimedByOther_SameVPSieNode(t *testing.T) {
	// Arrange - node points to the current VPSieNode
	k8sNode := newTestK8sNode("test-node", "10.0.0.1", map[string]string{
		"autoscaler.vpsie.com/vpsienode": "my-vpsienode",
	})
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient)

	vn := newTestVPSieNode("my-vpsienode")

	// Act
	result := discoverer.isNodeClaimedByOther(k8sNode, vn)

	// Assert
	assert.False(t, result) // Same node, not "other"
}

func TestIsNodeClaimedByOther_EmptyLabel(t *testing.T) {
	// Arrange - node has empty VPSieNode label
	k8sNode := newTestK8sNode("test-node", "10.0.0.1", map[string]string{
		"autoscaler.vpsie.com/vpsienode": "",
	})
	mockClient := NewMockVPSieClient()
	discoverer := newTestDiscoverer(t, mockClient)

	vn := newTestVPSieNode("my-vpsienode")

	// Act
	result := discoverer.isNodeClaimedByOther(k8sNode, vn)

	// Assert
	assert.False(t, result) // Empty label means not claimed
}

// Discovery edge case - VPSieNode with nil CreatedAt
func TestDiscoverVPSID_NilCreatedAt(t *testing.T) {
	// Arrange
	mockClient := NewMockVPSieClient()
	mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
		return []vpsieclient.VPS{
			{
				ID:        12345,
				Hostname:  "my-node-k8s-worker",
				Status:    "running",
				CreatedAt: time.Now(),
			},
		}, nil
	}

	discoverer := newTestDiscoverer(t, mockClient)
	vn := newTestVPSieNode("my-node")
	vn.Status.CreatedAt = nil // No CreatedAt set

	// Act
	vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

	// Assert
	require.NoError(t, err)
	assert.False(t, timedOut)
	require.NotNil(t, vps)
	assert.Equal(t, 12345, vps.ID)
}
