/**
 * Stability model — testing procedure tracking.
 *
 * Tracks whether an image spent sufficient time in each environment
 * (testing, staging) before being promoted. Purely about the testing
 * pipeline — does not consider patching status (see compliance.ts).
 *
 * For each image deployment, we check:
 *   - Did it spend at least `testingWindowDays` in testing?
 *   - Did it spend at least `stagingWindowDays` in staging?
 *
 * Border logic for pipeline visualization:
 *   - Solid border: spent the required time in that environment
 *   - Dashed border: didn't meet the required time (or still in progress)
 */

export interface StabilityConfig {
  /** Required days in testing before promotion to staging */
  testingWindowDays: number;
  /** Required days in staging before promotion to production */
  stagingWindowDays: number;
}

export const DEFAULT_STABILITY_CONFIG: StabilityConfig = {
  testingWindowDays: 2,
  stagingWindowDays: 2,
};

export type EnvironmentStatus = "promoted" | "current" | "rolled_back";

export interface EnvironmentHistoryEntry {
  environment: "testing" | "staging" | "production";
  tag: string;
  sha: string;
  deployedAt: string;
  status: EnvironmentStatus;
  /** Duration in this environment in milliseconds (null if still active) */
  durationMs: number | null;
  /** Whether this entry met the required time window */
  metRequiredTime: boolean;
}

export interface StabilityReport {
  /** Per-environment history for this image */
  environments: EnvironmentHistoryEntry[];
  /** Whether testing met its window */
  testingStable: boolean;
  /** Whether staging met its window */
  stagingStable: boolean;
  /** Overall stability (both testing and staging met requirements) */
  isStable: boolean;
}

/**
 * Required time in milliseconds for each environment.
 */
function requiredMs(windowDays: number): number {
  return windowDays * 24 * 60 * 60 * 1000;
}

/**
 * Compute stability for a specific image SHA across environments.
 */
export function computeStability(
  sha: string,
  deployments: { environment: string; tag: string; sha: string; deployedAt: string; status: EnvironmentStatus }[],
  config: StabilityConfig = DEFAULT_STABILITY_CONFIG,
  referenceDate?: Date
): StabilityReport {
  const now = referenceDate ?? new Date();
  const relevant = deployments
    .filter((d) => d.sha === sha)
    .sort((a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime());

  const environments: EnvironmentHistoryEntry[] = [];
  let testingMet = false;
  let stagingMet = false;

  for (const dep of relevant) {
    const startTime = new Date(dep.deployedAt).getTime();
    const endTime = dep.status === "current" ? now.getTime() : startTime + (dep.durationMs ?? 0);
    const durationMs = endTime - startTime;
    const required = requiredMs(
      dep.environment === "testing" ? config.testingWindowDays : config.stagingWindowDays
    );
    const metRequiredTime = durationMs >= required;

    if (dep.environment === "testing") testingMet = metRequiredTime;
    if (dep.environment === "staging") stagingMet = metRequiredTime;

    environments.push({
      environment: dep.environment as "testing" | "staging" | "production",
      tag: dep.tag,
      sha: dep.sha,
      deployedAt: dep.deployedAt,
      status: dep.status,
      durationMs,
      metRequiredTime,
    });
  }

  return {
    environments,
    testingStable: testingMet,
    stagingStable: stagingMet,
    isStable: testingMet && stagingMet,
  };
}

/**
 * Get border style for pipeline visualization.
 */
export function stabilityBorderStyle(metRequired: boolean): string {
  return metRequired ? "border-solid border-2" : "border-dashed border-2";
}

/**
 * Get color class for stability status.
 */
export function stabilityColor(isStable: boolean): string {
  return isStable ? "#22c55e" : "#f97316";
}

/**
 * Get human-readable stability label.
 */
export function stabilityLabel(isStable: boolean): string {
  return isStable ? "Stable" : "Unstable";
}
