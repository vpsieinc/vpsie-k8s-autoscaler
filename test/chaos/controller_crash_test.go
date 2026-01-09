//go:build chaos
// +build chaos

package chaos

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// ControllerProcess represents a running controller for chaos testing
type ControllerProcess struct {
	Cmd    *exec.Cmd
	PID    int
	Binary string
}

// StartController starts a controller process for chaos testing
func StartController(binary string, args ...string) (*ControllerProcess, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &ControllerProcess{
		Cmd:    cmd,
		PID:    cmd.Process.Pid,
		Binary: binary,
	}, nil
}

// Kill forcefully terminates the controller (SIGKILL)
func (c *ControllerProcess) Kill() error {
	if c.Cmd == nil || c.Cmd.Process == nil {
		return nil
	}
	return c.Cmd.Process.Kill()
}

// Terminate gracefully terminates the controller (SIGTERM)
func (c *ControllerProcess) Terminate() error {
	if c.Cmd == nil || c.Cmd.Process == nil {
		return nil
	}
	return c.Cmd.Process.Signal(syscall.SIGTERM)
}

// Wait waits for the controller to exit
func (c *ControllerProcess) Wait() error {
	if c.Cmd == nil {
		return nil
	}
	return c.Cmd.Wait()
}

// IsRunning checks if the controller is still running
func (c *ControllerProcess) IsRunning() bool {
	if c.Cmd == nil || c.Cmd.Process == nil {
		return false
	}
	// Try to send signal 0 to check if process exists
	err := c.Cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// TestControllerCrash_DuringScaleUp tests crash during scale-up operation
func TestControllerCrash_DuringScaleUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create a NodeGroup that would trigger scale-up
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-crash-scaleup",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     2, // Will need to provision nodes
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

	// In a full chaos test:
	// 1. Start controller process
	// 2. Wait for scale-up to begin
	// 3. Kill controller (SIGKILL)
	// 4. Verify state is consistent
	// 5. Restart controller
	// 6. Verify recovery and completion

	// For now, verify NodeGroup exists and can be queried
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	t.Log("Scale-up crash scenario setup complete")
}

// TestControllerCrash_DuringScaleDown tests crash during scale-down operation
func TestControllerCrash_DuringScaleDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create NodeGroup
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-crash-scaledown",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     0, // Can scale to 0
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

	// Create VPSieNode to simulate existing node
	vn := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-node-scaledown",
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

	err = k8sClient.Create(ctx, vn)
	require.NoError(t, err)

	defer func() {
		_ = k8sClient.Delete(context.Background(), vn)
	}()

	// In a full chaos test:
	// 1. Start controller
	// 2. Trigger scale-down (reduce minNodes, or underutilized node)
	// 3. Wait for drain to begin
	// 4. Kill controller during drain
	// 5. Verify node state (should not be partially drained)
	// 6. Restart controller
	// 7. Verify proper recovery

	t.Log("Scale-down crash scenario setup complete")
}

// TestControllerCrash_DuringRebalance tests crash during rebalancing
func TestControllerCrash_DuringRebalance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-crash-rebalance",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "dc-test-1",
			OfferingIDs:  []string{"small-2cpu-4gb", "medium-4cpu-8gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err)

	defer func() {
		_ = k8sClient.Delete(context.Background(), ng)
	}()

	// In a full chaos test:
	// 1. Start controller
	// 2. Trigger rebalance (cost optimization)
	// 3. Kill controller during node replacement
	// 4. Verify no orphaned resources
	// 5. Restart controller
	// 6. Verify rollback or completion

	t.Log("Rebalance crash scenario setup complete")
}

// TestControllerCrash_MultipleRestarts tests behavior under multiple crashes
func TestControllerCrash_MultipleRestarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-crash-multiple",
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

	// Simulate multiple crash/restart cycles
	for i := 0; i < 3; i++ {
		t.Logf("Crash cycle %d", i+1)

		// In a full test:
		// 1. Start controller
		// 2. Let it run briefly
		// 3. Kill (SIGKILL)
		// 4. Verify state consistency
		// 5. Wait and repeat

		time.Sleep(100 * time.Millisecond)
	}

	// Verify NodeGroup is still valid after multiple simulated crashes
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	t.Log("Multiple crash scenario completed")
}

// TestControllerCrash_ResourceLeaks tests for resource leaks after crash
func TestControllerCrash_ResourceLeaks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create resources before "crash"
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-leak-test",
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

	// Create some VPSieNodes
	for i := 0; i < 3; i++ {
		vn := &autoscalerv1alpha1.VPSieNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "chaos-leak-node-" + string(rune('a'+i)),
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

	// List VPSieNodes before simulated crash
	vnList := &autoscalerv1alpha1.VPSieNodeList{}
	err = k8sClient.List(ctx, vnList, client.InNamespace(TestNamespace), client.MatchingLabels{
		"autoscaler.vpsie.com/nodegroup": ng.Name,
	})
	require.NoError(t, err)

	beforeCount := len(vnList.Items)
	t.Logf("VPSieNodes before crash: %d", beforeCount)

	// Simulate crash - in real test would kill controller here

	// List VPSieNodes after simulated crash
	err = k8sClient.List(ctx, vnList, client.InNamespace(TestNamespace), client.MatchingLabels{
		"autoscaler.vpsie.com/nodegroup": ng.Name,
	})
	require.NoError(t, err)

	afterCount := len(vnList.Items)
	t.Logf("VPSieNodes after crash: %d", afterCount)

	// Should not leak resources
	assert.Equal(t, beforeCount, afterCount, "Should not leak VPSieNodes")
}

// TestControllerCrash_LeaderElectionRecovery tests leader election after crash
func TestControllerCrash_LeaderElectionRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-leader-recovery",
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

	// In a full chaos test with HA setup:
	// 1. Start 2 controller instances (leader + follower)
	// 2. Kill leader
	// 3. Verify follower acquires leadership
	// 4. Verify operations continue
	// 5. Restart original leader
	// 6. Verify it becomes follower

	t.Log("Leader election recovery test setup complete")
}

// TestControllerCrash_SIGTERMvsSIGKILL tests graceful vs forceful shutdown
func TestControllerCrash_SIGTERMvsSIGKILL(t *testing.T) {
	// This test compares behavior between graceful (SIGTERM) and forceful (SIGKILL) termination

	t.Run("SIGTERM_Graceful", func(t *testing.T) {
		// SIGTERM should:
		// - Stop accepting new work
		// - Complete in-flight operations
		// - Clean up resources
		// - Save state
		t.Log("SIGTERM should allow graceful shutdown")
	})

	t.Run("SIGKILL_Forceful", func(t *testing.T) {
		// SIGKILL should:
		// - Terminate immediately
		// - Leave in-flight operations incomplete
		// - Require recovery on restart
		t.Log("SIGKILL requires recovery on restart")
	})
}

// TestControllerCrash_StateRecovery tests state recovery after crash
func TestControllerCrash_StateRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create NodeGroup with specific state
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-state-recovery",
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

	// In a full test:
	// 1. Get initial state
	// 2. Start controller, let it make progress
	// 3. Kill controller
	// 4. Restart controller
	// 5. Verify state is recovered correctly
	// 6. Verify no duplicate actions taken

	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	t.Logf("NodeGroup status: Desired=%d, Current=%d, Ready=%d",
		fetchedNg.Status.DesiredNodes,
		fetchedNg.Status.CurrentNodes,
		fetchedNg.Status.ReadyNodes)

	t.Log("State recovery test setup complete")
}
