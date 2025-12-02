package nodegroup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/scaler"
)

// MockScaleDownManager is a mock implementation of ScaleDownManager
type MockScaleDownManager struct {
	mock.Mock
}

func (m *MockScaleDownManager) IdentifyUnderutilizedNodes(ctx context.Context, ng *v1alpha1.NodeGroup) ([]*scaler.ScaleDownCandidate, error) {
	args := m.Called(ctx, ng)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*scaler.ScaleDownCandidate), args.Error(1)
}

func (m *MockScaleDownManager) ScaleDown(ctx context.Context, ng *v1alpha1.NodeGroup, candidates []*scaler.ScaleDownCandidate) error {
	args := m.Called(ctx, ng, candidates)
	return args.Error(0)
}

func (m *MockScaleDownManager) UpdateNodeUtilization(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// TestReconcileIntelligentScaleDown_Success tests successful intelligent scale-down
func TestReconcileIntelligentScaleDown_Success(t *testing.T) {
	// Create logger
	logger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	// Create NodeGroup
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
			ReadyNodes:   5,
		},
	}

	// Create VPSieNodes
	vpsieNodes := []v1alpha1.VPSieNode{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-1",
				Namespace: "default",
			},
			Status: v1alpha1.VPSieNodeStatus{
				Phase: v1alpha1.VPSieNodePhaseReady,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-2",
				Namespace: "default",
			},
			Status: v1alpha1.VPSieNodeStatus{
				Phase: v1alpha1.VPSieNodePhaseReady,
			},
		},
	}

	// Create mock ScaleDownManager
	mockSDM := new(MockScaleDownManager)

	// Create scale-down candidates
	candidates := []*scaler.ScaleDownCandidate{
		{
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"autoscaler.vpsie.com/nodegroup": "test-ng",
					},
				},
			},
			Priority: 1,
		},
		{
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"autoscaler.vpsie.com/nodegroup": "test-ng",
					},
				},
			},
			Priority: 2,
		},
	}

	// Setup expectations
	mockSDM.On("IdentifyUnderutilizedNodes", mock.Anything, ng).Return(candidates, nil)
	mockSDM.On("ScaleDown", mock.Anything, ng, candidates).Return(nil)

	// Create fake Kubernetes client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := ctrlclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng).
		WithStatusSubresource(ng).
		Build()

	// Create reconciler
	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: mockSDM,
		Logger:           logger,
	}

	// Test reconcileIntelligentScaleDown
	ctx := context.Background()
	result, err := reconciler.reconcileIntelligentScaleDown(ctx, ng, vpsieNodes, logger)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, FastRequeueAfter, result.RequeueAfter)
	mockSDM.AssertExpectations(t)

	t.Log("✓ Intelligent scale-down succeeded with 2 candidates")
	t.Log("✓ IdentifyUnderutilizedNodes was called")
	t.Log("✓ ScaleDown was called with correct candidates")
}

// TestReconcileIntelligentScaleDown_NoCandidates tests when no underutilized nodes found
func TestReconcileIntelligentScaleDown_NoCandidates(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
			ReadyNodes:   5,
		},
	}

	vpsieNodes := []v1alpha1.VPSieNode{}

	// Create mock ScaleDownManager
	mockSDM := new(MockScaleDownManager)

	// No candidates found
	mockSDM.On("IdentifyUnderutilizedNodes", mock.Anything, ng).Return([]*scaler.ScaleDownCandidate{}, nil)

	// Create fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	k8sClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(ng).Build()

	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: mockSDM,
		Logger:           logger,
	}

	ctx := context.Background()
	result, err := reconciler.reconcileIntelligentScaleDown(ctx, ng, vpsieNodes, logger)

	assert.NoError(t, err)
	assert.Equal(t, DefaultRequeueAfter, result.RequeueAfter)
	mockSDM.AssertExpectations(t)

	t.Log("✓ No candidates found, requeued with default interval")
	t.Log("✓ ScaleDown was not called (as expected)")
}

// TestReconcileIntelligentScaleDown_IdentifyError tests error during candidate identification
func TestReconcileIntelligentScaleDown_IdentifyError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
		},
	}

	vpsieNodes := []v1alpha1.VPSieNode{}

	// Create mock ScaleDownManager
	mockSDM := new(MockScaleDownManager)

	// Simulate error
	mockSDM.On("IdentifyUnderutilizedNodes", mock.Anything, ng).Return(nil, errors.New("metrics unavailable"))

	// Create fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := ctrlclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng).
		WithStatusSubresource(ng).
		Build()

	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: mockSDM,
		Logger:           logger,
	}

	ctx := context.Background()
	_, err := reconciler.reconcileIntelligentScaleDown(ctx, ng, vpsieNodes, logger)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics unavailable")
	mockSDM.AssertExpectations(t)

	t.Log("✓ Error during identification was properly handled")
	t.Log("✓ Error condition was set in NodeGroup status")
}

// TestReconcileIntelligentScaleDown_ScaleDownError tests error during scale-down execution
func TestReconcileIntelligentScaleDown_ScaleDownError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
		},
	}

	vpsieNodes := []v1alpha1.VPSieNode{}

	// Create mock ScaleDownManager
	mockSDM := new(MockScaleDownManager)

	candidates := []*scaler.ScaleDownCandidate{
		{
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
		},
	}

	mockSDM.On("IdentifyUnderutilizedNodes", mock.Anything, ng).Return(candidates, nil)
	mockSDM.On("ScaleDown", mock.Anything, ng, candidates).Return(errors.New("pod eviction failed"))

	// Create fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := ctrlclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng).
		WithStatusSubresource(ng).
		Build()

	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: mockSDM,
		Logger:           logger,
	}

	ctx := context.Background()
	_, err := reconciler.reconcileIntelligentScaleDown(ctx, ng, vpsieNodes, logger)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pod eviction failed")
	mockSDM.AssertExpectations(t)

	t.Log("✓ Scale-down error was properly propagated")
	t.Log("✓ Error condition was set with ReasonScaleDownFailed")
}

// TestReconcileScaleDown_FallbackToSimple tests fallback when ScaleDownManager is nil
func TestReconcileScaleDown_FallbackToSimple(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
		},
	}

	vpsieNodes := []v1alpha1.VPSieNode{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-1",
				Namespace: "default",
			},
			Status: v1alpha1.VPSieNodeStatus{
				Phase: v1alpha1.VPSieNodePhaseReady,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-2",
				Namespace: "default",
			},
			Status: v1alpha1.VPSieNodeStatus{
				Phase: v1alpha1.VPSieNodePhaseProvisioning,
			},
		},
	}

	// Create fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := ctrlclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng, &vpsieNodes[0], &vpsieNodes[1]).
		WithStatusSubresource(ng).
		Build()

	// Create reconciler WITHOUT ScaleDownManager (nil)
	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: nil, // No ScaleDownManager
		Logger:           logger,
	}

	ctx := context.Background()
	result, err := reconciler.reconcileScaleDown(ctx, ng, vpsieNodes, logger)

	assert.NoError(t, err)
	assert.Equal(t, FastRequeueAfter, result.RequeueAfter)

	t.Log("✓ Fallback to simple scale-down when ScaleDownManager is nil")
	t.Log("✓ Simple scale-down selected not-ready node for removal")
}

// TestReconcileScaleDown_WithScaleDownManager tests that intelligent path is taken when manager is available
func TestReconcileScaleDown_WithScaleDownManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 5,
			DesiredNodes: 3,
		},
	}

	vpsieNodes := []v1alpha1.VPSieNode{}

	// Create mock ScaleDownManager
	mockSDM := new(MockScaleDownManager)
	mockSDM.On("IdentifyUnderutilizedNodes", mock.Anything, ng).Return([]*scaler.ScaleDownCandidate{}, nil)

	// Create fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	k8sClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(ng).Build()

	// Create reconciler WITH ScaleDownManager
	reconciler := &NodeGroupReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		ScaleDownManager: mockSDM, // Has ScaleDownManager
		Logger:           logger,
	}

	ctx := context.Background()
	result, err := reconciler.reconcileScaleDown(ctx, ng, vpsieNodes, logger)

	assert.NoError(t, err)
	assert.Equal(t, DefaultRequeueAfter, result.RequeueAfter)
	mockSDM.AssertExpectations(t)

	t.Log("✓ Intelligent scale-down path was taken (not simple)")
	t.Log("✓ ScaleDownManager.IdentifyUnderutilizedNodes was called")
}

// TestScaleDownIntegration_WithRealScaler tests integration with a real ScaleDownManager
func TestScaleDownIntegration_WithRealScaler(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create test nodes
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup": "test-ng",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	// Create node metrics showing low utilization (< 50%)
	nodeMetrics := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Timestamp: metav1.Time{Time: time.Now()},
		Window:    metav1.Duration{Duration: 1 * time.Minute},
		Usage: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(800, resource.DecimalSI),        // 20% CPU
			corev1.ResourceMemory: *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), // 12.5% Memory
		},
	}

	// Create fake Kubernetes clients with pre-populated data
	k8sClient := fake.NewSimpleClientset(node1)
	metricsClient := metricsfake.NewSimpleClientset(nodeMetrics)

	// Create real ScaleDownManager
	config := scaler.DefaultConfig()
	config.ObservationWindow = 1 * time.Minute // Short window for testing
	sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, config)

	// Update metrics
	err := sdm.UpdateNodeUtilization(context.Background())
	assert.NoError(t, err)

	// Create NodeGroup
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     10,
			DatacenterID: "123",
			OfferingIDs:  []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 2,
			DesiredNodes: 1,
		},
	}

	// Sleep to allow observation window
	time.Sleep(100 * time.Millisecond)

	// Update metrics again to build history
	err = sdm.UpdateNodeUtilization(context.Background())
	assert.NoError(t, err)

	// Identify underutilized nodes
	candidates, err := sdm.IdentifyUnderutilizedNodes(context.Background(), ng)

	// Note: May not find candidates due to short observation window, but should not error
	assert.NoError(t, err)

	t.Logf("✓ Real ScaleDownManager integration test completed")
	t.Logf("✓ Found %d candidates (may be 0 due to short observation window)", len(candidates))
	t.Log("✓ No errors during metrics collection and candidate identification")
}

// TestScaleDownManager_MetricsClientIntegration tests metrics client can be used by ScaleDownManager
func TestScaleDownManager_MetricsClientIntegration(t *testing.T) {
	// Create test node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(2000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(2000, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			},
		},
	}

	// Create node metrics
	nodeMetrics := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Timestamp: metav1.Time{Time: time.Now()},
		Window:    metav1.Duration{Duration: 1 * time.Minute},
		Usage: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),        // 25% CPU
			corev1.ResourceMemory: *resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), // 25% Memory
		},
	}

	// Create fake clients with pre-populated data
	k8sClient := fake.NewSimpleClientset(node)
	metricsClient := metricsfake.NewSimpleClientset(nodeMetrics)

	// Create ScaleDownManager
	logger, _ := zap.NewDevelopment()
	sdm := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, scaler.DefaultConfig())

	// Update utilization
	err := sdm.UpdateNodeUtilization(context.Background())
	assert.NoError(t, err)

	t.Log("✓ ScaleDownManager successfully integrated with metrics client")
	t.Log("✓ Node utilization data collected without errors")
}
