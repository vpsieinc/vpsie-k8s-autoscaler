package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultOptions(t *testing.T) {
	opts := NewDefaultOptions()

	assert.NotNil(t, opts)
	assert.Equal(t, ":8080", opts.MetricsAddr)
	assert.Equal(t, ":8081", opts.HealthProbeAddr)
	assert.True(t, opts.EnableLeaderElection)
	assert.Equal(t, "vpsie-autoscaler-leader", opts.LeaderElectionID)
	assert.Equal(t, "kube-system", opts.LeaderElectionNamespace)
	assert.Equal(t, 10*time.Minute, opts.SyncPeriod)
	assert.Equal(t, "vpsie-secret", opts.VPSieSecretName)
	assert.Equal(t, "kube-system", opts.VPSieSecretNamespace)
	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "json", opts.LogFormat)
	assert.False(t, opts.DevelopmentMode)
}

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Options
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid default options",
			opts:    NewDefaultOptions(),
			wantErr: false,
		},
		{
			name: "empty metrics address",
			opts: &Options{
				MetricsAddr:             "",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "metrics address cannot be empty",
		},
		{
			name: "empty health probe address",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         "",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "health probe address cannot be empty",
		},
		{
			name: "same metrics and health probe address",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8080",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "metrics address and health probe address cannot be the same",
		},
		{
			name: "empty leader election ID when enabled",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "leader election ID cannot be empty when leader election is enabled",
		},
		{
			name: "empty leader election namespace when enabled",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "leader election namespace cannot be empty when leader election is enabled",
		},
		{
			name: "zero sync period",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              0,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "sync period must be greater than zero",
		},
		{
			name: "negative sync period",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              -time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "sync period must be greater than zero",
		},
		{
			name: "empty VPSie secret name",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "VPSie secret name cannot be empty",
		},
		{
			name: "empty VPSie secret namespace",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "VPSie secret namespace cannot be empty",
		},
		{
			name: "invalid log level",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "invalid",
				LogFormat:               "json",
			},
			wantErr: true,
			errMsg:  "invalid log level 'invalid', must be one of: debug, info, warn, error",
		},
		{
			name: "invalid log format",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "invalid",
			},
			wantErr: true,
			errMsg:  "invalid log format 'invalid', must be one of: json, console",
		},
		{
			name: "leader election disabled with empty ID",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    false,
				LeaderElectionID:        "",
				LeaderElectionNamespace: "",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "json",
			},
			wantErr: false,
		},
		{
			name: "valid with debug log level",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "debug",
				LogFormat:               "json",
			},
			wantErr: false,
		},
		{
			name: "valid with console log format",
			opts: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				EnableLeaderElection:    true,
				LeaderElectionID:        "test",
				LeaderElectionNamespace: "default",
				SyncPeriod:              time.Minute,
				VPSieSecretName:         "secret",
				VPSieSecretNamespace:    "default",
				LogLevel:                "info",
				LogFormat:               "console",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOptions_Complete(t *testing.T) {
	tests := []struct {
		name     string
		opts     *Options
		expected *Options
	}{
		{
			name: "empty options get defaults",
			opts: &Options{},
			expected: &Options{
				MetricsAddr:             ":8080",
				HealthProbeAddr:         ":8081",
				LeaderElectionID:        "vpsie-autoscaler-leader",
				LeaderElectionNamespace: "kube-system",
				SyncPeriod:              10 * time.Minute,
				VPSieSecretName:         "vpsie-secret",
				VPSieSecretNamespace:    "kube-system",
				LogLevel:                "info",
				LogFormat:               "json",
			},
		},
		{
			name: "partial options get missing defaults",
			opts: &Options{
				MetricsAddr:     ":9090",
				HealthProbeAddr: ":9091",
			},
			expected: &Options{
				MetricsAddr:             ":9090",
				HealthProbeAddr:         ":9091",
				LeaderElectionID:        "vpsie-autoscaler-leader",
				LeaderElectionNamespace: "kube-system",
				SyncPeriod:              10 * time.Minute,
				VPSieSecretName:         "vpsie-secret",
				VPSieSecretNamespace:    "kube-system",
				LogLevel:                "info",
				LogFormat:               "json",
			},
		},
		{
			name: "fully specified options unchanged",
			opts: &Options{
				MetricsAddr:             ":9090",
				HealthProbeAddr:         ":9091",
				EnableLeaderElection:    false,
				LeaderElectionID:        "custom-leader",
				LeaderElectionNamespace: "custom-namespace",
				SyncPeriod:              5 * time.Minute,
				VPSieSecretName:         "custom-secret",
				VPSieSecretNamespace:    "custom-ns",
				LogLevel:                "debug",
				LogFormat:               "console",
				DevelopmentMode:         true,
			},
			expected: &Options{
				MetricsAddr:             ":9090",
				HealthProbeAddr:         ":9091",
				EnableLeaderElection:    false,
				LeaderElectionID:        "custom-leader",
				LeaderElectionNamespace: "custom-namespace",
				SyncPeriod:              5 * time.Minute,
				VPSieSecretName:         "custom-secret",
				VPSieSecretNamespace:    "custom-ns",
				LogLevel:                "debug",
				LogFormat:               "console",
				DevelopmentMode:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Complete()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tt.opts)
		})
	}
}
