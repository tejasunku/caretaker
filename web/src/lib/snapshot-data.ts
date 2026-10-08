// Auto-generated from kubernetes-snapshots/ — do not edit manually
// Run: python3 scripts/generate_data.py

export const SNAPSHOTS = [
  {
    "folder": "2026-09-20T10:00:00",
    "timestamp": "2026-09-20T10:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_base_111",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_base_111",
        "replicaCount": 2,
        "resourceTier": "medium"
      }
    },
    "registry": {
      "api:4.1@sha256:api_base_111": {
        "digest": "sha256:api_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "web:2.3@sha256:web_base_111": {
        "digest": "sha256:web_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      }
    }
  },
  {
    "folder": "2026-09-22T10:00:00",
    "timestamp": "2026-09-22T10:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_base_111",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      }
    },
    "registry": {
      "api:4.1@sha256:api_base_111": {
        "digest": "sha256:api_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "web:2.3@sha256:web_base_111": {
        "digest": "sha256:web_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "web:2.3@sha256:web_patched_222": {
        "digest": "sha256:web_patched_222",
        "pushed_at": "2026-09-22T08:00:00Z"
      }
    }
  },
  {
    "folder": "2026-09-22T18:00:00",
    "timestamp": "2026-09-22T18:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "auth": {
        "tag": "auth:1.0",
        "digest": "sha256:auth_base_111",
        "replicaCount": 2,
        "resourceTier": "small"
      },
      "payments": {
        "tag": "payments:1.2",
        "digest": "sha256:pay_patched_aaa",
        "replicaCount": 3,
        "resourceTier": "large"
      }
    },
    "registry": {
      "api:4.1@sha256:api_base_111": {
        "digest": "sha256:api_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "api:4.1@sha256:api_patched_222": {
        "digest": "sha256:api_patched_222",
        "pushed_at": "2026-09-25T10:00:00Z"
      },
      "web:2.3@sha256:web_patched_222": {
        "digest": "sha256:web_patched_222",
        "pushed_at": "2026-09-22T08:00:00Z"
      },
      "auth:1.0@sha256:auth_base_111": {
        "digest": "sha256:auth_base_111",
        "pushed_at": "2026-09-22T16:00:00Z"
      },
      "payments:1.2@sha256:pay_patched_aaa": {
        "digest": "sha256:pay_patched_aaa",
        "pushed_at": "2026-09-22T16:00:00Z"
      }
    }
  },
  {
    "folder": "2026-09-23T16:00:00",
    "timestamp": "2026-09-23T16:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "auth": {
        "tag": "auth:1.0",
        "digest": "sha256:auth_base_111",
        "replicaCount": 2,
        "resourceTier": "small"
      },
      "payments": {
        "tag": "payments:1.2",
        "digest": "sha256:pay_patched_bbb",
        "replicaCount": 3,
        "resourceTier": "large"
      },
      "worker": {
        "tag": "worker:3.2",
        "digest": "sha256:worker_base_111",
        "replicaCount": 1,
        "resourceTier": "large"
      },
      "cache": {
        "tag": "cache:1.0",
        "digest": "sha256:cache_patched_222",
        "replicaCount": 2,
        "resourceTier": "small"
      }
    },
    "registry": {
      "api:4.1@sha256:api_patched_222": {
        "digest": "sha256:api_patched_222",
        "pushed_at": "2026-09-25T10:00:00Z"
      },
      "web:2.3@sha256:web_patched_222": {
        "digest": "sha256:web_patched_222",
        "pushed_at": "2026-09-22T08:00:00Z"
      },
      "auth:1.0@sha256:auth_base_111": {
        "digest": "sha256:auth_base_111",
        "pushed_at": "2026-09-22T16:00:00Z"
      },
      "payments:1.2@sha256:pay_patched_aaa": {
        "digest": "sha256:pay_patched_aaa",
        "pushed_at": "2026-09-22T16:00:00Z"
      },
      "payments:1.2@sha256:pay_patched_bbb": {
        "digest": "sha256:pay_patched_bbb",
        "pushed_at": "2026-09-22T14:00:00Z"
      },
      "worker:3.2@sha256:worker_base_111": {
        "digest": "sha256:worker_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "cache:1.0@sha256:cache_patched_222": {
        "digest": "sha256:cache_patched_222",
        "pushed_at": "2026-09-24T10:00:00Z"
      }
    }
  },
  {
    "folder": "2026-09-24T18:00:00",
    "timestamp": "2026-09-24T18:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "payments": {
        "tag": "payments:1.2",
        "digest": "sha256:pay_patched_bbb",
        "replicaCount": 3,
        "resourceTier": "large"
      },
      "worker": {
        "tag": "worker:3.2",
        "digest": "sha256:worker_base_111",
        "replicaCount": 1,
        "resourceTier": "large"
      },
      "cache": {
        "tag": "cache:1.0",
        "digest": "sha256:cache_patched_222",
        "replicaCount": 2,
        "resourceTier": "small"
      }
    },
    "registry": {
      "api:4.1@sha256:api_patched_222": {
        "digest": "sha256:api_patched_222",
        "pushed_at": "2026-09-25T10:00:00Z"
      },
      "web:2.3@sha256:web_patched_222": {
        "digest": "sha256:web_patched_222",
        "pushed_at": "2026-09-22T08:00:00Z"
      },
      "payments:1.2@sha256:pay_patched_bbb": {
        "digest": "sha256:pay_patched_bbb",
        "pushed_at": "2026-09-22T14:00:00Z"
      },
      "worker:3.2@sha256:worker_base_111": {
        "digest": "sha256:worker_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "cache:1.0@sha256:cache_patched_222": {
        "digest": "sha256:cache_patched_222",
        "pushed_at": "2026-09-24T10:00:00Z"
      }
    }
  },
  {
    "folder": "2026-09-25T09:00:00",
    "timestamp": "2026-09-25T09:00:00Z",
    "services": {
      "api": {
        "tag": "api:4.1",
        "digest": "sha256:api_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "web": {
        "tag": "web:2.3",
        "digest": "sha256:web_patched_222",
        "replicaCount": 2,
        "resourceTier": "medium"
      },
      "payments": {
        "tag": "payments:1.2",
        "digest": "sha256:pay_patched_bbb",
        "replicaCount": 3,
        "resourceTier": "large"
      },
      "worker": {
        "tag": "worker:3.2",
        "digest": "sha256:worker_base_111",
        "replicaCount": 1,
        "resourceTier": "large"
      },
      "cache": {
        "tag": "cache:1.0",
        "digest": "sha256:cache_patched_222",
        "replicaCount": 2,
        "resourceTier": "small"
      }
    },
    "registry": {
      "api:4.1@sha256:api_patched_222": {
        "digest": "sha256:api_patched_222",
        "pushed_at": "2026-09-25T10:00:00Z"
      },
      "web:2.3@sha256:web_patched_222": {
        "digest": "sha256:web_patched_222",
        "pushed_at": "2026-09-22T08:00:00Z"
      },
      "payments:1.2@sha256:pay_patched_bbb": {
        "digest": "sha256:pay_patched_bbb",
        "pushed_at": "2026-09-22T14:00:00Z"
      },
      "worker:3.2@sha256:worker_base_111": {
        "digest": "sha256:worker_base_111",
        "pushed_at": "2026-09-18T10:00:00Z"
      },
      "cache:1.0@sha256:cache_patched_222": {
        "digest": "sha256:cache_patched_222",
        "pushed_at": "2026-09-24T10:00:00Z"
      }
    }
  }
] as const;
