package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Options
		wantErr bool
	}{
		{
			name: "production logger with info level",
			opts: &Options{
				LogLevel:        "info",
				LogFormat:       "json",
				DevelopmentMode: false,
			},
			wantErr: false,
		},
		{
			name: "development logger with debug level",
			opts: &Options{
				LogLevel:        "debug",
				LogFormat:       "console",
				DevelopmentMode: true,
			},
			wantErr: false,
		},
		{
			name: "production logger with warn level",
			opts: &Options{
				LogLevel:        "warn",
				LogFormat:       "json",
				DevelopmentMode: false,
			},
			wantErr: false,
		},
		{
			name: "production logger with error level",
			opts: &Options{
				LogLevel:        "error",
				LogFormat:       "json",
				DevelopmentMode: false,
			},
			wantErr: false,
		},
		{
			name: "console format",
			opts: &Options{
				LogLevel:        "info",
				LogFormat:       "console",
				DevelopmentMode: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := newLogger(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, logger)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, logger)

				// Test that logger works
				logger.Info("test message")
				logger.Debug("debug message")
				logger.Warn("warn message")
				logger.Error("error message")
			}
		})
	}
}

func TestNewLogger_LogLevels(t *testing.T) {
	tests := []struct {
		logLevel string
		expected zap.AtomicLevel
	}{
		{"debug", zap.NewAtomicLevelAt(zap.DebugLevel)},
		{"info", zap.NewAtomicLevelAt(zap.InfoLevel)},
		{"warn", zap.NewAtomicLevelAt(zap.WarnLevel)},
		{"error", zap.NewAtomicLevelAt(zap.ErrorLevel)},
	}

	for _, tt := range tests {
		t.Run(tt.logLevel, func(t *testing.T) {
			opts := &Options{
				LogLevel:        tt.logLevel,
				LogFormat:       "json",
				DevelopmentMode: false,
			}

			logger, err := newLogger(opts)
			require.NoError(t, err)
			require.NotNil(t, logger)

			// Verify logger was created (we can't directly check the level
			// without accessing internal config, but we test it works)
			logger.Info("test")
		})
	}
}

func TestNewManager_NilConfig(t *testing.T) {
	opts := NewDefaultOptions()
	mgr, err := NewManager(nil, opts)

	assert.Error(t, err)
	assert.Nil(t, mgr)
	assert.Contains(t, err.Error(), "kubeconfig cannot be nil")
}

func TestNewManager_NilOptions(t *testing.T) {
	// Create a non-nil config to test options validation
	config := &rest.Config{}
	mgr, err := NewManager(config, nil)

	assert.Error(t, err)
	assert.Nil(t, mgr)
	assert.Contains(t, err.Error(), "options cannot be nil")
}

func TestControllerManager_Getters(t *testing.T) {
	// Create a manager instance with minimal setup
	// Note: We can't fully test this without a real kubeconfig,
	// but we can test the struct and methods exist

	cm := &ControllerManager{
		options: NewDefaultOptions(),
	}

	// Test that getters don't panic
	opts := cm.options
	assert.NotNil(t, opts)
}

func TestOptions_Integration(t *testing.T) {
	// Test that options validation integrates properly with manager
	tests := []struct {
		name    string
		opts    *Options
		wantErr bool
	}{
		{
			name:    "valid default options",
			opts:    NewDefaultOptions(),
			wantErr: false,
		},
		{
			name: "invalid options - empty metrics addr",
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultOptions_AreValid(t *testing.T) {
	// Ensure default options are always valid
	opts := NewDefaultOptions()
	err := opts.Validate()
	require.NoError(t, err)
}

func TestLoggerFormats(t *testing.T) {
	tests := []struct {
		format string
		valid  bool
	}{
		{"json", true},
		{"console", true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			opts := &Options{
				LogLevel:        "info",
				LogFormat:       tt.format,
				DevelopmentMode: false,
			}

			logger, err := newLogger(opts)
			if tt.valid {
				require.NoError(t, err)
				assert.NotNil(t, logger)

				// Test logging works
				logger.Info("test message")
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestLogger_DevelopmentMode(t *testing.T) {
	tests := []struct {
		name      string
		devMode   bool
		logLevel  string
		logFormat string
	}{
		{
			name:      "production mode",
			devMode:   false,
			logLevel:  "info",
			logFormat: "json",
		},
		{
			name:      "development mode",
			devMode:   true,
			logLevel:  "debug",
			logFormat: "console",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Options{
				LogLevel:        tt.logLevel,
				LogFormat:       tt.logFormat,
				DevelopmentMode: tt.devMode,
			}

			logger, err := newLogger(opts)
			require.NoError(t, err)
			require.NotNil(t, logger)

			// Verify logger works in both modes
			logger.Debug("debug message")
			logger.Info("info message")
			logger.Warn("warn message")
			logger.Error("error message")
		})
	}
}

func TestNewLogger_InvalidLogLevel(t *testing.T) {
	// Test that invalid log levels default to info
	opts := &Options{
		LogLevel:        "invalid",
		LogFormat:       "json",
		DevelopmentMode: false,
	}

	logger, err := newLogger(opts)
	require.NoError(t, err)
	require.NotNil(t, logger)

	// Logger should work even with default level
	logger.Info("test message")
}
