import { CLUSTER_DATA } from "~/lib/cluster-data";

export interface PatchInfo {
  id: string;
  package: string;
  installedVersion: string;
  fixedVersion: string;
  severity: string;
  releasedAt: string;
}

export interface ClusterRow {
  id: string;
  stage: string;
  provider: string;
}

export interface DeploymentRow {
  cluster: string;
  stage: string;
  service: string;
  repository: string;
  tag: string;
  digest: string;
  replicas: number;
  latestPatchedTag: string | null;
  stale: boolean;
}

export interface EventRow {
  cluster: string;
  stage: string;
  provider: string;
  at: string;
  action: string;
  service: string;
  fromTag: string;
  fromDigest: string;
  toTag: string;
  toDigest: string;
}

interface RawData {
  generatedAt: string;
  clusters: ClusterRow[];
  deployments: DeploymentRow[];
  events: EventRow[];
  patches: PatchInfo[];
}

const DATA = CLUSTER_DATA as unknown as RawData;

export const REFERENCE_DATE = DATA.generatedAt;
export const PATCHES: PatchInfo[] = DATA.patches;

export function severityCounts(patches: PatchInfo[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const p of patches) counts[p.severity] = (counts[p.severity] || 0) + 1;
  return counts;
}

/** Patches an image is missing. Stale images are missing the whole mock set. */
export function missingPatchesFor(d: DeploymentRow): PatchInfo[] {
  return d.stale ? PATCHES : [];
}

export interface ClusterSummary extends ClusterRow {
  services: number;
  stale: number;
  critical: number;
}

export async function getOverview() {
  const criticalPerImage = (severityCounts(PATCHES)["critical"] || 0);

  const clusters: ClusterSummary[] = DATA.clusters.map((c) => {
    const deps = DATA.deployments.filter((d) => d.cluster === c.id);
    const stale = deps.filter((d) => d.stale);
    return {
      ...c,
      services: deps.length,
      stale: stale.length,
      critical: stale.length * criticalPerImage,
    };
  });

  const stats = {
    totalClusters: clusters.length,
    totalDeployments: DATA.deployments.length,
    staleDeployments: DATA.deployments.filter((d) => d.stale).length,
    criticalMissing: DATA.deployments.filter((d) => d.stale).length * criticalPerImage,
  };

  return {
    clusters,
    stats,
    patches: PATCHES,
    generatedAt: DATA.generatedAt,
  };
}

export async function getClusterDetail(id: string) {
  const cluster = DATA.clusters.find((c) => c.id === id);
  if (!cluster) return null;

  const deployments = DATA.deployments
    .filter((d) => d.cluster === id)
    .sort((a, b) => a.service.localeCompare(b.service));
  const events = DATA.events.filter((e) => e.cluster === id);

  return { cluster, deployments, events };
}

export interface ClusterHistory extends ClusterRow {
  events: EventRow[];
}

export async function getHistory() {
  const byCluster = new Map<string, EventRow[]>();
  for (const e of DATA.events) {
    if (!byCluster.has(e.cluster)) byCluster.set(e.cluster, []);
    byCluster.get(e.cluster)!.push(e);
  }
  const clusters: ClusterHistory[] = DATA.clusters.map((c) => {
    const events = (byCluster.get(c.id) ?? []).sort((a, b) => b.at.localeCompare(a.at));
    return { ...c, events };
  });
  return { clusters, generatedAt: DATA.generatedAt, totalEvents: DATA.events.length };
}

export async function getServiceAcrossClusters(name: string) {
  const deployments = DATA.deployments
    .filter((d) => d.service === name)
    .sort((a, b) => a.cluster.localeCompare(b.cluster));
  if (deployments.length === 0) return null;

  return {
    name,
    deployments,
    patches: PATCHES.filter((p) => deployments.some((d) => d.stale)),
  };
}

export interface PatchTargetService {
  name: string;
  staleClusters: number;
  totalClusters: number;
}

export interface PatchTargetCluster extends ClusterRow {
  services: number;
  stale: number;
}

export interface PatchTargets {
  services: PatchTargetService[];
  clusters: PatchTargetCluster[];
  generatedAt: string;
}

export async function getPatchTargets(): Promise<PatchTargets> {
  const names = [...new Set(DATA.deployments.map((d) => d.service))].sort();
  const services: PatchTargetService[] = names.map((name) => {
    const deps = DATA.deployments.filter((d) => d.service === name);
    return {
      name,
      staleClusters: deps.filter((d) => d.stale).length,
      totalClusters: deps.length,
    };
  });

  const clusters: PatchTargetCluster[] = DATA.clusters.map((c) => {
    const deps = DATA.deployments.filter((d) => d.cluster === c.id);
    return { ...c, services: deps.length, stale: deps.filter((d) => d.stale).length };
  });

  return {
    services,
    clusters,
    generatedAt: DATA.generatedAt,
  };
}
