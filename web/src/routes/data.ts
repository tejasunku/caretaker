import { SNAPSHOTS as RAW_SNAPSHOTS } from "~/lib/snapshot-data";
import { diffSnapshots } from "~/lib/snapshots";
import { MOCK_PATCHES, MOCK_ENVIRONMENT_DEPLOYMENTS } from "~/lib/mock-data";
import type { Patch, EnvironmentDeployment, Snapshot } from "~/lib/types";

// Cast to proper Snapshot type (raw data is inferred with literal types)
const SNAPSHOTS = RAW_SNAPSHOTS as unknown as Snapshot[];

// Reference date: Sep 25, 2026 (date of last snapshot)
export const REFERENCE_DATE = new Date("2026-09-25T12:00:00Z");

/**
 * Get patches that are missing from an image (released after image was built).
 * This is the core staleness check — no deadline logic.
 */
function getMissingPatches(imageBuiltAt: string): Patch[] {
  const builtAt = new Date(imageBuiltAt);
  return MOCK_PATCHES.filter((p) => new Date(p.releasedAt) > builtAt);
}

export async function getServiceOverview() {
  const snapshots = [...SNAPSHOTS];
  if (snapshots.length === 0) return { services: {}, snapshots: [] };

  const latest = snapshots[snapshots.length - 1];
  const diffs = snapshots.slice(1).map((s, i) => ({
    timestamp: s.timestamp,
    folder: s.folder,
    diff: diffSnapshots(snapshots[i], s),
  }));

  const services: Record<string, {
    tag: string;
    digest: string;
    isStale: boolean;
    missingPatches: string[];
    missingSeverity: Record<string, number>; // severity → count
    hasTestingGap: boolean;
    hasTagMutation: boolean;
    latestSnapshot: string;
    firstSeen: string;
  }> = {};

  const allServices = new Set<string>();
  for (const snap of snapshots) {
    for (const name of Object.keys(snap.services)) {
      allServices.add(name);
    }
  }

  const serviceLifecycle: Record<string, { first: string; last: string }> = {};
  for (const name of allServices) {
    let first = "";
    let last = "";
    for (const snap of snapshots) {
      if (name in snap.services) {
        if (!first) first = snap.folder;
        last = snap.folder;
      }
    }
    serviceLifecycle[name] = { first, last };
  }

  const latestFolder = latest.folder;

  for (const name of allServices) {
    const lifecycle = serviceLifecycle[name];
    const isActive = name in latest.services;

    if (!isActive) {
      services[name] = {
        tag: "",
        digest: "",
        isStale: false,
        missingPatches: [],
        missingSeverity: {},
        hasTestingGap: false,
        hasTagMutation: false,
        latestSnapshot: lifecycle.last,
        firstSeen: lifecycle.first,
      };
      continue;
    }

    const svcData = latest.services[name];
    const regKey = `${svcData.tag}@${svcData.digest}`;
    const regEntry = (latest.registry as any)[regKey];
    const builtAt = regEntry?.pushed_at ?? latest.timestamp;

    const missing = getMissingPatches(builtAt);
    const missingSeverity: Record<string, number> = {};
    for (const p of missing) {
      missingSeverity[p.severity] = (missingSeverity[p.severity] || 0) + 1;
    }

    services[name] = {
      tag: svcData.tag,
      digest: svcData.digest,
      isStale: missing.length > 0,
      missingPatches: missing.map((p) => p.id),
      missingSeverity,
      hasTestingGap: false,
      hasTagMutation: false,
      latestSnapshot: latestFolder,
      firstSeen: lifecycle.first,
    };
  }

  // Detect tag mutations (same tag, different digest)
  const tagMutations: TagMutation[] = [];
  const servicesWithMutation = new Set<string>();
  for (const name of allServices) {
    if (!(name in latest.services)) continue;
    for (let i = 1; i < snapshots.length; i++) {
      const prev = snapshots[i - 1];
      const curr = snapshots[i];
      if (prev.services[name] && curr.services[name]) {
        const prevTag = prev.services[name].tag;
        const currTag = curr.services[name].tag;
        if (prevTag === currTag && prev.services[name].digest !== curr.services[name].digest) {
          tagMutations.push({
            snapshot: curr.folder,
            timestamp: curr.timestamp,
            oldTag: prevTag,
            newTag: currTag,
            oldDigest: prev.services[name].digest,
            newDigest: curr.services[name].digest,
          });
          servicesWithMutation.add(name);
        }
      }
    }
  }

  // Detect testing gaps (digest went to staging/prod without testing)
  const testingGaps: TestingGap[] = [];
  const servicesWithGap = new Set<string>();
  for (const name of allServices) {
    if (!(name in latest.services)) continue;
    const serviceDeployments = MOCK_ENVIRONMENT_DEPLOYMENTS.filter(
      (dep) => dep.tag.startsWith(name + ":")
    );
    const digestsByEnv: Record<string, EnvironmentDeployment[]> = {};
    for (const dep of serviceDeployments) {
      if (!digestsByEnv[dep.digest]) digestsByEnv[dep.digest] = [];
      digestsByEnv[dep.digest].push(dep);
    }
    for (const [digest, deps] of Object.entries(digestsByEnv)) {
      const envs = [...new Set(deps.map((d) => d.environment))];
      const hasTesting = envs.includes("testing");
      if (!hasTesting && (envs.includes("staging") || envs.includes("production"))) {
        const firstDeploy = deps.sort(
          (a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime()
        )[0];
        testingGaps.push({
          tag: firstDeploy.tag,
          digest,
          environments: envs,
          deployedAt: firstDeploy.deployedAt,
          message: `Digest ${digest.slice(0, 13)}… deployed to ${envs.join(", ")} without passing through testing`,
        });
        servicesWithGap.add(name);
      }
    }
  }

  // Set per-service flags
  for (const name of allServices) {
    if (services[name]) {
      services[name].hasTestingGap = servicesWithGap.has(name);
      services[name].hasTagMutation = servicesWithMutation.has(name);
    }
  }

  return { services, snapshots: diffs, referenceDate: REFERENCE_DATE.toISOString(), tagMutations, testingGaps };
}

export interface TestingGap {
  tag: string;
  digest: string;
  environments: string[];
  deployedAt: string;
  message: string;
}

export interface TagMutation {
  snapshot: string;
  timestamp: string;
  oldTag: string;
  newTag: string;
  oldDigest: string;
  newDigest: string;
}

export interface FeatureLineStatus {
  tag: string;                    // Feature tag, e.g., "api:4.1"
  originatorDigest: string;       // Original feature build digest
  latestPatchedDigest: string | null;  // Latest patched digest (if any)
  isStale: boolean;               // Is prod image missing patches?
  missingPatches: string[];       // Patch IDs missing from prod image
  patchedExists: boolean;         // Does a patched version exist?
  patchedInTesting: boolean;      // Is patched version in testing?
  patchedInStaging: boolean;      // Is patched version in staging?
  patchedInProd: boolean;         // Is patched version in prod?
}

export interface ServiceDetailResult {
  name: string;
  history: {
    snapshot: { folder: string; timestamp: string };
    present: boolean;
    tag: string;
    digest: string;
    replicaCount: number;
    resourceTier: string;
    diffType: string | null;
  }[];
  featureLine: FeatureLineStatus | null;
  latestSvc: { tag: string; digest: string } | null;
  deployments: EnvironmentDeployment[];
  allDigests: { tag: string; digest: string; builtAt: string; isPatched: boolean }[];
  testingGaps: TestingGap[];
  tagMutations: TagMutation[];
  snapshotCount: number;
}

export async function getServiceDetail(name: string): Promise<ServiceDetailResult | null> {
  const snapshots = [...SNAPSHOTS];
  if (snapshots.length === 0) return null;

  const history: ServiceDetailResult["history"] = [];

  for (let i = 0; i < snapshots.length; i++) {
    const snap = snapshots[i];
    const svc = snap.services[name];
    const prev = i > 0 ? snapshots[i - 1] : null;
    let diffType: string | null = null;

    if (prev && prev.services[name] && svc) {
      const prevSvc = prev.services[name];
      if (prevSvc.tag !== svc.tag || prevSvc.digest !== svc.digest) {
        diffType = "digest_change";
      } else if (
        prevSvc.replicaCount !== svc.replicaCount ||
        prevSvc.resourceTier !== svc.resourceTier
      ) {
        diffType = "infra_change";
      } else {
        diffType = "unchanged";
      }
    } else if (!prev && svc) {
      diffType = "initial";
    } else if (prev && prev.services[name] && !svc) {
      diffType = "removed";
    }

    history.push({
      snapshot: { folder: snap.folder, timestamp: snap.timestamp },
      present: !!svc,
      tag: svc?.tag ?? "—",
      digest: svc?.digest ?? "",
      replicaCount: svc?.replicaCount ?? 0,
      resourceTier: svc?.resourceTier ?? "",
      diffType,
    });
  }

  const latest = snapshots[snapshots.length - 1];
  const latestSvc = latest.services[name];

  // Collect all unique digests for this service across all snapshots
  const allDigests: { tag: string; digest: string; builtAt: string; isPatched: boolean }[] = [];
  const seenDigests = new Set<string>();
  for (const snap of snapshots) {
    if (snap.services[name]) {
      const svc = snap.services[name];
      if (!seenDigests.has(svc.digest)) {
        seenDigests.add(svc.digest);
        const regKey = `${svc.tag}@${svc.digest}`;
        const regEntry = (snap.registry as any)[regKey];
        allDigests.push({
          tag: svc.tag,
          digest: svc.digest,
          builtAt: regEntry?.pushed_at ?? snap.timestamp,
          isPatched: svc.digest.includes("patched"), // heuristic for mock data
        });
      }
    }
  }

  // Determine which digest is current in each environment
  const serviceDeployments = MOCK_ENVIRONMENT_DEPLOYMENTS.filter(
    (dep) => dep.tag.startsWith(name + ":")
  );
  const latestByEnv: Record<string, EnvironmentDeployment | null> = { testing: null, staging: null, production: null };
  for (const env of ["testing", "staging", "production"] as const) {
    const envDeps = serviceDeployments
      .filter((d) => d.environment === env)
      .sort((a, b) => new Date(b.deployedAt).getTime() - new Date(a.deployedAt).getTime());
    latestByEnv[env] = envDeps[0] ?? null;
  }

  // Build feature line status
  let featureLine: FeatureLineStatus | null = null;
  if (latestSvc) {
    const prodDigest = latestByEnv.production?.digest;
    const prodImage = allDigests.find((d) => d.digest === prodDigest);
    const missing = prodImage ? getMissingPatches(prodImage.builtAt) : [];
    
    // Find latest patched digest (if any)
    const patchedDigests = allDigests.filter((d) => d.isPatched);
    const latestPatched = patchedDigests.sort(
      (a, b) => new Date(b.builtAt).getTime() - new Date(a.builtAt).getTime()
    )[0];

    featureLine = {
      tag: latestSvc.tag,
      originatorDigest: allDigests[0]?.digest ?? "",
      latestPatchedDigest: latestPatched?.digest ?? null,
      isStale: missing.length > 0,
      missingPatches: missing.map((p) => p.id),
      patchedExists: patchedDigests.length > 0,
      patchedInTesting: latestByEnv.testing?.digest === latestPatched?.digest,
      patchedInStaging: latestByEnv.staging?.digest === latestPatched?.digest,
      patchedInProd: latestByEnv.production?.digest === latestPatched?.digest,
    };
  }

  // Detect testing gaps
  const testingGaps: TestingGap[] = [];
  const digestsByEnv: Record<string, EnvironmentDeployment[]> = {};
  for (const dep of serviceDeployments) {
    if (!digestsByEnv[dep.digest]) digestsByEnv[dep.digest] = [];
    digestsByEnv[dep.digest].push(dep);
  }
  for (const [digest, deps] of Object.entries(digestsByEnv)) {
    const envs = [...new Set(deps.map((d) => d.environment))];
    const hasTesting = envs.includes("testing");
    if (!hasTesting && (envs.includes("staging") || envs.includes("production"))) {
      const firstDeploy = deps.sort(
        (a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime()
      )[0];
      testingGaps.push({
        tag: firstDeploy.tag,
        digest,
        environments: envs,
        deployedAt: firstDeploy.deployedAt,
        message: `Digest ${digest.slice(0, 13)}… deployed to ${envs.join(", ")} without passing through testing`,
      });
    }
  }

  // Detect tag mutations
  const tagMutations: TagMutation[] = [];
  for (let i = 1; i < snapshots.length; i++) {
    const prev = snapshots[i - 1];
    const curr = snapshots[i];
    if (prev.services[name] && curr.services[name]) {
      const prevTag = prev.services[name].tag;
      const currTag = curr.services[name].tag;
      if (prevTag === currTag && prev.services[name].digest !== curr.services[name].digest) {
        tagMutations.push({
          snapshot: curr.folder,
          timestamp: curr.timestamp,
          oldTag: prevTag,
          newTag: currTag,
          oldDigest: prev.services[name].digest,
          newDigest: curr.services[name].digest,
        });
      }
    }
  }

  return {
    name,
    history,
    featureLine,
    latestSvc: latestSvc ? { tag: latestSvc.tag, digest: latestSvc.digest } : null,
    deployments: MOCK_ENVIRONMENT_DEPLOYMENTS,
    allDigests,
    testingGaps,
    tagMutations,
    snapshotCount: snapshots.length,
  };
}
