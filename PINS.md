# PINS.md — Deferred Decisions

Things we've put a pin in. These are known concerns that are valid but out of scope for the prototype.

---

## Version String Format

**What:** Adding a timestamp to image tags (e.g., `4.1.1726912345`) to encode when an image was last patched.

**Why pinned:** The version string doesn't directly encode which patches were applied or which base image was used. It's a proxy, not ground truth. For a prototype, it's a reasonable shorthand. For production, we need to decide whether the tag is the source of truth or whether we maintain a separate metadata record.

**Open questions:**
- Should the tag encode only the last patch timestamp, or also the base image digest?
- What happens when a patch is rolled back? The timestamp would still advance.
- Does the tag format need to be backwards-compatible with existing tagging conventions?

---

## Base Image Provenance

**What:** Tracking which base image a feature build was derived from.

**Why pinned:** You can partially infer the base image from layer inspection (`docker inspect`, Trivy fingerprinting), but you can't get the exact tag without build-time metadata. For a prototype, we'll assume base images are recorded at build time. For production, we may need to require build-time provenance recording or use a base image registry with digest tracking.

**Open questions:**
- Will acquired startups' non-standard base images cause issues?
- Should we enforce a "base image registry" or allow arbitrary bases?
- Do we need to track multi-level base image inheritance (base A → base B → feature)?

---

## Patch Reconciliation Windows

**What:** Patches must be addressed within a time period, but different patches may have different SLAs.

**Prototype approach:** Time is bucketed into weekly intervals (Monday–Sunday). Each patch has:
- `releasedAt` (specific date, kept in data model but not surfaced in UI)
- `reconciliationWindowDays` (7, 14, or 30 days in mock data)

The UI shows: **introduced week** (week containing `releasedAt`) and **deadline week** (week containing `releasedAt + windowDays`). No exact dates shown.

4 weeks = 1 month, so monthly grouping is trivial.

**Open questions:**
- Should reconciliation windows be per-patch, per-severity, or per-image?
- Is the window measured from when the patch is released, or from when it's detected?
- What happens if a patch is released but no image needs it (e.g., not applicable)?

---

## Pinning Base Images

**What:** The possibility that some projects pin specific base image versions rather than always rebuilding on the latest.

**Why pinned:** If a project pins a base image, the build timestamp is no longer a reliable indicator of patch state. The image could be rebuilt on an old base and appear "current" while missing recent patches.

**Open questions:**
- Should we detect when an image's base is stale relative to the latest patched base?
- Is pinning a common enough practice that it needs to be a first-class concern?
- If pinning is allowed, do we need a separate tracking mechanism for pinned vs. unpinned images?

---

## Manifest / Bill of Materials

**What:** Generating or maintaining some sort of manifest that records what patches have been applied to each image.

**Why pinned:** A manifest would be the ground truth for "what patches does this image have?" — more reliable than timestamp inference. But it requires build-time integration to generate and maintain.

**Open questions:**
- Should the manifest be embedded in the image (e.g., as an annotation or layer) or stored externally?
- Who is responsible for generating it — the build pipeline, Copacetic, or a separate system?
- What format? OCI image spec annotations? A sidecar file? A database entry?

---

## Deployment Pipeline Integration

**What:** Images go through testing → staging → production, and the patch state needs to be tracked through this pipeline.

**Why pinned:** The prototype doesn't need to simulate the full deployment pipeline. We can show patch state at the image level without modeling the promotion flow.

**Open questions:**
- Should the system track per-environment patch state (e.g., "this image is patched but not yet tested")?
- What happens if a patch is applied after an image has already been deployed to testing?
- Do we need rollback tracking (i.e., "this image was promoted to production but then rolled back")?

---

## Feature Build Testing Gap

**What:** If a developer tests one version of a feature build all the way to staging, but only tests the newest version in testing, build-time-based patch tracking could put an insufficiently tested image to production.

**Why pinned:** There's no easy way to solve this within the scope of a patch-tracking prototype. This is fundamentally a build system / deployment pipeline governance problem.

**Assumption for prototype:** Feature builds are frozen once tagged — no further mutations under the same tag.

**Open questions:**
- This strongly suggests the build system itself needs modification to prevent untested builds from reaching production.
- Should the patch tracking system flag when an image under a tag has changed (same tag, different SHA)?
- Is this a hard blocker for production, or is it handled by process/policy?

---

## Image SHA Tracking

**What:** There's a system that lists which tagged images are deployed where, but can't tell if images under the same tag are identical.

**Why pinned:** For the prototype, we assume tags are immutable (once tagged, the image doesn't change). In reality, tags are mutable pointers — the same tag could reference different images over time.

**Assumption:** The real system likely stores image SHAs and can query by them. This is a reasonable assumption for production.

**Open questions:**
- Should the prototype track SHAs alongside tags?
- How do we detect "tag drift" (same tag, different content)?
- Is SHA-based lookup already available in the existing deployment system?

---

## Multi-Service Interactions

**What:** From a git repo, we can know what combinations of services were active at what periods of time. But managing a git repo for the prototype adds complexity.

**Why pinned:** The prototype needs to demonstrate how multiple services interact (e.g., "service A and service B were both deployed with these patches at this time") without requiring actual git history management.

**Approach:** Model environment snapshots as point-in-time records. Each snapshot captures which version of each service was deployed. This avoids git complexity while still demonstrating the concept.

**Open questions:**
- Should environment snapshots be manually created or generated from deployment events?
- How granular do snapshots need to be (per-deployment, per-hour, per-day)?
- Is there a K8s-native way to get environment snapshots without git?

---

## Helm Charts Without Kubernetes

**What:** The real system uses Helm charts deployed to K8s clusters. The prototype needs to demonstrate this interaction without running actual Kubernetes.

**Why pinned:** Running K8s locally (minikube, kind, k3d) adds significant complexity. The prototype should produce Helm chart artifacts that match real-world output without requiring a cluster.

**Approach for prototype:** Generate Helm chart YAML files (values.yaml, templates/) that represent what a real deployment would produce. These can be inspected and validated without a cluster. The patch tracking system reads these charts to determine which image versions are deployed where.

**Real-world flow (for context):**
1. Image pushed to registry with new tag
2. Image automation tool (Argo CD Image Updater, Flux Image Automation, Renovate, or custom CI/CD) detects new tag
3. Tool updates Helm chart values in a git repo with the new image tag
4. CD tool (Argo CD, Flux) syncs the cluster to match the git state
5. Git history provides natural point-in-time snapshots of every environment

**Open questions:**
- Should the prototype generate actual Helm chart files, or just model the values?
- Do we need to simulate the image automation detection step?
- How much of the GitOps flow do we need to demonstrate?

---

## GitOps as Reference Model

**What:** Assuming GitOps for the prototype, even though the real system might use something else (push-based CI/CD, custom tooling, etc.).

**Why pinned:** GitOps gives us a clean mental model — desired state as declarative config, point-in-time snapshots via commits, diff between states, drift detection, reconciliation status. All of these are modelable as pure data without touching actual git.

**GitOps-native features we can model as data:**
- **Desired state declaration:** Each environment snapshot is a declarative config (what version of each service should be running).
- **Immutable snapshots:** Each snapshot = one "commit" — a timestamped record of all service versions.
- **Diff between states:** Dict diff between any two snapshots answers "what changed?"
- **Drift detection:** Compare current snapshot vs. expected state.
- **Reconciliation status:** Track whether each service has converged to its desired state (pending, in_progress, synced, degraded).
- **Audit trail:** Snapshot history = git log equivalent.

**Assumption for prototype:** Model all of this as an array of timestamped snapshot objects. No actual git operations needed.

**Open questions:**
- Should drift detection be a query, or a continuously-running check?
- How do we model "reconciliation failed" (e.g., image push failed, K8s rejected the rollout)?
- Is the audit trail enough, or do we need to track *why* each change was made?

---

## GitOps Tool Abstraction Gap

**What:** GitOps tools (Argo CD, Flux) require a literal git repo — there's no abstract specification for repo structure that the tool can operate against. No dry-run or export mode that bypasses git.

**Why pinned:** This is a liability for people wanting to use GitOps in new and interesting ways. For the prototype, we work around this by modeling snapshots as folders.

**Prototype approach — Snapshot Folders:**
```
kubernetes-snapshots/
  2026-09-20T10:00:00/
    services.json        # {"api": {"tag": "4.1.1726912345"}, "web": {"tag": "2.3.1726912000"}}
    helm-values/         # Actual values.yaml per service
      api.yaml
      web.yaml
    registry.json        # Registry metadata snapshot (tags, digests, push times)
  2026-09-21T14:30:00/
    services.json
    helm-values/
      api.yaml
      web.yaml
    registry.json
```

This gives us:
- Point-in-time snapshots (folder = commit)
- Diff between snapshots (compare `services.json`) — including service add/remove
- Audit trail (folder listing = `git log`)
- Realistic Helm artifacts (actual values files)
- Registry visibility (what tags existed at each point)

**Open questions:**
- Should snapshots be auto-generated from deployment events, or manually created?
- How do we handle environments that diverge (e.g., staging has newer patches than production)?
- Is this folder structure sufficient, or do we need to model branch semantics too?

---

## Networking Configuration

**What:** Service networking (ingress, service mesh, load balancers, network policies) in Helm charts.

**Why pinned:** Networking is critical for production but irrelevant to patch tracking. The prototype focuses on image tags and resource config. Networking would add template complexity without demonstrating the core concept.

**Assumption for prototype:** Helm charts will not include networking configuration. Deployment templates will focus on containers, replicas, and resources only.

---

## Environment Structure

**What:** How environments (test, staging, production) are organized across clusters.

**Why pinned:** We don't know the actual internal structure. Different organizations use different models: unified staging with per-customer prod, per-region prod, per-team prod, etc.

**Assumption for prototype:** Unified test and staging environments. Production environment structure is pinned — the prototype doesn't need to model prod isolation.

**Open questions:**
- Is production per-customer, per-region, or per-team?
- Do different prod environments have different patch SLAs?
- Is there a promotion order (test → staging → prod-A → prod-B)?

---

## Update Lineage Tracking

**What:** Two independent tracks of updates can change what's deployed: (1) application updates (feature builds, patches) that change the image tag, and (2) infrastructure updates (replica count, resource tier) that change deployment config.

**Why pinned:** These are logically distinct but both result in Helm chart changes. The prototype needs to capture both without conflating them.

**Prototype approach:** Values files capture both tracks. A diff between two snapshots shows which changed:
- Image tag changed → application update (feature or patch)
- replicaCount / resourceTier changed → infrastructure update
- Both changed → concurrent updates (rare but possible)

**Open questions:**
- Should the system classify changes automatically (tag vs. infra)?
- Is there a审批 process that separates these tracks?
- Do infra updates need their own reconciliation windows?

---

## Registry Choice for Prototype

**What:** Use an existing OCI registry service (Docker Hub, GHCR) or mock one.

**Why pinned:** Spinning up a registry (e.g., distribution/distribution) adds operational complexity. Using an existing service is simpler but requires internet access and account setup.

**Prototype approach:** Registry metadata as JSON files. Each snapshot captures a point-in-time view of what tags exist, their digests, and push timestamps. No actual OCI API calls — just structured data that mirrors what a real registry would provide.

**Registry metadata snapshot format:**
```json
{
  "timestamp": "2026-09-20T10:00:00Z",
  "tags": {
    "api:4.1.1726912345": {"digest": "sha256:abc...", "pushed_at": "2026-09-18T14:30:00Z"},
    "web:2.3.1726912000": {"digest": "sha256:def...", "pushed_at": "2026-09-17T09:00:00Z"}
  }
}
```

**Open questions:**
- If the demo needs to show actual image pushing, which registry should we use?
- Is Docker Hub free tier sufficient, or should we use GHCR (GitHub Container Registry)?
- Should the mock registry expose the same API surface as a real OCI registry?

---

## Real OCI Registry Integration

**What:** Connecting to an actual OCI registry (Docker Hub, ECR, Harbor, etc.) to query tags, digests, and push timestamps.

**Why pinned:** For the prototype, registry metadata is captured as JSON files at snapshot time. Real OCI integration would allow live queries, automated snapshot generation, and drift detection against the actual registry state.

**Prototype approach:** Simulate registry data via JSON. The snapshot system reads from and writes to these JSON files, treating them as the source of truth.

**Production approach (pinned):**
- Query OCI registry REST API for tag listing and manifest details
- Compare registry state against snapshot to detect drift
- Auto-generate registry metadata snapshots on a schedule
- Handle pagination, authentication, rate limiting

**Open questions:**
- Which registry does the team actually use? (affects auth, API surface)
- Should snapshots be generated from registry events (webhooks) or polling?
- How do we handle tags that exist in the registry but not in our snapshot system?

---

## Testing/Staging Overlap and Multiple Versions

**What:** When promoting a new image version, there may be overlap where the old version is still in production while the new version is in testing or staging. Additionally, multiple versions could be tested simultaneously.

**Why pinned:** For the prototype, we show a simplified pipeline: one version per environment (testing, staging, production). In reality:
- While promoting v2 to staging, v1 may still be in production
- Multiple feature branches may be tested concurrently
- A version could be rolled back from staging while another is promoted

**Prototype assumption:** One version per environment, no overlap. When a new version is promoted, the previous version is replaced.

**Open questions:**
- Should we track the previous version in each environment during transition periods?
- How do we handle concurrent testing of multiple versions (e.g., feature flags, canary deployments)?
- What's the rollback workflow? Does a rolled-back version return to testing, or is it abandoned?
- Should the pipeline view show historical versions that were in each environment, or just the current state?

---

## Trend Analysis and Historical Compliance

**What:** Tracking whether the team is "generally keeping up" with patching cadence over time, and attributing failures to specific causes (late testing vs. tight windows).

**Why pinned:** Requires multiple reference dates or time-series compliance snapshots. The prototype captures point-in-time snapshots, not continuous compliance history.

**Prototype assumption:** Single reference date. Compliance is measured at one point in time, not trended.

**Open questions:**
- How frequently should compliance snapshots be taken? (daily? on every deployment?)
- Should we store compliance results alongside snapshot data?
- How do we attribute delays — is it the testing stage, staging stage, or reconciliation window that's the bottleneck?

---

## Feature Cadence Analysis

**What:** Determining the natural feature build cadence that emerges from testing procedures and compliance processes, and understanding how changing one affects the other.

**Why pinned:** Requires observing actual build frequency over time and simulating the effect of parameter changes. The prototype has mock data with fixed cadence.

**Prototype assumption:** Fixed mock data cadence. No simulation capability.

**Open questions:**
- Should the prototype include a "what-if" mode for adjusting reconciliation windows or testing durations?
- How do we measure "feature release delay" — from commit to prod? From build to prod?
- Is cadence a metric the UI should surface, or is it an underlying analysis concern?

---

## End-User Question Framework

**What:** The UI questions are organized around three tensions in the end user's workflow:
1. **Compliance** — Are we meeting patching requirements?
2. **Testing procedure** — Are we respecting the process?
3. **Overall stability** — Is the system sustainable?

**Prototype scope (directly answerable):**
- "Are we okay today?" → Compliance status per service
- "Have we pushed anything to prod/staging too soon?" → Deployment gaps
- "Which images are prod-ready? Staging-ready? Need testing?" → Pipeline status
- "Are we using the same image for a given tag?" → SHA tracking

**Prototype scope (partially answerable):**
- "Will it be okay tomorrow given procedure?" → Forward-looking compliance
- "How much wiggle-room?" → Days until deadline + pipeline position
- "What's the latest feature release that's prod-ready?" → Feature branch correlation
- "What's the requirement to make it prod-ready?" → Patches + testing + staging time

**Deferred (production concerns):**
- Trend analysis and historical compliance
- Feature cadence analysis
- Simulation/what-if capabilities

---

## Patch Aggregation: Active vs. Inactive Images

**What:** When aggregating patches for a time range, we should only care about images that are currently relevant — not old versions that have been superseded.

**Why pinned:** Patching an old version that's no longer in any environment is wasted effort. We need to distinguish between:
- **Active images**: Currently in testing, staging, or production — these need patches
- **Inactive images**: Superseded by newer versions with better compliance or more features — these don't

**Prototype assumption:** Only show patches needed for active images. Don't surface patch requirements for old versions.

**Supersession logic:**
- An image is superseded if a newer version exists with:
  - Better compliance status (more patches applied), OR
  - More features (newer tag timestamp)
- If two versions have equal compliance, the newer one supersedes the older
- If a newer version has worse compliance (e.g., missing patches the old one has), both may need attention

**Open questions:**
- How do we handle "feature branches" that are tested but not yet promoted? Are they active?
- Should we show a "historical patch gap" for old versions, or just ignore them?
- In production, should we allow fallback to older versions if the newer one fails compliance? What's the rollback policy?

---

## Copa Patching: Idempotency (Empirically Verified)

**What:** Running `copa patch` on the same source image twice produces identical image IDs (config SHA). The process is deterministic.

**Verified:** Patching `nginx:1.27` twice both produced `bba0eb1715aa8a86fedf1bde2ad37b3c5edbe04c1271de38b245d2077c5c5554`. Same layers, same config.

**Implication:** Re-patching an already-patched image should be safe — it won't introduce drift or unexpected changes. The v0.14.0 release notes confirm successive updates compute from the current patched state.

**Open questions:**
- Does idempotency hold when new CVEs are disclosed between patches? (Expected: yes, but untested.)
- What about patching an already-patched image that was patched with a *different* copa version?

---

## Copa Patching: EOL Base Images Fail

**What:** Debian 11 (bullseye) images cannot be patched — security repos moved to `archive.debian.org`, causing 404 errors when copa installs `busybox-static`.

**Verified:** `nginx:1.21.6` and `nginx:1.24` (both Debian 11) fail. `nginx:1.27` (Debian 12/bookworm) succeeds.

**Implication:** The prototype and any real deployment must ensure base images are on supported OS versions. Copa warns about EOL but still attempts (and fails).

**Open questions:**
- Should the patch tracking system flag EOL base images as a separate risk category?
- How do we handle acquired startups with custom/EOL base images?

---

## Copa Patching: BuildKit Requirement

**What:** Copa requires BuildKit — it cannot use podman/buildah directly. In this WSL+Podman environment, BuildKit runs as a container with TCP (`tcp://127.0.0.1:12345`).

**Implication:** Any CI/CD integration needs a BuildKit instance. This is an infrastructure requirement, not a code requirement.

**Open questions:**
- In the real system, is BuildKit already available, or do we need to provision it?
- Does the patching pipeline run copa directly, or via a wrapper (GitHub Action, etc.)?

---

## Copa Patching: Trivy as Verification

**What:** Trivy before/after scans provide the clearest demo: `nginx:1.27` shows 263 OS vulnerabilities before, 0 after comprehensive patching.

**Implication:** Trivy is useful for verification even when not used for targeted patching. The skill includes both workflows.

**Open questions:**
- In the real system, is Trivy already the scanner, or do we need to integrate with whatever they use?
- Does the patch tracking system need to store vulnerability counts, or just patch application status?

---

## Trivy May Miss Non-CVE Patches

**What:** Trivy detects vulnerabilities by matching packages against CVE databases (NVD, Debian Security Tracker, Ubuntu CVE Tracker, Red Hat Bugzilla, etc.). Some vendors maintain internal bug tracking systems that don't map to CVEs.

**Why pinned:** Microsoft and other vendors use internal bug IDs (e.g., MSRC cases) that may never get a CVE. If a vulnerability fix isn't described as a CVE, Trivy won't see it, and Copa's targeted mode (`-r report.json`) won't patch it.

**Implication:**
- **Copa targeted mode:** Only patches what Trivy reports — misses non-CVE fixes
- **Copa comprehensive mode:** Updates all outdated packages regardless — catches non-CVE fixes as a side effect

**Open questions:**
- Does the real system use vendor-internal patch sources (MSRC, Red Hat Bugzilla, etc.)?
- Should comprehensive mode be the default for production patching to avoid this gap?
- Is there a scanner that can ingest non-CVE vendor advisories?
