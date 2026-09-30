import { For, Show } from "solid-js";
import { createAsync } from "@solidjs/router";
import { getServiceOverview } from "./data";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString("sv-SE", { timeZone: "UTC" }).replace(" ", "T") + "Z";
}

export default function History() {
  const data = createAsync(() => getServiceOverview());

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Deployment History</h1>
      <p class="text-text-dim text-xs mb-6 font-mono">
        Reference date: {data()?.referenceDate}
      </p>

      <Show when={data()}>
        <div class="relative pl-6">
          <div class="absolute left-1.5 top-0 bottom-0 w-0.5 bg-border" />
          <For each={data()!.snapshots}>
            {(entry) => (
              <div class="relative mb-6 p-4 bg-card-bg border border-card-border rounded-lg before:content-[''] before:absolute before:-left-5 before:top-5 before:w-2.5 before:h-2.5 before:rounded-full before:bg-accent before:border-2 before:border-bg">
                <div class="text-xs text-text-dim font-mono mb-2">{formatDate(entry.folder)}</div>
                <div class="flex flex-col gap-1.5">
                  <For each={Object.keys(entry.diff.added)}>
                    {(name) => (
                      <div class="text-xs flex items-center gap-2">
                        <span class="text-xs px-1.5 py-0.5 rounded font-medium bg-green/15 text-green">added</span>
                        {name}: {entry.diff.added[name].tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.removed)}>
                    {(name) => (
                      <div class="text-xs flex items-center gap-2">
                        <span class="text-xs px-1.5 py-0.5 rounded font-medium bg-red/15 text-red">removed</span>
                        {name}: {entry.diff.removed[name].tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.changed)}>
                    {(name) => (
                      <div class="text-xs flex items-center gap-2">
                        <span class="text-xs px-1.5 py-0.5 rounded font-medium bg-yellow/15 text-yellow">
                          {entry.diff.changed[name].diffType}
                        </span>
                        {name}: {entry.diff.changed[name].prev.tag} →{" "}
                        {entry.diff.changed[name].curr.tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.unchanged)}>
                    {(name) => (
                      <div class="text-xs flex items-center gap-2">
                        <span class="text-xs px-1.5 py-0.5 rounded font-medium bg-gray/15 text-gray">unchanged</span>
                        {name}
                      </div>
                    )}
                  </For>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </>
  );
}
