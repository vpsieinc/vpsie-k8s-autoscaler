package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestCalculatePodResources tests pod resource calculation
func TestCalculatePodResources(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	tests := []struct {
		name           string
		pod            *corev1.Pod
		expectedCPU    string
		expectedMemory string
	}{
		{
			name: "Single container",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "500m",
			expectedMemory: "1Gi",
		},
		{
			name: "Multiple containers",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "750m",
			expectedMemory: "1536Mi",
		},
		{
			name: "Init container with higher requirements",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "1000m",
			expectedMemory: "2Gi",
		},
		{
			name: "Init container with lower requirements",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "1000m",
			expectedMemory: "2Gi",
		},
		{
			name: "No resource requests",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
			},
			expectedCPU:    "0",
			expectedMemory: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.CalculatePodResources(tt.pod)

			expectedCPU := resource.MustParse(tt.expectedCPU)
			expectedMemory := resource.MustParse(tt.expectedMemory)

			assert.True(t, result.CPU.Equal(expectedCPU), "CPU should be %s, got %s", tt.expectedCPU, result.CPU.String())
			assert.True(t, result.Memory.Equal(expectedMemory), "Memory should be %s, got %s", tt.expectedMemory, result.Memory.String())
		})
	}
}

// TestCalculateDeficit tests resource deficit calculation
func TestCalculateDeficit(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		events         []SchedulingEvent
		expectedCPU    string
		expectedMemory string
		expectedPods   int
	}{
		{
			name: "Single event",
			events: []SchedulingEvent{
				{
					Pod:        pod1,
					Constraint: ConstraintCPU,
				},
			},
			expectedCPU:    "500m",
			expectedMemory: "1Gi",
			expectedPods:   1,
		},
		{
			name: "Multiple events from different pods",
			events: []SchedulingEvent{
				{
					Pod:        pod1,
					Constraint: ConstraintCPU,
				},
				{
					Pod:        pod2,
					Constraint: ConstraintMemory,
				},
			},
			expectedCPU:    "1500m",
			expectedMemory: "3Gi",
			expectedPods:   2,
		},
		{
			name: "Duplicate events from same pod",
			events: []SchedulingEvent{
				{
					Pod:        pod1,
					Constraint: ConstraintCPU,
				},
				{
					Pod:        pod1,
					Constraint: ConstraintMemory,
				},
			},
			expectedCPU:    "500m",
			expectedMemory: "1Gi",
			expectedPods:   1,
		},
		{
			name:           "Empty events",
			events:         []SchedulingEvent{},
			expectedCPU:    "0",
			expectedMemory: "0",
			expectedPods:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.CalculateDeficit(tt.events)

			expectedCPU := resource.MustParse(tt.expectedCPU)
			expectedMemory := resource.MustParse(tt.expectedMemory)

			assert.True(t, result.CPU.Equal(expectedCPU), "CPU should be %s, got %s", tt.expectedCPU, result.CPU.String())
			assert.True(t, result.Memory.Equal(expectedMemory), "Memory should be %s, got %s", tt.expectedMemory, result.Memory.String())
			assert.Equal(t, tt.expectedPods, result.Pods)
		})
	}
}

// TestPodMatchesNodeGroup tests pod to NodeGroup matching
func TestPodMatchesNodeGroup(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	tests := []struct {
		name      string
		pod       *corev1.Pod
		nodeGroup *v1alpha1.NodeGroup
		matches   bool
	}{
		{
			name: "Pod matches NodeGroup labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"env": "production",
					},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Labels: map[string]string{
						"env": "production",
					},
				},
			},
			matches: true,
		},
		{
			name: "Pod does not match NodeGroup labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"env": "staging",
					},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Labels: map[string]string{
						"env": "production",
					},
				},
			},
			matches: false,
		},
		{
			name: "Pod has no node selector, NodeGroup has labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Labels: map[string]string{
						"env": "production",
					},
				},
			},
			matches: false,
		},
		{
			name: "Pod has node selector, NodeGroup has no labels",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"env": "production",
					},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Labels: map[string]string{},
				},
			},
			matches: false, // Pod requires labels, NodeGroup has none - no match
		},
		{
			name: "Pod tolerates NodeGroup taints",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "gpu",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Taints: []corev1.Taint{
						{
							Key:    "dedicated",
							Value:  "gpu",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			matches: true,
		},
		{
			name: "Pod does not tolerate NodeGroup taints",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{},
				},
			},
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					Taints: []corev1.Taint{
						{
							Key:    "dedicated",
							Value:  "gpu",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.podMatchesNodeGroup(tt.pod, tt.nodeGroup)
			assert.Equal(t, tt.matches, result)
		})
	}
}

// TestFindMatchingNodeGroups tests NodeGroup matching logic
func TestFindMatchingNodeGroups(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	pod1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"env": "production",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	pod2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"env": "staging",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
	}

	ng1 := v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ng-production",
			Labels: map[string]string{
				v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
			},
		},
		Spec: v1alpha1.NodeGroupSpec{
			Labels: map[string]string{
				"env": "production",
			},
			MaxNodes: 10,
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 2,
		},
	}

	ng2 := v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ng-staging",
			Labels: map[string]string{
				v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
			},
		},
		Spec: v1alpha1.NodeGroupSpec{
			Labels: map[string]string{
				"env": "staging",
			},
			MaxNodes: 5,
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 3,
		},
	}

	ng3 := v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ng-no-labels",
			Labels: map[string]string{
				v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
			},
		},
		Spec: v1alpha1.NodeGroupSpec{
			Labels:   map[string]string{},
			MaxNodes: 3,
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 1,
		},
	}

	tests := []struct {
		name              string
		pendingPods       []corev1.Pod
		nodeGroups        []v1alpha1.NodeGroup
		expectedMatches   int
		expectedNodeGroup string
	}{
		{
			name:              "Single pod matches single NodeGroup",
			pendingPods:       []corev1.Pod{pod1},
			nodeGroups:        []v1alpha1.NodeGroup{ng1},
			expectedMatches:   1,
			expectedNodeGroup: "ng-production",
		},
		{
			name:            "Multiple pods match multiple NodeGroups",
			pendingPods:     []corev1.Pod{pod1, pod2},
			nodeGroups:      []v1alpha1.NodeGroup{ng1, ng2},
			expectedMatches: 2,
		},
		{
			name:            "Pod does not match any NodeGroup",
			pendingPods:     []corev1.Pod{pod1},
			nodeGroups:      []v1alpha1.NodeGroup{ng2},
			expectedMatches: 0,
		},
		{
			name:            "NodeGroup with no labels matches no pods",
			pendingPods:     []corev1.Pod{pod1},
			nodeGroups:      []v1alpha1.NodeGroup{ng3},
			expectedMatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := analyzer.FindMatchingNodeGroups(tt.pendingPods, tt.nodeGroups)
			assert.Len(t, matches, tt.expectedMatches)

			if tt.expectedNodeGroup != "" {
				assert.Equal(t, tt.expectedNodeGroup, matches[0].NodeGroup.Name)
			}
		})
	}
}

// TestCalculateMatchScore tests NodeGroup match scoring
func TestCalculateMatchScore(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}}
	pod2 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}}

	deficit := ResourceDeficit{
		CPU:    resource.MustParse("1000m"),
		Memory: resource.MustParse("2Gi"),
		Pods:   2,
	}

	tests := []struct {
		name         string
		nodeGroup    *v1alpha1.NodeGroup
		matchingPods []*corev1.Pod
		expectedMin  int
	}{
		{
			name: "NodeGroup with capacity and preferred instance type",
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes:              10,
					PreferredInstanceType: "offering-1",
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 2,
				},
			},
			matchingPods: []*corev1.Pod{pod1, pod2},
			expectedMin:  200 + 100 + 400 + 100, // not at max + preferred + capacity*50 + pods*100
		},
		{
			name: "NodeGroup at max capacity",
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 5,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 5,
				},
			},
			matchingPods: []*corev1.Pod{pod1},
			expectedMin:  100, // only pod score
		},
		{
			name: "NodeGroup with no preferred instance type",
			nodeGroup: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes:              10,
					PreferredInstanceType: "",
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 5,
				},
			},
			matchingPods: []*corev1.Pod{pod1},
			expectedMin:  200 + 250 + 100, // not at max + capacity*50 + pods*100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := analyzer.calculateMatchScore(tt.nodeGroup, tt.matchingPods, deficit)
			assert.GreaterOrEqual(t, score, tt.expectedMin)
		})
	}
}

// TestEstimateNodesNeeded tests node count estimation
func TestEstimateNodesNeeded(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	instanceType := v1alpha1.InstanceTypeInfo{
		OfferingID: "offering-1",
		CPU:        4,
		MemoryMB:   8192,
		DiskGB:     80,
	}

	tests := []struct {
		name        string
		deficit     ResourceDeficit
		expectedMin int
		expectedMax int
	}{
		{
			name: "CPU-bound",
			deficit: ResourceDeficit{
				CPU:    resource.MustParse("8000m"), // 8 cores
				Memory: resource.MustParse("4Gi"),
				Pods:   2,
			},
			expectedMin: 2, // 8 cores / 4 cores per node
			expectedMax: 2,
		},
		{
			name: "Memory-bound",
			deficit: ResourceDeficit{
				CPU:    resource.MustParse("2000m"), // 2 cores
				Memory: resource.MustParse("16Gi"),  // 16 GB
				Pods:   2,
			},
			expectedMin: 2, // 16 GB / 8 GB per node
			expectedMax: 2,
		},
		{
			name: "Pod-count-bound",
			deficit: ResourceDeficit{
				CPU:    resource.MustParse("1000m"),
				Memory: resource.MustParse("2Gi"),
				Pods:   200, // Many pods
			},
			expectedMin: 2, // 200 pods / 110 max pods per node
			expectedMax: 2,
		},
		{
			name: "Small deficit",
			deficit: ResourceDeficit{
				CPU:    resource.MustParse("100m"),
				Memory: resource.MustParse("128Mi"),
				Pods:   1,
			},
			expectedMin: 1, // Always at least 1 node
			expectedMax: 1,
		},
		{
			name: "Zero deficit",
			deficit: ResourceDeficit{
				CPU:    resource.MustParse("0"),
				Memory: resource.MustParse("0"),
				Pods:   0,
			},
			expectedMin: 0,
			expectedMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.EstimateNodesNeeded(tt.deficit, instanceType)
			assert.GreaterOrEqual(t, result, tt.expectedMin)
			assert.LessOrEqual(t, result, tt.expectedMax)
		})
	}
}

// TestSelectInstanceType tests instance type selection
func TestSelectInstanceType(t *testing.T) {
	analyzer := NewResourceAnalyzer(zap.NewNop(), nil)

	deficit := ResourceDeficit{
		CPU:    resource.MustParse("2000m"),
		Memory: resource.MustParse("4Gi"),
		Pods:   2,
	}

	tests := []struct {
		name         string
		nodeGroup    *v1alpha1.NodeGroup
		expectedType string
		expectError  bool
	}{
		{
			name: "Use preferred instance type",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ng-1",
				},
				Spec: v1alpha1.NodeGroupSpec{
					OfferingIDs:           []string{"offering-1", "offering-2"},
					PreferredInstanceType: "offering-2",
				},
			},
			expectedType: "offering-2",
			expectError:  false,
		},
		{
			name: "Preferred type not in offerings, use first",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ng-1",
				},
				Spec: v1alpha1.NodeGroupSpec{
					OfferingIDs:           []string{"offering-1", "offering-2"},
					PreferredInstanceType: "offering-3",
				},
			},
			expectedType: "offering-1",
			expectError:  false,
		},
		{
			name: "No preferred type, use first offering",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ng-1",
				},
				Spec: v1alpha1.NodeGroupSpec{
					OfferingIDs:           []string{"offering-1", "offering-2"},
					PreferredInstanceType: "",
				},
			},
			expectedType: "offering-1",
			expectError:  false,
		},
		{
			name: "No offerings available",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ng-1",
				},
				Spec: v1alpha1.NodeGroupSpec{
					OfferingIDs:           []string{},
					PreferredInstanceType: "",
				},
			},
			expectedType: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.SelectInstanceType(tt.nodeGroup, deficit)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedType, result)
			}
		})
	}
}
