# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /workspace

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETARCH

# Build the controller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -a -installsuffix cgo \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -o vpsie-autoscaler \
    ./cmd/controller

# Runtime stage - use distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from builder
COPY --from=builder /workspace/vpsie-autoscaler .

# Use non-root user
USER 65532:65532

# Labels for metadata
LABEL org.opencontainers.image.title="VPSie Kubernetes Autoscaler"
LABEL org.opencontainers.image.description="Event-driven Kubernetes node autoscaler for VPSie cloud platform"
LABEL org.opencontainers.image.vendor="VPSie"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/vpsie/vpsie-k8s-autoscaler"
LABEL org.opencontainers.image.documentation="https://github.com/vpsie/vpsie-k8s-autoscaler/blob/main/README.md"

ENTRYPOINT ["/vpsie-autoscaler"]
