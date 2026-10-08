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
A record of container images, their build timestamps, and which base image they were built from.

For the prototype: a JSON file or SQLite database with entries like:
```json
{
  "tag": "myapp-4.1",
  "built_at": "2024-09-18T14:30:00Z",
  "base_image": "ubuntu:22.04",
  "base_digest": "sha256:abc123...",
  "layers": ["sha256:...", "sha256:..."]
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

**Prototype approach — Snapshot Folders:**
Instead of modeling git repos, we use a folder-based snapshot system. Each timestamped folder contains a full snapshot of everything that would be required in an actual GitOps setup:

```
kubernetes-snapshots/
  2026-09-20T10:00:00/
    services.json        # {"api": {"tag": "4.1.1726912345", "sha": "sha256:..."}, "web": ...}
    helm-values/
      api.yaml
      web.yaml
    registry.json        # Registry metadata snapshot (tags, digests, push times)
  2026-09-21T14:30:00/
    services.json
    helm-values/
      api.yaml           # tag updated (application change)
      web.yaml
    registry.json        # updated registry state
```

**What each snapshot contains:**
- `services.json`: Map of service name → image tag + SHA (the desired state declaration)
- `helm-values/`: Actual Helm values.yaml files showing deployed versions (realistic K8s artifacts)
- `registry.json`: Point-in-time registry metadata — which tags exist, their digests, when pushed

**What changes between snapshots (diff types):**
- **Tag update:** Image tag changed for an existing service (feature build or patch)
- **Infrastructure update:** replicaCount or resourceTier changed (no image change)
- **Service added:** New service appears in services.json (not in previous snapshot)
- **Service removed:** Service disappears from services.json (was in previous snapshot)
- **Multiple changes:** Combination of the above (rare but possible)

**GitOps-native features modeled:**
- **Diff between snapshots:** Compare `services.json` between any two folders to see what changed — including service additions and removals.
- **Drift detection:** Compare current expected state vs. what's in the latest snapshot.
- **Reconciliation status:** Track whether services have converged (synced, pending, degraded).
- **Audit trail:** Folder listing = `git log` equivalent.

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
    },
    {
      "name": "libssl3",
      "installedVersion": "3.0.13",
      "fixedVersion": "3.0.14-internal1",
      "vulnerabilityID": "INTERNAL-2024-001"
    }
  ]
}
```

**Key properties:**
- `vulnerabilityID` is just a string label — can be CVE, internal bug ID, vendor patch ID, or policy reference
- Format matches Copa's native report format (`--scanner native`)
- Can be fed directly to Copa for patching, or used for SBOM-based verification

**How it works:**
1. Generate CycloneDX SBOM via Trivy: `trivy image --format cyclonedx <image>`
2. Parse SBOM to extract package name → version map
3. For each patch in manifest, check if installed version >= fixed version
4. Report: satisfied (✓), missing (✗), unknown (?)

**Version comparison:** Debian-style comparison (handles epochs, revisions, suffixes). Uses `>=` — if installed version is newer than required, patch is satisfied.

**Files:**
- `patch-manifests/mock-patches.json` — mixed CVE + non-CVE patches
- `patch-manifests/cve-only.json` — CVE-only patches for comparison
- `web/src/lib/sbom-checker.ts` — TypeScript library (CLI + UI integration)
- `scripts/check-sbom.ts` — CLI wrapper

**Usage:**
```bash
# CLI
node --experimental-strip-types scripts/check-sbom.ts \
  --image nginx:1.27 \
  --manifest patch-manifests/mock-patches.json

# JSON output
node --experimental-strip-types scripts/check-sbom.ts \
  -i nginx:1.27 -m patch-manifests/mock-patches.json --json
```

### 7. Status Dashboard (Web UI)
SolidStart v2 web application showing patching status, stability, and deployment history.

**Tech stack:**
- SolidStart v2 (Vite 8 + Nitro v3)
- UnoCSS (Tailwind-compatible utility classes)
- SSR disabled (`solidStart({ ssr: false })`)
- Static data from snapshot folders (`kubernetes-snapshots/`)

**Pages:**

**1. Patching Overview (`/`)**
- Summary banner: compliant/at-risk/non-compliant counts
- Stats row: active services, compliant, at-risk, non-compliant, total missing patches, testing gaps, tag mutations
- Service cards: sorted by severity, showing compliance status, missing patches, deadline countdown
- Stability Concerns section: images that skipped testing
- Image Integrity Concerns section: tag mutations (same tag, different SHA)

**2. Service Deep Dive (`/service/[name]`)**
- Patching status cards: current status, current tag, days until deadline
- Testing Gap Alerts: red box showing SHAs deployed to staging/prod without testing
- Stability Pipeline: horizontal Testing→Staging→Production flow per image
  - Bold borders = met required time (2d testing, 2d staging)
  - Dashed borders = didn't meet
- Reconciliation Windows: patches with introduced/deadline weeks, feature line compliance
- Deployment Timeline: per-environment history with durations
- Snapshot History table

**3. Deployment History (`/history`)**
- Timeline of snapshots (newest first)
- Each snapshot shows: added services, removed services, changed services
- Change types: tag_update, sha_change, replica_change, resource_tier_change

**Navigation:** Sidebar in Router root callback (SolidStart v2 convention)

**Data flow:**
- `web/src/lib/snapshot-data.ts` — auto-generated from `kubernetes-snapshots/`
- `web/src/lib/mock-data.ts` — patches, environment deployments
- `web/src/routes/data.ts` — `getServiceOverview()`, `getServiceDetail(name)`
- `web/src/lib/compliance.ts` — `computePatchingStatus()`
- `web/src/lib/stability.ts` — `computeStability()`
- `web/src/lib/sbom-checker.ts` — SBOM verification (not yet integrated into UI)

**Reference date:** `REFERENCE_DATE = 2026-09-25T12:00:00Z` (fixed for deterministic mock data)

**Future UI enhancements (not yet implemented):**
- SBOM-based patch verification per service (using `sbom-checker.ts`)
- Patch manifest management UI
- Real image integration (replace mock images with registry images)
- Copa integration ("Apply Patches" button generating native reports)

---

## Future Tasks

### NVD CVE Fetcher
Fetch real CVE data from the NVD API to generate realistic patch manifests.

**Why:** Currently using mock patches with real CVE IDs but fake package/fix data. NVD provides real CVE IDs, severity scores, and descriptions — but NOT package-specific fix versions (that's distro-specific).

**Approach:**
1. Query NVD API for recent CVEs (free, no API key required for basic usage)
2. Extract: CVE ID, severity, description, affected CPE
3. Cross-reference with distro security trackers (Debian, Ubuntu, Red Hat) for fix versions
4. Generate Copa v1alpha1 manifests

**Limitations:** NVD doesn't provide package fix versions. Need to combine with distro-specific data (e.g., Debian Security Tracker API, Ubuntu CVE Tracker).

**Priority:** Medium — mock data is sufficient for prototype demo.

### Real Image Integration
Replace mock images with real container images for more realistic demos.

**Selected images:**
- **Minimal/OSless:** `gcr.io/distroless/static-debian12` (if Copa supports it) or `alpine:3.19`
- **Python:** `python:3.11-slim` (Debian 12, older for vulnerability demo)
- **Node:** `node:18-slim` (Debian 12, older for vulnerability demo)
- **Ubuntu base:** `ubuntu:22.04` (LTS, good Copa support)

**Why older versions:** Shows how the patch tracking system identifies missing patches in images that haven't been rebuilt recently.

**Transition path:**
1. Pull real images
2. Scan with Trivy to get real SBOMs
3. Check against mock patch manifests (demonstrates concept)
4. Later: generate real patch manifests from Trivy CVE data

### Trivy→Copa Converter (built, not yet integrated)
`web/src/lib/trivy-to-copa.ts` converts Trivy JSON reports to Copa v1alpha1 format. CLI at `scripts/trivy-to-copa.ts`. Not yet integrated into UI or workflow.

---

## Prototype Scope

### In Scope
- Mock data generation with realistic patch/image/timestamp relationships
- Both queries (A and B) working against mock data
- Environment snapshot tracking (which service versions are where)
- Helm chart generation (values files per service per snapshot)
- Helm chart validation (lint, template rendering)
- Web UI for patching overview, service detail, deployment history
- SBOM checker (Trivy + Copa-native patch manifests)
- Patch manifest format matching Copa v1alpha1
- Empirical timing measurements for Trivy/Copa

### Out of Scope (Pinned)
- Actual Copacetic patching execution (we verify patch state, don't apply patches)
- Running Kubernetes (minikube/kind/k3d)
- Real Docker image inspection (beyond Trivy SBOM)
- Multi-base-image inheritance tracking
- Rollback detection
- Actual image push/pull to a registry
- Image automation tooling (Argo CD Image Updater, Flux, Renovate)
- Networking configuration (ingress, service mesh, load balancers)
- Per-customer or per-region production environment isolation
- WASM compilation of Trivy/Copa (not feasible — both are complex Go binaries with external dependencies: network access for vuln databases, filesystem access for image inspection, BuildKit/container runtime for patching. Go can compile to WASM, but these tools' dependencies don't work in browser environments.)
- WASM compilation of Trivy/Copa (not feasible — both are complex Go binaries with external dependencies)

---

## UI Integration Plans

### Current State
- UI uses mock data (`web/src/lib/mock-data.ts`) with hardcoded patches and deployments
- Service detail page shows patching status, stability pipeline, reconciliation windows
- No connection to real image scanning

### Future UI Enhancements

**1. SBOM-based patch verification per service:**
- Add "Verify Patches" button on service detail page
- Calls SBOM checker via server function
- Shows: which patches are satisfied/missing based on actual package versions
- Displays version comparison details (installed vs. required)

**2. Patch manifest management:**
- UI to view/edit patch manifests
- Upload custom manifests (Copa v1alpha1 format)
- Show which manifests apply to which services

**3. Real image integration:**
- Replace mock images with real registry images
- Pull SBOMs on-demand or pre-cache them
- Show real package inventory alongside mock patch data

**4. Copa integration (future):**
- "Apply Patches" button that generates Copa native report
- Download report for use with `copa patch --scanner native`
- Track which patches were applied via Copa (vs. rebuild)

### Transition Path: Mock → Real

**Phase 1 (current):** Mock images + mock patches
- All data hardcoded in `mock-data.ts`
- SBOM checker works but has no real images to scan

**Phase 2:** Real images + mock patches
- Pull real images from registry (e.g., nginx:1.27)
- Scan with Trivy to get real SBOMs
- Check against mock patch manifests
- Demonstrate: "this real image is missing these patches"

**Phase 3:** Real images + real patches
- Integrate with actual patch registry (Trivy CVE data + custom manifests)
- Real-time SBOM scanning
- Copa integration for patch application
- Full pipeline: scan → check → patch → verify

**Challenge:** Mock data uses synthetic package versions that don't exist in real images. Transition requires either:
- (a) Keep mock patch manifests but scan real images (show which mock patches would apply)
- (b) Generate real patch manifests from Trivy CVE data for real images
- (c) Maintain both mock and real data paths, switch via config

**Recommendation:** Start with (a) — use real images with mock patches to demonstrate the concept, then transition to (b) for realistic data.

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
| Data Store | JSON files (snapshot folders) | No external dependencies, easy to inspect, GitOps-like |
| Scripts | Python (data generation) + Node (SBOM checker) | Python for snapshot tooling, Node for TypeScript integration |
| Patch Format | Copa v1alpha1 | Native Copa format, can feed directly to `copa patch --scanner native` |
| SBOM | CycloneDX via Trivy | Standard format, simple package version extraction |
| Patching | Copa (external binary) | Direct patching without rebuilds, empirical timing verified |

**Current state:** TypeScript/SolidStart for UI, Python for data generation, Node for SBOM checking. Copa and Trivy as external binaries.

---

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Patch Registry  │────▶│  Query Resolver   │◀────│ Image Registry   │
│  (Copa v1alpha1  │     │  (A: per-image    │     │ (when images     │
│   manifests)     │     │   B: per-range)   │     │  were built)     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                                │
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
        ┌──────────────┐ ┌──────────────┐ ┌──────────────────┐
        │  Environment  │ │ SBOM Checker │ │  Helm Chart      │
        │  Snapshots    │ │ (Trivy +     │ │  Generator       │
        │  (who is      │ │  Copa-native │ │  (values.yaml    │
        │   where)      │ │  manifests)  │ │   per service)   │
        └──────────────┘ └──────────────┘ └──────────────────┘
                │               │               │
                └───────────────┼───────────────┘
                                ▼
                        ┌──────────────────┐
                        │  Web UI          │
                        │  (SolidStart)    │
                        │                  │
                        │  - Overview      │
                        │  - Service Deep  │
                        │  - History       │
                        └──────────────────┘
```

---

## Success Criteria

The prototype is successful if:
1. You can run a command and see which images are missing patches
2. You can run a command and see what patches need to be applied for a given week
3. The output makes it obvious which images are "at risk" (past reconciliation window)
4. The system is understandable enough to demo to your dad's coworkers
