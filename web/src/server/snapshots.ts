import fs from "node:fs";
import path from "node:path";

const SNAPSHOTS_DIR = path.resolve(
  process.cwd(),
  "..",
  "kubernetes-snapshots"
);

export interface SnapshotData {
  folder: string;
  timestamp: string;
  services: Record<string, {
    tag: string;
    sha: string;
    replicaCount: number;
    resourceTier: string;
  }>;
  registry: Record<string, {
    digest: string;
    pushed_at: string;
  }>;
}

function folderToTimestamp(folder: string): string {
  return folder + "Z";
}

export function listSnapshots(): string[] {
  if (!fs.existsSync(SNAPSHOTS_DIR)) return [];
  return fs.readdirSync(SNAPSHOTS_DIR)
    .filter((d) => {
      const full = path.join(SNAPSHOTS_DIR, d);
      return fs.statSync(full).isDirectory() && d.length === 19 && d[4] === "-";
    })
    .sort();
}

export function loadSnapshot(folder: string): SnapshotData {
  const snapshotPath = path.join(SNAPSHOTS_DIR, folder);
  const servicesPath = path.join(snapshotPath, "services.json");
  const registryPath = path.join(snapshotPath, "registry.json");

  const services = fs.existsSync(servicesPath)
    ? JSON.parse(fs.readFileSync(servicesPath, "utf-8"))
    : {};
  const registry = fs.existsSync(registryPath)
    ? JSON.parse(fs.readFileSync(registryPath, "utf-8"))
    : {};

  return {
    folder,
    timestamp: folderToTimestamp(folder),
    services,
    registry,
  };
}

export function loadAllSnapshots(): SnapshotData[] {
  return listSnapshots().map(loadSnapshot);
}

export interface SnapshotDiff {
  added: Record<string, any>;
  removed: Record<string, any>;
  changed: Record<string, { prev: any; curr: any; diffType: string }>;
  unchanged: Record<string, any>;
}

export function diffSnapshots(prev: SnapshotData, curr: SnapshotData): SnapshotDiff {
  const added: Record<string, any> = {};
  const removed: Record<string, any> = {};
  const changed: Record<string, { prev: any; curr: any; diffType: string }> = {};
  const unchanged: Record<string, any> = {};

  for (const [name, currData] of Object.entries(curr.services)) {
    if (!(name in prev.services)) {
      added[name] = currData;
    } else {
      const prevData = prev.services[name];
      if (
        prevData.tag !== currData.tag ||
        prevData.sha !== currData.sha ||
        prevData.replicaCount !== currData.replicaCount ||
        prevData.resourceTier !== currData.resourceTier
      ) {
        const diffType: string[] = [];
        if (prevData.tag !== currData.tag) diffType.push("tag_update");
        if (prevData.sha !== currData.sha) diffType.push("sha_change");
        if (prevData.replicaCount !== currData.replicaCount) diffType.push("replica_change");
        if (prevData.resourceTier !== currData.resourceTier) diffType.push("resource_tier_change");
        changed[name] = { prev: prevData, curr: currData, diffType: diffType.join("+") };
      } else {
        unchanged[name] = currData;
      }
    }
  }

  for (const [name, prevData] of Object.entries(prev.services)) {
    if (!(name in curr.services)) {
      removed[name] = prevData;
    }
  }

  return { added, removed, changed, unchanged };
}
