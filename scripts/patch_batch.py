#!/usr/bin/env python3
"""patch_batch.py -- patch service images with Copa (no cluster changes).

For each selected service, patches the BASE version of its image (stripping any
existing -patched-<date> suffix) to <base>-patched-<today>, pushes it to the
local registry.  It never touches clusters -- run update_batch.py for that.

Usage:
  python3 scripts/patch_batch.py --services all
  python3 scripts/patch_batch.py --services api,web
  python3 scripts/patch_batch.py --services all --dry-run
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import caretaker as ct  # noqa: E402
from clusters import IMAGE_NAMESPACE, SERVICES  # noqa: E402


def latest_base_tag(repository: str) -> str | None:
    """Newest non-patched tag in the registry for a repository."""
    tags = [t for t in ct.registry_tags(repository) if "-patched-" not in t]
    if not tags:
        return None
    tags.sort(key=lambda t: ct.version_key(ct.base_version(t) or "0"))
    return tags[-1]


def resolve_services(selector: str | None) -> list[str]:
    if not selector or selector == "all":
        return list(SERVICES)
    return [s.strip() for s in selector.split(",") if s.strip()]


def patch_service(service: str, dry_run: bool, force: bool) -> bool:
    repository = f"{IMAGE_NAMESPACE}/{service}"
    base = latest_base_tag(repository)
    if base is None:
        print(f"[{service}] skip (no image in registry)")
        return False

    patched_tag = f"{base}-patched-{ct._today()}"
    if ct.registry_digest(repository, patched_tag):
        print(f"[{service}] already patched: {repository}:{patched_tag}")
        return True

    if not force:
        reason = ct.version_guard(service, patched_tag)
        if reason:
            print(f"[{service}] HALT: {reason}")
            return False

    print(f"[{service}] {repository}:{base} -> {patched_tag}")
    if dry_run:
        print(f"[{service}] would run: copa patch -i {repository}:{base} -t {patched_tag} --push")
        return True

    cmd = [
        "copa", "patch",
        "-i", f"{repository}:{base}",
        "-t", patched_tag,
        "--addr", "tcp://127.0.0.1:12345",
        "--platform", "linux/amd64",
        "--push",
    ]
    subprocess.run(cmd, check=True)
    digest = ct.registry_digest(repository, patched_tag)
    print(f"[{service}] pushed {repository}:{patched_tag} ({ct.short(digest)})")
    return True


def main() -> None:
    parser = argparse.ArgumentParser(description="Patch service images with Copa.")
    parser.add_argument("--services", default="all", help="comma-separated names, or 'all'")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()

    services = resolve_services(args.services)
    print(f"Patch images: services={','.join(services)} dry_run={args.dry_run}")

    ok = 0
    for service in services:
        try:
            if patch_service(service, args.dry_run, args.force):
                ok += 1
        except subprocess.CalledProcessError as err:
            print(f"[{service}] ERROR: copa failed (exit {err.returncode})")

    print(f"\nDone: {ok}/{len(services)} image(s) patched")
    sys.exit(0 if ok == len(services) else 1)


if __name__ == "__main__":
    main()
