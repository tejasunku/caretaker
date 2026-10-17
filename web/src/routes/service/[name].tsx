import { For, Show, createMemo } from "solid-js";
import { useParams, A, createAsync } from "@solidjs/router";
import { getServiceAcrossClusters, missingPatchesFor } from "../data";

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

export default function ServiceDetail() {
  const params = useParams();
  const data = createAsync(() => getServiceAcrossClusters(params.name ?? ""));

  const staleCount = createMemo(() => data()?.deployments.filter((d) => d.stale).length ?? 0);

  return (
    <>
      <A href="/" class="text-text-dim text-xs no-underline">
        ← Back to overview
      </A>

      <Show when={data()}>
        <h1 class="text-2xl font-semibold mt-3 mb-1">{data()!.name}</h1>
        <p class="text-text-dim text-sm mb-6">
          Deployed in {data()!.deployments.length} clusters ·{" "}
          <span style={{ color: staleCount() > 0 ? "var(--yellow)" : "var(--green)" }}>
            {staleCount() > 0 ? `${staleCount()} need patching` : "all up to date"}
          </span>
        </p>

        <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden mb-6">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-card-border text-text-dim text-xs">
                <th class="text-left p-3">Cluster</th>
                <th class="text-left p-3">Stage</th>
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
                      <A href={`/cluster/${dep.cluster}`} class="text-accent no-underline font-mono text-xs">
                        {dep.cluster}
                      </A>
                    </td>
                    <td class="p-3">
                      <span class={`text-xs px-1.5 py-0.5 rounded ${stageClass(dep.stage)}`}>
                        {dep.stage}
                      </span>
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

        <Show when={staleCount() > 0}>
          <h2 class="text-lg font-semibold mb-3">Missing Patches</h2>
          <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-card-border text-text-dim text-xs">
                  <th class="text-left p-3">Vulnerability</th>
                  <th class="text-left p-3">Package</th>
                  <th class="text-left p-3">Installed</th>
                  <th class="text-left p-3">Fixed</th>
                  <th class="text-left p-3">Severity</th>
                </tr>
              </thead>
              <tbody>
                <For each={missingPatchesFor(data()!.deployments.find((d) => d.stale)!)}>
                  {(patch) => (
                    <tr class="border-b border-card-border last:border-b-0">
                      <td class="p-3 font-mono text-xs">{patch.id}</td>
                      <td class="p-3 text-xs">{patch.package}</td>
                      <td class="p-3 font-mono text-xs">{patch.installedVersion}</td>
                      <td class="p-3 font-mono text-xs">{patch.fixedVersion}</td>
                      <td class="p-3 text-xs">{patch.severity}</td>
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
