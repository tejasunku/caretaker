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
    # Snapshot 1: Initial deployment — api and web only
    {
        "timestamp": "2026-09-20T10:00:00",
        "services": {
            "api": {"tag": "api:4.1.20260918-100000", "sha": "sha256:old111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260918-100000", "sha": "sha256:old222", "replicaCount": 2, "resourceTier": "medium"},
        },
        "registry": {
            "api:4.1.20260918-100000": {"digest": "sha256:old111", "pushed_at": "2026-09-18T10:00:00Z"},
            "web:2.3.20260918-100000": {"digest": "sha256:old222", "pushed_at": "2026-09-18T10:00:00Z"},
        },
    },
    # Snapshot 2: web rebuilt (tag update)
    {
        "timestamp": "2026-09-21T14:30:00",
        "services": {
            "api": {"tag": "api:4.1.20260918-100000", "sha": "sha256:old111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260921-080000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
        },
        "registry": {
            "api:4.1.20260918-100000": {"digest": "sha256:old111", "pushed_at": "2026-09-18T10:00:00Z"},
            "web:2.3.20260921-080000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-21T08:00:00Z"},
        },
    },
    # Snapshot 3: api rebuilt + auth + payments introduced
    {
        "timestamp": "2026-09-22T11:00:00",
        "services": {
            "api": {"tag": "api:4.1.20260922-090000", "sha": "sha256:old333", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260921-080000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0.20260922-160000", "sha": "sha256:ddd444", "replicaCount": 2, "resourceTier": "small"},
            "payments": {"tag": "payments:1.2.20260922-140000", "sha": "sha256:aaa999", "replicaCount": 3, "resourceTier": "large"},
        },
        "registry": {
            "api:4.1.20260922-090000": {"digest": "sha256:old333", "pushed_at": "2026-09-22T09:00:00Z"},
            "web:2.3.20260921-080000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-21T08:00:00Z"},
            "auth:1.0.20260922-160000": {"digest": "sha256:ddd444", "pushed_at": "2026-09-22T16:00:00Z"},
            "payments:1.2.20260922-140000": {"digest": "sha256:aaa999", "pushed_at": "2026-09-22T14:00:00Z"},
        },
    },
    # Snapshot 4: worker + cache added, payments to prod (tag mutation)
    {
        "timestamp": "2026-09-23T16:00:00",
        "services": {
            "api": {"tag": "api:4.1.20260922-090000", "sha": "sha256:old333", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260921-080000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0.20260922-160000", "sha": "sha256:ddd444", "replicaCount": 2, "resourceTier": "small"},
            "payments": {"tag": "payments:1.2.20260922-140000", "sha": "sha256:bbb333", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2.20260918-100000", "sha": "sha256:eee555", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.20260923-150000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.20260922-090000": {"digest": "sha256:old333", "pushed_at": "2026-09-22T09:00:00Z"},
            "web:2.3.20260921-080000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-21T08:00:00Z"},
            "auth:1.0.20260922-160000": {"digest": "sha256:ddd444", "pushed_at": "2026-09-22T16:00:00Z"},
            "payments:1.2.20260922-140000": {"digest": "sha256:bbb333", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2.20260918-100000": {"digest": "sha256:eee555", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0.20260923-150000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
        },
    },
    # Snapshot 5: cache rushed to prod, auth removed
    {
        "timestamp": "2026-09-24T18:00:00",
        "services": {
            "api": {"tag": "api:4.1.20260922-090000", "sha": "sha256:old333", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260921-080000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
            "payments": {"tag": "payments:1.2.20260922-140000", "sha": "sha256:bbb333", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2.20260918-100000", "sha": "sha256:eee555", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.20260923-150000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.20260922-090000": {"digest": "sha256:old333", "pushed_at": "2026-09-22T09:00:00Z"},
            "web:2.3.20260921-080000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-21T08:00:00Z"},
            "payments:1.2.20260922-140000": {"digest": "sha256:bbb333", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2.20260918-100000": {"digest": "sha256:eee555", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0.20260923-150000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
        },
    },
    # Snapshot 6: api rebuilt (final state), web enters testing
    {
        "timestamp": "2026-09-25T09:00:00",
        "services": {
            "api": {"tag": "api:4.1.20260925-100000", "sha": "sha256:aaa111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3.20260921-080000", "sha": "sha256:bbb222", "replicaCount": 2, "resourceTier": "medium"},
            "payments": {"tag": "payments:1.2.20260922-140000", "sha": "sha256:bbb333", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2.20260918-100000", "sha": "sha256:eee555", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0.20260923-150000", "sha": "sha256:fff666", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1.20260925-100000": {"digest": "sha256:aaa111", "pushed_at": "2026-09-25T10:00:00Z"},
            "web:2.3.20260921-080000": {"digest": "sha256:bbb222", "pushed_at": "2026-09-21T08:00:00Z"},
            "payments:1.2.20260922-140000": {"digest": "sha256:bbb333", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2.20260918-100000": {"digest": "sha256:eee555", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0.20260923-150000": {"digest": "sha256:fff666", "pushed_at": "2026-09-23T15:00:00Z"},
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
