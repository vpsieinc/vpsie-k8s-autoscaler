//go:build e2e
// +build e2e

// Package e2e contains end-to-end tests for the VPSie Kubernetes Autoscaler.
// These tests require a running Kubernetes cluster (kind recommended) and
// exercise the full autoscaling workflow from unschedulable pods to node provisioning.
package e2e

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
	// TestNamespace is the namespace for E2E test resources
	TestNamespace = "vpsie-e2e-test"

	// DefaultTimeout for E2E operations
	DefaultTimeout = 5 * time.Minute

	// PollInterval for checking conditions
	PollInterval = 5 * time.Second
)

var (
	// Global test configuration
	cfg       *rest.Config
	k8sClient client.Client
	clientset kubernetes.Interface
	scheme    *runtime.Scheme

	// Mock server for VPSie API
	mockServer *MockVPSieServer
)

// TestMain sets up the E2E test environment
func TestMain(m *testing.M) {
	var err error

	// Load kubeconfig
	kubeconfigPath := getKubeconfigPath()
	if kubeconfigPath == "" {
		fmt.Println("ERROR: Could not determine kubeconfig path")
		fmt.Println("Set KUBECONFIG environment variable or ensure ~/.kube/config exists")
		os.Exit(1)
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to load kubeconfig from %s: %v\n", kubeconfigPath, err)
		os.Exit(1)
	}
	fmt.Printf("Using kubeconfig: %s\n", kubeconfigPath)

	// Create clientset
	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("ERROR: Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	// Setup scheme with required types
	scheme = runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		fmt.Printf("ERROR: Failed to add core/v1 to scheme: %v\n", err)
		os.Exit(1)
	}
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		fmt.Printf("ERROR: Failed to add autoscaler/v1alpha1 to scheme: %v\n", err)
		os.Exit(1)
	}

	// Create controller-runtime client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Printf("ERROR: Failed to create controller-runtime client: %v\n", err)
		os.Exit(1)
	}

	// Verify cluster connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = clientset.CoreV1().Namespaces().Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		fmt.Printf("ERROR: Cannot connect to Kubernetes cluster: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Successfully connected to Kubernetes cluster")

	// Setup test namespace
	if err := setupTestNamespace(ctx); err != nil {
		fmt.Printf("ERROR: Failed to setup test namespace: %v\n", err)
		os.Exit(1)
	}

	// Start mock VPSie API server
	mockServer = NewMockVPSieServer()
	fmt.Printf("Mock VPSie API server started at: %s\n", mockServer.URL())

	// Run tests
	code := m.Run()

	// Cleanup
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cleanupCancel()

	if os.Getenv("SKIP_CLEANUP") != "true" {
		if err := cleanupTestNamespace(cleanupCtx); err != nil {
			fmt.Printf("WARNING: Failed to cleanup test namespace: %v\n", err)
		}
	}

	if mockServer != nil {
		mockServer.Close()
	}

	os.Exit(code)
}

// getKubeconfigPath returns the kubeconfig path from environment or default
func getKubeconfigPath() string {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// setupTestNamespace creates the test namespace if it doesn't exist
func setupTestNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "vpsie-autoscaler",
				"app.kubernetes.io/component":  "e2e-test",
				"app.kubernetes.io/managed-by": "e2e-tests",
			},
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		// Check if namespace already exists
		_, getErr := clientset.CoreV1().Namespaces().Get(ctx, TestNamespace, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create or get test namespace: %v", err)
		}
		fmt.Printf("Test namespace %s already exists\n", TestNamespace)
		return nil
	}

	fmt.Printf("Created test namespace: %s\n", TestNamespace)
	return nil
}

// cleanupTestNamespace deletes the test namespace and all resources
func cleanupTestNamespace(ctx context.Context) error {
	// Delete all NodeGroups in the namespace first
	ngList := &autoscalerv1alpha1.NodeGroupList{}
	if err := k8sClient.List(ctx, ngList, client.InNamespace(TestNamespace)); err == nil {
		for _, ng := range ngList.Items {
			if err := k8sClient.Delete(ctx, &ng); err != nil {
				fmt.Printf("WARNING: Failed to delete NodeGroup %s: %v\n", ng.Name, err)
			}
		}
	}

	// Delete all VPSieNodes in the namespace
	vnList := &autoscalerv1alpha1.VPSieNodeList{}
	if err := k8sClient.List(ctx, vnList, client.InNamespace(TestNamespace)); err == nil {
		for _, vn := range vnList.Items {
			if err := k8sClient.Delete(ctx, &vn); err != nil {
				fmt.Printf("WARNING: Failed to delete VPSieNode %s: %v\n", vn.Name, err)
			}
		}
	}

	// Delete namespace
	err := clientset.CoreV1().Namespaces().Delete(ctx, TestNamespace, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete test namespace: %v", err)
	}

	fmt.Printf("Deleted test namespace: %s\n", TestNamespace)
	return nil
}

// createVPSieSecret creates the VPSie credentials secret for testing
func createVPSieSecret(ctx context.Context, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-secret",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"clientId":     "test-client-id",
			"clientSecret": "test-client-secret",
			"url":          mockServer.URL(),
		},
	}

	_, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		// Check if secret already exists
		_, getErr := clientset.CoreV1().Secrets(namespace).Get(ctx, "vpsie-secret", metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create or get secret: %v", err)
		}
	}

	return nil
}
