import { For, Show } from "solid-js";
import { A, createAsync } from "@solidjs/router";
import { getHistory } from "./data";

function shortDigest(digest: string): string {
  if (!digest) return "—";
  return digest.slice(0, 19) + "…";
}

function formatDate(at: string): string {
  if (!at) return "—";
  const [date, time] = at.replace("Z", "").split("T");
  return `${date} ${(time ?? "").slice(0, 5)} UTC`;
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

const actionClass: Record<string, string> = {
  patch: "bg-accent/20 text-accent",
  promote: "bg-green/15 text-green",
  swap: "bg-border text-text-dim",
};

export default function History() {
  const data = createAsync(() => getHistory());

  return (
    <>
      <h1 class="text-2xl font-semibold mb-1">History</h1>
      <p class="text-text-dim text-sm mb-6">
        When each cluster was patched — from what version to what, and when.
        <Show when={data()}>
          {" "}
          {data()!.totalEvents} recorded patch event(s) across {data()!.clusters.length} cluster(s).
        </Show>
      </p>

      <Show when={data()}>
        <For each={data()!.clusters}>
          {(cluster) => (
            <div class="mb-8">
              <div class="flex items-center gap-3 mb-3">
                <A
                  href={`/cluster/${cluster.id}`}
                  class="text-lg font-semibold font-mono text-accent no-underline"
                >
                  {cluster.id}
                </A>
                <span class={`text-xs px-1.5 py-0.5 rounded ${stageClass(cluster.stage)}`}>
                  {cluster.stage}
                </span>
                <span class="text-xs px-1.5 py-0.5 rounded bg-border text-text-dim">
                  {cluster.provider}
                </span>
                <span class="text-text-dim text-xs">
                  {cluster.events.length} patch{cluster.events.length === 1 ? "" : "es"}
                </span>
              </div>

              <Show
                when={cluster.events.length > 0}
                fallback={
                  <div class="bg-card-bg border border-card-border rounded-lg p-4 text-text-dim text-sm">
                    No patches recorded yet.
                  </div>
                }
              >
                <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-card-border text-text-dim text-xs">
                        <th class="text-left p-3">Date</th>
                        <th class="text-left p-3">Service</th>
                        <th class="text-left p-3">Action</th>
                        <th class="text-left p-3">From</th>
                        <th class="text-left p-3">To</th>
                        <th class="text-left p-3">Digest</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={cluster.events}>
                        {(event) => (
                          <tr class="border-b border-card-border last:border-b-0">
                            <td class="p-3 font-mono text-xs whitespace-nowrap">
                              {formatDate(event.at)}
                            </td>
                            <td class="p-3">
                              <A
                                href={`/service/${event.service}`}
                                class="text-accent no-underline font-medium"
                              >
                                {event.service}
                              </A>
                            </td>
                            <td class="p-3">
                              <span
                                class={`text-xs px-1.5 py-0.5 rounded ${actionClass[event.action] ?? "bg-border text-text-dim"}`}
                              >
                                {event.action || "swap"}
                              </span>
                            </td>
                            <td class="p-3 font-mono text-xs text-text-dim">{event.fromTag}</td>
                            <td class="p-3 font-mono text-xs text-green">{event.toTag}</td>
                            <td class="p-3 font-mono text-xs">{shortDigest(event.toDigest)}</td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </div>
          )}
        </For>
      </Show>
    </>
  );
}
