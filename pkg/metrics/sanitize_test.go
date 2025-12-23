package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestSanitizeLabel(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedOutput  string
		expectedChanged bool
	}{
		{
			name:            "valid alphanumeric",
			input:           "nodegroup123",
			expectedOutput:  "nodegroup123",
			expectedChanged: false,
		},
		{
			name:            "valid with underscore",
			input:           "node_group_123",
			expectedOutput:  "node_group_123",
			expectedChanged: false,
		},
		{
			name:            "valid with hyphen",
			input:           "node-group-123",
			expectedOutput:  "node-group-123",
			expectedChanged: false,
		},
		{
			name:            "valid with dot",
			input:           "node.group.123",
			expectedOutput:  "node.group.123",
			expectedChanged: false,
		},
		{
			name:            "empty string",
			input:           "",
			expectedOutput:  "unknown",
			expectedChanged: true,
		},
		{
			name:            "spaces replaced with underscore",
			input:           "node group 123",
			expectedOutput:  "node_group_123",
			expectedChanged: true,
		},
		{
			name:            "special characters replaced",
			input:           "node@group#123",
			expectedOutput:  "node_group_123",
			expectedChanged: true,
		},
		{
			name:            "slashes replaced",
			input:           "node/group/123",
			expectedOutput:  "node_group_123",
			expectedChanged: true,
		},
		{
			name:            "parentheses replaced",
			input:           "node(group)123",
			expectedOutput:  "node_group_123",
			expectedChanged: true,
		},
		{
			name:            "equals sign replaced",
			input:           "key=value",
			expectedOutput:  "key_value",
			expectedChanged: true,
		},
		{
			name:            "colon replaced",
			input:           "namespace:name",
			expectedOutput:  "namespace_name",
			expectedChanged: true,
		},
		{
			name:            "truncated at max length",
			input:           strings.Repeat("a", 150),
			expectedOutput:  strings.Repeat("a", MaxLabelLength),
			expectedChanged: true,
		},
		{
			name:            "truncated and sanitized",
			input:           strings.Repeat("a@", 100),
			expectedOutput:  strings.Repeat("a_", 64), // 128 chars
			expectedChanged: true,
		},
		{
			name:            "exactly max length",
			input:           strings.Repeat("a", MaxLabelLength),
			expectedOutput:  strings.Repeat("a", MaxLabelLength),
			expectedChanged: false,
		},
		{
			name:            "unicode replaced",
			input:           "node™group®",
			expectedOutput:  "node_group_",
			expectedChanged: true,
		},
		{
			name:            "mixed valid and invalid",
			input:           "my-node_group.123@prod",
			expectedOutput:  "my-node_group.123_prod",
			expectedChanged: true,
		},
		{
			name:            "kubernetes label format",
			input:           "app.kubernetes.io/name",
			expectedOutput:  "app.kubernetes.io_name",
			expectedChanged: true,
		},
		{
			name:            "URL-like string",
			input:           "https://example.com/path",
			expectedOutput:  "https___example.com_path",
			expectedChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, changed := SanitizeLabel(tt.input)
			assert.Equal(t, tt.expectedOutput, output, "Unexpected output")
			assert.Equal(t, tt.expectedChanged, changed, "Unexpected changed flag")
		})
	}
}

func TestSanitizeLabelWithLog(t *testing.T) {
	// Create a no-op logger for testing
	logger := zap.NewNop()

	tests := []struct {
		name           string
		input          string
		labelName      string
		expectedOutput string
	}{
		{
			name:           "valid label no logging",
			input:          "valid-label",
			labelName:      "nodegroup",
			expectedOutput: "valid-label",
		},
		{
			name:           "invalid label with logging",
			input:          "invalid@label",
			labelName:      "namespace",
			expectedOutput: "invalid_label",
		},
		{
			name:           "empty label with logging",
			input:          "",
			labelName:      "reason",
			expectedOutput: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := SanitizeLabelWithLog(tt.input, tt.labelName, logger)
			assert.Equal(t, tt.expectedOutput, output)
		})
	}
}

func TestGetSanitizationReason(t *testing.T) {
	tests := []struct {
		name              string
		original          string
		sanitized         string
		expectedReasonKey string // Just check if key exists in reason
	}{
		{
			name:              "too long",
			original:          strings.Repeat("a", 150),
			sanitized:         strings.Repeat("a", 128),
			expectedReasonKey: "exceeded_max_length",
		},
		{
			name:              "invalid characters",
			original:          "test@label",
			sanitized:         "test_label",
			expectedReasonKey: "invalid_characters",
		},
		{
			name:              "empty value",
			original:          "",
			sanitized:         "unknown",
			expectedReasonKey: "empty_value",
		},
		{
			name:              "both too long and invalid chars",
			original:          strings.Repeat("a@", 100),
			sanitized:         strings.Repeat("a_", 64),
			expectedReasonKey: "exceeded_max_length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := getSanitizationReason(tt.original, tt.sanitized)
			assert.Contains(t, reason, tt.expectedReasonKey, "Expected reason to contain key")
		})
	}
}

func BenchmarkSanitizeLabel(b *testing.B) {
	testCases := []string{
		"valid-label",
		"invalid@label#with$special",
		strings.Repeat("a", 150),
		"mixed-valid_and.invalid@chars",
	}

	for _, tc := range testCases {
		b.Run(tc, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				SanitizeLabel(tc)
			}
		})
	}
}
