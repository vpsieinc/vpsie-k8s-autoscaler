#!/bin/bash
# restore-crds.sh - Restore VPSie Autoscaler CRDs and resources from backup
#
# This script restores NodeGroup and VPSieNode resources from a backup
# created by backup-crds.sh.
#
# Usage: ./restore-crds.sh [OPTIONS] <backup-path>
#
# Arguments:
#   backup-path               Path to backup directory or .tar.gz archive
#
# Options:
#   -n, --namespace NAMESPACE   Restore only to specified namespace (override)
#   -c, --crds-only             Only restore CRD definitions, not resources
#   -r, --resources-only        Only restore resources, not CRDs
#   -s, --include-secrets       Restore vpsie-secret (CAUTION: will overwrite!)
#   -d, --dry-run               Show what would be restored without applying
#   -f, --force                 Skip confirmation prompts
#   -q, --quiet                 Suppress non-error output
#   -h, --help                  Show this help message
#
# Example:
#   ./restore-crds.sh ./backups/20240115-120000
#   ./restore-crds.sh -d vpsie-autoscaler-backup-20240115-120000.tar.gz
#   ./restore-crds.sh -n production -r ./backups/20240115-120000

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
NAMESPACE=""
CRDS_ONLY=false
RESOURCES_ONLY=false
INCLUDE_SECRETS=false
DRY_RUN=false
FORCE=false
QUIET=false
BACKUP_PATH=""

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

log_info() {
    log "${BLUE}$1${NC}"
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
        -c|--crds-only)
            CRDS_ONLY=true
            shift
            ;;
        -r|--resources-only)
            RESOURCES_ONLY=true
            shift
            ;;
        -s|--include-secrets)
            INCLUDE_SECRETS=true
            shift
            ;;
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -f|--force)
            FORCE=true
            shift
            ;;
        -q|--quiet)
            QUIET=true
            shift
            ;;
        -h|--help)
            show_help
            ;;
        -*)
            log_error "Unknown option: $1"
            show_help
            ;;
        *)
            if [[ -z "$BACKUP_PATH" ]]; then
                BACKUP_PATH="$1"
            else
                log_error "Multiple backup paths specified"
                exit 1
            fi
            shift
            ;;
    esac
done

# Validate arguments
if [[ -z "$BACKUP_PATH" ]]; then
    log_error "Backup path is required"
    show_help
fi

if [[ "$CRDS_ONLY" == "true" && "$RESOURCES_ONLY" == "true" ]]; then
    log_error "Cannot specify both --crds-only and --resources-only"
    exit 1
fi

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

# Handle archive extraction
WORK_DIR=""
CLEANUP_WORK_DIR=false

if [[ "$BACKUP_PATH" == *.tar.gz || "$BACKUP_PATH" == *.tgz ]]; then
    if [[ ! -f "$BACKUP_PATH" ]]; then
        log_error "Archive not found: $BACKUP_PATH"
        exit 1
    fi

    WORK_DIR=$(mktemp -d)
    CLEANUP_WORK_DIR=true
    log "Extracting archive to temporary directory..."
    tar -xzf "$BACKUP_PATH" -C "$WORK_DIR"

    # Find the extracted directory
    EXTRACTED_DIR=$(find "$WORK_DIR" -mindepth 1 -maxdepth 1 -type d | head -1)
    if [[ -z "$EXTRACTED_DIR" ]]; then
        log_error "No directory found in archive"
        rm -rf "$WORK_DIR"
        exit 1
    fi
    BACKUP_PATH="$EXTRACTED_DIR"
fi

# Cleanup function
cleanup() {
    if [[ "$CLEANUP_WORK_DIR" == "true" && -n "$WORK_DIR" ]]; then
        rm -rf "$WORK_DIR"
    fi
}
trap cleanup EXIT

# Validate backup directory
if [[ ! -d "$BACKUP_PATH" ]]; then
    log_error "Backup directory not found: $BACKUP_PATH"
    exit 1
fi

# Check for metadata
METADATA_FILE="$BACKUP_PATH/backup-metadata.json"
if [[ -f "$METADATA_FILE" ]]; then
    log "\nBackup metadata:"
    log "  Timestamp: $(jq -r '.timestamp' "$METADATA_FILE" 2>/dev/null || echo 'unknown')"
    log "  Source cluster: $(jq -r '.cluster' "$METADATA_FILE" 2>/dev/null || echo 'unknown')"
    log "  Original namespace: $(jq -r '.namespace' "$METADATA_FILE" 2>/dev/null || echo 'unknown')"
    log "  Includes secrets: $(jq -r '.include_secrets' "$METADATA_FILE" 2>/dev/null || echo 'unknown')"
fi

# Current cluster context
CURRENT_CONTEXT=$(kubectl config current-context)
log "\nTarget cluster: $CURRENT_CONTEXT"

if [[ -n "$NAMESPACE" ]]; then
    log "Target namespace (override): $NAMESPACE"
fi

if [[ "$DRY_RUN" == "true" ]]; then
    log_info "\n=== DRY RUN MODE - No changes will be applied ==="
fi

# Confirmation prompt
if [[ "$FORCE" == "false" && "$DRY_RUN" == "false" ]]; then
    echo ""
    read -p "Do you want to proceed with the restore? (yes/no): " CONFIRM
    if [[ "$CONFIRM" != "yes" ]]; then
        log "Restore cancelled."
        exit 0
    fi
fi

log "\n=========================================="
log "Starting VPSie Autoscaler restore..."
log "==========================================\n"

KUBECTL_CMD="kubectl"
if [[ "$DRY_RUN" == "true" ]]; then
    KUBECTL_CMD="kubectl --dry-run=client"
fi

RESTORE_COUNT=0
SKIP_COUNT=0
ERROR_COUNT=0

# Restore CRDs first
if [[ "$RESOURCES_ONLY" != "true" ]]; then
    log "Restoring CRD definitions..."
    CRD_DIR="$BACKUP_PATH/crds"

    if [[ -d "$CRD_DIR" ]]; then
        for CRD_FILE in "$CRD_DIR"/*.yaml; do
            if [[ -f "$CRD_FILE" ]]; then
                CRD_NAME=$(basename "$CRD_FILE" .yaml)
                log "  Restoring CRD: $CRD_NAME"

                # Remove status and metadata that shouldn't be restored
                CLEAN_CRD=$(mktemp)
                # Remove resourceVersion, uid, creationTimestamp, generation
                sed -E \
                    -e '/^  resourceVersion:/d' \
                    -e '/^  uid:/d' \
                    -e '/^  creationTimestamp:/d' \
                    -e '/^  generation:/d' \
                    -e '/^status:/,/^[a-z]/{ /^status:/d; /^  /d; }' \
                    "$CRD_FILE" > "$CLEAN_CRD"

                if $KUBECTL_CMD apply -f "$CLEAN_CRD" 2>/dev/null; then
                    log_success "    Applied: $CRD_NAME"
                    ((RESTORE_COUNT++))
                else
                    log_error "    Failed to apply: $CRD_NAME"
                    ((ERROR_COUNT++))
                fi
                rm -f "$CLEAN_CRD"
            fi
        done
    else
        log_warning "  No CRD backups found"
    fi

    # Wait for CRDs to be established
    if [[ "$DRY_RUN" == "false" ]]; then
        log "\nWaiting for CRDs to be established..."
        sleep 2
        for CRD in nodegroups.autoscaler.vpsie.com vpsienodes.autoscaler.vpsie.com; do
            kubectl wait --for=condition=Established crd/"$CRD" --timeout=30s 2>/dev/null || true
        done
    fi
fi

# Restore NodeGroups
if [[ "$CRDS_ONLY" != "true" ]]; then
    log "\nRestoring NodeGroups..."
    NODEGROUPS_FILE="$BACKUP_PATH/nodegroups.yaml"

    if [[ -f "$NODEGROUPS_FILE" ]]; then
        # Process the file to clean metadata and optionally change namespace
        CLEAN_FILE=$(mktemp)

        if [[ -n "$NAMESPACE" ]]; then
            # Replace namespace in the YAML
            sed -E \
                -e "s/^  namespace: .*/  namespace: $NAMESPACE/" \
                -e '/^    resourceVersion:/d' \
                -e '/^    uid:/d' \
                -e '/^    creationTimestamp:/d' \
                -e '/^    generation:/d' \
                -e '/^  status:/,/^  [a-z]/{d}' \
                "$NODEGROUPS_FILE" > "$CLEAN_FILE"
        else
            sed -E \
                -e '/^    resourceVersion:/d' \
                -e '/^    uid:/d' \
                -e '/^    creationTimestamp:/d' \
                -e '/^    generation:/d' \
                -e '/^  status:/,/^  [a-z]/{d}' \
                "$NODEGROUPS_FILE" > "$CLEAN_FILE"
        fi

        if $KUBECTL_CMD apply -f "$CLEAN_FILE" 2>/dev/null; then
            COUNT=$(grep -c "kind: NodeGroup" "$CLEAN_FILE" 2>/dev/null || echo "0")
            log_success "  Restored $COUNT NodeGroup(s)"
            ((RESTORE_COUNT++))
        else
            log_error "  Failed to restore NodeGroups"
            ((ERROR_COUNT++))
        fi
        rm -f "$CLEAN_FILE"
    else
        log_warning "  No NodeGroup backup found"
        ((SKIP_COUNT++))
    fi

    # Restore VPSieNodes
    log "\nRestoring VPSieNodes..."
    VPSIENODES_FILE="$BACKUP_PATH/vpsienodes.yaml"

    if [[ -f "$VPSIENODES_FILE" ]]; then
        CLEAN_FILE=$(mktemp)

        if [[ -n "$NAMESPACE" ]]; then
            sed -E \
                -e "s/^  namespace: .*/  namespace: $NAMESPACE/" \
                -e '/^    resourceVersion:/d' \
                -e '/^    uid:/d' \
                -e '/^    creationTimestamp:/d' \
                -e '/^    generation:/d' \
                -e '/^  status:/,/^  [a-z]/{d}' \
                "$VPSIENODES_FILE" > "$CLEAN_FILE"
        else
            sed -E \
                -e '/^    resourceVersion:/d' \
                -e '/^    uid:/d' \
                -e '/^    creationTimestamp:/d' \
                -e '/^    generation:/d' \
                -e '/^  status:/,/^  [a-z]/{d}' \
                "$VPSIENODES_FILE" > "$CLEAN_FILE"
        fi

        if $KUBECTL_CMD apply -f "$CLEAN_FILE" 2>/dev/null; then
            COUNT=$(grep -c "kind: VPSieNode" "$CLEAN_FILE" 2>/dev/null || echo "0")
            log_success "  Restored $COUNT VPSieNode(s)"
            ((RESTORE_COUNT++))
        else
            log_error "  Failed to restore VPSieNodes"
            ((ERROR_COUNT++))
        fi
        rm -f "$CLEAN_FILE"
    else
        log_warning "  No VPSieNode backup found"
        ((SKIP_COUNT++))
    fi
fi

# Restore secrets if requested
if [[ "$INCLUDE_SECRETS" == "true" ]]; then
    log "\nRestoring secrets..."
    SECRETS_FILE="$BACKUP_PATH/secrets.yaml"

    if [[ -f "$SECRETS_FILE" ]]; then
        log_warning "  WARNING: This will overwrite existing vpsie-secret!"

        CLEAN_FILE=$(mktemp)
        TARGET_NS="${NAMESPACE:-kube-system}"

        sed -E \
            -e "s/^  namespace: .*/  namespace: $TARGET_NS/" \
            -e '/^  resourceVersion:/d' \
            -e '/^  uid:/d' \
            -e '/^  creationTimestamp:/d' \
            "$SECRETS_FILE" > "$CLEAN_FILE"

        if $KUBECTL_CMD apply -f "$CLEAN_FILE" 2>/dev/null; then
            log_success "  Restored vpsie-secret to namespace: $TARGET_NS"
            ((RESTORE_COUNT++))
        else
            log_error "  Failed to restore secret"
            ((ERROR_COUNT++))
        fi
        rm -f "$CLEAN_FILE"
    else
        log_warning "  No secrets backup found"
        ((SKIP_COUNT++))
    fi
fi

# Summary
log "\n=========================================="
if [[ "$DRY_RUN" == "true" ]]; then
    log_info "DRY RUN COMPLETE"
else
    if [[ "$ERROR_COUNT" -eq 0 ]]; then
        log_success "Restore completed successfully!"
    else
        log_warning "Restore completed with errors"
    fi
fi
log "==========================================\n"

log "Summary:"
log "  Resources restored: $RESTORE_COUNT"
log "  Resources skipped: $SKIP_COUNT"
log "  Errors: $ERROR_COUNT"

if [[ "$DRY_RUN" == "false" ]]; then
    log "\nVerifying restored resources..."

    if [[ "$RESOURCES_ONLY" != "true" ]]; then
        log "\nCRDs:"
        kubectl get crd | grep -E "autoscaler.vpsie.com|NAME" || true
    fi

    if [[ "$CRDS_ONLY" != "true" ]]; then
        NS_FLAG=""
        if [[ -n "$NAMESPACE" ]]; then
            NS_FLAG="-n $NAMESPACE"
        else
            NS_FLAG="--all-namespaces"
        fi

        log "\nNodeGroups:"
        kubectl get nodegroups.autoscaler.vpsie.com $NS_FLAG 2>/dev/null || log "  None found"

        log "\nVPSieNodes:"
        kubectl get vpsienodes.autoscaler.vpsie.com $NS_FLAG 2>/dev/null || log "  None found"
    fi
fi

exit $ERROR_COUNT
