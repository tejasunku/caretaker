#!/usr/bin/env bash
# patch-all.sh — Scan registry and patch all images with Copa
#
# Usage:
#   ./scripts/patch-all.sh [OPTIONS]
#
# Options:
#   --registry <url>     Registry URL (default: localhost:5000)
#   --dry-run            Show what would be patched without doing it
#   --output-dir <dir>   Directory for patch reports (default: patch-reports/)
#   --platform <plat>    Target platform (default: linux/amd64)
#   --buildkit-addr <addr> BuildKit address (default: tcp://127.0.0.1:12345)
#   --skip-existing      Skip images that already have a patched version
#   --help               Show this help

set -euo pipefail

# Defaults
REGISTRY="localhost:5000"
DRY_RUN=false
OUTPUT_DIR="patch-reports"
PLATFORM="linux/amd64"
BUILDKIT_ADDR="tcp://127.0.0.1:12345"
SKIP_EXISTING=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --registry)
      REGISTRY="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --platform)
      PLATFORM="$2"
      shift 2
      ;;
    --buildkit-addr)
      BUILDKIT_ADDR="$2"
      shift 2
      ;;
    --skip-existing)
      SKIP_EXISTING=true
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

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# Check dependencies
check_deps() {
  local missing=()
  for cmd in curl jq; do
    if ! command -v "$cmd" &>/dev/null; then
      missing+=("$cmd")
    fi
  done
  
  # Check for copa in bin/ or PATH
  if [[ -x "bin/copa" ]]; then
    COPA="bin/copa"
  elif command -v copa &>/dev/null; then
    COPA="copa"
  else
    missing+=("copa")
  fi
  
  if [[ ${#missing[@]} -gt 0 ]]; then
    log_error "Missing dependencies: ${missing[*]}"
    log_info "Run ./scripts/setup.sh to install dependencies"
    exit 1
  fi
}

# List all repositories in the registry
list_repos() {
  local response
  response=$(curl -s "http://${REGISTRY}/v2/_catalog" 2>/dev/null) || {
    log_error "Cannot connect to registry at ${REGISTRY}"
    log_info "Is the registry running? Start it with: ./scripts/start-registry.sh"
    exit 1
  }
  
  echo "$response" | jq -r '.repositories[]'
}

# List all tags for a repository
list_tags() {
  local repo="$1"
  curl -s "http://${REGISTRY}/v2/${repo}/tags/list" 2>/dev/null | jq -r '.tags[] // empty'
}

# Check if a patched version already exists
has_patched_version() {
  local repo="$1"
  local tag="$2"
  local patched_tag="${tag}-patched"
  
  # Check if patched tag exists
  list_tags "$repo" | grep -q "^${patched_tag}$"
}

# Get image digest
get_digest() {
  local repo="$1"
  local tag="$2"
  
  # Get manifest digest from registry
  local digest
  digest=$(curl -sI "http://${REGISTRY}/v2/${repo}/manifests/${tag}" \
    -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
    -H "Accept: application/vnd.oci.image.manifest.v1+json" \
    2>/dev/null | grep -i 'docker-content-digest' | awk '{print $2}' | tr -d '\r')
  
  echo "$digest"
}

# Patch a single image
patch_image() {
  local repo="$1"
  local tag="$2"
  
  local source="${REGISTRY}/${repo}:${tag}"
  local patched_tag="${tag}-patched"
  local target="${REGISTRY}/${repo}:${patched_tag}"
  
  log_info "Patching ${source} → ${target}"
  
  if $DRY_RUN; then
    log_info "  [DRY RUN] Would patch to ${target}"
    return 0
  fi
  
  # Create output directory for this image
  local img_dir="${OUTPUT_DIR}/${repo}/${tag}"
  mkdir -p "$img_dir"
  
  # Run copa patch
  local start_time
  start_time=$(date +%s)
  
  if "$COPA" patch \
    -i "$source" \
    -o "$target" \
    --platform "$PLATFORM" \
    --addr "$BUILDKIT_ADDR" \
    --progress plain \
    2>&1 | tee "${img_dir}/patch.log"; then
    
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_ok "  Patched in ${duration}s"
    
    # Push to registry (Copa loads into local daemon, we need to push)
    log_info "  Pushing to registry..."
    if docker push --tls-verify=false "$target" 2>&1 | tee -a "${img_dir}/patch.log"; then
      log_ok "  Pushed to ${target}"
    else
      log_warn "  Failed to push (image loaded locally)"
    fi
    
    # Save patch metadata
    cat > "${img_dir}/metadata.json" << EOF
{
  "source": "${source}",
  "target": "${target}",
  "source_digest": "$(get_digest "$repo" "$tag")",
  "target_digest": "$(get_digest "$repo" "$patched_tag")",
  "patched_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "duration_seconds": ${duration},
  "platform": "${PLATFORM}",
  "mode": "comprehensive"
}
EOF
    
    return 0
  else
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_error "  Failed to patch ${source} (took ${duration}s)"
    echo '{"status": "failed", "error": "see patch.log"}' > "${img_dir}/metadata.json"
    return 1
  fi
}

# Main
main() {
  check_deps
  
  log_info "=== Copa Batch Patcher ==="
  log_info "Registry: ${REGISTRY}"
  log_info "Platform: ${PLATFORM}"
  log_info "BuildKit: ${BUILDKIT_ADDR}"
  $DRY_RUN && log_info "Mode: DRY RUN (no images will be patched)"
  $SKIP_EXISTING && log_info "Skipping images with existing patched versions"
  echo
  
  # Get all repositories
  local repos
  repos=$(list_repos)
  
  if [[ -z "$repos" ]]; then
    log_warn "No repositories found in registry"
    exit 0
  fi
  
  local total_images=0
  local patched_images=0
  local skipped_images=0
  local failed_images=0
  
  # Process each repository
  for repo in $repos; do
    log_info "Repository: ${repo}"
    
    # Get all tags
    local tags
    tags=$(list_tags "$repo" 2>/dev/null)
    
    if [[ -z "$tags" ]]; then
      log_warn "  No tags found"
      continue
    fi
    
    for tag in $tags; do
      # Skip already-patched images
      if [[ "$tag" == *-patched ]]; then
        continue
      fi
      
      total_images=$((total_images + 1))
      
      # Check if we should skip
      if $SKIP_EXISTING && has_patched_version "$repo" "$tag"; then
        log_info "  Skipping ${repo}:${tag} (patched version exists)"
        skipped_images=$((skipped_images + 1))
        continue
      fi
      
      # Patch the image
      if patch_image "$repo" "$tag"; then
        patched_images=$((patched_images + 1))
      else
        failed_images=$((failed_images + 1))
      fi
    done
  done
  
  # Summary
  echo
  log_info "=== Summary ==="
  log_info "Total images: ${total_images}"
  log_ok "Patched: ${patched_images}"
  [[ $skipped_images -gt 0 ]] && log_info "Skipped: ${skipped_images}"
  [[ $failed_images -gt 0 ]] && log_error "Failed: ${failed_images}"
  
  if [[ $failed_images -gt 0 ]]; then
    exit 1
  fi
}

# Run main
main
