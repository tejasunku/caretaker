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

### 6. Status Dashboard (Optional for Prototype)
A simple CLI or web UI that shows:
- Current patch status for all images
- Overdue patches (past reconciliation window)
- Recommended actions
- Environment deployment state

---

## Prototype Scope

### In Scope
- Mock data generation with realistic patch/image/timestamp relationships
- Both queries (A and B) working against mock data
- A CLI interface to run queries and see results
- Clear output showing the "gap" between available and applied patches
- Environment snapshot tracking (which service versions are where)
- Helm chart generation (values files per service per snapshot)
- Helm chart validation (lint, template rendering)

### Out of Scope (Pinned)
- Actual Copacetic integration (we'll simulate patch application)
- Running Kubernetes (minikube/kind/k3d)
- Real Docker image inspection
- Multi-base-image inheritance tracking
- Rollback detection
- Actual image push/pull to a registry
- Image automation tooling (Argo CD Image Updater, Flux, Renovate)
- Networking configuration (ingress, service mesh, load balancers)
- Per-customer or per-region production environment isolation

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
| Language | Python or Go | Fast to prototype, good JSON handling |
| Data Store | SQLite or JSON files | No external dependencies, easy to inspect |
| CLI | Click (Python) or cobra (Go) | Simple command interface |
| Mock Data | Faker + custom generators | Realistic but deterministic data |

**Decision needed:** Python vs. Go. Python is faster to prototype; Go matches Copacetic's ecosystem. Recommend Python for speed, with a note that production would likely be Go.

---

## Data Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Patch Registry  │────▶│  Query Resolver   │◀────│ Image Registry   │
│  (when patches   │     │  (A: per-image    │     │ (when images     │
│   are released)  │     │   B: per-range)   │     │  were built)     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                                │
                                ▼
                        ┌──────────────────┐
                        │  Environment      │◀──── Helm Chart Generator
                        │  Snapshots        │     (produces values.yaml
                        │  (who is where)   │      showing deployed versions)
                        └──────────────────┘
                                │
                                ▼
                        ┌──────────────────┐
                        │  Status Output    │
                        │  (what's missing, │
                        │   what's needed,  │
                        │   where things are)│
                        └──────────────────┘
```

---

## Success Criteria

The prototype is successful if:
1. You can run a command and see which images are missing patches
2. You can run a command and see what patches need to be applied for a given week
3. The output makes it obvious which images are "at risk" (past reconciliation window)
4. The system is understandable enough to demo to your dad's coworkers
