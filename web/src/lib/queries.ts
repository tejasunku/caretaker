import type {
  Patch,
  ImageRecord,
  PatchApplication,
  PatchStatus,
  ImagePatchReport,
  TimeRangeReport,
  Snapshot,
  TagMutation,
  Environment,
  EnvironmentDeployment,
  TagMutationReport,
} from "./types";

/**
 * Query A: Given an image tag, which patches does it have/miss?
 */
export function resolveImagePatchStatus(
  imageTag: string,
  images: ImageRecord[],
  patches: Patch[],
  applications: PatchApplication[]
): ImagePatchReport | null {
  const image = images.find((i) => i.tag === imageTag);
  if (!image) return null;

  // Patches available at the time the image was built
  const availableAtBuild = patches.filter(
    (p) => new Date(p.releasedAt) <= new Date(image.builtAt)
  );

  // Patches applied to this image
  const app = applications.find((a) => a.imageTag === imageTag);
  const appliedIds = new Set(app?.patchesApplied ?? []);

  const details: PatchStatus[] = availableAtBuild.map((patch) => ({
    patch,
    applied: appliedIds.has(patch.id),
    appliedAt: app?.appliedAt,
  }));

  const patchesMissing = details.filter((d) => !d.applied).length;

  return {
    imageTag: image.tag,
    builtAt: image.builtAt,
    baseImage: image.baseImage,
    totalPatchesAvailable: availableAtBuild.length,
    patchesApplied: availableAtBuild.length - patchesMissing,
    patchesMissing,
    status: patchesMissing === 0 ? "compliant" : "non_compliant",
    details,
  };
}

/**
 * Query B: Given a time range, which patches need to be applied and to which images?
 */
export function resolveTimeRangePatches(
  rangeStart: string,
  rangeEnd: string,
  patches: Patch[],
  images: ImageRecord[],
  applications: PatchApplication[]
): TimeRangeReport {
  // Patches released in the time range
  const patchesReleased = patches.filter((p) => {
    const released = new Date(p.releasedAt);
    return released >= new Date(rangeStart) && released <= new Date(rangeEnd);
  });

  // For each image, check which of these patches are missing
  const imagesNeedingPatches = images
    .map((image) => {
      const app = applications.find((a) => a.imageTag === image.tag);
      const appliedIds = new Set(app?.patchesApplied ?? []);

      const missingPatches = patchesReleased.filter(
        (p) =>
          !appliedIds.has(p.id) &&
          new Date(p.releasedAt) <= new Date(image.builtAt)
      );

      if (missingPatches.length === 0) return null;

      // Check urgency based on oldest unapplied patch's reconciliation window
      const oldestMissing = missingPatches.reduce((oldest, p) =>
        new Date(p.releasedAt) < new Date(oldest.releasedAt) ? p : oldest
      );
      const deadline = new Date(oldestMissing.releasedAt);
      deadline.setDate(deadline.getDate() + oldestMissing.reconciliationWindowDays);
      const now = new Date();

      let urgency: "overdue" | "due_soon" | "ok" = "ok";
      if (now > deadline) {
        urgency = "overdue";
      } else {
        const daysLeft =
          (deadline.getTime() - now.getTime()) / (1000 * 60 * 60 * 24);
        if (daysLeft <= 2) urgency = "due_soon";
      }

      return {
        imageTag: image.tag,
        missingPatches,
        urgency,
      };
    })
    .filter(Boolean) as TimeRangeReport["imagesNeedingPatches"];

  return {
    rangeStart,
    rangeEnd,
    patchesReleased,
    imagesNeedingPatches,
  };
}

/**
 * Get all patches and their status for a given image.
 */
export function getPatchesForImage(
  imageTag: string,
  patches: Patch[],
  images: ImageRecord[],
  applications: PatchApplication[]
): PatchStatus[] {
  const image = images.find((i) => i.tag === imageTag);
  if (!image) return [];

  const availableAtBuild = patches.filter(
    (p) => new Date(p.releasedAt) <= new Date(image.builtAt)
  );

  const app = applications.find((a) => a.imageTag === imageTag);
  const appliedIds = new Set(app?.patchesApplied ?? []);

  return availableAtBuild.map((patch) => ({
    patch,
    applied: appliedIds.has(patch.id),
    appliedAt: app?.appliedAt,
  }));
}

/**
 * Get a summary of patch compliance across all images.
 */
export function getComplianceSummary(
  patches: Patch[],
  images: ImageRecord[],
  applications: PatchApplication[]
): {
  total: number;
  compliant: number;
  nonCompliant: number;
  unknown: number;
} {
  let compliant = 0;
  let nonCompliant = 0;
  let unknown = 0;

  for (const image of images) {
    const report = resolveImagePatchStatus(
      image.tag,
      images,
      patches,
      applications
    );
    if (!report) {
      unknown++;
    } else if (report.status === "compliant") {
      compliant++;
    } else {
      nonCompliant++;
    }
  }

  return {
    total: images.length,
    compliant,
    nonCompliant,
    unknown,
  };
}

// --- Tag Mutation Detection ---

/**
 * Detect tag mutations between two snapshots.
 * A tag mutation is when the same tag points to a different SHA.
 */
export function detectTagMutations(
  prev: Snapshot,
  curr: Snapshot
): TagMutation[] {
  const mutations: TagMutation[] = [];

  for (const [serviceName, currDeploy] of Object.entries(curr.services)) {
    const prevDeploy = prev.services[serviceName];
    if (!prevDeploy) continue; // new service, no previous state

    if (prevDeploy.sha !== currDeploy.sha) {
      // SHA changed — either tag update or tag mutation
      // We detect this when the tag is the SAME but SHA changed (mutation)
      // vs tag changed (normal update). Both are important.
      mutations.push({
        serviceName,
        tag: currDeploy.tag,
        previousSha: prevDeploy.sha,
        currentSha: currDeploy.sha,
        detectedInSnapshot: curr.folder,
        timestamp: curr.timestamp,
      });
    }
  }

  return mutations;
}

/**
 * Detect all tag mutations across a full snapshot history.
 */
export function detectAllTagMutations(snapshots: Snapshot[]): TagMutation[] {
  const mutations: TagMutation[] = [];
  for (let i = 1; i < snapshots.length; i++) {
    mutations.push(...detectTagMutations(snapshots[i - 1], snapshots[i]));
  }
  return mutations;
}

/**
 * Check if an image SHA has been deployed to a given environment.
 */
export function getEnvironmentDeployments(
  sha: string,
  deployments: EnvironmentDeployment[]
): EnvironmentDeployment[] {
  return deployments.filter((d) => d.sha === sha);
}

/**
 * Detect testing gaps — images deployed to staging or production
 * without ever being deployed to testing first.
 */
export function detectTestingGaps(
  deployments: EnvironmentDeployment[]
): { sha: string; tag: string; environments: Environment[] }[] {
  const bySha = new Map<string, EnvironmentDeployment[]>();
  for (const d of deployments) {
    const existing = bySha.get(d.sha) ?? [];
    existing.push(d);
    bySha.set(d.sha, existing);
  }

  const gaps: { sha: string; tag: string; environments: Environment[] }[] = [];

  for (const [sha, deps] of bySha) {
    const envs = new Set(deps.map((d) => d.environment));
    if (!envs.has("testing") && (envs.has("staging") || envs.has("production"))) {
      gaps.push({
        sha,
        tag: deps[0].tag,
        environments: [...envs] as Environment[],
      });
    }
  }

  return gaps;
}

/**
 * Build a full mutation report combining patch status, environment deployments,
 * and testing gaps for a mutated tag.
 */
export function buildMutationReport(
  mutation: TagMutation,
  images: ImageRecord[],
  patches: Patch[],
  applications: PatchApplication[],
  environmentDeployments: EnvironmentDeployment[]
): TagMutationReport {
  // Find the image record for the current SHA
  const currentImage = images.find(
    (i) => i.sha === mutation.currentSha
  );

  // Get patch status for the current image
  let patchStatus: ImagePatchReport | null = null;
  if (currentImage) {
    patchStatus = resolveImagePatchStatus(
      currentImage.tag,
      images,
      patches,
      applications
    );
  }

  // Get environment deployments for the current SHA
  const envDeployments = getEnvironmentDeployments(
    mutation.currentSha,
    environmentDeployments
  );

  // Check for testing gap
  const hasTestingGap =
    envDeployments.length > 0 &&
    !envDeployments.some((d) => d.environment === "testing");

  // Build summary
  const missingPatches = patchStatus?.patchesMissing ?? 0;
  const envList = envDeployments.map((d) => d.environment).join(", ") || "none";

  let summary: string;
  if (hasTestingGap && missingPatches > 0) {
    summary = `HIGH RISK: ${mutation.serviceName} deployed to [${envList}] without testing and missing ${missingPatches} patches`;
  } else if (hasTestingGap) {
    summary = `MEDIUM RISK: ${mutation.serviceName} deployed to [${envList}] without testing (patches OK)`;
  } else if (missingPatches > 0) {
    summary = `MEDIUM RISK: ${mutation.serviceName} missing ${missingPatches} patches (testing OK)`;
  } else {
    summary = `LOW RISK: ${mutation.serviceName} properly tested and patched`;
  }

  return {
    mutation,
    patchStatus,
    environmentDeployments: envDeployments,
    testingGap: hasTestingGap,
    summary,
  };
}
