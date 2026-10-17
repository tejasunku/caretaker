#!/usr/bin/env python3
"""Generate web/src/lib/cluster-data.ts from live cluster state.

Reads config/clusters.json, clusters/*.json and patch-manifests/mock-patches.json,
resolves the latest available patch per image from the registry, and emits a
TypeScript module the SolidStart UI imports directly.

Usage:
  python3 scripts/generate_cluster_data.py
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
import sys

sys.path.insert(0, str(ROOT / "scripts"))
from clusters import load_clusters, load_events, now_iso, provider_for  # noqa: E402

OUT = ROOT / "web" / "src" / "lib" / "cluster-data.ts"
REGISTRY = "http://localhost:5000"

# Mock severity/release metadata so the UI can render per-patch context.
_SEVERITIES = ["critical", "high", "medium", "high", "low", "medium", "high"]
_RELEASED = [
    "2026-09-18T10:00:00Z",
    "2026-09-20T10:00:00Z",
    "2026-09-21T10:00:00Z",
    "2026-09-22T10:00:00Z",
    "2026-09-24T10:00:00Z",
    "2026-09-15T10:00:00Z",
    "2026-09-11T10:00:00Z",
]


def registry_tags(repository: str) -> list[str]:
    name = repository.split("/", 1)[1] if "/" in repository else repository
    try:
        with urllib.request.urlopen(f"{REGISTRY}/v2/{name}/tags/list", timeout=3) as r:
            return json.loads(r.read()).get("tags", []) or []
    except (urllib.error.URLError, urllib.error.HTTPError):
        return []


def base_tag(tag: str) -> str:
    return tag.split("-patched-")[0] if "-patched-" in tag else tag


def latest_patched(repository: str, tag: str) -> str | None:
    root = base_tag(tag)
    matches = [t for t in registry_tags(repository) if t.startswith(f"{root}-patched-")]
    return sorted(matches)[-1] if matches else None


def load_patches() -> list[dict]:
    manifest = json.loads((ROOT / "patch-manifests" / "mock-patches.json").read_text())
    patches = []
    for i, update in enumerate(manifest["updates"]):
        patches.append(
            {
                "id": update["vulnerabilityID"],
                "package": update["name"],
                "installedVersion": update["installedVersion"],
                "fixedVersion": update["fixedVersion"],
                "severity": _SEVERITIES[i % len(_SEVERITIES)],
                "releasedAt": _RELEASED[i % len(_RELEASED)],
            }
        )
    return patches


def main() -> None:
    clusters = load_clusters()
    deployments = []
    events = []

    for cluster in clusters:
        # Read deployments through the provider so both simulated and real
        # (kind) clusters are represented.
        try:
            provider = provider_for(cluster)
            live = provider.list_deployments(cluster.id)
        except Exception as exc:  # noqa: BLE001 - keep generating for other clusters
            print(f"WARN: could not read {cluster.id}: {exc}", file=sys.stderr)
            live = []
        for d in live:
            patched = latest_patched(d.repository, d.tag)
            is_stale = bool(patched) and d.tag != patched
            deployments.append(
                {
                    "cluster": cluster.id,
                    "stage": cluster.stage,
                    "service": d.service,
                    "repository": d.repository,
                    "tag": d.tag,
                    "digest": d.digest,
                    "replicas": d.replicas,
                    "latestPatchedTag": patched,
                    "stale": is_stale,
                }
            )

    # History comes from the central, provider-agnostic event log written by
    # the caretaker CLI on every real hot swap (simulated and kind alike).
    for e in load_events():
        events.append(
            {
                "cluster": e.get("cluster", ""),
                "stage": e.get("stage", ""),
                "provider": e.get("provider", ""),
                "at": e.get("at", ""),
                "action": e.get("action", e.get("type", "")),
                "service": e.get("service", ""),
                "fromTag": e.get("fromTag", ""),
                "fromDigest": e.get("fromDigest", ""),
                "toTag": e.get("toTag", ""),
                "toDigest": e.get("toDigest", ""),
            }
        )
    events.sort(key=lambda x: x["at"], reverse=True)

    payload = {
        "generatedAt": now_iso(),
        "clusters": [{"id": c.id, "stage": c.stage, "provider": c.provider} for c in clusters],
        "deployments": deployments,
        "events": events,
        "patches": load_patches(),
    }

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(
        "// Auto-generated from cluster state. Do not edit manually.\n"
        "// Run scripts/generate_cluster_data.py to regenerate.\n\n"
        f"export const CLUSTER_DATA = {json.dumps(payload, indent=2)} as const;\n"
    )
    print(f"wrote {OUT.relative_to(ROOT)} ({len(deployments)} deployments, {len(events)} events)")


if __name__ == "__main__":
    main()
