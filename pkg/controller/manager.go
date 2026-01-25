package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller/nodegroup"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/controller/vpsienode"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/events"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/scaler"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/tracing"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/webhook"
)

// ControllerManager manages the lifecycle of all controllers
type ControllerManager struct {
	config            *rest.Config
	options           *Options
	mgr               ctrl.Manager
	vpsieClient       *vpsieclient.Client
	k8sClient         kubernetes.Interface
	metricsClient     metricsv1beta1.Interface
	scaleDownManager  *scaler.ScaleDownManager
	healthChecker     *HealthChecker
	logger            *zap.Logger
	scheme            *runtime.Scheme
	eventWatcher      *events.EventWatcher
	scaleUpController *events.ScaleUpController
	webhookServer     *webhook.Server
	tracer            *tracing.Tracer
	clusterConfig     *DiscoveredClusterConfig // Auto-discovered cluster configuration
}

// DiscoveredClusterConfig holds cluster configuration discovered from VPSie API
type DiscoveredClusterConfig struct {
	ClusterIdentifier string   // VPSie cluster UUID (resourceIdentifier)
	ClusterName       string   // Cluster display name
	DatacenterID      string   // Datacenter UUID
	ProjectID         string   // Project UUID
	KubernetesVersion string   // K8s version from cluster
	OfferingIDs       []string // Available offering IDs for this datacenter
	KubeSizeID        int      // Default K8s size ID
}

// extractClusterName extracts the VPSie cluster name from an API server hostname or node name.
// Example: "HEL-Kubernetes-49ab-master-main.6.k8s.vpsie.net" -> "HEL-Kubernetes-49ab"
// Example: "hel-kubernetes-49ab-slave-11cc" -> "HEL-Kubernetes-49ab"
func extractClusterName(hostname string) string {
	// Pattern: {DC}-Kubernetes-{id} followed by -master/-slave or other suffixes
	// Case-insensitive matching
	re := regexp.MustCompile(`(?i)^([a-z]+-kubernetes-[a-z0-9]+)`)
	matches := re.FindStringSubmatch(hostname)
	if len(matches) > 1 {
		// Normalize to uppercase DC code and title case Kubernetes
		// e.g., "hel-kubernetes-49ab" -> "HEL-Kubernetes-49ab"
		parts := strings.SplitN(matches[1], "-", 3)
		if len(parts) == 3 {
			return strings.ToUpper(parts[0]) + "-Kubernetes-" + parts[2]
		}
		return matches[1]
	}
	return ""
}

// discoverClusterConfig auto-discovers cluster configuration from VPSie API.
// It attempts to find the current cluster by matching the API server hostname.
func discoverClusterConfig(ctx context.Context, config *rest.Config, k8sClient kubernetes.Interface, vpsieClient *vpsieclient.Client, logger *zap.Logger) (*DiscoveredClusterConfig, error) {
	logger.Info("Starting cluster auto-discovery from VPSie API")

	// Extract API server hostname - try multiple sources
	apiServerHost := ""

	// First, try from kubeconfig (works when running locally)
	if config.Host != "" {
		parsedURL, err := url.Parse(config.Host)
		if err == nil {
			host := parsedURL.Hostname()
			// Skip internal IPs (10.x.x.x, kubernetes.default, etc.)
			if !strings.HasPrefix(host, "10.") && !strings.HasPrefix(host, "kubernetes") {
				apiServerHost = host
			}
		}
	}

	// If running in-cluster, try to get external API server URL from cluster-info ConfigMap
	if apiServerHost == "" || strings.HasPrefix(apiServerHost, "10.") {
		logger.Debug("Attempting to get API server URL from cluster-info ConfigMap")
		cm, err := k8sClient.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
		if err == nil && cm.Data != nil {
			if kubeconfig, ok := cm.Data["kubeconfig"]; ok {
				// Parse the kubeconfig to extract the server URL
				// The format is: server: https://hostname:port
				for _, line := range strings.Split(kubeconfig, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "server:") {
						serverURL := strings.TrimPrefix(line, "server:")
						serverURL = strings.TrimSpace(serverURL)
						if parsedURL, err := url.Parse(serverURL); err == nil {
							apiServerHost = parsedURL.Hostname()
							logger.Info("Extracted API server hostname from cluster-info ConfigMap",
								zap.String("apiServerHost", apiServerHost))
						}
						break
					}
				}
			}
		} else {
			logger.Debug("Could not get cluster-info ConfigMap", zap.Error(err))
		}
	}

	logger.Info("Looking for VPSie cluster matching API server",
		zap.String("apiServerHost", apiServerHost))

	// Extract cluster name from hostname (e.g., "HEL-Kubernetes-49ab" from "HEL-Kubernetes-49ab-master-main.6.k8s.vpsie.net")
	var clusterName string
	if apiServerHost != "" {
		clusterName = extractClusterName(apiServerHost)
	}

	// If we couldn't extract from API server hostname (e.g., it's an IP), try node hostnames
	if clusterName == "" {
		logger.Debug("Could not extract cluster name from API server hostname, trying node hostnames")
		nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodes.Items) > 0 {
			for _, node := range nodes.Items {
				// Try to extract from kubernetes.io/hostname label
				if hostname, ok := node.Labels["kubernetes.io/hostname"]; ok {
					clusterName = extractClusterName(hostname)
					if clusterName != "" {
						logger.Info("Extracted cluster name from node hostname",
							zap.String("nodeHostname", hostname),
							zap.String("clusterName", clusterName))
						break
					}
				}
				// Try node name directly
				clusterName = extractClusterName(node.Name)
				if clusterName != "" {
					logger.Info("Extracted cluster name from node name",
						zap.String("nodeName", node.Name),
						zap.String("clusterName", clusterName))
					break
				}
			}
		}
	}

	if clusterName == "" {
		return nil, fmt.Errorf("could not extract cluster name from API server hostname (%s) or node hostnames", apiServerHost)
	}
	logger.Info("Extracted cluster name from hostname",
		zap.String("clusterName", clusterName))

	// Try to get VPSie cluster info from vpsie-secret first
	// The secret can contain optional cluster configuration:
	//   - resourceIdentifier: VPSie cluster UUID
	//   - datacenterId: VPSie datacenter UUID
	//   - projectId: VPSie project UUID
	var clusterIdentifier, datacenterID, projectID string
	secret, err := k8sClient.CoreV1().Secrets("kube-system").Get(ctx, "vpsie-secret", metav1.GetOptions{})
	if err == nil && secret.Data != nil {
		if id, ok := secret.Data["resourceIdentifier"]; ok && len(id) > 0 {
			clusterIdentifier = string(id)
			logger.Info("Found cluster identifier in vpsie-secret", zap.String("clusterIdentifier", clusterIdentifier))
		}
		if dc, ok := secret.Data["datacenterId"]; ok && len(dc) > 0 {
			datacenterID = string(dc)
			logger.Info("Found datacenter ID in vpsie-secret", zap.String("datacenterID", datacenterID))
		}
		if proj, ok := secret.Data["projectId"]; ok && len(proj) > 0 {
			projectID = string(proj)
			logger.Info("Found project ID in vpsie-secret", zap.String("projectID", projectID))
		}
	}

	// Try to get VPSie cluster info from node labels/annotations (fallback)
	// VPSie-managed nodes should have labels with cluster identifier
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil && len(nodes.Items) > 0 {
		for _, node := range nodes.Items {
			// Check for VPSie labels
			if id, ok := node.Labels["vpsie.com/cluster-id"]; ok {
				clusterIdentifier = id
			}
			if dc, ok := node.Labels["vpsie.com/datacenter-id"]; ok {
				datacenterID = dc
			}
			if proj, ok := node.Labels["vpsie.com/project-id"]; ok {
				projectID = proj
			}
			// Also check annotations
			if id, ok := node.Annotations["vpsie.com/cluster-id"]; ok && clusterIdentifier == "" {
				clusterIdentifier = id
			}
			if dc, ok := node.Annotations["vpsie.com/datacenter-id"]; ok && datacenterID == "" {
				datacenterID = dc
			}
			// Check provider ID for VPSie format: vpsie://<dc>/<cluster>/<node-id>
			if node.Spec.ProviderID != "" && strings.HasPrefix(node.Spec.ProviderID, "vpsie://") {
				parts := strings.Split(strings.TrimPrefix(node.Spec.ProviderID, "vpsie://"), "/")
				if len(parts) >= 2 {
					if datacenterID == "" {
						datacenterID = parts[0]
					}
					if clusterIdentifier == "" && len(parts) >= 2 {
						clusterIdentifier = parts[1]
					}
				}
			}
			if clusterIdentifier != "" {
				break
			}
		}
		if clusterIdentifier != "" {
			logger.Info("Found VPSie cluster info from node labels/annotations",
				zap.String("clusterIdentifier", clusterIdentifier),
				zap.String("datacenterID", datacenterID),
				zap.String("projectID", projectID))
		}
	}

	// Try to list K8s clusters from VPSie API
	var matchedCluster *vpsieclient.K8sCluster
	clusters, err := vpsieClient.ListK8sClusters(ctx)
	if err != nil {
		logger.Warn("Could not list K8s clusters from VPSie API, will use extracted info",
			zap.Error(err),
			zap.String("clusterName", clusterName))
	} else {
		logger.Info("Found VPSie K8s clusters", zap.Int("count", len(clusters)))

		// Find the cluster that matches our cluster name
		for i := range clusters {
			cluster := &clusters[i]
			logger.Debug("Checking cluster",
				zap.String("name", cluster.Name),
				zap.String("identifier", cluster.Identifier),
				zap.String("kubeVersion", cluster.KubeVersion))

			// Match by exact cluster name
			if cluster.Name == clusterName {
				matchedCluster = cluster
				logger.Info("Matched cluster by name",
					zap.String("clusterName", cluster.Name),
					zap.String("identifier", cluster.Identifier))
				break
			}

			// Match by cluster name in hostname (e.g., "HEL-Kubernetes-49ab-master-main.6.k8s.vpsie.net")
			if cluster.Name != "" && strings.Contains(strings.ToLower(apiServerHost), strings.ToLower(cluster.Name)) {
				matchedCluster = cluster
				logger.Info("Matched cluster by name in hostname",
					zap.String("clusterName", cluster.Name),
					zap.String("apiServerHost", apiServerHost))
				break
			}
		}
	}

	// If we found a matched cluster from API, use its info
	if matchedCluster != nil {
		if clusterIdentifier == "" {
			clusterIdentifier = matchedCluster.Identifier
		}
		// Note: DCIdentifier and ProjectIdentifier are not in the cluster list response
		// They need to be obtained from node groups or set manually
	}

	// We need at least the cluster identifier to proceed
	if clusterIdentifier == "" {
		logger.Warn("Could not determine cluster identifier from VPSie API or node labels",
			zap.String("clusterName", clusterName),
			zap.String("hint", "Add 'resourceIdentifier' key to vpsie-secret, set --resource-identifier flag, or add vpsie.com/cluster-id label to nodes"))
		return nil, fmt.Errorf("could not determine VPSie cluster identifier for %s", clusterName)
	}

	// Get node groups for this cluster to find available offerings
	nodeGroups, err := vpsieClient.ListK8sNodeGroups(ctx, clusterIdentifier)
	if err != nil {
		logger.Warn("Failed to list node groups, will use defaults",
			zap.Error(err))
	}

	// Collect offering IDs from existing node groups
	offeringIDs := []string{}
	var defaultKubeSizeID int
	if len(nodeGroups) > 0 {
		for _, ng := range nodeGroups {
			// Use boxsize_id as offering ID
			offeringID := strconv.Itoa(ng.BoxsizeID)
			// Deduplicate
			found := false
			for _, existing := range offeringIDs {
				if existing == offeringID {
					found = true
					break
				}
			}
			if !found {
				offeringIDs = append(offeringIDs, offeringID)
			}
			// Use the first node group's boxsize as default
			if defaultKubeSizeID == 0 {
				defaultKubeSizeID = ng.BoxsizeID
			}
			// Get datacenter from node group if not set
			if datacenterID == "" && ng.DCIdentifier != "" {
				datacenterID = ng.DCIdentifier
			}
		}
	}

	// Get K8s offers for this datacenter if we didn't find offerings from node groups
	if len(offeringIDs) == 0 && datacenterID != "" {
		offers, err := vpsieClient.ListK8sOffers(ctx, datacenterID)
		if err != nil {
			logger.Warn("Failed to list K8s offers",
				zap.Error(err))
		} else {
			for _, offer := range offers {
				offeringIDs = append(offeringIDs, strconv.Itoa(offer.ID))
				if defaultKubeSizeID == 0 {
					defaultKubeSizeID = offer.ID
				}
			}
		}
	}

	// Get Kubernetes version from server
	kubernetesVersion := ""
	if matchedCluster != nil && matchedCluster.KubeVersion != "" {
		kubernetesVersion = matchedCluster.KubeVersion
		// VPSie API returns version without 'v' prefix (e.g., "1.34.1")
		// but CRD validation requires it (e.g., "v1.34.1")
		if !strings.HasPrefix(kubernetesVersion, "v") {
			kubernetesVersion = "v" + kubernetesVersion
		}
	} else {
		// Get from Kubernetes API (already has 'v' prefix)
		serverVersion, err := k8sClient.Discovery().ServerVersion()
		if err == nil {
			kubernetesVersion = serverVersion.GitVersion
		}
	}

	// Build discovered config
	discoveredConfig := &DiscoveredClusterConfig{
		ClusterIdentifier: clusterIdentifier,
		ClusterName:       clusterName,
		DatacenterID:      datacenterID,
		ProjectID:         projectID,
		KubernetesVersion: kubernetesVersion,
		OfferingIDs:       offeringIDs,
		KubeSizeID:        defaultKubeSizeID,
	}

	logger.Info("Cluster auto-discovery completed successfully",
		zap.String("clusterName", discoveredConfig.ClusterName),
		zap.String("clusterIdentifier", discoveredConfig.ClusterIdentifier),
		zap.String("datacenterID", discoveredConfig.DatacenterID),
		zap.String("projectID", discoveredConfig.ProjectID),
		zap.String("kubernetesVersion", discoveredConfig.KubernetesVersion),
		zap.Strings("offeringIDs", discoveredConfig.OfferingIDs),
		zap.Int("kubeSizeID", discoveredConfig.KubeSizeID))

	return discoveredConfig, nil
}

// NewManager creates a new ControllerManager
func NewManager(config *rest.Config, opts *Options) (*ControllerManager, error) {
	if config == nil {
		return nil, fmt.Errorf("kubeconfig cannot be nil")
	}

	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	// Validate options
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	// Create logger
	logger, err := newLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create scheme and register types
	scheme := runtime.NewScheme()
	// Register core Kubernetes types (Node, Secret, Pod, etc.)
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}
	// Register our custom CRDs
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add CRDs to scheme: %w", err)
	}

	// Create controller-runtime manager
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: opts.MetricsAddr,
		},
		HealthProbeBindAddress:  opts.HealthProbeAddr,
		LeaderElection:          opts.EnableLeaderElection,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.LeaderElectionNamespace,
		Logger:                  zapr.NewLogger(logger),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Add field indexer for pod's spec.nodeName field
	// This is required for efficient pod listing by node name (used in drainer)
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return nil, fmt.Errorf("failed to add pod node name indexer: %w", err)
	}

	// Create Kubernetes clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create metrics clientset
	metricsClient, err := metricsv1beta1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	// Initialize Sentry tracing
	// DSN can come from flag or SENTRY_DSN environment variable
	sentryDSN := opts.SentryDSN
	if sentryDSN == "" {
		sentryDSN = os.Getenv("SENTRY_DSN")
	}
	sentryEnv := opts.SentryEnvironment
	if sentryEnv == "" {
		sentryEnv = os.Getenv("SENTRY_ENVIRONMENT")
		if sentryEnv == "" {
			sentryEnv = "development"
		}
	}

	tracer, err := tracing.NewTracer(&tracing.Config{
		DSN:              sentryDSN,
		Environment:      sentryEnv,
		Release:          os.Getenv("VERSION"),
		TracesSampleRate: opts.SentryTracesSampleRate,
		ErrorSampleRate:  opts.SentryErrorSampleRate,
		ServerName:       os.Getenv("POD_NAME"),
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Sentry tracing: %w", err)
	}
	// Set global tracer for convenience functions
	tracing.SetGlobalTracer(tracer)

	// Create VPSie API client with tracing transport
	ctx := context.Background()
	vpsieClientOpts := &vpsieclient.ClientOptions{
		SecretName:      opts.VPSieSecretName,
		SecretNamespace: opts.VPSieSecretNamespace,
	}
	// Add tracing HTTP transport if Sentry is enabled
	if tracer.IsEnabled() {
		vpsieClientOpts.HTTPTransport = tracing.NewHTTPTransport(tracer, nil)
	}
	vpsieClient, err := vpsieclient.NewClient(ctx, k8sClient, vpsieClientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPSie client: %w", err)
	}

	// Create ScaleDownManager
	scaleDownConfig := scaler.DefaultConfig()
	scaleDownManager := scaler.NewScaleDownManager(k8sClient, metricsClient, logger, scaleDownConfig)

	// Create cost calculator for cost-aware NodeGroup selection
	costCalculator := cost.NewCalculator(vpsieClient)

	// Create ResourceAnalyzer for scale-up decisions with cost-aware selection
	resourceAnalyzer := events.NewResourceAnalyzer(logger, costCalculator)

	// Create health checker
	healthChecker := NewHealthChecker(vpsieClient)

	// Try auto-discovery if manual configuration is not provided
	var clusterConfig *DiscoveredClusterConfig
	needsDiscovery := opts.DefaultDatacenterID == "" || len(opts.DefaultOfferingIDs) == 0 || opts.ResourceIdentifier == ""

	if needsDiscovery {
		logger.Info("Manual cluster configuration not provided, attempting auto-discovery from VPSie API")
		discovered, err := discoverClusterConfig(ctx, config, k8sClient, vpsieClient, logger)
		if err != nil {
			logger.Warn("Cluster auto-discovery failed, dynamic NodeGroup creation will be disabled",
				zap.Error(err))
		} else {
			clusterConfig = discovered
		}
	}

	cm := &ControllerManager{
		config:           config,
		options:          opts,
		mgr:              mgr,
		vpsieClient:      vpsieClient,
		k8sClient:        k8sClient,
		metricsClient:    metricsClient,
		scaleDownManager: scaleDownManager,
		healthChecker:    healthChecker,
		logger:           logger,
		scheme:           scheme,
		tracer:           tracer,
		clusterConfig:    clusterConfig,
	}

	// Create DynamicNodeGroupCreator for automatic NodeGroup provisioning
	// Use auto-discovered values if manual configuration is not provided
	var nodeGroupTemplate *events.NodeGroupTemplate

	// Determine effective values: manual config takes precedence over auto-discovered
	effectiveDatacenterID := opts.DefaultDatacenterID
	effectiveOfferingIDs := opts.DefaultOfferingIDs
	effectiveResourceID := opts.ResourceIdentifier
	effectiveK8sVersion := opts.KubernetesVersion
	effectiveKubeSizeID := opts.KubeSizeID
	var effectiveProjectID string // Only from auto-discovery

	if clusterConfig != nil {
		if effectiveDatacenterID == "" {
			effectiveDatacenterID = clusterConfig.DatacenterID
		}
		if len(effectiveOfferingIDs) == 0 {
			effectiveOfferingIDs = clusterConfig.OfferingIDs
		}
		if effectiveResourceID == "" {
			effectiveResourceID = clusterConfig.ClusterIdentifier
		}
		if effectiveK8sVersion == "" {
			effectiveK8sVersion = clusterConfig.KubernetesVersion
		}
		if effectiveKubeSizeID == 0 {
			effectiveKubeSizeID = clusterConfig.KubeSizeID
		}
		effectiveProjectID = clusterConfig.ProjectID
	}

	if effectiveDatacenterID != "" && len(effectiveOfferingIDs) > 0 && effectiveResourceID != "" {
		nodeGroupTemplate = &events.NodeGroupTemplate{
			Namespace:           "kube-system",
			MinNodes:            1,
			MaxNodes:            10,
			DefaultDatacenterID: effectiveDatacenterID,
			DefaultOfferingIDs:  effectiveOfferingIDs,
			ResourceIdentifier:  effectiveResourceID,
			KubernetesVersion:   effectiveK8sVersion,
			KubeSizeID:          effectiveKubeSizeID,
			Project:             effectiveProjectID,
		}
		configSource := "manual"
		if clusterConfig != nil && needsDiscovery {
			configSource = "auto-discovered"
		}
		logger.Info("Dynamic NodeGroup creation enabled",
			zap.String("configSource", configSource),
			zap.String("datacenterID", effectiveDatacenterID),
			zap.Strings("offeringIDs", effectiveOfferingIDs),
			zap.String("resourceIdentifier", effectiveResourceID),
			zap.String("kubernetesVersion", effectiveK8sVersion),
			zap.Int("kubeSizeID", effectiveKubeSizeID),
			zap.String("projectID", effectiveProjectID),
			zap.Bool("dynamicKubeSizeSelection", effectiveKubeSizeID == 0),
		)
	} else {
		logger.Warn("Dynamic NodeGroup creation disabled - missing required configuration",
			zap.String("datacenterID", effectiveDatacenterID),
			zap.Int("offeringIDsCount", len(effectiveOfferingIDs)),
			zap.String("resourceIdentifier", effectiveResourceID),
			zap.String("kubernetesVersion", effectiveK8sVersion),
		)
	}
	dynamicCreator := events.NewDynamicNodeGroupCreator(
		mgr.GetClient(),
		vpsieClient,
		logger,
		nodeGroupTemplate,
	)

	// Create EventWatcher and ScaleUpController for pending pod detection
	// ScaleUpController is created first with nil watcher, then wired up after EventWatcher is created
	scaleUpController := events.NewScaleUpController(
		mgr.GetClient(),
		resourceAnalyzer,
		nil, // Will set EventWatcher after creation via SetWatcher
		dynamicCreator,
		logger,
	)

	eventWatcher := events.NewEventWatcher(
		mgr.GetClient(),
		k8sClient,
		logger,
		scaleUpController.HandleScaleUp,
	)

	// Wire up the ScaleUpController with the EventWatcher
	scaleUpController.SetWatcher(eventWatcher)

	cm.eventWatcher = eventWatcher
	cm.scaleUpController = scaleUpController

	// Add health checks to manager
	if err := cm.setupHealthChecks(); err != nil {
		return nil, fmt.Errorf("failed to setup health checks: %w", err)
	}

	// Setup controllers
	if err := cm.setupControllers(); err != nil {
		return nil, fmt.Errorf("failed to setup controllers: %w", err)
	}

	// Setup webhook server if enabled
	if opts.EnableWebhook {
		if err := cm.setupWebhook(); err != nil {
			return nil, fmt.Errorf("failed to setup webhook: %w", err)
		}
	}

	return cm, nil
}

// setupHealthChecks configures the health check endpoints
func (cm *ControllerManager) setupHealthChecks() error {
	// Add healthz endpoint (liveness probe)
	if err := cm.mgr.AddHealthzCheck("healthz", cm.healthzCheck); err != nil {
		return fmt.Errorf("failed to add healthz check: %w", err)
	}

	// Add readyz endpoint (readiness probe)
	if err := cm.mgr.AddReadyzCheck("readyz", cm.readyzCheck); err != nil {
		return fmt.Errorf("failed to add readyz check: %w", err)
	}

	// Add ping check
	if err := cm.mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add ping check: %w", err)
	}

	// Add VPSie API check
	if err := cm.mgr.AddReadyzCheck("vpsie-api", cm.vpsieAPICheck); err != nil {
		return fmt.Errorf("failed to add VPSie API check: %w", err)
	}

	return nil
}

// setupControllers sets up all controllers with the manager
func (cm *ControllerManager) setupControllers() error {
	// Setup NodeGroup controller
	nodeGroupReconciler := nodegroup.NewNodeGroupReconciler(
		cm.mgr.GetClient(),
		cm.scheme,
		cm.vpsieClient,
		cm.logger,
		cm.scaleDownManager,
	)

	if err := nodeGroupReconciler.SetupWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to setup NodeGroup controller: %w", err)
	}

	cm.logger.Info("Successfully registered NodeGroup controller")

	// Setup VPSieNode controller
	// Note: Node configuration is handled by VPSie API via QEMU agent
	vpsieNodeReconciler := vpsienode.NewVPSieNodeReconciler(
		cm.mgr.GetClient(),
		cm.scheme,
		cm.vpsieClient,
		cm.logger,
		cm.options.SSHKeyIDs,
		cm.options.FailedVPSieNodeTTL,
	)

	if err := vpsieNodeReconciler.SetupWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to setup VPSieNode controller: %w", err)
	}

	cm.logger.Info("Successfully registered VPSieNode controller")

	return nil
}

// setupWebhook configures the validating webhook server
func (cm *ControllerManager) setupWebhook() error {
	cm.logger.Info("Setting up validating webhook server",
		zap.String("addr", cm.options.WebhookAddr),
		zap.String("certDir", cm.options.WebhookCertDir),
	)

	server, err := webhook.NewServer(webhook.ServerConfig{
		Port:   extractPort(cm.options.WebhookAddr),
		Logger: cm.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create webhook server: %w", err)
	}

	cm.webhookServer = server
	cm.logger.Info("Webhook server configured successfully")

	return nil
}

// extractPort extracts the port number from an address string like ":9443" or "0.0.0.0:9443"
func extractPort(addr string) int {
	// Default port if parsing fails
	defaultPort := 9443

	if addr == "" {
		return defaultPort
	}

	// Find the last colon (for IPv6 compatibility)
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			portStr := addr[i+1:]
			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil {
				return port
			}
			break
		}
	}

	return defaultPort
}

// healthzCheck implements the liveness probe
func (cm *ControllerManager) healthzCheck(req *http.Request) error {
	if !cm.healthChecker.IsHealthy() {
		lastErr := cm.healthChecker.LastError()
		if lastErr != nil {
			return fmt.Errorf("health check failed: %w", lastErr)
		}
		return fmt.Errorf("controller is not healthy")
	}
	return nil
}

// readyzCheck implements the readiness probe
func (cm *ControllerManager) readyzCheck(req *http.Request) error {
	if !cm.healthChecker.IsReady() {
		lastErr := cm.healthChecker.LastError()
		if lastErr != nil {
			return fmt.Errorf("readiness check failed: %w", lastErr)
		}
		return fmt.Errorf("controller is not ready")
	}
	return nil
}

// vpsieAPICheck verifies connectivity to the VPSie API
func (cm *ControllerManager) vpsieAPICheck(req *http.Request) error {
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	_, err := cm.vpsieClient.ListVMs(ctx)
	if err != nil {
		return fmt.Errorf("VPSie API not reachable: %w", err)
	}
	return nil
}

// Start starts the controller manager and blocks until the context is cancelled
func (cm *ControllerManager) Start(ctx context.Context) error {
	cm.logger.Info("Starting VPSie Kubernetes Autoscaler",
		zap.String("version", os.Getenv("VERSION")),
		zap.String("commit", os.Getenv("COMMIT")),
		zap.Bool("leader_election", cm.options.EnableLeaderElection),
		zap.String("metrics_addr", cm.options.MetricsAddr),
		zap.String("health_addr", cm.options.HealthProbeAddr),
	)

	// Start health checker
	if err := cm.healthChecker.Start(ctx); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	cm.logger.Info("Health checks initialized successfully")

	// Load and activate AutoscalerConfig after cache is ready
	go func() {
		// Wait for cache to be ready by waiting for manager to start
		<-cm.mgr.Elected()
		cm.logger.Info("Leader elected, loading AutoscalerConfig")
		cm.loadAndActivateAutoscalerConfig(ctx)
	}()

	// Start event watcher for pending pod detection
	if cm.eventWatcher != nil {
		if err := cm.eventWatcher.Start(ctx); err != nil {
			return fmt.Errorf("failed to start event watcher: %w", err)
		}
		cm.logger.Info("Event watcher started for pending pod detection")
	}

	// Log VPSie client info
	cm.logger.Info("VPSie API client initialized",
		zap.String("endpoint", cm.vpsieClient.GetBaseURL()),
	)

	// Start node utilization metrics collection
	cm.startMetricsCollection(ctx)

	// Start webhook server if enabled
	if cm.webhookServer != nil {
		certFile := fmt.Sprintf("%s/%s", cm.options.WebhookCertDir, cm.options.WebhookCertFile)
		keyFile := fmt.Sprintf("%s/%s", cm.options.WebhookCertDir, cm.options.WebhookKeyFile)
		cm.logger.Info("Starting webhook server",
			zap.String("addr", cm.options.WebhookAddr),
			zap.String("certFile", certFile),
			zap.String("keyFile", keyFile),
		)
		go func() {
			if err := cm.webhookServer.Start(ctx, certFile, keyFile); err != nil {
				cm.logger.Error("Webhook server failed", zap.Error(err))
			}
		}()
	}

	// Start the manager (this blocks until context is cancelled)
	cm.logger.Info("Starting controller-runtime manager")
	if err := cm.mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	return nil
}

// startMetricsCollection starts a background goroutine to collect node utilization metrics
func (cm *ControllerManager) startMetricsCollection(ctx context.Context) {
	cm.logger.Info("Starting node utilization metrics collection",
		zap.Duration("interval", scaler.DefaultMetricsCollectionInterval))

	go func() {
		ticker := time.NewTicker(scaler.DefaultMetricsCollectionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				cm.logger.Info("Stopping metrics collection")
				return
			case <-ticker.C:
				// Use timeout to prevent goroutine leak if metrics API hangs
				// Timeout should be less than collection interval to avoid overlap
				// Note: cancel() is called immediately (not deferred) to avoid context leak in loop
				metricsCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				if err := cm.scaleDownManager.UpdateNodeUtilization(metricsCtx); err != nil {
					cm.logger.Error("Failed to update node utilization",
						zap.Error(err))
				}
				cancel() // Immediately clean up context resources
			}
		}
	}()
}

// GetManager returns the controller-runtime manager
func (cm *ControllerManager) GetManager() ctrl.Manager {
	return cm.mgr
}

// GetVPSieClient returns the VPSie API client
func (cm *ControllerManager) GetVPSieClient() *vpsieclient.Client {
	return cm.vpsieClient
}

// GetKubernetesClient returns the Kubernetes clientset
func (cm *ControllerManager) GetKubernetesClient() kubernetes.Interface {
	return cm.k8sClient
}

// GetLogger returns the logger
func (cm *ControllerManager) GetLogger() *zap.Logger {
	return cm.logger
}

// GetHealthChecker returns the health checker
func (cm *ControllerManager) GetHealthChecker() *HealthChecker {
	return cm.healthChecker
}

// GetScaleDownManager returns the scale-down manager
func (cm *ControllerManager) GetScaleDownManager() *scaler.ScaleDownManager {
	return cm.scaleDownManager
}

// Shutdown gracefully shuts down the controller manager
func (cm *ControllerManager) Shutdown(ctx context.Context) error {
	cm.logger.Info("Initiating graceful shutdown")

	// Mark as not ready to stop receiving new traffic
	cm.healthChecker.SetReady(false)

	// Wait a bit to allow load balancers to remove this instance
	shutdownDelay := 5 * time.Second
	cm.logger.Info("Waiting before shutdown", zap.Duration("delay", shutdownDelay))

	select {
	case <-time.After(shutdownDelay):
	case <-ctx.Done():
		cm.logger.Warn("Shutdown deadline exceeded during delay")
	}

	cm.logger.Info("Shutdown complete")
	return nil
}

// loadAndActivateAutoscalerConfig loads the AutoscalerConfig CRD and updates its status to active.
// This also applies configuration from the CRD to the DynamicNodeGroupCreator.
func (cm *ControllerManager) loadAndActivateAutoscalerConfig(ctx context.Context) {
	config := &v1alpha1.AutoscalerConfig{}
	err := cm.mgr.GetClient().Get(ctx, client.ObjectKey{Name: "default"}, config)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			cm.logger.Info("No AutoscalerConfig CRD found, using default configuration")
			return
		}
		cm.logger.Warn("Failed to get AutoscalerConfig", zap.Error(err))
		return
	}

	cm.logger.Info("Found AutoscalerConfig CRD, applying configuration",
		zap.Int32("maxClusterWorkers", config.Spec.GlobalSettings.MaxClusterWorkers),
		zap.Int32("minNodes", config.Spec.NodeGroupDefaults.MinNodes),
		zap.Int32("maxNodes", config.Spec.NodeGroupDefaults.MaxNodes),
	)

	// Update the AutoscalerConfig status to show it's active
	now := metav1.Now()
	config.Status.Active = true
	config.Status.ObservedGeneration = config.Generation
	config.Status.LastUpdated = &now
	config.Status.Message = "Configuration loaded and active"

	if err := cm.mgr.GetClient().Status().Update(ctx, config); err != nil {
		cm.logger.Warn("Failed to update AutoscalerConfig status", zap.Error(err))
	} else {
		cm.logger.Info("AutoscalerConfig status updated to active")
	}

	// Apply configuration to the DynamicNodeGroupCreator if we have one
	if cm.scaleUpController != nil {
		cm.logger.Info("Configuration from AutoscalerConfig will be used for dynamic NodeGroup creation")
	}

	// Update existing managed NodeGroups with the new configuration
	cm.syncManagedNodeGroups(ctx, config)
}

// syncManagedNodeGroups updates existing dynamically-created NodeGroups with AutoscalerConfig values
func (cm *ControllerManager) syncManagedNodeGroups(ctx context.Context, config *v1alpha1.AutoscalerConfig) {
	// List all NodeGroups with the managed label
	nodeGroupList := &v1alpha1.NodeGroupList{}
	if err := cm.mgr.GetClient().List(ctx, nodeGroupList, client.MatchingLabels{
		"autoscaler.vpsie.com/managed": "true",
	}); err != nil {
		cm.logger.Warn("Failed to list managed NodeGroups", zap.Error(err))
		return
	}

	if len(nodeGroupList.Items) == 0 {
		cm.logger.Debug("No managed NodeGroups found to sync")
		return
	}

	defaults := config.Spec.NodeGroupDefaults
	updated := 0

	for i := range nodeGroupList.Items {
		ngRef := &nodeGroupList.Items[i]

		// Update NodeGroup with retry logic to handle conflicts
		if cm.updateNodeGroupWithRetry(ctx, ngRef.Name, ngRef.Namespace, defaults) {
			updated++
		}
	}

	if updated > 0 {
		cm.logger.Info("Synced managed NodeGroups with AutoscalerConfig",
			zap.Int("updated", updated),
			zap.Int("total", len(nodeGroupList.Items)),
		)
	}
}

// updateNodeGroupWithRetry updates a NodeGroup with retry logic for conflict handling
func (cm *ControllerManager) updateNodeGroupWithRetry(ctx context.Context, name, namespace string, defaults v1alpha1.NodeGroupDefaults) bool {
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Always fetch fresh copy before update
		ng := &v1alpha1.NodeGroup{}
		if err := cm.mgr.GetClient().Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ng); err != nil {
			cm.logger.Warn("Failed to get NodeGroup for update",
				zap.String("nodeGroup", name),
				zap.Error(err),
			)
			return false
		}

		needsUpdate := false

		// Check if maxNodes needs to be updated
		if defaults.MaxNodes > 0 && ng.Spec.MaxNodes != defaults.MaxNodes {
			cm.logger.Info("Updating NodeGroup maxNodes",
				zap.String("nodeGroup", ng.Name),
				zap.Int32("oldMax", ng.Spec.MaxNodes),
				zap.Int32("newMax", defaults.MaxNodes),
			)
			ng.Spec.MaxNodes = defaults.MaxNodes
			needsUpdate = true
		}

		// Check if minNodes needs to be updated
		if ng.Spec.MinNodes != defaults.MinNodes {
			cm.logger.Info("Updating NodeGroup minNodes",
				zap.String("nodeGroup", ng.Name),
				zap.Int32("oldMin", ng.Spec.MinNodes),
				zap.Int32("newMin", defaults.MinNodes),
			)
			ng.Spec.MinNodes = defaults.MinNodes
			needsUpdate = true
		}

		if !needsUpdate {
			return false // No update needed
		}

		if err := cm.mgr.GetClient().Update(ctx, ng); err != nil {
			if strings.Contains(err.Error(), "the object has been modified") && attempt < maxRetries-1 {
				cm.logger.Debug("NodeGroup update conflict, retrying",
					zap.String("nodeGroup", name),
					zap.Int("attempt", attempt+1),
				)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			cm.logger.Warn("Failed to update managed NodeGroup",
				zap.String("nodeGroup", name),
				zap.Error(err),
			)
			return false
		}

		return true // Success
	}

	return false
}

// newLogger creates a new zap logger based on options
func newLogger(opts *Options) (*zap.Logger, error) {
	var config zap.Config

	if opts.DevelopmentMode {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	// Set log level
	switch opts.LogLevel {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Set encoding
	if opts.LogFormat == "console" {
		config.Encoding = "console"
	} else {
		config.Encoding = "json"
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}
