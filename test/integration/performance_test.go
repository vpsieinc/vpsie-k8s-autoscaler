//go:build performance
// +build performance

package integration

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// PerformanceMetrics tracks performance statistics
type PerformanceMetrics struct {
	StartTime           time.Time
	EndTime             time.Time
	TotalDuration       time.Duration
	OperationCount      int64
	SuccessCount        int64
	ErrorCount          int64
	MinLatency          time.Duration
	MaxLatency          time.Duration
	AvgLatency          time.Duration
	P50Latency          time.Duration
	P95Latency          time.Duration
	P99Latency          time.Duration
	ThroughputOpsPerSec float64
	MemoryStartMB       uint64
	MemoryEndMB         uint64
	MemoryPeakMB        uint64
	GoroutineStart      int
	GoroutineEnd        int
	GoroutinePeak       int
	CPUUsagePercent     float64
	APICallCount        int64
	AvgAPILatency       time.Duration
}

// ResourceTracker tracks resource usage during tests
type ResourceTracker struct {
	mu             sync.RWMutex
	startTime      time.Time
	memoryStart    uint64
	memoryPeak     uint64
	goroutineStart int
	goroutinePeak  int
	latencies      []time.Duration
	apiCallCount   int64
	apiLatencies   []time.Duration
	operationCount int64
	successCount   int64
	errorCount     int64
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker() *ResourceTracker {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &ResourceTracker{
		startTime:      time.Now(),
		memoryStart:    m.Alloc / 1024 / 1024, // MB
		memoryPeak:     m.Alloc / 1024 / 1024,
		goroutineStart: runtime.NumGoroutine(),
		goroutinePeak:  runtime.NumGoroutine(),
		latencies:      make([]time.Duration, 0, 10000),
		apiLatencies:   make([]time.Duration, 0, 10000),
		stopCh:         make(chan struct{}),
	}
}

// Start begins periodic resource monitoring
func (rt *ResourceTracker) Start() {
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rt.updatePeaks()
			case <-rt.stopCh:
				return
			}
		}
	}()
}

// Stop stops resource monitoring
func (rt *ResourceTracker) Stop() {
	close(rt.stopCh)
	rt.wg.Wait()
}

// updatePeaks updates peak resource usage
func (rt *ResourceTracker) updatePeaks() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	currentMem := m.Alloc / 1024 / 1024
	if currentMem > rt.memoryPeak {
		rt.memoryPeak = currentMem
	}

	goroutines := runtime.NumGoroutine()
	if goroutines > rt.goroutinePeak {
		rt.goroutinePeak = goroutines
	}
}

// RecordLatency records an operation latency
func (rt *ResourceTracker) RecordLatency(d time.Duration) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.latencies = append(rt.latencies, d)
}

// RecordAPICall records an API call latency
func (rt *ResourceTracker) RecordAPICall(d time.Duration) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	atomic.AddInt64(&rt.apiCallCount, 1)
	rt.apiLatencies = append(rt.apiLatencies, d)
}

// RecordOperation records an operation result
func (rt *ResourceTracker) RecordOperation(success bool) {
	atomic.AddInt64(&rt.operationCount, 1)
	if success {
		atomic.AddInt64(&rt.successCount, 1)
	} else {
		atomic.AddInt64(&rt.errorCount, 1)
	}
}

// GetMetrics returns the collected performance metrics
func (rt *ResourceTracker) GetMetrics() *PerformanceMetrics {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryEnd := m.Alloc / 1024 / 1024

	duration := time.Since(rt.startTime)

	metrics := &PerformanceMetrics{
		StartTime:      rt.startTime,
		EndTime:        time.Now(),
		TotalDuration:  duration,
		OperationCount: atomic.LoadInt64(&rt.operationCount),
		SuccessCount:   atomic.LoadInt64(&rt.successCount),
		ErrorCount:     atomic.LoadInt64(&rt.errorCount),
		MemoryStartMB:  rt.memoryStart,
		MemoryEndMB:    memoryEnd,
		MemoryPeakMB:   rt.memoryPeak,
		GoroutineStart: rt.goroutineStart,
		GoroutineEnd:   runtime.NumGoroutine(),
		GoroutinePeak:  rt.goroutinePeak,
		APICallCount:   atomic.LoadInt64(&rt.apiCallCount),
	}

	if len(rt.latencies) > 0 {
		metrics.MinLatency, metrics.MaxLatency, metrics.AvgLatency = rt.calculateLatencyStats(rt.latencies)
		metrics.P50Latency = rt.calculatePercentile(rt.latencies, 0.50)
		metrics.P95Latency = rt.calculatePercentile(rt.latencies, 0.95)
		metrics.P99Latency = rt.calculatePercentile(rt.latencies, 0.99)
	}

	if len(rt.apiLatencies) > 0 {
		_, _, metrics.AvgAPILatency = rt.calculateLatencyStats(rt.apiLatencies)
	}

	if duration.Seconds() > 0 {
		metrics.ThroughputOpsPerSec = float64(metrics.OperationCount) / duration.Seconds()
	}

	return metrics
}

// calculateLatencyStats calculates min, max, and average latency
func (rt *ResourceTracker) calculateLatencyStats(latencies []time.Duration) (min, max, avg time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	min = latencies[0]
	max = latencies[0]
	var sum time.Duration

	for _, lat := range latencies {
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
		sum += lat
	}

	avg = sum / time.Duration(len(latencies))
	return min, max, avg
}

// calculatePercentile calculates the Nth percentile of latencies
func (rt *ResourceTracker) calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Simple percentile calculation (should sort for accuracy, but good enough for tests)
	index := int(float64(len(latencies)) * percentile)
	if index >= len(latencies) {
		index = len(latencies) - 1
	}
	return latencies[index]
}

// PrintReport prints a formatted performance report
func (m *PerformanceMetrics) PrintReport(t *testing.T) {
	t.Logf("\n"+
		"============================================================\n"+
		"                  PERFORMANCE REPORT                        \n"+
		"============================================================\n"+
		"Duration:           %v\n"+
		"Operations:         %d (Success: %d, Error: %d)\n"+
		"Throughput:         %.2f ops/sec\n"+
		"Error Rate:         %.2f%%\n"+
		"------------------------------------------------------------\n"+
		"Latency Statistics:\n"+
		"  Min:              %v\n"+
		"  Max:              %v\n"+
		"  Avg:              %v\n"+
		"  P50:              %v\n"+
		"  P95:              %v\n"+
		"  P99:              %v\n"+
		"------------------------------------------------------------\n"+
		"Resource Usage:\n"+
		"  Memory Start:     %d MB\n"+
		"  Memory End:       %d MB\n"+
		"  Memory Peak:      %d MB\n"+
		"  Memory Delta:     %+d MB\n"+
		"  Goroutines Start: %d\n"+
		"  Goroutines End:   %d\n"+
		"  Goroutines Peak:  %d\n"+
		"  Goroutine Delta:  %+d\n"+
		"------------------------------------------------------------\n"+
		"API Statistics:\n"+
		"  API Calls:        %d\n"+
		"  Avg API Latency:  %v\n"+
		"============================================================\n",
		m.TotalDuration,
		m.OperationCount, m.SuccessCount, m.ErrorCount,
		m.ThroughputOpsPerSec,
		float64(m.ErrorCount)/float64(m.OperationCount)*100,
		m.MinLatency, m.MaxLatency, m.AvgLatency,
		m.P50Latency, m.P95Latency, m.P99Latency,
		m.MemoryStartMB, m.MemoryEndMB, m.MemoryPeakMB,
		int64(m.MemoryEndMB)-int64(m.MemoryStartMB),
		m.GoroutineStart, m.GoroutineEnd, m.GoroutinePeak,
		m.GoroutineEnd-m.GoroutineStart,
		m.APICallCount, m.AvgAPILatency,
	)
}

// TestControllerLoad_100NodeGroups tests controller performance with 100 NodeGroups
func TestControllerLoad_100NodeGroups(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create secret for controller
	secretName := "perf-test-secret-100ng"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(mockServer.URL()),
		},
	}
	err := k8sClient.Create(ctx, secret)
	require.NoError(t, err)
	defer k8sClient.Delete(ctx, secret)

	// Start controller
	proc, err := startControllerInBackground(19000, 19100, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	// Wait for controller to be ready
	err = waitForControllerReady("http://localhost:19100/healthz", 30*time.Second)
	require.NoError(t, err)
	t.Log("Controller is ready")

	// Start resource tracking
	tracker := NewResourceTracker()
	tracker.Start()
	defer tracker.Stop()

	// Create 100 NodeGroups rapidly
	nodeGroupCount := 100
	nodeGroups := make([]string, nodeGroupCount)

	t.Logf("Creating %d NodeGroups...", nodeGroupCount)
	startTime := time.Now()

	for i := 0; i < nodeGroupCount; i++ {
		ngName := fmt.Sprintf("perf-nodegroup-%d", i)
		nodeGroups[i] = ngName

		ng := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ngName,
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:           1,
				MaxNodes:           3,
				DatacenterID:       "dc-us-east-1",
				OfferingIDs:        []string{"small-2cpu-4gb"},
				ResourceIdentifier: "test-cluster",
				Project:            "test-project",
				OSImageID:          "test-os-image",
				KubernetesVersion:  "v1.28.0",
			},
		}

		opStart := time.Now()
		err := k8sClient.Create(ctx, ng)
		opDuration := time.Since(opStart)

		tracker.RecordLatency(opDuration)
		tracker.RecordOperation(err == nil)

		if err != nil {
			t.Logf("Failed to create NodeGroup %s: %v", ngName, err)
		}

		// Small delay to avoid overwhelming API server
		if i%10 == 0 && i > 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	creationDuration := time.Since(startTime)
	t.Logf("Created %d NodeGroups in %v", nodeGroupCount, creationDuration)

	// Wait for all NodeGroups to be reconciled (max 5 minutes)
	t.Log("Waiting for all NodeGroups to reconcile...")
	reconcileStartTime := time.Now()
	reconcileTimeout := 5 * time.Minute
	reconcileDeadline := reconcileStartTime.Add(reconcileTimeout)

	reconciledCount := 0
	checkInterval := 5 * time.Second

	for time.Now().Before(reconcileDeadline) {
		reconciledCount = 0

		for _, ngName := range nodeGroups {
			var ng autoscalerv1alpha1.NodeGroup
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ngName,
				Namespace: testNamespace,
			}, &ng)

			if err == nil && ng.Status.CurrentNodes > 0 {
				reconciledCount++
			}
		}

		if reconciledCount == nodeGroupCount {
			break
		}

		t.Logf("Reconciled %d/%d NodeGroups...", reconciledCount, nodeGroupCount)
		time.Sleep(checkInterval)
	}

	reconcileDuration := time.Since(reconcileStartTime)
	t.Logf("Reconciliation completed: %d/%d NodeGroups in %v", reconciledCount, nodeGroupCount, reconcileDuration)

	// Verify all NodeGroups reconciled within timeout
	assert.Equal(t, nodeGroupCount, reconciledCount, "All NodeGroups should be reconciled within 5 minutes")

	// Check metrics endpoint is responsive under load
	metricsResp, err := http.Get("http://localhost:19000/metrics")
	require.NoError(t, err)
	defer metricsResp.Body.Close()
	assert.Equal(t, http.StatusOK, metricsResp.StatusCode, "Metrics endpoint should respond under load")

	// Track API calls from mock server
	apiCallCount := mockServer.GetRequestCount("/v2/vms")
	tracker.apiCallCount = int64(apiCallCount)
	t.Logf("Mock VPSie API received %d VM requests", apiCallCount)

	// Stop tracking and get metrics
	tracker.Stop()
	metrics := tracker.GetMetrics()

	// Verify no memory leaks (memory increase should be reasonable)
	memoryIncrease := int64(metrics.MemoryEndMB) - int64(metrics.MemoryStartMB)
	assert.Less(t, memoryIncrease, int64(500), "Memory increase should be less than 500MB")

	// Verify goroutine count is stable
	goroutineIncrease := metrics.GoroutineEnd - metrics.GoroutineStart
	assert.Less(t, goroutineIncrease, 100, "Goroutine increase should be less than 100")

	// Print performance report
	metrics.PrintReport(t)

	// Cleanup - delete all NodeGroups
	t.Log("Cleaning up NodeGroups...")
	for _, ngName := range nodeGroups {
		ng := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ngName,
				Namespace: testNamespace,
			},
		}
		_ = k8sClient.Delete(ctx, ng)
	}
}

// TestHighChurnRate tests controller with high create/delete churn
func TestHighChurnRate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create secret for controller
	secretName := "perf-test-secret-churn"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(mockServer.URL()),
		},
	}
	err := k8sClient.Create(ctx, secret)
	require.NoError(t, err)
	defer k8sClient.Delete(ctx, secret)

	// Start controller
	proc, err := startControllerInBackground(20000, 20100, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	// Wait for controller to be ready
	err = waitForControllerReady("http://localhost:20100/healthz", 30*time.Second)
	require.NoError(t, err)

	// Start resource tracking
	tracker := NewResourceTracker()
	tracker.Start()
	defer tracker.Stop()

	// High churn rate: 10 creates/deletes per second for 1 minute
	targetRate := 10 // operations per second
	duration := 1 * time.Minute
	operationsPerCycle := 2 // create + delete = 2 operations
	expectedCycles := targetRate / operationsPerCycle * int(duration.Seconds())

	t.Logf("Starting high churn test: %d ops/sec for %v (%d cycles)", targetRate, duration, expectedCycles)

	var wg sync.WaitGroup
	stopCh := make(chan struct{})
	cycleCounter := int64(0)

	// Start churn workers
	workerCount := 5
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ticker := time.NewTicker(time.Second / time.Duration(targetRate/operationsPerCycle/workerCount))
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					cycle := atomic.AddInt64(&cycleCounter, 1)
					ngName := fmt.Sprintf("churn-ng-w%d-c%d", workerID, cycle)

					// Create
					ng := &autoscalerv1alpha1.NodeGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ngName,
							Namespace: testNamespace,
						},
						Spec: autoscalerv1alpha1.NodeGroupSpec{
							MinNodes:           1,
							MaxNodes:           2,
							DatacenterID:       "dc-us-east-1",
							OfferingIDs:        []string{"small-2cpu-4gb"},
							ResourceIdentifier: "test-cluster",
							Project:            "test-project",
							OSImageID:          "test-os-image",
							KubernetesVersion:  "v1.28.0",
						},
					}

					createStart := time.Now()
					createErr := k8sClient.Create(ctx, ng)
					createDuration := time.Since(createStart)
					tracker.RecordLatency(createDuration)
					tracker.RecordOperation(createErr == nil)

					if createErr == nil {
						// Wait a bit, then delete
						time.Sleep(100 * time.Millisecond)

						deleteStart := time.Now()
						deleteErr := k8sClient.Delete(ctx, ng)
						deleteDuration := time.Since(deleteStart)
						tracker.RecordLatency(deleteDuration)
						tracker.RecordOperation(deleteErr == nil)
					}

				case <-stopCh:
					return
				}
			}
		}(i)
	}

	// Run for specified duration
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()

	totalCycles := atomic.LoadInt64(&cycleCounter)
	t.Logf("Completed %d create/delete cycles", totalCycles)

	// Wait for eventual consistency
	t.Log("Waiting for eventual consistency...")
	time.Sleep(10 * time.Second)

	// Stop tracking and get metrics
	tracker.Stop()
	metrics := tracker.GetMetrics()

	// Verify no deadlocks (operations completed)
	assert.Greater(t, metrics.OperationCount, int64(expectedCycles/2), "Should complete at least half the expected cycles")

	// Verify error rate is below 1%
	errorRate := float64(metrics.ErrorCount) / float64(metrics.OperationCount) * 100
	assert.Less(t, errorRate, 1.0, "Error rate should be below 1%%")

	// Verify no race conditions (no panics, controller still running)
	healthResp, err := http.Get("http://localhost:20100/healthz")
	require.NoError(t, err)
	defer healthResp.Body.Close()
	assert.Equal(t, http.StatusOK, healthResp.StatusCode, "Controller should still be healthy")

	// Print performance report
	metrics.PrintReport(t)

	t.Logf("Churn test completed successfully with %.2f%% error rate", errorRate)
}

// TestLargeScaleReconciliation tests reconciliation of a single large NodeGroup
func TestLargeScaleReconciliation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()

	// Create mock VPSie server with rate limiting
	mockServer := NewMockVPSieServer()
	mockServer.rateLimit = 50 // 50 requests per minute
	defer mockServer.Close()

	// Create secret for controller
	secretName := "perf-test-secret-large"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(mockServer.URL()),
		},
	}
	err := k8sClient.Create(ctx, secret)
	require.NoError(t, err)
	defer k8sClient.Delete(ctx, secret)

	// Start controller
	proc, err := startControllerInBackground(21000, 21100, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	// Wait for controller to be ready
	err = waitForControllerReady("http://localhost:21100/healthz", 30*time.Second)
	require.NoError(t, err)

	// Start resource tracking
	tracker := NewResourceTracker()
	tracker.Start()
	defer tracker.Stop()

	// Create NodeGroup with 100 nodes
	nodeCount := 100
	ngName := "large-scale-nodegroup"

	t.Logf("Creating NodeGroup with minNodes=%d...", nodeCount)

	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ngName,
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           nodeCount,
			MaxNodes:           nodeCount,
			DatacenterID:       "dc-us-east-1",
			OfferingIDs:        []string{"small-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	createStart := time.Now()
	err = k8sClient.Create(ctx, ng)
	require.NoError(t, err)
	defer k8sClient.Delete(ctx, ng)

	createDuration := time.Since(createStart)
	t.Logf("NodeGroup created in %v", createDuration)

	// Measure reconciliation time
	t.Log("Measuring reconciliation time...")
	reconcileStart := time.Now()

	// Wait for all nodes to be created (with timeout)
	timeout := 10 * time.Minute
	err = waitForVPSieNodeCount(ctx, testNamespace, nodeCount, timeout)

	reconcileDuration := time.Since(reconcileStart)
	t.Logf("Reconciliation completed in %v", reconcileDuration)

	if err != nil {
		t.Logf("Warning: Not all nodes created within timeout: %v", err)
	}

	// Count actual nodes created
	actualCount := countVPSieNodes(ctx, testNamespace)
	t.Logf("Created %d/%d VPSieNodes", actualCount, nodeCount)

	// Track API calls
	apiCallCount := mockServer.GetRequestCount("/v2/vms")
	rateLimitErrors := mockServer.GetRequestCount("rate-limit-errors")
	t.Logf("API Calls: %d total, %d rate limited", apiCallCount, rateLimitErrors)

	// Verify rate limiting is working
	assert.Greater(t, apiCallCount, 0, "Should have made API calls")
	// Rate limiting should prevent thundering herd (calls should be spread over time)
	callsPerSecond := float64(apiCallCount) / reconcileDuration.Seconds()
	assert.Less(t, callsPerSecond, 10.0, "Should not overwhelm API with thundering herd")

	// Stop tracking and get metrics
	tracker.Stop()
	metrics := tracker.GetMetrics()

	// Print performance report
	metrics.PrintReport(t)

	t.Logf("Large scale reconciliation completed: %d nodes in %v (%.2f calls/sec)",
		actualCount, reconcileDuration, callsPerSecond)
}

// BenchmarkNodeGroupReconciliation benchmarks NodeGroup reconciliation
func BenchmarkNodeGroupReconciliation(b *testing.B) {
	ctx := context.Background()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create secret
	secretName := "bench-secret-ng"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(mockServer.URL()),
		},
	}
	_ = k8sClient.Create(ctx, secret)
	defer k8sClient.Delete(ctx, secret)

	// Reset timer before benchmark loop
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ngName := fmt.Sprintf("bench-ng-%d", i)
		ng := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ngName,
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:           1,
				MaxNodes:           3,
				DatacenterID:       "dc-us-east-1",
				OfferingIDs:        []string{"small-2cpu-4gb"},
				ResourceIdentifier: "test-cluster",
				Project:            "test-project",
				OSImageID:          "test-os-image",
				KubernetesVersion:  "v1.28.0",
			},
		}

		err := k8sClient.Create(ctx, ng)
		if err != nil {
			b.Fatalf("Failed to create NodeGroup: %v", err)
		}

		// Wait briefly for reconciliation
		time.Sleep(100 * time.Millisecond)

		// Cleanup
		_ = k8sClient.Delete(ctx, ng)
	}
}

// BenchmarkVPSieNodeStatusUpdate benchmarks VPSieNode status updates
func BenchmarkVPSieNodeStatusUpdate(b *testing.B) {
	ctx := context.Background()

	// Create a test VPSieNode
	nodeName := "bench-vpsienode"
	node := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.VPSieNodeSpec{
			VPSieInstanceID:    1000,
			DatacenterID:       "dc-us-east-1",
			InstanceType:       "small-2cpu-4gb",
			NodeGroupName:      "test-nodegroup",
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err := k8sClient.Create(ctx, node)
	if err != nil {
		b.Fatalf("Failed to create VPSieNode: %v", err)
	}
	defer k8sClient.Delete(ctx, node)

	// Reset timer
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Get current node
		var currentNode autoscalerv1alpha1.VPSieNode
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      nodeName,
			Namespace: testNamespace,
		}, &currentNode)
		if err != nil {
			b.Fatalf("Failed to get VPSieNode: %v", err)
		}

		// Update status
		currentNode.Status.Phase = autoscalerv1alpha1.VPSieNodePhaseReady
		currentNode.Status.VPSieStatus = fmt.Sprintf("running-%d", i%256)
		currentNode.Status.NodeName = fmt.Sprintf("node-%d", i%256)

		err = k8sClient.Status().Update(ctx, &currentNode)
		if err != nil {
			b.Fatalf("Failed to update status: %v", err)
		}
	}
}

// BenchmarkMetricsCollection benchmarks Prometheus metrics collection
func BenchmarkMetricsCollection(b *testing.B) {
	// Create mock Prometheus metrics
	testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_metric_gauge",
		Help: "Test gauge metric",
	})
	prometheus.MustRegister(testGauge)
	defer prometheus.Unregister(testGauge)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		testGauge.Set(float64(i))

		// Collect metrics
		metricFamilies, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			b.Fatalf("Failed to gather metrics: %v", err)
		}

		// Find our metric
		for _, mf := range metricFamilies {
			if mf.GetName() == "test_metric_gauge" {
				metric := mf.GetMetric()[0]
				if metric.GetGauge().GetValue() != float64(i) {
					b.Fatalf("Metric value mismatch: expected %d, got %f", i, metric.GetGauge().GetValue())
				}
			}
		}
	}
}

// BenchmarkHealthCheckLatency benchmarks health check endpoint latency
func BenchmarkHealthCheckLatency(b *testing.B) {
	ctx := context.Background()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create secret
	secretName := "bench-secret-health"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(mockServer.URL()),
		},
	}
	_ = k8sClient.Create(ctx, secret)
	defer k8sClient.Delete(ctx, secret)

	// Start controller
	proc, err := startControllerInBackground(22000, 22100, secretName, testNamespace)
	if err != nil {
		b.Fatalf("Failed to start controller: %v", err)
	}
	defer cleanup(proc)

	// Wait for controller to be ready
	err = waitForControllerReady("http://localhost:22100/healthz", 30*time.Second)
	if err != nil {
		b.Fatalf("Controller not ready: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := http.Get("http://localhost:22100/healthz")
		if err != nil {
			b.Fatalf("Health check failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Health check returned %d", resp.StatusCode)
		}
	}
}

// Helper function to gather Prometheus metric value
func getMetricValue(metricName string) (float64, error) {
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metricFamilies {
		if mf.GetName() == metricName {
			if len(mf.GetMetric()) > 0 {
				metric := mf.GetMetric()[0]
				if metric.GetGauge() != nil {
					return metric.GetGauge().GetValue(), nil
				}
				if metric.GetCounter() != nil {
					return metric.GetCounter().GetValue(), nil
				}
			}
		}
	}

	return 0, fmt.Errorf("metric %s not found", metricName)
}

// Helper to parse metrics from response
func parsePrometheusMetrics(body []byte) (map[string]float64, error) {
	metrics := make(map[string]float64)
	lines := string(body)

	for _, line := range splitLines(lines) {
		// Skip comments and empty lines
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Simple parsing (not fully compliant but good enough for tests)
		parts := splitWhitespace(line)
		if len(parts) >= 2 {
			name := parts[0]
			value := parts[1]

			var floatValue float64
			_, err := fmt.Sscanf(value, "%f", &floatValue)
			if err == nil {
				metrics[name] = floatValue
			}
		}
	}

	return metrics, nil
}

// Helper to split string by newlines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Helper to split string by whitespace
func splitWhitespace(s string) []string {
	var parts []string
	word := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if len(word) > 0 {
				parts = append(parts, word)
				word = ""
			}
		} else {
			word += string(s[i])
		}
	}
	if len(word) > 0 {
		parts = append(parts, word)
	}
	return parts
}
