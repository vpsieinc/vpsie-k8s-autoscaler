# VPSie API Authentication Migration Guide

The VPSie Kubernetes Autoscaler now uses **VPSie's custom authentication** with short-lived access tokens instead of long-lived API tokens.

## What Changed

### Authentication Flow

**Before (Long-lived Token):**
```
Client → VPSie API (with static Bearer token)
```

**After (VPSie Custom Auth with Short-lived Tokens):**
```
Client → /auth/from/api (with clientId + clientSecret)
       ← Access Token (expires in ~1 hour)
Client → VPSie API (with short-lived Bearer access_token)
       ← 401 Unauthorized (token expired)
Client → /auth/from/api (refresh automatically)
       ← New Access Token
Client → VPSie API (with new access_token)
```

### Features

✅ **Automatic Token Refresh**: Tokens are refreshed 5 minutes before expiration
✅ **401 Retry Logic**: If a request fails with 401, the client automatically refreshes the token and retries once
✅ **Thread-Safe**: All token operations are mutex-protected
✅ **No Manual Token Management**: The client handles everything internally

## Kubernetes Secret Format

### Old Format (No Longer Supported)
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vpsie-secret
  namespace: kube-system
type: Opaque
stringData:
  token: "your-long-lived-api-token"
  url: "https://api.vpsie.com/v2"  # optional
```

### New Format (Required)
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vpsie-secret
  namespace: kube-system
type: Opaque
stringData:
  clientId: "your-oauth-client-id"
  clientSecret: "your-oauth-client-secret"
  url: "https://api.vpsie.com/v2"  # optional, defaults to https://api.vpsie.com/v2
```

### Creating the Secret

```bash
kubectl create secret generic vpsie-secret \
  --from-literal=clientId=YOUR_CLIENT_ID \
  --from-literal=clientSecret=YOUR_CLIENT_SECRET \
  --namespace=kube-system
```

Or with a custom API endpoint:

```bash
kubectl create secret generic vpsie-secret \
  --from-literal=clientId=YOUR_CLIENT_ID \
  --from-literal=clientSecret=YOUR_CLIENT_SECRET \
  --from-literal=url=https://api.custom.vpsie.com/v2 \
  --namespace=kube-system
```

## Code Changes

### Client Struct

**New Fields:**
```go
type Client struct {
    httpClient     *http.Client
    rateLimiter    *rate.Limiter
    baseURL        string
    clientID       string        // NEW: OAuth client ID
    clientSecret   string        // NEW: OAuth client secret
    accessToken    string        // NEW: Short-lived access token
    tokenExpiresAt time.Time     // NEW: Token expiration time
    userAgent      string
    mu             sync.RWMutex
}
```

### New Constants

```go
const (
    SecretClientIDKey     = "clientId"           // Replaces SecretTokenKey
    SecretClientSecretKey = "clientSecret"       // NEW
    TokenEndpoint         = "/auth/from/api"     // NEW: VPSie custom auth endpoint
    DefaultTokenRefreshBuffer = 5 * time.Minute  // NEW
)
```

### API Changes

#### NewClientWithCredentials (Breaking Change)

**Old Signature:**
```go
func NewClientWithCredentials(baseURL, token string, opts *ClientOptions) (*Client, error)
```

**New Signature:**
```go
func NewClientWithCredentials(baseURL, clientID, clientSecret string, opts *ClientOptions) (*Client, error)
```

**Migration Example:**
```go
// Before
client, err := NewClientWithCredentials(
    "https://api.vpsie.com/v2",
    "long-lived-token",
    nil,
)

// After
client, err := NewClientWithCredentials(
    "https://api.vpsie.com/v2",
    "your-client-id",
    "your-client-secret",
    nil,
)
```

### New Methods

```go
// refreshToken obtains a new access token using client credentials
func (c *Client) refreshToken(ctx context.Context) error

// ensureValidToken checks if the token is still valid and refreshes if needed
func (c *Client) ensureValidToken(ctx context.Context) error
```

These methods are called automatically - you don't need to invoke them manually.

## Token Lifecycle

1. **Initial Token**: Obtained when creating the client (`NewClient` or `NewClientWithCredentials`)
2. **Proactive Refresh**: Before each API request, the client checks if the token expires within 5 minutes
3. **Reactive Refresh**: If an API request returns 401 Unauthorized, the client refreshes the token and retries once
4. **Concurrent Safety**: All token operations are protected by mutex locks

## VPSie Authentication Request

**Endpoint:** `POST /auth/from/api`

**Request Body (form-urlencoded):**
```
clientId=your-client-id
clientSecret=your-client-secret
```

**Response:**
```json
{
  "accessToken": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires": "2025-10-12T11:00:00Z"
  },
  "refreshToken": {
    "token": "refresh-token-value",
    "expires": "2025-10-13T10:00:00Z"
  }
}
```

**Note:** VPSie uses a custom authentication format, not standard OAuth 2.0.

## Error Handling

### Token Acquisition Failures

If the initial token request fails, client creation will fail:

```go
client, err := NewClient(ctx, k8sClient, nil)
if err != nil {
    // Could be:
    // - Secret not found
    // - Missing clientId or clientSecret keys
    // - OAuth endpoint unreachable
    // - Invalid credentials
    log.Fatalf("Failed to create client: %v", err)
}
```

### Token Refresh Failures

If a token refresh fails during operation:

```go
vms, err := client.ListVMs(ctx)
if err != nil {
    // Could be:
    // - Token refresh failed (network issue, invalid credentials)
    // - API request failed after successful refresh
    // - Other API errors
    log.Printf("API call failed: %v", err)
}
```

## Testing

### Test Changes Required

All tests using `NewClientWithCredentials` must be updated:

```go
// Before
client, err := NewClientWithCredentials(server.URL, "test-token", nil)

// After
client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
```

### Mock VPSie Auth Server

For testing, you'll need to mock the VPSie authentication endpoint:

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path == "/auth/from/api" {
        w.Header().Set("Content-Type", "application/json")
        expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
        json.NewEncoder(w).Encode(TokenResponse{
            AccessToken: AccessTokenInfo{
                Token:   "mock-access-token",
                Expires: expiresAt,
            },
            RefreshToken: RefreshTokenInfo{
                Token:   "mock-refresh-token",
                Expires: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
            },
        })
        return
    }
    // Handle other API endpoints...
}))
```

## Deployment Checklist

- [ ] Update Kubernetes secret with `clientId` and `clientSecret` keys
- [ ] Remove old `token` key from secret (it's no longer used)
- [ ] Verify VPSie auth endpoint (`/auth/from/api`) is accessible from your cluster
- [ ] Update any tests that create clients
- [ ] Review logs for token refresh activity (debug level shows token refreshes)
- [ ] Monitor for authentication errors after deployment

## Troubleshooting

### "secret does not contain 'clientId' key"

**Cause**: Secret still uses old format with `token` key

**Fix**: Update secret to use `clientId` and `clientSecret` keys

### "failed to obtain initial access token"

**Causes**:
- VPSie auth endpoint `/auth/from/api` is unreachable
- Invalid client credentials
- Network connectivity issues

**Debug**: Enable debug logging to see the full token request

### "token request failed with status 401"

**Cause**: Invalid `clientId` or `clientSecret`

**Fix**: Verify credentials with VPSie support

### Frequent Token Refreshes

**Expected Behavior**: Tokens are refreshed ~5 minutes before expiration

**If Excessive**: Check if `expires_in` from VPSie is too short

## Benefits of Token-Based Authentication

1. **Security**: Short-lived tokens reduce the impact of token theft
2. **Revocability**: Credentials can be revoked without changing application config
3. **Auditability**: Each token request can be logged and monitored
4. **Compliance**: Meets modern security standards for API authentication
5. **Automatic Rotation**: No need for manual token rotation
6. **Reduced Attack Surface**: Client credentials are only used for authentication, not API calls

## Backwards Compatibility

⚠️ **This is a BREAKING CHANGE**. The old long-lived token format is no longer supported.

If you need to support both formats during migration, you would need to:
1. Check for `clientId` key first
2. Fall back to `token` key if `clientId` is missing
3. Use appropriate authentication method based on what's available

This is not implemented in the current version to keep the code simple and encourage migration to the new VPSie authentication method.
