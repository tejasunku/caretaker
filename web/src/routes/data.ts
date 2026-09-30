import { SNAPSHOTS } from "~/lib/snapshot-data";
import { diffSnapshots } from "~/lib/snapshots";
import { resolveImagePatchStatus } from "~/lib/queries";
import { MOCK_PATCHES, MOCK_APPLICATIONS, MOCK_ENVIRONMENT_DEPLOYMENTS } from "~/lib/mock-data";
import { computePatchingStatus, DEFAULT_PATCHING_CONFIG, type PatchingResult } from "~/lib/compliance";
import type { Patch, PatchStatus, EnvironmentDeployment } from "~/lib/types";

// Reference date: Sep 25, 2026 (date of last snapshot)
export const REFERENCE_DATE = new Date("2026-09-25T12:00:00Z");

/** Get the Monday of the week containing a given date */
function getWeekStart(date: Date): Date {
  const d = new Date(date);
  const day = d.getUTCDay();
  const diff = day === 0 ? 6 : day - 1; // Monday = 0
  d.setUTCDate(d.getUTCDate() - diff);
  d.setUTCHours(0, 0, 0, 0);
  return d;
}

/** Build a WeekBucket from a date */
function weekBucketFromDate(date: Date): WeekBucket {
  const start = getWeekStart(date);
  const end = new Date(start);
  end.setUTCDate(end.getUTCDate() + 6);
  end.setUTCHours(23, 59, 59, 999);
  const fmt = (d: Date) => d.toISOString().split("T")[0];
  const shortFmt = (d: Date) => d.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
  return {
    startDate: fmt(start),
    endDate: fmt(end),
    label: `${shortFmt(start)} – ${shortFmt(end)}`,
  };
}

/** Get the current week bucket from REFERENCE_DATE */
export const CURRENT_WEEK = weekBucketFromDate(REFERENCE_DATE);

export function getServiceOverview() {
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
    sha: string;
    status: string;
    explanation: string;
    missingPatches: string[];
    daysUntilDeadline: number | null;
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
    const isNewService = lifecycle.first === latestFolder;

    if (!isActive) {
      services[name] = {
        tag: "",
        sha: "",
        status: "inactive",
        explanation: `Last seen in snapshot ${lifecycle.last}`,
        missingPatches: [],
        daysUntilDeadline: null,
        latestSnapshot: lifecycle.last,
        firstSeen: lifecycle.first,
      };
      continue;
    }

    const svcData = latest.services[name];
    const regEntry = (latest.registry as any)[svcData.tag];
    const images = [{
      tag: svcData.tag,
      sha: svcData.sha,
      builtAt: regEntry?.pushed_at ?? latest.timestamp,
      baseImage: "ubuntu:22.04",
      baseDigest: "",
    }];
    const report = resolveImagePatchStatus(
      svcData.tag,
      images,
      MOCK_PATCHES,
      MOCK_APPLICATIONS
    );

    if (!report) {
      services[name] = {
        tag: svcData.tag,
        sha: svcData.sha,
        status: "compliant",
        explanation: "No patch data available",
        missingPatches: [],
        daysUntilDeadline: null,
        latestSnapshot: latestFolder,
        firstSeen: lifecycle.first,
      };
      continue;
    }

    const missingPatchDetails = report.details.filter((d) => !d.applied);
    const missingPatches = missingPatchDetails.map((d) => d.patch.id);
    const hasViablePatchedVersion = report.patchesMissing > 0;

    const compliance = computePatchingStatus(
      missingPatchDetails.map((d) => ({
        id: d.patch.id,
        reconciliationWindowDays: 7,
        releasedAt: d.patch.releasedAt,
      })),
      hasViablePatchedVersion,
      false,
      isNewService,
      DEFAULT_PATCHING_CONFIG,
      REFERENCE_DATE
    );

    services[name] = {
      tag: svcData.tag,
      sha: svcData.sha,
      status: compliance.status,
      explanation: compliance.explanation,
      missingPatches,
      daysUntilDeadline: compliance.daysUntilDeadline,
      latestSnapshot: latestFolder,
      firstSeen: lifecycle.first,
    };
  }

  return { services, snapshots: diffs, referenceDate: REFERENCE_DATE.toISOString() };
}

export interface PatchDetail {
  patch: Patch;
  applied: boolean;
  appliedAt: string | null;
  deadline: string;
  daysUntilDeadline: number | null;
  overdue: boolean;
}

export interface TestingGap {
  tag: string;
  sha: string;
  environments: string[];
  deployedAt: string;
  message: string;
}

export interface TagMutation {
  snapshot: string;
  timestamp: string;
  oldTag: string;
  newTag: string;
  oldSha: string;
  newSha: string;
}

export interface ReconciliationWindowImage {
  featureTag: string;       // base tag, e.g. "api:4.1"
  latestImage: { tag: string; sha: string; builtAt: string };
  withinWindow: boolean;    // was latest image built during this window
  patched: boolean;         // does latest image have this patch
  isCurrentInProd: boolean;
  isCurrentInStaging: boolean;
  isCurrentInTesting: boolean;
}

export interface WeekBucket {
  startDate: string;   // ISO date (Monday)
  endDate: string;     // ISO date (Sunday)
  label: string;       // e.g. "Sep 15 – Sep 21"
}

export interface ReconciliationWindow {
  patch: Patch;
  introducedWeek: WeekBucket;
  deadlineWeek: WeekBucket;
  featureLines: ReconciliationWindowImage[];
  status: "compliant" | "non_compliant";
  explanation: string;
}

export interface StabilityImage {
  tag: string;
  sha: string;
  label: string;
  testing: { deployedAt: string; durationMs: number | null; metRequired: boolean } | null;
  staging: { deployedAt: string; durationMs: number | null; metRequired: boolean } | null;
  production: { deployedAt: string; durationMs: number | null; metRequired: boolean } | null;
  isStable: boolean;
}

export interface ServiceDetailResult {
  name: string;
  history: {
    snapshot: { folder: string; timestamp: string };
    present: boolean;
    tag: string;
    sha: string;
    replicaCount: number;
    resourceTier: string;
    diffType: string | null;
  }[];
  compliance: PatchingResult | null;
  latestSvc: { tag: string; sha: string } | null;
  referenceDate: string;
  currentWeek: WeekBucket;
  deployments: EnvironmentDeployment[];
  patchDetails: PatchDetail[];
  reconciliationWindows: ReconciliationWindow[];
  stabilityImages: StabilityImage[];
  testingGaps: TestingGap[];
  tagMutations: TagMutation[];
  snapshotCount: number;
}

export function getServiceDetail(name: string): ServiceDetailResult | null {
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
      if (prevSvc.tag !== svc.tag || prevSvc.sha !== svc.sha) {
        diffType = "tag_update";
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
      sha: svc?.sha ?? "",
      replicaCount: svc?.replicaCount ?? 0,
      resourceTier: svc?.resourceTier ?? "",
      diffType,
    });
  }

  const latest = snapshots[snapshots.length - 1];
  const latestSvc = latest.services[name];
  let compliance: PatchingResult | null = null;
  let patchDetails: PatchDetail[] = [];

  if (latestSvc) {
    const regEntry = (latest.registry as any)[latestSvc.tag];
    const images = [{
      tag: latestSvc.tag,
      sha: latestSvc.sha,
      builtAt: regEntry?.pushed_at ?? latest.timestamp,
      baseImage: "ubuntu:22.04",
      baseDigest: "",
    }];
    const report = resolveImagePatchStatus(
      latestSvc.tag,
      images,
      MOCK_PATCHES,
      MOCK_APPLICATIONS
    );
    if (report) {
      const missingPatchDetails = report.details.filter((d) => !d.applied);
      const hasViable = report.patchesMissing > 0;
      compliance = computePatchingStatus(
        missingPatchDetails.map((d) => ({
          id: d.patch.id,
          reconciliationWindowDays: 7,
          releasedAt: d.patch.releasedAt,
        })),
        hasViable,
        false,
        false,
        DEFAULT_PATCHING_CONFIG,
        REFERENCE_DATE
      );

      patchDetails = report.details.map((d) => {
        const deadline = new Date(d.patch.releasedAt);
        deadline.setDate(deadline.getDate() + d.patch.reconciliationWindowDays);
        const daysUntil = Math.ceil(
          (deadline.getTime() - REFERENCE_DATE.getTime()) / (1000 * 60 * 60 * 24)
        );
        return {
          patch: d.patch,
          applied: d.applied,
          appliedAt: d.appliedAt ?? null,
          deadline: deadline.toISOString(),
          daysUntilDeadline: daysUntil,
          overdue: daysUntil < 0,
        };
      });
    }
  }

  // Detect testing gaps: SHAs that went to staging or prod without going through testing first
  const serviceDeployments = MOCK_ENVIRONMENT_DEPLOYMENTS.filter(
    (dep) => dep.tag.startsWith(name + ":")
  );
  const testingGaps: TestingGap[] = [];
  const shasByEnv: Record<string, EnvironmentDeployment[]> = {};
  for (const dep of serviceDeployments) {
    if (!shasByEnv[dep.sha]) shasByEnv[dep.sha] = [];
    shasByEnv[dep.sha].push(dep);
  }
  for (const [sha, deps] of Object.entries(shasByEnv)) {
    const envs = [...new Set(deps.map((d) => d.environment))];
    const hasTesting = envs.includes("testing");
    if (!hasTesting && (envs.includes("staging") || envs.includes("production"))) {
      const firstDeploy = deps.sort(
        (a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime()
      )[0];
      testingGaps.push({
        tag: firstDeploy.tag,
        sha,
        environments: envs,
        deployedAt: firstDeploy.deployedAt,
        message: `SHA ${sha.slice(0, 13)}… deployed to ${envs.join(", ")} without passing through testing`,
      });
    }
  }

  // Detect tag mutations: same tag, different SHA across snapshots
  const tagMutations: TagMutation[] = [];
  for (let i = 1; i < snapshots.length; i++) {
    const prev = snapshots[i - 1];
    const curr = snapshots[i];
    if (prev.services[name] && curr.services[name]) {
      const prevTag = prev.services[name].tag;
      const currTag = curr.services[name].tag;
      if (prevTag === currTag && prev.services[name].sha !== curr.services[name].sha) {
        tagMutations.push({
          snapshot: curr.folder,
          timestamp: curr.timestamp,
          oldTag: prevTag,
          newTag: currTag,
          oldSha: prev.services[name].sha,
          newSha: curr.services[name].sha,
        });
      }
    }
  }

  // Collect all unique images for this service across all snapshots
  const allImages: { tag: string; sha: string; builtAt: string }[] = [];
  const seenTags = new Set<string>();
  for (const snap of snapshots) {
    if (snap.services[name]) {
      const svc = snap.services[name];
      if (!seenTags.has(svc.tag)) {
        seenTags.add(svc.tag);
        const regEntry = (snap.registry as any)[svc.tag];
        allImages.push({
          tag: svc.tag,
          sha: svc.sha,
          builtAt: regEntry?.pushed_at ?? snap.timestamp,
        });
      }
    }
  }

  // Determine which image is current in each environment
  const latestByEnv: Record<string, EnvironmentDeployment | null> = { testing: null, staging: null, production: null };
  for (const env of ["testing", "staging", "production"] as const) {
    const envDeps = serviceDeployments
      .filter((d) => d.environment === env)
      .sort((a, b) => new Date(b.deployedAt).getTime() - new Date(a.deployedAt).getTime());
    latestByEnv[env] = envDeps[0] ?? null;
  }

  // Build reconciliation windows — grouped by base feature tag, bucketed by week
  const reconciliationWindows: ReconciliationWindow[] = MOCK_PATCHES.map((patch) => {
    const introducedWeek = weekBucketFromDate(new Date(patch.releasedAt));
    const deadlineDate = new Date(patch.releasedAt);
    deadlineDate.setDate(deadlineDate.getDate() + patch.reconciliationWindowDays);
    const deadlineWeek = weekBucketFromDate(deadlineDate);

    // Group images by base feature tag (strip timestamp)
    const featureMap: Record<string, { tag: string; sha: string; builtAt: string }[]> = {};
    for (const img of allImages) {
      // tag format: "name:version.timestamp" → base is "name:version"
      const parts = img.tag.split(".");
      parts.pop(); // remove timestamp
      const featureTag = parts.join(".");
      if (!featureMap[featureTag]) featureMap[featureTag] = [];
      featureMap[featureTag].push(img);
    }

    const featureLines: ReconciliationWindowImage[] = Object.entries(featureMap).map(([featureTag, images]) => {
      // Sort by builtAt descending, pick latest
      const sorted = images.sort((a, b) => new Date(b.builtAt).getTime() - new Date(a.builtAt).getTime());
      const latest = sorted[0];
      const builtAt = new Date(latest.builtAt);
      const withinWindow = builtAt >= new Date(introducedWeek.startDate) && builtAt <= new Date(deadlineWeek.endDate);
      const app = MOCK_APPLICATIONS.find((a) => a.imageTag === latest.tag);
      const patched = app?.patchesApplied.includes(patch.id) ?? false;

      return {
        featureTag,
        latestImage: { tag: latest.tag, sha: latest.sha, builtAt: latest.builtAt },
        withinWindow,
        patched,
        isCurrentInProd: latestByEnv.production?.tag === latest.tag,
        isCurrentInStaging: latestByEnv.staging?.tag === latest.tag,
        isCurrentInTesting: latestByEnv.testing?.tag === latest.tag,
      };
    });

    const currentProdFeature = featureLines.find((f) => f.isCurrentInProd);
    const currentProdPatched = currentProdFeature?.patched ?? false;

    // Also check if an older build of this feature line is in production
    const prodImageExists = latestByEnv.production && featureLines.some((f) =>
      allImages.some((img) => img.tag === latestByEnv.production!.tag && img.tag.startsWith(f.featureTag))
    );

    const status: "compliant" | "non_compliant" = currentProdPatched ? "compliant" : "non_compliant";

    const explanation = currentProdPatched
      ? "Current production feature line has this patch"
      : currentProdFeature
        ? `Current production feature (${currentProdFeature.featureTag}) missing this patch`
        : prodImageExists
          ? (() => {
              // Check if the latest image for this feature line has the patch
              const latestForFeature = featureLines.find((f) =>
                allImages.some((img) => img.tag === latestByEnv.production!.tag && img.tag.startsWith(f.featureTag))
              );
              if (latestForFeature?.patched) {
                return `Production uses older build (${latestByEnv.production!.tag}) — patched version exists but not deployed`;
              }
              return `Production uses older build (${latestByEnv.production!.tag}) — missing this patch`;
            })()
          : "No production image found";

    return {
      patch,
      introducedWeek,
      deadlineWeek,
      featureLines,
      status,
      explanation,
    };
  });

  // Build stability images: latest in prod, last/current in staging, current/last in testing
  const stabilityImages: StabilityImage[] = [];
  const envOrder = ["testing", "staging", "production"] as const;

  function computeEnvDuration(tag: string, env: "testing" | "staging" | "production") {
    const deps = serviceDeployments
      .filter((d) => d.tag === tag && d.environment === env)
      .sort((a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime());
    if (deps.length === 0) return null;
    const latest = deps[deps.length - 1];
    const startTime = new Date(latest.deployedAt).getTime();
    const isCurrent = latestByEnv[env]?.tag === tag;
    const endTime = isCurrent ? REFERENCE_DATE.getTime() : startTime + 24 * 60 * 60 * 1000;
    const durationMs = endTime - startTime;
    const requiredDays = env === "testing" ? 2 : 2;
    const metRequired = durationMs >= requiredDays * 24 * 60 * 60 * 1000;
    return { deployedAt: latest.deployedAt, durationMs, metRequired };
  }

  // Latest in production
  if (latestByEnv.production) {
    const prodTag = latestByEnv.production.tag;
    stabilityImages.push({
      tag: prodTag,
      sha: latestByEnv.production.sha,
      label: "Production",
      testing: computeEnvDuration(prodTag, "testing"),
      staging: computeEnvDuration(prodTag, "staging"),
      production: computeEnvDuration(prodTag, "production"),
      isStable: (computeEnvDuration(prodTag, "testing")?.metRequired ?? false) &&
                (computeEnvDuration(prodTag, "staging")?.metRequired ?? false),
    });
  }

  // Last/current in staging (if different from prod)
  if (latestByEnv.staging && latestByEnv.staging.tag !== latestByEnv.production?.tag) {
    const stagTag = latestByEnv.staging.tag;
    stabilityImages.push({
      tag: stagTag,
      sha: latestByEnv.staging.sha,
      label: "Staging",
      testing: computeEnvDuration(stagTag, "testing"),
      staging: computeEnvDuration(stagTag, "staging"),
      production: computeEnvDuration(stagTag, "production"),
      isStable: (computeEnvDuration(stagTag, "testing")?.metRequired ?? false) &&
                (computeEnvDuration(stagTag, "staging")?.metRequired ?? false),
    });
  } else if (latestByEnv.staging) {
    // Same tag in staging and prod — still show it but note it
    const stagTag = latestByEnv.staging.tag;
    stabilityImages.push({
      tag: stagTag,
      sha: latestByEnv.staging.sha,
      label: "Staging (same as prod)",
      testing: computeEnvDuration(stagTag, "testing"),
      staging: computeEnvDuration(stagTag, "staging"),
      production: computeEnvDuration(stagTag, "production"),
      isStable: (computeEnvDuration(stagTag, "testing")?.metRequired ?? false) &&
                (computeEnvDuration(stagTag, "staging")?.metRequired ?? false),
    });
  }

  // Current/last in testing (if different from both)
  if (latestByEnv.testing && latestByEnv.testing.tag !== latestByEnv.production?.tag && latestByEnv.testing.tag !== latestByEnv.staging?.tag) {
    const testTag = latestByEnv.testing.tag;
    stabilityImages.push({
      tag: testTag,
      sha: latestByEnv.testing.sha,
      label: "Testing",
      testing: computeEnvDuration(testTag, "testing"),
      staging: computeEnvDuration(testTag, "staging"),
      production: computeEnvDuration(testTag, "production"),
      isStable: (computeEnvDuration(testTag, "testing")?.metRequired ?? false) &&
                (computeEnvDuration(testTag, "staging")?.metRequired ?? false),
    });
  }

  return {
    name,
    history,
    compliance,
    latestSvc: latestSvc ? { tag: latestSvc.tag, sha: latestSvc.sha } : null,
    referenceDate: REFERENCE_DATE.toISOString(),
    currentWeek: CURRENT_WEEK,
    deployments: MOCK_ENVIRONMENT_DEPLOYMENTS,
    patchDetails,
    reconciliationWindows,
    stabilityImages,
    testingGaps,
    tagMutations,
    snapshotCount: snapshots.length,
  };
}
