import type {
  Patch,
  ImageRecord,
  PatchApplication,
  PatchStatus,
  ImagePatchReport,
  TimeRangeReport,
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
