package utils

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "node with Ready=True condition",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-ready",
				},
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
			name: "node with Ready=False condition",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-not-ready",
				},
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
			name: "node with Ready=Unknown condition",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-unknown",
				},
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
			name: "node with no conditions",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-no-conditions",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{},
				},
			},
			expected: false,
		},
		{
			name: "node with nil conditions",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-nil-conditions",
				},
				Status: corev1.NodeStatus{},
			},
			expected: false,
		},
		{
			name: "node with other conditions but no Ready",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-other-conditions",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   corev1.NodeDiskPressure,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   corev1.NodePIDPressure,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "node with multiple conditions including Ready=True",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-multiple-conditions",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.NodeDiskPressure,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "node with Ready condition first",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-ready-first",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "node with Ready condition last",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-ready-last",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.NodeDiskPressure,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNodeReady(tt.node)
			if result != tt.expected {
				t.Errorf("IsNodeReady(%s) = %v, expected %v", tt.node.Name, result, tt.expected)
			}
		})
	}
}

func TestIsNodeReady_NilNode(t *testing.T) {
	// This test documents the expected behavior with nil node
	// The function will panic if called with nil, which is acceptable
	// as callers should not pass nil nodes
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling IsNodeReady with nil node")
		}
	}()
	IsNodeReady(nil)
}
