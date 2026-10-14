---
name: caretaker
description: Overview of the caretaker project — a patch tracking system for container images using Copacetic. Covers project structure, gitops repo, OCI registry with Referrers API, patching workflow, and the web UI.
---

# Caretaker Skill

Caretaker is a prototype system for tracking and applying CVE patches to container images using Copacetic (Copa). It demonstrates:
1. **Patch state:** Which patches does an image have/miss?
2. **Patch velocity:** How quickly are patches applied after release?
3. **Deployment tracking:** Where is each image deployed (testing/staging/production)?

## Project Structure

```
caretaker/
├── PINS.md                    # Deferred decisions and assumptions
├── REF_SYSTEM.md              # Reference implementation design
├── TIMING.md                  # Empirical Trivy/Copa timing measurements
├── bin/                       # Local binaries (gitignored)
│   ├── copa                   # Copacetic v0.15.0
│   ├── trivy                  # Trivy v0.75.0
│   ├── oras                   # ORAS v1.2.2 (OCI Referrers)
│   ├── zot                    # Zot registry
│   └── helm                   # Helm v3.22.0
├── config/
│   └── zot-minimal.json       # Zot registry config
├── dockerfiles/               # Test image Dockerfiles
│   ├── python-slim.Dockerfile
│   ├── node-slim.Dockerfile
│   └── ubuntu.Dockerfile
├── patch-manifests/           # Copa-native patch manifests
│   └── mock-patches.json      # Mixed CVE + non-CVE patches
├── gitops/                    # GitOps repo (separate git, gitignored)
│   ├── base/                  # Helm charts + shared defaults
│   │   └── <service>/
│   │       ├── Chart.yaml
│   │       ├── values.yaml    # repository, tag, infra defaults
│   │       └── templates/
│   ├── testing/               # Multiple concurrent deployments
│   │   └── <service>/
│   │       ├── 1.0.yaml
│   │       └── 1.0-patched-20261014.yaml
│   ├── staging/               # Single deployment (interaction checks)
│   │   └── <service>/values.yaml
│   └── production/            # Single deployment
│       └── <service>/values.yaml
├── scripts/
│   ├── setup.sh               # Install copa, trivy, oras, zot, helm
│   ├── start-registry.sh      # Start zot registry
│   ├── start-buildkit.sh      # Start BuildKit for Copa
│   ├── push-images.sh         # Build and push test images
│   ├── patch-all.sh           # Batch patch all images with Copa
│   ├── deploy-to-testing.sh   # Deploy patched images to testing
│   └── generate_gitops_data.py  # Generate web/src/lib/gitops-data.ts
└── web/                       # SolidStart v2 web UI
    └── src/
        ├── lib/
        │   ├── gitops-data.ts    # Auto-generated from gitops repo
        │   ├── types.ts          # TypeScript types
        │   ├── sbom-checker.ts   # SBOM verification
        │   └── trivy-to-copa.ts  # Trivy→Copa report converter
        └── routes/
            ├── index.tsx         # Patching Overview
            └── service/[name].tsx # Service Deep Dive
```

## Quick Start

```bash
# 1. Setup (installs binaries to bin/)
./scripts/setup.sh

# 2. Add binaries to PATH
export PATH="$PWD/bin:$PATH"

# 3. Start registry (zot on port 5000)
./scripts/start-registry.sh

# 4. Start BuildKit (required for Copa)
./scripts/start-buildkit.sh

# 5. Build and push test images
./scripts/push-images.sh

# 6. Patch all images
./scripts/patch-all.sh

# 7. Deploy patched images to testing
./scripts/deploy-to-testing.sh

# 8. Regenerate web data
python3 scripts/generate_gitops_data.py

# 9. Start web UI
cd web && npx vite dev --port 3000
```

## Key Concepts

### OCI Registry with Referrers API
- **Zot** runs locally on port 5000 (OCI Distribution API)
- **Referrers API** (OCI 1.1): Attach metadata to images without modifying them
- **ORAS** CLI: `oras attach <image> file.json --artifact-type "..." --plain-http`
- **Digests**: SHA256 of manifest (immutable identity). Tags are mutable pointers.

### Patch Workflow
1. **Copa patches image** → creates new digest with patch layer
2. **Docker tags** the Copa output → adds date suffix (`-patched-YYYYMMDD`)
3. **Push to registry** → new image with new digest
4. **Attach Referrers** → patch metadata + SBOM attached via ORAS

### Environment Model
- **Testing**: Multiple concurrent versions per service (single-service health checks)
- **Staging**: Single version per service (service interaction checks)
- **Production**: Single version per service (live traffic)

Same version never in both test and prod. Patched versions go to testing immediately.

### GitOps Structure (Environment-First)
```
base/<service>/     # Helm chart + shared defaults
testing/<service>/  # One file per concurrent deployment
staging/<service>/  # One file (interaction checks)
production/<service>/ # One file (live traffic)
```

Environment-specific health checks:
- Testing: Aggressive (5s liveness, 3s readiness, fail fast)
- Staging: Moderate (include service-to-service checks)
- Production: Strict (longer observation windows)

## Patching Commands

```bash
# Patch all images (comprehensive mode)
./scripts/patch-all.sh

# Patch specific image
./bin/copa patch -i localhost:5000/test/python-slim:1.0 \
  -o localhost:5000/test/python-slim:1.0-patched \
  --addr tcp://127.0.0.1:12345 --progress plain

# Scan image for vulnerabilities
./bin/trivy image localhost:5000/test/python-slim:1.0-patched

# Generate SBOM
./bin/trivy image --format cyclonedx --output sbom.cdx.json \
  localhost:5000/test/python-slim:1.0-patched

# Attach metadata as Referrer
./bin/oras attach --artifact-type "application/vnd.caretaker.patch-manifest.v1+json" \
  localhost:5000/test/python-slim:1.0-patched metadata.json \
  --plain-http --disable-path-validation

# Discover referrers for an image
./bin/oras discover localhost:5000/test/python-slim:1.0-patched --plain-http

# Check registry contents
curl -s http://localhost:5000/v2/_catalog | jq .
curl -s http://localhost:5000/v2/test/python-slim/tags/list | jq .
```

## Testing Images

Three test images in the registry:
- `test/python-slim:1.0` — Python 3.11 (Debian 12)
- `test/node-slim:1.0` — Node 18 (Debian 12)
- `test/ubuntu:1.0` — Ubuntu 22.04

Each has a `-patched-YYYYMMDD` version created by Copa.

## Web UI

Runs on port 3000. Reads from `web/src/lib/gitops-data.ts` (auto-generated).

- **Overview** (`/`): Service cards sorted by stale-first, patch status
- **Service Detail** (`/service/[name]`): Feature line status, environments, digests, git history

Regenerate data after gitops changes:
```bash
python3 scripts/generate_gitops_data.py
```

## Empirical Findings

- **Copa idempotency**: Re-patching produces identical image IDs (verified)
- **EOL images fail**: Debian 11 (nginx:1.21.6) — busybox-static 404s
- **Works on Debian 12**: nginx:1.27 (bookworm) patched successfully
- **Timing**: ~30s per image (comprehensive mode), faster with BuildKit cache
- **Trivy before/after**: 263 vulns → 0 vulns after comprehensive patch

See `TIMING.md` for detailed measurements.

## Common Issues

1. **BuildKit can't reach registry**: Must use `--network host` (see `start-buildkit.sh`)
2. **Copa names output `<source>:<tag>-patched`**: Ignores `-o` for daemon name. Need `docker tag` before push.
3. **ORAS rejects absolute paths**: Use `--disable-path-validation` flag
4. **Zot config**: Uses `"address"` not `"addr"` in http section
5. **`pkill` stalls**: Check port binding first (`ss -tlnp | grep :5000`), use `setsid + disown` for background processes
