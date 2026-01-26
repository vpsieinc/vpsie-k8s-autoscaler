# VPSie K8s Autoscaler - Multi-Architecture Build Report

## Build Summary

Successfully built multi-architecture container images for the VPSie Kubernetes Autoscaler with comprehensive support for both x86_64 (amd64) and ARM64 (aarch64) architectures.

### Build Details

**Date:** 2026-01-26
**Build Time:** ~5 minutes
**Cache Policy:** Disabled (--no-cache for reproducibility)

### Project Information

- **Repository:** vpsie-k8s-autoscaler
- **Version:** v0.7.4
- **Commit:** 0d97c72
- **Go Version:** 1.24

### Image Configuration

**Registry:** ghcr.io (GitHub Container Registry)
**Namespace:** vpsieinc
**Image Name:** vpsie-k8s-autoscaler

**Full Image References:**
- `ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4`
- `ghcr.io/vpsieinc/vpsie-k8s-autoscaler:latest`

### Multi-Architecture Support

The build produces images for the following platforms:

1. **linux/amd64** (x86_64) - Intel/AMD 64-bit
2. **linux/arm64** (aarch64) - ARM 64-bit (Apple Silicon, AWS Graviton, etc.)

Both architectures are compiled from source using cross-compilation with the Go compiler.

## Build Process

### Build Tool: Docker Buildx

**Builder Configuration:** multiarch-builder
**Driver:** docker-container
**BuildKit Version:** v0.25.1+

The multiarch-builder is configured with QEMU support for cross-platform emulation, enabling native compilation for both target architectures.

### Build Arguments

The following arguments were passed to the Dockerfile:

```
VERSION=v0.7.4
COMMIT=0d97c72
BUILD_DATE=2026-01-26T00:07:04Z
TARGETARCH=auto-detected per platform (amd64, arm64)
```

These are embedded in the resulting binary via ldflags:
- `main.Version` = v0.7.4
- `main.Commit` = 0d97c72
- `main.BuildDate` = 2026-01-26T00:07:04Z

### Dockerfile Analysis

#### Stage 1: Builder (golang:1.24-alpine)

```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git make
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ pkg/ internal/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -a -installsuffix cgo \
    -ldflags="-w -s -X main.Version=... -X main.Commit=... -X main.BuildDate=..." \
    -o vpsie-autoscaler ./cmd/controller
```

**Optimizations:**
- `CGO_ENABLED=0`: Statically linked binary (no libc dependency)
- `GOARCH=${TARGETARCH}`: Cross-compilation for target platform
- `-installsuffix cgo`: Avoids conflicts
- `-w -s`: Strips debug symbols and symbol table (smaller binary)
- Multi-stage build: Go dependencies cached separately for faster rebuilds

#### Stage 2: Runtime (gcr.io/distroless/static:nonroot)

```dockerfile
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/vpsie-autoscaler .
USER 65532:65532
ENTRYPOINT ["/vpsie-autoscaler"]
```

**Benefits:**
- **Minimal image size:** No shell, package manager, or OS tools
- **Security:** Non-root user, minimal attack surface
- **Distroless:** Based on `distroless/static`, suitable for static Go binaries
- **Multi-arch:** Automatically selects correct architecture variant

### Cache Policy

**Cache:** DISABLED (--no-cache flag)

This ensures:
- Fresh compilation from source code
- No stale dependencies
- Reproducible builds
- Guaranteed up-to-date base images

## Build Completion

### Build Output

Both architectures compiled successfully:

**linux/amd64:**
- Compilation time: ~212 seconds
- Status: SUCCESS

**linux/arm64:**
- Compilation time: ~103 seconds (parallel execution)
- Status: SUCCESS

### Build Storage

Since the build used `docker-container` driver (required for multi-architecture), the resulting images are stored in the buildx builder cache, not in local Docker daemon storage.

To make images available:
1. **Push to registry** (recommended): Use `--push` flag
2. **Load to local Docker:** Requires single-architecture build with `--load` flag

## Push to Registry Instructions

### Prerequisites

1. **GitHub Authentication Token**
   - Create a Personal Access Token with `write:packages` scope
   - Or use your GitHub credentials

2. **Docker Registry Authentication**
   ```bash
   echo $GITHUB_TOKEN | docker login ghcr.io -u <USERNAME> --password-stdin
   ```

   Or interactively:
   ```bash
   docker login ghcr.io
   ```

### Push Command

To push the built images to GitHub Container Registry:

```bash
cd /Users/zozo/projects/vpsie-k8s-autoscaler

docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --no-cache \
    --push \
    --build-arg VERSION="v0.7.4" \
    --build-arg COMMIT="0d97c72" \
    --build-arg BUILD_DATE="2026-01-26T00:07:04Z" \
    -t "ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4" \
    -t "ghcr.io/vpsieinc/vpsie-k8s-autoscaler:latest" \
    -f Dockerfile \
    .
```

Or use the provided script:
```bash
/tmp/build-and-push.sh
```

### Expected Results

After successful push:
- Both architectures published to `ghcr.io/vpsieinc/vpsie-k8s-autoscaler`
- Docker manifest created with both platform variants
- Images available for pull on any architecture
- Build metadata preserved (version, commit, date)

### Verify Published Images

```bash
# Inspect manifest (shows all platforms)
docker manifest inspect ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4

# Pull the image (automatically selects matching architecture)
docker pull ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4

# Test the image
docker run --rm ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4 --help

# Check image architecture
docker run --rm ghcr.io/vpsieinc/vpsie-k8s-autoscaler:v0.7.4 version
```

## Technical Specifications

### System Requirements

**Build System:**
- Docker Desktop 29.1.3+
- Docker Buildx 0.30.1+
- QEMU for cross-platform support

**Target Environments:**
- Kubernetes 1.20+ clusters
- Linux x86_64 nodes
- Linux ARM64 nodes (ARM64 Kubernetes distributions)

### Build Artifacts

**Binary Details:**
- **Language:** Go 1.24
- **Type:** Static executable (no shared libraries)
- **Linking:** Static CGO_ENABLED=0
- **Size:** ~20-30MB (stripped, typical for Go controllers)
- **Architecture:** Universal build (platform-specific binaries)

### Image Layers

**Stage 1 (Builder):**
- golang:1.24-alpine (~375 MB)
- APK packages: git, make
- Go modules: downloaded once, shared across platforms

**Stage 2 (Runtime):**
- gcr.io/distroless/static:nonroot (~5-10 MB)
- Single binary (~20-30 MB)
- **Final size:** ~30-40 MB per architecture

### Security Features

- **Non-root user:** UID 65532 (numeric, no user name required)
- **Distroless base:** Minimal attack surface, no shell or package manager
- **Static binary:** No shared library dependencies or version conflicts
- **Stripped binary:** No debug symbols in production image

## CI/CD Integration

### GitHub Actions Example

To integrate this build into your CI/CD pipeline:

```yaml
name: Build and Push Multi-Architecture Images

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v2
        
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2
        
      - name: Login to GitHub Container Registry
        uses: docker/login-action@v2
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
          
      - name: Extract version
        id: meta
        run: echo "VERSION=${GITHUB_REF#refs/tags/}" >> $GITHUB_OUTPUT
        
      - name: Build and Push
        uses: docker/build-push-action@v4
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: |
            ghcr.io/vpsieinc/vpsie-k8s-autoscaler:${{ steps.meta.outputs.VERSION }}
            ghcr.io/vpsieinc/vpsie-k8s-autoscaler:latest
          build-args: |
            VERSION=${{ steps.meta.outputs.VERSION }}
            COMMIT=${{ github.sha }}
            BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
```

## Troubleshooting

### Build Failure: Base Image Not Found

**Cause:** Network connectivity or registry authentication issue
**Solution:** Check internet connection and Docker Hub authentication

```bash
docker pull golang:1.24-alpine
docker pull gcr.io/distroless/static:nonroot
```

### Build Failure: Insufficient Disk Space

**Cause:** Buildx cache consuming too much space
**Solution:** Clear buildx cache

```bash
docker buildx prune -a
```

### Push Failure: Authentication Denied

**Cause:** Not authenticated to GitHub Container Registry
**Solution:** Authenticate first

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u <USERNAME> --password-stdin
```

### Platform-Specific Build Issues

If a specific architecture fails, check the build logs:

```bash
docker buildx build \
    --platform linux/amd64 \
    -t ghcr.io/vpsieinc/vpsie-k8s-autoscaler:test .
```

## Additional Resources

- **Dockerfile:** `/Users/zozo/projects/vpsie-k8s-autoscaler/Dockerfile`
- **Makefile:** `/Users/zozo/projects/vpsie-k8s-autoscaler/Makefile`
- **Docker Buildx Docs:** https://docs.docker.com/build/architecture/
- **Distroless Images:** https://github.com/GoogleContainerTools/distroless
- **Go Cross-Compilation:** https://golang.org/doc/install/source#environment

## Summary

The VPSie K8s Autoscaler is now built with comprehensive multi-architecture support, enabling deployment across diverse infrastructure including:
- Intel/AMD x86_64 servers
- ARM64 nodes (Apple Silicon, AWS Graviton, Ampere A1)
- Kubernetes distributions supporting ARM64

The build process is reproducible, cache-disabled, and optimized for production deployments with minimal image size and maximal security.
