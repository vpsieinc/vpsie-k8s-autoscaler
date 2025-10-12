// Package v1alpha1 contains the v1alpha1 API definitions for the VPSie Kubernetes Autoscaler.
//
// The autoscaler.vpsie.com API group provides custom resources for managing
// automatic scaling of Kubernetes worker nodes on VPSie infrastructure.
//
// Key Resources:
//
// NodeGroup: Defines a logical group of nodes with shared properties and scaling policies.
// NodeGroups specify the minimum and maximum number of nodes, instance types,
// datacenter location, and scaling behavior (when to add or remove nodes).
//
// VPSieNode: Represents a single VPSie VPS instance that is part of the Kubernetes cluster.
// VPSieNodes track the lifecycle of individual nodes from provisioning through deletion,
// maintaining the mapping between VPSie VPS instances and Kubernetes nodes.
//
// +kubebuilder:object:generate=true
// +groupName=autoscaler.vpsie.com
package v1alpha1
