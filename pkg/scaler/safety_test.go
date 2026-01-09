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
