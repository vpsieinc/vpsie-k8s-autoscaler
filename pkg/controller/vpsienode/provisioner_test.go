package vpsienode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestNewProvisioner(t *testing.T) {
	provisioner := NewProvisioner(nil, "cloud-init-template", []string{"key1", "key2"})

	assert.NotNil(t, provisioner)
	assert.Equal(t, "cloud-init-template", provisioner.cloudInitTemplate)
	assert.Equal(t, []string{"key1", "key2"}, provisioner.sshKeyIDs)
}

func TestProvisioner_GenerateHostname(t *testing.T) {
	provisioner := NewProvisioner(nil, "", nil)

	tests := []struct {
		name         string
		vn           *v1alpha1.VPSieNode
		expectedName string
	}{
		{
			name: "uses NodeName if set",
			vn: &v1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vpsienode-123",
					Namespace: "default",
				},
				Spec: v1alpha1.VPSieNodeSpec{
					NodeName: "my-custom-node",
				},
			},
			expectedName: "my-custom-node",
		},
		{
			name: "uses VPSieNode name if NodeName not set",
			vn: &v1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vpsienode-456",
					Namespace: "default",
				},
				Spec: v1alpha1.VPSieNodeSpec{},
			},
			expectedName: "vpsienode-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname := provisioner.generateHostname(tt.vn)
			assert.Equal(t, tt.expectedName, hostname)
		})
	}
}

func TestProvisioner_GenerateCloudInit(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vn       *v1alpha1.VPSieNode
		contains []string
	}{
		{
			name:     "uses template if provided",
			template: "custom-template-content",
			vn: &v1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vn",
					Namespace: "default",
				},
			},
			contains: []string{"custom-template-content"},
		},
		{
			name:     "generates default cloud-init",
			template: "",
			vn: &v1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vn",
					Namespace: "default",
				},
				Spec: v1alpha1.VPSieNodeSpec{
					NodeGroupName: "test-ng",
				},
			},
			contains: []string{
				"#cloud-config",
				"hostname:",
				"test-vn",
				"test-ng",
				"packages:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provisioner := NewProvisioner(nil, tt.template, nil)
			cloudInit := provisioner.generateCloudInit(tt.vn)

			assert.NotEmpty(t, cloudInit)
			for _, expected := range tt.contains {
				assert.Contains(t, cloudInit, expected)
			}
		})
	}
}

func TestParseVPSIDFromString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  int
		expectErr bool
	}{
		{
			name:      "valid numeric ID",
			input:     "12345",
			expected:  12345,
			expectErr: false,
		},
		{
			name:      "zero ID",
			input:     "0",
			expected:  0,
			expectErr: false,
		},
		{
			name:      "invalid non-numeric ID",
			input:     "abc",
			expected:  0,
			expectErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			expected:  0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseVPSIDFromString(tt.input)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, id)
			}
		})
	}
}
