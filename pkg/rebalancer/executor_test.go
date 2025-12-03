package rebalancer

import (
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestNewExecutor(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()

	t.Run("Create with default config", func(t *testing.T) {
		executor := NewExecutor(kubeClient, nil, nil)
		if executor == nil {
			t.Fatal("Expected executor to be created")
		}
		if executor.config == nil {
			t.Fatal("Expected default config to be set")
		}
		if executor.config.DrainTimeout != 5*time.Minute {
			t.Errorf("Expected DrainTimeout=5m, got %v", executor.config.DrainTimeout)
		}
		if executor.config.MaxRetries != 3 {
			t.Errorf("Expected MaxRetries=3, got %d", executor.config.MaxRetries)
		}
		if executor.config.ProvisionTimeout != 10*time.Minute {
			t.Errorf("Expected ProvisionTimeout=10m, got %v", executor.config.ProvisionTimeout)
		}
		if executor.config.HealthCheckInterval != 10*time.Second {
			t.Errorf("Expected HealthCheckInterval=10s, got %v", executor.config.HealthCheckInterval)
		}
	})

	t.Run("Create with custom config", func(t *testing.T) {
		config := &ExecutorConfig{
			DrainTimeout:        15 * time.Minute,
			ProvisionTimeout:    20 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          5,
		}
		executor := NewExecutor(kubeClient, nil, config)
		if executor.config.DrainTimeout != 15*time.Minute {
			t.Errorf("Expected DrainTimeout=15m, got %v", executor.config.DrainTimeout)
		}
		if executor.config.MaxRetries != 5 {
			t.Errorf("Expected MaxRetries=5, got %d", executor.config.MaxRetries)
		}
	})
}

// Note: Full ExecuteRebalance tests are complex and require realistic
// cluster state with node provisioning. These are better suited for
// integration tests rather than unit tests. The planner and analyzer
// components are tested separately above.

// Note: Additional integration tests for cordon, delete, rollback, pause/resume
// would require more complex test setups and are better suited for integration tests
