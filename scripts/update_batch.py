#!/usr/bin/env python3
"""update_batch.py -- hot-swap the latest patched image into running clusters.

For each selected (cluster, service), finds the newest -patched- tag matching the
deployment's current version and hot-swaps the running image to it.  No images are
built here -- run patch_batch.py first to produce patched images.

This respects independently-versioned clusters: the update is driven by what each
deployment currently runs (e.g. a cluster on 2.0 gets 2.0-patched-*, not 1.0-*).

Usage:
  python3 scripts/update_batch.py --clusters all --services all
  python3 scripts/update_batch.py --clusters prod-a,prod-b --services api,web
  python3 scripts/update_batch.py --clusters prod-a --services all --dry-run
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import caretaker as ct  # noqa: E402
from clusters import SERVICES, load_clusters, provider_for  # noqa: E402


def resolve_list(selector: str | None, all_values: list[str]) -> list[str]:
    if not selector or selector == "all":
        return list(all_values)
    return [s.strip() for s in selector.split(",") if s.strip()]


def update_cluster(cluster, services: list[str], dry_run: bool, force: bool) -> int:
    provider = provider_for(cluster)
    try:
        deployments = provider.list_deployments(cluster.id)
    except FileNotFoundError:
        print(f"{cluster.id}: (unseeded)")
        return 0

    changed = 0
    for d in deployments:
        if services and d.service not in services:
            continue
        base = ct.base_version(d.tag) or d.tag
        patched = ct.latest_patched_tag(d.repository, base)
        if not patched:
            print(f"  {cluster.id:<14} {d.service:<9} {d.tag}  (no patched version available)")
            continue
        if d.tag == patched:
            print(f"  {cluster.id:<14} {d.service:<9} {d.tag}  (already latest)")
            continue
        digest = "" if dry_run else ct.registry_digest(d.repository, patched)
        ct._swap(
            cluster, d.service, d.repository, patched, digest, dry_run,
            force=force, action="update",
        )
        changed += 1
    return changed


def main() -> None:
    parser = argparse.ArgumentParser(description="Hot-swap patched images into clusters.")
    parser.add_argument("--clusters", default="all", help="comma-separated ids, or 'all'")
    parser.add_argument("--services", default="all", help="comma-separated names, or 'all'")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()

    all_clusters = load_clusters()
    wanted = set(resolve_list(args.clusters, [c.id for c in all_clusters]))
    clusters = [c for c in all_clusters if c.id in wanted]
    services = resolve_list(args.services, SERVICES)

    print(f"Update clusters: clusters={','.join(c.id for c in clusters)} "
          f"services={','.join(services)} dry_run={args.dry_run}")

    total = 0
    for cluster in clusters:
        total += update_cluster(cluster, services, args.dry_run, args.force)

    print(f"\nDone: {total} deployment(s) updated")
    sys.exit(0)


if __name__ == "__main__":
    main()
