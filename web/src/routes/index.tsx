import { For, Show, createMemo } from "solid-js";
import { A, createAsync } from "@solidjs/router";
import { getServiceOverview } from "./data";

export default function Overview() {
  const data = createAsync(() => getServiceOverview());

  const sortedServices = () => {
    const d = data();
    if (!d) return [];
    return Object.entries(d.services);
  };

  const summary = createMemo(() => {
    const d = data();
    if (!d) return null;
    return d.stats;
  });

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Patching Overview</h1>
      <p class="text-text-dim text-sm mb-6">
        Which feature versions have stale production images that need patching?
      </p>

      <Show when={data()}>
        <p class="text-text-dim text-xs mb-6 font-mono">
          Reference date: {data()!.referenceDate}
        </p>

        <Show when={summary()}>
          <div
            class="border rounded-lg p-4 mb-6 text-lg font-medium"
            classList={{
              "bg-green/5 border-green/20 text-green": summary()!.stale === 0,
              "bg-yellow/5 border-yellow/20 text-yellow": summary()!.stale > 0 && summary()!.criticalMissing === 0,
              "bg-red/5 border-red/20 text-red": summary()!.criticalMissing > 0,
            }}
          >
            <Show
              when={summary()!.stale > 0}
              fallback={<span>All {summary()!.total} services have current images.</span>}
            >
              <span>
                {summary()!.stale} service{summary()!.stale > 1 ? "s" : ""} need patching
                {summary()!.criticalMissing > 0 ? ` (${summary()!.criticalMissing} critical)` : ""}.
              </span>
            </Show>
          </div>

          <div class="flex gap-4 mb-6">
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.total}</div>
              <div class="text-text-dim text-xs mt-1">Total Services</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-green">{summary()!.total - summary()!.stale}</div>
              <div class="text-text-dim text-xs mt-1">Up to Date</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-yellow">{summary()!.stale}</div>
              <div class="text-text-dim text-xs mt-1">Need Patching</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.totalMissingPatches}</div>
              <div class="text-text-dim text-xs mt-1">Total Missing Patches</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-red">{summary()!.criticalMissing}</div>
              <div class="text-text-dim text-xs mt-1">Critical Missing</div>
            </div>
          </div>
        </Show>

        <h2 class="text-lg font-semibold mb-3">Services</h2>
        <div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4 mb-8">
          <For each={sortedServices()}>
            {([name, svc]) => (
              <A
                href={`/service/${name}`}
                class="bg-card-bg border border-card-border rounded-lg p-4 cursor-pointer hover:border-accent transition-colors no-underline text-inherit block"
              >
                <div class="mb-2">
                  <span
                    class="inline-block w-2.5 h-2.5 rounded-full mr-2"
                    style={{ "background-color": svc.isStale ? "var(--yellow)" : "var(--green)" }}
                  />
                  <span class="font-semibold text-base">{name}</span>
                </div>
                <div class="text-text-dim text-xs font-mono mt-1">{svc.repository}:{svc.tag}</div>
                <div class="text-sm font-medium mt-2" style={{ color: svc.isStale ? "var(--yellow)" : "var(--green)" }}>
                  {svc.isStale ? "Needs Patching" : "Up to Date"}
                </div>
                <Show when={svc.missingPatches.length > 0}>
                  <div class="text-text-dim text-xs mt-1">
                    Missing: {svc.missingPatches.join(", ")}
                  </div>
                  <div class="text-text-dim text-xs mt-1">
                    By severity: {Object.entries(svc.missingSeverity).map(([sev, count]) => `${count} ${sev}`).join(", ")}
                  </div>
                </Show>
                <Show when={svc.lastChanged}>
                  <div class="text-text-dim text-xs mt-2 pt-2 border-t border-card-border">
                    Last changed: {svc.lastChanged!.date.split(" ")[0]}
                  </div>
                </Show>
              </A>
            )}
          </For>
        </div>

        <h2 class="text-lg font-semibold mb-3">Git History</h2>
        <div class="bg-card-bg border border-card-border rounded-lg p-4">
          <For each={data()?.gitHistory ?? []}>
            {(commit) => (
              <div class="text-xs text-text-dim mt-1 font-mono">
                <span class="text-accent">{commit.sha.slice(0, 7)}</span>{" "}
                {commit.date.split(" ")[0]}{" "}
                {commit.message}
              </div>
            )}
          </For>
        </div>
      </Show>
    </>
  );
}
