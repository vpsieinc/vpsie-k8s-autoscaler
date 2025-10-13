package vpsienode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNewJoiner(t *testing.T) {
	joiner := NewJoiner(nil, nil)

	assert.NotNil(t, joiner)
}

func TestJoiner_IsNodeReady(t *testing.T) {
	joiner := NewJoiner(nil, nil)

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "node is ready",
			node: &corev1.Node{
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
			name: "node is not ready",
			node: &corev1.Node{
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
			name: "node has no ready condition",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeDiskPressure,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "node has no conditions",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joiner.isNodeReady(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}

	assert.True(t, containsString(slice, "a"))
	assert.True(t, containsString(slice, "b"))
	assert.True(t, containsString(slice, "c"))
	assert.False(t, containsString(slice, "d"))
	assert.False(t, containsString([]string{}, "a"))
}

func TestRemoveString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		remove   string
		expected []string
	}{
		{
			name:     "remove existing string",
			slice:    []string{"a", "b", "c"},
			remove:   "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove non-existing string",
			slice:    []string{"a", "b", "c"},
			remove:   "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "remove from empty slice",
			slice:    []string{},
			remove:   "a",
			expected: nil,
		},
		{
			name:     "remove only element",
			slice:    []string{"a"},
			remove:   "a",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeString(tt.slice, tt.remove)
			assert.Equal(t, tt.expected, result)
		})
	}
}
