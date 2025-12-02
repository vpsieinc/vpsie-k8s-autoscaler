package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/webhook"
)

var (
	port        int
	certDir     string
	certFile    string
	keyFile     string
	logLevel    string
	logFormat   string
)

func init() {
	flag.IntVar(&port, "port", 9443, "Port to listen on for webhook requests")
	flag.StringVar(&certDir, "cert-dir", "/var/run/webhook-certs", "Directory containing TLS certificates")
	flag.StringVar(&certFile, "tls-cert-file", "tls.crt", "TLS certificate file name")
	flag.StringVar(&keyFile, "tls-key-file", "tls.key", "TLS private key file name")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.StringVar(&logFormat, "log-format", "json", "Log format (json, console)")
}

func main() {
	flag.Parse()

	// Create logger
	logger, err := createLogger(logLevel, logFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("starting VPSie Autoscaler validation webhook",
		zap.Int("port", port),
		zap.String("cert-dir", certDir),
		zap.String("log-level", logLevel))

	// Create webhook server
	server, err := webhook.NewServer(webhook.ServerConfig{
		Port:   port,
		Logger: logger,
	})
	if err != nil {
		logger.Fatal("failed to create webhook server", zap.Error(err))
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		sig := <-sigChan
		logger.Info("received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Construct certificate paths
	certPath := filepath.Join(certDir, certFile)
	keyPath := filepath.Join(certDir, keyFile)

	logger.Info("webhook server configuration",
		zap.String("cert-file", certPath),
		zap.String("key-file", keyPath))

	// Start webhook server
	if err := server.Start(ctx, certPath, keyPath); err != nil {
		logger.Fatal("webhook server failed", zap.Error(err))
	}

	logger.Info("webhook server shut down gracefully")
}

// createLogger creates a zap logger with the specified level and format
func createLogger(level, format string) (*zap.Logger, error) {
	// Parse log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}

	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	// Create encoder based on format
	var encoder zapcore.Encoder
	switch format {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case "console":
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		return nil, fmt.Errorf("invalid log format %q (must be 'json' or 'console')", format)
	}

	// Create logger
	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		zapLevel,
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)), nil
}
