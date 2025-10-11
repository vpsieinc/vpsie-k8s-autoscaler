# VPSie Client Usage Guide

This document provides examples and best practices for using the VPSie API client.

## Quick Start

### Creating a Client

#### From Kubernetes Secret (Production)
```go
package main

import (
    "context"
    "log"

    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"

    "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

func main() {
    // Create Kubernetes client
    config, err := rest.InClusterConfig()
    if err != nil {
        log.Fatal(err)
    }

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatal(err)
    }

    // Create VPSie client from secret
    ctx := context.Background()
    vpsieClient, err := client.NewClient(ctx, clientset, &client.ClientOptions{
        SecretName:      "vpsie-secret",      // default
        SecretNamespace: "kube-system",       // default
        RateLimit:       100,                 // requests per minute
        Timeout:         30 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Use the client...
}
```

#### With Explicit Credentials (Testing)
```go
vpsieClient, err := client.NewClientWithCredentials(
    "https://api.vpsie.com/v2",
    "your-api-token-here",
    &client.ClientOptions{
        RateLimit: 100,
    },
)
```

## VPS Lifecycle Operations

### List All VMs

```go
ctx := context.Background()

// List all VMs
vms, err := vpsieClient.ListVMs(ctx)
if err != nil {
    log.Fatalf("failed to list VMs: %v", err)
}

for _, vm := range vms {
    log.Printf("VM: %s (ID: %s, Status: %s, IP: %s)",
        vm.Name, vm.ID, vm.Status, vm.IPAddress)
}
```

### Create a VM

```go
ctx := context.Background()

// Define the VM configuration
req := client.CreateVPSRequest{
    Name:         "k8s-worker-node-1",
    Hostname:     "worker-1.k8s.local",
    OfferingID:   "offering-4cpu-8gb",
    DatacenterID: "dc-us-east-1",
    OSImageID:    "ubuntu-22.04-lts",
    SSHKeyIDs:    []string{"ssh-key-123"},
    Tags:         []string{"kubernetes", "worker", "autoscaler"},
    Notes:        "Created by VPSie K8s Autoscaler",
    UserData:     cloudInitUserData, // base64-encoded cloud-init
}

// Create the VM
vm, err := vpsieClient.CreateVM(ctx, req)
if err != nil {
    log.Fatalf("failed to create VM: %v", err)
}

log.Printf("Created VM: %s (ID: %s, IP: %s)", vm.Name, vm.ID, vm.IPAddress)

// Wait for VM to be running
for {
    vm, err := vpsieClient.GetVM(ctx, vm.ID)
    if err != nil {
        log.Fatalf("failed to get VM status: %v", err)
    }

    if vm.Status == "running" {
        log.Printf("VM is now running!")
        break
    }

    log.Printf("VM status: %s, waiting...", vm.Status)
    time.Sleep(5 * time.Second)
}
```

### Get VM Details

```go
ctx := context.Background()

vm, err := vpsieClient.GetVM(ctx, "vm-123")
if err != nil {
    if client.IsNotFound(err) {
        log.Printf("VM not found")
        return
    }
    log.Fatalf("failed to get VM: %v", err)
}

log.Printf("VM Details:")
log.Printf("  Name: %s", vm.Name)
log.Printf("  Status: %s", vm.Status)
log.Printf("  IP: %s", vm.IPAddress)
log.Printf("  CPU: %d cores", vm.CPU)
log.Printf("  RAM: %d MB", vm.RAM)
log.Printf("  Disk: %d GB", vm.Disk)
log.Printf("  Created: %s", vm.CreatedAt)
```

### Delete a VM

```go
ctx := context.Background()

// Simple deletion
err := vpsieClient.DeleteVM(ctx, "vm-123")
if err != nil {
    log.Fatalf("failed to delete VM: %v", err)
}
log.Printf("VM deleted successfully")

// With error handling
err = vpsieClient.DeleteVM(ctx, vmID)
if err != nil {
    if apiErr, ok := err.(*client.APIError); ok {
        switch apiErr.StatusCode {
        case 409:
            log.Printf("VM is still running, stopping first...")
            // Stop VM and retry
        case 403:
            log.Printf("Permission denied to delete VM")
        default:
            log.Printf("API error: %v", apiErr)
        }
    } else {
        log.Printf("Unexpected error: %v", err)
    }
}
```

## Error Handling

### Checking Error Types

```go
vm, err := vpsieClient.GetVM(ctx, "vm-123")
if err != nil {
    // Check for specific error types
    if client.IsNotFound(err) {
        log.Printf("VM does not exist")
        return
    }

    if client.IsUnauthorized(err) {
        log.Printf("Invalid API credentials")
        return
    }

    if client.IsRateLimited(err) {
        log.Printf("Rate limit exceeded, waiting...")
        time.Sleep(60 * time.Second)
        // Retry...
        return
    }

    // Check for APIError with more details
    if apiErr, ok := err.(*client.APIError); ok {
        log.Printf("API Error: %s (Request ID: %s)",
            apiErr.Message, apiErr.RequestID)
        return
    }

    log.Fatalf("Unexpected error: %v", err)
}
```

### Handling Specific Status Codes

```go
_, err := vpsieClient.CreateVM(ctx, req)
if err != nil {
    if apiErr, ok := err.(*client.APIError); ok {
        switch apiErr.StatusCode {
        case 400:
            log.Printf("Invalid request: %s", apiErr.Message)
        case 402:
            log.Printf("Insufficient balance")
        case 409:
            log.Printf("Resource conflict: %s", apiErr.Message)
        case 429:
            log.Printf("Rate limited, retry after delay")
        case 500, 502, 503:
            log.Printf("Server error, retry later")
        default:
            log.Printf("Unexpected API error: %s", apiErr.Error())
        }
    }
}
```

## Context Usage

### With Timeout

```go
// Set a timeout for the operation
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

vms, err := vpsieClient.ListVMs(ctx)
if err != nil {
    if ctx.Err() == context.DeadlineExceeded {
        log.Printf("Request timed out")
    } else {
        log.Printf("Request failed: %v", err)
    }
    return
}
```

### With Cancellation

```go
ctx, cancel := context.WithCancel(context.Background())

// Start a goroutine to cancel after some event
go func() {
    <-someChannel
    cancel()
}()

vm, err := vpsieClient.CreateVM(ctx, req)
if err != nil {
    if ctx.Err() == context.Canceled {
        log.Printf("Request was cancelled")
    }
}
```

## Best Practices

### 1. Always Use Context

```go
// Good - allows cancellation and timeout
vms, err := client.ListVMs(ctx)

// Bad - can't be cancelled or timed out
// (Don't use context.Background() without timeout in production)
```

### 2. Handle Rate Limiting

The client automatically rate limits to 100 requests/minute by default. The rate limiter will block until a token is available.

```go
// The client handles this automatically
// But you can configure the rate limit
opts := &client.ClientOptions{
    RateLimit: 200, // 200 requests per minute
}
```

### 3. Validate Input Before API Calls

```go
// The CreateVM method validates required fields
// But you should also validate business logic
if len(req.Name) > 64 {
    return nil, fmt.Errorf("VM name too long")
}

vm, err := client.CreateVM(ctx, req)
```

### 4. Use Idempotent Operations

```go
// DeleteVM is idempotent - 404 is treated as success
err := client.DeleteVM(ctx, vmID)
// Returns nil if VM is deleted or doesn't exist

// For CreateVM, check if VM exists first
_, err := client.GetVM(ctx, expectedID)
if err == nil {
    log.Printf("VM already exists, skipping creation")
} else if client.IsNotFound(err) {
    // Create the VM
    vm, err := client.CreateVM(ctx, req)
}
```

### 5. Retry Transient Failures

```go
func createVMWithRetry(ctx context.Context, client *client.Client, req client.CreateVPSRequest) (*client.VPS, error) {
    maxRetries := 3
    backoff := time.Second

    for i := 0; i < maxRetries; i++ {
        vm, err := client.CreateVM(ctx, req)
        if err == nil {
            return vm, nil
        }

        // Check if error is retryable
        if apiErr, ok := err.(*client.APIError); ok {
            if apiErr.IsServerError() || apiErr.IsRateLimited() {
                log.Printf("Retryable error, attempt %d/%d", i+1, maxRetries)
                time.Sleep(backoff)
                backoff *= 2 // Exponential backoff
                continue
            }
        }

        // Non-retryable error
        return nil, err
    }

    return nil, fmt.Errorf("max retries exceeded")
}
```

### 6. Credential Rotation

```go
// Update credentials without recreating the client
err := vpsieClient.UpdateCredentials(newToken, newBaseURL)
if err != nil {
    log.Printf("Failed to update credentials: %v", err)
}
```

## Integration with Kubernetes Controller

```go
type NodeGroupReconciler struct {
    client.Client
    VPSieClient *client.Client
}

func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get desired state from NodeGroup CR
    var nodeGroup autoscalerv1alpha1.NodeGroup
    if err := r.Get(ctx, req.NamespacedName, &nodeGroup); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // List current VMs
    vms, err := r.VPSieClient.ListVMs(ctx)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to list VMs: %w", err)
    }

    // Filter VMs by tags
    var nodeGroupVMs []client.VPS
    for _, vm := range vms {
        if hasTag(vm.Tags, nodeGroup.Name) {
            nodeGroupVMs = append(nodeGroupVMs, vm)
        }
    }

    // Scale up if needed
    if len(nodeGroupVMs) < nodeGroup.Spec.MinNodes {
        createReq := client.CreateVPSRequest{
            Name:         fmt.Sprintf("%s-node-%d", nodeGroup.Name, time.Now().Unix()),
            OfferingID:   nodeGroup.Spec.OfferingID,
            DatacenterID: nodeGroup.Spec.DatacenterID,
            OSImageID:    nodeGroup.Spec.OSImageID,
            Tags:         []string{nodeGroup.Name, "autoscaler"},
        }

        _, err := r.VPSieClient.CreateVM(ctx, createReq)
        if err != nil {
            return ctrl.Result{}, err
        }
    }

    return ctrl.Result{}, nil
}
```
