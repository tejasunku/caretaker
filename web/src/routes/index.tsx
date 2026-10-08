import { For, Show, createMemo } from "solid-js";
import { A, createAsync } from "@solidjs/router";
import { getServiceOverview } from "./data";

export default function Overview() {
  const data = createAsync(() => getServiceOverview());

  const sortedServices = () => {
    const d = data();
    if (!d) return [];
    return Object.entries(d.services).sort(([, a], [, b]) => {
      // Sort: stale first, then by number of missing patches
      if (a.isStale !== b.isStale) return b.isStale ? 1 : -1;
      return b.missingPatches.length - a.missingPatches.length;
    });
  };

  const summary = createMemo(() => {
    const d = data();
    if (!d) return null;
    const services = Object.values(d.services);
    const active = services.filter((s) => s.tag !== "");
    const stale = services.filter((s) => s.isStale);
    const current = services.filter((s) => !s.isStale && s.tag !== "");
    const totalMissing = services.reduce((sum, s) => sum + s.missingPatches.length, 0);
    const criticalMissing = services.reduce((sum, s) => sum + (s.missingSeverity["critical"] || 0), 0);
    return { active, stale, current, totalMissing, criticalMissing, testingGaps: d.testingGaps?.length ?? 0, tagMutations: d.tagMutations?.length ?? 0 };
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
              "bg-green/5 border-green/20 text-green": summary()!.stale.length === 0,
              "bg-yellow/5 border-yellow/20 text-yellow": summary()!.stale.length > 0 && summary()!.criticalMissing === 0,
              "bg-red/5 border-red/20 text-red": summary()!.criticalMissing > 0,
            }}
          >
            <Show
              when={summary()!.stale.length > 0}
              fallback={<span>All {summary()!.active.length} active services have current images.</span>}
            >
              <span>
                {summary()!.stale.length} service{summary()!.stale.length > 1 ? "s" : ""} need patching
                {summary()!.criticalMissing > 0 ? ` (${summary()!.criticalMissing} critical)` : ""}.
              </span>
            </Show>
          </div>

          <div class="flex gap-4 mb-6">
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.active.length}</div>
              <div class="text-text-dim text-xs mt-1">Active Services</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-green">{summary()!.current.length}</div>
              <div class="text-text-dim text-xs mt-1">Up to Date</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-yellow">{summary()!.stale.length}</div>
              <div class="text-text-dim text-xs mt-1">Need Patching</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.totalMissing}</div>
              <div class="text-text-dim text-xs mt-1">Total Missing Patches</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-red">{summary()!.criticalMissing}</div>
              <div class="text-text-dim text-xs mt-1">Critical Missing</div>
            </div>
          </div>

          {/* Integrity Alerts */}
          <Show when={summary()!.testingGaps > 0 || summary()!.tagMutations > 0}>
            <div class="flex gap-4 mb-6">
              <Show when={summary()!.testingGaps > 0}>
                <div class="bg-yellow/5 border border-yellow/20 rounded-lg p-4 flex-1">
                  <div class="text-3xl font-bold" style={{ color: "var(--yellow)" }}>{summary()!.testingGaps}</div>
                  <div class="text-text-dim text-xs mt-1">Testing Gaps (skipped testing)</div>
                </div>
              </Show>
              <Show when={summary()!.tagMutations > 0}>
                <div class="bg-orange/5 border border-orange/20 rounded-lg p-4 flex-1">
                  <div class="text-3xl font-bold" style={{ color: "#f97316" }}>{summary()!.tagMutations}</div>
                  <div class="text-text-dim text-xs mt-1">Tag Mutations (digest changed under tag)</div>
                </div>
              </Show>
            </div>
          </Show>
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
                  <Show when={svc.hasTestingGap}>
                    <span class="ml-2 text-xs px-1.5 py-0.5 rounded bg-yellow/15" style={{ color: "var(--yellow)" }}>testing gap</span>
                  </Show>
                  <Show when={svc.hasTagMutation}>
                    <span class="ml-2 text-xs px-1.5 py-0.5 rounded bg-orange/15" style={{ color: "#f97316" }}>tag mutation</span>
                  </Show>
                </div>
                <div class="text-text-dim text-xs font-mono mt-1">{svc.tag}</div>
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
              </A>
            )}
          </For>
        </div>

        {/* Detailed Testing Gaps */}
        <Show when={(data()?.testingGaps?.length ?? 0) > 0}>
          <h2 class="text-lg font-semibold mb-3">Testing Gaps</h2>
          <p class="text-text-dim text-xs mb-3">Digests deployed to staging or production without passing through testing first.</p>
          <div class="bg-yellow/5 border border-yellow/20 rounded-lg p-4 mb-6">
            <For each={data()?.testingGaps ?? []}>
              {(gap) => (
                <div class="text-xs text-text-dim mt-1">
                  <span class="font-mono">{gap.tag}</span> ({gap.digest.slice(0, 13)}…) was deployed to{" "}
                  <span class="font-medium" style={{ color: "var(--yellow)" }}>{gap.environments.join(", ")}</span> without passing through testing first.
                </div>
              )}
            </For>
          </div>
        </Show>

        {/* Detailed Tag Mutations */}
        <Show when={(data()?.tagMutations?.length ?? 0) > 0}>
          <h2 class="text-lg font-semibold mb-3">Tag Mutations</h2>
          <p class="text-text-dim text-xs mb-3">Tags that had their underlying digest change without a tag update — the version tested may not be the version deployed.</p>
          <div class="bg-orange/5 border border-orange/20 rounded-lg p-4 mb-6">
            <For each={data()?.tagMutations ?? []}>
              {(mut) => (
                <div class="text-xs text-text-dim mt-1">
                  Tag <span class="font-mono">{mut.oldTag}</span> had its digest change from{" "}
                  <span class="font-mono">{mut.oldDigest.slice(0, 13)}…</span> to{" "}
                  <span class="font-mono">{mut.newDigest.slice(0, 13)}…</span> in snapshot {mut.snapshot}.
                </div>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </>
  );
}
