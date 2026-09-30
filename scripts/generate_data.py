#!/usr/bin/env python3
"""Generate a static TypeScript data module from snapshot folders."""
import json
import os
import sys

SNAPSHOTS_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), "kubernetes-snapshots")
OUTPUT = os.path.join(os.path.dirname(os.path.dirname(__file__)), "web", "src", "lib", "snapshot-data.ts")

def load_snapshot(folder):
    path = os.path.join(SNAPSHOTS_DIR, folder)
    services_path = os.path.join(path, "services.json")
    registry_path = os.path.join(path, "registry.json")
    
    services = {}
    if os.path.exists(services_path):
        with open(services_path) as f:
            services = json.load(f)
    
    registry = {}
    if os.path.exists(registry_path):
        with open(registry_path) as f:
            registry = json.load(f)
    
    return {
        "folder": folder,
        "timestamp": folder + "Z",
        "services": services,
        "registry": registry,
    }

def main():
    folders = sorted([
        d for d in os.listdir(SNAPSHOTS_DIR)
        if os.path.isdir(os.path.join(SNAPSHOTS_DIR, d)) and len(d) == 19 and d[4] == "-"
    ])
    
    snapshots = [load_snapshot(f) for f in folders]
    
    content = '// Auto-generated from kubernetes-snapshots/ — do not edit manually\n'
    content += '// Run: python3 scripts/generate_data.py\n\n'
    content += f'export const SNAPSHOTS = {json.dumps(snapshots, indent=2)} as const;\n'
    
    with open(OUTPUT, "w") as f:
        f.write(content)
    
    print(f"Generated {OUTPUT} with {len(snapshots)} snapshots")

if __name__ == "__main__":
    main()
