#!/usr/bin/env bash
# deploy-to-testing.sh — Deploy patched images to testing
#
# Model:
#   Testing supports multiple concurrent versions of the same image
#   (for single-service health checks). Always deploy patched versions
#   of production images to testing immediately.
#
#   The same version should never be in both test and prod.
#   If prod has `1.0`, testing gets `1.0-patched-*`, not `1.0`.
#
# Logic:
#   For each service in gitops:
#     1. What's in production?
#     2. Does a patched version exist in the registry?
#     3. If patched exists → DEPLOY to testing
#     4. If no patched version → FLAG (need to patch first)
#
# Usage:
#   ./scripts/deploy-to-testing.sh [OPTIONS]
#
# Options:
#   --registry <url>     Registry URL (default: localhost:5000)
#   --gitops <dir>       Gitops repo path (default: gitops/)
#   --dry-run            Show decisions without updating gitops
#   --help               Show this help

set -euo pipefail

# Defaults
REGISTRY="localhost:5000"
GITOPS_DIR="gitops"
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --registry)
      REGISTRY="$2"
      shift 2
      ;;
    --gitops)
      GITOPS_DIR="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --help)
      grep '^#' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_action(){ echo -e "${CYAN}[ACTION]${NC} $*"; }

# Extract image info from a values file
# Returns: repository tag digest
extract_image() {
  local values_file="$1"
  
  if [[ ! -f "$values_file" ]]; then
    return 1
  fi
  
  # Repository from file (may be empty if not in this file)
  local repo=""
  repo=$(grep 'repository:' "$values_file" 2>/dev/null | sed 's/.*repository: *//' | tr -d '"' | tr -d "'" || true)
  
  # Tag and digest
  local tag digest
  tag=$(grep 'tag:' "$values_file" 2>/dev/null | head -1 | sed 's/.*tag: *//' | tr -d '"' | tr -d "'" || true)
  digest=$(grep 'digest:' "$values_file" 2>/dev/null | head -1 | sed 's/.*digest: *//' | tr -d '"' | tr -d "'" || true)
  
  # Strip comments and whitespace
  tag="${tag%%#*}"
  digest="${digest%%#*}"
  repo=$(echo "$repo" | xargs)
  tag=$(echo "$tag" | xargs)
  digest=$(echo "$digest" | xargs)
  
  # Always output 3 fields (use placeholder for empty repo)
  echo "${repo:-_} ${tag:-_} ${digest:-_}"
}

# Strip registry URL from repo path
# e.g., "localhost:5000/test/python-slim" → "test/python-slim"
strip_registry() {
  local repo="$1"
  echo "${repo#*/}"
}

# List all tags for a repository
list_tags() {
  local repo="$1"
  local clean_repo
  clean_repo=$(strip_registry "$repo")
  curl -s "http://${REGISTRY}/v2/${clean_repo}/tags/list" 2>/dev/null | jq -r '.tags[] // empty' 2>/dev/null || true
}

# Find the latest patched version of a tag
latest_patched_tag() {
  local repo="$1"
  local tag="$2"
  
  # || true handles case where grep finds no matches (exit code 1 with set -e)
  list_tags "$repo" | grep "^${tag}-patched-" | sort -t- -k3 -r | head -1 || true
}

# Get image digest from registry
get_digest() {
  local repo="$1"
  local tag="$2"
  
  local clean_repo
  clean_repo=$(strip_registry "$repo")
  
  curl -sI "http://${REGISTRY}/v2/${clean_repo}/manifests/${tag}" \
    -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
    -H "Accept: application/vnd.oci.image.manifest.v1+json" \
    2>/dev/null | grep -i 'docker-content-digest' | awk '{print $2}' | tr -d '\r' || true
}

# Strip the repository path to get just the image name
# e.g., "test/python-slim" → "python-slim"
image_name() {
  local repo="$1"
  echo "${repo##*/}"
}

# Create a new test deployment values file
create_test_deployment() {
  local testing_dir="$1"
  local new_tag="$2"
  local new_digest="$3"
  
  local values_file="$testing_dir/${new_tag}.yaml"
  
  if $DRY_RUN; then
    return 0
  fi
  
  mkdir -p "$testing_dir"
  
  cat > "$values_file" << EOF
# Testing deployment for ${new_tag}
# Created: $(date -u +%Y-%m-%dT%H:%M:%SZ)
# Single-service health checks — multiple versions can run concurrently

image:
  tag: "${new_tag}"
  digest: "${new_digest}"

replicaCount: 1

# Testing health checks — aggressive, fail fast
healthCheck:
  livenessProbe:
    httpGet:
      path: /healthz
      port: 8080
    initialDelaySeconds: 5
    periodSeconds: 5
    failureThreshold: 2
  readinessProbe:
    httpGet:
      path: /ready
      port: 8080
    initialDelaySeconds: 3
    periodSeconds: 3
    failureThreshold: 2
EOF
}

# List all test deployments for a service
list_test_deployments() {
  local testing_dir="$1"
  
  if [[ ! -d "$testing_dir" ]]; then
    return 0
  fi
  
  ls "$testing_dir"/*.yaml 2>/dev/null | xargs -rn1 basename | sed 's/\.yaml$//' || true
}

# Main
main() {
  log_info "=== Deploy to Testing Decision Engine ==="
  log_info "Registry: ${REGISTRY}"
  log_info "Gitops: ${GITOPS_DIR}"
  $DRY_RUN && log_info "Mode: DRY RUN"
  echo
  
  if [[ ! -d "$GITOPS_DIR" ]]; then
    log_error "Gitops directory not found: ${GITOPS_DIR}"
    exit 1
  fi
  
  local deploy_count=0
  local skip_count=0
  local flag_count=0
  
  # Iterate over each service in base/
  for base_dir in "$GITOPS_DIR"/base/*/; do
    local service_name
    service_name=$(basename "$base_dir")
    
    local base_values="$base_dir/values.yaml"
    local prod_values="$GITOPS_DIR/production/$service_name/values.yaml"
    
    # Extract production image
    local prod_info
    prod_info=$(extract_image "$prod_values") || continue
    local prod_repo prod_tag prod_digest
    read -r prod_repo prod_tag prod_digest <<< "$prod_info"
    
    # Replace placeholders with empty strings
    [[ "$prod_repo" == "_" ]] && prod_repo=""
    [[ "$prod_tag" == "_" ]] && prod_tag=""
    [[ "$prod_digest" == "_" ]] && prod_digest=""
    
    # Get repository from base values if not in production
    if [[ -z "$prod_repo" ]]; then
      prod_repo=$(grep 'repository:' "$base_values" | sed 's/.*repository: *//' | tr -d '"' | tr -d "'")
    fi
    
    # Skip if no production deployment
    if [[ -z "$prod_tag" ]]; then
      continue
    fi
    
    log_info "Service: ${service_name}"
    log_info "  Production: ${prod_repo}:${prod_tag}"
    
    # Find patched version
    local patched_tag
    patched_tag=$(latest_patched_tag "$prod_repo" "$prod_tag")
    
    if [[ -z "$patched_tag" ]]; then
      log_warn "  → FLAG: No patched version exists in registry (need to patch first)"
      flag_count=$((flag_count + 1))
      continue
    fi
    
    log_info "  Patched available: ${patched_tag}"
    
    # Check existing test deployments (informational only — we deploy regardless)
    local testing_dir="$GITOPS_DIR/testing/$service_name"
    local existing_tests
    existing_tests=$(list_test_deployments "$testing_dir")
    
    if [[ -n "$existing_tests" ]]; then
      log_info "  Testing has: $(echo "$existing_tests" | tr '\n' ', ' | sed 's/, $//')"
    fi
    
    # Check if this patched version is already in testing
    if echo "$existing_tests" | grep -q "^${patched_tag}$"; then
      log_info "  → SKIP: ${patched_tag} already in testing"
      skip_count=$((skip_count + 1))
      echo
      continue
    fi
    
    # Fetch digest for the patched image
    local patched_digest
    patched_digest=$(get_digest "$prod_repo" "$patched_tag")
    
    # Deploy patched version to testing (new file, doesn't overwrite existing)
    log_action "  → DEPLOY: ${patched_tag} → testing/${service_name}/${patched_tag}.yaml"
    create_test_deployment "$testing_dir" "$patched_tag" "$patched_digest"
    deploy_count=$((deploy_count + 1))
    
    echo
  done
  
  # Summary
  log_info "=== Summary ==="
  log_action "Deployed to testing: ${deploy_count}"
  [[ $flag_count -gt 0 ]] && log_warn "Flagged (need patching first): ${flag_count}"
  
  if ! $DRY_RUN && [[ $deploy_count -gt 0 ]]; then
    echo
    log_info "Gitops updated. Review changes with:"
    log_info "  cd ${GITOPS_DIR} && git diff"
  fi
}

# Run main
main
