package scaler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// TestIsSingleInstanceSystemPod tests the isSingleInstanceSystemPod function
func TestIsSingleInstanceSystemPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "kube-apiserver pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-apiserver-master",
					Namespace: "kube-system",
				},
			},
			expected: true,
		},
		{
			name: "etcd pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-master-1",
					Namespace: "kube-system",
				},
			},
			expected: true,
		},
		{
			name: "kube-controller-manager pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-controller-manager-master",
					Namespace: "kube-system",
				},
			},
			expected: true,
		},
		{
			name: "kube-scheduler pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-scheduler-master",
					Namespace: "kube-system",
				},
			},
			expected: true,
		},
		{
			name: "Regular system pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns-abc123",
					Namespace: "kube-system",
				},
			},
			expected: false,
		},
		{
			name: "Application pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-abc123",
					Namespace: "default",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSingleInstanceSystemPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsNodeReady tests the isNodeReady helper function
func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "Ready node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not ready node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-2"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Unknown status node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-3"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Node without ready condition",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-4"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Node with no conditions",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-5"},
				Status:     corev1.NodeStatus{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNodeReady(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasPersistentVolumes tests the HasPersistentVolumes function
func TestHasPersistentVolumes(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Pod with PVC",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "mysql-0"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mysql-data",
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Pod without PVC",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "nginx-config",
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Pod with no volumes",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "simple-pod"},
				Spec:       corev1.PodSpec{},
			},
			expected: false,
		},
		{
			name: "Pod with mixed volumes including PVC",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed-pod"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "app-config",
									},
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "app-data",
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasPersistentVolumes(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsPodControlledBy tests the IsPodControlledBy function
func TestIsPodControlledBy(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		kind     string
		expected bool
	}{
		{
			name: "ReplicaSet controlled pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nginx-abc123",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "nginx-rs"},
					},
				},
			},
			kind:     "ReplicaSet",
			expected: true,
		},
		{
			name: "StatefulSet controlled pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mysql-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "mysql"},
					},
				},
			},
			kind:     "StatefulSet",
			expected: true,
		},
		{
			name: "DaemonSet controlled pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fluentd-abc",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "DaemonSet", Name: "fluentd"},
					},
				},
			},
			kind:     "DaemonSet",
			expected: true,
		},
		{
			name: "Job controlled pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "backup-job-abc",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "Job", Name: "backup-job"},
					},
				},
			},
			kind:     "Job",
			expected: true,
		},
		{
			name: "Wrong owner type",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nginx-abc",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "nginx-rs"},
					},
				},
			},
			kind:     "DaemonSet",
			expected: false,
		},
		{
			name: "No owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standalone-pod",
				},
			},
			kind:     "ReplicaSet",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPodControlledBy(tt.pod, tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasNodeSelector tests the HasNodeSelector function
func TestHasNodeSelector(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Pod with node selector",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"gpu": "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "Pod without node selector",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "simple-pod"},
				Spec:       corev1.PodSpec{},
			},
			expected: false,
		},
		{
			name: "Pod with empty node selector",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-selector-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNodeSelector(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasNodeAffinity tests the HasNodeAffinity function
func TestHasNodeAffinity(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Pod with node affinity",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "zone-pod"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "topology.kubernetes.io/zone",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"us-east-1a"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Pod without affinity",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "simple-pod"},
				Spec:       corev1.PodSpec{},
			},
			expected: false,
		},
		{
			name: "Pod with pod affinity only",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-affinity"},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNodeAffinity(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetPodPriority tests the GetPodPriority function
func TestGetPodPriority(t *testing.T) {
	priority100 := int32(100)
	priority1000 := int32(1000)
	negativePriority := int32(-100)

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected int32
	}{
		{
			name: "Pod with priority 100",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "low-priority"},
				Spec: corev1.PodSpec{
					Priority: &priority100,
				},
			},
			expected: 100,
		},
		{
			name: "Pod with priority 1000",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "high-priority"},
				Spec: corev1.PodSpec{
					Priority: &priority1000,
				},
			},
			expected: 1000,
		},
		{
			name: "Pod with negative priority",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "negative-priority"},
				Spec: corev1.PodSpec{
					Priority: &negativePriority,
				},
			},
			expected: -100,
		},
		{
			name: "Pod without priority",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "no-priority"},
				Spec:       corev1.PodSpec{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPodPriority(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsSystemCriticalPod tests the IsSystemCriticalPod function
func TestIsSystemCriticalPod(t *testing.T) {
	systemCriticalPriority := int32(2000000000)
	normalPriority := int32(100)

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "system-cluster-critical priority class",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "critical-pod"},
				Spec: corev1.PodSpec{
					PriorityClassName: "system-cluster-critical",
				},
			},
			expected: true,
		},
		{
			name: "system-node-critical priority class",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "node-critical-pod"},
				Spec: corev1.PodSpec{
					PriorityClassName: "system-node-critical",
				},
			},
			expected: true,
		},
		{
			name: "High priority value",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "high-priority-pod"},
				Spec: corev1.PodSpec{
					Priority: &systemCriticalPriority,
				},
			},
			expected: true,
		},
		{
			name: "Normal priority",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "normal-pod"},
				Spec: corev1.PodSpec{
					Priority: &normalPriority,
				},
			},
			expected: false,
		},
		{
			name: "No priority class",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "default-pod"},
				Spec:       corev1.PodSpec{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSystemCriticalPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMatchesNodeSelector tests the MatchesNodeSelector function
func TestMatchesNodeSelector(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Matching node selector",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu-node",
					Labels: map[string]string{
						"gpu":  "nvidia",
						"zone": "us-east-1a",
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"gpu": "nvidia",
					},
				},
			},
			expected: true,
		},
		{
			name: "Non-matching node selector",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cpu-node",
					Labels: map[string]string{
						"type": "cpu",
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"gpu": "nvidia",
					},
				},
			},
			expected: false,
		},
		{
			name: "No node selector",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "any-node",
					Labels: map[string]string{},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "flexible-pod"},
				Spec:       corev1.PodSpec{},
			},
			expected: true,
		},
		{
			name: "Multiple selectors all match",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multi-label-node",
					Labels: map[string]string{
						"env":  "production",
						"tier": "frontend",
						"zone": "us-west-1",
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "specific-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"env":  "production",
						"tier": "frontend",
					},
				},
			},
			expected: true,
		},
		{
			name: "Multiple selectors partial match",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "partial-node",
					Labels: map[string]string{
						"env": "production",
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "specific-pod"},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"env":  "production",
						"tier": "frontend",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesNodeSelector(tt.node, tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasPodsWithLocalStorage tests the hasPodsWithLocalStorage method
func TestHasPodsWithLocalStorage(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name        string
		pods        []*corev1.Pod
		objects     []runtime.Object
		expectLocal bool
	}{
		{
			name: "Pod with EmptyDir",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cache-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "cache",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			},
			expectLocal: true,
		},
		{
			name: "Pod with Memory EmptyDir (allowed)",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "memory-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "memory-cache",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{
										Medium: corev1.StorageMediumMemory,
									},
								},
							},
						},
					},
				},
			},
			expectLocal: false,
		},
		{
			name: "Pod with HostPath",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "hostpath-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "host-data",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/data",
									},
								},
							},
						},
					},
				},
			},
			expectLocal: true,
		},
		{
			name: "Pod with ConfigMap only",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "app-config",
										},
									},
								},
							},
						},
					},
				},
			},
			expectLocal: false,
		},
		{
			name:        "No pods",
			pods:        []*corev1.Pod{},
			expectLocal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := tt.objects
			if objects == nil {
				objects = []runtime.Object{}
			}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
			}

			hasLocal, _ := manager.hasPodsWithLocalStorage(ctx, tt.pods)
			assert.Equal(t, tt.expectLocal, hasLocal)
		})
	}
}

// TestHasUniqueSystemPods tests the hasUniqueSystemPods method
func TestHasUniqueSystemPods(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := &ScaleDownManager{
		logger: logger.Sugar(),
	}

	tests := []struct {
		name       string
		pods       []*corev1.Pod
		expectUniq bool
	}{
		{
			name: "Has kube-apiserver",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-apiserver-master",
						Namespace: "kube-system",
					},
				},
			},
			expectUniq: true,
		},
		{
			name: "Has etcd",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd-master",
						Namespace: "kube-system",
					},
				},
			},
			expectUniq: true,
		},
		{
			name: "Only regular pods in kube-system",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-abc123",
						Namespace: "kube-system",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-proxy-xyz",
						Namespace: "kube-system",
					},
				},
			},
			expectUniq: false,
		},
		{
			name: "No system pods",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-pod",
						Namespace: "default",
					},
				},
			},
			expectUniq: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasUnique, _ := manager.hasUniqueSystemPods(tt.pods)
			assert.Equal(t, tt.expectUniq, hasUnique)
		})
	}
}

// TestIsSafeToRemove tests the IsSafeToRemove method
func TestIsSafeToRemove(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create nodes for capacity calculation
	readyNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup": "test-group",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	anotherNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-2",
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup": "test-group",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	tests := []struct {
		name        string
		node        *corev1.Node
		pods        []*corev1.Pod
		objects     []runtime.Object
		expectSafe  bool
		expectError bool
	}{
		{
			name: "Safe - no special pods",
			node: readyNode,
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "simple-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							},
						},
					},
				},
			},
			objects:    []runtime.Object{readyNode, anotherNode},
			expectSafe: true,
		},
		{
			name: "Unsafe - has local storage",
			node: readyNode,
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "local-storage-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "data",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/data",
									},
								},
							},
						},
					},
				},
			},
			objects:    []runtime.Object{readyNode, anotherNode},
			expectSafe: false,
		},
		{
			name: "Unsafe - protected node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "protected-node",
					Labels: map[string]string{
						"autoscaler.vpsie.com/nodegroup": "test-group",
					},
					Annotations: map[string]string{
						ProtectedNodeAnnotation: "true",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
			pods:       []*corev1.Pod{},
			objects:    []runtime.Object{readyNode, anotherNode},
			expectSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := tt.objects
			if objects == nil {
				objects = []runtime.Object{}
			}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
				config: DefaultConfig(),
			}

			safe, _, err := manager.IsSafeToRemove(ctx, tt.node, tt.pods)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectSafe, safe)
			}
		})
	}
}

// =============================================================================
// Enhanced Scale-Down Safety Tests - Design Doc: enhanced-scale-down-safety-design.md
// Generated: 2026-01-11 | Budget Used: 6 unit tests for safety.go
// =============================================================================

// TestTolerationMatching tests the toleration matching algorithm for scale-down safety.
// AC1: "Pods with tolerations for specific taints can only be scaled down if remaining nodes have matching taints"
func TestTolerationMatching(t *testing.T) {
	// AC1: Toleration Matching - BLOCKED scenario
	// ROI: 78 | Business Value: 9 (prevents GPU workload disruption) | Frequency: 7 (common)
	// Behavior: Pod tolerates specific taint → No remaining node has taint → Scale-down BLOCKED
	// @category: core-functionality
	// @dependency: tolerationsTolerateTaints, tolerationMatchesTaint functions
	// @complexity: medium
	t.Run("AC1: Scale-down blocked - pod tolerates taint but no remaining node has it", func(t *testing.T) {
		// Arrange: Create pod with toleration for "gpu=true:NoSchedule"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-workload",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:      "gpu",
						Value:    "true",
						Effect:   corev1.TaintEffectNoSchedule,
						Operator: corev1.TolerationOpEqual,
					},
				},
			},
		}

		// Create node with taint "gpu=true:NoSchedule" (to be removed)
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-node-1",
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "gpu",
						Value:  "true",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		// Create remaining node WITHOUT the gpu taint but WITH a different NoSchedule taint
		// The pod does NOT tolerate this taint, so it cannot be scheduled there
		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-no-gpu",
			},
			Spec: corev1.NodeSpec{
				// This node has a different taint that the pod doesn't tolerate
				Taints: []corev1.Taint{
					{
						Key:    "special",
						Value:  "reserved",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		// Act: Check if pod can be scheduled on remaining node
		// tolerationsTolerateTaints returns true if tolerations cover all NoSchedule/NoExecute taints
		result := tolerationsTolerateTaints(pod.Spec.Tolerations, remainingNode.Spec.Taints)

		// Assert: Pod should NOT be able to schedule on remaining node because:
		// - The remaining node has a "special=reserved:NoSchedule" taint
		// - The pod only tolerates "gpu=true:NoSchedule"
		// - The pod's tolerations do NOT cover the remaining node's taints
		// - Therefore scale-down should be BLOCKED
		_ = nodeToRemove

		assert.False(t, result, "Pod's tolerations should NOT cover remaining node's taints")
	})

	// AC1: Toleration Matching - ALLOWED scenario
	// ROI: 78 | Business Value: 9 | Frequency: 7
	// Behavior: Pod tolerates taint → Remaining node has same taint → Scale-down ALLOWED
	// @category: core-functionality
	// @dependency: tolerationsTolerateTaints function
	// @complexity: medium
	t.Run("AC1: Scale-down allowed - remaining node has matching taint", func(t *testing.T) {
		// Arrange: Create pod with toleration for "gpu=true:NoSchedule"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-workload",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:      "gpu",
						Value:    "true",
						Effect:   corev1.TaintEffectNoSchedule,
						Operator: corev1.TolerationOpEqual,
					},
				},
			},
		}

		// Create node with taint "gpu=true:NoSchedule" (to be removed)
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-node-1",
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "gpu",
						Value:  "true",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		// Create remaining node WITH the same "gpu=true:NoSchedule" taint
		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-node-2",
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "gpu",
						Value:  "true",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		// Act: Check if pod can be scheduled on remaining node
		// tolerationsTolerateTaints returns true if tolerations cover all NoSchedule/NoExecute taints
		result := tolerationsTolerateTaints(pod.Spec.Tolerations, remainingNode.Spec.Taints)

		// Assert: Pod SHOULD be able to schedule on remaining node because:
		// - Pod tolerates gpu=true:NoSchedule
		// - Remaining node has gpu=true:NoSchedule taint
		// - Pod's toleration matches the taint, so scheduling is allowed
		_ = nodeToRemove

		assert.True(t, result, "Pod's tolerations should cover remaining node's taints")
	})

	// AC1: Wildcard toleration - ALLOWED scenario
	// ROI: 45 | Business Value: 6 | Frequency: 3
	// Behavior: Pod has wildcard toleration (empty key + Exists) → Matches any taint → Scale-down ALLOWED
	// @category: edge-case
	// @dependency: tolerationMatches function
	// @complexity: low
	t.Run("AC1: Wildcard toleration matches any taint", func(t *testing.T) {
		// Arrange: Create pod with wildcard toleration (tolerates all taints)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tolerant-workload",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						// Wildcard toleration: empty key + Exists operator matches ANY taint
						Key:      "",
						Operator: corev1.TolerationOpExists,
						Effect:   "", // Empty effect matches all effects
					},
				},
			},
		}

		// Create node with any arbitrary taint
		nodeWithTaint := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "special-node",
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "special",
						Value:  "value",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		// Get the wildcard toleration and the taint for direct matching test
		wildcardToleration := pod.Spec.Tolerations[0]
		taint := nodeWithTaint.Spec.Taints[0]

		// Act: Check if wildcard toleration matches the taint
		result := tolerationMatches(&wildcardToleration, &taint)

		// Assert: Wildcard toleration SHOULD match any taint because:
		// - Empty Key + Exists operator matches all keys
		// - Empty Effect matches all effects (NoSchedule, NoExecute, PreferNoSchedule)
		_ = pod
		_ = nodeWithTaint

		// Verification items:
		// - tolerationMatches(&wildcardToleration, &taint) == true
		// - Pod can be scheduled on any node regardless of taints
		// - Scale-down should be ALLOWED for pods with wildcard toleration

		assert.True(t, result, "Wildcard toleration should match any taint")
	})

	// Test tolerationMatchesTaint with multiple tolerations
	t.Run("AC1: tolerationMatchesTaint finds matching toleration in list", func(t *testing.T) {
		// Arrange: Create a list of tolerations including the matching one
		tolerations := []corev1.Toleration{
			{
				Key:      "other",
				Value:    "value",
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpEqual,
			},
			{
				Key:      "gpu",
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpEqual,
			},
		}

		taint := &corev1.Taint{
			Key:    "gpu",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		}

		// Act: Check if any toleration in the list matches the taint
		result := tolerationMatchesTaint(tolerations, taint)

		// Assert: Should find the matching toleration
		assert.True(t, result, "Should find matching toleration for gpu=true:NoSchedule")
	})

	// Test tolerationMatchesTaint with no matching toleration
	t.Run("AC1: tolerationMatchesTaint returns false when no match", func(t *testing.T) {
		// Arrange: Create tolerations that don't match the taint
		tolerations := []corev1.Toleration{
			{
				Key:      "other",
				Value:    "value",
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpEqual,
			},
		}

		taint := &corev1.Taint{
			Key:    "gpu",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		}

		// Act: Check if any toleration matches
		result := tolerationMatchesTaint(tolerations, taint)

		// Assert: Should not find a match
		assert.False(t, result, "Should not find matching toleration for gpu=true:NoSchedule")
	})

	// Test tolerationMatches with Exists operator
	t.Run("AC1: Exists operator matches any value for the same key", func(t *testing.T) {
		// Arrange: Create toleration with Exists operator (matches any value)
		toleration := &corev1.Toleration{
			Key:      "gpu",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		}

		taint := &corev1.Taint{
			Key:    "gpu",
			Value:  "any-value",
			Effect: corev1.TaintEffectNoSchedule,
		}

		// Act: Check if toleration matches
		result := tolerationMatches(toleration, taint)

		// Assert: Exists operator should match any value
		assert.True(t, result, "Exists operator should match any value for the same key")
	})

	// Test tolerationMatches with Equal operator and different values
	t.Run("AC1: Equal operator requires exact value match", func(t *testing.T) {
		// Arrange: Create toleration with Equal operator
		toleration := &corev1.Toleration{
			Key:      "gpu",
			Value:    "v100",
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
		}

		taint := &corev1.Taint{
			Key:    "gpu",
			Value:  "a100", // Different value
			Effect: corev1.TaintEffectNoSchedule,
		}

		// Act: Check if toleration matches
		result := tolerationMatches(toleration, taint)

		// Assert: Equal operator should require exact value match
		assert.False(t, result, "Equal operator should require exact value match")
	})

	// Test tolerationMatches with empty toleration effect
	t.Run("AC1: Empty toleration effect matches all effects", func(t *testing.T) {
		// Arrange: Create toleration with empty effect (matches all effects)
		toleration := &corev1.Toleration{
			Key:      "gpu",
			Value:    "true",
			Operator: corev1.TolerationOpEqual,
			Effect:   "", // Empty effect matches all
		}

		// Test against different effects
		noScheduleTaint := &corev1.Taint{
			Key:    "gpu",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		}
		noExecuteTaint := &corev1.Taint{
			Key:    "gpu",
			Value:  "true",
			Effect: corev1.TaintEffectNoExecute,
		}

		// Act & Assert
		assert.True(t, tolerationMatches(toleration, noScheduleTaint), "Empty effect should match NoSchedule")
		assert.True(t, tolerationMatches(toleration, noExecuteTaint), "Empty effect should match NoExecute")
	})
}

// TestNodeSelectorInCanPodsBeRescheduled tests nodeSelector verification in scale-down safety.
// AC2: "Pods with nodeSelector can only be scaled down if remaining nodes have matching labels"
func TestNodeSelectorInCanPodsBeRescheduled(t *testing.T) {
	// AC2: NodeSelector Matching - BLOCKED scenario
	// ROI: 68 | Business Value: 8 (prevents zone/disktype workload disruption) | Frequency: 6
	// Behavior: Pod requires label → No remaining node has label → Scale-down BLOCKED
	// @category: core-functionality
	// @dependency: MatchesNodeSelector (existing), findSchedulableNode (new)
	// @complexity: medium
	t.Run("AC2: Scale-down blocked - no remaining node has required label", func(t *testing.T) {
		ctx := context.Background()
		logger := zaptest.NewLogger(t)

		// Arrange:
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ssd-node-to-remove",
				Labels: map[string]string{
					"disktype":                       "ssd",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hdd-node",
				Labels: map[string]string{
					"disktype":                       "hdd", // DOES NOT match required "ssd"
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ssd-app",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"disktype": "ssd",
				},
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Verify nodeSelector does not match remaining node
		nodeSelectorMatches := MatchesNodeSelector(remainingNode, pod)
		assert.False(t, nodeSelectorMatches, "Remaining node should NOT match pod's nodeSelector")

		// Act:
		// canSchedule, reason, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{pod})

		// Assert:
		// require.NoError(t, err)
		// assert.False(t, canSchedule, "Scale-down should be blocked - no SSD node available")
		// assert.Contains(t, reason, "ssd-app", "Reason should contain pod name")

		// Suppress unused variable warnings
		_ = ctx
		_ = manager

		t.Skip("Skeleton: Implementation required - findSchedulableNode function")
	})

	// AC2: NodeSelector Matching - ALLOWED scenario
	// ROI: 68 | Business Value: 8 | Frequency: 6
	// Behavior: Pod requires label → Remaining node has label → Scale-down ALLOWED
	// @category: core-functionality
	// @dependency: MatchesNodeSelector (existing), findSchedulableNode (new)
	// @complexity: medium
	t.Run("AC2: Scale-down allowed - remaining node has required label", func(t *testing.T) {
		ctx := context.Background()
		logger := zaptest.NewLogger(t)

		// Arrange:
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ssd-node-to-remove",
				Labels: map[string]string{
					"disktype":                       "ssd",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ssd-node-2",
				Labels: map[string]string{
					"disktype":                       "ssd", // MATCHES required "ssd"
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ssd-app",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"disktype": "ssd",
				},
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Verify nodeSelector matches remaining node
		nodeSelectorMatches := MatchesNodeSelector(remainingNode, pod)
		assert.True(t, nodeSelectorMatches, "Remaining node SHOULD match pod's nodeSelector")

		// Act:
		// canSchedule, reason, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{pod})

		// Assert:
		// require.NoError(t, err)
		// assert.True(t, canSchedule, "Scale-down should be allowed - SSD node available")

		// Suppress unused variable warnings
		_ = ctx
		_ = manager

		t.Skip("Skeleton: Implementation required - findSchedulableNode function")
	})
}

// TestAntiAffinityVerification tests pod anti-affinity constraint checking.
// AC3: "Pods with anti-affinity rules are checked for topology spread after removal"
func TestAntiAffinityVerification(t *testing.T) {
	// AC3: Anti-Affinity - BLOCKED scenario
	// ROI: 56 | Business Value: 7 (prevents HA violation) | Frequency: 5
	// Behavior: Pod has anti-affinity → Moving would violate constraint → Scale-down BLOCKED
	// @category: core-functionality
	// @dependency: hasPodAntiAffinityViolation (new), findSchedulableNode (new)
	// @complexity: high
	t.Run("AC3: Scale-down blocked - would violate pod anti-affinity", func(t *testing.T) {
		ctx := context.Background()
		logger := zaptest.NewLogger(t)

		// Arrange:
		// Pod to be moved has anti-affinity: cannot colocate with other "app=web" pods
		podToMove := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-replica-1",
				Namespace: "default",
				Labels: map[string]string{
					"app": "web",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "worker-to-remove",
				Affinity: &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "web",
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name: "web",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		// Node to be removed
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-to-remove",
				Labels: map[string]string{
					"kubernetes.io/hostname":         "worker-to-remove",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		// Remaining node already has another web replica - anti-affinity would be violated
		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"kubernetes.io/hostname":         "worker-1",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		// Existing pod on remaining node with label "app=web" - violates anti-affinity
		existingPodOnRemainingNode := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-replica-2",
				Namespace: "default",
				Labels: map[string]string{
					"app": "web", // Matches anti-affinity selector
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "worker-1",
				Containers: []corev1.Container{
					{
						Name: "web",
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode, podToMove, existingPodOnRemainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Act:
		// violation := hasPodAntiAffinityViolation(podToMove, remainingNode, []*corev1.Pod{existingPodOnRemainingNode})

		// Assert:
		// assert.True(t, violation, "Should detect anti-affinity violation")
		// canSchedule, reason, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{podToMove})
		// require.NoError(t, err)
		// assert.False(t, canSchedule, "Scale-down should be blocked - anti-affinity violation")
		// assert.Contains(t, reason, "anti-affinity", "Reason should mention anti-affinity")

		// Suppress unused variable warnings
		_ = ctx
		_ = manager
		_ = existingPodOnRemainingNode
		_ = remainingNode

		t.Skip("Skeleton: Implementation required - hasPodAntiAffinityViolation function")
	})

	// AC3: Anti-Affinity - ALLOWED scenario (no violation)
	// ROI: 56 | Business Value: 7 | Frequency: 5
	// Behavior: Pod has anti-affinity → Moving would NOT violate → Scale-down ALLOWED
	// @category: core-functionality
	// @dependency: hasPodAntiAffinityViolation (new)
	// @complexity: high
	t.Run("AC3: Scale-down allowed - anti-affinity not violated", func(t *testing.T) {
		ctx := context.Background()
		logger := zaptest.NewLogger(t)

		// Arrange:
		// Pod to be moved has anti-affinity: cannot colocate with other "app=web" pods
		podToMove := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-replica-1",
				Namespace: "default",
				Labels: map[string]string{
					"app": "web",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "worker-to-remove",
				Affinity: &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "web",
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name: "web",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		// Node to be removed
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-to-remove",
				Labels: map[string]string{
					"kubernetes.io/hostname":         "worker-to-remove",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		// Remaining node does NOT have any "app=web" pod - no anti-affinity violation
		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"kubernetes.io/hostname":         "worker-1",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		// Existing pod on remaining node with DIFFERENT label - no anti-affinity violation
		existingPodOnRemainingNode := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "db-pod",
				Namespace: "default",
				Labels: map[string]string{
					"app": "database", // Different label - does NOT match anti-affinity selector
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "worker-1",
				Containers: []corev1.Container{
					{
						Name: "db",
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode, podToMove, existingPodOnRemainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Act:
		// violation := hasPodAntiAffinityViolation(podToMove, remainingNode, []*corev1.Pod{existingPodOnRemainingNode})

		// Assert:
		// assert.False(t, violation, "Should NOT detect anti-affinity violation")
		// canSchedule, _, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{podToMove})
		// require.NoError(t, err)
		// assert.True(t, canSchedule, "Scale-down should be allowed - no anti-affinity violation")

		// Suppress unused variable warnings
		_ = ctx
		_ = manager
		_ = existingPodOnRemainingNode
		_ = remainingNode

		t.Skip("Skeleton: Implementation required - hasPodAntiAffinityViolation function")
	})
}

// TestClearBlockingMessages tests that scale-down blocking messages are informative.
// AC4: "Scale-down is blocked with clear log message when pods cannot be rescheduled"
func TestClearBlockingMessages(t *testing.T) {
	// AC4: Clear Blocking Messages
	// ROI: 54 | Business Value: 6 (debugging/operations) | Frequency: 8
	// Behavior: Scale-down blocked → Reason message includes pod name, constraint type, specific constraint
	// @category: ux
	// @dependency: canPodsBeRescheduled, findSchedulableNode
	// @complexity: low
	t.Run("AC4: Blocking message includes pod name and constraint type", func(t *testing.T) {
		ctx := context.Background()
		logger := zaptest.NewLogger(t)

		// Arrange:
		// Pod with nodeSelector that won't match any remaining node
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-abc123",
				Namespace: "myapp",
			},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"zone": "us-east-1",
				},
				Containers: []corev1.Container{
					{
						Name: "web",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		// Node to be removed - has the required label
		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "us-east-1-node",
				Labels: map[string]string{
					"zone":                           "us-east-1",
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		// Remaining node - does NOT have the required label
		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "us-west-1-node",
				Labels: map[string]string{
					"zone":                           "us-west-1", // Wrong zone
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Act:
		// canSchedule, reason, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{pod})

		// Assert:
		// require.NoError(t, err)
		// assert.False(t, canSchedule, "Scale-down should be blocked")
		// assert.Contains(t, reason, "myapp/web-abc123", "Reason should contain pod namespace/name")
		// // The reason should indicate what constraint failed (e.g., "no suitable node", "nodeSelector", etc.)
		// assert.True(t, strings.Contains(reason, "no suitable node") || strings.Contains(reason, "nodeSelector"),
		//     "Reason should describe the constraint that failed")

		// Suppress unused variable warnings
		_ = ctx
		_ = manager
		_ = pod

		t.Skip("Skeleton: Implementation required - canPodsBeRescheduled reason formatting")
	})
}

// TestBackwardCompatibility tests that existing scale-down behavior is preserved.
// AC6: "Existing clusters continue to work (nodes without special constraints scale down normally)"
func TestBackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// AC6: Backward Compatibility - Simple pods
	// ROI: 111 | Business Value: 10 (regression prevention) | Frequency: 10 (all users)
	// Behavior: Pods without constraints → Scale-down proceeds as before
	// @category: core-functionality
	// @dependency: IsSafeToRemove, canPodsBeRescheduled
	// @complexity: low
	t.Run("AC6: Scale-down works for simple pods without constraints", func(t *testing.T) {
		// Arrange:
		// - Create node to be removed with simple pod (no tolerations, nodeSelector, affinity)
		// - Create remaining node with sufficient capacity
		// - Both nodes without any taints or special labels

		nodeToRemove := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-remove",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		remainingNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-keep",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": "test-group",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		}

		simplePod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple-app",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				// No tolerations
				// No nodeSelector
				// No affinity
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode)
		manager := &ScaleDownManager{
			client: fakeClient,
			logger: logger.Sugar(),
			config: DefaultConfig(),
		}

		// Act:
		safe, reason, err := manager.IsSafeToRemove(ctx, nodeToRemove, []*corev1.Pod{simplePod})

		// Assert:
		// - Verify IsSafeToRemove returns (true, "safe to remove", nil)
		// - Verify no constraint-related blocking
		require.NoError(t, err, "IsSafeToRemove should not return error for simple pods")
		assert.True(t, safe, "Scale-down should be safe for simple pods without constraints")
		assert.Equal(t, "safe to remove", reason, "Expected 'safe to remove' reason for simple pods")

		// Verification items:
		// - safe == true
		// - reason == "safe to remove"
		// - err == nil
	})
}
