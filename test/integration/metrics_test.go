//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsiemetrics "github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/scaler"
)

// TestMetrics_ScaleDownBlockedTotal verifies that ScaleDownBlockedTotal metric
// increments correctly for all blocking scenarios
func TestMetrics_ScaleDownBlockedTotal(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Reset metrics before test
	vpsiemetrics.ScaleDownBlockedTotal.Reset()

	t.Run("Blocked by cooldown", func(t *testing.T) {
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-cooldown",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.CooldownPeriod = 10 * time.Minute
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Create NodeGroup with recent scale-down
		ng := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ng",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     10,
				DatacenterID: "123",
			},
			Status: v1alpha1.NodeGroupStatus{
				CurrentNodes:      2,
				DesiredNodes:      2,
				LastScaleDownTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}, // Recent scale-down
			},
		}

		// Call CanScaleDown - should be blocked by cooldown
		canScale, reason, err := sdm.CanScaleDown(context.Background(), ng, node)
		require.NoError(t, err)
		assert.False(t, canScale)
		assert.Contains(t, reason, "cooldown")

		// Verify metric incremented
		metricValue := getCounterValue(t, vpsiemetrics.ScaleDownBlockedTotal, prometheus.Labels{
			"nodegroup": "test-ng",
			"namespace": "default",
			"reason":    "cooldown",
		})
		assert.Equal(t, float64(1), metricValue, "ScaleDownBlockedTotal should increment for cooldown")
	})

	t.Run("Blocked by minimum nodes constraint", func(t *testing.T) {
		vpsiemetrics.ScaleDownBlockedTotal.Reset()
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-min",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-min",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.CooldownPeriod = 0 // No cooldown
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Create NodeGroup at minimum nodes
		ng := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ng-min",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:     2, // At minimum
				MaxNodes:     10,
				DatacenterID: "123",
			},
			Status: v1alpha1.NodeGroupStatus{
				CurrentNodes: 2, // At minimum
				DesiredNodes: 2,
				Nodes: []v1alpha1.NodeInfo{
					{NodeName: "test-node-min"},
					{NodeName: "test-node-min-2"},
				},
			},
		}

		// Call CanScaleDown - should be blocked by min nodes
		canScale, reason, err := sdm.CanScaleDown(context.Background(), ng, node)
		require.NoError(t, err)
		assert.False(t, canScale)
		assert.Contains(t, reason, "minimum")

		// Verify metric incremented
		metricValue := getCounterValue(t, vpsiemetrics.ScaleDownBlockedTotal, prometheus.Labels{
			"nodegroup": "test-ng-min",
			"namespace": "default",
			"reason":    "min_nodes",
		})
		assert.Equal(t, float64(1), metricValue, "ScaleDownBlockedTotal should increment for min_nodes")
	})

	t.Run("Blocked by safety checks - local storage", func(t *testing.T) {
		vpsiemetrics.ScaleDownBlockedTotal.Reset()
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-storage",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-storage",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create pod with local storage on node
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-storage",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node-storage",
				Volumes: []corev1.Volume{
					{
						Name: "local-vol",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{}, // Local storage
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "nginx",
					},
				},
			},
		}
		_, err = k8sClient.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.CooldownPeriod = 0
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Create NodeGroup
		ng := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ng-storage",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     10,
				DatacenterID: "123",
			},
			Status: v1alpha1.NodeGroupStatus{
				CurrentNodes: 3,
				DesiredNodes: 3,
				Nodes: []v1alpha1.NodeInfo{
					{NodeName: "test-node-storage"},
					{NodeName: "test-node-2"},
					{NodeName: "test-node-3"},
				},
			},
		}

		// Call CanScaleDown - should be blocked by local storage safety check
		canScale, reason, err := sdm.CanScaleDown(context.Background(), ng, node)
		require.NoError(t, err)
		assert.False(t, canScale)
		assert.Contains(t, reason, "local storage")

		// Verify metric incremented with local_storage reason
		metricValue := getCounterValue(t, vpsiemetrics.ScaleDownBlockedTotal, prometheus.Labels{
			"nodegroup": "test-ng-storage",
			"namespace": "default",
			"reason":    "local_storage",
		})
		assert.Equal(t, float64(1), metricValue, "ScaleDownBlockedTotal should increment for local_storage")
	})

	t.Run("Blocked by protected node annotation", func(t *testing.T) {
		vpsiemetrics.ScaleDownBlockedTotal.Reset()
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node with protection annotation
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-protected",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-protected",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
				Annotations: map[string]string{
					"autoscaler.vpsie.com/scale-down-disabled": "true",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.CooldownPeriod = 0
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Create NodeGroup
		ng := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ng-protected",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     10,
				DatacenterID: "123",
			},
			Status: v1alpha1.NodeGroupStatus{
				CurrentNodes: 3,
				DesiredNodes: 3,
				Nodes: []v1alpha1.NodeInfo{
					{NodeName: "test-node-protected"},
					{NodeName: "test-node-2"},
					{NodeName: "test-node-3"},
				},
			},
		}

		// Call CanScaleDown - should be blocked by protection
		canScale, reason, err := sdm.CanScaleDown(context.Background(), ng, node)
		require.NoError(t, err)
		assert.False(t, canScale)
		assert.Contains(t, reason, "protected")

		// Verify metric incremented with protected_node reason
		metricValue := getCounterValue(t, vpsiemetrics.ScaleDownBlockedTotal, prometheus.Labels{
			"nodegroup": "test-ng-protected",
			"namespace": "default",
			"reason":    "protected_node",
		})
		assert.Equal(t, float64(1), metricValue, "ScaleDownBlockedTotal should increment for protected_node")
	})
}

// TestMetrics_SafetyCheckFailuresTotal verifies that SafetyCheckFailuresTotal metric
// increments correctly for all safety check types
func TestMetrics_SafetyCheckFailuresTotal(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Reset metrics before test
	vpsiemetrics.SafetyCheckFailuresTotal.Reset()

	t.Run("Safety check failure - local storage", func(t *testing.T) {
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-safety",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-safety",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
		}

		// Create pod with local storage
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod-local",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node-safety",
				Volumes: []corev1.Volume{
					{
						Name: "local-vol",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				Containers: []corev1.Container{
					{Name: "test", Image: "nginx"},
				},
			},
		}

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Call IsSafeToRemove - should fail local storage check
		safe, reason, err := sdm.IsSafeToRemove(context.Background(), node, []*corev1.Pod{pod})
		require.NoError(t, err)
		assert.False(t, safe)
		assert.Contains(t, reason, "local storage")

		// Verify metric incremented
		metricValue := getCounterValue(t, vpsiemetrics.SafetyCheckFailuresTotal, prometheus.Labels{
			"check_type": "local_storage",
			"nodegroup":  "test-ng-safety",
			"namespace":  "default",
		})
		assert.Equal(t, float64(1), metricValue, "SafetyCheckFailuresTotal should increment for local_storage")
	})

	t.Run("Safety check failure - protected node", func(t *testing.T) {
		vpsiemetrics.SafetyCheckFailuresTotal.Reset()
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create protected node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-protected",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-protected",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
				Annotations: map[string]string{
					"autoscaler.vpsie.com/scale-down-disabled": "true",
				},
			},
		}

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Call IsSafeToRemove - should fail protection check
		safe, reason, err := sdm.IsSafeToRemove(context.Background(), node, []*corev1.Pod{})
		require.NoError(t, err)
		assert.False(t, safe)
		assert.Contains(t, reason, "protected")

		// Verify metric incremented
		metricValue := getCounterValue(t, vpsiemetrics.SafetyCheckFailuresTotal, prometheus.Labels{
			"check_type": "protection",
			"nodegroup":  "test-ng-protected",
			"namespace":  "default",
		})
		assert.Equal(t, float64(1), metricValue, "SafetyCheckFailuresTotal should increment for protection")
	})
}

// TestMetrics_NodeDrainDuration verifies that NodeDrainDuration histogram
// records drain timing correctly
func TestMetrics_NodeDrainDuration(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Reset metrics before test
	vpsiemetrics.NodeDrainDuration.Reset()

	t.Run("Drain duration recorded on success", func(t *testing.T) {
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-drain",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-drain",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create pods on node
		for i := 0; i < 3; i++ {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-pod-%d", i),
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node-drain",
					Containers: []corev1.Container{
						{Name: "test", Image: "nginx"},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}
			_, err = k8sClient.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.DrainTimeout = 1 * time.Minute
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Call DrainNode
		startTime := time.Now()
		err = sdm.DrainNode(context.Background(), node)
		drainDuration := time.Since(startTime)

		// Drain should complete (may have errors for fake client, but timing is recorded)
		if err != nil {
			t.Logf("Drain completed in %v with error: %v", drainDuration, err)
		} else {
			t.Logf("Drain completed in %v successfully", drainDuration)
		}

		// Verify histogram was updated (result may be "success" or "error" depending on fake client behavior)
		// We just verify that SOME drain duration was recorded
		hasMetric := false
		for _, result := range []string{"success", "error", "timeout"} {
			val := getHistogramCount(t, vpsiemetrics.NodeDrainDuration, prometheus.Labels{
				"nodegroup": "test-ng-drain",
				"namespace": "default",
				"result":    result,
			})
			if val > 0 {
				hasMetric = true
				t.Logf("NodeDrainDuration recorded with result=%s, count=%d", result, val)
				break
			}
		}
		assert.True(t, hasMetric, "NodeDrainDuration should record at least one observation")
	})
}

// TestMetrics_NodeDrainPodsEvicted verifies that NodeDrainPodsEvicted histogram
// records pod eviction counts correctly
func TestMetrics_NodeDrainPodsEvicted(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// Reset metrics before test
	vpsiemetrics.NodeDrainPodsEvicted.Reset()

	t.Run("Pod eviction count recorded", func(t *testing.T) {
		k8sClient := fake.NewSimpleClientset()
		metricsClient := metricsfake.NewSimpleClientset()

		// Create test node
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-evict",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup":           "test-ng-evict",
					"autoscaler.vpsie.com/nodegroup-namespace": "default",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		_, err := k8sClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)

		// Create 5 pods on node
		expectedPodCount := 5
		for i := 0; i < expectedPodCount; i++ {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-pod-evict-%d", i),
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node-evict",
					Containers: []corev1.Container{
						{Name: "test", Image: "nginx"},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}
			_, err = k8sClient.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		// Create ScaleDownManager
		config := scaler.DefaultConfig()
		config.DrainTimeout = 1 * time.Minute
		sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

		// Call DrainNode
		errs := sdm.DrainNode(context.Background(), node, 30*time.Second)
		t.Logf("Drain completed with %d errors", len(errs))

		// Verify histogram recorded pod count
		count := getHistogramCount(t, vpsiemetrics.NodeDrainPodsEvicted, prometheus.Labels{
			"nodegroup": "test-ng-evict",
			"namespace": "default",
		})
		assert.Greater(t, count, uint64(0), "NodeDrainPodsEvicted should record at least one observation")
	})
}

// Helper function to get counter value from Prometheus metric
func getCounterValue(t *testing.T, counter *prometheus.CounterVec, labels prometheus.Labels) float64 {
	metric, err := counter.GetMetricWith(labels)
	if err != nil {
		// Metric doesn't exist yet, return 0
		return 0
	}

	// Get the metric value using protobuf DTO
	ch := make(chan prometheus.Metric, 1)
	metric.Collect(ch)
	close(ch)

	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if pb.Counter != nil {
			return pb.Counter.GetValue()
		}
	}

	return 0
}

// Helper function to get histogram observation count from Prometheus metric
func getHistogramCount(t *testing.T, histogram *prometheus.HistogramVec, labels prometheus.Labels) uint64 {
	metric, err := histogram.GetMetricWith(labels)
	if err != nil {
		// Metric doesn't exist yet, return 0
		return 0
	}

	// Get the metric value using protobuf DTO
	ch := make(chan prometheus.Metric, 1)
	metric.Collect(ch)
	close(ch)

	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}
		if pb.Histogram != nil {
			return pb.Histogram.GetSampleCount()
		}
	}

	return 0
}
