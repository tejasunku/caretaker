---
name: caretaker
description: Overview of the caretaker project — a patch tracking and hot-swap system for container images using Copacetic + Copacetic patched images across Kubernetes clusters. Covers project structure, the cluster registry + Provider abstraction (simulated + real kind), the caretaker CLI, the OCI registry with Referrers API, and the web UI.
---

# Caretaker Skill

Caretaker is a prototype system that tracks and applies CVE patches to container images running in
Kubernetes clusters. The lowest unit of production is **a cluster running a service** (not a Helm
release). Caretaker sites on the side of existing deployment tooling and performs **in-place image
hot-swaps** — replacing the image on a running Deployment without changing declarative source.

It demonstrates:
1. **Patch state:** Which patches does a running image have/miss?
2. **Patch velocity:** How quickly are patches hot-swapped into clusters after release?
3. **Deployment tracking:** Which clusters run which version of each service, and which are stale?

## Project Structure

```
caretaker/
├── PINS.md                    # Deferred decisions and assumptions
├── REF_SYSTEM.md              # Reference implementation design
├── TIMING.md                  # Empirical Trivy/Copa timing measurements
├── bin/                       # Local binaries (gitignored)
│   └── copa trivy oras zot helm kubectl kind
├── config/
│   ├── zot-minimal.json       # Zot registry config (binds 0.0.0.0:5000)
│   └── clusters.json          # Cluster registry (id, stage, provider, namespace)
├── dockerfiles/
│   ├── <service>.Dockerfile   # One per service (api auth worker web payments cache)
│   └── content/<service>.txt  # Unique marker -> distinct image digests
├── patch-manifests/
│   └── mock-patches.json      # Copa-native patch manifest (7 CVE + non-CVE entries)
├── clusters/                  # Simulated cluster state (gitignored)
│   └── <cluster-id>.json      # Deployments + hot-swap event log
├── scripts/
│   ├── setup.sh               # Install copa, trivy, oras, zot, helm, kubectl, kind
│   ├── start-registry.sh      # Start zot registry on :5000
│   ├── start-buildkit.sh      # Start BuildKit (--network host) on :12345
│   ├── push-images.sh         # Build + push per-service images
│   ├── patch-all.sh           # Batch patch registry images with Copa
│   ├── clusters.py            # Provider abstraction (Simulated + Kind)
│   ├── seed_clusters.py       # Seed simulated cluster state with real digests
│   ├── caretaker.py           # Main CLI (clusters/deployments/status/swap/patch/promote)
│   ├── kind-setup.sh          # Bootstrap a real kind cluster + Deployments + registry mirror
│   └── generate_cluster_data.py  # Emit web/src/lib/cluster-data.ts from live clusters
└── web/                       # SolidStart v2 web UI (SSR disabled, UnoCSS)
    └── src/
        ├── lib/
        │   ├── cluster-data.ts   # Auto-generated from clusters
        │   ├── sbom-checker.ts   # SBOM verification
        │   └── trivy-to-copa.ts  # Trivy→Copa report converter
        └── routes/
            ├── index.tsx         # Cluster overview
            ├── cluster/[id].tsx  # Cluster detail + hot-swap events
            ├── service/[name].tsx# Service across clusters
            └── data.ts           # Cluster-centric data loaders
```

## Quick Start

```bash
# 0. Environment (WSL + podman-remote on Windows podman machine)
export DOCKER_HOST="unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock"
export CONTAINER_HOST="$DOCKER_HOST"
export PATH="$HOME/bin:$PWD/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=podman

# 1. Setup (installs binaries to bin/)
./scripts/setup.sh

# 2. Start registry + BuildKit
./scripts/start-registry.sh
./scripts/start-buildkit.sh

# 3. Build and push per-service images (tag 1.0)
./scripts/push-images.sh

# 4. Patch images with Copa (-patched-YYYYMMDD + Referrers)
./scripts/patch-all.sh --include '^test/(api|auth|worker|web|payments|cache)$'

# 5. Real cluster (optional but recommended)
./scripts/kind-setup.sh              # creates kind cluster, loads images, applies Deployments, sets up registry mirror

# 6. Seed simulated clusters with real digests
python3 scripts/seed_clusters.py

# 7. Drive the system
python3 scripts/caretaker.py clusters
python3 scripts/caretaker.py status

# 8. Regenerate web data + run UI
python3 scripts/generate_cluster_data.py
cd web && npx vite dev --port 3000
```

## The caretaker CLI

`scripts/caretaker.py` is the primary interface:

```bash
python3 scripts/caretaker.py clusters                          # list clusters
python3 scripts/caretaker.py deployments [--cluster ID] [--service SVC]
python3 scripts/caretaker.py status [--service SVC]            # stale summary
python3 scripts/caretaker.py swap --service api \
  --image localhost:5000/test/api:1.0-patched-20261016 \
  --clusters prod-a,prod-b [--dry-run] [--force]               # hot-swap an image
python3 scripts/caretaker.py promote --service api \
  --from test-1 --to staging-1 [--dry-run] [--force]           # copy a deployment between clusters
python3 scripts/caretaker.py patch --service worker \
  --clusters staging-1 [--dry-run] [--force]                   # Copa patch + push + hot-swap
```

`swap` hot-swaps the image on the running Deployment (digest-pinned: `repo:tag@sha256:...`).
`patch` runs Copa (`copa patch -i <ref> -t <tag>-patched-YYYYMMDD --addr tcp://127.0.0.1:12345
--platform linux/amd64 --push`), then hot-swaps the result into the target clusters.

### Version guard
If a **newer base version** of a service is deployed in ANY cluster, `swap`/`patch`/`promote` refuse
to deploy an older `-patched-` tag and print
`HALT <tag>: <newer> is being deployed (refusing to patch <target>) (use --force to override)`.
Override with `--force`. `patch` exits 1 on halt.

## Cluster model

- **Cluster registry** `config/clusters.json`: `{id, stage, provider, namespace}`.
  Defaults: `test-1` (testing/simulated), `staging-1` (staging/simulated), `prod-a`, `prod-b`
  (production/simulated), `kind-caretaker` (production/kind, namespace `caretaker`).
- Clusters are **independently versioned** — different prod clusters may run different versions.
- **Provider abstraction** (`scripts/clusters.py`):
  - `SimulatedProvider` — file-backed (`clusters/<id>.json`), rewrites a JSON field + appends an
    event log. Offline fallback only.
  - `KindProvider` — real `kubectl --context <cluster-id> -n <namespace>`; `list_deployments` via
    `kubectl get deployments -o json`; `set_image` via
    `kubectl set image deployment/<svc> <svc>=<repo>:<tag>@<digest>`.
  - KindProvider maps the caretaker cluster id DIRECTLY to the kube-context name (id == context).
- **Hot swap** = replace the image on a live Deployment without touching declarative source. Real
  form: `kubectl set image deployment/<svc> <svc>=<repo>@<digest>`.

## Registry + image availability to the kind node

- Zot binds `0.0.0.0:5000` (`config/zot-minimal.json`) so the kind node can reach it.
- kind network gateway is the host interface (`podman network inspect kind`; default `10.89.2.1`,
  node `10.89.2.2`). A containerd mirror is installed in the node:
  `[plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5000"] endpoint=["http://10.89.2.1:5000"]`.
  `kind-setup.sh` applies this idempotently and restarts containerd. Without it, digest-pinned refs
  fail with `ImagePullBackOff` (the node's bridge netns cannot resolve `localhost:5000`).
- `kind load docker-image` is still used to preload tag refs; with the mirror, real pulls work too.

## OCI Registry with Referrers API

- **Referrers API** (OCI 1.1, Feb 2024) attaches metadata to an image **without changing its digest**.
- **ORAS** CLI: `oras attach <image> metadata.json --artifact-type "..." --plain-http --disable-path-validation`
  then `oras discover <image> --plain-http`.
- Patch metadata artifact type: `application/vnd.caretaker.patch-manifest.v1+json`; SBOM:
  `application/vnd.cyclonedx+json`.

### Patch workflow
1. **Copa** patches the image → new digest with patch layer.
2. Output tagged `-patched-YYYYMMDD` and pushed to the registry.
3. Referrers attached (patch manifest + CycloneDX SBOM) for parent lineage.
4. `caretaker patch` (or `swap`) hot-swaps the new digest into target clusters.

## Web UI

SolidStart v2, port 3000, SSR disabled, UnoCSS. Reads `web/src/lib/cluster-data.ts` (generated by
`scripts/generate_cluster_data.py`, which queries each provider live so the real kind cluster is
included).

- **Overview** (`/`): clusters grid + stats (Clusters / Deployments / Need Patching / Critical Missing) + Available Patches.
- **Cluster Detail** (`/cluster/[id]`): per-service deployments (image, digest, replicas, status) + hot-swap events.
- **Service Across Clusters** (`/service/[name]`): every cluster running the service + missing patches.

## Empirical Findings

- **Copa idempotency**: Re-patching produces identical image IDs (verified).
- **Timing**: ~25-55s per image (comprehensive mode); faster with BuildKit cache.
- **Trivy before/after**: 263 vulns → 0 vulns after comprehensive patch.
- **Kind on podman**: the default docker provider fails
  (`unsupported bridge network option com.docker.network.bridge.enable_ip_masquerade`) — MUST set
  `KIND_EXPERIMENTAL_PROVIDER=podman`.

See `TIMING.md` for detailed measurements.

## Common Issues

1. **BuildKit can't reach registry**: must use `--network host` (see `start-buildkit.sh`).
2. **`podman` not on PATH**: kind execs `podman` directly; `~/bin/podman` is a symlink to `podman-remote`.
3. **Kind node can't pull**: ensure zot binds `0.0.0.0` and the containerd mirror points at the kind gateway.
4. **ORAS rejects absolute paths**: use `--disable-path-validation`.
5. **Zot config**: uses `"address"` not `"addr"`; registry supports DELETE (202) even without `deleteEnabled`.
6. **`pkill` stalls**: check the port binding first (`ss -tlnp | grep :5000`), use `setsid + disown`.
7. **SolidStart dev-toolbar console errors** (`@jridgewell/resolve-uri`): pre-existing, harmless.

## Related Skills

- **copacetic**: Copa patching (comprehensive/targeted modes, BuildKit setup)
- **trivy**: Trivy scanning (vulnerabilities, SBOMs, report formats)
