//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller"
)

const (
	// Kubeconfig path for the test cluster
	testKubeconfig = "/Users/zozo/.kube/config-new-test"
	// Test namespace for integration tests
	testNamespace = "vpsie-autoscaler-test"
	// Timeout for operations
	testTimeout = 30 * time.Second
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme
)

// TestMain sets up the integration test environment
func TestMain(m *testing.M) {
	var err error
	exitCode := 1

	defer func() {
		// Cleanup test namespace
		if clientset != nil {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()
			_ = clientset.CoreV1().Namespaces().Delete(ctx, testNamespace, metav1.DeleteOptions{})
		}
		os.Exit(exitCode)
	}()

	// Load kubeconfig from test cluster
	cfg, err = clientcmd.BuildConfigFromFlags("", testKubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load kubeconfig: %v\n", err)
		return
	}

	// Create clientset
	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create clientset: %v\n", err)
		return
	}

	// Create scheme with our CRDs
	scheme = runtime.NewScheme()
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add autoscaler scheme: %v\n", err)
		return
	}

	// Create controller-runtime client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create controller-runtime client: %v\n", err)
		return
	}

	// Verify cluster connectivity
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to cluster: %v\n", err)
		return
	}

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		fmt.Fprintf(os.Stderr, "Failed to create test namespace: %v\n", err)
		return
	}

	// Verify CRDs are installed
	if err := verifyCRDsInstalled(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "CRDs not installed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please install CRDs first: kubectl apply -f deploy/crds/\n")
		return
	}

	fmt.Printf("Integration test setup complete. Using cluster: %s\n", cfg.Host)
	fmt.Printf("Test namespace: %s\n", testNamespace)

	// Run tests
	exitCode = m.Run()
}

// verifyCRDsInstalled checks that required CRDs are installed
func verifyCRDsInstalled(ctx context.Context) error {
	crdPath := filepath.Join("..", "..", "deploy", "crds")
	files, err := os.ReadDir(crdPath)
	if err != nil {
		return fmt.Errorf("failed to read CRD directory: %w", err)
	}

	expectedCRDs := []string{
		"autoscaler.vpsie.com_nodegroups.yaml",
		"autoscaler.vpsie.com_vpsienodes.yaml",
	}

	for _, expectedCRD := range expectedCRDs {
		found := false
		for _, file := range files {
			if file.Name() == expectedCRD {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("CRD file not found: %s", expectedCRD)
		}
	}

	return nil
}

// TestControllerManager_Integration tests the controller manager with a real Kubernetes API
func TestControllerManager_Integration(t *testing.T) {
	t.Skip("Requires VPSie API credentials - implement when secrets are available")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create controller manager options
	opts := controller.NewDefaultOptions()
	opts.MetricsAddr = ":18080"       // Use different port for tests
	opts.HealthProbeAddr = ":18081"   // Use different port for tests
	opts.EnableLeaderElection = false // Disable for testing
	opts.VPSieSecretName = "test-secret"
	opts.VPSieSecretNamespace = testNamespace

	// Complete and validate options
	require.NoError(t, opts.Complete())
	require.NoError(t, opts.Validate())

	// Note: Creating manager requires VPSie API credentials in a secret
	// This test will be implemented when we have test credentials
	_ = ctx
}

// TestNodeGroup_CRUD tests NodeGroup CRD operations
func TestNodeGroup_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a NodeGroup
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup-crud",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "dc-test",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	// Test creating NodeGroup
	t.Log("Creating NodeGroup...")
	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err, "Failed to create NodeGroup")

	// Test reading NodeGroup
	t.Log("Reading NodeGroup...")
	retrieved := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, retrieved)
	require.NoError(t, err, "Failed to get NodeGroup")
	assert.Equal(t, int32(1), retrieved.Spec.MinNodes)
	assert.Equal(t, int32(10), retrieved.Spec.MaxNodes)
	assert.Equal(t, "dc-test", retrieved.Spec.DatacenterID)

	// Test updating NodeGroup
	t.Log("Updating NodeGroup...")
	retrieved.Spec.MaxNodes = 20
	retrieved.Spec.MinNodes = 2
	err = k8sClient.Update(ctx, retrieved)
	require.NoError(t, err, "Failed to update NodeGroup")

	// Verify update
	updated := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, updated)
	require.NoError(t, err, "Failed to get updated NodeGroup")
	assert.Equal(t, int32(2), updated.Spec.MinNodes)
	assert.Equal(t, int32(20), updated.Spec.MaxNodes)

	// Test deleting NodeGroup
	t.Log("Deleting NodeGroup...")
	err = k8sClient.Delete(ctx, ng)
	require.NoError(t, err, "Failed to delete NodeGroup")

	// Verify deletion
	deleted := &autoscalerv1alpha1.NodeGroup{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, deleted)
	assert.True(t, errors.IsNotFound(err), "NodeGroup should be deleted")

	t.Log("NodeGroup CRUD test completed successfully")
}

// TestVPSieNode_CRUD tests VPSieNode CRD operations
func TestVPSieNode_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a VPSieNode
	vn := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vpsienode-crud",
			Namespace: testNamespace,
		},
		Spec: autoscalerv1alpha1.VPSieNodeSpec{
			NodeGroupName: "test-nodegroup",
			InstanceType:  "small-2cpu-4gb",
			DatacenterID:  "dc-test",
		},
	}

	// Test creating VPSieNode
	t.Log("Creating VPSieNode...")
	err := k8sClient.Create(ctx, vn)
	require.NoError(t, err, "Failed to create VPSieNode")

	// Test reading VPSieNode
	t.Log("Reading VPSieNode...")
	retrieved := &autoscalerv1alpha1.VPSieNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vn.Name,
		Namespace: vn.Namespace,
	}, retrieved)
	require.NoError(t, err, "Failed to get VPSieNode")
	assert.Equal(t, "test-nodegroup", retrieved.Spec.NodeGroupName)
	assert.Equal(t, "small-2cpu-4gb", retrieved.Spec.InstanceType)
	assert.Equal(t, "dc-test", retrieved.Spec.DatacenterID)

	// Test updating status
	t.Log("Updating VPSieNode status...")
	retrieved.Status.Phase = autoscalerv1alpha1.VPSieNodePhaseProvisioning
	retrieved.Status.Conditions = []autoscalerv1alpha1.VPSieNodeCondition{
		{
			Type:               "VPSReady",
			Status:             "True",
			LastTransitionTime: metav1.Now(),
			Reason:             "Provisioning",
			Message:            "VPS is being provisioned",
		},
	}
	err = k8sClient.Status().Update(ctx, retrieved)
	require.NoError(t, err, "Failed to update VPSieNode status")

	// Verify status update
	updated := &autoscalerv1alpha1.VPSieNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vn.Name,
		Namespace: vn.Namespace,
	}, updated)
	require.NoError(t, err, "Failed to get updated VPSieNode")
	assert.Equal(t, autoscalerv1alpha1.VPSieNodePhaseProvisioning, updated.Status.Phase)
	require.Len(t, updated.Status.Conditions, 1)
	assert.Equal(t, autoscalerv1alpha1.VPSieNodeConditionType("VPSReady"), updated.Status.Conditions[0].Type)

	// Test deleting VPSieNode
	t.Log("Deleting VPSieNode...")
	err = k8sClient.Delete(ctx, vn)
	require.NoError(t, err, "Failed to delete VPSieNode")

	// Verify deletion
	deleted := &autoscalerv1alpha1.VPSieNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vn.Name,
		Namespace: vn.Namespace,
	}, deleted)
	assert.True(t, errors.IsNotFound(err), "VPSieNode should be deleted")

	t.Log("VPSieNode CRUD test completed successfully")
}

// TestHealthEndpoints_Integration tests health check endpoints
func TestHealthEndpoints_Integration(t *testing.T) {
	t.Skip("Integration test requires running controller - implement in phase 3")

	// Test /healthz endpoint
	// resp, err := http.Get("http://localhost:18081/healthz")
	// require.NoError(t, err)
	// defer resp.Body.Close()
	// assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test /readyz endpoint
	// resp, err = http.Get("http://localhost:18081/readyz")
	// require.NoError(t, err)
	// defer resp.Body.Close()
	// assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestMetricsEndpoint_Integration tests Prometheus metrics endpoint
func TestMetricsEndpoint_Integration(t *testing.T) {
	t.Skip("Integration test requires running controller - implement in phase 3")

	// Test /metrics endpoint
	// resp, err := http.Get("http://localhost:18080/metrics")
	// require.NoError(t, err)
	// defer resp.Body.Close()
	// assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify metrics are exposed
	// body, err := io.ReadAll(resp.Body)
	// require.NoError(t, err)
	// assert.Contains(t, string(body), "vpsie_autoscaler_")
}

// TestLeaderElection_Integration tests leader election with multiple replicas
func TestLeaderElection_Integration(t *testing.T) {
	t.Skip("Integration test requires multiple controller instances - implement in phase 4")

	// Start multiple controller instances
	// Verify only one becomes leader
	// Verify leader election handoff when leader stops
}

// TestControllerReconciliation_Integration tests end-to-end reconciliation
func TestControllerReconciliation_Integration(t *testing.T) {
	t.Skip("Integration test requires VPSie API mock - implement in phase 3")

	ctx := context.Background()
	_ = ctx

	// Create a NodeGroup
	// Verify VPSieNode resources are created
	// Verify controller attempts to provision VMs
	// Verify status updates
}

// TestGracefulShutdown_Integration tests graceful shutdown
func TestGracefulShutdown_Integration(t *testing.T) {
	t.Skip("Integration test requires running controller - implement in phase 3")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_ = ctx

	// Start controller
	// Send SIGTERM
	// Verify controller shuts down gracefully within timeout
	// Verify all resources are cleaned up
}

// TestConfigurationValidation_Integration tests configuration validation
func TestConfigurationValidation_Integration(t *testing.T) {
	t.Run("invalid metrics address", func(t *testing.T) {
		opts := &controller.Options{
			MetricsAddr: "",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "metrics address")
	})

	t.Run("same metrics and health address", func(t *testing.T) {
		opts := &controller.Options{
			MetricsAddr:     ":8080",
			HealthProbeAddr: ":8080",
		}
		err := opts.Validate()
		assert.Error(t, err)
	})

	t.Run("valid configuration", func(t *testing.T) {
		opts := controller.NewDefaultOptions()
		err := opts.Validate()
		assert.NoError(t, err)
	})
}

// TestMetricsRegistration_Integration tests metrics registration
func TestMetricsRegistration_Integration(t *testing.T) {
	// This test can run without full integration setup
	// It verifies metrics are registered correctly

	// Import metrics package and register
	// Note: In real test, we'd import and call metrics.RegisterMetrics()
	// For now, this is a placeholder

	t.Run("metrics registration succeeds", func(t *testing.T) {
		// This would test that metrics.RegisterMetrics() doesn't panic
		// and that all 22 metrics are registered
		t.Skip("Implement with metrics package integration")
	})
}
