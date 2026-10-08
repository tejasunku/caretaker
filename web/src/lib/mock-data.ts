import type {
  Patch,
  ImageRecord,
  Snapshot,
  SnapshotDiff,
  ServiceDeployment,
  EnvironmentDeployment,
} from "./types";

/**
 * Mock patch registry — 6 CVEs.
 * Reference date: 2026-09-25T12:00:00Z
 *
 * The goal is patching velocity, not compliance enforcement.
 * We just track what's available and what's been applied.
 */
export const MOCK_PATCHES: Patch[] = [
  {
    id: "CVE-2024-1001",
    severity: "critical",
    package: "openssl",
    fixedVersion: "3.0.14",
    releasedAt: "2026-09-18T10:00:00Z",
  },
  {
    id: "CVE-2024-1002",
    severity: "high",
    package: "curl",
    fixedVersion: "8.5.1",
    releasedAt: "2026-09-20T08:00:00Z",
  },
  {
    id: "CVE-2024-1003",
    severity: "medium",
    package: "libpng",
    fixedVersion: "1.6.41",
    releasedAt: "2026-09-21T14:00:00Z",
  },
  {
    id: "CVE-2024-1004",
    severity: "critical",
    package: "log4j",
    fixedVersion: "2.17.2",
    releasedAt: "2026-09-22T09:00:00Z",
  },
  {
    id: "CVE-2024-1005",
    severity: "low",
    package: "zlib",
    fixedVersion: "1.3.1",
    releasedAt: "2026-09-24T11:00:00Z",
  },
  {
    id: "CVE-2024-1006",
    severity: "high",
    package: "openssl",
    fixedVersion: "3.0.15",
    releasedAt: "2026-09-15T16:00:00Z",
  },
];

/**
 * Mock image registry — digest-based lineage model.
 *
 * Tags are simple feature identifiers (api:4.1, web:2.3).
 * Patched images are new digests tracking originator + parent.
 *
 * Lineage example:
 *   api:4.1 → digest A (originator=A, parent=base)
 *     └── digest B (originator=A, parent=A, patched)
 */
export const MOCK_IMAGES: ImageRecord[] = [
  // === API SERVICE ===
  {
    tag: "api:4.1",
    digest: "sha256:api_base_111",
    builtAt: "2026-09-18T10:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:node20_base",
    originatorDigest: "sha256:api_base_111",
    parentDigest: "sha256:node20_base",
    isPatched: false,
    patchesApplied: [],
  },
  {
    tag: "api:4.1",
    digest: "sha256:api_patched_222",
    builtAt: "2026-09-25T10:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:node20_base",
    originatorDigest: "sha256:api_base_111",
    parentDigest: "sha256:api_base_111",
    isPatched: true,
    patchesApplied: [
      "CVE-2024-1001", "CVE-2024-1002", "CVE-2024-1003",
      "CVE-2024-1004", "CVE-2024-1005", "CVE-2024-1006",
    ],
  },

  // === WEB SERVICE ===
  {
    tag: "web:2.3",
    digest: "sha256:web_base_111",
    builtAt: "2026-09-18T10:00:00Z",
    baseImage: "nginx:1.25",
    baseDigest: "sha256:nginx125_base",
    originatorDigest: "sha256:web_base_111",
    parentDigest: "sha256:nginx125_base",
    isPatched: false,
    patchesApplied: [],
  },
  {
    tag: "web:2.3",
    digest: "sha256:web_patched_222",
    builtAt: "2026-09-22T08:00:00Z",
    baseImage: "nginx:1.25",
    baseDigest: "sha256:nginx125_base",
    originatorDigest: "sha256:web_base_111",
    parentDigest: "sha256:web_base_111",
    isPatched: true,
    patchesApplied: ["CVE-2024-1001", "CVE-2024-1006"],
  },

  // === WORKER SERVICE ===
  {
    tag: "worker:3.2",
    digest: "sha256:worker_base_111",
    builtAt: "2026-09-18T10:00:00Z",
    baseImage: "golang:1.22",
    baseDigest: "sha256:golang122_base",
    originatorDigest: "sha256:worker_base_111",
    parentDigest: "sha256:golang122_base",
    isPatched: false,
    patchesApplied: [],
  },

  // === CACHE SERVICE ===
  {
    tag: "cache:1.0",
    digest: "sha256:cache_base_111",
    builtAt: "2026-09-23T15:00:00Z",
    baseImage: "redis:7.2",
    baseDigest: "sha256:redis72_base",
    originatorDigest: "sha256:cache_base_111",
    parentDigest: "sha256:redis72_base",
    isPatched: false,
    patchesApplied: [],
  },
  {
    tag: "cache:1.0",
    digest: "sha256:cache_patched_222",
    builtAt: "2026-09-24T10:00:00Z",
    baseImage: "redis:7.2",
    baseDigest: "sha256:redis72_base",
    originatorDigest: "sha256:cache_base_111",
    parentDigest: "sha256:cache_base_111",
    isPatched: true,
    patchesApplied: [
      "CVE-2024-1001", "CVE-2024-1002", "CVE-2024-1003",
      "CVE-2024-1004", "CVE-2024-1006",
    ],
  },

  // === AUTH SERVICE ===
  {
    tag: "auth:1.0",
    digest: "sha256:auth_base_111",
    builtAt: "2026-09-22T16:00:00Z",
    baseImage: "python:3.12-slim",
    baseDigest: "sha256:python312_base",
    originatorDigest: "sha256:auth_base_111",
    parentDigest: "sha256:python312_base",
    isPatched: false,
    patchesApplied: [],
  },

  // === PAYMENTS SERVICE ===
  {
    tag: "payments:1.2",
    digest: "sha256:pay_base_111",
    builtAt: "2026-09-22T14:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:node20_base",
    originatorDigest: "sha256:pay_base_111",
    parentDigest: "sha256:node20_base",
    isPatched: false,
    patchesApplied: [],
  },
  {
    tag: "payments:1.2",
    digest: "sha256:pay_patched_aaa",
    builtAt: "2026-09-22T16:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:node20_base",
    originatorDigest: "sha256:pay_base_111",
    parentDigest: "sha256:pay_base_111",
    isPatched: true,
    patchesApplied: ["CVE-2024-1001", "CVE-2024-1006"],
  },
  {
    tag: "payments:1.2",
    digest: "sha256:pay_patched_bbb",
    builtAt: "2026-09-22T14:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:node20_base",
    originatorDigest: "sha256:pay_base_111",
    parentDigest: "sha256:pay_base_111",
    isPatched: true,
    patchesApplied: ["CVE-2024-1001", "CVE-2024-1002", "CVE-2024-1006"],
  },
];

/**
 * Mock environment deployments — tracks which digests are deployed where.
 */
export const MOCK_ENVIRONMENT_DEPLOYMENTS: EnvironmentDeployment[] = [
  // api:4.1 — full pipeline, patched version in prod
  { environment: "testing", digest: "sha256:api_patched_222", tag: "api:4.1", deployedAt: "2026-09-23T10:00:00Z", status: "promoted" },
  { environment: "staging", digest: "sha256:api_patched_222", tag: "api:4.1", deployedAt: "2026-09-23T10:00:00Z", status: "promoted" },
  { environment: "production", digest: "sha256:api_patched_222", tag: "api:4.1", deployedAt: "2026-09-25T10:00:00Z", status: "deployed" },

  // web:2.3 — patched version in testing, old version in prod
  { environment: "testing", digest: "sha256:web_base_111", tag: "web:2.3", deployedAt: "2026-09-19T10:00:00Z", status: "promoted" },
  { environment: "staging", digest: "sha256:web_base_111", tag: "web:2.3", deployedAt: "2026-09-20T10:00:00Z", status: "promoted" },
  { environment: "production", digest: "sha256:web_base_111", tag: "web:2.3", deployedAt: "2026-09-20T10:00:00Z", status: "deployed" },
  { environment: "testing", digest: "sha256:web_patched_222", tag: "web:2.3", deployedAt: "2026-09-25T08:00:00Z", status: "deployed" },

  // worker:3.2 — not deployed anywhere (needs patching)

  // cache:1.0 — patched version rushed to prod
  { environment: "testing", digest: "sha256:cache_patched_222", tag: "cache:1.0", deployedAt: "2026-09-24T10:00:00Z", status: "promoted" },
  { environment: "staging", digest: "sha256:cache_patched_222", tag: "cache:1.0", deployedAt: "2026-09-25T10:00:00Z", status: "promoted" },
  { environment: "production", digest: "sha256:cache_patched_222", tag: "cache:1.0", deployedAt: "2026-09-25T12:00:00Z", status: "deployed" },

  // auth:1.0 — skipped testing, went straight to staging then prod
  { environment: "staging", digest: "sha256:auth_base_111", tag: "auth:1.0", deployedAt: "2026-09-22T18:00:00Z", status: "promoted" },
  { environment: "production", digest: "sha256:auth_base_111", tag: "auth:1.0", deployedAt: "2026-09-22T20:00:00Z", status: "deployed" },

  // payments:1.2 — tag mutation (tested digest ≠ prod digest)
  { environment: "testing", digest: "sha256:pay_patched_aaa", tag: "payments:1.2", deployedAt: "2026-09-22T16:00:00Z", status: "promoted" },
  { environment: "staging", digest: "sha256:pay_patched_aaa", tag: "payments:1.2", deployedAt: "2026-09-23T09:00:00Z", status: "promoted" },
  { environment: "production", digest: "sha256:pay_patched_bbb", tag: "payments:1.2", deployedAt: "2026-09-23T15:00:00Z", status: "deployed" },
];

/**
 * Mock snapshots — deployment timeline.
 */
export const MOCK_SNAPSHOTS: Snapshot[] = [
  {
    folder: "2026-09-20T10:00:00",
    timestamp: "2026-09-20T10:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_base_111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_base_111", replicaCount: 2, resourceTier: "medium" },
    },
    registry: {
      "api:4.1@sha256:api_base_111": { digest: "sha256:api_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "web:2.3@sha256:web_base_111": { digest: "sha256:web_base_111", pushed_at: "2026-09-18T10:00:00Z" },
    },
  },
  {
    folder: "2026-09-22T10:00:00",
    timestamp: "2026-09-22T10:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_base_111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_patched_222", replicaCount: 2, resourceTier: "medium" },
    },
    registry: {
      "api:4.1@sha256:api_base_111": { digest: "sha256:api_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "web:2.3@sha256:web_base_111": { digest: "sha256:web_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "web:2.3@sha256:web_patched_222": { digest: "sha256:web_patched_222", pushed_at: "2026-09-22T08:00:00Z" },
    },
  },
  {
    folder: "2026-09-22T18:00:00",
    timestamp: "2026-09-22T18:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_patched_222", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_patched_222", replicaCount: 2, resourceTier: "medium" },
      auth: { tag: "auth:1.0", digest: "sha256:auth_base_111", replicaCount: 2, resourceTier: "small" },
      payments: { tag: "payments:1.2", digest: "sha256:pay_patched_aaa", replicaCount: 3, resourceTier: "large" },
    },
    registry: {
      "api:4.1@sha256:api_base_111": { digest: "sha256:api_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "api:4.1@sha256:api_patched_222": { digest: "sha256:api_patched_222", pushed_at: "2026-09-25T10:00:00Z" },
      "web:2.3@sha256:web_patched_222": { digest: "sha256:web_patched_222", pushed_at: "2026-09-22T08:00:00Z" },
      "auth:1.0@sha256:auth_base_111": { digest: "sha256:auth_base_111", pushed_at: "2026-09-22T16:00:00Z" },
      "payments:1.2@sha256:pay_patched_aaa": { digest: "sha256:pay_patched_aaa", pushed_at: "2026-09-22T16:00:00Z" },
    },
  },
  {
    folder: "2026-09-23T16:00:00",
    timestamp: "2026-09-23T16:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_patched_222", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_patched_222", replicaCount: 2, resourceTier: "medium" },
      auth: { tag: "auth:1.0", digest: "sha256:auth_base_111", replicaCount: 2, resourceTier: "small" },
      payments: { tag: "payments:1.2", digest: "sha256:pay_patched_bbb", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2", digest: "sha256:worker_base_111", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0", digest: "sha256:cache_patched_222", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1@sha256:api_patched_222": { digest: "sha256:api_patched_222", pushed_at: "2026-09-25T10:00:00Z" },
      "web:2.3@sha256:web_patched_222": { digest: "sha256:web_patched_222", pushed_at: "2026-09-22T08:00:00Z" },
      "auth:1.0@sha256:auth_base_111": { digest: "sha256:auth_base_111", pushed_at: "2026-09-22T16:00:00Z" },
      "payments:1.2@sha256:pay_patched_aaa": { digest: "sha256:pay_patched_aaa", pushed_at: "2026-09-22T16:00:00Z" },
      "payments:1.2@sha256:pay_patched_bbb": { digest: "sha256:pay_patched_bbb", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2@sha256:worker_base_111": { digest: "sha256:worker_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "cache:1.0@sha256:cache_patched_222": { digest: "sha256:cache_patched_222", pushed_at: "2026-09-24T10:00:00Z" },
    },
  },
  {
    folder: "2026-09-24T18:00:00",
    timestamp: "2026-09-24T18:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_patched_222", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_patched_222", replicaCount: 2, resourceTier: "medium" },
      payments: { tag: "payments:1.2", digest: "sha256:pay_patched_bbb", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2", digest: "sha256:worker_base_111", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0", digest: "sha256:cache_patched_222", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1@sha256:api_patched_222": { digest: "sha256:api_patched_222", pushed_at: "2026-09-25T10:00:00Z" },
      "web:2.3@sha256:web_patched_222": { digest: "sha256:web_patched_222", pushed_at: "2026-09-22T08:00:00Z" },
      "payments:1.2@sha256:pay_patched_bbb": { digest: "sha256:pay_patched_bbb", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2@sha256:worker_base_111": { digest: "sha256:worker_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "cache:1.0@sha256:cache_patched_222": { digest: "sha256:cache_patched_222", pushed_at: "2026-09-24T10:00:00Z" },
    },
  },
  {
    folder: "2026-09-25T09:00:00",
    timestamp: "2026-09-25T09:00:00Z",
    services: {
      api: { tag: "api:4.1", digest: "sha256:api_patched_222", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3", digest: "sha256:web_patched_222", replicaCount: 2, resourceTier: "medium" },
      payments: { tag: "payments:1.2", digest: "sha256:pay_patched_bbb", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2", digest: "sha256:worker_base_111", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0", digest: "sha256:cache_patched_222", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1@sha256:api_patched_222": { digest: "sha256:api_patched_222", pushed_at: "2026-09-25T10:00:00Z" },
      "web:2.3@sha256:web_patched_222": { digest: "sha256:web_patched_222", pushed_at: "2026-09-22T08:00:00Z" },
      "payments:1.2@sha256:pay_patched_bbb": { digest: "sha256:pay_patched_bbb", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2@sha256:worker_base_111": { digest: "sha256:worker_base_111", pushed_at: "2026-09-18T10:00:00Z" },
      "cache:1.0@sha256:cache_patched_222": { digest: "sha256:cache_patched_222", pushed_at: "2026-09-24T10:00:00Z" },
    },
  },
];

/**
 * Diff two snapshots.
 */
export function diffSnapshots(prev: Snapshot, curr: Snapshot): SnapshotDiff {
  const prevServices = prev.services;
  const currServices = curr.services;

  const added: Record<string, ServiceDeployment> = {};
  const removed: Record<string, ServiceDeployment> = {};
  const changed: Record<string, { prev: ServiceDeployment; curr: ServiceDeployment; diffType: string }> = {};
  const unchanged: Record<string, ServiceDeployment> = {};

  for (const [name, currData] of Object.entries(currServices)) {
    if (!(name in prevServices)) {
      added[name] = currData;
    } else {
      const prevData = prevServices[name];
      if (
        prevData.tag !== currData.tag ||
        prevData.digest !== currData.digest ||
        prevData.replicaCount !== currData.replicaCount ||
        prevData.resourceTier !== currData.resourceTier
      ) {
        const diffType = [];
        if (prevData.tag !== currData.tag) diffType.push("tag_update");
        if (prevData.digest !== currData.digest) diffType.push("digest_change");
        if (prevData.replicaCount !== currData.replicaCount) diffType.push("replica_change");
        if (prevData.resourceTier !== currData.resourceTier) diffType.push("resource_tier_change");
        changed[name] = { prev: prevData, curr: currData, diffType: diffType.join("+") };
      } else {
        unchanged[name] = currData;
      }
    }
  }

  for (const [name, prevData] of Object.entries(prevServices)) {
    if (!(name in currServices)) {
      removed[name] = prevData;
    }
  }

  return { added, removed, changed, unchanged };
}
