package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

var (
	// Version information (set via ldflags during build)
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Config holds the controller configuration
type Config struct {
	KubeConfig      string
	VPSieSecretName string
	VPSieNamespace  string
	ResyncInterval  time.Duration
	LogLevel        string
}

func main() {
	// Parse command line flags
	config := &Config{}
	flag.StringVar(&config.KubeConfig, "kubeconfig", "", "Path to kubeconfig file (optional, uses in-cluster config if not specified)")
	flag.StringVar(&config.VPSieSecretName, "vpsie-secret", "vpsie-secret", "Name of the Kubernetes secret containing VPSie API credentials")
	flag.StringVar(&config.VPSieNamespace, "vpsie-namespace", "kube-system", "Namespace of the VPSie secret")
	flag.DurationVar(&config.ResyncInterval, "resync-interval", 30*time.Second, "Resync interval for controller reconciliation")
	flag.StringVar(&config.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	showVersion := flag.Bool("version", false, "Show version information and exit")
	flag.Parse()

	// Show version information
	if *showVersion {
		fmt.Printf("VPSie Kubernetes Autoscaler\n")
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Commit:     %s\n", Commit)
		fmt.Printf("  Build Date: %s\n", BuildDate)
		os.Exit(0)
	}

	// Print startup banner
	fmt.Printf("Starting VPSie Kubernetes Autoscaler %s (commit: %s)\n", Version, Commit)

	// Create Kubernetes client
	k8sConfig, err := buildKubeConfig(config.KubeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building kubeconfig: %v\n", err)
		os.Exit(1)
	}

	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	// Create VPSie API client
	ctx := context.Background()
	vpsieClient, err := client.NewClient(ctx, k8sClient, &client.ClientOptions{
		SecretName:      config.VPSieSecretName,
		SecretNamespace: config.VPSieNamespace,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating VPSie API client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully initialized VPSie API client (endpoint: %s)\n", vpsieClient.GetBaseURL())

	// Setup signal handling for graceful shutdown
	stopCh := setupSignalHandler()

	// Run the controller
	if err := run(ctx, k8sClient, vpsieClient, config, stopCh); err != nil {
		fmt.Fprintf(os.Stderr, "Error running controller: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Controller stopped gracefully")
}

// buildKubeConfig creates a Kubernetes client configuration
func buildKubeConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		// Use out-of-cluster config from kubeconfig file
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// Use in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}
	return config, nil
}

// run executes the main controller loop
func run(ctx context.Context, k8sClient kubernetes.Interface, vpsieClient *client.Client, config *Config, stopCh <-chan struct{}) error {
	fmt.Printf("Controller running with resync interval: %s\n", config.ResyncInterval)

	// Create a ticker for periodic reconciliation
	ticker := time.NewTicker(config.ResyncInterval)
	defer ticker.Stop()

	// Initial status check
	if err := performHealthCheck(ctx, vpsieClient); err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	fmt.Println("Controller is ready and watching for changes...")

	// Main control loop
	for {
		select {
		case <-stopCh:
			fmt.Println("Received shutdown signal, stopping controller...")
			return nil

		case <-ticker.C:
			// Periodic reconciliation would happen here
			// For now, just perform a health check
			if err := performHealthCheck(ctx, vpsieClient); err != nil {
				fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
			} else {
				fmt.Println("Health check passed")
			}
		}
	}
}

// performHealthCheck verifies connectivity to the VPSie API
func performHealthCheck(ctx context.Context, vpsieClient *client.Client) error {
	// Create a context with timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try to list VMs to verify API connectivity
	// We'll just discard the result - we only care if it succeeds
	_, err := vpsieClient.ListVMs(checkCtx)
	if err != nil {
		return fmt.Errorf("VPSie API health check failed: %w", err)
	}

	return nil
}

// setupSignalHandler creates a channel that receives OS signals for graceful shutdown
func setupSignalHandler() <-chan struct{} {
	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		close(stopCh)
		<-sigCh
		os.Exit(1) // Force exit on second signal
	}()

	return stopCh
}
