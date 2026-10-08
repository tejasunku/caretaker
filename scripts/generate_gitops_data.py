#!/usr/bin/env python3
"""Generate TypeScript data from the gitops repo.

Reads current Helm values and git history from the gitops/ directory
and generates web/src/lib/gitops-data.ts for the UI.
"""

import json
import os
import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path

PROJECT_ROOT = Path(__file__).parent.parent
GITOPS_DIR = PROJECT_ROOT / "gitops"
OUTPUT_FILE = PROJECT_ROOT / "web" / "src" / "lib" / "gitops-data.ts"

# Mock patch data (will be replaced with real data later)
MOCK_PATCHES = [
    {"id": "CVE-2024-1001", "severity": "critical", "package": "openssl", "fixedVersion": "3.0.14", "releasedAt": "2026-09-18T10:00:00Z"},
    {"id": "CVE-2024-1002", "severity": "high", "package": "curl", "fixedVersion": "8.9.1", "releasedAt": "2026-09-20T10:00:00Z"},
    {"id": "CVE-2024-1003", "severity": "medium", "package": "zlib", "fixedVersion": "1.3.1", "releasedAt": "2026-09-21T10:00:00Z"},
    {"id": "CVE-2024-1004", "severity": "high", "package": "libssl3", "fixedVersion": "3.0.15", "releasedAt": "2026-09-22T10:00:00Z"},
    {"id": "CVE-2024-1005", "severity": "low", "package": "tar", "fixedVersion": "1.35", "releasedAt": "2026-09-24T10:00:00Z"},
    {"id": "CVE-2024-1006", "severity": "medium", "package": "gzip", "fixedVersion": "1.13", "releasedAt": "2026-09-15T10:00:00Z"},
]


def run_git(*args: str) -> str:
    """Run a git command in the gitops directory."""
    result = subprocess.run(
        ["git"] + list(args),
        cwd=GITOPS_DIR,
        capture_output=True,
        text=True,
        check=True,
    )
    return result.stdout.strip()


def strip_yaml_comment(value: str) -> str:
    """Strip inline YAML comments from a value."""
    # Find " #" (space followed by #) which indicates a comment
    # But don't strip if # is part of the value (e.g., in a digest or URL)
    idx = value.find(" #")
    if idx >= 0:
        return value[:idx].strip()
    return value.strip()


def parse_yaml_image_values(content: str) -> dict:
    """Parse image-related values from a YAML file."""
    result = {}
    lines = content.split("\n")
    in_image = False
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("image:"):
            in_image = True
            continue
        if in_image:
            if line.startswith(" ") or line.startswith("\t"):
                if ":" in stripped:
                    key, _, value = stripped.partition(":")
                    value = strip_yaml_comment(value)
                    if value:  # Skip empty values (comments only)
                        result[key.strip()] = value.strip('"').strip("'")
            else:
                in_image = False
    return result


def parse_yaml_values(content: str) -> dict:
    """Parse top-level values from a simple YAML file."""
    result = {}
    for line in content.split("\n"):
        stripped = line.strip()
        if stripped and not stripped.startswith("#") and ":" in stripped:
            key, _, value = stripped.partition(":")
            value = strip_yaml_comment(value)
            if value:
                result[key.strip()] = value.strip('"').strip("'")
    return result


def get_services() -> list[str]:
    """List all services in the charts directory."""
    charts_dir = GITOPS_DIR / "charts"
    if not charts_dir.exists():
        return []
    return sorted([d.name for d in charts_dir.iterdir() if d.is_dir()])


def get_current_state() -> dict:
    """Read current state from all values files."""
    services = {}
    for service in get_services():
        chart_dir = GITOPS_DIR / "charts" / service
        svc_data = {"environments": {}, "infra": {}}

        # Read base values (image spec)
        values_file = chart_dir / "values.yaml"
        if values_file.exists():
            content = values_file.read_text()
            svc_data.update(parse_yaml_image_values(content))

        # Read infra values
        infra_file = chart_dir / "values-infra.yaml"
        if infra_file.exists():
            content = infra_file.read_text()
            svc_data["infra"] = parse_yaml_values(content)

        # Read environment-specific values
        for env in ["testing", "staging", "production"]:
            env_file = chart_dir / f"values-{env}.yaml"
            if env_file.exists():
                content = env_file.read_text()
                env_data = parse_yaml_image_values(content)
                env_data.update(parse_yaml_values(content))
                svc_data["environments"][env] = env_data

        services[service] = svc_data
    return services


def get_git_history() -> list[dict]:
    """Get git log for all values files."""
    try:
        # Get all commits that touched any values file
        log_output = run_git(
            "log",
            "--all",
            "--pretty=format:%H|%ai|%s",
            "--",
            "charts/*/values*.yaml",
        )
        
        if not log_output:
            return []
        
        history = []
        for line in log_output.split("\n"):
            if not line.strip():
                continue
            parts = line.split("|", 2)
            if len(parts) < 3:
                continue
            sha, date_str, message = parts
            history.append({
                "sha": sha,
                "date": date_str,
                "message": message,
            })
        
        return history
    except subprocess.CalledProcessError:
        return []


def get_file_history(filepath: str) -> list[dict]:
    """Get git log for a specific file."""
    try:
        log_output = run_git(
            "log",
            "--pretty=format:%H|%ai|%s",
            "--",
            filepath,
        )
        
        if not log_output:
            return []
        
        history = []
        for line in log_output.split("\n"):
            if not line.strip():
                continue
            parts = line.split("|", 2)
            if len(parts) < 3:
                continue
            sha, date_str, message = parts
            history.append({
                "sha": sha,
                "date": date_str,
                "message": message,
            })
        
        return history
    except subprocess.CalledProcessError:
        return []


def get_service_history(service: str) -> list[dict]:
    """Get git history for a specific service's values files."""
    chart_path = f"charts/{service}"
    return get_file_history(chart_path)


def generate_typescript() -> str:
    """Generate TypeScript code from gitops data."""
    services = get_current_state()
    git_history = get_git_history()
    
    # Build service history
    service_histories = {}
    for service in services:
        service_histories[service] = get_service_history(service)
    
    # Generate TypeScript
    ts_lines = [
        "// Auto-generated from gitops repo. Do not edit manually.",
        "// Run scripts/generate_gitops_data.py to regenerate.",
        "",
        "export const MOCK_PATCHES = " + json.dumps(MOCK_PATCHES, indent=2) + " as const;",
        "",
        "export const CURRENT_STATE = " + json.dumps(services, indent=2) + ";",
        "",
        "export const GIT_HISTORY = " + json.dumps(git_history, indent=2) + ";",
        "",
        "export const SERVICE_HISTORIES = " + json.dumps(service_histories, indent=2) + ";",
        "",
    ]
    
    return "\n".join(ts_lines)


def main():
    """Main entry point."""
    if not GITOPS_DIR.exists():
        print(f"Error: gitops directory not found at {GITOPS_DIR}", file=sys.stderr)
        sys.exit(1)
    
    print("Reading gitops repo...")
    ts_code = generate_typescript()
    
    # Ensure output directory exists
    OUTPUT_FILE.parent.mkdir(parents=True, exist_ok=True)
    
    # Write the file
    OUTPUT_FILE.write_text(ts_code)
    print(f"Generated {OUTPUT_FILE}")
    
    # Print summary
    services = get_current_state()
    history = get_git_history()
    print(f"  Services: {len(services)}")
    print(f"  Git commits: {len(history)}")


if __name__ == "__main__":
    main()
