package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller"
)

var (
	// Version information (set via ldflags during build)
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
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

	// Build kubeconfig
	config, err := buildKubeConfig(opts.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Create controller manager
	mgr, err := controller.NewManager(config, opts)
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := setupSignalHandler()
	defer cancel()

	// Start the controller manager
	logger := mgr.GetLogger()
	logger.Info("Starting VPSie Kubernetes Autoscaler")

	// Start manager in a goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := mgr.Start(ctx); err != nil {
			errCh <- fmt.Errorf("manager exited with error: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		logger.Info("Received shutdown signal")

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Perform graceful shutdown
		if err := mgr.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during shutdown", err)
			return err
		}

		// Wait for manager to finish
		if err := <-errCh; err != nil {
			logger.Error("Manager stopped with error", err)
			return err
		}

		logger.Info("Controller stopped gracefully")
		return nil

	case err := <-errCh:
		if err != nil {
			logger.Error("Manager failed", err)
			return err
		}
		logger.Info("Manager stopped")
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
// SIGTERM or SIGINT signals
func setupSignalHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal, starting graceful shutdown...")
		cancel()

		// Force exit on second signal
		<-sigCh
		fmt.Println("\nReceived second signal, forcing exit...")
		os.Exit(1)
	}()

	return ctx, cancel
}
