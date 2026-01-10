//go:build integration || performance
// +build integration performance

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestCluster represents a test Kubernetes cluster
type TestCluster struct {
	Config    *rest.Config
	Client    client.Client
	Clientset kubernetes.Interface
	Namespace string
	TempDir   string
	cleanups  []func()
	mu        sync.Mutex
}

// NewTestCluster creates a new test cluster instance
func NewTestCluster(t *testing.T, namespace string) (*TestCluster, error) {
	// Load kubeconfig
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Setup scheme
	scheme := k8sruntime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core/v1 to scheme: %w", err)
	}
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add autoscaler/v1alpha1 to scheme: %w", err)
	}

	// Create controller-runtime client
	k8sClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// Create temp directory for test artifacts
	tempDir, err := os.MkdirTemp("", "autoscaler-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Ensure namespace exists
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err = k8sClient.Create(context.Background(), ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	tc := &TestCluster{
		Config:    config,
		Client:    k8sClient,
		Clientset: clientset,
		Namespace: namespace,
		TempDir:   tempDir,
		cleanups:  []func(){},
	}

	// Add namespace cleanup
	tc.AddCleanup(func() {
		// Delete namespace if it was created for this test
		if namespace != "default" && namespace != "kube-system" {
			_ = k8sClient.Delete(context.Background(), ns)
		}
	})

	// Add temp dir cleanup
	tc.AddCleanup(func() {
		os.RemoveAll(tempDir)
	})

	return tc, nil
}

// AddCleanup adds a cleanup function to be run on teardown
func (tc *TestCluster) AddCleanup(fn func()) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.cleanups = append(tc.cleanups, fn)
}

// Teardown cleans up all test resources
func (tc *TestCluster) Teardown() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Run cleanups in reverse order
	for i := len(tc.cleanups) - 1; i >= 0; i-- {
		tc.cleanups[i]()
	}
}

// WaitForCRDs waits for custom resource definitions to be available
func (tc *TestCluster) WaitForCRDs(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := tc.Clientset.Discovery().ServerResourcesForGroupVersion("autoscaler.vpsie.com/v1alpha1")
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// retry
		}
	}
	return fmt.Errorf("CRDs not available after %v", timeout)
}

// ControllerInstance represents a running controller instance
type ControllerInstance struct {
	Process       *os.Process
	MetricsPort   int
	HealthPort    int
	LogFile       string
	SecretName    string
	SecretNS      string
	cmd           *exec.Cmd
	stdoutFile    *os.File
	stderrFile    *os.File
	metricsClient *http.Client
	healthClient  *http.Client
}

// NewControllerInstance creates a new controller instance
func NewControllerInstance(metricsPort, healthPort int, secretName, secretNS string) *ControllerInstance {
	return &ControllerInstance{
		MetricsPort:   metricsPort,
		HealthPort:    healthPort,
		SecretName:    secretName,
		SecretNS:      secretNS,
		metricsClient: &http.Client{Timeout: 5 * time.Second},
		healthClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Start starts the controller process
func (ci *ControllerInstance) Start(ctx context.Context) error {
	// Create log files
	logDir := "/tmp"
	ci.LogFile = filepath.Join(logDir, fmt.Sprintf("controller-%d.log", ci.MetricsPort))

	stdoutFile, err := os.Create(ci.LogFile + ".stdout")
	if err != nil {
		return fmt.Errorf("failed to create stdout log: %w", err)
	}
	ci.stdoutFile = stdoutFile

	stderrFile, err := os.Create(ci.LogFile + ".stderr")
	if err != nil {
		return fmt.Errorf("failed to create stderr log: %w", err)
	}
	ci.stderrFile = stderrFile

	// Build command
	binaryPath := filepath.Join(".", "bin", "vpsie-autoscaler")
	ci.cmd = exec.CommandContext(ctx, binaryPath,
		"--metrics-addr", fmt.Sprintf(":%d", ci.MetricsPort),
		"--health-addr", fmt.Sprintf(":%d", ci.HealthPort),
		"--vpsie-secret-name", ci.SecretName,
		"--vpsie-secret-namespace", ci.SecretNS,
		"--log-level", "debug",
		"--disable-leader-election",
	)

	ci.cmd.Stdout = ci.stdoutFile
	ci.cmd.Stderr = ci.stderrFile
	ci.cmd.Env = os.Environ()

	// Start the process
	if err := ci.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start controller: %w", err)
	}

	ci.Process = ci.cmd.Process
	return nil
}

// Stop stops the controller process
func (ci *ControllerInstance) Stop() error {
	if ci.cmd != nil && ci.cmd.Process != nil {
		// Send graceful shutdown signal
		ci.cmd.Process.Signal(os.Interrupt)

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- ci.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(30 * time.Second):
			// Force kill after timeout
			ci.cmd.Process.Kill()
		}
	}

	// Close log files
	if ci.stdoutFile != nil {
		ci.stdoutFile.Close()
	}
	if ci.stderrFile != nil {
		ci.stderrFile.Close()
	}

	return nil
}

// WaitForHealthy waits for the controller to become healthy
func (ci *ControllerInstance) WaitForHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := ci.healthClient.Get(fmt.Sprintf("http://localhost:%d/healthz", ci.HealthPort))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("controller did not become healthy within %v", timeout)
}

// GetMetrics fetches metrics from the controller
func (ci *ControllerInstance) GetMetrics() (map[string]*dto.MetricFamily, error) {
	resp, err := ci.metricsClient.Get(fmt.Sprintf("http://localhost:%d/metrics", ci.MetricsPort))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return mf, nil
}

// GetHealth checks the health status
func (ci *ControllerInstance) GetHealth() (bool, error) {
	resp, err := ci.healthClient.Get(fmt.Sprintf("http://localhost:%d/healthz", ci.HealthPort))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetReadiness checks the readiness status
func (ci *ControllerInstance) GetReadiness() (bool, error) {
	resp, err := ci.healthClient.Get(fmt.Sprintf("http://localhost:%d/readyz", ci.HealthPort))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// MockVPSieServerManager manages mock VPSie server instances
type MockVPSieServerManager struct {
	servers []*MockVPSieServer
	mu      sync.Mutex
}

// NewMockVPSieServerManager creates a new server manager
func NewMockVPSieServerManager() *MockVPSieServerManager {
	return &MockVPSieServerManager{
		servers: []*MockVPSieServer{},
	}
}

// CreateServer creates and starts a new mock server
func (m *MockVPSieServerManager) CreateServer() *MockVPSieServer {
	m.mu.Lock()
	defer m.mu.Unlock()

	server := NewMockVPSieServer()
	m.servers = append(m.servers, server)
	return server
}

// CreateServerWithConfig creates a server with specific configuration
func (m *MockVPSieServerManager) CreateServerWithConfig(config MockServerConfig) *MockVPSieServer {
	server := m.CreateServer()
	server.Latency = config.Latency
	server.ErrorRate = config.ErrorRate
	server.RateLimitRemaining = config.RateLimitRemaining
	server.StateTransitions = config.StateTransitions
	return server
}

// StopAll stops all managed servers
func (m *MockVPSieServerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, server := range m.servers {
		server.Close()
	}
	m.servers = []*MockVPSieServer{}
}

// MockServerConfig holds mock server configuration
type MockServerConfig struct {
	Latency            time.Duration
	ErrorRate          float64
	RateLimitRemaining int
	StateTransitions   []VMStateTransition
}

// MetricParser provides utilities for parsing Prometheus metrics
type MetricParser struct {
	families map[string]*dto.MetricFamily
}

// NewMetricParser creates a new metric parser from raw metrics text
func NewMetricParser(metricsText string) (*MetricParser, error) {
	parser := expfmt.TextParser{}
	families, err := parser.TextToMetricFamilies(strings.NewReader(metricsText))
	if err != nil {
		return nil, err
	}

	return &MetricParser{
		families: families,
	}, nil
}

// GetCounter retrieves a counter metric value
func (mp *MetricParser) GetCounter(name string, labels prometheus.Labels) (float64, error) {
	family, ok := mp.families[name]
	if !ok {
		return 0, fmt.Errorf("metric %s not found", name)
	}

	if family.GetType() != dto.MetricType_COUNTER {
		return 0, fmt.Errorf("metric %s is not a counter", name)
	}

	for _, metric := range family.GetMetric() {
		if mp.matchLabels(metric.GetLabel(), labels) {
			return metric.GetCounter().GetValue(), nil
		}
	}

	return 0, fmt.Errorf("metric %s with labels %v not found", name, labels)
}

// GetGauge retrieves a gauge metric value
func (mp *MetricParser) GetGauge(name string, labels prometheus.Labels) (float64, error) {
	family, ok := mp.families[name]
	if !ok {
		return 0, fmt.Errorf("metric %s not found", name)
	}

	if family.GetType() != dto.MetricType_GAUGE {
		return 0, fmt.Errorf("metric %s is not a gauge", name)
	}

	for _, metric := range family.GetMetric() {
		if mp.matchLabels(metric.GetLabel(), labels) {
			return metric.GetGauge().GetValue(), nil
		}
	}

	return 0, fmt.Errorf("metric %s with labels %v not found", name, labels)
}

// GetHistogram retrieves histogram metric statistics
func (mp *MetricParser) GetHistogram(name string, labels prometheus.Labels) (*HistogramStats, error) {
	family, ok := mp.families[name]
	if !ok {
		return nil, fmt.Errorf("metric %s not found", name)
	}

	if family.GetType() != dto.MetricType_HISTOGRAM {
		return nil, fmt.Errorf("metric %s is not a histogram", name)
	}

	for _, metric := range family.GetMetric() {
		if mp.matchLabels(metric.GetLabel(), labels) {
			hist := metric.GetHistogram()
			return &HistogramStats{
				Count: hist.GetSampleCount(),
				Sum:   hist.GetSampleSum(),
				Mean:  hist.GetSampleSum() / float64(hist.GetSampleCount()),
			}, nil
		}
	}

	return nil, fmt.Errorf("metric %s with labels %v not found", name, labels)
}

// matchLabels checks if metric labels match expected labels
func (mp *MetricParser) matchLabels(metricLabels []*dto.LabelPair, expectedLabels prometheus.Labels) bool {
	if len(expectedLabels) == 0 {
		return true
	}

	for key, value := range expectedLabels {
		found := false
		for _, label := range metricLabels {
			if label.GetName() == key && label.GetValue() == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// HistogramStats holds histogram statistics
type HistogramStats struct {
	Count uint64
	Sum   float64
	Mean  float64
}

// EventWatcher watches for Kubernetes events
type EventWatcher struct {
	client    kubernetes.Interface
	namespace string
	events    []corev1.Event
	stopCh    chan struct{}
	mu        sync.RWMutex
}

// NewEventWatcher creates a new event watcher
func NewEventWatcher(client kubernetes.Interface, namespace string) *EventWatcher {
	return &EventWatcher{
		client:    client,
		namespace: namespace,
		events:    []corev1.Event{},
		stopCh:    make(chan struct{}),
	}
}

// Start starts watching for events
func (ew *EventWatcher) Start(ctx context.Context) error {
	watcher, err := ew.client.CoreV1().Events(ew.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to start event watch: %w", err)
	}

	go func() {
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ew.stopCh:
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					return
				}

				if event.Type == watch.Added {
					if e, ok := event.Object.(*corev1.Event); ok {
						ew.mu.Lock()
						ew.events = append(ew.events, *e)
						ew.mu.Unlock()
					}
				}
			}
		}
	}()

	return nil
}

// Stop stops watching for events
func (ew *EventWatcher) Stop() {
	close(ew.stopCh)
}

// GetEvents returns all captured events
func (ew *EventWatcher) GetEvents() []corev1.Event {
	ew.mu.RLock()
	defer ew.mu.RUnlock()

	events := make([]corev1.Event, len(ew.events))
	copy(events, ew.events)
	return events
}

// GetEventsForObject returns events for a specific object
func (ew *EventWatcher) GetEventsForObject(kind, name string) []corev1.Event {
	ew.mu.RLock()
	defer ew.mu.RUnlock()

	var filtered []corev1.Event
	for _, event := range ew.events {
		if event.InvolvedObject.Kind == kind && event.InvolvedObject.Name == name {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// ResourceCreationHelpers provides utilities for creating test resources

// CreateTestNodeGroup creates a NodeGroup for testing
func CreateTestNodeGroup(ctx context.Context, client client.Client, name, namespace string, minNodes, maxNodes int32) (*autoscalerv1alpha1.NodeGroup, error) {
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:          minNodes,
			MaxNodes:          maxNodes,
			TargetUtilization: 70,
			DatacenterID:      "test-dc",
			OfferingID:        "test-offering",
		},
	}

	err := client.Create(ctx, nodeGroup)
	return nodeGroup, err
}

// CreateTestVPSieNode creates a VPSieNode for testing
func CreateTestVPSieNode(ctx context.Context, client client.Client, name, namespace, nodeGroupName string) (*autoscalerv1alpha1.VPSieNode, error) {
	vpsieNode := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalerv1alpha1.VPSieNodeSpec{
			NodeGroupName: nodeGroupName,
			OfferingID:    "test-offering",
			DatacenterID:  "test-dc",
		},
	}

	err := client.Create(ctx, vpsieNode)
	return vpsieNode, err
}

// CreateTestPod creates a pod for testing
func CreateTestPod(ctx context.Context, client client.Client, name, namespace string, resources corev1.ResourceRequirements) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      "test-container",
					Image:     "busybox:latest",
					Command:   []string{"/bin/sh", "-c", "sleep 3600"},
					Resources: resources,
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	err := client.Create(ctx, pod)
	return pod, err
}

// ResourceTracker tracks resource usage during tests
type ResourceTracker struct {
	startTime     time.Time
	measurements  []ResourceMeasurement
	mu            sync.Mutex
	stopCh        chan struct{}
	controllerPID int
}

// ResourceMeasurement represents a point-in-time resource measurement
type ResourceMeasurement struct {
	Timestamp  time.Time
	CPUPercent float64
	MemoryMB   float64
	Goroutines int
	OpenFiles  int
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(controllerPID int) *ResourceTracker {
	return &ResourceTracker{
		startTime:     time.Now(),
		measurements:  []ResourceMeasurement{},
		stopCh:        make(chan struct{}),
		controllerPID: controllerPID,
	}
}

// Start starts tracking resources
func (rt *ResourceTracker) Start(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-rt.stopCh:
				return
			case <-ticker.C:
				measurement := rt.measure()
				rt.mu.Lock()
				rt.measurements = append(rt.measurements, measurement)
				rt.mu.Unlock()
			}
		}
	}()
}

// Stop stops tracking resources
func (rt *ResourceTracker) Stop() {
	close(rt.stopCh)
}

// measure takes a resource measurement
func (rt *ResourceTracker) measure() ResourceMeasurement {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return ResourceMeasurement{
		Timestamp:  time.Now(),
		MemoryMB:   float64(m.Alloc) / 1024 / 1024,
		Goroutines: runtime.NumGoroutine(),
		OpenFiles:  rt.countOpenFiles(),
	}
}

// countOpenFiles counts open file descriptors
func (rt *ResourceTracker) countOpenFiles() int {
	// This is Linux-specific, adjust for other platforms
	pid := rt.controllerPID
	if pid == 0 {
		pid = os.Getpid()
	}

	fdPath := fmt.Sprintf("/proc/%d/fd", pid)
	files, err := os.ReadDir(fdPath)
	if err != nil {
		return -1
	}

	return len(files)
}

// GetReport generates a resource usage report
func (rt *ResourceTracker) GetReport() ResourceReport {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if len(rt.measurements) == 0 {
		return ResourceReport{}
	}

	var maxMemory, avgMemory, totalMemory float64
	var maxGoroutines, avgGoroutines, totalGoroutines int

	for _, m := range rt.measurements {
		if m.MemoryMB > maxMemory {
			maxMemory = m.MemoryMB
		}
		totalMemory += m.MemoryMB

		if m.Goroutines > maxGoroutines {
			maxGoroutines = m.Goroutines
		}
		totalGoroutines += m.Goroutines
	}

	count := float64(len(rt.measurements))
	avgMemory = totalMemory / count
	avgGoroutines = int(float64(totalGoroutines) / count)

	return ResourceReport{
		Duration:         time.Since(rt.startTime),
		MaxMemoryMB:      maxMemory,
		AvgMemoryMB:      avgMemory,
		MaxGoroutines:    maxGoroutines,
		AvgGoroutines:    avgGoroutines,
		MeasurementCount: len(rt.measurements),
	}
}

// ResourceReport contains resource usage statistics
type ResourceReport struct {
	Duration         time.Duration
	MaxMemoryMB      float64
	AvgMemoryMB      float64
	MaxGoroutines    int
	AvgGoroutines    int
	MeasurementCount int
}

// String returns a formatted report
func (r ResourceReport) String() string {
	return fmt.Sprintf(`Resource Usage Report:
Duration: %v
Memory (Max/Avg): %.2f MB / %.2f MB
Goroutines (Max/Avg): %d / %d
Measurements: %d`,
		r.Duration,
		r.MaxMemoryMB, r.AvgMemoryMB,
		r.MaxGoroutines, r.AvgGoroutines,
		r.MeasurementCount)
}

// TestReporter generates test reports
type TestReporter struct {
	results []TestResult
	mu      sync.Mutex
}

// TestResult represents a test execution result
type TestResult struct {
	Name      string
	Duration  time.Duration
	Passed    bool
	Error     error
	Metrics   map[string]float64
	Resources ResourceReport
}

// NewTestReporter creates a new test reporter
func NewTestReporter() *TestReporter {
	return &TestReporter{
		results: []TestResult{},
	}
}

// AddResult adds a test result
func (tr *TestReporter) AddResult(result TestResult) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.results = append(tr.results, result)
}

// GenerateReport generates a test report
func (tr *TestReporter) GenerateReport() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	var report strings.Builder
	report.WriteString("=== Integration Test Report ===\n\n")

	totalTests := len(tr.results)
	passedTests := 0
	totalDuration := time.Duration(0)

	for _, result := range tr.results {
		if result.Passed {
			passedTests++
		}
		totalDuration += result.Duration
	}

	report.WriteString(fmt.Sprintf("Summary: %d/%d tests passed (%.1f%%)\n",
		passedTests, totalTests, float64(passedTests)/float64(totalTests)*100))
	report.WriteString(fmt.Sprintf("Total Duration: %v\n\n", totalDuration))

	report.WriteString("Test Results:\n")
	for _, result := range tr.results {
		status := "✓ PASS"
		if !result.Passed {
			status = "✗ FAIL"
		}

		report.WriteString(fmt.Sprintf("\n%s %s (%v)\n", status, result.Name, result.Duration))

		if result.Error != nil {
			report.WriteString(fmt.Sprintf("  Error: %v\n", result.Error))
		}

		if len(result.Metrics) > 0 {
			report.WriteString("  Metrics:\n")
			for key, value := range result.Metrics {
				report.WriteString(fmt.Sprintf("    %s: %.2f\n", key, value))
			}
		}

		if result.Resources.MeasurementCount > 0 {
			report.WriteString(fmt.Sprintf("  %s\n", result.Resources.String()))
		}
	}

	return report.String()
}

// SaveReport saves the report to a file
func (tr *TestReporter) SaveReport(filename string) error {
	report := tr.GenerateReport()
	return os.WriteFile(filename, []byte(report), 0644)
}

// GenerateJSONReport generates a JSON report
func (tr *TestReporter) GenerateJSONReport() ([]byte, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	type JSONReport struct {
		Summary struct {
			Total    int           `json:"total"`
			Passed   int           `json:"passed"`
			Failed   int           `json:"failed"`
			Duration time.Duration `json:"duration"`
		} `json:"summary"`
		Results []TestResult `json:"results"`
	}

	report := JSONReport{}
	report.Summary.Total = len(tr.results)

	for _, result := range tr.results {
		if result.Passed {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
		report.Summary.Duration += result.Duration
	}

	report.Results = tr.results

	return json.MarshalIndent(report, "", "  ")
}
