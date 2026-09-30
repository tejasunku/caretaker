import { SNAPSHOTS } from "~/lib/snapshot-data";
import { diffSnapshots } from "~/lib/snapshots";
import { MOCK_PATCHES, MOCK_ENVIRONMENT_DEPLOYMENTS } from "~/lib/mock-data";
import { computePatchingStatus, DEFAULT_PATCHING_CONFIG, type PatchingResult } from "~/lib/compliance";
import type { Patch, EnvironmentDeployment } from "~/lib/types";

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
    const builtAt = new Date(regEntry?.pushed_at ?? latest.timestamp);

    // Image has all patches released before its build time
    const missingPatches = MOCK_PATCHES
      .filter((p) => new Date(p.releasedAt) > builtAt)
      .map((p) => p.id);

    // Compute deadline for the most urgent missing patch
    let daysUntilDeadline: number | null = null;
    if (missingPatches.length > 0) {
      const missingDetails = MOCK_PATCHES
        .filter((p) => missingPatches.includes(p.id))
        .map((p) => {
          const deadline = new Date(p.releasedAt);
          deadline.setDate(deadline.getDate() + p.reconciliationWindowDays);
          return {
            id: p.id,
            reconciliationWindowDays: p.reconciliationWindowDays,
            releasedAt: p.releasedAt,
            deadline,
          };
        })
        .sort((a, b) => a.deadline.getTime() - b.deadline.getTime());
      const earliest = missingDetails[0];
      daysUntilDeadline = Math.ceil(
        (earliest.deadline.getTime() - REFERENCE_DATE.getTime()) / (1000 * 60 * 60 * 24)
      );
    }

    const compliance = computePatchingStatus(
      missingPatches.map((id) => {
        const p = MOCK_PATCHES.find((pp) => pp.id === id)!;
        return { id: p.id, reconciliationWindowDays: 7, releasedAt: p.releasedAt };
      }),
      missingPatches.length > 0,
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

  // Detect tag mutations across all services
  const tagMutations: TagMutation[] = [];
  for (const name of allServices) {
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
  }

  // Detect testing gaps across all services
  const testingGaps: TestingGap[] = [];
  for (const name of allServices) {
    const serviceDeployments = MOCK_ENVIRONMENT_DEPLOYMENTS.filter(
      (dep) => dep.tag.startsWith(name + ":")
    );
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
  }

  return { services, snapshots: diffs, referenceDate: REFERENCE_DATE.toISOString(), tagMutations, testingGaps };
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

export interface ReconciliationWindowFeatureLine {
  featureTag: string;
  latestImage: { tag: string; sha: string; builtAt: string };
  withinWindow: boolean;
  allPatched: boolean;
  missingPatches: string[];
  isCurrentInProd: boolean;
  isCurrentInStaging: boolean;
  isCurrentInTesting: boolean;
}

export interface ReconciliationWindow {
  week: WeekBucket;
  patches: { id: string; severity: string; package: string }[];
  featureLines: ReconciliationWindowFeatureLine[];
  status: "compliant" | "non_compliant";
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
    const builtAt = new Date(regEntry?.pushed_at ?? latest.timestamp);
    const images = [{
      tag: latestSvc.tag,
      sha: latestSvc.sha,
      builtAt: regEntry?.pushed_at ?? latest.timestamp,
      baseImage: "ubuntu:22.04",
      baseDigest: "",
    }];

    // Determine which patches this image has based on build time
    patchDetails = MOCK_PATCHES.map((patch) => {
      const hasPatch = new Date(patch.releasedAt) <= builtAt;
      const deadline = new Date(patch.releasedAt);
      deadline.setDate(deadline.getDate() + patch.reconciliationWindowDays);
      const daysUntil = Math.ceil(
        (deadline.getTime() - REFERENCE_DATE.getTime()) / (1000 * 60 * 60 * 24)
      );
      return {
        patch,
        applied: hasPatch,
        appliedAt: hasPatch ? latestSvc.tag : null,
        deadline: deadline.toISOString(),
        daysUntilDeadline: daysUntil,
        overdue: !hasPatch && daysUntil < 0,
      };
    });

    const missingPatchDetails = patchDetails.filter((d) => !d.applied);
    const hasViable = missingPatchDetails.length > 0;
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

  // Build reconciliation windows — grouped by deadline week
  const featureMap: Record<string, { tag: string; sha: string; builtAt: string }[]> = {};
  for (const img of allImages) {
    const parts = img.tag.split(".");
    parts.pop();
    const featureTag = parts.join(".");
    if (!featureMap[featureTag]) featureMap[featureTag] = [];
    featureMap[featureTag].push(img);
  }

  // Group patches by deadline week (sorted chronologically)
  const weekEntries: { week: WeekBucket; patches: Patch[] }[] = [];
  const weekMap: Record<string, { week: WeekBucket; patches: Patch[] }> = {};
  for (const patch of MOCK_PATCHES) {
    const deadlineDate = new Date(patch.releasedAt);
    deadlineDate.setDate(deadlineDate.getDate() + patch.reconciliationWindowDays);
    const deadlineWeek = weekBucketFromDate(deadlineDate);
    const key = deadlineWeek.startDate;
    if (!weekMap[key]) {
      weekMap[key] = { week: deadlineWeek, patches: [] };
      weekEntries.push(weekMap[key]);
    }
    weekMap[key].patches.push(patch);
  }
  weekEntries.sort((a, b) => a.week.startDate.localeCompare(b.week.startDate));

  // For each feature line, find the latest week it fully satisfies
  // An image has all patches released before its build time
  const featureLineWeeks: Record<string, number> = {};
  for (const [featureTag, images] of Object.entries(featureMap)) {
    const sorted = images.sort((a, b) => new Date(b.builtAt).getTime() - new Date(a.builtAt).getTime());
    const latest = sorted[0];
    const builtAt = new Date(latest.builtAt);

    let latestFullySatisfied = -1;
    for (let i = 0; i < weekEntries.length; i++) {
      // Image has all patches released before its build time
      const allPatched = weekEntries[i].patches.every((p) => new Date(p.releasedAt) <= builtAt);
      if (allPatched) latestFullySatisfied = i;
    }
    featureLineWeeks[featureTag] = latestFullySatisfied;
  }

  // Build windows: each week gets feature lines that fully satisfy up to that week
  const reconciliationWindows: ReconciliationWindow[] = weekEntries.map((entry, weekIdx) => {
    const featureLines: ReconciliationWindowFeatureLine[] = Object.entries(featureMap).map(([featureTag, images]) => {
      const sorted = images.sort((a, b) => new Date(b.builtAt).getTime() - new Date(a.builtAt).getTime());
      const latest = sorted[0];
      const builtAt = new Date(latest.builtAt);
      const missingPatches = entry.patches
        .filter((p) => new Date(p.releasedAt) > builtAt)
        .map((p) => p.id);

      return {
        featureTag,
        latestImage: { tag: latest.tag, sha: latest.sha, builtAt: latest.builtAt },
        withinWindow: true,
        allPatched: missingPatches.length === 0,
        missingPatches,
        isCurrentInProd: latestByEnv.production?.tag === latest.tag,
        isCurrentInStaging: latestByEnv.staging?.tag === latest.tag,
        isCurrentInTesting: latestByEnv.testing?.tag === latest.tag,
      };
    });

    // Feature lines placed here = those whose latest fully-satisfied week is this one
    const placedHere = featureLines.filter((f) => featureLineWeeks[f.featureTag] === weekIdx);

    const status: "compliant" | "non_compliant" =
      placedHere.some((f) => f.isCurrentInProd)
        ? "compliant"
        : "non_compliant";

    return {
      week: entry.week,
      patches: entry.patches.map((p) => ({ id: p.id, severity: p.severity, package: p.package })),
      featureLines: placedHere,
      status,
    };
  });

  // Reverse to show newest windows first
  reconciliationWindows.reverse();

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
