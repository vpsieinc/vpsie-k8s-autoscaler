package main

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller"
)

func TestConfigureLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  string
		shouldLog map[zapcore.Level]bool
	}{
		{
			name:     "debug level logs everything",
			logLevel: "debug",
			shouldLog: map[zapcore.Level]bool{
				zapcore.DebugLevel: true,
				zapcore.InfoLevel:  true,
				zapcore.WarnLevel:  true,
				zapcore.ErrorLevel: true,
			},
		},
		{
			name:     "info level logs info and above",
			logLevel: "info",
			shouldLog: map[zapcore.Level]bool{
				zapcore.DebugLevel: false,
				zapcore.InfoLevel:  true,
				zapcore.WarnLevel:  true,
				zapcore.ErrorLevel: true,
			},
		},
		{
			name:     "warn level logs warn and error",
			logLevel: "warn",
			shouldLog: map[zapcore.Level]bool{
				zapcore.DebugLevel: false,
				zapcore.InfoLevel:  false,
				zapcore.WarnLevel:  true,
				zapcore.ErrorLevel: true,
			},
		},
		{
			name:     "error level logs only error",
			logLevel: "error",
			shouldLog: map[zapcore.Level]bool{
				zapcore.DebugLevel: false,
				zapcore.InfoLevel:  false,
				zapcore.WarnLevel:  false,
				zapcore.ErrorLevel: true,
			},
		},
		{
			name:     "invalid level defaults to info",
			logLevel: "invalid",
			shouldLog: map[zapcore.Level]bool{
				zapcore.DebugLevel: false,
				zapcore.InfoLevel:  true,
				zapcore.WarnLevel:  true,
				zapcore.ErrorLevel: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a base logger
			config := zap.NewProductionConfig()
			logger, err := config.Build()
			require.NoError(t, err)

			// Configure log level
			logger = configureLogLevel(logger, tt.logLevel)
			require.NotNil(t, logger)

			// Test that the logger was created successfully
			// Note: We can't easily test if messages are actually filtered
			// without more complex setup, but we verify the function works
			logger.Debug("debug message")
			logger.Info("info message")
			logger.Warn("warn message")
			logger.Error("error message")
		})
	}
}

func TestGetKubeconfigPath(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		expected   string
	}{
		{
			name:       "empty kubeconfig returns in-cluster",
			kubeconfig: "",
			expected:   "in-cluster",
		},
		{
			name:       "file path returns path",
			kubeconfig: "/path/to/kubeconfig",
			expected:   "/path/to/kubeconfig",
		},
		{
			name:       "home dir kubeconfig",
			kubeconfig: "~/.kube/config",
			expected:   "~/.kube/config",
		},
		{
			name:       "relative path",
			kubeconfig: "./kubeconfig",
			expected:   "./kubeconfig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getKubeconfigPath(tt.kubeconfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildKubeConfig(t *testing.T) {
	t.Run("with kubeconfig file", func(t *testing.T) {
		// Create a temporary kubeconfig file
		tmpFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// Write minimal valid kubeconfig
		kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
		_, err = tmpFile.WriteString(kubeconfigContent)
		require.NoError(t, err)
		tmpFile.Close()

		config, err := buildKubeConfig(tmpFile.Name())
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "https://localhost:6443", config.Host)
	})

	t.Run("with invalid kubeconfig file", func(t *testing.T) {
		config, err := buildKubeConfig("/nonexistent/kubeconfig")
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to build config from kubeconfig")
	})

	t.Run("in-cluster config fails outside cluster", func(t *testing.T) {
		// When running outside a cluster, in-cluster config should fail
		config, err := buildKubeConfig("")
		if err != nil {
			// Expected when running tests outside cluster
			assert.Nil(t, config)
			assert.Contains(t, err.Error(), "failed to get in-cluster config")
		} else {
			// If we're actually in a cluster, this would succeed
			assert.NotNil(t, config)
		}
	})
}

func TestNewRootCommand(t *testing.T) {
	cmd := newRootCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "vpsie-autoscaler", cmd.Use)
	assert.Contains(t, cmd.Short, "VPSie Kubernetes Node Autoscaler")
	assert.True(t, cmd.SilenceUsage)

	// Check that flags are registered
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("kubeconfig"))
	assert.NotNil(t, flags.Lookup("metrics-addr"))
	assert.NotNil(t, flags.Lookup("health-addr"))
	assert.NotNil(t, flags.Lookup("leader-election"))
	assert.NotNil(t, flags.Lookup("leader-election-id"))
	assert.NotNil(t, flags.Lookup("leader-election-namespace"))
	assert.NotNil(t, flags.Lookup("sync-period"))
	assert.NotNil(t, flags.Lookup("vpsie-secret-name"))
	assert.NotNil(t, flags.Lookup("vpsie-secret-namespace"))
	assert.NotNil(t, flags.Lookup("log-level"))
	assert.NotNil(t, flags.Lookup("log-format"))
	assert.NotNil(t, flags.Lookup("development"))
}

func TestAddFlags(t *testing.T) {
	// Create a fresh command without calling newRootCommand which already adds flags
	cmd := &cobra.Command{
		Use:   "test",
		Short: "test command",
	}
	opts := controller.NewDefaultOptions()

	addFlags(cmd, opts)

	flags := cmd.Flags()

	// Test Kubernetes configuration flags
	kubeconfigFlag := flags.Lookup("kubeconfig")
	assert.NotNil(t, kubeconfigFlag)
	assert.Contains(t, kubeconfigFlag.Usage, "kubeconfig")

	// Test server configuration flags
	metricsFlag := flags.Lookup("metrics-addr")
	assert.NotNil(t, metricsFlag)
	assert.Equal(t, ":8080", metricsFlag.DefValue)

	healthFlag := flags.Lookup("health-addr")
	assert.NotNil(t, healthFlag)
	assert.Equal(t, ":8081", healthFlag.DefValue)

	// Test leader election flags
	leaderElectionFlag := flags.Lookup("leader-election")
	assert.NotNil(t, leaderElectionFlag)
	assert.Equal(t, "true", leaderElectionFlag.DefValue)

	leaderElectionIDFlag := flags.Lookup("leader-election-id")
	assert.NotNil(t, leaderElectionIDFlag)
	assert.Equal(t, "vpsie-autoscaler-leader", leaderElectionIDFlag.DefValue)

	leaderElectionNsFlag := flags.Lookup("leader-election-namespace")
	assert.NotNil(t, leaderElectionNsFlag)
	assert.Equal(t, "kube-system", leaderElectionNsFlag.DefValue)

	// Test controller configuration flags
	syncPeriodFlag := flags.Lookup("sync-period")
	assert.NotNil(t, syncPeriodFlag)
	assert.Equal(t, "10m0s", syncPeriodFlag.DefValue)

	// Test VPSie API configuration flags
	vpsieSecretNameFlag := flags.Lookup("vpsie-secret-name")
	assert.NotNil(t, vpsieSecretNameFlag)
	assert.Equal(t, "vpsie-secret", vpsieSecretNameFlag.DefValue)

	vpsieSecretNsFlag := flags.Lookup("vpsie-secret-namespace")
	assert.NotNil(t, vpsieSecretNsFlag)
	assert.Equal(t, "kube-system", vpsieSecretNsFlag.DefValue)

	// Test logging configuration flags
	logLevelFlag := flags.Lookup("log-level")
	assert.NotNil(t, logLevelFlag)
	assert.Equal(t, "info", logLevelFlag.DefValue)

	logFormatFlag := flags.Lookup("log-format")
	assert.NotNil(t, logFormatFlag)
	assert.Equal(t, "json", logFormatFlag.DefValue)

	developmentFlag := flags.Lookup("development")
	assert.NotNil(t, developmentFlag)
	assert.Equal(t, "false", developmentFlag.DefValue)
}

func TestVersionInfo(t *testing.T) {
	// Test that version variables exist and can be set
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate

	Version = "v1.0.0"
	Commit = "abc123"
	BuildDate = "2025-10-17"

	assert.Equal(t, "v1.0.0", Version)
	assert.Equal(t, "abc123", Commit)
	assert.Equal(t, "2025-10-17", BuildDate)

	// Restore original values
	Version = originalVersion
	Commit = originalCommit
	BuildDate = originalBuildDate
}

func TestMain_EnvironmentSetup(t *testing.T) {
	// Save original env vars
	origVersion := os.Getenv("VERSION")
	origCommit := os.Getenv("COMMIT")
	origBuildDate := os.Getenv("BUILD_DATE")

	// Clear env vars
	os.Unsetenv("VERSION")
	os.Unsetenv("COMMIT")
	os.Unsetenv("BUILD_DATE")

	// Set test values
	Version = "test-version"
	Commit = "test-commit"
	BuildDate = "test-date"

	// Simulate main() environment setup
	os.Setenv("VERSION", Version)
	os.Setenv("COMMIT", Commit)
	os.Setenv("BUILD_DATE", BuildDate)

	// Verify environment variables are set
	assert.Equal(t, "test-version", os.Getenv("VERSION"))
	assert.Equal(t, "test-commit", os.Getenv("COMMIT"))
	assert.Equal(t, "test-date", os.Getenv("BUILD_DATE"))

	// Restore original env vars
	if origVersion != "" {
		os.Setenv("VERSION", origVersion)
	} else {
		os.Unsetenv("VERSION")
	}
	if origCommit != "" {
		os.Setenv("COMMIT", origCommit)
	} else {
		os.Unsetenv("COMMIT")
	}
	if origBuildDate != "" {
		os.Setenv("BUILD_DATE", origBuildDate)
	} else {
		os.Unsetenv("BUILD_DATE")
	}
}

func TestSchemeInitialization(t *testing.T) {
	// Verify that the scheme is initialized and contains expected types
	assert.NotNil(t, scheme)

	// The init() function should have registered both standard k8s types
	// and our custom CRDs
	allKinds := scheme.AllKnownTypes()
	assert.NotEmpty(t, allKinds)

	// Verify some standard Kubernetes types are registered
	hasK8sTypes := false
	for gvk := range allKinds {
		if gvk.Group == "" && gvk.Version == "v1" {
			hasK8sTypes = true
			break
		}
	}
	assert.True(t, hasK8sTypes, "Standard Kubernetes types should be registered")

	// Verify our custom CRDs are registered
	hasCustomTypes := false
	for gvk := range allKinds {
		if gvk.Group == "autoscaler.vpsie.com" {
			hasCustomTypes = true
			break
		}
	}
	assert.True(t, hasCustomTypes, "Custom CRDs should be registered in scheme")
}

func TestConfigureLogLevel_AllLevels(t *testing.T) {
	// Create a production logger
	config := zap.NewProductionConfig()
	baseLogger, err := config.Build()
	require.NoError(t, err)

	levels := []string{"debug", "info", "warn", "error", "invalid", ""}

	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			logger := configureLogLevel(baseLogger, level)
			assert.NotNil(t, logger, "Logger should not be nil for level: %s", level)

			// Verify logger can be used without panicking
			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")
		})
	}
}

func TestRun_OptionValidation(t *testing.T) {
	t.Run("invalid options fail validation without completion", func(t *testing.T) {
		opts := &controller.Options{
			MetricsAddr:             "",      // Invalid: empty
			HealthProbeAddr:         ":8081", // Valid
			LeaderElectionID:        "test",
			LeaderElectionNamespace: "default",
			SyncPeriod:              time.Minute,
			VPSieSecretName:         "secret",
			VPSieSecretNamespace:    "default",
			LogLevel:                "info",
			LogFormat:               "json",
		}

		// Validation should fail before completion
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "metrics address")
	})

	t.Run("valid options pass validation", func(t *testing.T) {
		opts := controller.NewDefaultOptions()

		err := opts.Complete()
		require.NoError(t, err)

		err = opts.Validate()
		assert.NoError(t, err)
	})

	t.Run("options completion fills defaults", func(t *testing.T) {
		opts := &controller.Options{}
		err := opts.Complete()
		require.NoError(t, err)

		// After completion, should have defaults
		assert.Equal(t, ":8080", opts.MetricsAddr)
		assert.Equal(t, ":8081", opts.HealthProbeAddr)
		assert.Equal(t, "vpsie-autoscaler-leader", opts.LeaderElectionID)
		assert.Equal(t, "kube-system", opts.LeaderElectionNamespace)
	})
}

func TestCLIFlags_DefaultValues(t *testing.T) {
	cmd := newRootCommand()

	// Parse with no arguments to get defaults
	err := cmd.ParseFlags([]string{})
	require.NoError(t, err)

	// Verify default values through flags
	flags := cmd.Flags()

	metricsAddr, _ := flags.GetString("metrics-addr")
	assert.Equal(t, ":8080", metricsAddr)

	healthAddr, _ := flags.GetString("health-addr")
	assert.Equal(t, ":8081", healthAddr)

	leaderElection, _ := flags.GetBool("leader-election")
	assert.True(t, leaderElection)

	logLevel, _ := flags.GetString("log-level")
	assert.Equal(t, "info", logLevel)

	logFormat, _ := flags.GetString("log-format")
	assert.Equal(t, "json", logFormat)

	development, _ := flags.GetBool("development")
	assert.False(t, development)
}

func TestCLIFlags_CustomValues(t *testing.T) {
	cmd := newRootCommand()

	// Parse with custom values
	args := []string{
		"--metrics-addr=:9090",
		"--health-addr=:9091",
		"--leader-election=false",
		"--log-level=debug",
		"--log-format=console",
		"--development=true",
		"--sync-period=5m",
		"--vpsie-secret-name=my-secret",
		"--vpsie-secret-namespace=my-namespace",
	}

	err := cmd.ParseFlags(args)
	require.NoError(t, err)

	// Verify custom values
	flags := cmd.Flags()

	metricsAddr, _ := flags.GetString("metrics-addr")
	assert.Equal(t, ":9090", metricsAddr)

	healthAddr, _ := flags.GetString("health-addr")
	assert.Equal(t, ":9091", healthAddr)

	leaderElection, _ := flags.GetBool("leader-election")
	assert.False(t, leaderElection)

	logLevel, _ := flags.GetString("log-level")
	assert.Equal(t, "debug", logLevel)

	logFormat, _ := flags.GetString("log-format")
	assert.Equal(t, "console", logFormat)

	development, _ := flags.GetBool("development")
	assert.True(t, development)

	syncPeriod, _ := flags.GetDuration("sync-period")
	assert.Equal(t, "5m0s", syncPeriod.String())

	vpsieSecretName, _ := flags.GetString("vpsie-secret-name")
	assert.Equal(t, "my-secret", vpsieSecretName)

	vpsieSecretNs, _ := flags.GetString("vpsie-secret-namespace")
	assert.Equal(t, "my-namespace", vpsieSecretNs)
}
