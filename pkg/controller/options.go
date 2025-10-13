package controller

import (
	"fmt"
	"time"
)

// Options holds configuration options for the controller manager
type Options struct {
	// Kubeconfig is the path to the kubeconfig file.
	// If empty, uses in-cluster configuration
	Kubeconfig string

	// MetricsAddr is the address the metric endpoint binds to
	MetricsAddr string

	// HealthProbeAddr is the address the health probe endpoint binds to
	HealthProbeAddr string

	// EnableLeaderElection enables leader election for controller manager.
	// Enabling this will ensure there is only one active controller manager
	EnableLeaderElection bool

	// LeaderElectionID is the name of the ConfigMap that leader election will use
	LeaderElectionID string

	// LeaderElectionNamespace is the namespace where the leader election ConfigMap will be created
	LeaderElectionNamespace string

	// SyncPeriod is the period for syncing resources
	SyncPeriod time.Duration

	// VPSieSecretName is the name of the Kubernetes secret containing VPSie credentials
	VPSieSecretName string

	// VPSieSecretNamespace is the namespace of the VPSie credentials secret
	VPSieSecretNamespace string

	// LogLevel is the log verbosity level (debug, info, warn, error)
	LogLevel string

	// LogFormat is the log format (json, console)
	LogFormat string

	// DevelopmentMode enables development mode with more verbose logging
	DevelopmentMode bool
}

// NewDefaultOptions returns Options with default values
func NewDefaultOptions() *Options {
	return &Options{
		Kubeconfig:              "",
		MetricsAddr:             ":8080",
		HealthProbeAddr:         ":8081",
		EnableLeaderElection:    true,
		LeaderElectionID:        "vpsie-autoscaler-leader",
		LeaderElectionNamespace: "kube-system",
		SyncPeriod:              10 * time.Minute,
		VPSieSecretName:         "vpsie-secret",
		VPSieSecretNamespace:    "kube-system",
		LogLevel:                "info",
		LogFormat:               "json",
		DevelopmentMode:         false,
	}
}

// Validate validates the options and returns an error if any option is invalid
func (o *Options) Validate() error {
	if o.MetricsAddr == "" {
		return fmt.Errorf("metrics address cannot be empty")
	}

	if o.HealthProbeAddr == "" {
		return fmt.Errorf("health probe address cannot be empty")
	}

	if o.MetricsAddr == o.HealthProbeAddr {
		return fmt.Errorf("metrics address and health probe address cannot be the same")
	}

	if o.EnableLeaderElection {
		if o.LeaderElectionID == "" {
			return fmt.Errorf("leader election ID cannot be empty when leader election is enabled")
		}
		if o.LeaderElectionNamespace == "" {
			return fmt.Errorf("leader election namespace cannot be empty when leader election is enabled")
		}
	}

	if o.SyncPeriod <= 0 {
		return fmt.Errorf("sync period must be greater than zero")
	}

	if o.VPSieSecretName == "" {
		return fmt.Errorf("VPSie secret name cannot be empty")
	}

	if o.VPSieSecretNamespace == "" {
		return fmt.Errorf("VPSie secret namespace cannot be empty")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[o.LogLevel] {
		return fmt.Errorf("invalid log level '%s', must be one of: debug, info, warn, error", o.LogLevel)
	}

	// Validate log format
	validLogFormats := map[string]bool{
		"json":    true,
		"console": true,
	}
	if !validLogFormats[o.LogFormat] {
		return fmt.Errorf("invalid log format '%s', must be one of: json, console", o.LogFormat)
	}

	return nil
}

// Complete fills in any fields not set that are required to have valid data
func (o *Options) Complete() error {
	// Set defaults for empty fields
	defaults := NewDefaultOptions()

	if o.MetricsAddr == "" {
		o.MetricsAddr = defaults.MetricsAddr
	}

	if o.HealthProbeAddr == "" {
		o.HealthProbeAddr = defaults.HealthProbeAddr
	}

	if o.LeaderElectionID == "" {
		o.LeaderElectionID = defaults.LeaderElectionID
	}

	if o.LeaderElectionNamespace == "" {
		o.LeaderElectionNamespace = defaults.LeaderElectionNamespace
	}

	if o.SyncPeriod == 0 {
		o.SyncPeriod = defaults.SyncPeriod
	}

	if o.VPSieSecretName == "" {
		o.VPSieSecretName = defaults.VPSieSecretName
	}

	if o.VPSieSecretNamespace == "" {
		o.VPSieSecretNamespace = defaults.VPSieSecretNamespace
	}

	if o.LogLevel == "" {
		o.LogLevel = defaults.LogLevel
	}

	if o.LogFormat == "" {
		o.LogFormat = defaults.LogFormat
	}

	return nil
}
