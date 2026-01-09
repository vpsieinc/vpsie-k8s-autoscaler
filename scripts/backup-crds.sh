#!/bin/bash
# backup-crds.sh - Backup VPSie Autoscaler CRDs and resources
#
# This script creates a backup of all NodeGroup and VPSieNode resources
# along with their associated secrets and configurations.
#
# Usage: ./backup-crds.sh [OPTIONS]
#
# Options:
#   -n, --namespace NAMESPACE   Backup only resources in specified namespace
#   -o, --output DIR            Output directory (default: ./backups/$(date +%Y%m%d-%H%M%S))
#   -s, --include-secrets       Include vpsie-secret in backup (CAUTION: contains credentials)
#   -q, --quiet                 Suppress non-error output
#   -h, --help                  Show this help message
#
# Example:
#   ./backup-crds.sh -n production -o ./backups/prod -s
#   ./backup-crds.sh --namespace default

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
NAMESPACE=""
OUTPUT_DIR="./backups/$(date +%Y%m%d-%H%M%S)"
INCLUDE_SECRETS=false
QUIET=false

# Function to print messages
log() {
    if [[ "$QUIET" == "false" ]]; then
        echo -e "$1"
    fi
}

log_error() {
    echo -e "${RED}ERROR: $1${NC}" >&2
}

log_success() {
    log "${GREEN}$1${NC}"
}

log_warning() {
    log "${YELLOW}WARNING: $1${NC}"
}

# Function to show help
show_help() {
    head -30 "$0" | grep "^#" | sed 's/^# //' | sed 's/^#//'
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -s|--include-secrets)
            INCLUDE_SECRETS=true
            shift
            ;;
        -q|--quiet)
            QUIET=true
            shift
            ;;
        -h|--help)
            show_help
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            ;;
    esac
done

# Check kubectl is available
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl not found. Please install kubectl and configure cluster access."
    exit 1
fi

# Check cluster connectivity
if ! kubectl cluster-info &> /dev/null; then
    log_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

log "Starting VPSie Autoscaler backup..."
log "Output directory: $OUTPUT_DIR"

# Build namespace selector
NS_SELECTOR=""
if [[ -n "$NAMESPACE" ]]; then
    NS_SELECTOR="-n $NAMESPACE"
    log "Filtering by namespace: $NAMESPACE"
else
    NS_SELECTOR="--all-namespaces"
    log "Backing up resources from all namespaces"
fi

# Backup NodeGroups
log "\nBacking up NodeGroups..."
NODEGROUPS_FILE="$OUTPUT_DIR/nodegroups.yaml"
if kubectl get nodegroups.autoscaler.vpsie.com $NS_SELECTOR -o yaml > "$NODEGROUPS_FILE" 2>/dev/null; then
    COUNT=$(kubectl get nodegroups.autoscaler.vpsie.com $NS_SELECTOR --no-headers 2>/dev/null | wc -l | tr -d ' ')
    log_success "  Backed up $COUNT NodeGroup(s) to nodegroups.yaml"
else
    log_warning "  No NodeGroups found or CRD not installed"
    rm -f "$NODEGROUPS_FILE"
fi

# Backup VPSieNodes
log "\nBacking up VPSieNodes..."
VPSIENODES_FILE="$OUTPUT_DIR/vpsienodes.yaml"
if kubectl get vpsienodes.autoscaler.vpsie.com $NS_SELECTOR -o yaml > "$VPSIENODES_FILE" 2>/dev/null; then
    COUNT=$(kubectl get vpsienodes.autoscaler.vpsie.com $NS_SELECTOR --no-headers 2>/dev/null | wc -l | tr -d ' ')
    log_success "  Backed up $COUNT VPSieNode(s) to vpsienodes.yaml"
else
    log_warning "  No VPSieNodes found or CRD not installed"
    rm -f "$VPSIENODES_FILE"
fi

# Backup CRD definitions
log "\nBacking up CRD definitions..."
CRD_DIR="$OUTPUT_DIR/crds"
mkdir -p "$CRD_DIR"

for CRD in nodegroups.autoscaler.vpsie.com vpsienodes.autoscaler.vpsie.com; do
    CRD_FILE="$CRD_DIR/${CRD}.yaml"
    if kubectl get crd "$CRD" -o yaml > "$CRD_FILE" 2>/dev/null; then
        log_success "  Backed up CRD: $CRD"
    else
        log_warning "  CRD not found: $CRD"
        rm -f "$CRD_FILE"
    fi
done

# Optionally backup secrets
if [[ "$INCLUDE_SECRETS" == "true" ]]; then
    log "\nBacking up VPSie secrets..."
    log_warning "  WARNING: Secrets contain sensitive credentials!"

    SECRETS_FILE="$OUTPUT_DIR/secrets.yaml"

    # Backup vpsie-secret from kube-system (or specified namespace)
    SECRET_NS="${NAMESPACE:-kube-system}"
    if kubectl get secret vpsie-secret -n "$SECRET_NS" -o yaml > "$SECRETS_FILE" 2>/dev/null; then
        log_success "  Backed up vpsie-secret from namespace: $SECRET_NS"
        # Encrypt or secure the file
        chmod 600 "$SECRETS_FILE"
    else
        log_warning "  vpsie-secret not found in namespace: $SECRET_NS"
        rm -f "$SECRETS_FILE"
    fi
fi

# Create backup metadata
METADATA_FILE="$OUTPUT_DIR/backup-metadata.json"
cat > "$METADATA_FILE" << EOF
{
    "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
    "namespace": "${NAMESPACE:-all}",
    "include_secrets": $INCLUDE_SECRETS,
    "cluster": "$(kubectl config current-context)",
    "kubernetes_version": "$(kubectl version --short --client 2>/dev/null | head -1 || echo 'unknown')",
    "files": [
$(ls -1 "$OUTPUT_DIR" | grep -v backup-metadata.json | sed 's/^/        "/' | sed 's/$/"/' | paste -sd ',' -)
    ]
}
EOF

# Create a tarball
BACKUP_ARCHIVE="$OUTPUT_DIR/../vpsie-autoscaler-backup-$(date +%Y%m%d-%H%M%S).tar.gz"
tar -czf "$BACKUP_ARCHIVE" -C "$(dirname "$OUTPUT_DIR")" "$(basename "$OUTPUT_DIR")"

log "\n=========================================="
log_success "Backup completed successfully!"
log "=========================================="
log "Output directory: $OUTPUT_DIR"
log "Archive file: $BACKUP_ARCHIVE"
log ""
log "Contents:"
ls -la "$OUTPUT_DIR"

if [[ "$INCLUDE_SECRETS" == "true" ]]; then
    log ""
    log_warning "IMPORTANT: The backup contains sensitive credentials."
    log_warning "Store the backup securely and consider encrypting it."
fi
