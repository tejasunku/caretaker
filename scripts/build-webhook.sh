#!/usr/bin/env bash
set -euo pipefail

# Build and push the in-cluster webhook image, then print its digest-pinned
# reference for `caretaker webhook deploy --image <ref>`.
# Prerequisites: start-registry.sh must be running.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
REGISTRY="${REGISTRY:-localhost:5000}"
TAG="${TAG:-1.0}"
IMAGE="$REGISTRY/test/caretaker-webhook:$TAG"

echo "Building $IMAGE ..."
docker build \
    -f "$PROJECT_ROOT/Dockerfile.webhook" \
    -t "$IMAGE" \
    "$PROJECT_ROOT"

echo "Pushing $IMAGE ..."
docker push --tls-verify=false "$IMAGE"

REPO_PATH="${IMAGE#"$REGISTRY"/}"
REPO="${REPO_PATH%:*}"
DIGEST="$(curl -fsSI -H 'Accept: application/vnd.oci.image.manifest.v1+json' \
    "http://$REGISTRY/v2/$REPO/manifests/$TAG" | tr -d '\r' \
    | awk -F': ' 'tolower($1) == "docker-content-digest" {print $2}')"
if [[ -z "$DIGEST" ]]; then
    echo "could not determine a digest for $IMAGE" >&2
    exit 1
fi
DIGEST_REF="$REGISTRY/$REPO@$DIGEST"
echo
echo "$DIGEST_REF"
