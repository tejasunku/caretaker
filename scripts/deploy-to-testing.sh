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
  local service_dir="$1"
  local env="$2"  # testing, staging, or production
  
  local base_values="$service_dir/values.yaml"
  local env_values="$service_dir/values-${env}.yaml"
  
  if [[ ! -f "$base_values" ]]; then
    return 1
  fi
  
  # Repository from base values
  local repo
  repo=$(grep 'repository:' "$base_values" | sed 's/.*repository: *//' | tr -d '"' | tr -d "'")
  
  # Tag and digest from env values (fall back to base)
  local tag digest
  if [[ -f "$env_values" ]]; then
    tag=$(grep 'tag:' "$env_values" | head -1 | sed 's/.*tag: *//' | tr -d '"' | tr -d "'")
    digest=$(grep 'digest:' "$env_values" | head -1 | sed 's/.*digest: *//' | tr -d '"' | tr -d "'")
  fi
  
  # Fall back to base values if env doesn't have them
  [[ -z "$tag" ]] && tag=$(grep 'tag:' "$base_values" | head -1 | sed 's/.*tag: *//' | tr -d '"' | tr -d "'")
  [[ -z "$digest" ]] && digest=$(grep 'digest:' "$base_values" | head -1 | sed 's/.*digest: *//' | tr -d '"' | tr -d "'")
  
  # Strip comments
  tag="${tag%%#*}"
  digest="${digest%%#*}"
  tag=$(echo "$tag" | xargs)
  digest=$(echo "$digest" | xargs)
  
  echo "$repo $tag $digest"
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

# Strip the repository path to get just the image name
# e.g., "test/python-slim" → "python-slim"
image_name() {
  local repo="$1"
  echo "${repo##*/}"
}

# Update a values file with a new image
update_values_file() {
  local values_file="$1"
  local new_tag="$2"
  local new_digest="$3"
  
  if $DRY_RUN; then
    return 0
  fi
  
  # Update tag and digest in the image section
  sed -i "s/\(tag: *\).*/\1${new_tag}/" "$values_file"
  if [[ -n "$new_digest" ]]; then
    if grep -q 'digest:' "$values_file"; then
      sed -i "s/\(digest: *\).*/\1${new_digest}/" "$values_file"
    else
      # Add digest after tag line
      sed -i "/tag: *${new_tag}/a\\  digest: ${new_digest}" "$values_file"
    fi
  fi
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
  
  # Iterate over each service in gitops
  for service_dir in "$GITOPS_DIR"/charts/*/; do
    local service_name
    service_name=$(basename "$service_dir")
    
    local prod_values="$service_dir/values-production.yaml"
    local test_values="$service_dir/values-testing.yaml"
    
    # Extract production image
    local prod_info
    prod_info=$(extract_image "$service_dir" "production") || continue
    local prod_repo prod_tag prod_digest
    read -r prod_repo prod_tag prod_digest <<< "$prod_info"
    
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
    
    # Check current testing state (informational only — we deploy regardless)
    local test_info
    test_info=$(extract_image "$service_dir" "testing") || { test_info=""; }
    local test_repo test_tag test_digest
    read -r test_repo test_tag test_digest <<< "$test_info"
    
    if [[ -n "$test_tag" ]]; then
      log_info "  Testing currently has: ${test_tag} (will coexist with patched version)"
    fi
    
    # Always deploy patched version to testing (multiple versions can coexist)
    log_action "  → DEPLOY: ${patched_tag} → testing"
    update_values_file "$test_values" "$patched_tag" ""
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
