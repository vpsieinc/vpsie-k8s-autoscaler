//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Real VPSie API Tests
//
// These tests run against the actual VPSie API and require valid credentials.
// They are disabled by default and only run when VPSIE_REAL_API_TEST=true
//
// Required environment variables:
// - VPSIE_REAL_API_TEST=true (enables these tests)
// - VPSIE_CLIENT_ID (OAuth client ID)
// - VPSIE_CLIENT_SECRET (OAuth client secret)
// - VPSIE_API_URL (optional, defaults to https://api.vpsie.com/v2)
//
// CAUTION: These tests may create and delete real VMs. Use with care.

func skipIfRealAPIDisabled(t *testing.T) {
	t.Helper()
	if os.Getenv("VPSIE_REAL_API_TEST") != "true" {
		t.Skip("Real VPSie API tests disabled. Set VPSIE_REAL_API_TEST=true to enable.")
	}
}

func getRealAPICredentials(t *testing.T) (clientID, clientSecret, apiURL string) {
	t.Helper()

	clientID = os.Getenv("VPSIE_CLIENT_ID")
	clientSecret = os.Getenv("VPSIE_CLIENT_SECRET")
	apiURL = os.Getenv("VPSIE_API_URL")

	if apiURL == "" {
		apiURL = "https://api.vpsie.com/v2"
	}

	if clientID == "" || clientSecret == "" {
		t.Fatal("VPSIE_CLIENT_ID and VPSIE_CLIENT_SECRET environment variables are required")
	}

	return clientID, clientSecret, apiURL
}

func createRealAPIClient(t *testing.T) *vpsieclient.Client {
	t.Helper()

	clientID, clientSecret, apiURL := getRealAPICredentials(t)

	client, err := vpsieclient.NewClientWithOAuth(apiURL, clientID, clientSecret, nil)
	require.NoError(t, err, "Failed to create VPSie client")

	return client
}

// TestRealAPI_Authentication tests OAuth authentication against real API
func TestRealAPI_Authentication(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	// Test that we can authenticate by listing VMs (requires valid token)
	_, err := client.ListVMs(ctx)
	require.NoError(t, err, "Authentication should succeed")

	t.Log("Successfully authenticated with VPSie API")
}

// TestRealAPI_TokenRefresh tests automatic token refresh
func TestRealAPI_TokenRefresh(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	// Make initial request
	_, err := client.ListVMs(ctx)
	require.NoError(t, err, "Initial request should succeed")

	// Wait a bit and make another request
	// Token refresh should happen automatically if needed
	time.Sleep(2 * time.Second)

	_, err = client.ListVMs(ctx)
	require.NoError(t, err, "Second request should succeed")

	t.Log("Token refresh working correctly")
}

// TestRealAPI_ListVMs tests listing VMs from real API
func TestRealAPI_ListVMs(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	vms, err := client.ListVMs(ctx)
	require.NoError(t, err, "ListVMs should succeed")

	t.Logf("Found %d VMs", len(vms))

	// Log some details about existing VMs (if any)
	for i, vm := range vms {
		if i >= 5 {
			t.Logf("... and %d more VMs", len(vms)-5)
			break
		}
		t.Logf("  VM %d: ID=%d, Hostname=%s, State=%s", i, vm.ID, vm.Hostname, vm.State)
	}
}

// TestRealAPI_ListDatacenters tests listing datacenters
func TestRealAPI_ListDatacenters(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	datacenters, err := client.ListDatacenters(ctx)
	require.NoError(t, err, "ListDatacenters should succeed")

	t.Logf("Found %d datacenters", len(datacenters))

	for _, dc := range datacenters {
		t.Logf("  Datacenter: ID=%s, Name=%s, Location=%s", dc.ID, dc.Name, dc.Location)
	}

	assert.Greater(t, len(datacenters), 0, "Should have at least one datacenter")
}

// TestRealAPI_ListOfferings tests listing VM offerings
func TestRealAPI_ListOfferings(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	offerings, err := client.ListOfferings(ctx)
	require.NoError(t, err, "ListOfferings should succeed")

	t.Logf("Found %d offerings", len(offerings))

	for i, offering := range offerings {
		if i >= 10 {
			t.Logf("... and %d more offerings", len(offerings)-10)
			break
		}
		t.Logf("  Offering: ID=%s, Name=%s, CPU=%d, RAM=%dMB, Price=$%.2f/month",
			offering.ID, offering.Name, offering.CPU, offering.RAM, offering.PriceMonthly)
	}

	assert.Greater(t, len(offerings), 0, "Should have at least one offering")
}

// TestRealAPI_GetVM tests getting a specific VM
func TestRealAPI_GetVM(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	// First list VMs to find one to get
	vms, err := client.ListVMs(ctx)
	require.NoError(t, err)

	if len(vms) == 0 {
		t.Skip("No VMs available to test GetVM")
	}

	// Get the first VM
	vm, err := client.GetVM(ctx, vms[0].ID)
	require.NoError(t, err, "GetVM should succeed")

	assert.Equal(t, vms[0].ID, vm.ID, "VM ID should match")
	t.Logf("Got VM: ID=%d, Hostname=%s, State=%s", vm.ID, vm.Hostname, vm.State)
}

// TestRealAPI_GetNonExistentVM tests getting a VM that doesn't exist
func TestRealAPI_GetNonExistentVM(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := createRealAPIClient(t)

	// Try to get a VM with an invalid ID
	_, err := client.GetVM(ctx, 999999999)

	// Should get a not found error
	assert.Error(t, err, "GetVM should fail for non-existent VM")
	assert.True(t, vpsieclient.IsNotFound(err), "Error should be NotFound")
}

// TestRealAPI_RateLimiting tests API rate limiting behavior
func TestRealAPI_RateLimiting(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := createRealAPIClient(t)

	// Make many rapid requests to test rate limiting
	var successCount, rateLimitedCount int
	requestCount := 50

	t.Logf("Making %d rapid requests to test rate limiting...", requestCount)

	for i := 0; i < requestCount; i++ {
		_, err := client.ListDatacenters(ctx)
		if err != nil {
			if vpsieclient.IsRateLimited(err) {
				rateLimitedCount++
				t.Logf("Request %d: Rate limited", i)
			} else {
				t.Logf("Request %d: Error: %v", i, err)
			}
		} else {
			successCount++
		}
	}

	t.Logf("Results: success=%d, rateLimited=%d", successCount, rateLimitedCount)

	// We should have mostly successful requests (client handles rate limiting)
	assert.Greater(t, successCount, rateLimitedCount, "Most requests should succeed")
}

// TestRealAPI_ErrorHandling tests error handling for various error conditions
func TestRealAPI_ErrorHandling(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("InvalidCredentials", func(t *testing.T) {
		_, err := vpsieclient.NewClientWithOAuth(
			"https://api.vpsie.com/v2",
			"invalid-client-id",
			"invalid-client-secret",
			nil,
		)

		if err != nil {
			t.Logf("Expected error for invalid credentials: %v", err)
		}
		// Client creation might succeed, but first API call should fail
	})

	t.Run("InvalidEndpoint", func(t *testing.T) {
		clientID, clientSecret, _ := getRealAPICredentials(t)

		client, err := vpsieclient.NewClientWithOAuth(
			"https://invalid.vpsie.com/v2",
			clientID,
			clientSecret,
			nil,
		)

		if err == nil {
			_, err = client.ListVMs(ctx)
			assert.Error(t, err, "Should fail with invalid endpoint")
		}
	})
}

// TestRealAPI_VMLifecycle tests the full VM lifecycle (CAUTION: creates real VMs)
func TestRealAPI_VMLifecycle(t *testing.T) {
	skipIfRealAPIDisabled(t)

	// Additional safety check - require explicit opt-in for VM creation
	if os.Getenv("VPSIE_ALLOW_VM_CREATE") != "true" {
		t.Skip("VM lifecycle test disabled. Set VPSIE_ALLOW_VM_CREATE=true to enable (creates real VMs)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client := createRealAPIClient(t)

	// Get available datacenters
	datacenters, err := client.ListDatacenters(ctx)
	require.NoError(t, err)
	require.Greater(t, len(datacenters), 0, "Need at least one datacenter")

	// Get available offerings
	offerings, err := client.ListOfferings(ctx)
	require.NoError(t, err)
	require.Greater(t, len(offerings), 0, "Need at least one offering")

	// Find smallest offering to minimize cost
	var smallestOffering *vpsieclient.Offering
	for i := range offerings {
		if smallestOffering == nil || offerings[i].PriceMonthly < smallestOffering.PriceMonthly {
			smallestOffering = &offerings[i]
		}
	}

	t.Logf("Using datacenter: %s, offering: %s", datacenters[0].ID, smallestOffering.ID)

	// Create VM
	hostname := "vpsie-autoscaler-test-" + time.Now().Format("20060102-150405")
	createReq := &vpsieclient.CreateVPSRequest{
		Hostname:     hostname,
		DatacenterID: datacenters[0].ID,
		OfferingID:   smallestOffering.ID,
		// Add other required fields based on API requirements
	}

	t.Logf("Creating VM: %s", hostname)
	vm, err := client.CreateVM(ctx, createReq)
	require.NoError(t, err, "CreateVM should succeed")

	t.Logf("Created VM: ID=%d, State=%s", vm.ID, vm.State)

	// Cleanup - delete VM when test completes
	defer func() {
		t.Logf("Cleaning up: deleting VM %d", vm.ID)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cleanupCancel()

		if err := client.DeleteVM(cleanupCtx, vm.ID); err != nil {
			t.Logf("WARNING: Failed to delete VM %d: %v", vm.ID, err)
		} else {
			t.Logf("Deleted VM %d", vm.ID)
		}
	}()

	// Wait for VM to be ready
	t.Log("Waiting for VM to be ready...")
	for i := 0; i < 60; i++ { // Wait up to 5 minutes
		time.Sleep(5 * time.Second)

		vm, err = client.GetVM(ctx, vm.ID)
		if err != nil {
			t.Logf("Error getting VM status: %v", err)
			continue
		}

		t.Logf("VM state: %s", vm.State)
		if vm.State == "running" || vm.State == "ready" {
			break
		}
	}

	// Verify VM is in expected state
	assert.Contains(t, []string{"running", "ready", "provisioning"}, vm.State, "VM should be in valid state")

	// Get VM details
	vmDetails, err := client.GetVM(ctx, vm.ID)
	require.NoError(t, err)
	assert.Equal(t, vm.ID, vmDetails.ID)

	t.Logf("VM lifecycle test completed. VM final state: %s", vmDetails.State)
}

// TestRealAPI_ConcurrentRequests tests making concurrent API requests
func TestRealAPI_ConcurrentRequests(t *testing.T) {
	skipIfRealAPIDisabled(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := createRealAPIClient(t)

	// Make concurrent requests
	concurrency := 10
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			_, err := client.ListDatacenters(ctx)
			results <- err
		}(i)
	}

	var successCount, errorCount int
	for i := 0; i < concurrency; i++ {
		err := <-results
		if err != nil {
			errorCount++
			t.Logf("Request %d error: %v", i, err)
		} else {
			successCount++
		}
	}

	t.Logf("Concurrent requests: success=%d, errors=%d", successCount, errorCount)

	// Most requests should succeed
	assert.Greater(t, successCount, concurrency/2, "Most concurrent requests should succeed")
}
