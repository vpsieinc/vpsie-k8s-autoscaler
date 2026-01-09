//go:build chaos
// +build chaos

// Package chaos contains chaos engineering tests for the VPSie Kubernetes Autoscaler.
// These tests validate system resilience under various failure conditions including
// API failures, network partitions, and controller crashes.
package chaos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

const (
	// TestNamespace for chaos tests
	TestNamespace = "vpsie-chaos-test"

	// Default timeouts
	DefaultTimeout = 5 * time.Minute
	ShortTimeout   = 30 * time.Second

	// Chaos parameters
	DefaultErrorRate        = 0.5
	HighErrorRate           = 0.9
	DefaultLatency          = 500 * time.Millisecond
	HighLatency             = 2 * time.Second
	NetworkPartitionTimeout = 30 * time.Second
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme
)

// ChaosScenario defines a chaos test scenario
type ChaosScenario struct {
	Name        string
	Description string
	Setup       func(ctx context.Context, t *testing.T) error
	Execute     func(ctx context.Context, t *testing.T) error
	Verify      func(ctx context.Context, t *testing.T) error
	Cleanup     func(ctx context.Context, t *testing.T) error
	Timeout     time.Duration
}

// TestMain sets up the chaos test environment
func TestMain(m *testing.M) {
	var err error

	kubeconfigPath := getKubeconfigPath()
	if kubeconfigPath == "" {
		fmt.Println("ERROR: Could not determine kubeconfig path")
		os.Exit(1)
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("ERROR: Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	scheme = runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = autoscalerv1alpha1.AddToScheme(scheme)

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Printf("ERROR: Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Setup test namespace
	ctx := context.Background()
	if err := setupTestNamespace(ctx); err != nil {
		fmt.Printf("ERROR: Failed to setup test namespace: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Cleanup
	if os.Getenv("SKIP_CLEANUP") != "true" {
		_ = cleanupTestNamespace(context.Background())
	}

	os.Exit(code)
}

func getKubeconfigPath() string {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}

func setupTestNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "vpsie-autoscaler",
				"app.kubernetes.io/component": "chaos-test",
			},
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		_, getErr := clientset.CoreV1().Namespaces().Get(ctx, TestNamespace, metav1.GetOptions{})
		if getErr != nil {
			return err
		}
	}
	return nil
}

func cleanupTestNamespace(ctx context.Context) error {
	return clientset.CoreV1().Namespaces().Delete(ctx, TestNamespace, metav1.DeleteOptions{})
}

// RunChaosScenario executes a chaos scenario with proper setup and cleanup
func RunChaosScenario(t *testing.T, scenario ChaosScenario) {
	t.Helper()

	timeout := scenario.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	t.Logf("Running chaos scenario: %s", scenario.Name)
	t.Logf("Description: %s", scenario.Description)

	// Setup
	if scenario.Setup != nil {
		t.Log("Setting up chaos scenario...")
		if err := scenario.Setup(ctx, t); err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	// Cleanup (deferred)
	if scenario.Cleanup != nil {
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), ShortTimeout)
			defer cleanupCancel()
			if err := scenario.Cleanup(cleanupCtx, t); err != nil {
				t.Logf("WARNING: Cleanup failed: %v", err)
			}
		}()
	}

	// Execute chaos
	if scenario.Execute != nil {
		t.Log("Executing chaos...")
		if err := scenario.Execute(ctx, t); err != nil {
			t.Fatalf("Execution failed: %v", err)
		}
	}

	// Verify recovery
	if scenario.Verify != nil {
		t.Log("Verifying recovery...")
		if err := scenario.Verify(ctx, t); err != nil {
			t.Fatalf("Verification failed: %v", err)
		}
	}

	t.Log("Chaos scenario completed successfully")
}

// ChaosConfig holds configuration for chaos injection
type ChaosConfig struct {
	ErrorRate       float64
	Latency         time.Duration
	LatencyVariance time.Duration
	FailureMode     string // "random", "always", "never"
	Duration        time.Duration
}

// DefaultChaosConfig returns default chaos configuration
func DefaultChaosConfig() ChaosConfig {
	return ChaosConfig{
		ErrorRate:       DefaultErrorRate,
		Latency:         DefaultLatency,
		LatencyVariance: 100 * time.Millisecond,
		FailureMode:     "random",
		Duration:        30 * time.Second,
	}
}

// HighChaosConfig returns aggressive chaos configuration
func HighChaosConfig() ChaosConfig {
	return ChaosConfig{
		ErrorRate:       HighErrorRate,
		Latency:         HighLatency,
		LatencyVariance: 500 * time.Millisecond,
		FailureMode:     "random",
		Duration:        60 * time.Second,
	}
}
