# TIMING.md — Empirical Performance Measurements

Measured on 2026-10-07 in WSL + Podman environment.

## Environment

- **OS:** WSL (Linux)
- **Container runtime:** Podman (Windows host)
- **BuildKit:** v0.23.2 (containerized, TCP on 127.0.0.1:12345)
- **Trivy:** v0.75.0
- **Copa:** v0.15.0
- **CPU:** AMD64
- **Note:** Image pull time excluded from all measurements. Images pre-pulled before timing.

## Test Images

| Image | Size | OS | Vulns (before) | Notes |
|-------|------|-----|----------------|-------|
| alpine:3.20 | 8MB | Alpine 3.20 | 0 | EOL — security updates stopped |
| nginx:1.27 | 187MB | Debian 12.11 | 263 | Good test case, many vulns |
| node:20-slim | 195MB | Debian 12.13 | 61 | Common app base |
| python:3.12-slim | 117MB | Debian 13.7 | 0 | Too new for known vulns |

## Trivy Scan Times

| Image | Wall Time | Vulnerabilities | Notes |
|-------|-----------|-----------------|-------|
| alpine:3.20 | **1.10s** | 0 | Minimal packages (14) |
| nginx:1.27 | **1.24s** | 263 | 144 packages |
| node:20-slim | **2.49s** | 61 | 88 packages |
| python:3.12-slim | **2.16s** | 0 | 87 packages |

**Summary:** Trivy scans complete in **1–2.5 seconds** per image (cached DB). First scan adds ~10–30s for DB download. Vulnerability count has minimal impact on scan time.

## Copa Comprehensive Patching

Updates ALL outdated packages (no report needed).

| Image | Wall Time | Vulns Before → After | Size Before → After | Notes |
|-------|-----------|----------------------|---------------------|-------|
| alpine:3.20 | **5.81s** | 0 → 0 | 8MB → N/A | Failed: "no package updates found" (EOL) |
| nginx:1.27 | **43.33s** | 263 → 0 | 187MB → 283MB | +96MB patch layer |
| node:20-slim | **30.12s** | 61 → 0 | 195MB → 230MB | +35MB patch layer |
| python:3.12-slim | **23.64s** | 0 → 0 | 117MB → 118MB | +1MB (minimal updates) |

**Summary:** Comprehensive patching takes **20–45 seconds** per image. Time correlates with number of packages needing updates.

## Copa Targeted Patching

Patches only packages from Trivy report (`-r report.json`).

| Image | Wall Time | Vulns Before → After | Size Before → After | Patched |
|-------|-----------|----------------------|---------------------|---------|
| nginx:1.27 | **35.37s** | 263 → 0 | 187MB → 278MB | 263/263 |
| node:20-slim | **25.57s** | 61 → 0 | 195MB → 228MB | 61/61 |

**Summary:** Targeted patching is **~15–20% faster** than comprehensive (skips package discovery, uses report directly).

## Full Workflow Timing

| Workflow | Time per image | Notes |
|----------|---------------|-------|
| Trivy scan only | 1–2.5s | Detection |
| Trivy scan + Copa comprehensive | 25–45s | Full remediation |
| Trivy scan + Copa targeted | 30–40s | Includes report generation |
| Batch: scan 10 images | 15–30s | Parallelizable |
| Batch: scan 10, patch 3 vulnerable | 2–4 min | Scan all, patch selective |

## Key Findings

1. **Trivy scans are fast:** 1–2.5s per image with cached DB. Cheap enough to run on every image regularly.

2. **Copa patching is minutes, not hours:** 20–45s per image. Much faster than full rebuild (10–30+ min).

3. **Targeted mode is slightly faster:** ~15–20% speedup vs comprehensive. Both achieve 0 vulns for fixable issues.

4. **EOL images fail fast:** Alpine 3.20 failed in ~6s with clear error. Copa detects "no package updates" quickly.

5. **Patch layer size varies:** +1MB (minimal updates) to +96MB (many package updates). Larger patches = more packages updated.

6. **Comprehensive mode catches non-CVE fixes:** Even with 0 CVEs (python:3.12-slim), comprehensive mode still applied package updates (+1MB). Targeted mode with empty report would skip entirely.

## Implications for Prototype

- **Scan-first workflow is viable:** Trivy is cheap enough to scan all images before deciding what to patch.
- **Patching fits in CI:** 20–45s per image is acceptable for pipeline integration.
- **Comprehensive mode is safer:** Catches non-CVE fixes (see PINS.md: "Trivy May Miss Non-CVE Patches").
- **Idempotency confirmed:** Re-patching produces identical results (verified earlier).

## Comparison: Patch vs Rebuild

| Approach | Time | Notes |
|----------|------|-------|
| Copa patch | 20–45s | Adds patch layer only |
| Full rebuild | 10–30+ min | New layers, breaks cache |
| Rebuild + test | 30–60+ min | Includes test cycle |

Copa is **10–50x faster** than rebuild for OS package updates.
