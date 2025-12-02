package nodegroup

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

// VersionInfo represents a parsed Kubernetes version
type VersionInfo struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	Original   string
}

// ParseVersion parses a Kubernetes version string (e.g., "v1.28.0", "v1.29.1-rc.0")
func ParseVersion(version string) (*VersionInfo, error) {
	if version == "" {
		return nil, fmt.Errorf("version string is empty")
	}

	// Remove 'v' prefix if present
	v := strings.TrimPrefix(version, "v")

	// Split on '+' to separate build metadata
	parts := strings.Split(v, "+")
	v = parts[0]
	build := ""
	if len(parts) > 1 {
		build = parts[1]
	}

	// Split on '-' to separate prerelease
	parts = strings.Split(v, "-")
	v = parts[0]
	prerelease := ""
	if len(parts) > 1 {
		prerelease = parts[1]
	}

	// Split version into major.minor.patch
	versionParts := strings.Split(v, ".")
	if len(versionParts) != 3 {
		return nil, fmt.Errorf("invalid version format: %s (expected major.minor.patch)", version)
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", versionParts[0])
	}

	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", versionParts[1])
	}

	patch, err := strconv.Atoi(versionParts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version: %s", versionParts[2])
	}

	return &VersionInfo{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		Build:      build,
		Original:   version,
	}, nil
}

// String returns the version as a string
func (v *VersionInfo) String() string {
	return v.Original
}

// IsCompatibleWith checks if this version is compatible with another version
// Kubernetes supports nodes within ±1 minor version of the control plane
func (v *VersionInfo) IsCompatibleWith(controlPlane *VersionInfo) bool {
	// Major version must match
	if v.Major != controlPlane.Major {
		return false
	}

	// Minor version must be within ±1
	minorDiff := v.Minor - controlPlane.Minor
	if minorDiff < -1 || minorDiff > 1 {
		return false
	}

	return true
}

// GetClusterVersion retrieves the Kubernetes version of the cluster
func GetClusterVersion(ctx context.Context, clientset kubernetes.Interface) (*VersionInfo, error) {
	// Get server version from discovery client
	discoveryClient := clientset.Discovery()
	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster version: %w", err)
	}

	// Parse the version string
	// ServerVersion returns GitVersion like "v1.28.0"
	version, err := ParseVersion(serverVersion.GitVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster version %s: %w", serverVersion.GitVersion, err)
	}

	return version, nil
}

// ValidateNodeVersion validates that a node's Kubernetes version is compatible with the cluster
func ValidateNodeVersion(ctx context.Context, clientset kubernetes.Interface, nodeVersion string) error {
	// Parse the node version
	node, err := ParseVersion(nodeVersion)
	if err != nil {
		return fmt.Errorf("invalid node version: %w", err)
	}

	// Get cluster version
	cluster, err := GetClusterVersion(ctx, clientset)
	if err != nil {
		return fmt.Errorf("failed to get cluster version: %w", err)
	}

	// Check compatibility
	if !node.IsCompatibleWith(cluster) {
		return fmt.Errorf(
			"node version %s is not compatible with cluster version %s (nodes must be within ±1 minor version of control plane)",
			node.String(),
			cluster.String(),
		)
	}

	return nil
}

// GetDiscoveryClient is a helper for testing
func GetDiscoveryClient(clientset kubernetes.Interface) discovery.DiscoveryInterface {
	return clientset.Discovery()
}
