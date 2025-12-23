package scaler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestIsSafeToRemove(t *testing.T) {
	logger := zap.NewNop()
	clientset := fake.NewSimpleClientset()
	sm := NewScaleDownManager(clientset, logger)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup":           "test-ng",
				"autoscaler.vpsie.com/nodegroup-namespace": "default",
			},
		},
	}

	tests := []struct {
		name          string
		pods          []*corev1.Pod
		expectedSafe  bool
		expectedReason string
	}{
		{
			name:          "no pods - safe to remove",
			pods:          []*corev1.Pod{},
			expectedSafe:  true,
			expectedReason: "",
		},
		{
			name: "pods with local storage - unsafe",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "local-vol",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			},
			expectedSafe:  false,
			expectedReason: "node has pods with local storage (EmptyDir volumes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe, reason, err := sm.IsSafeToRemove(context.TODO(), node, tt.pods)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSafe, safe)
			if !tt.expectedSafe {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

func TestHasPodsWithLocalStorage(t *testing.T) {
	logger := zap.NewNop()
	clientset := fake.NewSimpleClientset()
	sm := NewScaleDownManager(clientset, logger)

	tests := []struct {
		name             string
		pods             []*corev1.Pod
		expectedHasLocal bool
		expectedReason   string
	}{
		{
			name:             "no pods",
			pods:             []*corev1.Pod{},
			expectedHasLocal: false,
		},
		{
			name: "pod with EmptyDir volume",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
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
			expectedHasLocal: true,
			expectedReason:   "EmptyDir volumes",
		},
		{
			name: "pod with HostPath volume",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
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
			expectedHasLocal: true,
			expectedReason:   "HostPath volumes",
		},
		{
			name: "pod with ConfigMap volume - safe",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-3"},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "config"},
									},
								},
							},
						},
					},
				},
			},
			expectedHasLocal: false,
		},
		{
			name: "pod with PersistentVolumeClaim - safe",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-4"},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "data",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-1",
									},
								},
							},
						},
					},
				},
			},
			expectedHasLocal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasLocal, reason := sm.hasPodsWithLocalStorage(context.TODO(), tt.pods)
			assert.Equal(t, tt.expectedHasLocal, hasLocal)
			if tt.expectedHasLocal {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

func TestCanPodsBeRescheduled(t *testing.T) {
	logger := zap.NewNop()
	clientset := fake.NewSimpleClientset()
	sm := NewScaleDownManager(clientset, logger)

	tests := []struct {
		name              string
		pods              []*corev1.Pod
		expectedCanSched  bool
		expectedReason    string
	}{
		{
			name:             "no pods - can reschedule",
			pods:             []*corev1.Pod{},
			expectedCanSched: true,
		},
		{
			name: "DaemonSet pod - can reschedule",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ds-pod",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "DaemonSet",
								Name: "test-ds",
							},
						},
					},
				},
			},
			expectedCanSched: true,
		},
		{
			name: "Pod with node selector matching other nodes - can reschedule",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
					Spec: corev1.PodSpec{
						NodeSelector: map[string]string{
							"type": "worker",
						},
					},
				},
			},
			expectedCanSched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canSched, reason, err := sm.canPodsBeRescheduled(context.TODO(), tt.pods)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCanSched, canSched)
			if !tt.expectedCanSched {
				assert.NotEmpty(t, reason)
			}
		})
	}
}

func TestHasSystemPods(t *testing.T) {
	logger := zap.NewNop()
	clientset := fake.NewSimpleClientset()
	sm := NewScaleDownManager(clientset, logger)

	tests := []struct {
		name               string
		pods               []*corev1.Pod
		expectedHasSystem  bool
		expectedReason     string
	}{
		{
			name:              "no pods",
			pods:              []*corev1.Pod{},
			expectedHasSystem: false,
		},
		{
			name: "kube-system pod",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-proxy",
						Namespace: "kube-system",
					},
				},
			},
			expectedHasSystem: true,
			expectedReason:    "kube-system",
		},
		{
			name: "user namespace pod - safe",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-pod",
						Namespace: "default",
					},
				},
			},
			expectedHasSystem: false,
		},
		{
			name: "pod with critical priority class",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "critical-pod",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						PriorityClassName: "system-node-critical",
					},
				},
			},
			expectedHasSystem: true,
			expectedReason:    "system-node-critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasSystem, reason := sm.hasSystemPods(tt.pods)
			assert.Equal(t, tt.expectedHasSystem, hasSystem)
			if tt.expectedHasSystem {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

func TestHasPodsWithAntiAffinity(t *testing.T) {
	logger := zap.NewNop()
	clientset := fake.NewSimpleClientset()
	sm := NewScaleDownManager(clientset, logger)

	tests := []struct {
		name                 string
		pods                 []*corev1.Pod
		expectedHasAffinity  bool
	}{
		{
			name:                "no pods",
			pods:                []*corev1.Pod{},
			expectedHasAffinity: false,
		},
		{
			name: "pod with anti-affinity",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
					Spec: corev1.PodSpec{
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{"app": "test"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedHasAffinity: true,
		},
		{
			name: "pod without anti-affinity",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
					Spec:       corev1.PodSpec{},
				},
			},
			expectedHasAffinity: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasAffinity, _ := sm.hasPodsWithAntiAffinity(tt.pods)
			assert.Equal(t, tt.expectedHasAffinity, hasAffinity)
		})
	}
}

func TestWouldBreakCapacity(t *testing.T) {
	logger := zap.NewNop()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	tests := []struct {
		name             string
		nodes            []*corev1.Node
		expectedBreaks   bool
	}{
		{
			name: "sufficient capacity in cluster",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
					Status: corev1.NodeStatus{
						Capacity: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("16Gi"),
						},
						Allocatable: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("7"),
							corev1.ResourceMemory: resource.MustParse("14Gi"),
						},
					},
				},
			},
			expectedBreaks: false,
		},
		{
			name:           "no other nodes - would break capacity",
			nodes:          []*corev1.Node{},
			expectedBreaks: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			for _, n := range tt.nodes {
				_, err := clientset.CoreV1().Nodes().Create(context.TODO(), n, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			sm := NewScaleDownManager(clientset, logger)
			breaks, _, err := sm.wouldBreakCapacity(context.TODO(), node)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedBreaks, breaks)
		})
	}
}

func TestHasProtectionAnnotation(t *testing.T) {
	tests := []struct {
		name               string
		annotations        map[string]string
		expectedProtected  bool
	}{
		{
			name:              "no annotations",
			annotations:       nil,
			expectedProtected: false,
		},
		{
			name: "protected annotation present",
			annotations: map[string]string{
				"autoscaler.vpsie.com/scale-down-disabled": "true",
			},
			expectedProtected: true,
		},
		{
			name: "protected annotation false",
			annotations: map[string]string{
				"autoscaler.vpsie.com/scale-down-disabled": "false",
			},
			expectedProtected: false,
		},
		{
			name: "other annotations only",
			annotations: map[string]string{
				"some-other-annotation": "value",
			},
			expectedProtected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: tt.annotations,
				},
			}

			logger := zap.NewNop()
			clientset := fake.NewSimpleClientset()
			sm := NewScaleDownManager(clientset, logger)

			protected, _ := sm.hasProtectionAnnotation(node)
			assert.Equal(t, tt.expectedProtected, protected)
		})
	}
}
