package client

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError represents an error returned by the VPSie API
type APIError struct {
	StatusCode int
	Message    string
	Details    string
	RequestID  string
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("VPSie API error (status: %d, request_id: %s): %s - %s",
			e.StatusCode, e.RequestID, e.Message, e.Details)
	}
	return fmt.Sprintf("VPSie API error (status: %d): %s - %s",
		e.StatusCode, e.Message, e.Details)
}

// IsNotFound returns true if the error is a 404 Not Found error
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsUnauthorized returns true if the error is a 401 Unauthorized error
func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// IsForbidden returns true if the error is a 403 Forbidden error
func (e *APIError) IsForbidden() bool {
	return e.StatusCode == http.StatusForbidden
}

// IsRateLimited returns true if the error is a 429 Too Many Requests error
func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

// IsServerError returns true if the error is a 5xx server error
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// NewAPIError creates a new APIError
func NewAPIError(statusCode int, message, details string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
	}
}

// NewAPIErrorWithRequestID creates a new APIError with a request ID
func NewAPIErrorWithRequestID(statusCode int, message, details, requestID string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
		RequestID:  requestID,
	}
}

// SecretError represents an error when reading Kubernetes secrets
type SecretError struct {
	SecretName      string
	SecretNamespace string
	Reason          string
	Err             error
}

// Error implements the error interface
func (e *SecretError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to access secret %s/%s: %s: %v",
			e.SecretNamespace, e.SecretName, e.Reason, e.Err)
	}
	return fmt.Sprintf("failed to access secret %s/%s: %s",
		e.SecretNamespace, e.SecretName, e.Reason)
}

// Unwrap returns the underlying error
func (e *SecretError) Unwrap() error {
	return e.Err
}

// NewSecretError creates a new SecretError
func NewSecretError(secretName, secretNamespace, reason string, err error) *SecretError {
	return &SecretError{
		SecretName:      secretName,
		SecretNamespace: secretNamespace,
		Reason:          reason,
		Err:             err,
	}
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field  string
	Reason string
}

// Error implements the error interface
func (e *ConfigError) Error() string {
	return fmt.Sprintf("configuration error in field '%s': %s", e.Field, e.Reason)
}

// NewConfigError creates a new ConfigError
func NewConfigError(field, reason string) *ConfigError {
	return &ConfigError{
		Field:  field,
		Reason: reason,
	}
}

// IsNotFound checks if an error is a 404 Not Found error
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsNotFound()
	}
	return false
}

// IsUnauthorized checks if an error is a 401 Unauthorized error
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsUnauthorized()
	}
	return false
}

// IsRateLimited checks if an error is a 429 Too Many Requests error
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRateLimited()
	}
	return false
}
