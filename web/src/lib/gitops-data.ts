// Auto-generated from gitops repo. Do not edit manually.
// Run scripts/generate_gitops_data.py to regenerate.

export const MOCK_PATCHES = [
  {
    "id": "CVE-2024-1001",
    "severity": "critical",
    "package": "openssl",
    "fixedVersion": "3.0.14",
    "releasedAt": "2026-09-18T10:00:00Z"
  },
  {
    "id": "CVE-2024-1002",
    "severity": "high",
    "package": "curl",
    "fixedVersion": "8.9.1",
    "releasedAt": "2026-09-20T10:00:00Z"
  },
  {
    "id": "CVE-2024-1003",
    "severity": "medium",
    "package": "zlib",
    "fixedVersion": "1.3.1",
    "releasedAt": "2026-09-21T10:00:00Z"
  },
  {
    "id": "CVE-2024-1004",
    "severity": "high",
    "package": "libssl3",
    "fixedVersion": "3.0.15",
    "releasedAt": "2026-09-22T10:00:00Z"
  },
  {
    "id": "CVE-2024-1005",
    "severity": "low",
    "package": "tar",
    "fixedVersion": "1.35",
    "releasedAt": "2026-09-24T10:00:00Z"
  },
  {
    "id": "CVE-2024-1006",
    "severity": "medium",
    "package": "gzip",
    "fixedVersion": "1.13",
    "releasedAt": "2026-09-15T10:00:00Z"
  }
] as const;

export const CURRENT_STATE = {
  "api": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "2",
      "cpu": "500m",
      "memory": "256Mi",
      "path": "/ready",
      "port": "80",
      "initialDelaySeconds": "5",
      "periodSeconds": "5",
      "targetPort": "8080"
    },
    "repository": "localhost:5000/test/python-slim",
    "tag": "1.0",
    "digest": ""
  },
  "auth": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "2",
      "cpu": "500m",
      "memory": "256Mi",
      "path": "/ready",
      "port": "80",
      "initialDelaySeconds": "5",
      "periodSeconds": "5",
      "targetPort": "8080"
    },
    "repository": "localhost:5000/test/python-slim",
    "tag": "1.0",
    "digest": ""
  },
  "cache": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "3",
      "cpu": "200m",
      "memory": "128Mi",
      "port": "6379",
      "initialDelaySeconds": "2",
      "periodSeconds": "5",
      "targetPort": "6379"
    },
    "repository": "localhost:5000/test/ubuntu",
    "tag": "1.0",
    "digest": ""
  },
  "payments": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "2",
      "cpu": "1000m",
      "memory": "512Mi",
      "path": "/ready",
      "port": "80",
      "initialDelaySeconds": "5",
      "periodSeconds": "5",
      "targetPort": "8080"
    },
    "repository": "localhost:5000/test/node-slim",
    "tag": "1.0",
    "digest": ""
  },
  "web": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "2",
      "cpu": "500m",
      "memory": "256Mi",
      "path": "/ready",
      "port": "80",
      "initialDelaySeconds": "5",
      "periodSeconds": "5",
      "targetPort": "3000"
    },
    "repository": "localhost:5000/test/node-slim",
    "tag": "1.0",
    "digest": ""
  },
  "worker": {
    "environments": {
      "testing": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "1"
      },
      "staging": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "2"
      },
      "production": {
        "tag": "1.0",
        "digest": "",
        "replicaCount": "3"
      }
    },
    "infra": {
      "replicaCount": "2",
      "cpu": "1000m",
      "memory": "512Mi",
      "initialDelaySeconds": "15",
      "periodSeconds": "10",
      "port": "80",
      "targetPort": "8080"
    },
    "repository": "localhost:5000/test/python-slim",
    "tag": "1.0",
    "digest": ""
  }
};

export const GIT_HISTORY = [
  {
    "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
    "date": "2026-10-08 12:36:57 -0700",
    "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
  }
];

export const SERVICE_HISTORIES = {
  "api": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ],
  "auth": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ],
  "cache": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ],
  "payments": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ],
  "web": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ],
  "worker": [
    {
      "sha": "a7019c27e3f4f5ba36063b81d9fd0715ed2070ce",
      "date": "2026-10-08 12:36:57 -0700",
      "message": "Initial commit: Helm charts for all services with separated image/infra/env concerns"
    }
  ]
};
