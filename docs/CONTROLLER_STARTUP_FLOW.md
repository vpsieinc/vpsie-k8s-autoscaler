# VPSie Autoscaler - Controller Startup Flow

## Startup Sequence Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CONTROLLER STARTUP FLOW                            │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────┐
│   main()     │  Set version environment variables
└──────┬───────┘  Execute Cobra root command
       │
       ▼
┌──────────────────┐
│ newRootCommand() │  Create CLI with all flags
└──────┬───────────┘  Parse command line arguments
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              run() Function                                   │
└──────────────────────────────────────────────────────────────────────────────┘

    ┌─────────────────────────────────────────────────────────────────┐
    │ 1. Validate Configuration                                        │
    ├──────────────────────────────────────────────────────────────────┤
    │ • opts.Complete()  - Fill in defaults                            │
    │ • opts.Validate()  - Validate all options                        │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 2. Initialize Structured Logger                                  │
    ├──────────────────────────────────────────────────────────────────┤
    │ • logging.NewLogger(development)                                 │
    │ • Configure log level (debug/info/warn/error)                    │
    │ • Log initialization with version info                           │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 3. Register Prometheus Metrics                                   │
    ├──────────────────────────────────────────────────────────────────┤
    │ • metrics.RegisterMetrics()                                      │
    │ • 22 metrics registered with controller-runtime registry         │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 4. Set Controller-Runtime Logger                                 │
    ├──────────────────────────────────────────────────────────────────┤
    │ • ctrl.SetLogger(logging.NewZapLogger())                         │
    │ • Bridge zap.Logger → logr.Logger                                │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 5. Build Kubernetes Config                                       │
    ├──────────────────────────────────────────────────────────────────┤
    │ • buildKubeConfig(kubeconfig)                                    │
    │ • In-cluster config OR kubeconfig file                           │
    │ • Log configuration source                                       │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 6. Log Controller Configuration                                  │
    ├──────────────────────────────────────────────────────────────────┤
    │ • Metrics address: :8080                                         │
    │ • Health probe address: :8081                                    │
    │ • Leader election settings                                       │
    │ • Sync period: 10m                                               │
    │ • VPSie secret location                                          │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 7. Create Controller Manager                                     │
    ├──────────────────────────────────────────────────────────────────┤
    │ • controller.NewManager(config, opts)                            │
    │   ├─ Create controller-runtime manager                           │
    │   ├─ Create Kubernetes clientset                                 │
    │   ├─ Create VPSie API client                                     │
    │   ├─ Setup health checks                                         │
    │   └─ Register controllers (NodeGroup, VPSieNode)                 │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 8. Setup Signal Handler                                          │
    ├──────────────────────────────────────────────────────────────────┤
    │ • setupSignalHandler()                                           │
    │ • Listen for SIGINT, SIGTERM, SIGQUIT                            │
    │ • Create cancellable context                                     │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 9. Start Manager (Async)                                         │
    ├──────────────────────────────────────────────────────────────────┤
    │ • mgr.Start(ctx) in goroutine                                    │
    │ • Starts metrics server on :8080                                 │
    │ • Starts health probe server on :8081                            │
    │ • Starts controllers (NodeGroup, VPSieNode)                      │
    │ • Performs leader election (if enabled)                          │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 10. Wait for Shutdown Signal or Error                            │
    ├──────────────────────────────────────────────────────────────────┤
    │ select {                                                         │
    │   case <-ctx.Done():     // Graceful shutdown                    │
    │   case err := <-errCh:   // Manager error                        │
    │ }                                                                │
    └─────────────────────────────────────────────────────────────────┘


┌──────────────────────────────────────────────────────────────────────────────┐
│                         GRACEFUL SHUTDOWN FLOW                                │
└──────────────────────────────────────────────────────────────────────────────┘

    Signal Received (SIGTERM/SIGINT/SIGQUIT)
                │
                ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 1. Log Shutdown Initiation                                       │
    │ "Received shutdown signal, initiating graceful shutdown"         │
    └─────────────────────────────────────────────────────────────────┘
                │
                ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 2. Create Shutdown Context                                       │
    │ • 30-second timeout                                              │
    │ • Prevents hanging on shutdown                                   │
    └─────────────────────────────────────────────────────────────────┘
                │
                ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 3. Call Manager Shutdown                                         │
    │ • mgr.Shutdown(shutdownCtx)                                      │
    │   ├─ Stop controllers                                            │
    │   ├─ Stop metrics server                                         │
    │   ├─ Stop health probe server                                    │
    │   ├─ Release leader election lock                                │
    │   └─ Cleanup resources                                           │
    └─────────────────────────────────────────────────────────────────┘
                │
                ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 4. Wait for Manager to Exit                                      │
    │ • Check for errors from manager goroutine                        │
    │ • Log any shutdown errors                                        │
    └─────────────────────────────────────────────────────────────────┘
                │
                ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │ 5. Exit Successfully                                             │
    │ "Controller stopped gracefully"                                  │
    └─────────────────────────────────────────────────────────────────┘


┌──────────────────────────────────────────────────────────────────────────────┐
│                      CONTROLLER-RUNTIME MANAGER                               │
└──────────────────────────────────────────────────────────────────────────────┘

    Manager Components (pkg/controller/manager.go)
    ───────────────────────────────────────────────

    ┌─────────────────────────────────────────────────────────────────┐
    │                     ControllerManager Struct                     │
    ├─────────────────────────────────────────────────────────────────┤
    │ • config        *rest.Config                                     │
    │ • options       *Options                                         │
    │ • mgr           ctrl.Manager        ◄─── controller-runtime      │
    │ • vpsieClient   *client.Client      ◄─── VPSie API client        │
    │ • k8sClient     kubernetes.Interface ◄─── Kubernetes client      │
    │ • healthChecker *HealthChecker      ◄─── Health checks           │
    │ • logger        *zap.Logger         ◄─── Structured logging      │
    │ • scheme        *runtime.Scheme     ◄─── CRD registration        │
    └─────────────────────────────────────────────────────────────────┘
                                    │
                                    ├─────────────┐
                                    │             │
                                    ▼             ▼
    ┌──────────────────────────────────┐  ┌──────────────────────────┐
    │    Health Check Endpoints        │  │    Metrics Server        │
    ├──────────────────────────────────┤  ├──────────────────────────┤
    │ • /healthz - Liveness probe      │  │ Port: :8080              │
    │ • /readyz  - Readiness probe     │  │ 22 Prometheus metrics    │
    │ • /ping    - Ping check          │  │ Auto-registered          │
    │ • /vpsie   - VPSie API check     │  └──────────────────────────┘
    └──────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │                        Controllers                               │
    ├─────────────────────────────────────────────────────────────────┤
    │ NodeGroupReconciler                                              │
    │ ├─ Watches: NodeGroup CRD                                        │
    │ ├─ Reconciles desired vs actual nodes                            │
    │ └─ Creates/Deletes VPSieNode resources                           │
    │                                                                  │
    │ VPSieNodeReconciler                                              │
    │ ├─ Watches: VPSieNode CRD                                        │
    │ ├─ Manages VPS lifecycle (8 phases)                              │
    │ └─ Integrates with VPSie API                                     │
    └─────────────────────────────────────────────────────────────────┘


┌──────────────────────────────────────────────────────────────────────────────┐
│                        OBSERVABILITY INTEGRATION                              │
└──────────────────────────────────────────────────────────────────────────────┘

    Logging (pkg/logging/)
    ──────────────────────
    • Structured zap logger
    • Request ID tracking (UUID)
    • ISO8601 timestamps
    • Caller + stack traces
    • Log levels: debug/info/warn/error
    • JSON or console output

    Metrics (pkg/metrics/)
    ──────────────────────
    • 22 Prometheus metrics
    • NodeGroup metrics (5)
    • VPSieNode metrics (3)
    • Controller metrics (3)
    • VPSie API metrics (3)
    • Scaling metrics (4)
    • Pod metrics (2)
    • Event metrics (1)
    • Node lifecycle metrics (1)

    Events (pkg/events/)
    ────────────────────
    • Kubernetes event emitter
    • 20+ event types
    • Automatic metrics recording
    • Event reason tracking


┌──────────────────────────────────────────────────────────────────────────────┐
│                            CONFIGURATION                                      │
└──────────────────────────────────────────────────────────────────────────────┘

    Default Values (pkg/controller/options.go)
    ──────────────────────────────────────────

    Kubeconfig:               "" (in-cluster)
    MetricsAddr:              ":8080"
    HealthProbeAddr:          ":8081"
    EnableLeaderElection:     true
    LeaderElectionID:         "vpsie-autoscaler-leader"
    LeaderElectionNamespace:  "kube-system"
    SyncPeriod:               10 minutes
    VPSieSecretName:          "vpsie-secret"
    VPSieSecretNamespace:     "kube-system"
    LogLevel:                 "info"
    LogFormat:                "json"
    DevelopmentMode:          false

    All values can be overridden via CLI flags!
