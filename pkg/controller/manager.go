package controller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller/nodegroup"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller/vpsienode"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/events"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/scaler"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
)

// ControllerManager manages the lifecycle of all controllers
type ControllerManager struct {
	config            *rest.Config
	options           *Options
	mgr               ctrl.Manager
	vpsieClient       *vpsieclient.Client
	k8sClient         kubernetes.Interface
	metricsClient     metricsv1beta1.Interface
	scaleDownManager  *scaler.ScaleDownManager
	healthChecker     *HealthChecker
	logger            *zap.Logger
	scheme            *runtime.Scheme
	eventWatcher      *events.EventWatcher
	scaleUpController *events.ScaleUpController
}

// NewManager creates a new ControllerManager
func NewManager(config *rest.Config, opts *Options) (*ControllerManager, error) {
	if config == nil {
		return nil, fmt.Errorf("kubeconfig cannot be nil")
	}

	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	// Validate options
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	// Create logger
	logger, err := newLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create scheme and register types
	scheme := runtime.NewScheme()
	// Register core Kubernetes types (Node, Secret, Pod, etc.)
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}
	// Register our custom CRDs
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add CRDs to scheme: %w", err)
	}

	// Create controller-runtime manager
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: opts.MetricsAddr,
		},
		HealthProbeBindAddress:  opts.HealthProbeAddr,
		LeaderElection:          opts.EnableLeaderElection,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.LeaderElectionNamespace,
		Logger:                  zapr.NewLogger(logger),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Add field indexer for pod's spec.nodeName field
	// This is required for efficient pod listing by node name (used in drainer)
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return nil, fmt.Errorf("failed to add pod node name indexer: %w", err)
	}

	// Create Kubernetes clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create metrics clientset
	metricsClient, err := metricsv1beta1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	// Create VPSie API client
	ctx := context.Background()
	vpsieClient, err := vpsieclient.NewClient(ctx, k8sClient, &vpsieclient.ClientOptions{
		SecretName:      opts.VPSieSecretName,
		SecretNamespace: opts.VPSieSecretNamespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create VPSie client: %w", err)
	}

	// Create ScaleDownManager
	scaleDownConfig := scaler.DefaultConfig()
	scaleDownManager := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, scaleDownConfig)

	// Create cost calculator for cost-aware NodeGroup selection
	costCalculator := cost.NewCalculator(vpsieClient)

	// Create ResourceAnalyzer for scale-up decisions with cost-aware selection
	resourceAnalyzer := events.NewResourceAnalyzer(logger, costCalculator)

	// Create health checker
	healthChecker := NewHealthChecker(vpsieClient)

	cm := &ControllerManager{
		config:           config,
		options:          opts,
		mgr:              mgr,
		vpsieClient:      vpsieClient,
		k8sClient:        k8sClient,
		metricsClient:    metricsClient,
		scaleDownManager: scaleDownManager,
		healthChecker:    healthChecker,
		logger:           logger,
		scheme:           scheme,
	}

	// Create DynamicNodeGroupCreator for automatic NodeGroup provisioning
	// Uses default template - can be customized via configuration
	dynamicCreator := events.NewDynamicNodeGroupCreator(
		mgr.GetClient(),
		logger,
		nil, // Uses default template
	)

	// Create EventWatcher and ScaleUpController for pending pod detection
	// ScaleUpController is created first with nil watcher, then wired up after EventWatcher is created
	scaleUpController := events.NewScaleUpController(
		mgr.GetClient(),
		resourceAnalyzer,
		nil, // Will set EventWatcher after creation via SetWatcher
		dynamicCreator,
		logger,
	)

	eventWatcher := events.NewEventWatcher(
		mgr.GetClient(),
		k8sClient,
		logger,
		scaleUpController.HandleScaleUp,
	)

	// Wire up the ScaleUpController with the EventWatcher
	scaleUpController.SetWatcher(eventWatcher)

	cm.eventWatcher = eventWatcher
	cm.scaleUpController = scaleUpController

	// Add health checks to manager
	if err := cm.setupHealthChecks(); err != nil {
		return nil, fmt.Errorf("failed to setup health checks: %w", err)
	}

	// Setup controllers
	if err := cm.setupControllers(); err != nil {
		return nil, fmt.Errorf("failed to setup controllers: %w", err)
	}

	return cm, nil
}

// setupHealthChecks configures the health check endpoints
func (cm *ControllerManager) setupHealthChecks() error {
	// Add healthz endpoint (liveness probe)
	if err := cm.mgr.AddHealthzCheck("healthz", cm.healthzCheck); err != nil {
		return fmt.Errorf("failed to add healthz check: %w", err)
	}

	// Add readyz endpoint (readiness probe)
	if err := cm.mgr.AddReadyzCheck("readyz", cm.readyzCheck); err != nil {
		return fmt.Errorf("failed to add readyz check: %w", err)
	}

	// Add ping check
	if err := cm.mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add ping check: %w", err)
	}

	// Add VPSie API check
	if err := cm.mgr.AddReadyzCheck("vpsie-api", cm.vpsieAPICheck); err != nil {
		return fmt.Errorf("failed to add VPSie API check: %w", err)
	}

	return nil
}

// setupControllers sets up all controllers with the manager
func (cm *ControllerManager) setupControllers() error {
	// Setup NodeGroup controller
	nodeGroupReconciler := nodegroup.NewNodeGroupReconciler(
		cm.mgr.GetClient(),
		cm.scheme,
		cm.vpsieClient,
		cm.logger,
		cm.scaleDownManager,
	)

	if err := nodeGroupReconciler.SetupWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to setup NodeGroup controller: %w", err)
	}

	cm.logger.Info("Successfully registered NodeGroup controller")

	// Setup VPSieNode controller
	// Note: Node configuration is handled by VPSie API via QEMU agent
	vpsieNodeReconciler := vpsienode.NewVPSieNodeReconciler(
		cm.mgr.GetClient(),
		cm.scheme,
		cm.vpsieClient,
		cm.logger,
		cm.options.SSHKeyIDs,
	)

	if err := vpsieNodeReconciler.SetupWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to setup VPSieNode controller: %w", err)
	}

	cm.logger.Info("Successfully registered VPSieNode controller")

	return nil
}

// healthzCheck implements the liveness probe
func (cm *ControllerManager) healthzCheck(req *http.Request) error {
	if !cm.healthChecker.IsHealthy() {
		lastErr := cm.healthChecker.LastError()
		if lastErr != nil {
			return fmt.Errorf("health check failed: %w", lastErr)
		}
		return fmt.Errorf("controller is not healthy")
	}
	return nil
}

// readyzCheck implements the readiness probe
func (cm *ControllerManager) readyzCheck(req *http.Request) error {
	if !cm.healthChecker.IsReady() {
		lastErr := cm.healthChecker.LastError()
		if lastErr != nil {
			return fmt.Errorf("readiness check failed: %w", lastErr)
		}
		return fmt.Errorf("controller is not ready")
	}
	return nil
}

// vpsieAPICheck verifies connectivity to the VPSie API
func (cm *ControllerManager) vpsieAPICheck(req *http.Request) error {
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	_, err := cm.vpsieClient.ListVMs(ctx)
	if err != nil {
		return fmt.Errorf("VPSie API not reachable: %w", err)
	}
	return nil
}

// Start starts the controller manager and blocks until the context is cancelled
func (cm *ControllerManager) Start(ctx context.Context) error {
	cm.logger.Info("Starting VPSie Kubernetes Autoscaler",
		zap.String("version", os.Getenv("VERSION")),
		zap.String("commit", os.Getenv("COMMIT")),
		zap.Bool("leader_election", cm.options.EnableLeaderElection),
		zap.String("metrics_addr", cm.options.MetricsAddr),
		zap.String("health_addr", cm.options.HealthProbeAddr),
	)

	// Start health checker
	if err := cm.healthChecker.Start(ctx); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	cm.logger.Info("Health checks initialized successfully")

	// Start event watcher for pending pod detection
	if cm.eventWatcher != nil {
		if err := cm.eventWatcher.Start(ctx); err != nil {
			return fmt.Errorf("failed to start event watcher: %w", err)
		}
		cm.logger.Info("Event watcher started for pending pod detection")
	}

	// Log VPSie client info
	cm.logger.Info("VPSie API client initialized",
		zap.String("endpoint", cm.vpsieClient.GetBaseURL()),
	)

	// Start node utilization metrics collection
	cm.startMetricsCollection(ctx)

	// Start the manager (this blocks until context is cancelled)
	cm.logger.Info("Starting controller-runtime manager")
	if err := cm.mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	return nil
}

// startMetricsCollection starts a background goroutine to collect node utilization metrics
func (cm *ControllerManager) startMetricsCollection(ctx context.Context) {
	cm.logger.Info("Starting node utilization metrics collection",
		zap.Duration("interval", scaler.DefaultMetricsCollectionInterval))

	go func() {
		ticker := time.NewTicker(scaler.DefaultMetricsCollectionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				cm.logger.Info("Stopping metrics collection")
				return
			case <-ticker.C:
				// Use timeout to prevent goroutine leak if metrics API hangs
				// Timeout should be less than collection interval to avoid overlap
				// Note: cancel() is called immediately (not deferred) to avoid context leak in loop
				metricsCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				if err := cm.scaleDownManager.UpdateNodeUtilization(metricsCtx); err != nil {
					cm.logger.Error("Failed to update node utilization",
						zap.Error(err))
				}
				cancel() // Immediately clean up context resources
			}
		}
	}()
}

// GetManager returns the controller-runtime manager
func (cm *ControllerManager) GetManager() ctrl.Manager {
	return cm.mgr
}

// GetVPSieClient returns the VPSie API client
func (cm *ControllerManager) GetVPSieClient() *vpsieclient.Client {
	return cm.vpsieClient
}

// GetKubernetesClient returns the Kubernetes clientset
func (cm *ControllerManager) GetKubernetesClient() kubernetes.Interface {
	return cm.k8sClient
}

// GetLogger returns the logger
func (cm *ControllerManager) GetLogger() *zap.Logger {
	return cm.logger
}

// GetHealthChecker returns the health checker
func (cm *ControllerManager) GetHealthChecker() *HealthChecker {
	return cm.healthChecker
}

// GetScaleDownManager returns the scale-down manager
func (cm *ControllerManager) GetScaleDownManager() *scaler.ScaleDownManager {
	return cm.scaleDownManager
}

// Shutdown gracefully shuts down the controller manager
func (cm *ControllerManager) Shutdown(ctx context.Context) error {
	cm.logger.Info("Initiating graceful shutdown")

	// Mark as not ready to stop receiving new traffic
	cm.healthChecker.SetReady(false)

	// Wait a bit to allow load balancers to remove this instance
	shutdownDelay := 5 * time.Second
	cm.logger.Info("Waiting before shutdown", zap.Duration("delay", shutdownDelay))

	select {
	case <-time.After(shutdownDelay):
	case <-ctx.Done():
		cm.logger.Warn("Shutdown deadline exceeded during delay")
	}

	cm.logger.Info("Shutdown complete")
	return nil
}

// newLogger creates a new zap logger based on options
func newLogger(opts *Options) (*zap.Logger, error) {
	var config zap.Config

	if opts.DevelopmentMode {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	// Set log level
	switch opts.LogLevel {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Set encoding
	if opts.LogFormat == "console" {
		config.Encoding = "console"
	} else {
		config.Encoding = "json"
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}
