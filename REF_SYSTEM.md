# REF_SYSTEM.md — Reference Implementation Design

What a meaningful reference implementation looks like, how we test it, and what tradeoffs we're making for the prototype.

---

## Goal

A working local system that demonstrates the two core queries:
1. **Query A:** "Given an image tag, which patches does it have (and which is it missing)?"
2. **Query B:** "Given a time range, which patches need to be applied, and to which images?"

The system should make the gap between "patch available" and "patch in production" visible and measurable.

---

## Core Concepts

### Patch Registry
A record of CVE patches and when they were released. This is the ground truth for "what patches exist."

For the prototype: a JSON file or SQLite database with entries like:
```json
{
  "id": "CVE-2024-1234",
  "severity": "critical",
  "package": "openssl",
  "fixed_version": "3.0.13",
  "released_at": "2024-09-15T10:00:00Z",
  "reconciliation_window_days": 7
}
```

### Image Registry
A record of container images, their lineage (originator + parent digests), and build timestamps.

**Registry:** Zot (local OCI registry on port 5000). All major registries (Docker Hub, ECR, GCR, ACR, Harbor, GHCR) implement the same OCI Distribution API.

**Digest-based lineage model:** Tags are simple feature identifiers (`api:4.1`). Patched images are new digests that track their lineage back to the original feature build via OCI Referrers API.

```
Feature build: api:4.1 → digest A (originator=A, parent=base digest)
  ├── Patched: digest B (originator=A, parent=A)     ← first Copa patch
  │     ├── Referrer: patch-manifest (parent=A, patchedAt=...)
  │     └── Referrer: SBOM (CycloneDX)
  └── Patched: digest D (originator=A, parent=A)      ← alternative patch path
```

**OCI Referrers API (OCI 1.1, Feb 2024):**
- Manifests have a `subject` field pointing to another manifest
- Query: `GET /v2/<name>/referrers/<digest>` returns OCI Index of artifacts
- Each referrer has an `artifactType` field (e.g., `application/vnd.cyclonedx+json`)
- Zot supports Referrers API fully
- ORAS CLI: `oras attach <image> file.json --artifact-type "application/vnd.caretaker.patch-manifest.v1+json"`

**Why Referrers over manifest annotations:**
- Image digest stays frozen (annotations would change it)
- Multiple artifacts can attach (patch metadata + SBOM + signatures)
- This is what Referrers API is designed for

**Patched image tags:** `<tag>-patched-YYYYMMDD` format (e.g., `1.0-patched-20261014`)

For the prototype: images in Zot registry with Referrers for patch metadata:
```json
{
  "parentDigest": "sha256:abc123...",
  "parentTag": "1.0",
  "patchedAt": "2026-10-14T19:17:54Z",
  "mode": "comprehensive",
  "platform": "linux/amd64"
}
```

### Patch Application Record
A record of which patches were applied to which images, and when.

For the prototype: a JSON file or SQLite database with entries like:
```json
{
  "image_tag": "myapp-4.1",
  "applied_at": "2024-09-19T09:00:00Z",
  "patches_applied": ["CVE-2024-1234", "CVE-2024-5678"],
  "copacetic_report": "path/to/report.json"
}
```

### Deployment Pipeline Model

**Environments:**
- **Testing** — Single-service health checks. Multiple versions of the same image can run concurrently (different replicas/pods testing different versions simultaneously).
- **Staging** — Service interaction checks. Verifies services work together correctly. One version per service.
- **Production** — Live traffic. One version per service (normally). Deployment rules per service are Kubernetes-based (specifics unknown).

**Pipeline path:** testing → staging → production, automatable per service because each service provides guarantees in a defined way.

**Patching strategy (key insight):**

When a patched image is created, push it to testing immediately. The same version should never be in both test and prod — if prod has `1.0`, testing should get `1.0-patched-*`, not `1.0`.

```
Production has: api:4.1 (stale, missing patches)
Testing has:    api:4.1 (old, will be replaced)

Action: Push api:4.1-patched-20261014 to testing (replaces api:4.1)
```

**Don't search for images to deploy.** We're patching all images anyway — as soon as something hits prod, its patched version goes to testing. No need to scan testing/staging for deployable candidates.

**Multiple production versions:**
If multiple images are active in production (canary, blue/green, rollback), push ALL patched versions to testing.

**No searching for deployable images:** We're patching all images anyway. As soon as something hits prod, its patched version goes to testing. Don't scan testing/staging for candidates.

**What we don't know yet:**
- Specific deployment patterns in production
- Exact Kubernetes health check configurations
- How staging promotion works (automated vs. manual)
- Per-service deployment guarantees

**Future work:**
- Simple health checks (HTTP endpoint, process check)
- Identify cases where patches cause health check failures
- Measure how often patches cause noticeable issues beyond security fixes

---

## System Components

### 1. Mock Data Generator
Creates realistic test data:
- A set of base images with known patch histories
- A set of feature builds at various points in time
- A set of CVE patches released at various times
- Simulated patch application events

### 2. Patch State Resolver (Query A)
Given an image tag, determines:
- What patches were available at the time the image was built
- Which of those patches have been applied to this image
- Which patches are missing (available but not applied)

Logic:
1. Look up the image's build timestamp
2. Query the patch registry for all patches released before that timestamp
3. Query the patch application record for patches applied to this image
4. Diff: missing = (available at build time) - (applied)

### 3. Patch Aggregation Resolver (Query B)
Given a time range, determines:
- Which patches were released in that window
- Which images in the registry need those patches
- What the remediation action is (rebuild on latest base, or apply via Copacetic)

Logic:
1. Query the patch registry for patches released in the time range
2. For each patch, find images that are affected (contain the vulnerable package)
3. For each affected image, check if the patch has already been applied
4. Return: images that need patching, with recommended actions

### 4. Environment Snapshot Manager
A record of which service versions were deployed at any given point in time. This is the GitOps model — each snapshot is equivalent to a git commit that declares the desired state of all services.

**Prototype approach — GitOps Repo:**
Instead of snapshot folders, we use a separate git repo (`gitops/`, gitignored) containing Helm charts. Each commit is a point-in-time snapshot of all services.

```
gitops/
  charts/
    api/
      values.yaml              # image spec (repository, tag, digest)
      values-infra.yaml        # replicas, resources, health checks
      values-testing.yaml      # testing environment overlay
      values-staging.yaml      # staging environment overlay
      values-production.yaml   # production environment overlay
      templates/
        deployment.yaml        # K8s deployment manifest
        _helpers.tpl
    web/
      ...
```

**What each values file captures:**
- Base `values.yaml`: image repository, tag, digest (the application track)
- Env overlays: tag/digest overrides per environment (testing/staging/production)
- Infra values: replicaCount, resourceTier, health checks (infrastructure track)

**Data pipeline:** `scripts/generate_gitops_data.py` reads gitops repo (values files + git log) and generates `web/src/lib/gitops-data.ts` for the UI.

**GitOps-native features:**
- **Diff between commits:** Compare values files between any two commits
- **Drift detection:** Compare current values vs. expected state
- **Audit trail:** Git log = natural point-in-time history
- **Desired state declaration:** Values files ARE the source of truth

**Why gitops repo over snapshot folders:**
- Realistic Helm chart structure (matches production GitOps workflows)
- Git provides built-in history, diff, and rollback
- No custom snapshot management code needed

### 5. Helm Chart Generator
Produces Helm chart artifacts (values files, rendered manifests) that represent real-world deployment state. This lets us demonstrate the patch tracking system's interaction with K8s deployments without running a cluster.

**Chart structure (standard Helm layout):**
```
charts/<service-name>/
  Chart.yaml           # Chart metadata
  values.yaml          # Base defaults (image, replicas, resources)
  values-staging.yaml  # Staging overrides
  values-production.yaml # Production overrides (if needed)
  templates/
    deployment.yaml    # K8s deployment manifest template
```

**Values file captures two independent update tracks:**
```yaml
# values.yaml — single source of truth for a service at a point in time
image:
  repository: registry.example.com/myapp
  tag: "4.1.1726912345"     # Application track (features/patches)
replicaCount: 2             # Infrastructure track
resourceTier: medium        # Infrastructure track (maps to CPU/memory limits)
```

**What the generator produces:**
1. Helm values files per service per snapshot (the primary artifact)
2. Optionally rendered K8s manifests via `helm template` (for inspection)
3. Validated via `helm lint` before storage

**How it integrates with snapshots:**
```
kubernetes-snapshots/
  2026-09-20T10:00:00/
    services.json
    helm-values/
      api.yaml          # ← generated by Helm Chart Generator
      web.yaml
  2026-09-21T14:30:00/
    services.json
    helm-values/
      api.yaml          # ← tag updated (application change)
      web.yaml
```

**Prototype approach:** Python script that generates values files from a service definition. No actual `helm` CLI dependency — produces valid YAML that `helm lint` and `helm template` can consume.

### 6. SBOM Checker
Verifies image patch state by comparing package versions against Copa-native patch manifests.

**Patch manifest format (Copa v1alpha1):**
```json
{
  "apiVersion": "v1alpha1",
  "metadata": {
    "os": { "type": "debian", "version": "12.11" },
    "config": { "arch": "amd64" }
  },
  "updates": [
    {
      "name": "openssl",
      "installedVersion": "3.0.13",
      "fixedVersion": "3.0.14",
      "vulnerabilityID": "CVE-2024-1001"
    }
  ]
}
```

**SBOM as OCI Referrer:**
SBOMs are attached to patched images as Referrers using ORAS:
```bash
# Generate SBOM with Trivy
trivy image --format cyclonedx --output sbom.cdx.json localhost:5000/test/python-slim:1.0-patched

# Attach as Referrer
oras attach --artifact-type "application/vnd.cyclonedx+json" \
  localhost:5000/test/python-slim:1.0-patched sbom.cdx.json --plain-http
```

**How it works:**
1. Generate CycloneDX SBOM via Trivy: `trivy image --format cyclonedx <image>`
2. Parse SBOM to extract package name → version map
3. For each patch in manifest, check if installed version >= fixed version
4. Report: satisfied (✓), missing (✗), unknown (?)

**Version comparison:** Debian-style comparison (handles epochs, revisions, suffixes). Uses `>=` — if installed version is newer than required, patch is satisfied.

**Files:**
- `patch-manifests/mock-patches.json` — mixed CVE + non-CVE patches
- `web/src/lib/sbom-checker.ts` — TypeScript library (CLI + UI integration)
- `scripts/check-sbom.ts` — CLI wrapper

**Usage:**
```bash
# CLI
node --experimental-strip-types scripts/check-sbom.ts \
  --image nginx:1.27 \
  --manifest patch-manifests/mock-patches.json
```

### 7. Status Dashboard (Web UI)
SolidStart v2 web application showing patching status, stability, and deployment history.

**Tech stack:**
- SolidStart v2 (Vite 8 + Nitro v3)
- UnoCSS (Tailwind-compatible utility classes)
- SSR disabled (`solidStart({ ssr: false })`)
- Static data from gitops repo (`web/src/lib/gitops-data.ts`)

**Operational model:** The system AUGMENTS existing deployment pipelines, not replaces them. Weekly (or so), check if the feature version in prod is stale. If stale, patch with Copa and deploy to test. The original team handles testing → staging → prod on their own schedule.

**Pages:**

**1. Patching Overview (`/)`)
- Stats row: active services, up-to-date, need-patching, total missing patches, critical missing
- Service cards: sorted by stale-first, showing patching status, missing patches
- Git history section: recent commits from gitops repo

**2. Service Deep Dive (`/service/[name]`)`
- Feature line status: current tag, missing patches, whether patched version exists
- Environments grid: testing/staging/production versions per service
- Infrastructure: replicas, resource tier
- All digests table: complete image history
- Git history: service-specific commits

**Navigation:** Sidebar in Router root callback (SolidStart v2 convention)

**Data flow:**
- `web/src/lib/gitops-data.ts` — auto-generated from gitops repo (CURRENT_STATE, GIT_HISTORY, SERVICE_HISTORIES, MOCK_PATCHES)
- `web/src/routes/data.ts` — `getServiceOverview()`, `getServiceDetail(name)`
- `web/src/lib/sbom-checker.ts` — SBOM verification (not yet integrated into UI)

**Future UI enhancements (not yet implemented):**
- Patch All page (trigger Copa patching via UI)
- Deploy to Testing page (show deployment decisions)
- Last Patched tracking per image
- SBOM-based patch verification per service

---

## Future Tasks

### NVD CVE Fetcher
Fetch real CVE data from the NVD API to generate realistic patch manifests.

**Why:** Currently using mock patches with real CVE IDs but fake package/fix data. NVD provides real CVE IDs, severity scores, and descriptions — but NOT package-specific fix versions (that's distro-specific).

**Priority:** Medium — mock data is sufficient for prototype demo.

### Real Image Integration — IN PROGRESS
Three real images pushed to Zot registry: `test/python-slim:1.0`, `test/node-slim:1.0`, `test/ubuntu:1.0`.

**Remaining:** Patch all images with Copa, attach Referrers, deploy to testing.

### Patch All + Deploy to Testing UI Pages
- Patch All page: show all images, trigger Copa patching, show last-patched timestamps
- Deploy to Testing page: show deployment decisions based on `deploy-to-testing.sh` logic

---

## Prototype Scope

### In Scope
- Local Zot registry with real images (python-slim, node-slim, ubuntu)
- Copa patching with comprehensive mode
- OCI Referrers API for patch metadata + SBOM attachment
- GitOps repo with Helm charts for 6 services
- Deploy-to-testing decision logic
- Web UI for patching overview and service detail
- SBOM checker (Trivy + Copa-native patch manifests)
- Patch manifest format matching Copa v1alpha1
- Empirical timing measurements for Trivy/Copa

### Out of Scope (Pinned)
- Running Kubernetes (minikube/kind/k3d)
- Multi-base-image inheritance tracking
- Rollback detection
- Image automation tooling (Argo CD Image Updater, Flux, Renovate)
- Networking configuration (ingress, service mesh, load balancers)
- Per-customer or per-region production environment isolation
- WASM compilation of Trivy/Copa (not feasible — both are complex Go binaries with external dependencies)
- Actual Kubernetes cluster deployment (we produce Helm values, don't deploy)
- Email/notification system for deployment alerts

---

## UI Integration Plans

### Current State
- UI reads from `web/src/lib/gitops-data.ts` (generated from gitops repo)
- Overview page shows service cards with patching status
- Service detail page shows feature line status, environments, digests

### Planned UI Enhancements

**1. Patch All page:**
- List all images in registry with last-patched timestamps
- "Patch All" button triggers `patch-all.sh`
- Show patching progress and results

**2. Deploy to Testing page:**
- Show deployment decisions from `deploy-to-testing.sh`
- Per-service: prod image, patched version available, testing state
- "Deploy" button updates gitops values-testing.yaml

**3. SBOM-based patch verification:**
- "Verify Patches" button on service detail page
- Calls SBOM checker via server function
- Shows which patches are satisfied/missing based on actual package versions

---

## Testing Strategy

### How we validate the prototype works
1. **Unit tests** for the query logic (Patch State Resolver, Patch Aggregation Resolver)
2. **Fixture data** with known expected outputs:
   - "Image built on date X should have patches A, B, C but not D"
   - "Time range Y should require patches D, E, F applied to images G, H"
3. **Edge cases to cover:**
   - Image built before any patches exist
   - Image built after all patches are applied
   - Patch released exactly at the reconciliation boundary
   - Multiple images with different build times needing different patch sets

### How we validate the system is meaningful
- Can a human look at the output and immediately understand which images are at risk?
- Does the output directly answer the two core queries?
- Is the gap between "patch available" and "patch applied" clearly visible?

---

## Technology Choices (Prototype)

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Web UI | SolidStart v2 (TypeScript) | Fast dev experience, good for dashboards |
| Styling | UnoCSS | Tailwind-compatible, lightweight |
| Data Store | GitOps repo (Helm charts + git log) | Realistic, built-in history, no custom code |
| Scripts | Bash (patching) + Python (data gen) + Node (SBOM) | Match tool requirements |
| Patch Format | Copa v1alpha1 | Native Copa format, can feed directly to `copa patch --scanner native` |
| SBOM | CycloneDX via Trivy | Standard format, simple package version extraction |
| Patching | Copa (external binary) | Direct patching without rebuilds, empirical timing verified |
| Registry | Zot (local OCI registry) | Lightweight, supports Referrers API |
| Referrers | ORAS CLI | Standard tool for attaching artifacts to OCI images |

**Current state:** Zot registry on port 5000, Copa + Trivy + ORAS in `bin/`, gitops repo for Helm charts, SolidStart for UI.

---

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Zot Registry    │────▶│  Patch Script     │◀────│ GitOps Repo     │
│  (images +       │     │  (Copa patch +    │     │ (Helm charts +  │
│   Referrers)     │     │   ORAS attach)    │     │  git history)   │
└─────────────────┘     └──────────────────┘     └─────────────────┘
        │                       │                        │
        │    ┌──────────────────┼────────────────────────┘
        │    │                  │
        ▼    ▼                  ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Referrers API   │     │  SBOM Checker    │     │ Data Generator  │
│  (patch metadata │     │  (Trivy +        │     │ (gitops → TS)   │
│   + SBOM)        │     │  Copa-native)    │     │                 │
└─────────────────┘     └──────────────────┘     └─────────────────┘
        │                       │                        │
        └───────────────────────┼────────────────────────┘
                                ▼
                        ┌──────────────────┐
                        │  Web UI          │
                        │  (SolidStart)    │
                        │                  │
                        │  - Overview      │
                        │  - Service Deep  │
                        └──────────────────┘
```

---

## Success Criteria

The prototype is successful if:
1. You can run a command and see which images are missing patches
2. You can run a command and see what patches need to be applied for a given week
3. The output makes it obvious which images are "at risk" (past reconciliation window)
4. The system is understandable enough to demo to your dad's coworkers
