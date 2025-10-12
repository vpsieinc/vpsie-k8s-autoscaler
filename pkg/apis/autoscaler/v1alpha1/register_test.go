package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupVersion(t *testing.T) {
	assert.Equal(t, "autoscaler.vpsie.com", GroupVersion.Group)
	assert.Equal(t, "v1alpha1", GroupVersion.Version)
}

func TestSchemeBuilder_AddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()

	// Add our types to the scheme
	err := AddToScheme(scheme)
	assert.NoError(t, err)

	// Verify NodeGroup is registered
	gvk := schema.GroupVersionKind{
		Group:   "autoscaler.vpsie.com",
		Version: "v1alpha1",
		Kind:    "NodeGroup",
	}
	obj, err := scheme.New(gvk)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
	_, ok := obj.(*NodeGroup)
	assert.True(t, ok, "Expected *NodeGroup type")

	// Verify NodeGroupList is registered
	gvkList := schema.GroupVersionKind{
		Group:   "autoscaler.vpsie.com",
		Version: "v1alpha1",
		Kind:    "NodeGroupList",
	}
	objList, err := scheme.New(gvkList)
	assert.NoError(t, err)
	assert.NotNil(t, objList)
	_, ok = objList.(*NodeGroupList)
	assert.True(t, ok, "Expected *NodeGroupList type")

	// Verify VPSieNode is registered
	gvkNode := schema.GroupVersionKind{
		Group:   "autoscaler.vpsie.com",
		Version: "v1alpha1",
		Kind:    "VPSieNode",
	}
	objNode, err := scheme.New(gvkNode)
	assert.NoError(t, err)
	assert.NotNil(t, objNode)
	_, ok = objNode.(*VPSieNode)
	assert.True(t, ok, "Expected *VPSieNode type")

	// Verify VPSieNodeList is registered
	gvkNodeList := schema.GroupVersionKind{
		Group:   "autoscaler.vpsie.com",
		Version: "v1alpha1",
		Kind:    "VPSieNodeList",
	}
	objNodeList, err := scheme.New(gvkNodeList)
	assert.NoError(t, err)
	assert.NotNil(t, objNodeList)
	_, ok = objNodeList.(*VPSieNodeList)
	assert.True(t, ok, "Expected *VPSieNodeList type")
}

func TestSchemeBuilder_KnownTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	err := AddToScheme(scheme)
	assert.NoError(t, err)

	// Get all known types for our GroupVersion
	knownTypes := scheme.KnownTypes(GroupVersion)
	assert.NotEmpty(t, knownTypes)

	// Verify our types are known
	expectedTypes := []string{
		"NodeGroup",
		"NodeGroupList",
		"VPSieNode",
		"VPSieNodeList",
	}

	for _, typeName := range expectedTypes {
		_, exists := knownTypes[typeName]
		assert.True(t, exists, "Type %s should be registered", typeName)
	}
}

func TestSchemeBuilder_MultipleAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()

	// Adding multiple times should not cause errors
	err1 := AddToScheme(scheme)
	assert.NoError(t, err1)

	err2 := AddToScheme(scheme)
	assert.NoError(t, err2)

	// Types should still be registered correctly
	gvk := schema.GroupVersionKind{
		Group:   "autoscaler.vpsie.com",
		Version: "v1alpha1",
		Kind:    "NodeGroup",
	}
	obj, err := scheme.New(gvk)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}
