package scaler

import (
	"context"
	"strconv"
	"strings"
	"time"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"

	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
)

// PolicyMode defines the scale-down behavior mode
type PolicyMode string

const (
	// PolicyModeAggressive removes nodes quickly when underutilized
	PolicyModeAggressive PolicyMode = "aggressive"

	// PolicyModeBalanced balances between cost savings and stability
	PolicyModeBalanced PolicyMode = "balanced"

	// PolicyModeConservative removes nodes slowly and carefully
	PolicyModeConservative PolicyMode = "conservative"

	// PolicyModeDisabled disables scale-down entirely
	PolicyModeDisabled PolicyMode = "disabled"
)

// TimeWindow defines a time-based policy window
type TimeWindow struct {
	StartHour int // 0-23
	EndHour   int // 0-23
	Days      []time.Weekday
	Mode      PolicyMode
}

// PolicyEngine manages scale-down policies
type PolicyEngine struct {
	config      *Config
	logger      *zap.SugaredLogger
	timeWindows []TimeWindow
	defaultMode PolicyMode
}

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine(logger *zap.SugaredLogger, config *Config) *PolicyEngine {
	return &PolicyEngine{
		config:      config,
		logger:      logger,
		timeWindows: []TimeWindow{},
		defaultMode: PolicyModeBalanced,
	}
}

// AllowScaleDown determines if scale-down is allowed based on policies
func (p *PolicyEngine) AllowScaleDown(
	ctx context.Context,
	nodeGroup *autoscalerv1alpha1.NodeGroup,
	node *corev1.Node,
) bool {
	// Check if scale-down is globally disabled
	currentMode := p.getCurrentMode()
	if currentMode == PolicyModeDisabled {
		p.logger.Debug("scale-down disabled by policy", "node", node.Name)
		return false
	}

	// Check NodeGroup-specific scale-down policy
	if !nodeGroup.Spec.ScaleDownPolicy.Enabled {
		p.logger.Debug("scale-down disabled for NodeGroup",
			"nodeGroup", nodeGroup.Name,
			"node", node.Name)
		return false
	}

	// Check time-based policies
	if !p.isWithinAllowedTime() {
		p.logger.Debug("scale-down not allowed at current time", "node", node.Name)
		return false
	}

	// Check node-specific policy annotations
	if !p.isNodeAllowedForScaleDown(node) {
		return false
	}

	return true
}

// getCurrentMode returns the current policy mode based on time windows
func (p *PolicyEngine) getCurrentMode() PolicyMode {
	now := time.Now()
	currentHour := now.Hour()
	currentDay := now.Weekday()

	for _, window := range p.timeWindows {
		// Check if current day matches
		dayMatches := false
		for _, day := range window.Days {
			if day == currentDay {
				dayMatches = true
				break
			}
		}

		if !dayMatches {
			continue
		}

		// Check if current hour is within window
		if p.isHourInWindow(currentHour, window.StartHour, window.EndHour) {
			return window.Mode
		}
	}

	return p.defaultMode
}

// isHourInWindow checks if an hour falls within a time window
func (p *PolicyEngine) isHourInWindow(hour, start, end int) bool {
	if start <= end {
		return hour >= start && hour < end
	}
	// Handle overnight windows (e.g., 22:00 - 02:00)
	return hour >= start || hour < end
}

// isWithinAllowedTime checks if current time allows scale-down
func (p *PolicyEngine) isWithinAllowedTime() bool {
	mode := p.getCurrentMode()
	return mode != PolicyModeDisabled
}

// isNodeAllowedForScaleDown checks node-specific policies
func (p *PolicyEngine) isNodeAllowedForScaleDown(node *corev1.Node) bool {
	if node.Annotations == nil {
		return true
	}

	// Check for scale-down disable annotation
	if val, exists := node.Annotations["autoscaler.vpsie.com/scale-down"]; exists && val == "disabled" {
		p.logger.Debug("scale-down disabled by node annotation", "node", node.Name)
		return false
	}

	// Check for specific time window annotation
	if val, exists := node.Annotations["autoscaler.vpsie.com/scale-down-allowed-hours"]; exists {
		if !p.isWithinAnnotatedHours(val) {
			p.logger.Debug("current time outside node's allowed scale-down hours",
				"node", node.Name,
				"allowedHours", val)
			return false
		}
	}

	return true
}

// isWithinAnnotatedHours checks if current time is within annotated hours
// Format: "HH:MM-HH:MM" e.g., "09:00-17:00" or "22:00-02:00" (overnight)
func (p *PolicyEngine) isWithinAnnotatedHours(hoursStr string) bool {
	now := time.Now()
	currentHour := now.Hour()
	currentMinute := now.Minute()

	// Parse format "HH:MM-HH:MM"
	parts := strings.Split(hoursStr, "-")
	if len(parts) != 2 {
		p.logger.Warn("invalid hours format, expected HH:MM-HH:MM", "format", hoursStr)
		return true // Fail open for safety - allow scale-down
	}

	// Parse start time
	startParts := strings.Split(strings.TrimSpace(parts[0]), ":")
	if len(startParts) != 2 {
		p.logger.Warn("invalid start time format", "time", parts[0])
		return true
	}
	startHour, err1 := strconv.Atoi(startParts[0])
	startMin, err2 := strconv.Atoi(startParts[1])
	if err1 != nil || err2 != nil || startHour < 0 || startHour > 23 || startMin < 0 || startMin > 59 {
		p.logger.Warn("invalid start time values", "time", parts[0])
		return true
	}

	// Parse end time
	endParts := strings.Split(strings.TrimSpace(parts[1]), ":")
	if len(endParts) != 2 {
		p.logger.Warn("invalid end time format", "time", parts[1])
		return true
	}
	endHour, err3 := strconv.Atoi(endParts[0])
	endMin, err4 := strconv.Atoi(endParts[1])
	if err3 != nil || err4 != nil || endHour < 0 || endHour > 23 || endMin < 0 || endMin > 59 {
		p.logger.Warn("invalid end time values", "time", parts[1])
		return true
	}

	// Convert to minutes since midnight for easier comparison
	currentMinutesSinceMidnight := currentHour*60 + currentMinute
	startMinutesSinceMidnight := startHour*60 + startMin
	endMinutesSinceMidnight := endHour*60 + endMin

	// Handle overnight windows (e.g., 22:00-02:00)
	if endMinutesSinceMidnight < startMinutesSinceMidnight {
		// Overnight: we're in window if current time >= start OR current time < end
		return currentMinutesSinceMidnight >= startMinutesSinceMidnight ||
			currentMinutesSinceMidnight < endMinutesSinceMidnight
	}

	// Normal window: we're in window if current time is between start and end
	return currentMinutesSinceMidnight >= startMinutesSinceMidnight &&
		currentMinutesSinceMidnight < endMinutesSinceMidnight
}

// SetDefaultMode sets the default policy mode
func (p *PolicyEngine) SetDefaultMode(mode PolicyMode) {
	p.defaultMode = mode
	p.logger.Info("policy mode updated", "mode", mode)
}

// AddTimeWindow adds a time-based policy window
func (p *PolicyEngine) AddTimeWindow(window TimeWindow) {
	p.timeWindows = append(p.timeWindows, window)
	p.logger.Info("time window added",
		"startHour", window.StartHour,
		"endHour", window.EndHour,
		"mode", window.Mode)
}

// GetThresholds returns scale-down thresholds based on current mode
func (p *PolicyEngine) GetThresholds() (cpuThreshold, memoryThreshold float64, observationWindow time.Duration) {
	mode := p.getCurrentMode()

	switch mode {
	case PolicyModeAggressive:
		return 60.0, 60.0, 5 * time.Minute
	case PolicyModeBalanced:
		return p.config.CPUThreshold, p.config.MemoryThreshold, p.config.ObservationWindow
	case PolicyModeConservative:
		return 40.0, 40.0, 20 * time.Minute
	case PolicyModeDisabled:
		return 100.0, 100.0, 24 * time.Hour
	default:
		return p.config.CPUThreshold, p.config.MemoryThreshold, p.config.ObservationWindow
	}
}

// ShouldDelayScaleDown determines if scale-down should be delayed
func (p *PolicyEngine) ShouldDelayScaleDown(nodeGroup *autoscalerv1alpha1.NodeGroup) bool {
	mode := p.getCurrentMode()

	switch mode {
	case PolicyModeAggressive:
		return false // No delay
	case PolicyModeBalanced:
		return false // Use configured cooldown
	case PolicyModeConservative:
		return true // Always delay for extra safety
	default:
		return false
	}
}

// GetMaxConcurrentScaleDowns returns max nodes to scale down concurrently
func (p *PolicyEngine) GetMaxConcurrentScaleDowns() int {
	mode := p.getCurrentMode()

	switch mode {
	case PolicyModeAggressive:
		return p.config.MaxNodesPerScaleDown * 2
	case PolicyModeBalanced:
		return p.config.MaxNodesPerScaleDown
	case PolicyModeConservative:
		return 1 // One at a time
	default:
		return p.config.MaxNodesPerScaleDown
	}
}

// Example time windows for common scenarios

// GetBusinessHoursWindow returns a policy for business hours (9-17, Mon-Fri)
func GetBusinessHoursWindow() TimeWindow {
	return TimeWindow{
		StartHour: 9,
		EndHour:   17,
		Days: []time.Weekday{
			time.Monday,
			time.Tuesday,
			time.Wednesday,
			time.Thursday,
			time.Friday,
		},
		Mode: PolicyModeConservative, // Be conservative during business hours
	}
}

// GetOffHoursWindow returns a policy for off hours
func GetOffHoursWindow() TimeWindow {
	return TimeWindow{
		StartHour: 18,
		EndHour:   8, // Next day
		Days: []time.Weekday{
			time.Monday,
			time.Tuesday,
			time.Wednesday,
			time.Thursday,
			time.Friday,
			time.Saturday,
			time.Sunday,
		},
		Mode: PolicyModeAggressive, // Aggressive cost savings off hours
	}
}

// GetWeekendWindow returns a policy for weekends
func GetWeekendWindow() TimeWindow {
	return TimeWindow{
		StartHour: 0,
		EndHour:   24,
		Days: []time.Weekday{
			time.Saturday,
			time.Sunday,
		},
		Mode: PolicyModeAggressive, // Aggressive on weekends
	}
}

// PolicyPreset defines common policy configurations
type PolicyPreset string

const (
	PresetProduction  PolicyPreset = "production"
	PresetDevelopment PolicyPreset = "development"
	PresetCostSaving  PolicyPreset = "cost-saving"
)

// ApplyPreset configures the policy engine with a preset
func (p *PolicyEngine) ApplyPreset(preset PolicyPreset) {
	switch preset {
	case PresetProduction:
		p.SetDefaultMode(PolicyModeConservative)
		p.AddTimeWindow(GetBusinessHoursWindow())
		p.logger.Info("applied production preset")

	case PresetDevelopment:
		p.SetDefaultMode(PolicyModeBalanced)
		p.logger.Info("applied development preset")

	case PresetCostSaving:
		p.SetDefaultMode(PolicyModeAggressive)
		p.AddTimeWindow(GetOffHoursWindow())
		p.AddTimeWindow(GetWeekendWindow())
		p.logger.Info("applied cost-saving preset")
	}
}

// GetPolicyStatus returns current policy status
func (p *PolicyEngine) GetPolicyStatus() map[string]interface{} {
	cpuThreshold, memThreshold, observationWindow := p.GetThresholds()

	return map[string]interface{}{
		"currentMode":        p.getCurrentMode(),
		"defaultMode":        p.defaultMode,
		"cpuThreshold":       cpuThreshold,
		"memoryThreshold":    memThreshold,
		"observationWindow":  observationWindow.String(),
		"maxConcurrentScale": p.GetMaxConcurrentScaleDowns(),
		"timeWindows":        len(p.timeWindows),
	}
}
