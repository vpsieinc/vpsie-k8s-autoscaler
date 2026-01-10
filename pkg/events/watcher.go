package events

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

const (
	// ReasonFailedScheduling is the event reason for failed pod scheduling
	ReasonFailedScheduling = "FailedScheduling"

	// DefaultStabilizationWindow is the default time to wait before scaling up
	DefaultStabilizationWindow = 60 * time.Second

	// DefaultEventBufferSize is the default size of the event buffer
	DefaultEventBufferSize = 100

	// MaxEventBufferSize is the maximum size of the event buffer to prevent unbounded growth
	MaxEventBufferSize = 1000

	// EventProcessInterval is how often to process accumulated events
	EventProcessInterval = 5 * time.Second
)

// Pre-compiled regex patterns for parseConstraint to avoid repeated compilation
var (
	// Pod count patterns
	podsPatternRe = regexp.MustCompile(`too many pods|maximum number of pods`)

	// Resource patterns
	cpuPatternRe    = regexp.MustCompile(`insufficient.*cpu`)
	memoryPatternRe = regexp.MustCompile(`insufficient.*memory`)

	// Taint patterns
	taintPatternRe = regexp.MustCompile(`untolerated taint|had taint|taints that the pod didn't tolerate`)

	// Affinity/Anti-affinity patterns
	antiAffinityPatternRe = regexp.MustCompile(`anti-affinity|didn't match pod anti-affinity|pod anti-affinity`)
	affinityPatternRe     = regexp.MustCompile(`pod affinity|didn't match pod affinity`)

	// Node selector patterns
	nodeSelectorPatternRe = regexp.MustCompile(`node selector|didn't match pod's node selector|didn't match pod requirements|no nodes available|nodes are available`)
)

// ResourceConstraint represents a type of resource constraint
type ResourceConstraint string

const (
	// ConstraintCPU indicates insufficient CPU
	ConstraintCPU ResourceConstraint = "cpu"

	// ConstraintMemory indicates insufficient memory
	ConstraintMemory ResourceConstraint = "memory"

	// ConstraintPods indicates too many pods
	ConstraintPods ResourceConstraint = "pods"

	// ConstraintNodeSelector indicates pod's node selector couldn't be satisfied
	ConstraintNodeSelector ResourceConstraint = "node_selector"

	// ConstraintTaint indicates node(s) had taints the pod didn't tolerate
	ConstraintTaint ResourceConstraint = "taint"

	// ConstraintAffinity indicates pod affinity rules couldn't be satisfied
	ConstraintAffinity ResourceConstraint = "affinity"

	// ConstraintAntiAffinity indicates pod anti-affinity rules couldn't be satisfied
	ConstraintAntiAffinity ResourceConstraint = "anti_affinity"

	// ConstraintUnknown indicates an unknown constraint (still triggers scale-up)
	ConstraintUnknown ResourceConstraint = "unknown"
)

// SchedulingEvent represents a pod scheduling failure event
type SchedulingEvent struct {
	Pod        *corev1.Pod
	Event      *corev1.Event
	Timestamp  time.Time
	Constraint ResourceConstraint
	Message    string
}

// EventWatcher watches for pod scheduling failure events
type EventWatcher struct {
	client              client.Client
	clientset           kubernetes.Interface
	logger              *zap.Logger
	informer            cache.SharedIndexInformer
	stopCh              chan struct{}
	eventBuffer         []SchedulingEvent
	eventBufferMu       sync.RWMutex
	scaleUpHandler      ScaleUpHandler
	lastScaleTime       map[string]time.Time
	lastScaleTimeMu     sync.RWMutex
	stabilizationWindow time.Duration
}

// ScaleUpHandler is called when scale-up is needed
type ScaleUpHandler func(ctx context.Context, events []SchedulingEvent) error

// NewEventWatcher creates a new event watcher
func NewEventWatcher(
	client client.Client,
	clientset kubernetes.Interface,
	logger *zap.Logger,
	scaleUpHandler ScaleUpHandler,
) *EventWatcher {
	return &EventWatcher{
		client:              client,
		clientset:           clientset,
		logger:              logger.Named("event-watcher"),
		stopCh:              make(chan struct{}),
		eventBuffer:         make([]SchedulingEvent, 0, DefaultEventBufferSize),
		scaleUpHandler:      scaleUpHandler,
		lastScaleTime:       make(map[string]time.Time),
		stabilizationWindow: DefaultStabilizationWindow,
	}
}

// Start starts the event watcher
func (w *EventWatcher) Start(ctx context.Context) error {
	w.logger.Info("Starting event watcher")

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(w.clientset, 0)
	w.informer = informerFactory.Core().V1().Events().Informer()

	// Add event handler
	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event, ok := obj.(*corev1.Event)
			if !ok {
				return
			}
			w.handleEvent(ctx, event)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			event, ok := newObj.(*corev1.Event)
			if !ok {
				return
			}
			w.handleEvent(ctx, event)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	// Start informer
	go w.informer.Run(w.stopCh)

	// Wait for cache sync
	if !cache.WaitForCacheSync(w.stopCh, w.informer.HasSynced) {
		return fmt.Errorf("failed to sync event cache")
	}

	w.logger.Info("Event watcher started and cache synced")

	// Start event processor
	go w.processEventsLoop(ctx)

	return nil
}

// Stop stops the event watcher
func (w *EventWatcher) Stop() {
	w.logger.Info("Stopping event watcher")
	close(w.stopCh)
}

// handleEvent processes a single event
func (w *EventWatcher) handleEvent(ctx context.Context, event *corev1.Event) {
	// Only process FailedScheduling events
	if event.Reason != ReasonFailedScheduling {
		return
	}

	// Check if event is related to a pod
	if event.InvolvedObject.Kind != "Pod" {
		return
	}

	// Parse the constraint type from the message
	// All FailedScheduling events are considered - the ResourceAnalyzer will filter
	// pods that can't match any NodeGroup
	constraint := parseConstraint(event.Message)

	w.logger.Debug("Detected scheduling failure",
		zap.String("pod", event.InvolvedObject.Name),
		zap.String("namespace", event.InvolvedObject.Namespace),
		zap.String("constraint", string(constraint)),
		zap.String("message", event.Message),
	)

	// Get the pod using direct clientset (not cached client) to avoid cache startup issues
	pod, err := w.clientset.CoreV1().Pods(event.InvolvedObject.Namespace).Get(ctx, event.InvolvedObject.Name, metav1.GetOptions{})
	if err != nil {
		w.logger.Warn("Failed to get pod for event",
			zap.String("pod", event.InvolvedObject.Name),
			zap.Error(err),
		)
		return
	}

	// Create scheduling event
	schedEvent := SchedulingEvent{
		Pod:        pod,
		Event:      event,
		Timestamp:  time.Now(),
		Constraint: constraint,
		Message:    event.Message,
	}

	// Add to buffer with size limit to prevent unbounded growth
	w.eventBufferMu.Lock()
	if len(w.eventBuffer) >= MaxEventBufferSize {
		// Drop oldest event to make room
		w.logger.Warn("Event buffer full, dropping oldest event",
			zap.Int("bufferSize", len(w.eventBuffer)),
			zap.Int("maxSize", MaxEventBufferSize),
		)
		w.eventBuffer = w.eventBuffer[1:]
		metrics.EventBufferDropped.Inc()
	}
	w.eventBuffer = append(w.eventBuffer, schedEvent)
	metrics.EventBufferSize.Set(float64(len(w.eventBuffer)))
	w.eventBufferMu.Unlock()
}

// processEventsLoop periodically processes accumulated events
func (w *EventWatcher) processEventsLoop(ctx context.Context) {
	ticker := time.NewTicker(EventProcessInterval)
	defer ticker.Stop()

	// Cleanup ticker runs less frequently (2x stabilization window)
	cleanupInterval := 2 * w.stabilizationWindow
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.processEvents(ctx)
		case <-cleanupTicker.C:
			w.cleanupLastScaleTime()
		}
	}
}

// cleanupLastScaleTime removes stale entries from the lastScaleTime map.
// Entries older than 2x stabilization window are removed to prevent memory leaks.
func (w *EventWatcher) cleanupLastScaleTime() {
	w.lastScaleTimeMu.Lock()
	defer w.lastScaleTimeMu.Unlock()

	cutoff := time.Now().Add(-2 * w.stabilizationWindow)
	removed := 0

	for name, lastScale := range w.lastScaleTime {
		if lastScale.Before(cutoff) {
			delete(w.lastScaleTime, name)
			removed++
		}
	}

	if removed > 0 {
		w.logger.Debug("Cleaned up stale lastScaleTime entries",
			zap.Int("removed", removed),
			zap.Int("remaining", len(w.lastScaleTime)),
		)
	}
}

// processEvents processes accumulated events and triggers scale-up if needed
func (w *EventWatcher) processEvents(ctx context.Context) {
	w.eventBufferMu.Lock()
	if len(w.eventBuffer) == 0 {
		w.eventBufferMu.Unlock()
		return
	}

	// Get events and clear buffer
	events := make([]SchedulingEvent, len(w.eventBuffer))
	copy(events, w.eventBuffer)
	w.eventBuffer = w.eventBuffer[:0]
	metrics.EventBufferSize.Set(0)
	w.eventBufferMu.Unlock()

	w.logger.Info("Processing scheduling failure events",
		zap.Int("count", len(events)),
	)

	// Filter out stale events (older than stabilization window)
	recentEvents := w.filterRecentEvents(events)
	if len(recentEvents) == 0 {
		w.logger.Debug("No recent events after filtering")
		return
	}

	// Call scale-up handler
	if w.scaleUpHandler != nil {
		if err := w.scaleUpHandler(ctx, recentEvents); err != nil {
			w.logger.Error("Scale-up handler failed",
				zap.Error(err),
			)
		}
	}
}

// filterRecentEvents filters events within the stabilization window
func (w *EventWatcher) filterRecentEvents(events []SchedulingEvent) []SchedulingEvent {
	now := time.Now()
	cutoff := now.Add(-w.stabilizationWindow)

	filtered := make([]SchedulingEvent, 0, len(events))
	for _, event := range events {
		if event.Timestamp.After(cutoff) {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// RecordScaleEvent records a scale event for a NodeGroup
func (w *EventWatcher) RecordScaleEvent(nodeGroupName string) {
	w.lastScaleTimeMu.Lock()
	defer w.lastScaleTimeMu.Unlock()
	w.lastScaleTime[nodeGroupName] = time.Now()
}

// CanScale checks if a NodeGroup can be scaled (respects cooldown)
func (w *EventWatcher) CanScale(nodeGroupName string) bool {
	w.lastScaleTimeMu.RLock()
	defer w.lastScaleTimeMu.RUnlock()

	lastScale, exists := w.lastScaleTime[nodeGroupName]
	if !exists {
		return true
	}

	// Check if we're still in cooldown period
	cooldown := time.Since(lastScale)
	return cooldown >= w.stabilizationWindow
}

// parseConstraint extracts the resource constraint type from event message.
// Uses pre-compiled regex patterns for efficiency since this is called for every event.
func parseConstraint(message string) ResourceConstraint {
	message = strings.ToLower(message)

	// Check in order of specificity using pre-compiled patterns

	// Check pods first (most specific resource constraint)
	if podsPatternRe.MatchString(message) {
		return ConstraintPods
	}

	// Check CPU
	if cpuPatternRe.MatchString(message) {
		return ConstraintCPU
	}

	// Check memory
	if memoryPatternRe.MatchString(message) {
		return ConstraintMemory
	}

	// Check taints (often combined with affinity in messages)
	if taintPatternRe.MatchString(message) {
		return ConstraintTaint
	}

	// Check anti-affinity (before affinity since anti-affinity contains "affinity")
	if antiAffinityPatternRe.MatchString(message) {
		return ConstraintAntiAffinity
	}

	// Check affinity
	if affinityPatternRe.MatchString(message) {
		return ConstraintAffinity
	}

	// Check node selector
	if nodeSelectorPatternRe.MatchString(message) {
		return ConstraintNodeSelector
	}

	return ConstraintUnknown
}

// GetPendingPods returns all currently pending pods
func (w *EventWatcher) GetPendingPods(ctx context.Context) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := w.client.List(ctx, podList, &client.ListOptions{
		LabelSelector: labels.Everything(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pending := make([]corev1.Pod, 0)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodPending &&
			pod.Spec.NodeName == "" &&
			pod.DeletionTimestamp == nil {
			pending = append(pending, pod)
		}
	}

	return pending, nil
}

// GetNodeGroups returns all managed NodeGroups.
// NodeGroup isolation: Only returns NodeGroups with the managed label
// (autoscaler.vpsie.com/managed=true) to prevent the autoscaler from
// interfering with externally created or static NodeGroups.
func (w *EventWatcher) GetNodeGroups(ctx context.Context) ([]v1alpha1.NodeGroup, error) {
	nodeGroupList := &v1alpha1.NodeGroupList{}
	// Filter to only managed NodeGroups using label selector
	err := w.client.List(ctx, nodeGroupList, client.MatchingLabels{
		v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list managed NodeGroups: %w", err)
	}

	return nodeGroupList.Items, nil
}
