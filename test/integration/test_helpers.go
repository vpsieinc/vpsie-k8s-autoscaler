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
	"strconv"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// IsHealthy checks if the controller is healthy
func (c *ControllerProcess) IsHealthy() bool {
	if c == nil || c.Cmd == nil || c.Cmd.Process == nil {
		return false
	}

	// Check if process is running
	if !isProcessRunning(c.PID) {
		return false
	}

	// Check health endpoint
	healthURL := fmt.Sprintf("http://localhost%s/healthz", c.HealthAddr)
	resp, err := http.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Stop gracefully stops the controller process
func (c *ControllerProcess) Stop() error {
	if c == nil || c.Cmd == nil || c.Cmd.Process == nil {
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if err := c.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try to kill the process
		_ = c.Cmd.Process.Kill()
		return err
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- c.Cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited gracefully
		cleanup(c)
		return err
	case <-time.After(30 * time.Second):
		// Timeout - force kill
		_ = c.Cmd.Process.Kill()
		cleanup(c)
		return fmt.Errorf("process did not exit gracefully")
	}
}

// Shutdown sends a shutdown signal to the controller
func (c *ControllerProcess) Shutdown() error {
	if c == nil || c.Cmd == nil || c.Cmd.Process == nil {
		return fmt.Errorf("invalid process")
	}

	// Send SIGTERM for graceful shutdown
	return c.Cmd.Process.Signal(syscall.SIGTERM)
}

// GetLogs returns the stdout and stderr logs from the controller
func (c *ControllerProcess) GetLogs() (stdout, stderr string, err error) {
	return readControllerLogs(c)
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

// startMultipleControllersWithLeaderElection starts multiple controller instances with leader election
func startMultipleControllersWithLeaderElection(count int, secretName, secretNS, leaderElectionID string) ([]*ControllerProcess, error) {
	controllers := make([]*ControllerProcess, 0, count)

	// Base ports - each controller gets unique ports
	baseMetricsPort := 19000
	baseHealthPort := 19100

	for i := 0; i < count; i++ {
		metricsPort := baseMetricsPort + i
		healthPort := baseHealthPort + i

		controller, err := startControllerWithLeaderElection(
			metricsPort,
			healthPort,
			secretName,
			secretNS,
			leaderElectionID,
		)
		if err != nil {
			// Clean up any controllers we already started
			stopAllControllers(controllers)
			return nil, fmt.Errorf("failed to start controller %d: %w", i, err)
		}

		controllers = append(controllers, controller)

		// Give each controller a moment to start
		time.Sleep(2 * time.Second)
	}

	return controllers, nil
}

// stopAllControllers stops all controller processes
func stopAllControllers(controllers []*ControllerProcess) {
	for _, controller := range controllers {
		if controller != nil {
			_ = controller.Stop()
		}
	}
}

// identifyLeader identifies which controller is the leader
func identifyLeader(controllers []*ControllerProcess) (*ControllerProcess, int) {
	var leader *ControllerProcess
	leaderCount := 0

	for _, controller := range controllers {
		if isControllerLeader(controller) {
			leader = controller
			leaderCount++
		}
	}

	return leader, leaderCount
}

// isControllerLeader checks if a controller is the leader
func isControllerLeader(controller *ControllerProcess) bool {
	if controller == nil || !isProcessRunning(controller.PID) {
		return false
	}

	// Check metrics for leader election status
	// Look for leader_election_master_status metric
	resp, err := http.Get(fmt.Sprintf("http://localhost%s/metrics", controller.MetricsAddr))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	metrics := string(body)

	// Check for leader election metric
	// leader_election_master_status{name="..."} 1 means leader
	lines := strings.Split(metrics, "\n")
	for _, line := range lines {
		if strings.Contains(line, "leader_election_master_status") && !strings.HasPrefix(line, "#") {
			if strings.Contains(line, " 1") {
				return true
			}
		}
	}

	// Alternative: Check if controller is actively reconciling
	// This is less reliable but can work if metrics aren't available
	reconcileCount := getMetricValueFromString(metrics, "vpsie_autoscaler_controller_reconcile_total")
	return reconcileCount > 0
}

// getMetricValueFromString extracts a metric value from metrics string
func getMetricValueFromString(metrics string, metricName string) float64 {
	lines := strings.Split(metrics, "\n")
	for _, line := range lines {
		if strings.Contains(line, metricName) && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				value := strings.TrimSpace(parts[len(parts)-1])
				if val, err := strconv.ParseFloat(value, 64); err == nil {
					return val
				}
			}
		}
	}
	return 0
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

// ========== Scaling Test Helper Functions ==========

// createUnschedulablePod creates a pod that cannot be scheduled due to resource requirements
func createUnschedulablePod(ctx context.Context, name, namespace, nodeGroup string, cpuRequest, memRequest string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"nodegroup": nodeGroup,
				"test":      "scaling",
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"nodegroup": nodeGroup,
			},
			Containers: []corev1.Container{
				{
					Name:    "test-container",
					Image:   "busybox:latest",
					Command: []string{"/bin/sh", "-c", "sleep 3600"},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpuRequest),
							corev1.ResourceMemory: resource.MustParse(memRequest),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	err := k8sClient.Create(ctx, pod)
	return pod, err
}

// markPodUnschedulable simulates a pod being unschedulable
func markPodUnschedulable(ctx context.Context, pod *corev1.Pod) error {
	pod.Status.Phase = corev1.PodPending
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:               corev1.PodScheduled,
			Status:             corev1.ConditionFalse,
			Reason:             "Unschedulable",
			Message:            "0/1 nodes are available: insufficient cpu",
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
		},
	}
	return k8sClient.Status().Update(ctx, pod)
}

// countNodesForNodeGroup counts VPSieNodes for a specific NodeGroup
func countNodesForNodeGroup(ctx context.Context, namespace, nodeGroupName string) (int, error) {
	var vpsieNodeList autoscalerv1alpha1.VPSieNodeList
	err := k8sClient.List(ctx, &vpsieNodeList, client.InNamespace(namespace))
	if err != nil {
		return 0, err
	}

	count := 0
	for _, node := range vpsieNodeList.Items {
		if node.Spec.NodeGroupName == nodeGroupName {
			count++
		}
	}
	return count, nil
}

// waitForNodeGroupScaling waits for a NodeGroup to scale to a specific size
func waitForNodeGroupScaling(ctx context.Context, namespace, nodeGroupName string, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		count, err := countNodesForNodeGroup(ctx, namespace, nodeGroupName)
		if err == nil && count == expectedCount {
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	currentCount, _ := countNodesForNodeGroup(ctx, namespace, nodeGroupName)
	return fmt.Errorf("NodeGroup %s: expected %d nodes, but found %d after %v",
		nodeGroupName, expectedCount, currentCount, timeout)
}

// getVPSieNodePhases returns a map of node phases for a NodeGroup
func getVPSieNodePhases(ctx context.Context, namespace, nodeGroupName string) (map[autoscalerv1alpha1.VPSieNodePhase]int, error) {
	var vpsieNodeList autoscalerv1alpha1.VPSieNodeList
	err := k8sClient.List(ctx, &vpsieNodeList, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	phases := make(map[autoscalerv1alpha1.VPSieNodePhase]int)
	for _, node := range vpsieNodeList.Items {
		if node.Spec.NodeGroupName == nodeGroupName {
			phases[node.Status.Phase]++
		}
	}

	return phases, nil
}

// waitForAllNodesReady waits for all nodes in a NodeGroup to become ready
func waitForAllNodesReady(ctx context.Context, namespace, nodeGroupName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		phases, err := getVPSieNodePhases(ctx, namespace, nodeGroupName)
		if err != nil {
			return err
		}

		// Check if all nodes are ready
		totalNodes := 0
		readyNodes := 0
		for phase, count := range phases {
			totalNodes += count
			if phase == autoscalerv1alpha1.VPSieNodePhaseReady {
				readyNodes += count
			}
		}

		if totalNodes > 0 && totalNodes == readyNodes {
			// All nodes are ready
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	phases, _ := getVPSieNodePhases(ctx, namespace, nodeGroupName)
	return fmt.Errorf("not all nodes became ready within %v, current phases: %v", timeout, phases)
}

// simulateNodeReady simulates a VPSieNode becoming ready
func simulateNodeReady(ctx context.Context, namespace, nodeName string) error {
	node := &autoscalerv1alpha1.VPSieNode{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      nodeName,
		Namespace: namespace,
	}, node)
	if err != nil {
		return err
	}

	node.Status.Phase = autoscalerv1alpha1.VPSieNodePhaseReady
	node.Status.Conditions = []autoscalerv1alpha1.VPSieNodeCondition{
		{
			Type:               autoscalerv1alpha1.VPSieNodeConditionReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "NodeReady",
			Message:            "Node is ready and joined to cluster",
		},
	}

	return k8sClient.Status().Update(ctx, node)
}

// validateScalingMetrics checks if scaling metrics are properly updated
func validateScalingMetrics(metricsURL string, expectedScaleUps, expectedScaleDowns int) error {
	resp, err := http.Get(metricsURL + "/metrics")
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read metrics: %w", err)
	}

	metrics := string(body)

	// Parse scale-up operations
	scaleUpCount := parseMetricValue(metrics, "vpsie_autoscaler_scale_up_operations_total")
	if scaleUpCount < float64(expectedScaleUps) {
		return fmt.Errorf("expected at least %d scale-up operations, got %f", expectedScaleUps, scaleUpCount)
	}

	// Parse scale-down operations (if implemented)
	if expectedScaleDowns > 0 {
		scaleDownCount := parseMetricValue(metrics, "vpsie_autoscaler_scale_down_operations_total")
		if scaleDownCount < float64(expectedScaleDowns) {
			return fmt.Errorf("expected at least %d scale-down operations, got %f", expectedScaleDowns, scaleDownCount)
		}
	}

	return nil
}

// parseMetricValue extracts a metric value from Prometheus format text
func parseMetricValue(metrics, metricName string) float64 {
	lines := strings.Split(metrics, "\n")
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

// ScalingTestConfig holds configuration for scaling tests
type ScalingTestConfig struct {
	NodeGroupName     string
	Namespace         string
	InitialMinNodes   int32
	InitialMaxNodes   int32
	TargetNodes       int32
	PodCount          int
	PodCPU            string
	PodMemory         string
	MockServerLatency time.Duration
	ScaleTimeout      time.Duration
}

// runScalingTest executes a complete scaling test scenario
func runScalingTest(ctx context.Context, config ScalingTestConfig) error {
	// Create NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.NodeGroupName,
			Namespace: config.Namespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     config.InitialMinNodes,
			MaxNodes:     config.InitialMaxNodes,
			DatacenterID: "us-west-1",
			OfferingID:   "standard-4cpu-8gb",
		},
	}

	if err := k8sClient.Create(ctx, nodeGroup); err != nil {
		return fmt.Errorf("failed to create NodeGroup: %w", err)
	}

	// Wait for initial provisioning
	if err := waitForNodeGroupScaling(ctx, config.Namespace, config.NodeGroupName,
		int(config.InitialMinNodes), config.ScaleTimeout); err != nil {
		return fmt.Errorf("initial provisioning failed: %w", err)
	}

	// Create unschedulable pods to trigger scale-up
	for i := 0; i < config.PodCount; i++ {
		podName := fmt.Sprintf("%s-pod-%d", config.NodeGroupName, i)
		pod, err := createUnschedulablePod(ctx, podName, config.Namespace,
			config.NodeGroupName, config.PodCPU, config.PodMemory)
		if err != nil {
			return fmt.Errorf("failed to create pod %s: %w", podName, err)
		}

		if err := markPodUnschedulable(ctx, pod); err != nil {
			fmt.Printf("Warning: failed to mark pod %s as unschedulable: %v\n", podName, err)
		}
	}

	// Wait for scale-up to target
	if err := waitForNodeGroupScaling(ctx, config.Namespace, config.NodeGroupName,
		int(config.TargetNodes), config.ScaleTimeout); err != nil {
		return fmt.Errorf("scale-up failed: %w", err)
	}

	return nil
}

// cleanupScalingTest cleans up resources created during a scaling test
func cleanupScalingTest(ctx context.Context, namespace, nodeGroupName string) {
	// Delete NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeGroupName,
			Namespace: namespace,
		},
	}
	_ = k8sClient.Delete(ctx, nodeGroup)

	// Delete all pods with nodegroup label
	podList := &corev1.PodList{}
	_ = k8sClient.List(ctx, podList, client.InNamespace(namespace),
		client.MatchingLabels{"nodegroup": nodeGroupName})
	for _, pod := range podList.Items {
		_ = k8sClient.Delete(ctx, &pod)
	}

	// Delete all VPSieNodes for this NodeGroup
	vpsieNodeList := &autoscalerv1alpha1.VPSieNodeList{}
	_ = k8sClient.List(ctx, vpsieNodeList, client.InNamespace(namespace))
	for _, node := range vpsieNodeList.Items {
		if node.Spec.NodeGroupName == nodeGroupName {
			_ = k8sClient.Delete(ctx, &node)
		}
	}
}
