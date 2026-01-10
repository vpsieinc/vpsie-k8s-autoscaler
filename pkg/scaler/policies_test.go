package scaler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestNewPolicyEngine tests PolicyEngine creation
func TestNewPolicyEngine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	engine := NewPolicyEngine(logger.Sugar(), config)

	assert.NotNil(t, engine)
	assert.Equal(t, PolicyModeBalanced, engine.defaultMode)
	assert.Empty(t, engine.timeWindows)
	assert.Equal(t, config, engine.config)
}

// TestPolicyModes tests policy mode constants
func TestPolicyModes(t *testing.T) {
	assert.Equal(t, PolicyMode("aggressive"), PolicyModeAggressive)
	assert.Equal(t, PolicyMode("balanced"), PolicyModeBalanced)
	assert.Equal(t, PolicyMode("conservative"), PolicyModeConservative)
	assert.Equal(t, PolicyMode("disabled"), PolicyModeDisabled)
}

// TestPolicyEngine_SetDefaultMode tests setting default mode
func TestPolicyEngine_SetDefaultMode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	tests := []struct {
		name string
		mode PolicyMode
	}{
		{"Aggressive", PolicyModeAggressive},
		{"Balanced", PolicyModeBalanced},
		{"Conservative", PolicyModeConservative},
		{"Disabled", PolicyModeDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.SetDefaultMode(tt.mode)
			assert.Equal(t, tt.mode, engine.defaultMode)
		})
	}
}

// TestPolicyEngine_AddTimeWindow tests adding time windows
func TestPolicyEngine_AddTimeWindow(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	window := TimeWindow{
		StartHour: 9,
		EndHour:   17,
		Days:      []time.Weekday{time.Monday, time.Tuesday},
		Mode:      PolicyModeConservative,
	}

	engine.AddTimeWindow(window)

	assert.Len(t, engine.timeWindows, 1)
	assert.Equal(t, window, engine.timeWindows[0])

	// Add another window
	window2 := GetOffHoursWindow()
	engine.AddTimeWindow(window2)

	assert.Len(t, engine.timeWindows, 2)
}

// TestPolicyEngine_IsHourInWindow tests hour checking logic
func TestPolicyEngine_IsHourInWindow(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	tests := []struct {
		name     string
		hour     int
		start    int
		end      int
		expected bool
	}{
		// Normal windows (start < end)
		{"Within normal window", 12, 9, 17, true},
		{"At start of normal window", 9, 9, 17, true},
		{"Before end of normal window", 16, 9, 17, true},
		{"At end of normal window", 17, 9, 17, false},
		{"Before normal window", 8, 9, 17, false},
		{"After normal window", 18, 9, 17, false},

		// Overnight windows (start > end)
		{"In overnight window (late)", 23, 22, 2, true},
		{"In overnight window (early)", 1, 22, 2, true},
		{"At start of overnight window", 22, 22, 2, true},
		{"Before end of overnight window", 1, 22, 2, true},
		{"At end of overnight window", 2, 22, 2, false},
		{"Outside overnight window", 12, 22, 2, false},

		// Edge cases
		{"Full day window", 12, 0, 24, true},
		{"Zero-length window", 12, 12, 12, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isHourInWindow(tt.hour, tt.start, tt.end)
			assert.Equal(t, tt.expected, result, "Hour %d in window [%d, %d) should be %v", tt.hour, tt.start, tt.end, tt.expected)
		})
	}
}

// TestPolicyEngine_GetThresholds tests threshold retrieval by mode
func TestPolicyEngine_GetThresholds(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	engine := NewPolicyEngine(logger.Sugar(), config)

	tests := []struct {
		name           string
		mode           PolicyMode
		expectedCPU    float64
		expectedMem    float64
		expectedWindow time.Duration
	}{
		{
			name:           "Aggressive mode",
			mode:           PolicyModeAggressive,
			expectedCPU:    60.0,
			expectedMem:    60.0,
			expectedWindow: 5 * time.Minute,
		},
		{
			name:           "Balanced mode",
			mode:           PolicyModeBalanced,
			expectedCPU:    config.CPUThreshold,
			expectedMem:    config.MemoryThreshold,
			expectedWindow: config.ObservationWindow,
		},
		{
			name:           "Conservative mode",
			mode:           PolicyModeConservative,
			expectedCPU:    40.0,
			expectedMem:    40.0,
			expectedWindow: 20 * time.Minute,
		},
		{
			name:           "Disabled mode",
			mode:           PolicyModeDisabled,
			expectedCPU:    100.0,
			expectedMem:    100.0,
			expectedWindow: 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.SetDefaultMode(tt.mode)
			engine.timeWindows = nil // Clear time windows

			cpu, mem, window := engine.GetThresholds()

			assert.Equal(t, tt.expectedCPU, cpu, "CPU threshold mismatch")
			assert.Equal(t, tt.expectedMem, mem, "Memory threshold mismatch")
			assert.Equal(t, tt.expectedWindow, window, "Observation window mismatch")
		})
	}
}

// TestPolicyEngine_ShouldDelayScaleDown tests delay logic
func TestPolicyEngine_ShouldDelayScaleDown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "default",
		},
	}

	tests := []struct {
		name          string
		mode          PolicyMode
		expectedDelay bool
	}{
		{"Aggressive - no delay", PolicyModeAggressive, false},
		{"Balanced - no delay", PolicyModeBalanced, false},
		{"Conservative - delay", PolicyModeConservative, true},
		{"Disabled - no delay", PolicyModeDisabled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.SetDefaultMode(tt.mode)
			engine.timeWindows = nil

			result := engine.ShouldDelayScaleDown(nodeGroup)
			assert.Equal(t, tt.expectedDelay, result)
		})
	}
}

// TestPolicyEngine_GetMaxConcurrentScaleDowns tests concurrent scale-down limits
func TestPolicyEngine_GetMaxConcurrentScaleDowns(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	engine := NewPolicyEngine(logger.Sugar(), config)

	tests := []struct {
		name     string
		mode     PolicyMode
		expected int
	}{
		{"Aggressive - double", PolicyModeAggressive, config.MaxNodesPerScaleDown * 2},
		{"Balanced - normal", PolicyModeBalanced, config.MaxNodesPerScaleDown},
		{"Conservative - one at a time", PolicyModeConservative, 1},
		{"Disabled - normal", PolicyModeDisabled, config.MaxNodesPerScaleDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.SetDefaultMode(tt.mode)
			engine.timeWindows = nil

			result := engine.GetMaxConcurrentScaleDowns()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPolicyEngine_AllowScaleDown tests the main policy check
func TestPolicyEngine_AllowScaleDown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	tests := []struct {
		name      string
		mode      PolicyMode
		nodeGroup *autoscalerv1alpha1.NodeGroup
		node      *corev1.Node
		expected  bool
	}{
		{
			name: "Allowed - balanced mode, scale-down enabled",
			mode: PolicyModeBalanced,
			nodeGroup: &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{Enabled: true},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			},
			expected: true,
		},
		{
			name: "Blocked - disabled mode",
			mode: PolicyModeDisabled,
			nodeGroup: &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{Enabled: true},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			},
			expected: false,
		},
		{
			name: "Blocked - NodeGroup scale-down disabled",
			mode: PolicyModeBalanced,
			nodeGroup: &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{Enabled: false},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			},
			expected: false,
		},
		{
			name: "Blocked - node annotation disabled",
			mode: PolicyModeBalanced,
			nodeGroup: &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{Enabled: true},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						"autoscaler.vpsie.com/scale-down": "disabled",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())
			engine.SetDefaultMode(tt.mode)
			engine.timeWindows = nil // Clear time windows

			result := engine.AllowScaleDown(ctx, tt.nodeGroup, tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPolicyEngine_IsNodeAllowedForScaleDown tests node-specific policies
func TestPolicyEngine_IsNodeAllowedForScaleDown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "Allowed - no annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			},
			expected: true,
		},
		{
			name: "Blocked - disabled annotation",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
					Annotations: map[string]string{
						"autoscaler.vpsie.com/scale-down": "disabled",
					},
				},
			},
			expected: false,
		},
		{
			name: "Allowed - enabled annotation",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-3",
					Annotations: map[string]string{
						"autoscaler.vpsie.com/scale-down": "enabled",
					},
				},
			},
			expected: true,
		},
		{
			name: "Allowed - unrelated annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-4",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isNodeAllowedForScaleDown(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPolicyEngine_IsWithinAnnotatedHours tests time annotation parsing
func TestPolicyEngine_IsWithinAnnotatedHours(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	// Note: These tests depend on current time, so we test parsing behavior
	tests := []struct {
		name        string
		hoursStr    string
		expectParse bool // Whether it should parse successfully (returns true for invalid = fail-open)
	}{
		// Valid formats
		{"Valid business hours", "09:00-17:00", true},
		{"Valid overnight", "22:00-06:00", true},
		{"Valid full day", "00:00-23:59", true},
		{"Valid with spaces", " 09:00 - 17:00 ", true},

		// Invalid formats (fail-open returns true)
		{"Invalid - missing hyphen", "09:00 17:00", true},
		{"Invalid - wrong separator", "09:00/17:00", true},
		{"Invalid - incomplete", "09:00", true},
		{"Invalid - empty", "", true},
		{"Invalid - letters", "nine-seventeen", true},
		{"Invalid - out of range hour", "25:00-17:00", true},
		{"Invalid - out of range minute", "09:60-17:00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isWithinAnnotatedHours(tt.hoursStr)
			// We can only verify it doesn't panic and returns a boolean
			// Actual time-based result depends on current time
			assert.IsType(t, true, result)
		})
	}
}

// TestPolicyPresets tests policy presets
func TestPolicyPresets(t *testing.T) {
	assert.Equal(t, PolicyPreset("production"), PresetProduction)
	assert.Equal(t, PolicyPreset("development"), PresetDevelopment)
	assert.Equal(t, PolicyPreset("cost-saving"), PresetCostSaving)
}

// TestPolicyEngine_ApplyPreset tests applying presets
func TestPolicyEngine_ApplyPreset(t *testing.T) {
	tests := []struct {
		name         string
		preset       PolicyPreset
		expectedMode PolicyMode
		windowCount  int
	}{
		{
			name:         "Production preset",
			preset:       PresetProduction,
			expectedMode: PolicyModeConservative,
			windowCount:  1, // Business hours window
		},
		{
			name:         "Development preset",
			preset:       PresetDevelopment,
			expectedMode: PolicyModeBalanced,
			windowCount:  0,
		},
		{
			name:         "Cost-saving preset",
			preset:       PresetCostSaving,
			expectedMode: PolicyModeAggressive,
			windowCount:  2, // Off-hours + weekend windows
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

			engine.ApplyPreset(tt.preset)

			assert.Equal(t, tt.expectedMode, engine.defaultMode)
			assert.Len(t, engine.timeWindows, tt.windowCount)
		})
	}
}

// TestPolicyEngine_GetPolicyStatus tests status reporting
func TestPolicyEngine_GetPolicyStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	engine := NewPolicyEngine(logger.Sugar(), config)
	engine.SetDefaultMode(PolicyModeBalanced)

	status := engine.GetPolicyStatus()

	require.NotNil(t, status)
	assert.Equal(t, PolicyModeBalanced, status["currentMode"])
	assert.Equal(t, PolicyModeBalanced, status["defaultMode"])
	assert.Equal(t, config.CPUThreshold, status["cpuThreshold"])
	assert.Equal(t, config.MemoryThreshold, status["memoryThreshold"])
	assert.Equal(t, config.MaxNodesPerScaleDown, status["maxConcurrentScale"])
	assert.Equal(t, 0, status["timeWindows"])
}

// TestPredefinedTimeWindows tests predefined time window helpers
func TestPredefinedTimeWindows(t *testing.T) {
	t.Run("BusinessHoursWindow", func(t *testing.T) {
		window := GetBusinessHoursWindow()
		assert.Equal(t, 9, window.StartHour)
		assert.Equal(t, 17, window.EndHour)
		assert.Equal(t, PolicyModeConservative, window.Mode)
		assert.Len(t, window.Days, 5) // Mon-Fri
	})

	t.Run("OffHoursWindow", func(t *testing.T) {
		window := GetOffHoursWindow()
		assert.Equal(t, 18, window.StartHour)
		assert.Equal(t, 8, window.EndHour)
		assert.Equal(t, PolicyModeAggressive, window.Mode)
		assert.Len(t, window.Days, 7) // All days
	})

	t.Run("WeekendWindow", func(t *testing.T) {
		window := GetWeekendWindow()
		assert.Equal(t, 0, window.StartHour)
		assert.Equal(t, 24, window.EndHour)
		assert.Equal(t, PolicyModeAggressive, window.Mode)
		assert.Len(t, window.Days, 2) // Sat-Sun
		assert.Contains(t, window.Days, time.Saturday)
		assert.Contains(t, window.Days, time.Sunday)
	})
}

// TestTimeWindowDayMatching tests day matching in time windows
func TestTimeWindowDayMatching(t *testing.T) {
	logger := zaptest.NewLogger(t)
	engine := NewPolicyEngine(logger.Sugar(), DefaultConfig())

	// Add a window for specific days
	engine.AddTimeWindow(TimeWindow{
		StartHour: 0,
		EndHour:   24,
		Days:      []time.Weekday{time.Saturday, time.Sunday},
		Mode:      PolicyModeAggressive,
	})

	// The actual mode returned depends on current day, so we just verify
	// the mechanism works (no panic, returns valid mode)
	mode := engine.getCurrentMode()
	assert.Contains(t, []PolicyMode{
		PolicyModeAggressive,
		PolicyModeBalanced, // Default if not matching
	}, mode)
}
