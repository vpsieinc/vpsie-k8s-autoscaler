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

	// SSHKeyIDs are the VPSie SSH key IDs to inject into provisioned nodes
	// Optional: can be empty if SSH keys are not required
	SSHKeyIDs []string

	// Dynamic NodeGroup creation template options
	// These are used when the autoscaler creates NodeGroups on-the-fly for pending pods

	// DefaultDatacenterID is the VPSie datacenter ID for dynamic NodeGroups
	DefaultDatacenterID string

	// DefaultOfferingIDs are the VPSie offering IDs available for dynamic NodeGroups
	DefaultOfferingIDs []string

	// ResourceIdentifier is the VPSie Kubernetes cluster identifier
	ResourceIdentifier string

	// KubernetesVersion is the Kubernetes version for dynamic NodeGroups (e.g., "v1.34.1")
	KubernetesVersion string

	// KubeSizeID is the VPSie Kubernetes size/package ID for dynamic NodeGroups
	// Get available IDs from the k8s/offers API endpoint
	KubeSizeID int

	// FailedVPSieNodeTTL is the duration after which failed VPSieNodes are automatically deleted
	// Set to 0 to disable automatic cleanup
	FailedVPSieNodeTTL time.Duration

	// Webhook configuration

	// EnableWebhook enables the validating webhook server
	EnableWebhook bool

	// WebhookAddr is the address the webhook server binds to
	WebhookAddr string

	// WebhookCertDir is the directory containing TLS certificates for the webhook
	WebhookCertDir string

	// WebhookCertFile is the name of the TLS certificate file
	WebhookCertFile string

	// WebhookKeyFile is the name of the TLS key file
	WebhookKeyFile string

	// Sentry configuration

	// SentryDSN is the Sentry Data Source Name (can also be set via SENTRY_DSN env var)
	SentryDSN string

	// SentryEnvironment is the deployment environment (e.g., "production", "staging")
	SentryEnvironment string

	// SentryTracesSampleRate is the sample rate for performance traces (0.0 to 1.0)
	SentryTracesSampleRate float64

	// SentryErrorSampleRate is the sample rate for error events (0.0 to 1.0)
	SentryErrorSampleRate float64
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
		SSHKeyIDs:               nil, // No SSH keys by default
		DefaultDatacenterID:     "",  // Must be set for dynamic NodeGroup creation
		DefaultOfferingIDs:      nil, // Must be set for dynamic NodeGroup creation
		ResourceIdentifier:      "",  // Must be set for dynamic NodeGroup creation
		KubernetesVersion:       "",  // Must be set for dynamic NodeGroup creation
		KubeSizeID:              0,   // Must be set for dynamic NodeGroup creation
		FailedVPSieNodeTTL:      30 * time.Minute,
		EnableWebhook:           false,
		WebhookAddr:             ":9443",
		WebhookCertDir:          "/var/run/webhook-certs",
		WebhookCertFile:         "tls.crt",
		WebhookKeyFile:          "tls.key",
		SentryDSN:               "",  // Set via SENTRY_DSN env var or --sentry-dsn flag
		SentryEnvironment:       "",  // Defaults to "development" if not set
		SentryTracesSampleRate:  0.1, // 10% of transactions
		SentryErrorSampleRate:   1.0, // 100% of errors
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

	// Validate FailedVPSieNodeTTL (0 is valid, meaning disabled)
	if o.FailedVPSieNodeTTL < 0 {
		return fmt.Errorf("failed VPSieNode TTL cannot be negative")
	}

	// Validate webhook configuration
	if o.EnableWebhook {
		if o.WebhookAddr == "" {
			return fmt.Errorf("webhook address cannot be empty when webhook is enabled")
		}
		if o.WebhookCertDir == "" {
			return fmt.Errorf("webhook cert directory cannot be empty when webhook is enabled")
		}
		if o.WebhookCertFile == "" {
			return fmt.Errorf("webhook cert file cannot be empty when webhook is enabled")
		}
		if o.WebhookKeyFile == "" {
			return fmt.Errorf("webhook key file cannot be empty when webhook is enabled")
		}
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

	// Webhook defaults
	if o.WebhookAddr == "" {
		o.WebhookAddr = defaults.WebhookAddr
	}
	if o.WebhookCertDir == "" {
		o.WebhookCertDir = defaults.WebhookCertDir
	}
	if o.WebhookCertFile == "" {
		o.WebhookCertFile = defaults.WebhookCertFile
	}
	if o.WebhookKeyFile == "" {
		o.WebhookKeyFile = defaults.WebhookKeyFile
	}

	return nil
}
