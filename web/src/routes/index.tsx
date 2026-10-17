import { For, Show, createMemo } from "solid-js";
import { A, createAsync } from "@solidjs/router";
import { getOverview } from "./data";

function stageClass(stage: string): string {
  switch (stage) {
    case "testing":
      return "bg-green/15 text-green";
    case "staging":
      return "bg-yellow/15 text-yellow";
    default:
      return "bg-blue/15 text-blue";
  }
}

export default function Overview() {
  const data = createAsync(() => getOverview());

  const summary = createMemo(() => data()?.stats ?? null);

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Patching Overview</h1>
      <p class="text-text-dim text-sm mb-6">
        Which clusters are running stale images that need patching?
      </p>

      <Show when={data()}>
        <p class="text-text-dim text-xs mb-6 font-mono">
          Generated: {data()!.generatedAt}
        </p>

        <Show when={summary()}>
          <div
            class="border rounded-lg p-4 mb-6 text-lg font-medium"
            classList={{
              "bg-green/5 border-green/20 text-green": summary()!.staleDeployments === 0,
              "bg-yellow/5 border-yellow/20 text-yellow":
                summary()!.staleDeployments > 0 && summary()!.criticalMissing === 0,
              "bg-red/5 border-red/20 text-red": summary()!.criticalMissing > 0,
            }}
          >
            <Show
              when={summary()!.staleDeployments > 0}
              fallback={<span>All {summary()!.totalDeployments} deployments across {summary()!.totalClusters} clusters are up to date.</span>}
            >
              <span>
                {summary()!.staleDeployments} deployment{summary()!.staleDeployments > 1 ? "s" : ""} across{" "}
                {summary()!.totalClusters} clusters need patching
                {summary()!.criticalMissing > 0 ? ` (${summary()!.criticalMissing} critical)` : ""}.
              </span>
            </Show>
          </div>

          <div class="flex gap-4 mb-6">
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.totalClusters}</div>
              <div class="text-text-dim text-xs mt-1">Clusters</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.totalDeployments}</div>
              <div class="text-text-dim text-xs mt-1">Deployments</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-yellow">{summary()!.staleDeployments}</div>
              <div class="text-text-dim text-xs mt-1">Need Patching</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-red">{summary()!.criticalMissing}</div>
              <div class="text-text-dim text-xs mt-1">Critical Missing</div>
            </div>
          </div>
        </Show>

        <h2 class="text-lg font-semibold mb-3">Clusters</h2>
        <div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4 mb-8">
          <For each={data()!.clusters}>
            {(cluster) => (
              <A
                href={`/cluster/${cluster.id}`}
                class="bg-card-bg border border-card-border rounded-lg p-4 cursor-pointer hover:border-accent transition-colors no-underline text-inherit block"
              >
                <div class="flex items-center justify-between mb-1">
                  <div>
                    <span
                      class="inline-block w-2.5 h-2.5 rounded-full mr-2"
                      style={{ "background-color": cluster.stale > 0 ? "var(--yellow)" : "var(--green)" }}
                    />
                    <span class="font-semibold text-base font-mono">{cluster.id}</span>
                  </div>
                </div>
                <div class="flex gap-1 mt-1 mb-3">
                  <span class={`text-xs px-1.5 py-0.5 rounded ${stageClass(cluster.stage)}`}>
                    {cluster.stage}
                  </span>
                  <span class="text-xs px-1.5 py-0.5 rounded bg-border text-text-dim">
                    {cluster.provider}
                  </span>
                </div>
                <div
                  class="text-sm font-medium"
                  style={{ color: cluster.stale > 0 ? "var(--yellow)" : "var(--green)" }}
                >
                  {cluster.stale > 0
                    ? `${cluster.stale} of ${cluster.services} need patching`
                    : `All ${cluster.services} up to date`}
                </div>
              </A>
            )}
          </For>
        </div>

        <h2 class="text-lg font-semibold mb-3">Available Patches</h2>
        <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-card-border text-text-dim text-xs">
                <th class="text-left p-3">Vulnerability</th>
                <th class="text-left p-3">Package</th>
                <th class="text-left p-3">Fixed Version</th>
                <th class="text-left p-3">Severity</th>
                <th class="text-left p-3">Released</th>
              </tr>
            </thead>
            <tbody>
              <For each={data()!.patches}>
                {(patch) => (
                  <tr class="border-b border-card-border last:border-b-0">
                    <td class="p-3 font-mono text-xs">{patch.id}</td>
                    <td class="p-3 text-xs">{patch.package}</td>
                    <td class="p-3 font-mono text-xs">{patch.fixedVersion}</td>
                    <td class="p-3 text-xs">{patch.severity}</td>
                    <td class="p-3 text-xs">{patch.releasedAt.split("T")[0]}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>
    </>
  );
}
