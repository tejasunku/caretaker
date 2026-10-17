#!/usr/bin/env python3
"""Seed simulated cluster state from the cluster registry.

For every cluster in ``config/clusters.json`` this writes
``clusters/<id>.json`` with one deployment per service.  Images are resolved
against the local OCI registry so the seeded state carries real digests.

Stage -> tag defaults (override with --testing-tag / --prod-tag):

  testing     1.0-patched-20261016   (validated patch candidate)
  staging     1.0                    (previous release)
  production  1.0                    (stale, still needs the patch)

Usage:
  python3 scripts/seed_clusters.py [--reset]
"""

from __future__ import annotations

import argparse
import json
import urllib.error
import urllib.request
from pathlib import Path

from clusters import (
    CLUSTERS_DIR,
    IMAGE_NAMESPACE,
    SERVICES,
    load_clusters,
    now_iso,
)

REGISTRY = "http://localhost:5000"
ACCEPT = ", ".join(
    [
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    ]
)

STAGE_TAGS = {
    "testing": "1.0-patched-20261016",
    "staging": "1.0",
    "production": "1.0",
}


def resolve_digest(repository: str, tag: str) -> str:
    """Return the manifest digest for repository:tag, or '' if unavailable."""
    name = repository.split("/", 1)[1] if "/" in repository else repository
    url = f"{REGISTRY}/v2/{name}/manifests/{tag}"
    request = urllib.request.Request(url, method="HEAD", headers={"Accept": ACCEPT})
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return response.headers.get("Docker-Content-Digest", "")
    except (urllib.error.URLError, urllib.error.HTTPError):
        return ""


def build_state(cluster, tag: str) -> dict:
    deployments = {}
    for service in SERVICES:
        repository = f"{IMAGE_NAMESPACE}/{service}"
        deployments[service] = {
            "service": service,
            "container": service,
            "image": {
                "repository": repository,
                "tag": tag,
                "digest": resolve_digest(repository, tag),
            },
            "replicas": 1 if cluster.stage != "production" else 3,
        }
    return {
        "id": cluster.id,
        "stage": cluster.stage,
        "provider": cluster.provider,
        "seededAt": now_iso(),
        "deployments": deployments,
        "events": [],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reset", action="store_true", help="overwrite existing state")
    parser.add_argument("--testing-tag", default=STAGE_TAGS["testing"])
    parser.add_argument("--staging-tag", default=STAGE_TAGS["staging"])
    parser.add_argument("--prod-tag", default=STAGE_TAGS["production"])
    args = parser.parse_args()

    tags = {
        "testing": args.testing_tag,
        "staging": args.staging_tag,
        "production": args.prod_tag,
    }

    CLUSTERS_DIR.mkdir(exist_ok=True)
    for cluster in load_clusters():
        path = CLUSTERS_DIR / f"{cluster.id}.json"
        if path.exists() and not args.reset:
            print(f"skip  {cluster.id:<10} (exists; use --reset to overwrite)")
            continue
        state = build_state(cluster, tags[cluster.stage])
        path.write_text(json.dumps(state, indent=2) + "\n")
        print(f"seed  {cluster.id:<10} stage={cluster.stage:<10} tag={tags[cluster.stage]}")


if __name__ == "__main__":
    main()
