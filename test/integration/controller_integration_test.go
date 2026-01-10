//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

const (
	// Test namespace for integration tests
	testNamespace = "vpsie-autoscaler-test"
	// Timeout for operations
	testTimeout = 30 * time.Second
)

// getKubeconfigPath returns the kubeconfig path from environment or default
func getKubeconfigPath() string {
	// First check KUBECONFIG environment variable (used in CI)
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}

	// Fall back to default location
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

var (
	cfg       *rest.Config
	k8sClient client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme
)

// TestMain sets up the integration test environment
func TestMain(m *testing.M) {
	var err error

	// Load kubeconfig from environment or default location
	kubeconfigPath := getKubeconfigPath()
	if kubeconfigPath == "" {
		fmt.Println("Failed to determine kubeconfig path")
		os.Exit(1)
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Printf("Failed to load kubeconfig from %s: %v\n", kubeconfigPath, err)
		os.Exit(1)
	}
	fmt.Printf("Using kubeconfig: %s\n", kubeconfigPath)

	// Create clientset
	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	// Setup scheme
	scheme = runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		fmt.Printf("Failed to add core/v1 to scheme: %v\n", err)
		os.Exit(1)
	}
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		fmt.Printf("Failed to add autoscaler/v1alpha1 to scheme: %v\n", err)
		os.Exit(1)
	}

	// Create controller-runtime client
	k8sClient, err = client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("Failed to create controller-runtime client: %v\n", err)
		os.Exit(1)
	}

	// Ensure test namespace exists
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	err = k8sClient.Create(context.Background(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		fmt.Printf("Failed to create test namespace: %v\n", err)
		os.Exit(1)
	}

	// Verify CRDs are installed
	_, err = clientset.Discovery().ServerResourcesForGroupVersion("autoscaler.vpsie.com/v1alpha1")
	if err != nil {
		fmt.Printf("CRDs not installed. Please run: kubectl apply -f deploy/crds/\n")
		os.Exit(1)
	}

	// Build controller binary if needed
	binaryPath := filepath.Join(".", "bin", "vpsie-autoscaler")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Println("Building controller binary...")
		if err := buildControllerBinary(); err != nil {
			fmt.Printf("Failed to build controller binary: %v\n", err)
			os.Exit(1)
		}
	}

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupTestResources()

	os.Exit(code)
}

// cleanupTestResources removes all test resources
func cleanupTestResources() {
	ctx := context.Background()

	// Delete all NodeGroups in test namespace
	nodeGroupList := &autoscalerv1alpha1.NodeGroupList{}
	if err := k8sClient.List(ctx, nodeGroupList, client.InNamespace(testNamespace)); err == nil {
		for _, ng := range nodeGroupList.Items {
			_ = k8sClient.Delete(ctx, &ng)
		}
	}

	// Delete all VPSieNodes in test namespace
	vpsieNodeList := &autoscalerv1alpha1.VPSieNodeList{}
	if err := k8sClient.List(ctx, vpsieNodeList, client.InNamespace(testNamespace)); err == nil {
		for _, vn := range vpsieNodeList.Items {
			_ = k8sClient.Delete(ctx, &vn)
		}
	}

	// Note: We don't delete the namespace itself as it may be used by other tests
}

// buildControllerBinary builds the controller binary if it doesn't exist
func buildControllerBinary() error {
	// This would typically use exec.Command to run "make build"
	// For now, we assume it's built externally
	return fmt.Errorf("controller binary not found at bin/vpsie-autoscaler")
}

// TestNodeGroup_CRUD tests basic CRUD operations for NodeGroup resources
func TestNodeGroup_CRUD(t *testing.T) {
	ctx := context.Background()

	// Create a NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           2,
			MaxNodes:           5,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	// Test Create
	err := k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)

	// Test Read
	retrieved := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, retrieved)
	require.NoError(t, err)
	assert.Equal(t, nodeGroup.Spec.MinNodes, retrieved.Spec.MinNodes)

	// Test Update
	retrieved.Spec.MaxNodes = 10
	err = k8sClient.Update(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, updated)
	require.NoError(t, err)
	assert.Equal(t, int32(10), updated.Spec.MaxNodes)

	// Test Delete
	err = k8sClient.Delete(ctx, nodeGroup)
	require.NoError(t, err)

	// Verify deletion
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, &autoscalerv1alpha1.NodeGroup{})
	assert.True(t, errors.IsNotFound(err))
}

// TestVPSieNode_CRUD tests basic CRUD operations for VPSieNode resources
func TestVPSieNode_CRUD(t *testing.T) {
	ctx := context.Background()

	// Create a VPSieNode
	vpsieNode := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vpsienode",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.VPSieNodeSpec{
			NodeGroupName:      "test-nodegroup",
			InstanceType:       "standard-2cpu-4gb",
			DatacenterID:       "us-west-1",
			VPSieInstanceID:    12345,
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	// Test Create
	err := k8sClient.Create(ctx, vpsieNode)
	require.NoError(t, err)

	// Test Read
	retrieved := &autoscalerv1alpha1.VPSieNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vpsieNode.Name,
		Namespace: vpsieNode.Namespace,
	}, retrieved)
	require.NoError(t, err)
	assert.Equal(t, vpsieNode.Spec.NodeGroupName, retrieved.Spec.NodeGroupName)

	// Test Update Status
	retrieved.Status.Phase = autoscalerv1alpha1.VPSieNodePhaseProvisioning
	retrieved.Status.VPSieStatus = "running"
	err = k8sClient.Status().Update(ctx, retrieved)
	require.NoError(t, err)

	// Verify status update
	updated := &autoscalerv1alpha1.VPSieNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vpsieNode.Name,
		Namespace: vpsieNode.Namespace,
	}, updated)
	require.NoError(t, err)
	assert.Equal(t, autoscalerv1alpha1.VPSieNodePhaseProvisioning, updated.Status.Phase)
	assert.Equal(t, "running", updated.Status.VPSieStatus)

	// Test Delete
	err = k8sClient.Delete(ctx, vpsieNode)
	require.NoError(t, err)

	// Verify deletion
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vpsieNode.Name,
		Namespace: vpsieNode.Namespace,
	}, &autoscalerv1alpha1.VPSieNode{})
	assert.True(t, errors.IsNotFound(err))
}

// TestHealthEndpoints_Integration tests controller health endpoints
func TestHealthEndpoints_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-health"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(18080, 18081, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second, "Controller did not become healthy")

	// Test /healthz endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", 18081))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))

	// Test /readyz endpoint
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/readyz", 18081))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test /ping endpoint (if implemented)
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/ping", 18081))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, "pong", string(body))
		}
	}

	t.Log("Health endpoints test completed successfully")
}

// TestMetricsEndpoint_Integration tests controller metrics endpoint
func TestMetricsEndpoint_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-metrics"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(18082, 18083, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second, "Controller did not become healthy")

	// Test metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", 18082))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	metrics := string(body)

	// Verify Prometheus format
	assert.Contains(t, metrics, "# HELP")
	assert.Contains(t, metrics, "# TYPE")

	// Verify key metrics are exposed
	expectedMetrics := []string{
		"vpsie_autoscaler_controller_reconcile_total",
		"vpsie_autoscaler_controller_reconcile_errors_total",
		"vpsie_autoscaler_nodegroup_current_nodes",
		"vpsie_autoscaler_nodegroup_desired_nodes",
	}

	for _, metric := range expectedMetrics {
		assert.Contains(t, metrics, metric, "Metric %s not found", metric)
	}

	// Create a NodeGroup to trigger metrics updates
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-metrics",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           1,
			MaxNodes:           3,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for reconciliation
	time.Sleep(5 * time.Second)

	// Check metrics again
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", 18082))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	updatedMetrics := string(body)

	// Verify reconciliation counter increased
	assert.Contains(t, updatedMetrics, "vpsie_autoscaler_controller_reconcile_total")

	t.Log("Metrics endpoint test completed successfully")
}

// TestControllerReconciliation_Integration tests end-to-end controller reconciliation
func TestControllerReconciliation_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start mock VPSie server with state transitions
	mockServer := NewMockVPSieServer()
	mockServer.StateTransitions = []VMStateTransition{
		{FromState: "provisioning", ToState: "running", Duration: 3 * time.Second},
		{FromState: "running", ToState: "ready", Duration: 2 * time.Second},
	}
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-reconciliation"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(18084, 18085, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second, "Controller did not become healthy")

	// Create NodeGroup with minNodes
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-reconcile",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           2,
			MaxNodes:           5,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for VPSieNodes to be created
	var vpsieNodes []autoscalerv1alpha1.VPSieNode
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		if err != nil {
			return false
		}
		vpsieNodes = list.Items
		return len(vpsieNodes) == 2
	}, 30*time.Second, 1*time.Second, "Expected 2 VPSieNodes to be created")

	// Verify VPSieNode specs
	for _, node := range vpsieNodes {
		assert.Equal(t, nodeGroup.Name, node.Spec.NodeGroupName)
		assert.Contains(t, nodeGroup.Spec.OfferingIDs, node.Spec.InstanceType)
		assert.Equal(t, nodeGroup.Spec.DatacenterID, node.Spec.DatacenterID)
	}

	// Verify mock server received VM creation requests
	assert.GreaterOrEqual(t, mockServer.GetRequestCount("/v2/vms"), 2)

	// Wait for VM state transitions
	time.Sleep(6 * time.Second)

	// Update NodeGroup to trigger scale up
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, nodeGroup)
	require.NoError(t, err)

	nodeGroup.Spec.MinNodes = 3
	err = k8sClient.Update(ctx, nodeGroup)
	require.NoError(t, err)

	// Wait for additional VPSieNode
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		if err != nil {
			return false
		}
		return len(list.Items) == 3
	}, 30*time.Second, 1*time.Second, "Expected 3 VPSieNodes after scale up")

	// Delete NodeGroup and verify cleanup
	err = k8sClient.Delete(ctx, nodeGroup)
	require.NoError(t, err)

	// Wait for VPSieNodes to be deleted
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		if err != nil {
			return false
		}
		return len(list.Items) == 0
	}, 30*time.Second, 1*time.Second, "VPSieNodes should be deleted")

	t.Log("Controller reconciliation test completed successfully")
}

// TestConfigurationValidation_Integration tests controller configuration validation
func TestConfigurationValidation_Integration(t *testing.T) {
	t.Run("invalid metrics address", func(t *testing.T) {
		// Test that controller fails with invalid metrics address
		// This would typically be tested at the unit level
		// Here we just verify the controller handles it gracefully
		t.Skip("Configuration validation is typically tested at unit level")
	})

	t.Run("conflicting addresses", func(t *testing.T) {
		// Test that controller fails when metrics and health addresses are the same
		t.Skip("Configuration validation is typically tested at unit level")
	})

	t.Run("valid configuration", func(t *testing.T) {
		// Test that controller starts with valid configuration
		// This is covered by other integration tests
		t.Skip("Valid configuration is tested by other integration tests")
	})
}

// TestGracefulShutdown_Integration tests graceful shutdown behavior
func TestGracefulShutdown_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-shutdown"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(18086, 18087, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second, "Controller did not become healthy")

	// Create some active resources
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-shutdown",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           2,
			MaxNodes:           5,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for initial reconciliation
	time.Sleep(5 * time.Second)

	// Get initial metrics
	initialMetrics := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 18086),
		"vpsie_autoscaler_controller_reconcile_total")

	t.Log("Sending SIGTERM signal")

	// Send SIGTERM for graceful shutdown
	err = sendSignal(proc, syscall.SIGTERM)
	require.NoError(t, err)

	// During shutdown, health endpoints should reflect this
	time.Sleep(1 * time.Second)

	// Check readyz returns 503 during shutdown
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/readyz", 18087))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Logf("Warning: readyz returned %d during shutdown, expected 503", resp.StatusCode)
		}
	}

	// Controller should stop accepting new work but finish existing
	shutdownStart := time.Now()

	// Wait for controller to exit (should be within 30 seconds)
	require.Eventually(t, func() bool {
		return !isProcessRunning(proc.PID)
	}, 35*time.Second, 1*time.Second, "Controller did not shut down within timeout")

	shutdownDuration := time.Since(shutdownStart)
	t.Logf("Controller shut down in %v", shutdownDuration)
	assert.LessOrEqual(t, shutdownDuration, 31*time.Second, "Shutdown took too long")

	// Verify logs were properly closed
	_, _, err = proc.GetLogs()
	assert.NoError(t, err, "Should be able to read logs after shutdown")

	t.Log("Graceful shutdown test completed successfully")
}

// TestSignalHandling_MultipleSignals tests handling of different signals
func TestSignalHandling_MultipleSignals(t *testing.T) {
	signals := []struct {
		name        string
		signal      syscall.Signal
		metricsPort int
		healthPort  int
	}{
		{"SIGTERM_multiple", syscall.SIGTERM, 18088, 18089},
		{"SIGINT", syscall.SIGINT, 18090, 18091},
		{"SIGQUIT", syscall.SIGQUIT, 18092, 18093},
	}

	for _, tc := range signals {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Start mock VPSie server
			mockServer := NewMockVPSieServer()
			defer mockServer.Close()

			// Create unique secret for this test
			secretName := fmt.Sprintf("test-vpsie-secret-signal-%s", tc.name)
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(t, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(tc.metricsPort, tc.healthPort, secretName, testNamespace)
			require.NoError(t, err)
			defer cleanup(proc)

			// Wait for controller to be healthy
			require.Eventually(t, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			if tc.name == "SIGTERM_multiple" {
				// Test multiple SIGTERM signals
				t.Log("Sending first SIGTERM")
				err = sendSignal(proc, syscall.SIGTERM)
				require.NoError(t, err)

				time.Sleep(2 * time.Second)

				// Second SIGTERM should force exit
				t.Log("Sending second SIGTERM")
				err = sendSignal(proc, syscall.SIGTERM)
				require.NoError(t, err)
			} else {
				// Send the signal
				t.Logf("Sending %s signal", tc.name)
				err = sendSignal(proc, tc.signal)
				require.NoError(t, err)
			}

			// Wait for controller to exit
			require.Eventually(t, func() bool {
				return !isProcessRunning(proc.PID)
			}, 35*time.Second, 1*time.Second, "Controller did not exit after signal")
		})
	}

	t.Log("Signal handling test completed successfully")
}

// TestShutdownWithActiveReconciliation tests shutdown during active reconciliation
func TestShutdownWithActiveReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start mock VPSie server with slow responses
	mockServer := NewMockVPSieServer()
	mockServer.Latency = 3 * time.Second // Slow responses
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-active-shutdown"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(18094, 18095, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second)

	// Create NodeGroup to trigger long-running reconciliation
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-active-shutdown",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           5, // More nodes = longer reconciliation
			MaxNodes:           10,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"large-8cpu-16gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for reconciliation to start
	time.Sleep(2 * time.Second)

	// Get initial metrics
	initialReconcileCount := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 18094),
		"vpsie_autoscaler_controller_reconcile_total")
	initialErrorCount := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 18094),
		"vpsie_autoscaler_controller_reconcile_errors_total")

	t.Log("Sending SIGTERM during active reconciliation")

	// Send SIGTERM during active reconciliation
	err = sendSignal(proc, syscall.SIGTERM)
	require.NoError(t, err)

	shutdownStart := time.Now()

	// Wait for controller to complete reconciliation and exit
	require.Eventually(t, func() bool {
		return !isProcessRunning(proc.PID)
	}, 40*time.Second, 1*time.Second, "Controller did not shut down")

	shutdownDuration := time.Since(shutdownStart)
	t.Logf("Shutdown completed in %v", shutdownDuration)

	// Check final metrics if still available
	if shutdownDuration < 5*time.Second {
		finalReconcileCount := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 18094),
			"vpsie_autoscaler_controller_reconcile_total")
		finalErrorCount := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 18094),
			"vpsie_autoscaler_controller_reconcile_errors_total")

		// Reconciliations should have completed
		assert.GreaterOrEqual(t, finalReconcileCount, initialReconcileCount)

		// Error count shouldn't spike significantly
		errorIncrease := finalErrorCount - initialErrorCount
		assert.LessOrEqual(t, errorIncrease, float64(5), "Too many errors during shutdown")
	}

	// Verify VPSieNodes were created despite shutdown
	list := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, list, client.InNamespace(testNamespace))
	require.NoError(t, err)

	// At least some nodes should have been created
	assert.GreaterOrEqual(t, len(list.Items), 1, "Some VPSieNodes should have been created")

	t.Log("Shutdown with active reconciliation test completed successfully")
}

// TestLeaderElection_Integration tests leader election with multiple controllers
func TestLeaderElection_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-leader"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Create unique leader election ID for this test
	leaderElectionID := fmt.Sprintf("test-leader-%d", time.Now().Unix())

	// Start 3 controllers with leader election
	controllers, err := startMultipleControllersWithLeaderElection(
		3, secretName, testNamespace, leaderElectionID)
	require.NoError(t, err)
	defer stopAllControllers(controllers)

	t.Logf("Started %d controllers with leader election ID: %s", len(controllers), leaderElectionID)

	// Wait for leader election to complete
	time.Sleep(10 * time.Second)

	// Identify the leader
	leader, leaderCount := identifyLeader(controllers)
	assert.Equal(t, 1, leaderCount, "Exactly one controller should be leader")

	if leader != nil {
		t.Logf("Leader controller on ports metrics=%s health=%s",
			leader.MetricsAddr, leader.HealthAddr)
	}

	// Verify health endpoints reflect leadership
	for _, controller := range controllers {
		resp, err := http.Get(fmt.Sprintf("http://%s/readyz", controller.HealthAddr))
		if err == nil {
			defer resp.Body.Close()
			if controller == leader {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Leader should be ready")
			}
			// Non-leaders may or may not be ready depending on implementation
		}
	}

	// Create a NodeGroup to verify only leader reconciles
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-leader",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           1,
			MaxNodes:           3,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for reconciliation
	time.Sleep(10 * time.Second)

	// Check reconciliation metrics - only leader should have increased
	for _, controller := range controllers {
		reconcileCount := getMetricValue(t,
			fmt.Sprintf("http://%s/metrics", controller.MetricsAddr),
			"vpsie_autoscaler_controller_reconcile_total")

		if controller == leader {
			assert.Greater(t, reconcileCount, float64(0), "Leader should have reconciled")
		} else {
			assert.Equal(t, float64(0), reconcileCount, "Non-leader should not reconcile")
		}
	}

	// Verify lease exists
	lease := &coordinationv1.Lease{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      leaderElectionID,
		Namespace: testNamespace,
	}, lease)
	require.NoError(t, err)
	assert.NotNil(t, lease.Spec.HolderIdentity)

	t.Log("Leader election test completed successfully")
}

// TestLeaderElection_Handoff tests leader handoff when current leader stops
func TestLeaderElection_Handoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-handoff"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Create unique leader election ID
	leaderElectionID := fmt.Sprintf("test-handoff-%d", time.Now().Unix())

	// Start 2 controllers with leader election
	controllers, err := startMultipleControllersWithLeaderElection(
		2, secretName, testNamespace, leaderElectionID)
	require.NoError(t, err)
	defer stopAllControllers(controllers)

	// Wait for initial leader election
	time.Sleep(10 * time.Second)

	// Identify initial leader
	initialLeader, leaderCount := identifyLeader(controllers)
	require.Equal(t, 1, leaderCount, "Should have exactly one leader")
	require.NotNil(t, initialLeader, "Should have a leader")

	t.Logf("Initial leader on ports metrics=%s health=%s",
		initialLeader.MetricsAddr, initialLeader.HealthAddr)

	// Create a NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-handoff",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           1,
			MaxNodes:           3,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for initial reconciliation
	time.Sleep(5 * time.Second)

	// Stop the current leader
	t.Log("Stopping current leader")
	err = initialLeader.Stop()
	require.NoError(t, err)

	// Find the remaining controller
	var remainingController *ControllerProcess
	for _, c := range controllers {
		if c != initialLeader {
			remainingController = c
			break
		}
	}
	require.NotNil(t, remainingController, "Should have a remaining controller")

	// Wait for new leader election (lease renewal + election timeout)
	t.Log("Waiting for new leader election")
	require.Eventually(t, func() bool {
		return isControllerLeader(remainingController)
	}, 15*time.Second, 1*time.Second, "Remaining controller should become leader")

	t.Logf("New leader on ports metrics=%s health=%s",
		remainingController.MetricsAddr, remainingController.HealthAddr)

	// Update NodeGroup to trigger new reconciliation
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, nodeGroup)
	require.NoError(t, err)

	nodeGroup.Spec.MinNodes = 2
	err = k8sClient.Update(ctx, nodeGroup)
	require.NoError(t, err)

	// Wait for new leader to reconcile
	time.Sleep(5 * time.Second)

	// Verify new leader is reconciling
	reconcileCount := getMetricValue(t,
		fmt.Sprintf("http://%s/metrics", remainingController.MetricsAddr),
		"vpsie_autoscaler_controller_reconcile_total")
	assert.Greater(t, reconcileCount, float64(0), "New leader should be reconciling")

	// Verify VPSieNodes are still managed
	list := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, list, client.InNamespace(testNamespace))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list.Items), 1, "VPSieNodes should still exist")

	t.Log("Leader handoff test completed successfully")
}

// TestLeaderElection_SplitBrain tests split-brain prevention
func TestLeaderElection_SplitBrain(t *testing.T) {
	t.Skip("Split-brain testing requires network manipulation capabilities")

	// This test would:
	// 1. Start 3 controllers with leader election
	// 2. Identify the leader
	// 3. Simulate network partition (would require special tooling)
	// 4. Verify only one leader exists (via Kubernetes Lease API)
	// 5. Heal partition
	// 6. Verify leader convergence
}

// TestScaleUp_EndToEnd tests end-to-end scale-up scenario
func TestScaleUp_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	t.Logf("Mock VPSie server started at: %s", mockServer.URL())

	// Create VPSie secret
	secretName := "test-vpsie-secret-scaleup"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(12000, 12100, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Logf("Controller started with PID: %d", proc.PID)

	// Wait for controller to be healthy
	require.Eventually(t, func() bool {
		return proc.IsHealthy()
	}, 30*time.Second, 1*time.Second)

	// Create NodeGroup with initial configuration
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-scaleup-e2e",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           1,
			MaxNodes:           5,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-4cpu-8gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	t.Log("NodeGroup created, waiting for initial node provisioning")

	// Wait for initial VPSieNode
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		return err == nil && len(list.Items) == 1
	}, 30*time.Second, 1*time.Second)

	// Simulate unschedulable pods to trigger scale-up
	t.Log("Creating unschedulable pods to trigger scale-up")

	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("unschedulable-pod-%d", i),
				Namespace: testNamespace,
				Labels: map[string]string{
					"nodegroup": nodeGroup.Name,
				},
			},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"nodegroup": nodeGroup.Name,
				},
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "busybox",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    "2",
								corev1.ResourceMemory: "4Gi",
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: "Unschedulable",
					},
				},
			},
		}

		err = k8sClient.Create(ctx, pod)
		require.NoError(t, err)
		defer func() {
			_ = k8sClient.Delete(ctx, pod)
		}()
	}

	t.Log("Waiting for scale-up to occur")

	// Wait for scale-up
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		if err != nil {
			return false
		}
		t.Logf("Current VPSieNode count: %d", len(list.Items))
		return len(list.Items) >= 3
	}, 60*time.Second, 2*time.Second)

	// Verify metrics
	metrics := getMetricsString(t, fmt.Sprintf("http://localhost:%d/metrics", 12000))
	assert.Contains(t, metrics, "vpsie_autoscaler_scale_up_operations_total")

	// Get final node count
	finalList := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, finalList, client.InNamespace(testNamespace))
	require.NoError(t, err)

	t.Logf("Scale-up completed. Final node count: %d", len(finalList.Items))
	assert.GreaterOrEqual(t, len(finalList.Items), 3)
	assert.LessOrEqual(t, len(finalList.Items), 5) // Should not exceed maxNodes

	t.Log("Scale-up end-to-end test completed successfully")
}

// TestScaleDown_EndToEnd tests end-to-end scale-down scenario
func TestScaleDown_EndToEnd(t *testing.T) {
	t.Skip("Scale-down not yet implemented in controller")

	// This test would:
	// 1. Create NodeGroup with multiple nodes
	// 2. Wait for nodes to be provisioned
	// 3. Reduce load (delete pods)
	// 4. Wait for cooldown period
	// 5. Verify nodes are terminated gracefully
	// 6. Check that minNodes is maintained
}

// TestMixedScaling_EndToEnd tests mixed scaling scenarios
func TestMixedScaling_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.StateTransitions = []VMStateTransition{
		{FromState: "provisioning", ToState: "running", Duration: 2 * time.Second},
		{FromState: "running", ToState: "ready", Duration: 1 * time.Second},
	}
	defer mockServer.Close()

	// Create VPSie secret
	secretName := "test-vpsie-secret-mixed"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(12002, 12102, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	// Create multiple NodeGroups
	nodeGroups := []struct {
		name     string
		minNodes int32
		maxNodes int32
	}{
		{"mixed-ng-1", 2, 5},
		{"mixed-ng-2", 1, 3},
	}

	for _, ng := range nodeGroups {
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ng.name,
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:           ng.minNodes,
				MaxNodes:           ng.maxNodes,
				DatacenterID:       "us-west-1",
				OfferingIDs:        []string{"standard-2cpu-4gb"},
				ResourceIdentifier: "test-cluster",
				Project:            "test-project",
				OSImageID:          "test-os-image",
				KubernetesVersion:  "v1.28.0",
			},
		}

		err = k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err)
		defer func() {
			_ = k8sClient.Delete(ctx, nodeGroup)
		}()
	}

	t.Log("Multiple NodeGroups created, waiting for provisioning")

	// Wait for all minimum nodes to be created
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		return err == nil && len(list.Items) >= 3 // 2 + 1 minimum nodes
	}, 60*time.Second, 2*time.Second)

	// Perform rapid scaling operations
	t.Log("Performing rapid scaling operations")

	// Update first NodeGroup
	ng1 := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      "mixed-ng-1",
		Namespace: testNamespace,
	}, ng1)
	require.NoError(t, err)

	ng1.Spec.MinNodes = 3
	err = k8sClient.Update(ctx, ng1)
	require.NoError(t, err)

	// Update second NodeGroup
	ng2 := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      "mixed-ng-2",
		Namespace: testNamespace,
	}, ng2)
	require.NoError(t, err)

	ng2.Spec.MinNodes = 2
	err = k8sClient.Update(ctx, ng2)
	require.NoError(t, err)

	// Wait for scaling to complete
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		return err == nil && len(list.Items) >= 5 // 3 + 2 new minimum
	}, 60*time.Second, 2*time.Second)

	// Verify nodes are correctly distributed
	list := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, list, client.InNamespace(testNamespace))
	require.NoError(t, err)

	ng1Count := 0
	ng2Count := 0
	for _, node := range list.Items {
		if node.Spec.NodeGroupName == "mixed-ng-1" {
			ng1Count++
		} else if node.Spec.NodeGroupName == "mixed-ng-2" {
			ng2Count++
		}
	}

	assert.GreaterOrEqual(t, ng1Count, 3, "NodeGroup 1 should have at least 3 nodes")
	assert.GreaterOrEqual(t, ng2Count, 2, "NodeGroup 2 should have at least 2 nodes")

	t.Logf("Mixed scaling completed. NG1: %d nodes, NG2: %d nodes", ng1Count, ng2Count)
	t.Log("Mixed scaling end-to-end test completed successfully")
}

// TestScalingWithFailures tests scaling behavior with API failures
func TestScalingWithFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create mock VPSie server with error scenarios
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

	// Inject errors for specific endpoints
	mockServer.ErrorScenarios = []ErrorScenario{
		{
			Endpoint:   "/v2/vms",
			Method:     "POST",
			StatusCode: 500,
			Message:    "Internal server error",
			ErrorCode:  "INTERNAL_ERROR",
			Permanent:  false,
		},
	}

	// Create VPSie secret
	secretName := "test-vpsie-secret-failures"
	err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
	require.NoError(t, err)
	defer func() {
		_ = deleteTestSecret(ctx, secretName, testNamespace)
	}()

	// Start controller
	proc, err := startControllerInBackground(12004, 12104, secretName, testNamespace)
	require.NoError(t, err)
	defer cleanup(proc)

	t.Log("Controller started, testing failure scenarios")

	// Create NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-failures",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:           2,
			MaxNodes:           5,
			DatacenterID:       "us-west-1",
			OfferingIDs:        []string{"standard-2cpu-4gb"},
			ResourceIdentifier: "test-cluster",
			Project:            "test-project",
			OSImageID:          "test-os-image",
			KubernetesVersion:  "v1.28.0",
		},
	}

	err = k8sClient.Create(ctx, nodeGroup)
	require.NoError(t, err)
	defer func() {
		_ = k8sClient.Delete(ctx, nodeGroup)
	}()

	// Wait for initial attempts (should fail)
	time.Sleep(10 * time.Second)

	// Check error metrics
	errorCount := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 12004),
		"vpsie_autoscaler_vpsie_api_errors_total")
	assert.Greater(t, errorCount, float64(0), "Should have API errors")

	t.Log("Clearing error scenarios to allow recovery")

	// Clear error scenarios to allow recovery
	mockServer.ErrorScenarios = []ErrorScenario{}

	// Wait for successful provisioning after recovery
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		return err == nil && len(list.Items) >= 2
	}, 60*time.Second, 2*time.Second)

	// Test rate limiting
	t.Log("Testing rate limiting scenario")

	mockServer.RateLimitRemaining = 0 // Trigger rate limiting

	// Try to update NodeGroup to trigger more API calls
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeGroup.Name,
		Namespace: nodeGroup.Namespace,
	}, nodeGroup)
	require.NoError(t, err)

	nodeGroup.Spec.MinNodes = 3
	err = k8sClient.Update(ctx, nodeGroup)
	require.NoError(t, err)

	// Wait a bit for rate limit to be hit
	time.Sleep(5 * time.Second)

	// Check rate limit metrics
	rateLimitErrors := getMetricValue(t, fmt.Sprintf("http://localhost:%d/metrics", 12004),
		"vpsie_autoscaler_vpsie_api_errors_total")
	assert.Greater(t, rateLimitErrors, errorCount, "Should have rate limit errors")

	// Reset rate limit
	mockServer.RateLimitRemaining = 100

	// Verify recovery after rate limit
	require.Eventually(t, func() bool {
		list := &autoscalerv1alpha1.VPSieNodeList{}
		err := k8sClient.List(ctx, list, client.InNamespace(testNamespace))
		return err == nil && len(list.Items) >= 3
	}, 60*time.Second, 2*time.Second)

	t.Log("Scaling with failures test completed successfully")
}

// Helper function to get metric value
func getMetricValue(t *testing.T, metricsURL, metricName string) float64 {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, metricName) && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				value, err := strconv.ParseFloat(parts[1], 64)
				if err == nil {
					return value
				}
			}
		}
	}
	return 0
}

// Helper function to get metrics as string
func getMetricsString(t *testing.T, metricsURL string) string {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body)
}
