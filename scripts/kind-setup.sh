#!/usr/bin/env bash
# kind-setup.sh -- provision a real kind cluster with the caretaker services.
#
# Creates (if needed) a kind cluster backed by the podman machine, loads the
# service images into the node, and applies one Deployment per service.  The
# pods run a trivial sleep loop so they stay Running and can be hot-swapped.
#
# Usage:
#   ./scripts/kind-setup.sh [cluster-name] [tag]
#
# Defaults: cluster "caretaker", tag "1.0".

set -euo pipefail

CLUSTER="${1:-caretaker}"
TAG="${2:-1.0}"
NS="caretaker"
REGISTRY="localhost:5000/test"
SERVICES=(api auth worker web payments cache)
CONTEXT="kind-${CLUSTER}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

export DOCKER_HOST="${DOCKER_HOST:-unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock}"
export CONTAINER_HOST="${CONTAINER_HOST:-$DOCKER_HOST}"
export PATH="$HOME/bin:$PROJECT_ROOT/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=podman

log() { echo "[kind-setup] $*"; }

# 1. Create the cluster if it does not exist.
if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  log "creating kind cluster '$CLUSTER'..."
  kind create cluster --name "$CLUSTER"
else
  log "cluster '$CLUSTER' already exists"
fi

# 1b. Point the node's containerd at the host registry.  The node runs on a
# bridge network, so "localhost:5000" inside the node is the node itself, not
# the WSL host where zot listens.  We add a registry mirror to the kind network
# gateway (the host) so the node can pull over plain HTTP.  This makes both
# tag- and digest-pinned image refs resolve without preloading.
NODE="${CLUSTER}-control-plane"
if docker inspect "$NODE" >/dev/null 2>&1; then
  GW=$(podman network inspect kind --format '{{range .Subnets}}{{.Gateway}}{{"\n"}}{{end}}' 2>/dev/null | grep -E '^[0-9]+\.' | head -1)
  if [ -n "$GW" ]; then
    if ! docker exec "$NODE" grep -q 'registry.mirrors' /etc/containerd/config.toml; then
      log "configuring node registry mirror localhost:5000 -> http://${GW}:5000"
      docker exec "$NODE" sh -c "cat >> /etc/containerd/config.toml <<EOF

[plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors.\"localhost:5000\"]
  endpoint = [\"http://${GW}:5000\"]
EOF"
      docker exec "$NODE" systemctl restart containerd
      sleep 5
    else
      log "node registry mirror already configured"
    fi
  else
    log "WARN: could not determine kind network gateway; registry mirror skipped"
  fi
fi

# 2. Load the current and patched images into the node.
log "loading images into node..."
images=()
for svc in "${SERVICES[@]}"; do
  images+=("${REGISTRY}/${svc}:${TAG}")
done
patched=$(kind get clusters >/dev/null 2>&1; true)
# Load any patched tags present in the local store for this cluster.
available=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep -E "^${REGISTRY}/(${SERVICES[*]// /|})-patched-" || true)
for img in $available; do
  images+=("$img")
done
kind load docker-image --name "$CLUSTER" "${images[@]}" 2>&1 | grep -vE "^(enabling|using podman)" || true

# 3. Apply the namespace and deployments.
log "applying namespace '$NS' and ${#SERVICES[@]} deployments at tag ${TAG}..."
kubectl --context "$CONTEXT" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f - >/dev/null

for svc in "${SERVICES[@]}"; do
  kubectl --context "$CONTEXT" apply -f - >/dev/null <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${svc}
  namespace: ${NS}
  labels: { app: ${svc} }
spec:
  replicas: 3
  selector:
    matchLabels: { app: ${svc} }
  template:
    metadata:
      labels: { app: ${svc} }
    spec:
      containers:
        - name: ${svc}
          image: ${REGISTRY}/${svc}:${TAG}
          imagePullPolicy: IfNotPresent
          command: ["/bin/sh", "-c", "while true; do sleep 3600; done"]
EOF
done

log "waiting for rollouts..."
for svc in "${SERVICES[@]}"; do
  kubectl --context "$CONTEXT" -n "$NS" rollout status "deployment/${svc}" --timeout=180s
done

log "done. Pods:"
kubectl --context "$CONTEXT" -n "$NS" get pods -o wide
