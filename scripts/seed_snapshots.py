#!/usr/bin/env python3
"""Create the 6 mock snapshots on disk as folders."""
import json
import sys
from datetime import datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from snapshot import create_snapshot, DEFAULT_SNAPSHOT_DIR

SNAPSHOT_DIR = Path(__file__).parent.parent / DEFAULT_SNAPSHOT_DIR

snapshots = [
    {
        "timestamp": "2026-09-20T10:00:00",
        "services": {
            "api": {"tag": "api:4.1.1726912345", "sha": "sha256:aaa111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.1726912000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
        },
        "registry": {
            "api:4.1.1726912345": {"digest": "sha256:aaa111", "pushed_at": "2026-09-16T12:00:00Z"},
            "web:2.3.1726912000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-17T09:00:00Z"},
        },
    },
    {
        "timestamp": "2026-09-21T14:30:00",
        "services": {
            "api": {"tag": "api:4.1.1726990000", "sha": "sha256:ccc333", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.1726912000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0.1726950000", "sha": "sha256:ddd444", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.1726990000": {"digest": "sha256:ccc333", "pushed_at": "2026-09-21T08:00:00Z"},
            "web:2.3.1726912000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-17T09:00:00Z"},
            "auth:1.0.1726950000": {"digest": "sha256:ddd444", "pushed_at": "2026-09-20T16:00:00Z"},
        },
    },
    {
        "timestamp": "2026-09-22T11:00:00",
        "services": {
            "api": {"tag": "api:4.1.1726990000", "sha": "sha256:ccc333", "replicaCount": 3, "resourceTier": "large"},
            "web": {"tag": "web:2.3.1726912000", "sha": "sha256:bbb222", "replicaCount": 4, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0.1726950000", "sha": "sha256:ddd444", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.1726990000": {"digest": "sha256:ccc333", "pushed_at": "2026-09-21T08:00:00Z"},
            "web:2.3.1726912000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-17T09:00:00Z"},
            "auth:1.0.1726950000": {"digest": "sha256:ddd444", "pushed_at": "2026-09-20T16:00:00Z"},
        },
    },
    {
        "timestamp": "2026-09-23T16:00:00",
        "services": {
            "api": {"tag": "api:4.1.1726990000", "sha": "sha256:ccc333", "replicaCount": 3, "resourceTier": "large"},
            "web": {"tag": "web:2.3.1726912000", "sha": "sha256:bbb222", "replicaCount": 4, "resourceTier": "medium"},
            "worker": {"tag": "worker:3.2.1726880000", "sha": "sha256:eee555", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.1727100000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.1726990000": {"digest": "sha256:ccc333", "pushed_at": "2026-09-21T08:00:00Z"},
            "web:2.3.1726912000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-17T09:00:00Z"},
            "worker:3.2.1726880000": {"digest": "sha256:eee555", "pushed_at": "2026-09-14T10:00:00Z"},
            "cache:1.0.1727100000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
        },
    },
    {
        "timestamp": "2026-09-24T18:00:00",
        "services": {
            "api": {"tag": "api:4.1.1726990000", "sha": "sha256:ccc333", "replicaCount": 2, "resourceTier": "large"},
            "web": {"tag": "web:2.3.1727100000", "sha": "sha256:ggg777", "replicaCount": 4, "resourceTier": "medium"},
            "worker": {"tag": "worker:3.2.1727150000", "sha": "sha256:hhh888", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.1727100000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.1726990000": {"digest": "sha256:ccc333", "pushed_at": "2026-09-21T08:00:00Z"},
            "web:2.3.1727100000": {"digest": "sha256:ggg777", "pushed_at": "2026-09-24T10:00:00Z"},
            "worker:3.2.1727150000": {"digest": "sha256:hhh888", "pushed_at": "2026-09-24T14:00:00Z"},
            "cache:1.0.1727100000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
        },
    },
    {
        "timestamp": "2026-09-25T09:00:00",
        "services": {
            "api": {"tag": "api:4.1.1726990000", "sha": "sha256:ccc333", "replicaCount": 2, "resourceTier": "large"},
            "web": {"tag": "web:2.3.1727100000", "sha": "sha256:ggg777", "replicaCount": 4, "resourceTier": "medium"},
            "worker": {"tag": "worker:3.2.1727150000", "sha": "sha256:hhh888", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.1727100000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.1726990000": {"digest": "sha256:ccc333", "pushed_at": "2026-09-21T08:00:00Z"},
            "web:2.3.1727100000": {"digest": "sha256:ggg777", "pushed_at": "2026-09-24T10:00:00Z"},
            "worker:3.2.1727150000": {"digest": "sha256:hhh888", "pushed_at": "2026-09-24T14:00:00Z"},
            "cache:1.0.1727100000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
        },
    },
]

if SNAPSHOT_DIR.exists():
    import shutil
    shutil.rmtree(SNAPSHOT_DIR)

for s in snapshots:
    ts_str = s.pop("timestamp")
    ts = datetime.fromisoformat(ts_str)
    folder = create_snapshot(
        SNAPSHOT_DIR,
        services=s["services"],
        registry=s["registry"],
        timestamp=ts,
    )
    print(f"Created: {folder}")

print(f"\nAll snapshots in: {SNAPSHOT_DIR}")
