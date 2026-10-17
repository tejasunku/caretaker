# REF_SYSTEM.md — Reference Implementation Design

What a meaningful reference implementation looks like, how we test it, and what tradeoffs we're making for the prototype.

---

## Goal

A working local system that demonstrates the core queries:

1. **Query A:** "Given an image running in a cluster, which patches does it have (and which is it missing)?"
2. **Query B:** "Given a time range, which patches need to be applied, and to which clusters?"
3. **Patch velocity:** How quickly does a patch reach production after release?

The system makes the gap between "patch available" and "patch running in production" visible and measurable.

---

## Core Concepts

### The cluster is the unit of production

The lowest unit of production is **a cluster running a service** — not a Helm release. Caretaker
sites beside existing deployment tooling (it does not replace it) and operates on **clusters**:

- Clusters are **independently versioned**: `prod-a` and `prod-b` may run different versions of the
  same service.
- Caretaker is **agnostic about how clusters are created**; it only needs a way to read deployments
  and to set an image.
- Deployment state is read from the live cluster, not from declarative source (gitops is retired).

### Patch registry

A record of CVE patches and when they were released — the ground truth for "what patches exist."
For the prototype: `patch-manifests/mock-patches.json` (Copa v1alpha1 format, mixed CVE + non-CVE).

### Image registry

Zot (local OCI registry on `:5000`). All major registries (Docker Hub, ECR, GCR, ACR, Harbor, GHCR)
implement the same OCI Distribution API.

**Digest-based lineage.** Tags are simple feature identifiers (`api:1.0`). Patched images are NEW
digests that track lineage back to the original build via the OCI Referrers API. The image digest
stays frozen when metadata is attached.

```
Build:    test/api:1.0            → digest A
  └── Patched: test/api:1.0-patched-20261016 → digest B (parent = A)
        ├── Referrer: patch-manifest.v1+json (parent=A, patchedAt=…)
        └── Referrer: SBOM (CycloneDX)
```

**OCI Referrers API (OCI 1.1, Feb 2024):**
- Manifests have a `subject` field pointing at another manifest
- Query: `GET /v2/<name>/referrers/<digest>` returns an OCI Index of artifacts
- Each referrer has an `artifactType` (e.g. `application/vnd.cyclonedx+json`)
- Zot supports the Referrers API fully
- ORAS CLI: `oras attach <image> file.json --artifact-type "application/vnd.caretaker.patch-manifest.v1+json"`

**Why Referrers over manifest annotations:** the image digest stays frozen, multiple artifacts can
attach (patch metadata + SBOM + signatures), and this is exactly what the API was designed for.

**Patched image tags:** `<tag>-patched-YYYYMMDD` (e.g. `1.0-patched-20261016`).

Patch metadata payload (Referrer):
```json
{
  "parentDigest": "sha256:abc123...",
  "parentTag": "1.0",
  "patchedAt": "2026-10-16T19:17:54Z",
  "mode": "comprehensive",
  "platform": "linux/amd64"
}
```

### Hot swap

A hot swap replaces the image on a running Deployment **without changing declarative source**:

```
kubectl set image deployment/<svc> <svc>=<repo>:<tag>@<digest>
```

The digest is pinned so the exact patched artifact is guaranteed. A rolling update is triggered and
old pods are progressively replaced. Caretaker's `KindProvider` performs exactly this; the
`SimulatedProvider` is an offline fallback that only rewrites a JSON field.

### Provider abstraction

`scripts/clusters.py` defines `Provider` with `list_deployments(cluster_id)`, `get_deployment(...)`,
and `set_image(cluster_id, service, repo, tag, digest)`:

- **`SimulatedProvider`** — file-backed (`clusters/<id>.json`); appends an event log on `set_image`.
- **`KindProvider`** — real cluster via `kubectl --context <cluster-id> -n <namespace>`. The caretaker
  cluster id maps directly to the kube-context name.

`config/clusters.json` holds the cluster registry: `{id, stage, provider, namespace}`. Defaults:
`test-1` (testing/simulated), `staging-1` (staging/simulated), `prod-a`/`prod-b`
(production/simulated), `kind-caretaker` (production/kind, namespace `caretaker`).

### Deployment pipeline model

- **Testing** — single-service health checks; multiple concurrent versions allowed.
- **Staging** — service interaction checks; one version per service.
- **Production** — live traffic; one version per service (normally).

Patching strategy: when a patched image is created, deploy it to testing immediately. The same
version must never be in both test and prod — if prod has `1.0`, testing gets `1.0-patched-*`.

**Version guard:** deploying an older `-patched-` tag is refused when a **newer base version** of the
service is already deployed in any cluster. This prevents a v1 patch from landing while a v2 rollout
is underway. Override with `--force`.

---

## System Components

### 1. Cluster registry + Providers (`config/clusters.json`, `scripts/clusters.py`)
Defines clusters and abstracts over simulated vs. real (kind) clusters. `SimulatedProvider` is a
fallback; `KindProvider` is the real implementation (`kubectl get deployments -o json` /
`kubectl set image ...`).

### 2. caretaker CLI (`scripts/caretaker.py`)
Primary interface:
- `clusters` — list clusters
- `deployments [--cluster ID] [--service SVC]` — list running deployments
- `status [--service SVC]` — stale summary across clusters
- `swap --service SVC --image REF --clusters IDS [--dry-run] [--force]` — hot-swap an image
- `promote --service SVC --from CLUSTER --to CLUSTERS [--dry-run] [--force]` — copy a deployment
- `patch --service SVC --clusters IDS [--dry-run] [--force]` — Copa patch + push + hot-swap

Registry helpers: `registry_tags`, `registry_digest`, `latest_patched_tag`, and the version guard
`max_deployed_version` / `version_guard`.

### 3. Image build + patch pipeline
- `scripts/push-images.sh` — builds one unique image per service from
  `dockerfiles/<svc>.Dockerfile` (each COPYs a unique marker `dockerfiles/content/<svc>.txt`) and
  pushes `test/<svc>:1.0`.
- `scripts/patch-all.sh` — batch-patches registry images with Copa, tags `-patched-YYYYMMDD`, pushes,
  and attaches Referrers via ORAS. Supports `--include`/`--exclude` repo filters.
- `caretaker patch` — the single-image form (Copa + push + hot-swap).

### 4. kind bootstrap (`scripts/kind-setup.sh`)
Creates a real kind cluster on podman (requires `KIND_EXPERIMENTAL_PROVIDER=podman`), preloads images
with `kind load docker-image`, applies 6 Deployments (namespace `caretaker`), and installs a
**containerd registry mirror** so digest-pinned pulls from `localhost:5000` work from inside the node.

### 5. Data generator (`scripts/generate_cluster_data.py`)
Queries every provider live (so the real kind cluster is included) and emits
`web/src/lib/cluster-data.ts`:
`{generatedAt, clusters[], deployments[], events[], patches[]}`.

### 6. SBOM Checker
Verifies image patch state by comparing package versions against Copa-native patch manifests.

**Patch manifest format (Copa v1alpha1):**
```json
{
  "apiVersion": "v1alpha1",
  "metadata": { "os": { "type": "debian", "version": "12.11" }, "config": { "arch": "amd64" } },
  "updates": [
    { "name": "openssl", "installedVersion": "3.0.13", "fixedVersion": "3.0.14", "vulnerabilityID": "CVE-2024-1001" }
  ]
}
```
`vulnerabilityID` is just a label (CVE, internal bug ID, vendor advisory, …).

**SBOM as OCI Referrer:**
```bash
trivy image --format cyclonedx --output sbom.cdx.json localhost:5000/test/api:1.0-patched-20261016
oras attach --artifact-type "application/vnd.cyclonedx+json" \
  localhost:5000/test/api:1.0-patched-20261016 sbom.cdx.json --plain-http
```
Version comparison is Debian-style using `>=` (installed >= fixed ⇒ satisfied).

Files: `patch-manifests/mock-patches.json`, `web/src/lib/sbom-checker.ts`, `scripts/check-sbom.ts`.

### 7. Status Dashboard (Web UI)
SolidStart v2 web app (Vite + Nitro), UnoCSS, SSR disabled, driven by
`web/src/lib/cluster-data.ts`.

**Operational model:** the system AUGMENTS existing deployment pipelines. Periodically check whether
the version running in production is stale; if so, patch with Copa and hot-swap. The original team's
pipeline is untouched.

**Pages:**
- **Overview (`/`)** — clusters grid + stats (Clusters / Deployments / Need Patching / Critical
  Missing) + Available Patches.
- **Cluster Detail (`/cluster/[id]`)** — per-service deployments (image, digest, replicas, status)
  and hot-swap events.
- **Service Across Clusters (`/service/[name]`)** — every cluster running the service, plus the
  missing-patches table when stale.

Data flow: `cluster-data.ts` (generated) → `web/src/routes/data.ts`
(`getOverview`, `getClusterDetail`, `getServiceAcrossClusters`).

---

## Data Flow

```
┌──────────────┐   ┌───────────────┐   ┌───────────────────┐
│ Zot Registry  │──▶│ patch-all.sh  │◀──│ per-service images│
│ (images +     │   │ caretaker     │   │ dockerfiles/      │
│  Referrers)   │   │ patch (Copa)  │   └───────────────────┘
└──────┬───────┘   └───────┬───────┘
       │                   │ hot-swap (kubectl set image)
       │                   ▼
       │           ┌───────────────────────────────┐
       │           │ Clusters (Provider abstraction)│
       │           │ simulated: clusters/*.json     │
       │           │ real: kind-caretaker (kubectl) │
       │           └───────────────┬───────────────┘
       │                           │
       ▼                           ▼
┌──────────────┐         ┌──────────────────┐
│ Referrers API │         │ generate_cluster_ │
│ SBOM Checker  │         │ data.py → TS      │
└──────────────┘         └────────┬──────────┘
                                  ▼
                         ┌──────────────────┐
                         │  Web UI          │
                         │  Overview /      │
                         │  Cluster /       │
                         │  Service         │
                         └──────────────────┘
```

---

## Scope

### In Scope
- Local Zot registry with real per-service images (api auth worker web payments cache)
- Copa comprehensive patching + `-patched-YYYYMMDD` tags + Referrers (patch metadata + SBOM)
- Real kind cluster on podman with in-place image hot-swap (`kubectl set image`)
- Cluster registry + Provider abstraction (simulated + real)
- caretaker CLI (clusters/deployments/status/swap/patch/promote) with a version guard
- Web UI: cluster overview, cluster detail, service-across-clusters
- SBOM checker (Trivy + Copa-native patch manifests)
- Empirical timing measurements

### Out of Scope (Pinned)
- Multi-base-image inheritance tracking
- Rollback detection
- Image automation tooling (Argo CD Image Updater, Flux, Renovate)
- Networking configuration (ingress, service mesh, load balancers)
- Per-customer or per-region production isolation
- WASM compilation of Trivy/Copa (not feasible)
- Email/notification system for deployment alerts

---

## Future Tasks

### NVD CVE Fetcher
Fetch real CVE data from the NVD API (real IDs/severity/descriptions) to generate realistic patch
manifests. Note: NVD does not provide distro-specific fix versions. **Priority: Medium.**

### UI enhancements
- Patch page: list images + last-patched timestamps, trigger patching from the UI
- "Verify Patches" button wired to the SBOM checker
- Live cluster auto-refresh (regenerate `cluster-data.ts` on demand)

### Slack/email alerting for stale deployments
Notify when a canonical patch exists but a cluster is past the reconciliation window.

---

## Testing Strategy

1. **Unit tests** for query logic (patch state, aggregation) and the version guard.
2. **Fixture data** with known expected outputs.
3. **Edge cases:** image built before any patches; built after all patches; patch released exactly at
   the reconciliation boundary; per-cluster version divergence; newer-version guard halting an older patch.

**Meaningfulness check:** can a human immediately see which clusters are at risk, and does the output
answer the core queries?

---

## Technology Choices (Prototype)

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Web UI | SolidStart v2 (TypeScript) | Fast dev experience, good for dashboards |
| Styling | UnoCSS | Tailwind-compatible, lightweight |
| Cluster state | Live clusters (Provider abstraction) | Real hot-swap; simulated fallback offline |
| Real cluster | kind on podman-remote | Genuine K8s API + `kubectl set image` |
| Scripts | Bash (patching) + Python (clusters/CLI/data) + Node (SBOM) | Match tool requirements |
| Patch Format | Copa v1alpha1 | Native Copa format |
| SBOM | CycloneDX via Trivy | Standard format, simple extraction |
| Patching | Copa (external binary) | Patch without rebuilds |
| Registry | Zot (local OCI registry) | Lightweight, supports Referrers API |
| Referrers | ORAS CLI | Standard tool for attaching artifacts |
| CLI tooling | kubectl + kind in `bin/` | Pinned versions |

---

## Success Criteria

The prototype is successful if:
1. You can run a command and see which clusters run images missing patches.
2. You can run a command and see what patches need to be applied for a given week.
3. The output makes it obvious which clusters are at risk.
4. A real hot-swap can be demonstrated end-to-end (Copa patch → registry → `kubectl set image`).
5. The system is understandable enough to demo.
