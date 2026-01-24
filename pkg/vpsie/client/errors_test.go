package client

import (
	"errors"
	"testing"
)

func TestIsTerminalError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "worker nodes limit exceeded",
			err:      errors.New("Worker nodes count exceeds the allowed limit 5"),
			expected: true,
		},
		{
			name:     "quota exceeded",
			err:      errors.New("Resource quota exceeded for this project"),
			expected: true,
		},
		{
			name:     "plan restriction",
			err:      errors.New("Your plan does not allow more than 3 clusters"),
			expected: true,
		},
		{
			name:     "maximum number reached",
			err:      errors.New("Maximum number of VMs reached for your account"),
			expected: true,
		},
		{
			name:     "limit reached",
			err:      errors.New("CPU limit reached for this datacenter"),
			expected: true,
		},
		{
			name:     "subscription restriction",
			err:      errors.New("Your subscription does not allow this operation"),
			expected: true,
		},
		{
			name:     "VPSie API error with limit message",
			err:      NewAPIError(500, "Internal Server Error", `{"error":true,"code":500,"message":"Worker nodes count exceeds the allowed limit 5","type":false}`),
			expected: true,
		},
		{
			name:     "regular API error",
			err:      NewAPIError(500, "Internal Server Error", "Something went wrong"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      NewAPIError(404, "Not Found", "Resource not found"),
			expected: false,
		},
		{
			name:     "network timeout",
			err:      errors.New("connection timeout"),
			expected: false,
		},
		{
			name:     "rate limited",
			err:      NewAPIError(429, "Too Many Requests", "Rate limit exceeded"),
			expected: false,
		},
		{
			name:     "case insensitive - uppercase",
			err:      errors.New("WORKER NODES COUNT EXCEEDS THE ALLOWED LIMIT"),
			expected: true,
		},
		{
			name:     "case insensitive - mixed case",
			err:      errors.New("Quota Exceeded for your account"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTerminalError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTerminalError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "404 error",
			err:      NewAPIError(404, "Not Found", "Resource not found"),
			expected: true,
		},
		{
			name:     "500 error",
			err:      NewAPIError(500, "Server Error", "Something went wrong"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "429 error",
			err:      NewAPIError(429, "Too Many Requests", "Rate limit exceeded"),
			expected: true,
		},
		{
			name:     "500 error",
			err:      NewAPIError(500, "Server Error", "Something went wrong"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimited(tt.err)
			if result != tt.expected {
				t.Errorf("IsRateLimited(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
