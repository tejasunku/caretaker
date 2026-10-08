import { CURRENT_STATE, GIT_HISTORY, SERVICE_HISTORIES, MOCK_PATCHES } from "~/lib/gitops-data";
import type { Patch } from "~/lib/types";

// Reference date: use the latest git commit date or current date
export const REFERENCE_DATE = GIT_HISTORY.length > 0
  ? new Date(GIT_HISTORY[0].date)
  : new Date();

export interface ServiceEnvState {
  tag: string;
  digest: string;
  replicaCount: string;
}

export interface ServiceState {
  repository: string;
  tag: string;
  digest: string;
  infra: Record<string, string>;
  environments: Record<string, ServiceEnvState>;
}

export interface GitCommit {
  sha: string;
  date: string;
  message: string;
}

export interface ServiceOverview {
  name: string;
  repository: string;
  tag: string;
  digest: string;
  isStale: boolean;
  missingPatches: string[];
  missingSeverity: Record<string, number>;
  environments: Record<string, ServiceEnvState>;
  lastChanged: GitCommit | null;
}

export interface ServiceDetail {
  name: string;
  state: ServiceState;
  history: GitCommit[];
  allDigests: { digest: string; tag: string; environments: string[] }[];
}

/**
 * Get patches that are missing from an image (released after image was built).
 * For gitops model, we check based on when the digest was last updated.
 */
function getMissingPatches(lastUpdated: string): Patch[] {
  const updated = new Date(lastUpdated);
  return MOCK_PATCHES.filter((p) => new Date(p.releasedAt) > updated);
}

export async function getServiceOverview() {
  const services: Record<string, ServiceOverview> = {};

  for (const [name, state] of Object.entries(CURRENT_STATE)) {
    const serviceHistory = SERVICE_HISTORIES[name as keyof typeof SERVICE_HISTORIES] || [];
    const lastChanged = serviceHistory.length > 0 ? serviceHistory[0] : null;

    // For now, use git commit date as proxy for when image was last updated
    const lastUpdated = lastChanged?.date ?? REFERENCE_DATE.toISOString();
    const missing = getMissingPatches(lastUpdated);
    const missingSeverity: Record<string, number> = {};
    for (const p of missing) {
      missingSeverity[p.severity] = (missingSeverity[p.severity] || 0) + 1;
    }

    services[name] = {
      name,
      repository: state.repository,
      tag: state.tag,
      digest: state.digest,
      isStale: missing.length > 0,
      missingPatches: missing.map((p) => p.id),
      missingSeverity,
      environments: state.environments as Record<string, ServiceEnvState>,
      lastChanged,
    };
  }

  // Sort: stale first, then by name
  const sorted = Object.entries(services).sort(([, a], [, b]) => {
    if (a.isStale !== b.isStale) return a.isStale ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  const servicesMap = Object.fromEntries(sorted);

  return {
    services: servicesMap,
    gitHistory: GIT_HISTORY,
    referenceDate: REFERENCE_DATE.toISOString(),
    stats: {
      total: sorted.length,
      stale: sorted.filter(([, s]) => s.isStale).length,
      totalMissingPatches: sorted.reduce((acc, [, s]) => acc + s.missingPatches.length, 0),
      criticalMissing: sorted.reduce(
        (acc, [, s]) => acc + (s.missingSeverity["critical"] || 0),
        0
      ),
    },
  };
}

export async function getServiceDetail(name: string): Promise<ServiceDetail | null> {
  const state = CURRENT_STATE[name as keyof typeof CURRENT_STATE];
  if (!state) return null;

  const history = SERVICE_HISTORIES[name as keyof typeof SERVICE_HISTORIES] || [];

  // Collect all unique digests across environments
  const digestMap = new Map<string, { digest: string; tag: string; environments: string[] }>();
  
  // Add base digest if present
  if (state.digest) {
    digestMap.set(state.digest, {
      digest: state.digest,
      tag: state.tag,
      environments: [],
    });
  }

  // Add environment-specific digests
  for (const [env, envState] of Object.entries(state.environments)) {
    if (envState.digest) {
      const existing = digestMap.get(envState.digest);
      if (existing) {
        existing.environments.push(env);
      } else {
        digestMap.set(envState.digest, {
          digest: envState.digest,
          tag: envState.tag,
          environments: [env],
        });
      }
    }
  }

  return {
    name,
    state: state as ServiceState,
    history,
    allDigests: Array.from(digestMap.values()),
  };
}
