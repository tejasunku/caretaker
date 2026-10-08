---
name: trivy
description: Use Trivy to scan container images for vulnerabilities and generate SBOMs. Covers installation, vulnerability scanning, SBOM generation (CycloneDX/SPDX), report formats, and integration with Copacetic for this repo.
---

# Trivy Skill

Trivy is a comprehensive security scanner for container images, filesystems, and git repositories. It detects vulnerabilities, misconfigurations, secrets, and generates SBOMs.

## Installation

Trivy is installed locally in `bin/` (gitignored). Install via:

```bash
bash scripts/setup.sh
export PATH="$PWD/bin:$PATH"
```

- `bin/trivy` — v0.75.0

## Vulnerability Scanning

### Basic scan

```bash
trivy image <image>
```

### OS packages only (for Copa integration)

```bash
trivy image --pkg-types os --ignore-unfixed <image>
```

### JSON output (for Copa targeted patching)

```bash
trivy image --pkg-types os --ignore-unfixed -f json -o report.json <image>
```

### Summary output

```bash
trivy image --report summary --scanners vuln --pkg-types os --ignore-unfixed <image>
```

## SBOM Generation

### CycloneDX (used by SBOM checker)

```bash
trivy image --format cyclonedx --output sbom.json <image>
```

Note: `--format cyclonedx` disables security scanning by default. Add `--scanners vuln` to include vulnerabilities.

### SPDX

```bash
trivy image --format spdx-json --output sbom.spdx <image>
```

## Key Flags

| Flag | Purpose |
|------|---------|
| `--pkg-types os` | Scan only OS packages (replaces deprecated `--vuln-type`) |
| `--ignore-unfixed` | Skip vulnerabilities without available fixes |
| `--format json` | JSON output (for Copa integration) |
| `--format cyclonedx` | CycloneDX SBOM output |
| `--format spdx-json` | SPDX SBOM output |
| `--report summary` | Summary-only output |
| `--scanners vuln` | Only vulnerability scanning (disable secrets, misconfig) |
| `--output <file>` | Write output to file |
| `--quiet` | Suppress progress output |
| `--no-progress` | Disable progress bar |

## Empirical Timing (from this repo)

Measured on 2026-10-07, cached DB:

| Image | Time | Vulns |
|-------|------|-------|
| alpine:3.20 | 1.10s | 0 |
| nginx:1.27 | 1.24s | 263 |
| node:20-slim | 2.49s | 61 |
| python:3.12-slim | 2.16s | 0 |

First scan adds ~10–30s for vulnerability DB download (~50MB).

## SBOM Format (CycloneDX)

Used by `web/src/lib/sbom-checker.ts`:

```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "components": [
    {
      "name": "openssl",
      "version": "3.0.16-1~deb12u1",
      "purl": "pkg:deb/debian/openssl@3.0.16-1~deb12u1?arch=amd64"
    }
  ]
}
```

The SBOM checker extracts `name` → `version` pairs and compares against Copa patch manifests.

## Integration with Copa

1. **Targeted patching:** Generate Trivy JSON report → feed to `copa patch -r report.json`
2. **SBOM verification:** Generate CycloneDX SBOM → check against patch manifests
3. **Comprehensive patching:** No Trivy report needed (Copa updates all packages)

## Known Limitations

- Trivy detects vulnerabilities via CVE databases — misses non-CVE patches (internal bug fixes, vendor backports)
- For non-CVE patches, use SBOM-based verification against a patch manifest
- OS package detection uses package manager databases (dpkg, apk, rpm) — version numbers, not binary hashes
- Java images may be slower (increase `--timeout` if needed)

## Docker/Podman Socket

In this WSL environment, set `DOCKER_HOST` for Trivy to access local images:

```bash
export DOCKER_HOST=unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock
```

## Quick Reference

```bash
# Scan for vulnerabilities
trivy image --pkg-types os --ignore-unfixed nginx:1.27

# Generate JSON report for Copa
trivy image --pkg-types os --ignore-unfixed -f json -o report.json nginx:1.27

# Generate CycloneDX SBOM
trivy image --format cyclonedx --output sbom.json nginx:1.27

# Full pipeline (scan → patch → verify)
trivy image --pkg-types os --ignore-unfixed -f json -o report.json nginx:1.27
copa patch -r report.json -i nginx:1.27 -t nginx:1.27-patched
trivy image --pkg-types os --ignore-unfixed nginx:1.27-patched
```
