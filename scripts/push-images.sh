#!/usr/bin/env bash
set -euo pipefail

# Build and push per-service test images to local zot registry.
# Each service has its own Dockerfile with a unique content file,
# so every service image has a distinct digest.
# Prerequisites: start-registry.sh must be running.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
REGISTRY="localhost:5000"
DOCKERFILES_DIR="$PROJECT_ROOT/dockerfiles"

# podman-remote is exposed as `docker` via a symlink in ~/bin
export DOCKER_HOST="${DOCKER_HOST:-unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock}"

SERVICES=(api auth worker web payments cache)

echo "Building and pushing per-service images to $REGISTRY..."
echo ""

push_service() {
    local svc="$1"
    local tag="${2:-1.0}"

    echo "--- test/$svc:$tag ---"
    docker build \
        -t "$REGISTRY/test/$svc:$tag" \
        -f "$DOCKERFILES_DIR/$svc.Dockerfile" \
        "$PROJECT_ROOT" 2>&1 | tail -3

    docker push --tls-verify=false "$REGISTRY/test/$svc:$tag" 2>&1 | tail -2
    echo ""
}

for svc in "${SERVICES[@]}"; do
    push_service "$svc" "1.0"
done

echo "Done. Images available at $REGISTRY/test/"
echo ""
echo "Registry contents:"
echo "  curl -s http://$REGISTRY/v2/_catalog"
echo ""
echo "List tags for a service:"
echo "  curl -s http://$REGISTRY/v2/test/api/tags/list"
