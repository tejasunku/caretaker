#!/usr/bin/env bash
set -euo pipefail

# Build and push test images to local zot registry.
# Prerequisites: start-registry.sh must be running.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
REGISTRY="localhost:5000"
DOCKERFILES_DIR="$PROJECT_ROOT/dockerfiles"

echo "Building and pushing images to $REGISTRY..."
echo ""

# Function to build and push an image
push_image() {
    local name="$1"
    local dockerfile="$2"
    local tag="${3:-1.0}"

    echo "--- $name:$tag ---"
    podman build \
        -t "$REGISTRY/$name:$tag" \
        -f "$DOCKERFILES_DIR/$dockerfile" \
        "$PROJECT_ROOT" 2>&1 | tail -3

    podman push --tls-verify=false "$REGISTRY/$name:$tag" 2>&1 | tail -2
    echo ""
}

# Push all test images
push_image "test/python-slim" "python-slim.Dockerfile" "1.0"
push_image "test/python-slim" "python-slim.Dockerfile" "1.1"
push_image "test/node-slim" "node-slim.Dockerfile" "1.0"
push_image "test/ubuntu" "ubuntu.Dockerfile" "1.0"

echo "Done. Images available at $REGISTRY/test/"
echo ""
echo "List images in registry:"
echo "  curl -s http://$REGISTRY/v2/_catalog"
echo ""
echo "List tags for an image:"
echo "  curl -s http://$REGISTRY/v2/test/python-slim/tags/list"
