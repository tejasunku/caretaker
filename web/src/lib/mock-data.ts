import type {
  Patch,
  ImageRecord,
  Snapshot,
  SnapshotDiff,
  ServiceDeployment,
  EnvironmentDeployment,
} from "./types";

/**
 * Mock patch registry — realistic CVE data over a 2-week window.
 */
export const MOCK_PATCHES: Patch[] = [
  {
    id: "CVE-2024-1001",
    severity: "critical",
    package: "openssl",
    fixedVersion: "3.0.14",
    releasedAt: "2026-09-15T10:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1002",
    severity: "high",
    package: "curl",
    fixedVersion: "8.5.1",
    releasedAt: "2026-09-17T08:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1003",
    severity: "medium",
    package: "libpng",
    fixedVersion: "1.6.41",
    releasedAt: "2026-09-18T14:00:00Z",
    reconciliationWindowDays: 14,
  },
  {
    id: "CVE-2024-1004",
    severity: "critical",
    package: "log4j",
    fixedVersion: "2.17.2",
    releasedAt: "2026-09-20T09:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1005",
    severity: "low",
    package: "zlib",
    fixedVersion: "1.3.1",
    releasedAt: "2026-09-22T11:00:00Z",
    reconciliationWindowDays: 30,
  },
  {
    id: "CVE-2024-1006",
    severity: "high",
    package: "openssl",
    fixedVersion: "3.0.15",
    releasedAt: "2026-09-24T16:00:00Z",
    reconciliationWindowDays: 7,
  },
];

/**
 * Mock image registry — images built at various times.
 * Each image represents a real build with a timestamp.
 */
export const MOCK_IMAGES: ImageRecord[] = [
  // api: built twice — first build missing patches, second build gets some
  {
    tag: "api:4.1.20260916-120000",
    sha: "sha256:aaa111",
    builtAt: "2026-09-16T12:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base111",
  },
  {
    tag: "api:4.1.20260921-080000",
    sha: "sha256:ccc333",
    builtAt: "2026-09-21T08:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base111",
  },
  // web: built once, never patched
  {
    tag: "web:2.3.20260917-090000",
    sha: "sha256:bbb222",
    builtAt: "2026-09-17T09:00:00Z",
    baseImage: "nginx:1.25",
    baseDigest: "sha256:base222",
  },
  // auth: built mid-window, partially patched
  {
    tag: "auth:1.0.20260920-160000",
    sha: "sha256:ddd444",
    builtAt: "2026-09-20T16:00:00Z",
    baseImage: "python:3.12-slim",
    baseDigest: "sha256:base333",
  },
  // worker: old build, many patches missing
  {
    tag: "worker:3.2.20260914-100000",
    sha: "sha256:eee555",
    builtAt: "2026-09-14T10:00:00Z",
    baseImage: "golang:1.22",
    baseDigest: "sha256:base444",
  },
  // cache: new service, built recently
  {
    tag: "cache:1.0.20260923-150000",
    sha: "sha256:fff666",
    builtAt: "2026-09-23T15:00:00Z",
    baseImage: "redis:7.2",
    baseDigest: "sha256:base555",
  },
];

/**
 * Mock environment deployments — tracks which SHAs are deployed where.
 * This is the testing/staging/production promotion trail.
 *
 * Key scenario: api:4.1.20260921-080000 was pushed to staging without being tested first.
 */
export const MOCK_ENVIRONMENT_DEPLOYMENTS: EnvironmentDeployment[] = [
  // api:4.1.20260916-120000 — old build, went through full pipeline
  { environment: "testing", sha: "sha256:aaa111", tag: "api:4.1.20260916-120000", deployedAt: "2026-09-16T14:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:aaa111", tag: "api:4.1.20260916-120000", deployedAt: "2026-09-17T09:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:aaa111", tag: "api:4.1.20260916-120000", deployedAt: "2026-09-17T15:00:00Z", status: "promoted" },

  // api:4.1.20260921-080000 — NEW BUILD, testing gap: skipped testing, went straight to staging
  { environment: "staging", sha: "sha256:ccc333", tag: "api:4.1.20260921-080000", deployedAt: "2026-09-21T10:00:00Z", status: "deployed" },
  // Note: no testing entry for sha256:ccc333 — TESTING GAP

  // web:2.3.20260917-090000 — normal pipeline
  { environment: "testing", sha: "sha256:bbb222", tag: "web:2.3.20260917-090000", deployedAt: "2026-09-17T12:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:bbb222", tag: "web:2.3.20260917-090000", deployedAt: "2026-09-18T09:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:bbb222", tag: "web:2.3.20260917-090000", deployedAt: "2026-09-18T15:00:00Z", status: "promoted" },

  // web:2.3.20260924-100000 — rebuilt web, went through testing but not yet prod
  { environment: "testing", sha: "sha256:ggg777", tag: "web:2.3.20260924-100000", deployedAt: "2026-09-24T12:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:ggg777", tag: "web:2.3.20260924-100000", deployedAt: "2026-09-24T16:00:00Z", status: "deployed" },

  // auth:1.0.20260920-160000 — normal pipeline, then deprecated
  { environment: "testing", sha: "sha256:ddd444", tag: "auth:1.0.20260920-160000", deployedAt: "2026-09-21T13:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:ddd444", tag: "auth:1.0.20260920-160000", deployedAt: "2026-09-22T09:00:00Z", status: "rolled_back" },

  // worker:3.2.20260914-100000 — old build, never tested
  // No entries — completely untested

  // worker:3.2.20260924-140000 — rebuilt, testing in progress
  { environment: "testing", sha: "sha256:hhh888", tag: "worker:3.2.20260924-140000", deployedAt: "2026-09-24T15:00:00Z", status: "deployed" },

  // cache:1.0.20260923-150000 — new service, went straight to production (risky)
  { environment: "production", sha: "sha256:fff666", tag: "cache:1.0.20260923-150000", deployedAt: "2026-09-23T18:00:00Z", status: "deployed" },
];

/**
 * Mock snapshots — deployment timeline showing various change types.
 *
 * Timeline:
 *   Sep 20: Initial state (api + web)
 *   Sep 21: api patched (tag update) + auth added (new service)
 *   Sep 22: web scaled up (replica change) + api upgraded to large (tier change)
 *   Sep 23: auth removed (deprecated) + cache + worker added (service churn)
 *   Sep 24: web patched (tag update) + worker patched (tag update) + api scaled down
 *   Sep 25: Final state — all services stable
 */
export const MOCK_SNAPSHOTS: Snapshot[] = [
  // Snapshot 1: Initial deployment — api and web only
  {
    folder: "2026-09-20T10:00:00",
    timestamp: "2026-09-20T10:00:00Z",
    services: {
      api: { tag: "api:4.1.20260916-120000", sha: "sha256:aaa111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260917-090000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
    },
    registry: {
      "api:4.1.20260916-120000": { digest: "sha256:aaa111", pushed_at: "2026-09-16T12:00:00Z" },
      "web:2.3.20260917-090000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
    },
  },
  // Snapshot 2: api rebuilt (tag update) + auth service introduced
  {
    folder: "2026-09-21T14:30:00",
    timestamp: "2026-09-21T14:30:00Z",
    services: {
      api: { tag: "api:4.1.20260921-080000", sha: "sha256:ccc333", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260917-090000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
      auth: { tag: "auth:1.0.20260920-160000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260921-080000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.20260917-090000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
      "auth:1.0.20260920-160000": { digest: "sha256:ddd444", pushed_at: "2026-09-20T16:00:00Z" },
    },
  },
  // Snapshot 3: web scaled up (infra change) + api tier upgrade (infra change)
  {
    folder: "2026-09-22T11:00:00",
    timestamp: "2026-09-22T11:00:00Z",
    services: {
      api: { tag: "api:4.1.20260921-080000", sha: "sha256:ccc333", replicaCount: 3, resourceTier: "large" },
      web: { tag: "web:2.3.20260917-090000", sha: "sha256:bbb222", replicaCount: 4, resourceTier: "medium" },
      auth: { tag: "auth:1.0.20260920-160000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260921-080000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.20260917-090000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
      "auth:1.0.20260920-160000": { digest: "sha256:ddd444", pushed_at: "2026-09-20T16:00:00Z" },
    },
  },
  // Snapshot 4: auth deprecated (removed) + worker and cache introduced
  {
    folder: "2026-09-23T16:00:00",
    timestamp: "2026-09-23T16:00:00Z",
    services: {
      api: { tag: "api:4.1.20260921-080000", sha: "sha256:ccc333", replicaCount: 3, resourceTier: "large" },
      web: { tag: "web:2.3.20260917-090000", sha: "sha256:bbb222", replicaCount: 4, resourceTier: "medium" },
      worker: { tag: "worker:3.2.20260914-100000", sha: "sha256:eee555", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260921-080000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.20260917-090000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
      "worker:3.2.20260914-100000": { digest: "sha256:eee555", pushed_at: "2026-09-14T10:00:00Z" },
      "cache:1.0.20260923-150000": { digest: "sha256:fff666", pushed_at: "2026-09-23T15:00:00Z" },
    },
  },
  // Snapshot 5: web + worker patched (tag updates) + api scaled down (infra)
  {
    folder: "2026-09-24T18:00:00",
    timestamp: "2026-09-24T18:00:00Z",
    services: {
      api: { tag: "api:4.1.20260921-080000", sha: "sha256:ccc333", replicaCount: 2, resourceTier: "large" },
      web: { tag: "web:2.3.20260924-100000", sha: "sha256:ggg777", replicaCount: 4, resourceTier: "medium" },
      worker: { tag: "worker:3.2.20260924-140000", sha: "sha256:hhh888", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260921-080000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.20260924-100000": { digest: "sha256:ggg777", pushed_at: "2026-09-24T10:00:00Z" },
      "worker:3.2.20260924-140000": { digest: "sha256:hhh888", pushed_at: "2026-09-24T14:00:00Z" },
      "cache:1.0.20260923-150000": { digest: "sha256:fff666", pushed_at: "2026-09-23T15:00:00Z" },
    },
  },
  // Snapshot 6: Final stable state — no changes from snapshot 5
  {
    folder: "2026-09-25T09:00:00",
    timestamp: "2026-09-25T09:00:00Z",
    services: {
      api: { tag: "api:4.1.20260921-080000", sha: "sha256:ccc333", replicaCount: 2, resourceTier: "large" },
      web: { tag: "web:2.3.20260924-100000", sha: "sha256:ggg777", replicaCount: 4, resourceTier: "medium" },
      worker: { tag: "worker:3.2.20260924-140000", sha: "sha256:hhh888", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260921-080000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.20260924-100000": { digest: "sha256:ggg777", pushed_at: "2026-09-24T10:00:00Z" },
      "worker:3.2.20260924-140000": { digest: "sha256:hhh888", pushed_at: "2026-09-24T14:00:00Z" },
      "cache:1.0.20260923-150000": { digest: "sha256:fff666", pushed_at: "2026-09-23T15:00:00Z" },
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
        prevData.sha !== currData.sha ||
        prevData.replicaCount !== currData.replicaCount ||
        prevData.resourceTier !== currData.resourceTier
      ) {
        const diffType = [];
        if (prevData.tag !== currData.tag) diffType.push("tag_update");
        if (prevData.sha !== currData.sha) diffType.push("sha_change");
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
