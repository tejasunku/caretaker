import type {
  Patch,
  ImageRecord,
  PatchApplication,
  Snapshot,
  SnapshotDiff,
  ServiceDeployment,
} from "./types";

/**
 * Mock patch registry — realistic CVE data.
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
 */
export const MOCK_IMAGES: ImageRecord[] = [
  {
    tag: "api:4.1.1726912345",
    sha: "sha256:aaa111",
    builtAt: "2026-09-16T12:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base111",
  },
  {
    tag: "api:4.1.1726990000",
    sha: "sha256:ccc333",
    builtAt: "2026-09-21T08:00:00Z",
    baseImage: "node:20-slim",
    baseDigest: "sha256:base111",
  },
  {
    tag: "web:2.3.1726912000",
    sha: "sha256:bbb222",
    builtAt: "2026-09-17T09:00:00Z",
    baseImage: "nginx:1.25",
    baseDigest: "sha256:base222",
  },
  {
    tag: "auth:1.0.1726950000",
    sha: "sha256:ddd444",
    builtAt: "2026-09-20T16:00:00Z",
    baseImage: "python:3.12-slim",
    baseDigest: "sha256:base333",
  },
  {
    tag: "worker:3.2.1726880000",
    sha: "sha256:eee555",
    builtAt: "2026-09-14T10:00:00Z",
    baseImage: "golang:1.22",
    baseDigest: "sha256:base444",
  },
];

/**
 * Mock patch applications — which images have which patches applied.
 */
export const MOCK_APPLICATIONS: PatchApplication[] = [
  {
    imageTag: "api:4.1.1726912345",
    appliedAt: "2026-09-17T10:00:00Z",
    patchesApplied: ["CVE-2024-1001"],
  },
  {
    imageTag: "api:4.1.1726990000",
    appliedAt: "2026-09-22T09:00:00Z",
    patchesApplied: ["CVE-2024-1001", "CVE-2024-1002", "CVE-2024-1003"],
  },
  {
    imageTag: "web:2.3.1726912000",
    appliedAt: "2026-09-18T14:00:00Z",
    patchesApplied: ["CVE-2024-1001"],
  },
  {
    imageTag: "auth:1.0.1726950000",
    appliedAt: "2026-09-21T11:00:00Z",
    patchesApplied: ["CVE-2024-1001", "CVE-2024-1002"],
  },
];

/**
 * Mock snapshots — deployment state at different points in time.
 */
export const MOCK_SNAPSHOTS: Snapshot[] = [
  {
    folder: "2026-09-20T10:00:00",
    timestamp: "2026-09-20T10:00:00Z",
    services: {
      api: { tag: "api:4.1.1726912345", sha: "sha256:aaa111", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.1726912000", sha: "sha256:bbb222", replicaCount: 3, resourceTier: "medium" },
    },
    registry: {
      "api:4.1.1726912345": { digest: "sha256:aaa111", pushed_at: "2026-09-16T12:00:00Z" },
      "web:2.3.1726912000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
    },
  },
  {
    folder: "2026-09-21T14:30:00",
    timestamp: "2026-09-21T14:30:00Z",
    services: {
      api: { tag: "api:4.1.1726990000", sha: "sha256:ccc333", replicaCount: 2, resourceTier: "medium" },
      web: { tag: "web:2.3.1726912000", sha: "sha256:bbb222", replicaCount: 3, resourceTier: "medium" },
      auth: { tag: "auth:1.0.1726950000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
    },
    registry: {
      "api:4.1.1726990000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.1726912000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
      "auth:1.0.1726950000": { digest: "sha256:ddd444", pushed_at: "2026-09-20T16:00:00Z" },
    },
  },
  {
    folder: "2026-09-24T09:00:00",
    timestamp: "2026-09-24T09:00:00Z",
    services: {
      api: { tag: "api:4.1.1726990000", sha: "sha256:ccc333", replicaCount: 3, resourceTier: "large" },
      web: { tag: "web:2.3.1726912000", sha: "sha256:bbb222", replicaCount: 3, resourceTier: "medium" },
      auth: { tag: "auth:1.0.1726950000", sha: "sha256:ddd444", replicaCount: 2, resourceTier: "small" },
      worker: { tag: "worker:3.2.1726880000", sha: "sha256:eee555", replicaCount: 1, resourceTier: "large" },
    },
    registry: {
      "api:4.1.1726990000": { digest: "sha256:ccc333", pushed_at: "2026-09-21T08:00:00Z" },
      "web:2.3.1726912000": { digest: "sha256:bbb222", pushed_at: "2026-09-17T09:00:00Z" },
      "auth:1.0.1726950000": { digest: "sha256:ddd444", pushed_at: "2026-09-20T16:00:00Z" },
      "worker:3.2.1726880000": { digest: "sha256:eee555", pushed_at: "2026-09-14T10:00:00Z" },
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
