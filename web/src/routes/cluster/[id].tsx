import { For, Show, createMemo } from "solid-js";
import { useParams, A, createAsync } from "@solidjs/router";
import { getClusterDetail } from "../data";

function shortDigest(digest: string): string {
  if (!digest) return "—";
  return digest.slice(0, 19) + "…";
}

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

export default function ClusterDetail() {
  const params = useParams();
  const data = createAsync(() => getClusterDetail(params.id ?? ""));

  const staleCount = createMemo(() => data()?.deployments.filter((d) => d.stale).length ?? 0);

  return (
    <>
      <A href="/" class="text-text-dim text-xs no-underline">
        ← Back to overview
      </A>

      <Show when={data()}>
        <div class="flex items-center gap-3 mt-3 mb-1">
          <h1 class="text-2xl font-semibold font-mono">{data()!.cluster.id}</h1>
          <span class={`text-xs px-1.5 py-0.5 rounded ${stageClass(data()!.cluster.stage)}`}>
            {data()!.cluster.stage}
          </span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-border text-text-dim">
            {data()!.cluster.provider}
          </span>
        </div>
        <p class="text-text-dim text-sm mb-6">
          {data()!.deployments.length} services ·{" "}
          <span style={{ color: staleCount() > 0 ? "var(--yellow)" : "var(--green)" }}>
            {staleCount() > 0 ? `${staleCount()} need patching` : "all up to date"}
          </span>
        </p>

        <h2 class="text-lg font-semibold mb-3">Deployments</h2>
        <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden mb-6">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-card-border text-text-dim text-xs">
                <th class="text-left p-3">Service</th>
                <th class="text-left p-3">Image</th>
                <th class="text-left p-3">Digest</th>
                <th class="text-left p-3">Replicas</th>
                <th class="text-left p-3">Status</th>
              </tr>
            </thead>
            <tbody>
              <For each={data()!.deployments}>
                {(dep) => (
                  <tr class="border-b border-card-border last:border-b-0">
                    <td class="p-3">
                      <A href={`/service/${dep.service}`} class="text-accent no-underline font-medium">
                        {dep.service}
                      </A>
                    </td>
                    <td class="p-3 font-mono text-xs">{dep.repository}:{dep.tag}</td>
                    <td class="p-3 font-mono text-xs">{shortDigest(dep.digest)}</td>
                    <td class="p-3 text-xs">{dep.replicas}</td>
                    <td class="p-3">
                      <Show
                        when={dep.stale}
                        fallback={<span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">up to date</span>}
                      >
                        <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">
                          needs patch → {dep.latestPatchedTag}
                        </span>
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>

        <Show when={data()!.events.length > 0}>
          <h2 class="text-lg font-semibold mb-3">Hot-swap Events</h2>
          <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-card-border text-text-dim text-xs">
                  <th class="text-left p-3">At</th>
                  <th class="text-left p-3">Service</th>
                  <th class="text-left p-3">From</th>
                  <th class="text-left p-3">To</th>
                </tr>
              </thead>
              <tbody>
                <For each={data()!.events}>
                  {(event) => (
                    <tr class="border-b border-card-border last:border-b-0">
                      <td class="p-3 font-mono text-xs">{event.at}</td>
                      <td class="p-3 text-xs">{event.service}</td>
                      <td class="p-3 font-mono text-xs">{event.fromTag}</td>
                      <td class="p-3 font-mono text-xs text-green">{event.toTag}</td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </Show>
    </>
  );
}
