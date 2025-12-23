package metrics

import (
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// MaxLabelLength is the maximum length for a Prometheus label value
// Recommended by Prometheus best practices to prevent cardinality explosion
const MaxLabelLength = 128

// labelSanitizeRegex matches characters that are NOT allowed in Prometheus labels
// Allowed: alphanumeric, underscore, hyphen, dot
var labelSanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)

// SanitizeLabel sanitizes a string to be used as a Prometheus label value
// - Replaces invalid characters with underscore
// - Truncates to MaxLabelLength characters
// - Returns sanitized value and true if sanitization occurred
func SanitizeLabel(value string) (string, bool) {
	if value == "" {
		return "unknown", true
	}

	original := value
	sanitized := false

	// Replace invalid characters with underscore
	if labelSanitizeRegex.MatchString(value) {
		value = labelSanitizeRegex.ReplaceAllString(value, "_")
		sanitized = true
	}

	// Truncate to max length
	if len(value) > MaxLabelLength {
		value = value[:MaxLabelLength]
		sanitized = true
	}

	// Ensure it's not empty after sanitization
	if value == "" {
		return "unknown", true
	}

	return value, sanitized || (value != original)
}

// SanitizeLabelWithLog sanitizes a label value and logs a warning if sanitization occurred
func SanitizeLabelWithLog(value string, labelName string, logger *zap.Logger) string {
	sanitized, changed := SanitizeLabel(value)

	if changed {
		logger.Warn("Sanitized metric label value",
			zap.String("label", labelName),
			zap.String("original", value),
			zap.String("sanitized", sanitized),
			zap.String("reason", getSanitizationReason(value, sanitized)),
		)
	}

	return sanitized
}

// getSanitizationReason returns a human-readable reason for why sanitization occurred
func getSanitizationReason(original, sanitized string) string {
	reasons := []string{}

	if len(original) > MaxLabelLength {
		reasons = append(reasons, "exceeded_max_length")
	}

	if labelSanitizeRegex.MatchString(original) {
		reasons = append(reasons, "invalid_characters")
	}

	if original == "" {
		reasons = append(reasons, "empty_value")
	}

	if len(reasons) == 0 {
		return "unknown"
	}

	return strings.Join(reasons, ",")
}
