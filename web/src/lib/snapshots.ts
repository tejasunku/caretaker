// Pure snapshot diff logic — no Node.js dependencies

export interface SnapshotDiff {
  added: Record<string, any>;
  removed: Record<string, any>;
  changed: Record<string, { prev: any; curr: any; diffType: string }>;
  unchanged: Record<string, any>;
}

export function diffSnapshots(prev: { services: Record<string, any> }, curr: { services: Record<string, any> }): SnapshotDiff {
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
