---
name: copacetic
description: Use Copacetic (copa) to patch container image vulnerabilities without rebuilds. Covers installation, BuildKit setup, comprehensive and targeted patching, trivy verification, and the podman/WSL workflow for this repo.
---

# Copacetic (copa) Skill

Copacetic patches container images directly using BuildKit — no full rebuild needed. Adds a single patch layer on top of existing image layers.

## Binaries

All tools live in `bin/` (gitignored). Install via:

```bash
bash scripts/setup.sh
export PATH="$PWD/bin:$PATH"
```

- `bin/copa` — v0.15.0
- `bin/trivy` — v0.75.0
- `bin/helm` — v3.22.0

## Prerequisites

Copa requires **BuildKit** and a **container runtime** (Docker or Podman).

### This environment (Podman + WSL)

Podman runs on Windows; WSL connects via socket. BuildKit runs as a container with TCP.

```bash
# Ensure buildkitd is running (TCP on 127.0.0.1:12345)
docker rm -f buildkitd 2>/dev/null
docker run -d --name buildkitd --privileged \
  -p 127.0.0.1:12345:12345 \
  moby/buildkit:latest --addr tcp://0.0.0.0:12345

# Point docker/podman CLI at the podman socket
export DOCKER_HOST=unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock
export PATH="$PWD/bin:$PATH"
```

Copa flags for this setup:
- `--addr tcp://127.0.0.1:12345` — BuildKit TCP address
- `--oci-dir <path>` — export as OCI layout instead of loading into runtime
- `-t <tag>` — output image tag
- `--platform linux/amd64` — target single platform (avoids multi-platform pull)

### Standard Docker

If Docker with BuildKit is available natively, copa auto-detects it. No special flags needed.

## Patching Modes

### Comprehensive (no report)

Updates ALL outdated packages to latest versions:

```bash
copa patch -i <image> -t <image>-patched
# Example:
copa patch -i docker.io/library/nginx:1.27 \
  -t docker.io/library/nginx:1.27-patched \
  --addr tcp://127.0.0.1:12345 \
  --platform linux/amd64
```

### Targeted (Trivy report)

Patches only packages with known vulnerabilities:

```bash
# 1. Scan and generate report
trivy image --vuln-type os --ignore-unfixed \
  -f json -o report.json <image>

# 2. Patch using report
copa patch -r report.json -i <image> -t <image>-patched \
  --addr tcp://127.0.0.1:12345
```

### Key flags

| Flag | Purpose |
|------|---------|
| `-i` | Input image |
| `-t` | Output image tag (default: `<input>-patched`) |
| `-r` | Trivy report file for targeted patching |
| `--addr` | BuildKit address |
| `--oci-dir` | Export as OCI layout directory |
| `--platform` | Target platform (e.g. `linux/amd64`) |
| `--push` | Push patched image to registry |
| `--debug` | Verbose logging |
| `--ignore-errors` | Continue on per-platform failures |

## Verification

### Trivy before/after scan

```bash
# Before
trivy image --vuln-type os --ignore-unfixed <image>

# After
trivy image --vuln-type os --ignore-unfixed <image>-patched
```

Expected: vulnerability count drops to 0 (for comprehensive patching on supported images).

### Inspect patch layer

```bash
docker history <image>-patched --format "table {{.ID}}\t{{.CreatedSince}}\t{{.Size}}\t{{.Comment}}"
```

The first entry is the patch layer added by copa.

## Empirical Findings

### Idempotency

Patching the **same source image twice produces identical image IDs** (config SHA). Verified:

```
nginx:1.27-patched       → bba0eb1715aa8a86fedf1bde2ad37b3c5edbe04c1271de38b245d2077c5c5554
nginx:1.27-second-patch  → bba0eb1715aa8a86fedf1bde2ad37b3c5edbe04c1271de38b245d2077c5c5554
```

Same layers, same config. The process is deterministic for the same input.

### EOL image failure

Debian 11 (bullseye) images fail to patch — security repos moved to `archive.debian.org`, causing 404s when copa tries to install `busybox-static`. Copa warns: *"The operating system debian 11 appears to be End-of-Support-Life."*

**Use non-EOL base images** (Debian 12/bookworm or newer) for patching.

### Known limitations

- Copa needs BuildKit — cannot use podman/buildah directly
- BuildKit container can't see podman's local images; pull from registry or use `--oci-dir`
- Multi-platform images: copa patches all platforms by default; use `--platform` to limit
- `nginx:1.21.6` (tutorial image) is Debian 11 — will fail on current repos

## Demo Image

`docker.io/library/nginx:1.27` (Debian 12.11) works well:

- **Before**: 263 OS vulnerabilities (8 CRITICAL, 78 HIGH)
- **After comprehensive patch**: 0 vulnerabilities
- Patch layer size: ~101 MB
- Total image: ~297 MB (from ~197 MB)

## Quick Demo Script

```bash
#!/usr/bin/env bash
set -euo pipefail
export PATH="$PWD/bin:$PATH"
export DOCKER_HOST=unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock
ADDR="tcp://127.0.0.1:12345"
IMAGE="docker.io/library/nginx:1.27"

echo "=== Before ==="
trivy image --vuln-type os --ignore-unfixed "$IMAGE" 2>&1 | grep "Total:"

echo "=== Patching ==="
copa patch -i "$IMAGE" -t "${IMAGE}-patched" --addr "$ADDR" --platform linux/amd64

echo "=== After ==="
trivy image --vuln-type os --ignore-unfixed "${IMAGE}-patched" 2>&1 | grep -A2 "Vulnerabilities"
```
