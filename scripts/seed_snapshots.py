#!/usr/bin/env python3
"""Create the 6 mock snapshots on disk as folders — digest-based lineage model."""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from snapshot import create_snapshot, DEFAULT_SNAPSHOT_DIR

SNAPSHOT_DIR = Path(__file__).parent.parent / DEFAULT_SNAPSHOT_DIR

snapshots = [
    # Snapshot 1: Initial — api and web (original builds)
    {
        "timestamp": "2026-09-20T10:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_base_111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_base_111", "replicaCount": 2, "resourceTier": "medium"},
        },
        "registry": {
            "api:4.1@sha256:api_base_111": {"digest": "sha256:api_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "web:2.3@sha256:web_base_111": {"digest": "sha256:web_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
        },
    },
    # Snapshot 2: web patched (new digest under same tag)
    {
        "timestamp": "2026-09-22T10:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_base_111", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_patched_222", "replicaCount": 2, "resourceTier": "medium"},
        },
        "registry": {
            "api:4.1@sha256:api_base_111": {"digest": "sha256:api_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "web:2.3@sha256:web_base_111": {"digest": "sha256:web_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "web:2.3@sha256:web_patched_222": {"digest": "sha256:web_patched_222", "pushed_at": "2026-09-22T08:00:00Z"},
        },
    },
    # Snapshot 3: api patched + auth + payments introduced
    {
        "timestamp": "2026-09-22T18:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0", "digest": "sha256:auth_base_111", "replicaCount": 2, "resourceTier": "small"},
            "payments": {"tag": "payments:1.2", "digest": "sha256:pay_patched_aaa", "replicaCount": 3, "resourceTier": "large"},
        },
        "registry": {
            "api:4.1@sha256:api_base_111": {"digest": "sha256:api_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "api:4.1@sha256:api_patched_222": {"digest": "sha256:api_patched_222", "pushed_at": "2026-09-25T10:00:00Z"},
            "web:2.3@sha256:web_patched_222": {"digest": "sha256:web_patched_222", "pushed_at": "2026-09-22T08:00:00Z"},
            "auth:1.0@sha256:auth_base_111": {"digest": "sha256:auth_base_111", "pushed_at": "2026-09-22T16:00:00Z"},
            "payments:1.2@sha256:pay_patched_aaa": {"digest": "sha256:pay_patched_aaa", "pushed_at": "2026-09-22T16:00:00Z"},
        },
    },
    # Snapshot 4: worker + cache added, payments to prod (tag mutation)
    {
        "timestamp": "2026-09-23T16:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "auth": {"tag": "auth:1.0", "digest": "sha256:auth_base_111", "replicaCount": 2, "resourceTier": "small"},
            "payments": {"tag": "payments:1.2", "digest": "sha256:pay_patched_bbb", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2", "digest": "sha256:worker_base_111", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0", "digest": "sha256:cache_patched_222", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1@sha256:api_patched_222": {"digest": "sha256:api_patched_222", "pushed_at": "2026-09-25T10:00:00Z"},
            "web:2.3@sha256:web_patched_222": {"digest": "sha256:web_patched_222", "pushed_at": "2026-09-22T08:00:00Z"},
            "auth:1.0@sha256:auth_base_111": {"digest": "sha256:auth_base_111", "pushed_at": "2026-09-22T16:00:00Z"},
            "payments:1.2@sha256:pay_patched_aaa": {"digest": "sha256:pay_patched_aaa", "pushed_at": "2026-09-22T16:00:00Z"},
            "payments:1.2@sha256:pay_patched_bbb": {"digest": "sha256:pay_patched_bbb", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2@sha256:worker_base_111": {"digest": "sha256:worker_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0@sha256:cache_patched_222": {"digest": "sha256:cache_patched_222", "pushed_at": "2026-09-24T10:00:00Z"},
        },
    },
    # Snapshot 5: cache rushed to prod, auth removed
    {
        "timestamp": "2026-09-24T18:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "payments": {"tag": "payments:1.2", "digest": "sha256:pay_patched_bbb", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2", "digest": "sha256:worker_base_111", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0", "digest": "sha256:cache_patched_222", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1@sha256:api_patched_222": {"digest": "sha256:api_patched_222", "pushed_at": "2026-09-25T10:00:00Z"},
            "web:2.3@sha256:web_patched_222": {"digest": "sha256:web_patched_222", "pushed_at": "2026-09-22T08:00:00Z"},
            "payments:1.2@sha256:pay_patched_bbb": {"digest": "sha256:pay_patched_bbb", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2@sha256:worker_base_111": {"digest": "sha256:worker_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0@sha256:cache_patched_222": {"digest": "sha256:cache_patched_222", "pushed_at": "2026-09-24T10:00:00Z"},
        },
    },
    # Snapshot 6: Final state
    {
        "timestamp": "2026-09-25T09:00:00",
        "services": {
            "api": {"tag": "api:4.1", "digest": "sha256:api_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "web": {"tag": "web:2.3", "digest": "sha256:web_patched_222", "replicaCount": 2, "resourceTier": "medium"},
            "payments": {"tag": "payments:1.2", "digest": "sha256:pay_patched_bbb", "replicaCount": 3, "resourceTier": "large"},
            "worker": {"tag": "worker:3.2", "digest": "sha256:worker_base_111", "replicaCount": 1, "resourceTier": "large"},
            "cache": {"tag": "cache:1.0", "digest": "sha256:cache_patched_222", "replicaCount": 2, "resourceTier": "small"},
        },
        "registry": {
            "api:4.1@sha256:api_patched_222": {"digest": "sha256:api_patched_222", "pushed_at": "2026-09-25T10:00:00Z"},
            "web:2.3@sha256:web_patched_222": {"digest": "sha256:web_patched_222", "pushed_at": "2026-09-22T08:00:00Z"},
            "payments:1.2@sha256:pay_patched_bbb": {"digest": "sha256:pay_patched_bbb", "pushed_at": "2026-09-22T14:00:00Z"},
            "worker:3.2@sha256:worker_base_111": {"digest": "sha256:worker_base_111", "pushed_at": "2026-09-18T10:00:00Z"},
            "cache:1.0@sha256:cache_patched_222": {"digest": "sha256:cache_patched_222", "pushed_at": "2026-09-24T10:00:00Z"},
        },
    },
]

for snap in snapshots:
    from datetime import datetime
    ts = datetime.fromisoformat(snap["timestamp"])
    create_snapshot(SNAPSHOT_DIR, snap["services"], snap["registry"], timestamp=ts)

print(f"Created {len(snapshots)} snapshots in {SNAPSHOT_DIR}")
