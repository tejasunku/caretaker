import { For, Show, Suspense, createMemo } from "solid-js";
import { useParams, A, createAsync } from "@solidjs/router";
import { getServiceDetail } from "../data";

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("sv-SE", { timeZone: "UTC" }).replace(" ", "T") + "Z";
}

function formatDuration(startIso: string, endIso: string): string {
  const ms = new Date(endIso).getTime() - new Date(startIso).getTime();
  const hours = Math.floor(ms / (1000 * 60 * 60));
  const days = Math.floor(hours / 24);
  const remainHours = hours % 24;
  if (days > 0) return `${days}d ${remainHours}h`;
  return `${hours}h`;
}

function shortDigest(digest: string): string {
  return digest.slice(0, 13) + "…";
}

interface DeploymentRecord {
  tag: string;
  digest: string;
  deployedAt: string;
  status: string;
  endedAt: string | null;
  duration: string | null;
  isCurrent: boolean;
}

export default function ServiceDetail() {
  const params = useParams();
  const data = createAsync(() => getServiceDetail(params.name ?? ""));

  const timeline = createMemo(() => {
    const d = data();
    if (!d) return null;

    const envs = ["testing", "staging", "production"] as const;
    const now = new Date().toISOString();

    const result: Record<string, DeploymentRecord[]> = {};

    for (const env of envs) {
      const deploys = d.deployments
        .filter((dep) => dep.environment === env && dep.tag.startsWith(d.name + ":"))
        .sort((a, b) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime());

      const records: DeploymentRecord[] = deploys.map((dep, i) => {
        const nextDeploy = deploys[i + 1];
        const isLast = i === deploys.length - 1;
        const endedAt = isLast ? null : nextDeploy.deployedAt;
        const duration = endedAt
          ? formatDuration(dep.deployedAt, endedAt)
          : formatDuration(dep.deployedAt, now);

        return {
          tag: dep.tag,
          digest: dep.digest,
          deployedAt: dep.deployedAt,
          status: dep.status,
          endedAt,
          duration,
          isCurrent: isLast,
        };
      });

      result[env] = records;
    }

    return result;
  });

  return (
    <>
      <A href="/" class="text-text-dim text-xs no-underline">
        ← Back to overview
      </A>

      <Suspense fallback={<p>Loading...</p>}>
        <Show when={data()}>
          <h1 class="text-2xl font-semibold mt-3 mb-1">{data()!.name}</h1>
          <p class="text-text-dim text-sm mb-6">
            Is the production image stale? Does a patched version exist and where is it in the pipeline?
          </p>

          {/* Feature Line Status */}
          <Show when={data()!.featureLine}>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 mb-6">
              <div class="flex items-center gap-3 mb-4">
                <span
                  class="inline-block w-3 h-3 rounded-full"
                  style={{ "background-color": data()!.featureLine!.isStale ? "var(--yellow)" : "var(--green)" }}
                />
                <span class="text-lg font-semibold">
                  {data()!.featureLine!.isStale ? "Needs Patching" : "Up to Date"}
                </span>
                <span class="text-text-dim text-sm font-mono">{data()!.featureLine!.tag}</span>
              </div>

              <Show when={data()!.featureLine!.isStale}>
                <div class="text-sm text-text-dim mb-4">
                  Production is missing {data()!.featureLine!.missingPatches.length} patch{data()!.featureLine!.missingPatches.length > 1 ? "es" : ""}:{" "}
                  <span class="font-mono">{data()!.featureLine!.missingPatches.join(", ")}</span>
                </div>
              </Show>

              <div class="grid grid-cols-4 gap-4 text-sm">
                <div>
                  <div class="text-text-dim text-xs">Production Digest</div>
                  <div class="font-mono text-xs mt-1">{shortDigest(data()!.featureLine!.originatorDigest)}</div>
                  <div class="text-xs mt-1" style={{ color: data()!.featureLine!.isStale ? "var(--yellow)" : "var(--green)" }}>
                    {data()!.featureLine!.isStale ? "stale" : "current"}
                  </div>
                </div>
                <div>
                  <div class="text-text-dim text-xs">Patched Version Exists</div>
                  <div class="text-xs mt-1">
                    {data()!.featureLine!.patchedExists ? (
                      <span class="text-green">✓ yes</span>
                    ) : (
                      <span class="text-text-dim">— no</span>
                    )}
                  </div>
                  <Show when={data()!.featureLine!.latestPatchedDigest}>
                    <div class="font-mono text-xs mt-1">{shortDigest(data()!.featureLine!.latestPatchedDigest!)}</div>
                  </Show>
                </div>
                <div>
                  <div class="text-text-dim text-xs">In Testing</div>
                  <div class="text-xs mt-1">
                    {data()!.featureLine!.patchedInTesting ? (
                      <span class="text-green">✓ yes</span>
                    ) : (
                      <span class="text-text-dim">— no</span>
                    )}
                  </div>
                </div>
                <div>
                  <div class="text-text-dim text-xs">In Production</div>
                  <div class="text-xs mt-1">
                    {data()!.featureLine!.patchedInProd ? (
                      <span class="text-green">✓ yes</span>
                    ) : (
                      <span class="text-text-dim">— no</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </Show>

          {/* Testing Gap Alerts */}
          <Show when={data()!.testingGaps.length > 0}>
            <div class="bg-red/5 border border-red/20 rounded-lg p-4 mb-6">
              <div class="text-sm font-semibold text-red mb-2">⚠ Testing Gaps Detected</div>
              <For each={data()!.testingGaps}>
                {(gap) => (
                  <div class="text-xs text-text-dim mt-1">
                    <span class="font-mono">{gap.tag}</span> ({shortDigest(gap.digest)}) was deployed to{" "}
                    <span class="font-medium text-red">{gap.environments.join(", ")}</span> without passing through testing first.
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* Tag Mutation Alerts */}
          <Show when={data()!.tagMutations.length > 0}>
            <div class="bg-yellow/5 border border-yellow/20 rounded-lg p-4 mb-6">
              <div class="text-sm font-semibold mb-2" style={{ color: "var(--yellow)" }}>⚠ Tag Mutations Detected</div>
              <For each={data()!.tagMutations}>
                {(mut) => (
                  <div class="text-xs text-text-dim mt-1">
                    Tag <span class="font-mono">{mut.oldTag}</span> had its digest change from{" "}
                    <span class="font-mono">{shortDigest(mut.oldDigest)}</span> to{" "}
                    <span class="font-mono">{shortDigest(mut.newDigest)}</span> in snapshot {mut.snapshot}.
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* Deployment Timeline */}
          <Show when={timeline()}>
            <h2 class="text-lg font-semibold mb-3">Deployment Timeline</h2>
            <p class="text-text-dim text-xs mb-4">
              Which digests are in each environment. The team handles promotion — we just track where things are.
            </p>
            <div class="flex flex-col gap-4 mb-6">
              <For each={["production", "staging", "testing"]}>
                {(env) => (
                  <div class="bg-card-bg border border-card-border rounded-lg p-4">
                    <div class="text-xs uppercase tracking-wide text-text-dim mb-3 font-semibold">
                      {env}
                    </div>
                    <Show when={timeline()![env].length > 0} fallback={<div class="text-text-dim text-sm">No deployments</div>}>
                      <div class="flex flex-col gap-2">
                        <For each={timeline()![env]}>
                          {(record) => (
                            <div
                              class="flex items-center gap-3 p-3 rounded-md text-sm"
                              classList={{
                                "bg-green/5 border border-green/20": record.isCurrent && record.status === "promoted",
                                "bg-yellow/5 border border-yellow/20": record.isCurrent && record.status === "deployed",
                                "bg-red/5 border border-red/20": record.status === "rolled_back",
                                "bg-gray/5 border border-gray/20": !record.isCurrent && record.status !== "rolled_back",
                              }}
                            >
                              <div class="flex-1">
                                <div class="font-mono">{record.tag}</div>
                                <div class="text-xs text-text-dim mt-0.5 font-mono">{shortDigest(record.digest)}</div>
                                <div class="text-xs text-text-dim mt-0.5">
                                  {formatDateTime(record.deployedAt)}
                                  {record.endedAt && ` → ${formatDateTime(record.endedAt)}`}
                                  {!record.endedAt && ` → now`}
                                </div>
                              </div>
                              <div class="text-right">
                                <div class="font-medium">{record.duration}</div>
                                <div
                                  class="text-xs mt-0.5"
                                  classList={{
                                    "text-green": record.status === "promoted",
                                    "text-yellow": record.status === "deployed",
                                    "text-red": record.status === "rolled_back",
                                    "text-text-dim": !record.isCurrent && record.status !== "promoted" && record.status !== "rolled_back",
                                  }}
                                >
                                  {record.isCurrent ? "current" : record.status}
                                </div>
                              </div>
                            </div>
                          )}
                        </For>
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* All Digests */}
          <Show when={data()!.allDigests.length > 0}>
            <h2 class="text-lg font-semibold mb-3">All Digests for {data()!.name}</h2>
            <p class="text-text-dim text-xs mb-4">
              Every unique digest seen across snapshots. Lineage: originator → parent → current.
            </p>
            <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden mb-6">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-card-border text-text-dim text-xs">
                    <th class="text-left p-3">Digest</th>
                    <th class="text-left p-3">Built At</th>
                    <th class="text-left p-3">Type</th>
                    <th class="text-left p-3">In Environments</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={data()!.allDigests}>
                    {(img) => {
                      const envs = data()!.deployments
                        .filter((d) => d.digest === img.digest)
                        .map((d) => d.environment);
                      const uniqueEnvs = [...new Set(envs)];
                      return (
                        <tr class="border-b border-card-border last:border-b-0">
                          <td class="p-3 font-mono text-xs">{shortDigest(img.digest)}</td>
                          <td class="p-3 text-xs">{formatDateTime(img.builtAt)}</td>
                          <td class="p-3">
                            {img.isPatched ? (
                              <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">patched</span>
                            ) : (
                              <span class="text-xs px-1.5 py-0.5 rounded bg-gray/15 text-gray">original</span>
                            )}
                          </td>
                          <td class="p-3">
                            <div class="flex gap-1">
                              {uniqueEnvs.includes("production") && (
                                <span class="text-xs px-1.5 py-0.5 rounded bg-blue/15 text-blue">prod</span>
                              )}
                              {uniqueEnvs.includes("staging") && (
                                <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">staging</span>
                              )}
                              {uniqueEnvs.includes("testing") && (
                                <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">testing</span>
                              )}
                              {uniqueEnvs.length === 0 && (
                                <span class="text-xs text-text-dim">not deployed</span>
                              )}
                            </div>
                          </td>
                        </tr>
                      );
                    }}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Snapshot History */}
          <h2 class="text-lg font-semibold mb-3">Snapshot History</h2>
          <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-card-border text-text-dim text-xs">
                  <th class="text-left p-3">Snapshot</th>
                  <th class="text-left p-3">Tag</th>
                  <th class="text-left p-3">Digest</th>
                  <th class="text-left p-3">Change</th>
                </tr>
              </thead>
              <tbody>
                <For each={data()!.history}>
                  {(entry) => (
                    <tr
                      class="border-b border-card-border last:border-b-0"
                      classList={{ "bg-gray/5": !entry.present }}
                    >
                      <td class="p-3 font-mono text-xs">{entry.snapshot.folder}</td>
                      <td class="p-3 font-mono text-xs">{entry.present ? entry.tag : "—"}</td>
                      <td class="p-3 font-mono text-xs">{entry.present ? shortDigest(entry.digest) : "—"}</td>
                      <td class="p-3">
                        {entry.diffType === "initial" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">initial</span>
                        )}
                        {entry.diffType === "digest_change" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">digest change</span>
                        )}
                        {entry.diffType === "infra_change" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-blue/15 text-blue">infra change</span>
                        )}
                        {entry.diffType === "removed" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-red/15 text-red">removed</span>
                        )}
                        {entry.diffType === "unchanged" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-gray/15 text-gray">unchanged</span>
                        )}
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </Suspense>
    </>
  );
}
