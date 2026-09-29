#!/usr/bin/env python3
"""
Environment Snapshot Manager for the patch tracking prototype.

Creates timestamped snapshots of deployment state, including:
- services.json: which version of each service is deployed
- helm-values/: Helm values files per service
- registry.json: registry metadata (tags, digests, push times)

Supports creating snapshots, listing them, and diffing between them.
"""

import argparse
import json
import os
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path

import yaml


DEFAULT_SNAPSHOT_DIR = "kubernetes-snapshots"


def timestamp_to_folder(ts: datetime) -> str:
    """Convert datetime to folder-safe ISO timestamp."""
    return ts.strftime("%Y-%m-%dT%H:%M:%S")


def folder_to_timestamp(folder: str) -> datetime:
    """Parse folder name back to datetime."""
    return datetime.strptime(folder, "%Y-%m-%dT%H:%M:%S").replace(tzinfo=timezone.utc)


def list_snapshots(snapshot_dir: Path) -> list[str]:
    """List all snapshot folders, sorted chronologically."""
    if not snapshot_dir.exists():
        return []
    folders = [
        d.name for d in snapshot_dir.iterdir()
        if d.is_dir() and len(d.name) == 19 and d.name[4] == "-"
    ]
    return sorted(folders)


def create_snapshot(
    snapshot_dir: Path,
    services: dict,
    registry: dict,
    charts_dir: Path | None = None,
    timestamp: datetime | None = None,
) -> str:
    """
    Create a new snapshot.

    Args:
        snapshot_dir: Root snapshots directory
        services: {"service_name": {"tag": "...", "sha": "...", ...}}
        registry: {"tag:digest_prefix": {"digest": "...", "pushed_at": "..."}}
        charts_dir: Path to charts directory (to copy values files from)
        timestamp: Optional timestamp (defaults to now)

    Returns:
        Name of the created snapshot folder
    """
    ts = timestamp or datetime.now(timezone.utc)
    folder_name = timestamp_to_folder(ts)
    snapshot_path = snapshot_dir / folder_name
    snapshot_path.mkdir(parents=True, exist_ok=True)

    # Write services.json
    with open(snapshot_path / "services.json", "w") as f:
        json.dump(services, f, indent=2)

    # Write registry.json
    with open(snapshot_path / "registry.json", "w") as f:
        json.dump(registry, f, indent=2)

    # Copy helm values files if charts_dir provided
    if charts_dir and charts_dir.exists():
        helm_dir = snapshot_path / "helm-values"
        helm_dir.mkdir(exist_ok=True)
        for service_name in services:
            values_file = charts_dir / service_name / "values.yaml"
            if values_file.exists():
                shutil.copy2(values_file, helm_dir / f"{service_name}.yaml")

    return folder_name


def load_snapshot(snapshot_dir: Path, folder_name: str) -> dict:
    """Load a snapshot's data."""
    snapshot_path = snapshot_dir / folder_name
    result = {"folder": folder_name}

    services_path = snapshot_path / "services.json"
    if services_path.exists():
        with open(services_path) as f:
            result["services"] = json.load(f)

    registry_path = snapshot_path / "registry.json"
    if registry_path.exists():
        with open(registry_path) as f:
            result["registry"] = json.load(f)

    return result


def diff_snapshots(prev: dict, curr: dict) -> dict:
    """
    Compute the diff between two snapshots.

    Returns:
        {
            "added": {"service_name": {...}},
            "removed": {"service_name": {...}},
            "changed": {"service_name": {"prev": {...}, "curr": {...}, "diff_type": "..."}},
            "unchanged": {"service_name": {...}}
        }
    """
    prev_services = prev.get("services", {})
    curr_services = curr.get("services", {})

    added = {}
    removed = {}
    changed = {}
    unchanged = {}

    # Find added and changed services
    for name, curr_data in curr_services.items():
        if name not in prev_services:
            added[name] = curr_data
        else:
            prev_data = prev_services[name]
            if prev_data != curr_data:
                # Determine diff type
                diff_type = []
                if prev_data.get("tag") != curr_data.get("tag"):
                    diff_type.append("tag_update")
                if prev_data.get("replicaCount") != curr_data.get("replicaCount"):
                    diff_type.append("replica_change")
                if prev_data.get("resourceTier") != curr_data.get("resourceTier"):
                    diff_type.append("resource_tier_change")
                if prev_data.get("sha") != curr_data.get("sha"):
                    diff_type.append("sha_change")

                changed[name] = {
                    "prev": prev_data,
                    "curr": curr_data,
                    "diff_type": "+".join(diff_type) if diff_type else "unknown",
                }
            else:
                unchanged[name] = curr_data

    # Find removed services
    for name, prev_data in prev_services.items():
        if name not in curr_services:
            removed[name] = prev_data

    return {
        "added": added,
        "removed": removed,
        "changed": changed,
        "unchanged": unchanged,
    }


def format_diff(diff: dict, prev_name: str, curr_name: str) -> str:
    """Format a diff as human-readable text."""
    lines = [f"Diff: {prev_name} → {curr_name}", ""]

    if diff["added"]:
        lines.append("ADDED:")
        for name, data in diff["added"].items():
            tag = data.get("tag", "unknown")
            lines.append(f"  + {name}: {tag}")
        lines.append("")

    if diff["removed"]:
        lines.append("REMOVED:")
        for name, data in diff["removed"].items():
            tag = data.get("tag", "unknown")
            lines.append(f"  - {name}: {tag}")
        lines.append("")

    if diff["changed"]:
        lines.append("CHANGED:")
        for name, change in diff["changed"].items():
            lines.append(f"  ~ {name} ({change['diff_type']}):")
            lines.append(f"    prev: {change['prev'].get('tag', 'unknown')}")
            lines.append(f"    curr: {change['curr'].get('tag', 'unknown')}")
        lines.append("")

    if diff["unchanged"]:
        lines.append("UNCHANGED:")
        for name, data in diff["unchanged"].items():
            lines.append(f"  = {name}: {data.get('tag', 'unknown')}")

    if not any([diff["added"], diff["removed"], diff["changed"]]):
        lines.append("No differences found.")

    return "\n".join(lines)


def validate_snapshot(snapshot_dir: Path, folder_name: str) -> list[str]:
    """
    Validate a single snapshot folder.

    Returns a list of error strings (empty = valid).
    """
    errors = []
    snapshot_path = snapshot_dir / folder_name

    if not snapshot_path.is_dir():
        errors.append(f"Snapshot folder does not exist: {folder_name}")
        return errors

    # Check required files
    services_path = snapshot_path / "services.json"
    registry_path = snapshot_path / "registry.json"

    if not services_path.exists():
        errors.append("Missing services.json")
    if not registry_path.exists():
        errors.append("Missing registry.json")

    if errors:
        return errors

    # Load and parse JSON
    try:
        with open(services_path) as f:
            services = json.load(f)
    except json.JSONDecodeError as e:
        errors.append(f"Invalid JSON in services.json: {e}")
        return errors

    try:
        with open(registry_path) as f:
            registry = json.load(f)
    except json.JSONDecodeError as e:
        errors.append(f"Invalid JSON in registry.json: {e}")
        return errors

    # Validate folder name is a valid ISO timestamp
    try:
        folder_to_timestamp(folder_name)
    except ValueError:
        errors.append(f"Folder name is not a valid ISO timestamp: {folder_name}")

    # Validate services
    required_service_fields = {"tag", "sha", "replicaCount", "resourceTier"}
    known_tiers = {"small", "medium", "large"}
    registry_tags = set(registry.keys())

    for svc_name, svc_data in services.items():
        if not isinstance(svc_data, dict):
            errors.append(f"Service '{svc_name}': must be a JSON object")
            continue

        # Check required fields
        missing = required_service_fields - set(svc_data.keys())
        if missing:
            errors.append(f"Service '{svc_name}': missing fields: {', '.join(missing)}")

        # Validate tag format (service:version)
        tag = svc_data.get("tag", "")
        if ":" not in tag:
            errors.append(f"Service '{svc_name}': tag must contain ':' (got '{tag}')")

        # Validate SHA format
        sha = svc_data.get("sha", "")
        if not sha.startswith("sha256:"):
            errors.append(f"Service '{svc_name}': sha must start with 'sha256:' (got '{sha}')")
        elif len(sha) <= 7:
            errors.append(f"Service '{svc_name}': sha has empty digest after prefix")

        # Validate replica count
        replicas = svc_data.get("replicaCount")
        if not isinstance(replicas, int) or replicas < 0:
            errors.append(f"Service '{svc_name}': replicaCount must be a non-negative integer")

        # Validate resource tier
        tier = svc_data.get("resourceTier", "")
        if tier not in known_tiers:
            errors.append(f"Service '{svc_name}': resourceTier must be one of {known_tiers} (got '{tier}')")

        # Check tag exists in registry
        if tag not in registry_tags:
            errors.append(f"Service '{svc_name}': tag '{tag}' not found in registry.json")

    # Validate registry entries
    for tag, reg_data in registry.items():
        if not isinstance(reg_data, dict):
            errors.append(f"Registry '{tag}': must be a JSON object")
            continue
        if "digest" not in reg_data:
            errors.append(f"Registry '{tag}': missing 'digest'")
        if "pushed_at" not in reg_data:
            errors.append(f"Registry '{tag}': missing 'pushed_at'")
        else:
            try:
                datetime.fromisoformat(reg_data["pushed_at"].replace("Z", "+00:00"))
            except ValueError:
                errors.append(f"Registry '{tag}': invalid pushed_at timestamp")

    return errors


def validate_all(snapshot_dir: Path) -> dict[str, list[str]]:
    """
    Validate all snapshots in a directory.

    Returns {folder_name: [errors]} for invalid snapshots.
    Empty dict means all valid.
    """
    snapshots = list_snapshots(snapshot_dir)
    results = {}
    for s in snapshots:
        errors = validate_snapshot(snapshot_dir, s)
        if errors:
            results[s] = errors
    return results


def main():
    parser = argparse.ArgumentParser(description="Environment Snapshot Manager")
    subparsers = parser.add_subparsers(dest="command", required=True)

    # create
    create_parser = subparsers.add_parser("create", help="Create a new snapshot")
    create_parser.add_argument("--services", required=True, help="Services JSON file")
    create_parser.add_argument("--registry", help="Registry JSON file (optional)")
    create_parser.add_argument("--charts-dir", default="./charts", help="Charts directory")
    create_parser.add_argument("--output", default=DEFAULT_SNAPSHOT_DIR, help="Snapshots directory")
    create_parser.add_argument("--timestamp", help="ISO timestamp (defaults to now)")

    # list
    list_parser = subparsers.add_parser("list", help="List all snapshots")
    list_parser.add_argument("--output", default=DEFAULT_SNAPSHOT_DIR, help="Snapshots directory")

    # diff
    diff_parser = subparsers.add_parser("diff", help="Diff two snapshots")
    diff_parser.add_argument("prev", help="Previous snapshot folder name")
    diff_parser.add_argument("curr", help="Current snapshot folder name")
    diff_parser.add_argument("--output", default=DEFAULT_SNAPSHOT_DIR, help="Snapshots directory")

    # show
    show_parser = subparsers.add_parser("show", help="Show a snapshot's contents")
    show_parser.add_argument("name", help="Snapshot folder name")
    show_parser.add_argument("--output", default=DEFAULT_SNAPSHOT_DIR, help="Snapshots directory")

    # validate
    validate_parser = subparsers.add_parser("validate", help="Validate snapshot folders")
    validate_parser.add_argument("name", nargs="?", help="Specific snapshot to validate (all if omitted)")
    validate_parser.add_argument("--output", default=DEFAULT_SNAPSHOT_DIR, help="Snapshots directory")

    args = parser.parse_args()
    snapshot_dir = Path(args.output)

    if args.command == "create":
        with open(args.services) as f:
            services = json.load(f)
        registry = {}
        if args.registry:
            with open(args.registry) as f:
                registry = json.load(f)
        ts = datetime.fromisoformat(args.timestamp) if args.timestamp else None
        folder = create_snapshot(
            snapshot_dir, services, registry,
            charts_dir=Path(args.charts_dir), timestamp=ts,
        )
        print(f"Created snapshot: {folder}")

    elif args.command == "list":
        snapshots = list_snapshots(snapshot_dir)
        if not snapshots:
            print("No snapshots found.")
        else:
            for s in snapshots:
                print(s)

    elif args.command == "diff":
        prev = load_snapshot(snapshot_dir, args.prev)
        curr = load_snapshot(snapshot_dir, args.curr)
        diff = diff_snapshots(prev, curr)
        print(format_diff(diff, args.prev, args.curr))

    elif args.command == "show":
        data = load_snapshot(snapshot_dir, args.name)
        print(json.dumps(data, indent=2))

    elif args.command == "validate":
        if args.name:
            errors = validate_snapshot(snapshot_dir, args.name)
            if errors:
                print(f"INVALID: {args.name}")
                for e in errors:
                    print(f"  - {e}")
                sys.exit(1)
            else:
                print(f"VALID: {args.name}")
        else:
            results = validate_all(snapshot_dir)
            if not results:
                count = len(list_snapshots(snapshot_dir))
                print(f"All {count} snapshots valid.")
            else:
                print(f"{len(results)} invalid snapshot(s):")
                for folder, errors in results.items():
                    print(f"\n  {folder}:")
                    for e in errors:
                        print(f"    - {e}")
                sys.exit(1)


if __name__ == "__main__":
    main()
