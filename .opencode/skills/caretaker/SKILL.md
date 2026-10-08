---
name: caretaker
description: Overview of the caretaker project — a patch tracking system for container images. Covers project structure, the web UI, snapshot system, SBOM checker, patch manifests, and how to run/develop the prototype.
---

# Caretaker Skill

Caretaker is a prototype system for tracking CVE patching of container images. It demonstrates two core queries:
1. **Query A:** "Given an image tag, which patches does it have (and which is it missing)?"
2. **Query B:** "Given a time range, which patches need to be applied, and to which images?"

## Project Structure

```
caretaker/
├── PINS.md                    # Deferred decisions and assumptions
├── REF_SYSTEM.md              # Reference implementation design
├── TIMING.md                  # Empirical Trivy/Copa timing measurements
├── patch-manifests/           # Copa-native patch manifests
│   ├── mock-patches.json      # Mixed CVE + non-CVE patches
│   └── cve-only.json          # CVE-only patches
├── kubernetes-snapshots/      # Environment snapshots (GitOps model)
│   ├── 2026-09-20T10:00:00/
│   │   ├── services.json      # Service → tag/SHA mapping
│   │   ├── helm-values/       # Helm values per service
│   │   └── registry.json      # Registry metadata snapshot
│   └── ...
├── scripts/                   # Python/Node tooling
│   ├── setup.sh               # Install helm, copa, trivy, Python venv
│   ├── seed_snapshots.py      # Generate snapshot folders
│   ├── generate_data.py       # Generate snapshot-data.ts from folders
│   ├── generate_chart.py      # Helm chart generator
│   ├── snapshot.py            # Snapshot management (create, diff, validate)
│   └── check-sbom.ts          # SBOM checker CLI
├── web/                       # SolidStart v2 web UI
│   ├── src/
│   │   ├── app.tsx            # Router root with sidebar
│   │   ├── routes/
│   │   │   ├── index.tsx      # Patching Overview page
│   │   │   ├── service/[name].tsx  # Service Deep Dive
│   │   │   ├── history.tsx    # Deployment History
│   │   │   └── data.ts        # Data loading functions
│   │   └── lib/
│   │       ├── types.ts       # TypeScript type definitions
│   │       ├── mock-data.ts   # Patches, environment deployments
│   │       ├── snapshot-data.ts  # Auto-generated from snapshots
│   │       ├── compliance.ts  # Patching status logic
│   │       ├── stability.ts   # Testing/staging stability logic
│   │       ├── snapshots.ts   # Snapshot diff logic
│   │       └── sbom-checker.ts  # SBOM verification
│   └── uno.config.ts          # UnoCSS config
├── .opencode/skills/          # Skills for opencode
│   ├── copacetic/             # Copa patching skill
│   ├── trivy/                 # Trivy scanning skill
│   └── caretaker/             # This skill
└── bin/                       # Local binaries (gitignored)
    ├── helm
    ├── copa
    └── trivy
```

## Running the Web UI

```bash
cd web/
npx vite dev --port 3000
```

The UI runs with SSR disabled and reads data from snapshot folders via static imports.

**Pages:**
- `/` — Patching Overview (service cards, stats, stability/integrity concerns)
- `/service/[name]` — Service Deep Dive (patching status, stability pipeline, reconciliation windows)
- `/history` — Deployment History (snapshot timeline)

## Running the SBOM Checker

```bash
export DOCKER_HOST=unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock
export PATH="$PWD/bin:$PATH"

# Check image against patch manifest
node --experimental-strip-types scripts/check-sbom.ts \
  --image nginx:1.27 \
  --manifest patch-manifests/mock-patches.json

# JSON output
node --experimental-strip-types scripts/check-sbom.ts \
  -i nginx:1.27 -m patch-manifests/cve-only.json --json
```

## Patch Manifest Format (Copa v1alpha1)

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

**Key points:**
- `vulnerabilityID` is just a string label — can be CVE, internal bug ID, etc.
- Format matches Copa's native report (`--scanner native`)
- SBOM checker uses `>=` comparison (installed >= fixed = satisfied)

## Snapshot System

Snapshots are timestamped folders representing point-in-time environment state (GitOps model).

**Structure:**
```
kubernetes-snapshots/
  2026-09-20T10:00:00/
    services.json        # {"api": {"tag": "4.1.20260925-100000", "sha": "sha256:..."}}
    helm-values/
      api.yaml           # Helm values for each service
    registry.json        # Registry metadata (tags, digests, push times)
```

**Management:**
```bash
# Create snapshot
.venv/bin/python scripts/snapshot.py create --timestamp 2026-09-26T10:00:00

# Diff two snapshots
.venv/bin/python scripts/snapshot.py diff 2026-09-25T12:00:00 2026-09-24T18:00:00

# Validate snapshots
.venv/bin/python scripts/snapshot.py validate
```

## Key Concepts

### Patching vs. Stability
- **Patching status:** Are CVEs addressed? (compliant, at_risk, non_compliant)
- **Stability:** Did images spend enough time in testing/staging? (2d each required)

### Compliance Statuses
- `inactive` — not currently deployed
- `new` — recently built, no patches expected yet
- `non_compliant` — past reconciliation deadline
- `at_risk` — insufficient time to patch before deadline
- `barely` — barely enough time
- `compliant` — all patches applied or plenty of time

### Week Bucketing
Patches are grouped by week (Monday–Sunday). UI shows introduced week and deadline week.

## Development

### Setup
```bash
bash scripts/setup.sh
export PATH="$PWD/bin:$PATH"
source .venv/bin/activate
```

### Regenerating Data
```bash
# After modifying mock data or snapshots
.venv/bin/python scripts/seed_snapshots.py
.venv/bin/python scripts/generate_data.py
```

### Environment Variables
```bash
# For Docker/Podman access (WSL)
export DOCKER_HOST=unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock

# For local binaries
export PATH="$PWD/bin:$PATH"
```

## Related Skills

- **copacetic**: Copa patching (comprehensive/targeted modes, BuildKit setup)
- **trivy**: Trivy scanning (vulnerabilities, SBOMs, report formats)
