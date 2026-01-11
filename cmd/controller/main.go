package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

var (
	// Version information (set via ldflags during build)
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"

	// Scheme for all Kubernetes API types
	scheme = runtime.NewScheme()
)

func init() {
	// Register standard Kubernetes types
	_ = clientgoscheme.AddToScheme(scheme)
	// Register our custom resources
	_ = autoscalerv1alpha1.AddToScheme(scheme)
}

func main() {
	// Set version info in environment for controller manager to use
	os.Setenv("VERSION", Version)
	os.Setenv("COMMIT", Commit)
	os.Setenv("BUILD_DATE", BuildDate)

	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// newRootCommand creates the root cobra command
func newRootCommand() *cobra.Command {
	opts := controller.NewDefaultOptions()

	cmd := &cobra.Command{
		Use:   "vpsie-autoscaler",
		Short: "VPSie Kubernetes Node Autoscaler",
		Long: `VPSie Kubernetes Node Autoscaler is an event-driven autoscaler that
dynamically provisions and optimizes Kubernetes nodes using the VPSie cloud platform.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(opts)
		},
		SilenceUsage: true,
	}

	// Add flags
	addFlags(cmd, opts)

	return cmd
}

// addFlags adds all CLI flags to the command
func addFlags(cmd *cobra.Command, opts *controller.Options) {
	flags := cmd.Flags()

	// Kubernetes configuration
	flags.StringVar(&opts.Kubeconfig, "kubeconfig", opts.Kubeconfig,
		"Path to kubeconfig file (optional, uses in-cluster config if not specified)")

	// Server configuration
	flags.StringVar(&opts.MetricsAddr, "metrics-addr", opts.MetricsAddr,
		"Address for the metrics server to bind to")
	flags.StringVar(&opts.HealthProbeAddr, "health-addr", opts.HealthProbeAddr,
		"Address for the health probe server to bind to")

	// Leader election configuration
	flags.BoolVar(&opts.EnableLeaderElection, "leader-election", opts.EnableLeaderElection,
		"Enable leader election for controller manager (for HA deployments)")
	flags.StringVar(&opts.LeaderElectionID, "leader-election-id", opts.LeaderElectionID,
		"Name of the ConfigMap used for leader election")
	flags.StringVar(&opts.LeaderElectionNamespace, "leader-election-namespace", opts.LeaderElectionNamespace,
		"Namespace for the leader election ConfigMap")

	// Controller configuration
	flags.DurationVar(&opts.SyncPeriod, "sync-period", opts.SyncPeriod,
		"Period for syncing resources with Kubernetes API")

	// VPSie API configuration
	flags.StringVar(&opts.VPSieSecretName, "vpsie-secret-name", opts.VPSieSecretName,
		"Name of the Kubernetes secret containing VPSie credentials")
	flags.StringVar(&opts.VPSieSecretNamespace, "vpsie-secret-namespace", opts.VPSieSecretNamespace,
		"Namespace of the VPSie credentials secret")

	// Dynamic NodeGroup creation configuration
	flags.StringVar(&opts.DefaultDatacenterID, "default-datacenter-id", opts.DefaultDatacenterID,
		"VPSie datacenter ID for dynamic NodeGroup creation")
	flags.StringSliceVar(&opts.DefaultOfferingIDs, "default-offering-ids", opts.DefaultOfferingIDs,
		"Comma-separated VPSie offering IDs for dynamic NodeGroup creation")
	flags.StringVar(&opts.ResourceIdentifier, "resource-identifier", opts.ResourceIdentifier,
		"VPSie Kubernetes cluster identifier for dynamic NodeGroup creation")
	flags.StringVar(&opts.KubernetesVersion, "kubernetes-version", opts.KubernetesVersion,
		"Kubernetes version for dynamic NodeGroups (e.g., v1.34.1)")
	flags.IntVar(&opts.KubeSizeID, "kube-size-id", opts.KubeSizeID,
		"VPSie Kubernetes size/package ID for dynamic NodeGroups (from k8s/offers API)")

	// Logging configuration
	flags.StringVar(&opts.LogLevel, "log-level", opts.LogLevel,
		"Log level (debug, info, warn, error)")
	flags.StringVar(&opts.LogFormat, "log-format", opts.LogFormat,
		"Log format (json, console)")
	flags.BoolVar(&opts.DevelopmentMode, "development", opts.DevelopmentMode,
		"Enable development mode with verbose logging")
}

// run starts the controller manager
func run(opts *controller.Options) error {
	// Complete and validate options
	if err := opts.Complete(); err != nil {
		return fmt.Errorf("failed to complete options: %w", err)
	}

	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	// Initialize structured logger using the logging package
	logger, err := logging.NewLogger(opts.DevelopmentMode)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync() // Ignore sync errors in defer
	}()

	// Set log level based on options
	logger = configureLogLevel(logger, opts.LogLevel)

	logger.Info("Initializing VPSie Kubernetes Autoscaler",
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.String("buildDate", BuildDate),
		zap.String("logLevel", opts.LogLevel),
		zap.String("logFormat", opts.LogFormat),
		zap.Bool("development", opts.DevelopmentMode),
	)

	// Register Prometheus metrics
	logger.Info("Registering Prometheus metrics")
	metrics.RegisterMetrics()

	// Set controller-runtime logger
	ctrl.SetLogger(logging.NewZapLogger(logger, opts.DevelopmentMode))

	// Build kubeconfig
	logger.Info("Building Kubernetes client configuration",
		zap.String("kubeconfig", getKubeconfigPath(opts.Kubeconfig)),
	)
	config, err := buildKubeConfig(opts.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Log configuration details
	logger.Info("Controller configuration",
		zap.String("metricsAddr", opts.MetricsAddr),
		zap.String("healthProbeAddr", opts.HealthProbeAddr),
		zap.Bool("leaderElection", opts.EnableLeaderElection),
		zap.String("leaderElectionID", opts.LeaderElectionID),
		zap.String("leaderElectionNamespace", opts.LeaderElectionNamespace),
		zap.Duration("syncPeriod", opts.SyncPeriod),
		zap.String("vpsieSecretName", opts.VPSieSecretName),
		zap.String("vpsieSecretNamespace", opts.VPSieSecretNamespace),
	)

	// Create controller manager
	logger.Info("Creating controller manager")
	mgr, err := controller.NewManager(config, opts)
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := setupSignalHandler()
	defer cancel()

	// Start the controller manager
	mgrLogger := mgr.GetLogger()
	mgrLogger.Info("Starting VPSie Kubernetes Autoscaler",
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.Bool("leaderElection", opts.EnableLeaderElection),
		zap.String("metricsAddr", opts.MetricsAddr),
		zap.String("healthAddr", opts.HealthProbeAddr),
	)

	// Start manager in a goroutine
	errCh := make(chan error, 1)
	go func() {
		mgrLogger.Info("Starting controller-runtime manager",
			zap.String("metricsAddress", opts.MetricsAddr),
			zap.String("healthAddress", opts.HealthProbeAddr),
		)
		if err := mgr.Start(ctx); err != nil {
			errCh <- fmt.Errorf("manager exited with error: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		mgrLogger.Info("Received shutdown signal, initiating graceful shutdown")

		// Create shutdown context with timeout
		shutdownTimeout := 30 * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		mgrLogger.Info("Performing graceful shutdown",
			zap.Duration("timeout", shutdownTimeout),
		)

		// Perform graceful shutdown
		if err := mgr.Shutdown(shutdownCtx); err != nil {
			mgrLogger.Error("Error during shutdown", zap.Error(err))
			return err
		}

		// Wait for manager to finish
		if err := <-errCh; err != nil {
			mgrLogger.Error("Manager stopped with error", zap.Error(err))
			return err
		}

		mgrLogger.Info("Controller stopped gracefully")
		return nil

	case err := <-errCh:
		if err != nil {
			mgrLogger.Error("Manager failed", zap.Error(err))
			return err
		}
		mgrLogger.Info("Manager stopped normally")
		return nil
	}
}

// buildKubeConfig creates a Kubernetes client configuration
func buildKubeConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		// Use out-of-cluster config from kubeconfig file
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
		return config, nil
	}

	// Use in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	return config, nil
}

// setupSignalHandler creates a context that is cancelled when the process receives
// SIGTERM or SIGINT signals. It handles graceful shutdown and forced exit on second signal.
func setupSignalHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal %v, starting graceful shutdown...\n", sig)
		cancel()

		// Force exit on second signal
		sig = <-sigCh
		fmt.Printf("\nReceived second signal %v, forcing immediate exit...\n", sig)
		os.Exit(1)
	}()

	return ctx, cancel
}

// configureLogLevel adjusts the logger's level based on the provided log level string
func configureLogLevel(logger *zap.Logger, logLevel string) *zap.Logger {
	var level zap.AtomicLevel

	switch logLevel {
	case "debug":
		level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return logger.WithOptions(zap.IncreaseLevel(level))
}

// getKubeconfigPath returns the kubeconfig path for logging purposes
func getKubeconfigPath(kubeconfig string) string {
	if kubeconfig != "" {
		return kubeconfig
	}
	return "in-cluster"
}
