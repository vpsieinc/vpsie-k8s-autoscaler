//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// ControllerProcess represents a running controller process
type ControllerProcess struct {
	Cmd           *exec.Cmd
	PID           int
	MetricsAddr   string
	HealthAddr    string
	SecretName    string
	SecretNS      string
	LogFile       *os.File
	StdoutLogPath string
	StderrLogPath string
}

// startControllerInBackground starts the controller as a separate process
func startControllerInBackground(metricsPort, healthPort int, secretName, secretNS string) (*ControllerProcess, error) {
	// Build the controller binary if it doesn't exist
	binaryPath := filepath.Join("..", "..", "bin", "vpsie-autoscaler")

	// Check if binary exists, if not try to build it
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try building the binary
		buildCmd := exec.Command("make", "build")
		buildCmd.Dir = filepath.Join("..", "..")
		if err := buildCmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to build controller binary: %w", err)
		}
	}

	// Create log files for stdout and stderr
	tmpDir := os.TempDir()
	stdoutPath := filepath.Join(tmpDir, fmt.Sprintf("controller-stdout-%d.log", time.Now().Unix()))
	stderrPath := filepath.Join(tmpDir, fmt.Sprintf("controller-stderr-%d.log", time.Now().Unix()))

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout log file: %w", err)
	}

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return nil, fmt.Errorf("failed to create stderr log file: %w", err)
	}

	// Prepare controller arguments
	metricsAddr := fmt.Sprintf(":%d", metricsPort)
	healthAddr := fmt.Sprintf(":%d", healthPort)

	cmd := exec.Command(
		binaryPath,
		"--metrics-addr", metricsAddr,
		"--health-addr", healthAddr,
		"--leader-election=false",
		"--vpsie-secret", secretName,
		"--vpsie-namespace", secretNS,
		"--log-level", "debug",
		"--kubeconfig", testKubeconfig,
	)

	// Set output to log files
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	// Start the process in a new process group so we can send signals
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return nil, fmt.Errorf("failed to start controller process: %w", err)
	}

	proc := &ControllerProcess{
		Cmd:           cmd,
		PID:           cmd.Process.Pid,
		MetricsAddr:   metricsAddr,
		HealthAddr:    healthAddr,
		SecretName:    secretName,
		SecretNS:      secretNS,
		StdoutLogPath: stdoutPath,
		StderrLogPath: stderrPath,
	}

	// Note: We don't close the files here as the process is still writing to them
	// They will be closed when the process is stopped

	return proc, nil
}

// waitForControllerReady polls the health endpoint until controller is ready
func waitForControllerReady(healthAddr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://localhost%s/healthz", healthAddr)

	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("controller did not become ready within %v", timeout)
}

// sendSignal sends a signal to the controller process
func sendSignal(proc *ControllerProcess, sig syscall.Signal) error {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return fmt.Errorf("invalid process")
	}

	return proc.Cmd.Process.Signal(sig)
}

// waitForShutdown waits for the controller process to exit
func waitForShutdown(proc *ControllerProcess, timeout time.Duration) error {
	if proc == nil || proc.Cmd == nil {
		return fmt.Errorf("invalid process")
	}

	done := make(chan error, 1)
	go func() {
		done <- proc.Cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited
		return err
	case <-time.After(timeout):
		// Timeout - force kill
		if proc.Cmd.Process != nil {
			_ = proc.Cmd.Process.Kill()
		}
		return fmt.Errorf("process did not exit within %v", timeout)
	}
}

// killController forcefully kills the controller process
func killController(proc *ControllerProcess) error {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return nil
	}

	return proc.Cmd.Process.Kill()
}

// cleanup cleans up controller process resources
func cleanup(proc *ControllerProcess) {
	if proc == nil {
		return
	}

	// Make sure process is killed
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		_ = proc.Cmd.Process.Kill()
		_ = proc.Cmd.Wait()
	}

	// Clean up log files
	if proc.StdoutLogPath != "" {
		_ = os.Remove(proc.StdoutLogPath)
	}
	if proc.StderrLogPath != "" {
		_ = os.Remove(proc.StderrLogPath)
	}
}

// isProcessRunning checks if a process is still running
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. We need to send signal 0 to check if it's alive
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// getHealthStatus gets the current health status from the health endpoint
func getHealthStatus(healthAddr string, endpoint string) (int, error) {
	url := fmt.Sprintf("http://localhost%s%s", healthAddr, endpoint)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// waitForHealthStatusChange waits for health status to change from expected
func waitForHealthStatusChange(healthAddr string, endpoint string, currentStatus int, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := getHealthStatus(healthAddr, endpoint)
		if err != nil || status != currentStatus {
			return status, err
		}

		time.Sleep(200 * time.Millisecond)
	}

	return currentStatus, fmt.Errorf("health status did not change within %v", timeout)
}

// readControllerLogs reads the controller logs from the log files
func readControllerLogs(proc *ControllerProcess) (stdout, stderr string, err error) {
	if proc == nil {
		return "", "", fmt.Errorf("invalid process")
	}

	if proc.StdoutLogPath != "" {
		stdoutBytes, err := os.ReadFile(proc.StdoutLogPath)
		if err == nil {
			stdout = string(stdoutBytes)
		}
	}

	if proc.StderrLogPath != "" {
		stderrBytes, err := os.ReadFile(proc.StderrLogPath)
		if err == nil {
			stderr = string(stderrBytes)
		}
	}

	return stdout, stderr, nil
}

// createTestSecret creates a VPSie secret for testing
func createTestSecret(ctx context.Context, name, namespace, vpsieURL string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"clientId":     []byte("test-client-id"),
			"clientSecret": []byte("test-client-secret"),
			"url":          []byte(vpsieURL),
		},
	}

	return k8sClient.Create(ctx, secret)
}

// deleteTestSecret deletes a test secret
func deleteTestSecret(ctx context.Context, name, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	return k8sClient.Delete(ctx, secret)
}

// startControllerWithLeaderElection starts a controller with leader election enabled
func startControllerWithLeaderElection(metricsPort, healthPort int, secretName, secretNS, leaderElectionID string) (*ControllerProcess, error) {
	// Build the controller binary if it doesn't exist
	binaryPath := filepath.Join("..", "..", "bin", "vpsie-autoscaler")

	// Check if binary exists, if not try to build it
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		buildCmd := exec.Command("make", "build")
		buildCmd.Dir = filepath.Join("..", "..")
		if err := buildCmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to build controller binary: %w", err)
		}
	}

	// Create log files
	tmpDir := os.TempDir()
	stdoutPath := filepath.Join(tmpDir, fmt.Sprintf("controller-le-%d-stdout-%d.log", healthPort, time.Now().Unix()))
	stderrPath := filepath.Join(tmpDir, fmt.Sprintf("controller-le-%d-stderr-%d.log", healthPort, time.Now().Unix()))

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout log file: %w", err)
	}

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return nil, fmt.Errorf("failed to create stderr log file: %w", err)
	}

	metricsAddr := fmt.Sprintf(":%d", metricsPort)
	healthAddr := fmt.Sprintf(":%d", healthPort)

	cmd := exec.Command(
		binaryPath,
		"--metrics-addr", metricsAddr,
		"--health-addr", healthAddr,
		"--leader-election=true",
		"--leader-election-id", leaderElectionID,
		"--leader-election-namespace", testNamespace,
		"--vpsie-secret", secretName,
		"--vpsie-namespace", secretNS,
		"--log-level", "info",
		"--kubeconfig", testKubeconfig,
	)

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return nil, fmt.Errorf("failed to start controller process: %w", err)
	}

	proc := &ControllerProcess{
		Cmd:           cmd,
		PID:           cmd.Process.Pid,
		MetricsAddr:   metricsAddr,
		HealthAddr:    healthAddr,
		SecretName:    secretName,
		SecretNS:      secretNS,
		StdoutLogPath: stdoutPath,
		StderrLogPath: stderrPath,
	}

	return proc, nil
}

// startMultipleControllers starts N controllers with leader election
func startMultipleControllers(count int, baseMetricsPort, baseHealthPort int, secretName, secretNS, leaderElectionID string) ([]*ControllerProcess, error) {
	controllers := make([]*ControllerProcess, 0, count)

	for i := 0; i < count; i++ {
		metricsPort := baseMetricsPort + i
		healthPort := baseHealthPort + i

		proc, err := startControllerWithLeaderElection(metricsPort, healthPort, secretName, secretNS, leaderElectionID)
		if err != nil {
			// Cleanup already started controllers
			for _, p := range controllers {
				cleanup(p)
			}
			return nil, fmt.Errorf("failed to start controller %d: %w", i, err)
		}

		controllers = append(controllers, proc)
	}

	return controllers, nil
}

// identifyLeader identifies which controller is the current leader
func identifyLeader(controllers []*ControllerProcess, timeout time.Duration) (*ControllerProcess, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		for _, proc := range controllers {
			// Leader should have readyz returning 200
			status, err := getHealthStatus(proc.HealthAddr, "/readyz")
			if err == nil && status == http.StatusOK {
				return proc, nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil, fmt.Errorf("no leader identified within %v", timeout)
}

// waitForLeaderElection waits for exactly one leader to be elected
func waitForLeaderElection(controllers []*ControllerProcess, timeout time.Duration) (*ControllerProcess, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var leader *ControllerProcess
		leaderCount := 0

		for _, proc := range controllers {
			status, err := getHealthStatus(proc.HealthAddr, "/readyz")
			if err == nil && status == http.StatusOK {
				leader = proc
				leaderCount++
			}
		}

		if leaderCount == 1 {
			return leader, nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil, fmt.Errorf("leader election did not complete within %v", timeout)
}

// verifyOnlyOneLeader verifies that exactly one controller is the leader
func verifyOnlyOneLeader(controllers []*ControllerProcess) (leader *ControllerProcess, nonLeaders []*ControllerProcess, err error) {
	leaderCount := 0

	for _, proc := range controllers {
		status, err := getHealthStatus(proc.HealthAddr, "/readyz")
		if err == nil && status == http.StatusOK {
			leader = proc
			leaderCount++
		} else {
			nonLeaders = append(nonLeaders, proc)
		}
	}

	if leaderCount != 1 {
		return nil, nil, fmt.Errorf("expected exactly 1 leader, found %d", leaderCount)
	}

	return leader, nonLeaders, nil
}

// cleanupMultipleControllers cleans up multiple controller processes
func cleanupMultipleControllers(controllers []*ControllerProcess) {
	for _, proc := range controllers {
		cleanup(proc)
	}
}

// getLeaderElectionLease gets the leader election lease object
func getLeaderElectionLease(ctx context.Context, leaderElectionID, namespace string) (string, error) {
	// This would query the Kubernetes API for the Lease object
	// For now, return empty - actual implementation would use clientset
	return "", fmt.Errorf("not implemented - requires coordination.k8s.io/v1 Lease support")
}

// verifyLeaderMetrics checks leader election metrics from the metrics endpoint
func verifyLeaderMetrics(proc *ControllerProcess) (map[string]string, error) {
	metricsURL := fmt.Sprintf("http://localhost%s/metrics", proc.MetricsAddr)
	resp, err := http.Get(metricsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	metrics := make(map[string]string)
	metricsText := string(body)

	// Look for leader election metrics
	lines := strings.Split(metricsText, "\n")
	for _, line := range lines {
		if strings.Contains(line, "leader_election") {
			metrics[line] = line
		}
	}

	return metrics, nil
}

// waitForAllControllersReady waits for all controllers to report healthy (not necessarily ready as leader)
func waitForAllControllersReady(controllers []*ControllerProcess, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		allHealthy := true

		for _, proc := range controllers {
			status, err := getHealthStatus(proc.HealthAddr, "/healthz")
			if err != nil || status != http.StatusOK {
				allHealthy = false
				break
			}
		}

		if allHealthy {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("not all controllers became healthy within %v", timeout)
}

// countVPSieNodes counts VPSieNodes in a namespace with optional label selector
func countVPSieNodes(ctx context.Context, namespace string) (int, error) {
	var vpsieNodeList autoscalerv1alpha1.VPSieNodeList
	err := k8sClient.List(ctx, &vpsieNodeList, client.InNamespace(namespace))
	if err != nil {
		return 0, err
	}
	return len(vpsieNodeList.Items), nil
}

// waitForVPSieNodeCount waits for a specific number of VPSieNodes to exist
func waitForVPSieNodeCount(ctx context.Context, namespace string, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		count, err := countVPSieNodes(ctx, namespace)
		if err == nil && count == expectedCount {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	currentCount, _ := countVPSieNodes(ctx, namespace)
	return fmt.Errorf("expected %d VPSieNodes, found %d after %v", expectedCount, currentCount, timeout)
}

// waitForVPSieNodeCountAtLeast waits for at least N VPSieNodes to exist
func waitForVPSieNodeCountAtLeast(ctx context.Context, namespace string, minCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		count, err := countVPSieNodes(ctx, namespace)
		if err == nil && count >= minCount {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	currentCount, _ := countVPSieNodes(ctx, namespace)
	return fmt.Errorf("expected at least %d VPSieNodes, found %d after %v", minCount, currentCount, timeout)
}

// getNodeGroupStatus gets the current status of a NodeGroup
func getNodeGroupStatus(ctx context.Context, name, namespace string) (*autoscalerv1alpha1.NodeGroupStatus, error) {
	var ng autoscalerv1alpha1.NodeGroup
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &ng)
	if err != nil {
		return nil, err
	}
	return &ng.Status, nil
}

// waitForNodeGroupDesiredNodes waits for NodeGroup to have specific desired node count
func waitForNodeGroupDesiredNodes(ctx context.Context, name, namespace string, desiredCount int32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := getNodeGroupStatus(ctx, name, namespace)
		if err == nil && status.DesiredNodes == desiredCount {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	status, _ := getNodeGroupStatus(ctx, name, namespace)
	actualCount := int32(0)
	if status != nil {
		actualCount = status.DesiredNodes
	}
	return fmt.Errorf("expected DesiredNodes=%d, got %d after %v", desiredCount, actualCount, timeout)
}
