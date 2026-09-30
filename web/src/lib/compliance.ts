/**
 * Patching compliance model.
 *
 * Tracks whether a service's current image has all required CVE patches
 * applied within their reconciliation windows. Purely about patching —
 * does not consider testing/staging procedures (see stability.ts).
 *
 * Status logic (from worst to best):
 *   inactive     — service no longer in latest snapshot
 *   new          — service introduced within current reconciliation window
 *   non_compliant — past deadline, patches missing
 *   at_risk      — not past deadline, but insufficient time to patch
 *   barely       — barely enough time to patch
 *   compliant    — all patches applied or plenty of time
 */

export interface PatchingConfig {
  /** Per-patch window to apply the patch (typically 7 days) */
  reconciliationWindowDays: number;
  /** Below this many days of buffer = "barely" */
  barelyThresholdDays: number;
}

export const DEFAULT_PATCHING_CONFIG: PatchingConfig = {
  reconciliationWindowDays: 7,
  barelyThresholdDays: 1,
};

export type PatchingStatus =
  | "inactive"
  | "new"
  | "non_compliant"
  | "at_risk"
  | "barely"
  | "compliant";

export interface PatchingResult {
  status: PatchingStatus;
  /** Days until the oldest unapplied patch's reconciliation deadline */
  daysUntilDeadline: number | null;
  /** Days of buffer after accounting for the patch itself */
  bufferDays: number | null;
  /** Whether there's a viable patched version ready to deploy */
  hasViablePatchedVersion: boolean;
  /** Number of patches missing */
  missingPatchCount: number;
  /** Human-readable explanation */
  explanation: string;
}

/**
 * Compute the patching compliance status for a service.
 */
export function computePatchingStatus(
  missingPatches: { id: string; reconciliationWindowDays: number; releasedAt: string }[],
  hasViablePatchedVersion: boolean,
  isInactive: boolean,
  isNew: boolean,
  config: PatchingConfig = DEFAULT_PATCHING_CONFIG,
  referenceDate?: Date
): PatchingResult {
  if (isInactive) {
    return {
      status: "inactive",
      daysUntilDeadline: null,
      bufferDays: null,
      hasViablePatchedVersion: false,
      missingPatchCount: 0,
      explanation: "Service is no longer deployed",
    };
  }

  if (isNew) {
    return {
      status: "new",
      daysUntilDeadline: null,
      bufferDays: null,
      hasViablePatchedVersion: false,
      missingPatchCount: missingPatches.length,
      explanation: "Service introduced within current reconciliation window",
    };
  }

  if (missingPatches.length === 0) {
    return {
      status: "compliant",
      daysUntilDeadline: null,
      bufferDays: null,
      hasViablePatchedVersion: true,
      missingPatchCount: 0,
      explanation: "All patches applied",
    };
  }

  // Find the most urgent patch (nearest deadline)
  const now = referenceDate ?? new Date();
  const urgentPatch = missingPatches.reduce((mostUrgent, p) => {
    const deadline = new Date(p.releasedAt);
    deadline.setDate(deadline.getDate() + p.reconciliationWindowDays);
    const mostUrgentDeadline = new Date(mostUrgent.releasedAt);
    mostUrgentDeadline.setDate(mostUrgentDeadline.getDate() + mostUrgent.reconciliationWindowDays);
    return deadline < mostUrgentDeadline ? p : mostUrgent;
  });

  const deadline = new Date(urgentPatch.releasedAt);
  deadline.setDate(deadline.getDate() + urgentPatch.reconciliationWindowDays);
  const daysUntilDeadline = Math.round(
    (deadline.getTime() - now.getTime()) / (1000 * 60 * 60 * 24)
  );

  const bufferDays = daysUntilDeadline;

  if (daysUntilDeadline <= 0) {
    return {
      status: "non_compliant",
      daysUntilDeadline,
      bufferDays,
      hasViablePatchedVersion,
      missingPatchCount: missingPatches.length,
      explanation: hasViablePatchedVersion
        ? `Past deadline by ${Math.abs(daysUntilDeadline)} days — patched version exists but not deployed`
        : `Past deadline by ${Math.abs(daysUntilDeadline)} days — no patched version available`,
    };
  }

  if (bufferDays <= config.barelyThresholdDays) {
    return {
      status: "barely",
      daysUntilDeadline,
      bufferDays,
      hasViablePatchedVersion,
      missingPatchCount: missingPatches.length,
      explanation: `${daysUntilDeadline} days left — barely enough time`,
    };
  }

  if (!hasViablePatchedVersion && daysUntilDeadline <= config.reconciliationWindowDays / 2) {
    return {
      status: "at_risk",
      daysUntilDeadline,
      bufferDays,
      hasViablePatchedVersion: false,
      missingPatchCount: missingPatches.length,
      explanation: `${daysUntilDeadline} days left — no patched version, insufficient time to build and deploy`,
    };
  }

  return {
    status: "compliant",
    daysUntilDeadline,
    bufferDays,
    hasViablePatchedVersion,
    missingPatchCount: missingPatches.length,
    explanation: hasViablePatchedVersion
      ? `${daysUntilDeadline} days left — patched version exists, on track`
      : `${daysUntilDeadline} days left — plenty of time`,
  };
}

/**
 * Get the color for a patching status.
 */
export function patchingStatusColor(status: PatchingStatus): string {
  switch (status) {
    case "inactive": return "#6b7280";
    case "new": return "#3b82f6";
    case "non_compliant": return "#ef4444";
    case "at_risk": return "#f97316";
    case "barely": return "#eab308";
    case "compliant": return "#22c55e";
  }
}

/**
 * Get a human-readable label for a patching status.
 */
export function patchingStatusLabel(status: PatchingStatus): string {
  switch (status) {
    case "inactive": return "Inactive";
    case "new": return "New";
    case "non_compliant": return "Non-Compliant";
    case "at_risk": return "At Risk";
    case "barely": return "Barely Compliant";
    case "compliant": return "Compliant";
  }
}
