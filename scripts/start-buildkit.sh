#!/usr/bin/env bash
set -euo pipefail

# Start BuildKit for Copa patching.
# Must use --network host so BuildKit can reach the registry at localhost:5000.
# Without host networking, BuildKit runs in a container and localhost inside
# it is NOT the host — registry connections fail.

export DOCKER_HOST="${DOCKER_HOST:-unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock}"

echo "Starting BuildKit with host networking..."
echo "Registry must be running at http://localhost:5000"
echo ""

# Stop existing container if running
docker stop buildkitd 2>/dev/null && docker rm buildkitd 2>/dev/null || true

# Start with --network host so BuildKit can reach localhost:5000
docker run -d \
  --name buildkitd \
  --privileged \
  --network host \
  moby/buildkit:latest \
  --addr tcp://0.0.0.0:12345

echo "BuildKit started on tcp://127.0.0.1:12345"
echo ""
echo "Test with:"
echo "  ./bin/copa patch -i <image> -o <output> --addr tcp://127.0.0.1:12345 --progress plain"
