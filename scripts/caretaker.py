#!/usr/bin/env python3
"""caretaker -- operate on services running across clusters.

Commands:
  clusters                  List registered clusters.
  deployments [--cluster]   Show running image per (cluster, service).
  status [--service]        Flag deployments that lag the latest patch.
  swap                      Hot-swap an explicit image into clusters.
  patch                     Patch the running image with Copa, then hot-swap.
  promote                   Copy a running image from one cluster to others.

Examples:
  python3 scripts/caretaker.py clusters
  python3 scripts/caretaker.py deployments --cluster prod-a
  python3 scripts/caretaker.py patch --service api --clusters prod-a,prod-b
  python3 scripts/caretaker.py swap --service web --image localhost:5000/test/web:1.0-patched-20261016 --clusters prod-a
  python3 scripts/caretaker.py promote --service api --from test-1 --to staging-1,prod-a
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from clusters import (  # noqa: E402
    Deployment,
    ROOT,
    _split_image_ref,
    get_cluster,
    load_clusters,
    provider_for,
    record_event,
)

REGISTRY = "http://localhost:5000"
ACCEPT = ", ".join(
    [
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    ]
)


def short(digest: str) -> str:
    return (digest[:19] + "…") if digest else "—"


def registry_tags(repository: str) -> list[str]:
    name = repository.split("/", 1)[1] if "/" in repository else repository
    try:
        with urllib.request.urlopen(f"{REGISTRY}/v2/{name}/tags/list", timeout=5) as r:
            import json

            return json.loads(r.read()).get("tags", []) or []
    except (urllib.error.URLError, urllib.error.HTTPError):
        return []


def registry_digest(repository: str, tag: str) -> str:
    name = repository.split("/", 1)[1] if "/" in repository else repository
    request = urllib.request.Request(
        f"{REGISTRY}/v2/{name}/manifests/{tag}", method="HEAD", headers={"Accept": ACCEPT}
    )
    try:
        with urllib.request.urlopen(request, timeout=5) as r:
            return r.headers.get("Docker-Content-Digest", "")
    except (urllib.error.URLError, urllib.error.HTTPError):
        return ""


def latest_patched_tag(repository: str, tag: str) -> str | None:
    matches = [t for t in registry_tags(repository) if t.startswith(f"{tag}-patched-")]
    return sorted(matches)[-1] if matches else None


VERSION_RE = re.compile(r"^(?P<base>\d+(?:\.\d+)*)(?P<suffix>-patched-\d+)?$")


def base_version(tag: str) -> str | None:
    """Return the base version of a tag, ignoring a -patched-<date> suffix."""
    match = VERSION_RE.match(tag)
    return match.group("base") if match else None


def version_key(version: str) -> tuple[int, ...]:
    return tuple(int(part) for part in version.split("."))


def max_deployed_version(service: str) -> str | None:
    """Highest base version of `service` currently deployed in any cluster."""
    best: str | None = None
    for cluster in load_clusters():
        try:
            deployments = provider_for(cluster).list_deployments(cluster.id)
        except FileNotFoundError:
            continue
        for d in deployments:
            if d.service != service:
                continue
            version = base_version(d.tag)
            if version and (best is None or version_key(version) > version_key(best)):
                best = version
    return best


def version_guard(service: str, tag: str) -> str | None:
    """Refuse to deploy a patch for an out-of-date version.

    If the tag is a patched version (e.g. 1.0-patched-20261016) but a newer base
    version of the service (e.g. 2.0) is already being deployed somewhere, return
    a human-readable reason to halt; otherwise return None.
    """
    if "-patched-" not in tag:
        return None
    target = base_version(tag)
    if target is None:
        return None
    newer = max_deployed_version(service)
    if newer and version_key(newer) > version_key(target):
        return f"{newer} is being deployed (refusing to patch {target})"
    return None


def resolve_clusters(selector: str | None) -> list:
    all_clusters = load_clusters()
    if not selector:
        return all_clusters
    wanted = [s.strip() for s in selector.split(",") if s.strip()]
    chosen = [c for c in all_clusters if c.id in wanted]
    missing = set(wanted) - {c.id for c in chosen}
    if missing:
        sys.exit(f"unknown cluster(s): {', '.join(sorted(missing))}")
    return chosen


# --------------------------------------------------------------------------- #
# Commands
# --------------------------------------------------------------------------- #

def cmd_clusters(_args) -> None:
    print(f"{'CLUSTER':<12} {'STAGE':<12} {'PROVIDER':<10} DEPLOYMENTS")
    for cluster in load_clusters():
        provider = provider_for(cluster)
        try:
            count = len(provider.list_deployments(cluster.id))
            count_s = str(count)
        except FileNotFoundError:
            count_s = "(unseeded)"
        print(f"{cluster.id:<12} {cluster.stage:<12} {cluster.provider:<10} {count_s}")


def cmd_deployments(args) -> None:
    for cluster in resolve_clusters(args.cluster):
        provider = provider_for(cluster)
        print(f"\n{cluster.id} ({cluster.stage})")
        try:
            deployments = provider.list_deployments(cluster.id)
        except FileNotFoundError:
            print("  (unseeded)")
            continue
        for d in deployments:
            if args.service and d.service != args.service:
                continue
            print(f"  {d.service:<9} {d.repository}:{d.tag}  {short(d.digest)}  x{d.replicas}")


def cmd_status(args) -> None:
    stale = 0
    for cluster in load_clusters():
        provider = provider_for(cluster)
        try:
            deployments = provider.list_deployments(cluster.id)
        except FileNotFoundError:
            continue
        for d in deployments:
            if args.service and d.service != args.service:
                continue
            base = base_version(d.tag) or d.tag
            patched = latest_patched_tag(d.repository, base)
            if patched and d.tag != patched:
                stale += 1
                marker = "STALE"
            else:
                marker = "ok"
            print(f"  [{marker:<5}] {cluster.id:<10} {d.service:<9} {d.tag} -> {patched or '—'}")
    print(f"\n{stale} deployment(s) behind the latest patch")


def _swap(cluster, service, repository, tag, digest, dry_run, force=False, action="swap") -> None:
    provider = provider_for(cluster)
    before = provider.get_deployment(cluster.id, service)
    if before is None:
        print(f"  {cluster.id:<14} {service:<9} skip (no {service})")
        return
    if before.repository == repository and before.tag == tag and before.digest == digest:
        print(f"  {cluster.id:<14} {service:<9} already at {tag}")
        return
    if not force:
        reason = version_guard(service, tag)
        if reason:
            print(f"  {cluster.id:<14} {service:<9} HALT {tag}: {reason} (use --force to override)")
            return
    if dry_run:
        print(f"  {cluster.id:<14} {service:<9} would swap {before.tag} -> {tag} ({short(digest)})")
        return
    after = provider.set_image(cluster.id, service, repository, tag, digest)
    record_event(
        cluster, service, action, before.tag, before.digest, after.tag, after.digest
    )
    print(f"  {cluster.id:<14} {service:<9} swapped {before.tag} -> {after.tag} ({short(after.digest)})")


def cmd_swap(args) -> None:
    repository, tag, digest = _split_image_ref(args.image)
    if not digest:
        digest = registry_digest(repository, tag)
    targets = resolve_clusters(args.clusters)
    print(f"Hot-swap {repository}:{tag} into {len(targets)} cluster(s):")
    for cluster in targets:
        _swap(cluster, args.service, repository, tag, digest, args.dry_run, args.force)


def cmd_promote(args) -> None:
    source = get_cluster(args.source)
    if source is None:
        sys.exit(f"unknown source cluster {args.source!r}")
    src_provider = provider_for(source)
    dep = src_provider.get_deployment(source.id, args.service)
    if dep is None:
        sys.exit(f"{args.service} not deployed in {args.source}")
    targets = resolve_clusters(args.to)
    print(
        f"Promote {args.service} from {source.id} "
        f"({dep.tag} {short(dep.digest)}) to {len(targets)} cluster(s):"
    )
    for cluster in targets:
        _swap(cluster, args.service, dep.repository, dep.tag, dep.digest, args.dry_run, args.force, action="promote")


def cmd_patch(args) -> None:
    """Patch the running image once with Copa, then hot-swap into targets."""
    targets = resolve_clusters(args.clusters)
    # Use the first target's current image as the patch source.
    first = provider_for(targets[0]).get_deployment(targets[0].id, args.service)
    if first is None:
        sys.exit(f"{args.service} not deployed in {targets[0].id}")
    source_ref = f"{first.repository}:{first.tag}"
    patched_tag = f"{first.tag}-patched-{_today()}"
    if not args.force:
        reason = version_guard(args.service, patched_tag)
        if reason:
            sys.exit(f"HALT: {reason} (use --force to override)")
    print(f"Patching {source_ref} -> {first.repository}:{patched_tag}")
    if not args.dry_run:
        cmd = [
            "copa", "patch",
            "-i", source_ref,
            "-t", patched_tag,
            "--addr", "tcp://127.0.0.1:12345",
            "--platform", "linux/amd64",
            "--push",
        ]
        subprocess.run(cmd, check=True)
    digest = "" if args.dry_run else registry_digest(first.repository, patched_tag)
    print(f"\nHot-swap into {len(targets)} cluster(s):")
    for cluster in targets:
        _swap(cluster, args.service, first.repository, patched_tag, digest, args.dry_run, args.force, action="patch")


def _today() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).strftime("%Y%m%d")


# --------------------------------------------------------------------------- #
# Argument parsing
# --------------------------------------------------------------------------- #

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Operate on clusters running caretaker services.")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("clusters", help="list clusters").set_defaults(func=cmd_clusters)

    p = sub.add_parser("deployments", help="show running images")
    p.add_argument("--cluster", help="comma-separated cluster ids (default: all)")
    p.add_argument("--service", help="filter to one service")
    p.set_defaults(func=cmd_deployments)

    p = sub.add_parser("status", help="flag deployments behind the latest patch")
    p.add_argument("--service", help="filter to one service")
    p.set_defaults(func=cmd_status)

    p = sub.add_parser("swap", help="hot-swap an explicit image into clusters")
    p.add_argument("--service", required=True)
    p.add_argument("--image", required=True, help="repo:tag or repo:tag@sha256:...")
    p.add_argument("--clusters", help="comma-separated ids (default: all)")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--force", action="store_true", help="override the newer-version guard")
    p.set_defaults(func=cmd_swap)

    p = sub.add_parser("promote", help="copy a running image to other clusters")
    p.add_argument("--service", required=True)
    p.add_argument("--from", dest="source", required=True)
    p.add_argument("--to", required=True, help="comma-separated ids")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--force", action="store_true", help="override the newer-version guard")
    p.set_defaults(func=cmd_promote)

    p = sub.add_parser("patch", help="patch running image with Copa then hot-swap")
    p.add_argument("--service", required=True)
    p.add_argument("--clusters", required=True, help="comma-separated target ids")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--force", action="store_true", help="override the newer-version guard")
    p.set_defaults(func=cmd_patch)

    return parser


def main() -> None:
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
