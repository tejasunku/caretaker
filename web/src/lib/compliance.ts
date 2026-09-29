/**
 * Compliance timing model.
 *
 * Defines the windows required for testing, staging, and the threshold
 * that separates "barely" from "plenty of time."
 *
 * The reconciliation window is per-patch (typically 7 days).
 * Within that window, we need:
 *   1. Time to deploy to testing and validate
 *   2. Time to deploy to staging and validate
 *   3. Buffer before the deadline
 *
 * Status logic (from worst to best):
 *   inactive     — service no longer in latest snapshot
 *   new          — service introduced within current reconciliation window
 *   non_compliant — current prod version is past deadline, no viable patched version
 *   at_risk_next_window — compliant now, but won't be for next window
 *   at_risk_staging — enough time for testing, not enough for staging
 *   barely       — barely enough time for both testing and staging
 *   compliant    — plenty of time, on track
 */

export interface ComplianceConfig {
  reconciliationWindowDays: number;
  testingWindowDays: number;
  stagingWindowDays: number;
  barelyThresholdDays: number;  // below this = "barely", above = "plenty"
}

export const DEFAULT_CONFIG: ComplianceConfig = {
  reconciliationWindowDays: 7,
  testingWindowDays: 2,
  stagingWindowDays: 2,
  barelyThresholdDays: 1,  // less than 1 day buffer = barely
};

export type ComplianceStatus =
  | "inactive"
  | "new"
  | "non_compliant"
  | "at_risk_next_window"
  | "at_risk_staging"
  | "barely"
  | "compliant";

export interface ComplianceResult {
  status: ComplianceStatus;
  /** Days until the oldest unapplied patch's reconciliation deadline */
  daysUntilDeadline: number | null;
  /** Days of buffer after testing + staging are accounted for */
  bufferDays: number | null;
  /** Whether there's a viable patched version ready to deploy */
  hasViablePatchedVersion: boolean;
  /** Number of patches missing */
  missingPatchCount: number;
  /** The reconciliation window of the most urgent patch */
  urgentPatchWindowDays: number | null;
  /** Human-readable explanation */
  explanation: string;
}

/**
 * Compute the compliance status for a service.
 */
export function computeCompliance(
  serviceTag: string,
  missingPatches: { id: string; reconciliationWindowDays: number; releasedAt: string }[],
  hasViablePatchedVersion: boolean,
  isInactive: boolean,
  isNew: boolean,
  config: ComplianceConfig = DEFAULT_CONFIG
): ComplianceResult {
  if (isInactive) {
    return {
      status: "inactive",
      daysUntilDeadline: null,
      bufferDays: null,
      hasViablePatchedVersion: false,
      missingPatchCount: 0,
      urgentPatchWindowDays: null,
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
      urgentPatchWindowDays: null,
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
      urgentPatchWindowDays: null,
      explanation: "All patches applied",
    };
  }

  // Find the most urgent patch (smallest reconciliation window, or nearest deadline)
  const now = new Date();
  const urgentPatch = missingPatches.reduce((mostUrgent, p) => {
    const deadline = new Date(p.releasedAt);
    deadline.setDate(deadline.getDate() + p.reconciliationWindowDays);
    const mostUrgentDeadline = new Date(mostUrgent.releasedAt);
    mostUrgentDeadline.setDate(mostUrgentDeadline.getDate() + mostUrgent.reconciliationWindowDays);
    return deadline < mostUrgentDeadline ? p : mostUrgent;
  });

  const deadline = new Date(urgentPatch.releasedAt);
  deadline.setDate(deadline.getDate() + urgentPatch.reconciliationWindowDays);
  const daysUntilDeadline = Math.ceil(
    (deadline.getTime() - now.getTime()) / (1000 * 60 * 60 * 24)
  );

  const totalRequired = config.testingWindowDays + config.stagingWindowDays;
  const bufferDays = daysUntilDeadline - totalRequired;

  if (daysUntilDeadline <= 0) {
    // Past deadline
    if (!hasViablePatchedVersion) {
      return {
        status: "non_compliant",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: false,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Past deadline by ${Math.abs(daysUntilDeadline)} days, no patched version available`,
      };
    } else {
      // Has a patched version but it's past deadline — still non-compliant
      return {
        status: "non_compliant",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: true,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Past deadline by ${Math.abs(daysUntilDeadline)} days, patched version exists but not deployed`,
      };
    }
  }

  if (!hasViablePatchedVersion) {
    // No patched version exists yet — check if there's time to create one + test + stage
    if (bufferDays <= 0) {
      return {
        status: "at_risk_next_window",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: false,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Only ${daysUntilDeadline} days left — not enough time to build, test, and stage`,
      };
    } else if (daysUntilDeadline <= config.testingWindowDays) {
      return {
        status: "at_risk_staging",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: false,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Only ${daysUntilDeadline} days left — enough for testing but not staging`,
      };
    } else if (bufferDays <= config.barelyThresholdDays) {
      return {
        status: "barely",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: false,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `${daysUntilDeadline} days left — barely enough time for testing and staging`,
      };
    } else {
      return {
        status: "compliant",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: false,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `${daysUntilDeadline} days left — plenty of time`,
      };
    }
  } else {
    // Has a patched version — check if there's time to promote it
    if (daysUntilDeadline <= config.testingWindowDays) {
      return {
        status: "at_risk_staging",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: true,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Patched version exists but only ${daysUntilDeadline} days left — not enough for staging`,
      };
    } else if (bufferDays <= config.barelyThresholdDays) {
      return {
        status: "barely",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: true,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Patched version exists — ${daysUntilDeadline} days left, barely enough`,
      };
    } else {
      return {
        status: "compliant",
        daysUntilDeadline,
        bufferDays,
        hasViablePatchedVersion: true,
        missingPatchCount: missingPatches.length,
        urgentPatchWindowDays: urgentPatch.reconciliationWindowDays,
        explanation: `Patched version exists — ${daysUntilDeadline} days left, on track`,
      };
    }
  }
}

/**
 * Get the color/class for a compliance status.
 */
export function statusColor(status: ComplianceStatus): string {
  switch (status) {
    case "inactive": return "#6b7280";      // gray
    case "new": return "#3b82f6";           // blue
    case "non_compliant": return "#ef4444"; // red
    case "at_risk_next_window": return "#f97316"; // orange
    case "at_risk_staging": return "#eab308";     // yellow
    case "barely": return "#facc15";              // yellow-ish
    case "compliant": return "#22c55e";           // green
  }
}

/**
 * Get a human-readable label for a compliance status.
 */
export function statusLabel(status: ComplianceStatus): string {
  switch (status) {
    case "inactive": return "Inactive";
    case "new": return "New";
    case "non_compliant": return "Non-Compliant";
    case "at_risk_next_window": return "At Risk (Next Window)";
    case "at_risk_staging": return "At Risk (Staging)";
    case "barely": return "Barely Compliant";
    case "compliant": return "Compliant";
  }
}
