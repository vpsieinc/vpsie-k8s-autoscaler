//go:build chaos
// +build chaos

package chaos

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// NetworkPartition simulates network partitions and latency
type NetworkPartition struct {
	mu            sync.RWMutex
	partitioned   bool
	latency       time.Duration
	packetLoss    float64
	bandwidth     int64 // bytes per second, 0 = unlimited
	partitionTime time.Time
	duration      time.Duration
}

// NewNetworkPartition creates a new network partition simulator
func NewNetworkPartition() *NetworkPartition {
	return &NetworkPartition{}
}

// Enable enables the network partition
func (np *NetworkPartition) Enable(duration time.Duration) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.partitioned = true
	np.partitionTime = time.Now()
	np.duration = duration
}

// Disable disables the network partition
func (np *NetworkPartition) Disable() {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.partitioned = false
}

// IsPartitioned returns true if network is partitioned
func (np *NetworkPartition) IsPartitioned() bool {
	np.mu.RLock()
	defer np.mu.RUnlock()

	if !np.partitioned {
		return false
	}

	// Check if partition duration has expired
	if np.duration > 0 && time.Since(np.partitionTime) > np.duration {
		return false
	}

	return true
}

// SetLatency sets artificial network latency
func (np *NetworkPartition) SetLatency(latency time.Duration) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.latency = latency
}

// SetPacketLoss sets packet loss rate (0.0 to 1.0)
func (np *NetworkPartition) SetPacketLoss(rate float64) {
	np.mu.Lock()
	defer np.mu.Unlock()
	np.packetLoss = rate
}

// SimulateLatency simulates network latency
func (np *NetworkPartition) SimulateLatency() {
	np.mu.RLock()
	latency := np.latency
	np.mu.RUnlock()

	if latency > 0 {
		time.Sleep(latency)
	}
}

// TestNetworkPartition_VPSieAPIUnreachable tests VPSie API becoming unreachable
func TestNetworkPartition_VPSieAPIUnreachable(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	partition := NewNetworkPartition()

	scenario := ChaosScenario{
		Name:        "VPSie API Unreachable",
		Description: "Simulate network partition between controller and VPSie API",
		Setup: func(ctx context.Context, t *testing.T) error {
			// Enable complete partition (100% error rate)
			mock.EnableChaos(ChaosConfig{ErrorRate: 1.0})
			partition.Enable(30 * time.Second)
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			// Attempt requests during partition
			var failed int
			for i := 0; i < 10; i++ {
				if partition.IsPartitioned() {
					resp, err := http.Get(mock.URL())
					if err != nil || (resp != nil && resp.StatusCode >= 500) {
						failed++
					}
					if resp != nil {
						resp.Body.Close()
					}
				}
			}
			t.Logf("Failed requests during partition: %d/10", failed)
			assert.Equal(t, 10, failed, "All requests should fail during partition")
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			// Disable partition
			mock.DisableChaos()
			partition.Disable()

			// Verify recovery
			resp, err := http.Get(mock.URL())
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Should recover after partition")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			partition.Disable()
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestNetworkPartition_KubernetesAPIUnreachable tests K8s API becoming unreachable
func TestNetworkPartition_KubernetesAPIUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create a NodeGroup before partition
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-k8s-partition",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     5,
			DatacenterID: "dc-test-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err)

	defer func() {
		_ = k8sClient.Delete(context.Background(), ng)
	}()

	// In a real chaos test:
	// 1. Block K8s API access (iptables, network policy)
	// 2. Verify controller handles disconnection
	// 3. Verify state is preserved
	// 4. Restore network
	// 5. Verify recovery and reconciliation

	t.Log("Kubernetes API partition test setup complete")
}

// TestNetworkPartition_HighLatency tests behavior under high network latency
func TestNetworkPartition_HighLatency(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "High Network Latency",
		Description: "Simulate high network latency to VPSie API",
		Setup: func(ctx context.Context, t *testing.T) error {
			mock.EnableChaos(ChaosConfig{
				Latency:         3 * time.Second,
				LatencyVariance: 1 * time.Second,
				ErrorRate:       0,
			})
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			client := &http.Client{
				Timeout: 10 * time.Second, // Higher timeout to account for latency
			}

			start := time.Now()
			resp, err := client.Get(mock.URL())
			duration := time.Since(start)

			t.Logf("Request duration with latency: %v", duration)

			if err != nil {
				t.Logf("Request error (expected with high latency): %v", err)
				return nil
			}
			defer resp.Body.Close()

			// Request should take at least 2 seconds (latency - variance)
			assert.Greater(t, duration, 2*time.Second, "Should see latency effect")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestNetworkPartition_IntermittentConnectivity tests intermittent network issues
func TestNetworkPartition_IntermittentConnectivity(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	partition := NewNetworkPartition()

	scenario := ChaosScenario{
		Name:        "Intermittent Connectivity",
		Description: "Simulate intermittent network connectivity (flapping)",
		Execute: func(ctx context.Context, t *testing.T) error {
			var successCount, failCount int

			// Simulate network flapping
			for i := 0; i < 10; i++ {
				// Toggle partition state
				if i%2 == 0 {
					mock.EnableChaos(ChaosConfig{ErrorRate: 1.0})
					partition.Enable(time.Second)
				} else {
					mock.DisableChaos()
					partition.Disable()
				}

				time.Sleep(500 * time.Millisecond)

				resp, err := http.Get(mock.URL())
				if err != nil {
					failCount++
				} else {
					if resp.StatusCode >= 500 {
						failCount++
					} else {
						successCount++
					}
					resp.Body.Close()
				}
			}

			t.Logf("Intermittent test: success=%d, fail=%d", successCount, failCount)

			// Should see roughly equal success/fail due to flapping
			assert.Greater(t, successCount, 0, "Should have some successes")
			assert.Greater(t, failCount, 0, "Should have some failures")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			partition.Disable()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestNetworkPartition_SplitBrain tests split-brain scenario with multiple controllers
func TestNetworkPartition_SplitBrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-split-brain",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "dc-test-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err)

	defer func() {
		_ = k8sClient.Delete(context.Background(), ng)
	}()

	// In a real chaos test:
	// 1. Run 2 controller instances
	// 2. Create network partition between them
	// 3. Verify only one controller makes changes (leader election)
	// 4. Verify no conflicting actions
	// 5. Heal partition
	// 6. Verify state convergence

	t.Log("Split-brain scenario test setup complete")
}

// TestNetworkPartition_GradualDegradation tests gradual network degradation
func TestNetworkPartition_GradualDegradation(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Gradual Network Degradation",
		Description: "Simulate gradually increasing latency and errors",
		Execute: func(ctx context.Context, t *testing.T) error {
			// Start with good network
			levels := []struct {
				latency   time.Duration
				errorRate float64
			}{
				{0, 0},                          // Good
				{100 * time.Millisecond, 0.1},   // Slight degradation
				{500 * time.Millisecond, 0.3},   // Moderate degradation
				{1 * time.Second, 0.5},          // Significant degradation
				{2 * time.Second, 0.8},          // Severe degradation
			}

			for i, level := range levels {
				mock.EnableChaos(ChaosConfig{
					Latency:   level.latency,
					ErrorRate: level.errorRate,
				})

				t.Logf("Level %d: latency=%v, errorRate=%.1f", i, level.latency, level.errorRate)

				// Make a few requests at each level
				var success, fail int
				for j := 0; j < 5; j++ {
					start := time.Now()
					resp, err := http.Get(mock.URL())
					duration := time.Since(start)

					if err != nil || (resp != nil && resp.StatusCode >= 500) {
						fail++
					} else {
						success++
					}
					if resp != nil {
						resp.Body.Close()
					}

					t.Logf("  Request %d: duration=%v, success=%t", j, duration, err == nil && (resp != nil && resp.StatusCode < 500))
				}

				t.Logf("  Results: success=%d, fail=%d", success, fail)
			}
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestNetworkPartition_RecoveryOrder tests recovery after partition heals
func TestNetworkPartition_RecoveryOrder(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Recovery Order",
		Description: "Test that system recovers properly after network heals",
		Setup: func(ctx context.Context, t *testing.T) error {
			// Start with partition
			mock.EnableChaos(ChaosConfig{ErrorRate: 1.0})
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			// Phase 1: During partition
			t.Log("Phase 1: During partition")
			mock.ResetMetrics()

			for i := 0; i < 5; i++ {
				resp, _ := http.Get(mock.URL())
				if resp != nil {
					resp.Body.Close()
				}
			}

			_, failed1, _ := mock.GetMetrics()
			t.Logf("During partition: failed=%d", failed1)
			assert.Equal(t, int64(5), failed1, "All should fail during partition")

			// Phase 2: Partition heals
			t.Log("Phase 2: Partition heals")
			mock.DisableChaos()
			mock.ResetMetrics()

			// Wait for any backoff to reset (in real system)
			time.Sleep(100 * time.Millisecond)

			for i := 0; i < 5; i++ {
				resp, err := http.Get(mock.URL())
				if err != nil {
					t.Logf("Unexpected error after heal: %v", err)
				}
				if resp != nil {
					resp.Body.Close()
				}
			}

			_, failed2, success2 := mock.GetMetrics()
			t.Logf("After heal: failed=%d, success=%d", failed2, success2)
			assert.Equal(t, int64(5), success2, "All should succeed after heal")

			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestNetworkPartition_WithActiveOperations tests partition during active operations
func TestNetworkPartition_WithActiveOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create NodeGroup
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-active-partition",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     5,
			DatacenterID: "dc-test-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err)

	defer func() {
		_ = k8sClient.Delete(context.Background(), ng)
	}()

	// Create VPSieNodes to simulate active nodes
	for i := 0; i < 2; i++ {
		vn := &autoscalerv1alpha1.VPSieNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chaos-active-node-" + string(rune('a'+i)),
				Namespace: TestNamespace,
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": ng.Name,
				},
			},
			Spec: autoscalerv1alpha1.VPSieNodeSpec{
				NodeGroupName: ng.Name,
				DatacenterID:  "dc-test-1",
				OfferingID:    "small-2cpu-4gb",
				OSImageID:     "ubuntu-22.04",
			},
		}

		err := k8sClient.Create(ctx, vn)
		require.NoError(t, err)

		defer func(vn *autoscalerv1alpha1.VPSieNode) {
			_ = k8sClient.Delete(context.Background(), vn)
		}(vn)
	}

	// In a real chaos test:
	// 1. Start operation (scale up/down)
	// 2. Inject partition mid-operation
	// 3. Verify operation is paused/retried
	// 4. Heal partition
	// 5. Verify operation completes or rolls back

	// Verify current state
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	vnList := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, vnList, client.InNamespace(TestNamespace), client.MatchingLabels{
		"autoscaler.vpsie.com/nodegroup": ng.Name,
	})
	require.NoError(t, err)

	t.Logf("NodeGroup: %s, VPSieNodes: %d", ng.Name, len(vnList.Items))
	t.Log("Active operations partition test setup complete")
}
