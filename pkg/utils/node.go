package utils

import (
	corev1 "k8s.io/api/core/v1"
)

// IsNodeReady checks if a Kubernetes Node is in Ready condition.
// Returns true if the node has a Ready condition with status True.
func IsNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
