import type {
  Patch,
  ImageRecord,
  Snapshot,
  SnapshotDiff,
  ServiceDeployment,
  EnvironmentDeployment,
} from "./types";

/**
 * Mock patch registry — 6 CVEs with varying deadlines.
 * Reference date: 2026-09-25T12:00:00Z
 *
 * CVE-2024-1001: released Sep 18, 7d window → deadline Sep 25 (today)
 * CVE-2024-1002: released Sep 20, 7d window → deadline Sep 27 (2d)
 * CVE-2024-1003: released Sep 21, 7d window → deadline Sep 28 (3d)
 * CVE-2024-1004: released Sep 22, 7d window → deadline Sep 29 (4d)
 * CVE-2024-1005: released Sep 24, 14d window → deadline Oct 8
 * CVE-2024-1006: released Sep 15, 30d window → deadline Oct 15
 */
export const MOCK_PATCHES: Patch[] = [
  {
    id: "CVE-2024-1001",
    severity: "critical",
    package: "openssl",
    fixedVersion: "3.0.14",
    releasedAt: "2026-09-18T10:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1002",
    severity: "high",
    package: "curl",
    fixedVersion: "8.5.1",
    releasedAt: "2026-09-20T08:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1003",
    severity: "medium",
    package: "libpng",
    fixedVersion: "1.6.41",
    releasedAt: "2026-09-21T14:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1004",
    severity: "critical",
    package: "log4j",
    fixedVersion: "2.17.2",
    releasedAt: "2026-09-22T09:00:00Z",
    reconciliationWindowDays: 7,
  },
  {
    id: "CVE-2024-1005",
    severity: "low",
    package: "zlib",
    fixedVersion: "1.3.1",
    releasedAt: "2026-09-24T11:00:00Z",
    reconciliationWindowDays: 14,
  },
  {
    id: "CVE-2024-1006",
    severity: "high",
    package: "openssl",
    fixedVersion: "3.0.15",
    releasedAt: "2026-09-15T16:00:00Z",
    reconciliationWindowDays: 30,
  },
];

/**
 * Mock image registry — images built at various times.
 * Each image has all patches released before its build time.
 */
export const MOCK_IMAGES: ImageRecord[] = [
  // api: built today, has all 6 patches — no concerns
  {
    tag: "api:4.1.20260925-100000",
    sha: "sha256:aaa111",
    builtAt: "2026-09-25T10:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base111",
  },

  // web: built Sep 21 (before CVE-2024-1003 released at Sep 21T14:00), missing 1 patch
  // Deadline for CVE-2024-1003 is Sep 28 (3 days). Currently in testing since Sep 25.
  // Needs 2d testing → push to staging by Sep 27 → 1d staging before deadline. Barely makes it.
  {
    tag: "web:2.3.20260921-080000",
    sha: "sha256:bbb222",
    builtAt: "2026-09-21T08:00:00Z",
    baseImage: "nginx:1.25",
    baseDigest: "sha256:base222",
  },

  // worker: built Sep 18 (before CVE-2024-1002/1003/1004), missing 3 patches
  // Earliest deadline: CVE-2024-1002 on Sep 27 (2 days). Not tested at all.
  // Needs 4d minimum (2d testing + 2d staging). Cannot make it.
  {
    tag: "worker:3.2.20260918-100000",
    sha: "sha256:eee555",
    builtAt: "2026-09-18T10:00:00Z",
    baseImage: "golang:1.22",
    baseDigest: "sha256:base444",
  },

  // cache: built Sep 23, has patches through Sep 23 (missing CVE-2024-1005 released Sep 24)
  // Tested only 1d (needs 2d), staged only 1d (needs 2d). Rushed through pipeline.
  {
    tag: "cache:1.0.20260923-150000",
    sha: "sha256:fff666",
    builtAt: "2026-09-23T15:00:00Z",
    baseImage: "redis:7.2",
    baseDigest: "sha256:base555",
  },

  // auth: built Sep 22, has patches through Sep 22
  // Went straight to staging → prod, never tested. Skipped testing.
  {
    tag: "auth:1.0.20260922-160000",
    sha: "sha256:ddd444",
    builtAt: "2026-09-22T16:00:00Z",
    baseImage: "python:3.12-slim",
    baseDigest: "sha256:base333",
  },

  // payments: built Sep 22, has patches through Sep 22
  // Tag tested with SHA aaa, but prod has SHA bbb — tag mutation
  {
    tag: "payments:1.2.20260922-140000",
    sha: "sha256:bbb333",
    builtAt: "2026-09-22T14:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base666",
  },
  // payments: earlier build with different SHA — this is the one that was tested
  {
    tag: "payments:1.2.20260922-140000",
    sha: "sha256:aaa999",
    builtAt: "2026-09-22T14:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base666",
  },
];

/**
 * Mock environment deployments — tracks which SHAs are deployed where.
 *
 * Scenarios:
 * - api: full pipeline, stable, compliant
 * - web: currently in testing, barely makes it
 * - worker: not tested at all, cannot make it
 * - cache: rushed through pipeline (1d testing, 1d staging)
 * - auth: skipped testing entirely
 * - payments: tag mutation (tested SHA ≠ prod SHA)
 */
export const MOCK_ENVIRONMENT_DEPLOYMENTS: EnvironmentDeployment[] = [
  // api:4.1.20260925-100000 — full pipeline, compliant
  // Tested Sep 23-25 (2d), staged Sep 25 (today), prod Sep 25
  { environment: "testing", sha: "sha256:aaa111", tag: "api:4.1.20260925-100000", deployedAt: "2026-09-23T10:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:aaa111", tag: "api:4.1.20260925-100000", deployedAt: "2026-09-23T10:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:aaa111", tag: "api:4.1.20260925-100000", deployedAt: "2026-09-25T10:00:00Z", status: "deployed" },

  // web:2.3.20260921-080000 — currently in testing (started Sep 25), barely makes it
  // Previous build (web:2.3.20260918-100000) was in production
  { environment: "testing", sha: "sha256:old222", tag: "web:2.3.20260918-100000", deployedAt: "2026-09-19T10:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:old222", tag: "web:2.3.20260918-100000", deployedAt: "2026-09-20T10:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:old222", tag: "web:2.3.20260918-100000", deployedAt: "2026-09-20T10:00:00Z", status: "deployed" },
  // Current build in testing (started Sep 25)
  { environment: "testing", sha: "sha256:bbb222", tag: "web:2.3.20260921-080000", deployedAt: "2026-09-25T08:00:00Z", status: "deployed" },

  // worker:3.2.20260918-100000 — not tested at all, cannot make it
  // No deployment entries — completely untested

  // cache:1.0.20260923-150000 — rushed through pipeline
  // Tested only 1d (needs 2d), staged only 0.5d (needs 2d). Rushed to prod.
  { environment: "testing", sha: "sha256:fff666", tag: "cache:1.0.20260923-150000", deployedAt: "2026-09-24T10:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:fff666", tag: "cache:1.0.20260923-150000", deployedAt: "2026-09-25T10:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:fff666", tag: "cache:1.0.20260923-150000", deployedAt: "2026-09-25T12:00:00Z", status: "deployed" },

  // auth:1.0.20260922-160000 — skipped testing, went straight to staging then prod
  { environment: "staging", sha: "sha256:ddd444", tag: "auth:1.0.20260922-160000", deployedAt: "2026-09-22T18:00:00Z", status: "promoted" },
  { environment: "production", sha: "sha256:ddd444", tag: "auth:1.0.20260922-160000", deployedAt: "2026-09-22T20:00:00Z", status: "deployed" },

  // payments:1.2.20260922-140000 — tag mutation
  // Tested with SHA aaa999, but prod has SHA bbb333 under the same tag
  { environment: "testing", sha: "sha256:aaa999", tag: "payments:1.2.20260922-140000", deployedAt: "2026-09-22T16:00:00Z", status: "promoted" },
  { environment: "staging", sha: "sha256:aaa999", tag: "payments:1.2.20260922-140000", deployedAt: "2026-09-23T09:00:00Z", status: "promoted" },
  // Production has DIFFERENT SHA under the same tag — tag mutation!
  { environment: "production", sha: "sha256:bbb333", tag: "payments:1.2.20260922-140000", deployedAt: "2026-09-23T15:00:00Z", status: "deployed" },
];

/**
 * Mock snapshots — deployment timeline showing various change types.
 *
 * Timeline:
 *   Sep 20: Initial state (api + web)
 *   Sep 21: web rebuilt (tag update)
 *   Sep 22: api rebuilt (tag update) + auth + payments added
 *   Sep 23: worker + cache added, payments to prod
 *   Sep 24: cache rushed through pipeline
 *   Sep 25: Final state — api rebuilt, web in testing
 */
export const MOCK_SNAPSHOTS: Snapshot[] = [
  // Snapshot 1: Initial deployment — api and web only
  {
    folder: "2026-09-20T10:00:00",
    timestamp: "2026-09-20T10:00:00Z",
    services: {
      api: { tag: "api:4.1.20260918-100000", sha: "sha256:old111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260918-100000", sha: "sha256:old222", replicaCount: 2, resourceTier: "medium" },
    },
    registry: {
      "api:4.1.20260918-100000": { digest: "sha256:old111", pushed_at: "2026-09-18T10:00:00Z" },
      "web:2.3.20260918-100000": { digest: "sha256:old222", pushed_at: "2026-09-18T10:00:00Z" },
    },
  },
  // Snapshot 2: web rebuilt (tag update)
  {
    folder: "2026-09-21T14:30:00",
    timestamp: "2026-09-21T14:30:00Z",
    services: {
      api: { tag: "api:4.1.20260918-100000", sha: "sha256:old111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260921-080000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
    },
    registry: {
      "api:4.1.20260918-100000": { digest: "sha256:old111", pushed_at: "2026-09-18T10:00:00Z" },
      "web:2.3.20260921-080000": { digest: "sha256:bbb222", pushed_at: "2026-09-21T08:00:00Z" },
    },
  },
  // Snapshot 3: api rebuilt + auth + payments introduced
  {
    folder: "2026-09-22T11:00:00",
    timestamp: "2026-09-22T11:00:00Z",
    services: {
      api: { tag: "api:4.1.20260922-090000", sha: "sha256:old333", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260921-080000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
      auth: { tag: "auth:1.0.20260922-160000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
      payments: { tag: "payments:1.2.20260922-140000", sha: "sha256:aaa999", replicaCount: 3, resourceTier: "large" },
    },
    registry: {
      "api:4.1.20260922-090000": { digest: "sha256:old333", pushed_at: "2026-09-22T09:00:00Z" },
      "web:2.3.20260921-080000": { digest: "sha256:bbb222", pushed_at: "2026-09-21T08:00:00Z" },
      "auth:1.0.20260922-160000": { digest: "sha256:ddd444", pushed_at: "2026-09-22T16:00:00Z" },
      "payments:1.2.20260922-140000": { digest: "sha256:aaa999", pushed_at: "2026-09-22T14:00:00Z" },
    },
  },
  // Snapshot 4: worker + cache added, payments prod (tag mutation happens here)
  {
    folder: "2026-09-23T16:00:00",
    timestamp: "2026-09-23T16:00:00Z",
    services: {
      api: { tag: "api:4.1.20260922-090000", sha: "sha256:old333", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260921-080000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
      auth: { tag: "auth:1.0.20260922-160000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
      payments: { tag: "payments:1.2.20260922-140000", sha: "sha256:bbb333", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2.20260918-100000", sha: "sha256:eee555", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260922-090000": { digest: "sha256:old333", pushed_at: "2026-09-22T09:00:00Z" },
      "web:2.3.20260921-080000": { digest: "sha256:bbb222", pushed_at: "2026-09-21T08:00:00Z" },
      "auth:1.0.20260922-160000": { digest: "sha256:ddd444", pushed_at: "2026-09-22T16:00:00Z" },
      "payments:1.2.20260922-140000": { digest: "sha256:bbb333", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2.20260918-100000": { digest: "sha256:eee555", pushed_at: "2026-09-18T10:00:00Z" },
      "cache:1.0.20260923-150000": { digest: "sha256:fff666", pushed_at: "2026-09-23T15:00:00Z" },
    },
  },
  // Snapshot 5: cache rushed to prod, auth removed
  {
    folder: "2026-09-24T18:00:00",
    timestamp: "2026-09-24T18:00:00Z",
    services: {
      api: { tag: "api:4.1.20260922-090000", sha: "sha256:old333", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260921-080000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
      payments: { tag: "payments:1.2.20260922-140000", sha: "sha256:bbb333", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2.20260918-100000", sha: "sha256:eee555", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260922-090000": { digest: "sha256:old333", pushed_at: "2026-09-22T09:00:00Z" },
      "web:2.3.20260921-080000": { digest: "sha256:bbb222", pushed_at: "2026-09-21T08:00:00Z" },
      "payments:1.2.20260922-140000": { digest: "sha256:bbb333", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2.20260918-100000": { digest: "sha256:eee555", pushed_at: "2026-09-18T10:00:00Z" },
      "cache:1.0.20260923-150000": { digest: "sha256:fff666", pushed_at: "2026-09-23T15:00:00Z" },
    },
  },
  // Snapshot 6: api rebuilt (final state), web enters testing
  {
    folder: "2026-09-25T09:00:00",
    timestamp: "2026-09-25T09:00:00Z",
    services: {
      api: { tag: "api:4.1.20260925-100000", sha: "sha256:aaa111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.20260921-080000", sha: "sha256:bbb222", replicaCount: 2, resourceTier: "medium" },
      payments: { tag: "payments:1.2.20260922-140000", sha: "sha256:bbb333", replicaCount: 3, resourceTier: "large" },
      worker: { tag: "worker:3.2.20260918-100000", sha: "sha256:eee555", replicaCount: 1, resourceTier: "large" },
      cache: { tag: "cache:1.0.20260923-150000", sha: "sha256:fff666", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.20260925-100000": { digest: "sha256:aaa111", pushed_at: "2026-09-25T10:00:00Z" },
      "web:2.3.20260921-080000": { digest: "sha256:bbb222", pushed_at: "2026-09-21T08:00:00Z" },
      "payments:1.2.20260922-140000": { digest: "sha256:bbb333", pushed_at: "2026-09-22T14:00:00Z" },
      "worker:3.2.20260918-100000": { digest: "sha256:eee555", pushed_at: "2026-09-18T10:00:00Z" },
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
