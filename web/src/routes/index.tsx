import { For, Show, Suspense, createMemo } from "solid-js";
import { A, createAsync } from "@solidjs/router";
import { getServiceOverview } from "./data";
import { patchingStatusColor, patchingStatusLabel, type PatchingStatus } from "~/lib/compliance";

const statusOrder: PatchingStatus[] = [
  "non_compliant",
  "at_risk",
  "barely",
  "compliant",
  "new",
  "inactive",
];

export default function Overview() {
  const data = createAsync(() => getServiceOverview());

  const sortedServices = () => {
    const d = data();
    if (!d) return [];
    return Object.entries(d.services).sort(([, a], [, b]) => {
      return statusOrder.indexOf(a.status as PatchingStatus) - statusOrder.indexOf(b.status as PatchingStatus);
    });
  };

  const summary = createMemo(() => {
    const d = data();
    if (!d) return null;
    const services = Object.values(d.services);
    const active = services.filter((s) => s.status !== "inactive");
    const compliant = services.filter((s) => s.status === "compliant");
    const nonCompliant = services.filter((s) => s.status === "non_compliant");
    const atRisk = services.filter((s) => s.status.startsWith("at_risk") || s.status === "barely");
    const totalMissing = services.reduce((sum, s) => sum + s.missingPatches.length, 0);
    const stabilityIssues = d.testingGaps.length;
    const integrityIssues = d.tagMutations.length;
    return { active, compliant, nonCompliant, atRisk, totalMissing, stabilityIssues, integrityIssues };
  });

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Patching Overview</h1>

      <Show when={data()}>
        <p class="text-text-dim text-xs mb-6 font-mono">
          Reference date: {data()!.referenceDate}
        </p>

        <Show when={summary()}>
          <div
            class="border rounded-lg p-4 mb-6 text-lg font-medium"
            classList={{
              "bg-green/5 border-green/20 text-green": summary()!.nonCompliant.length === 0,
              "bg-red/5 border-red/20 text-red": summary()!.nonCompliant.length > 0,
            }}
          >
            <Show
              when={summary()!.nonCompliant.length > 0}
              fallback={
                <span>All {summary()!.active.length} active services are compliant.</span>
              }
            >
              <span>
                {summary()!.nonCompliant.length} service{summary()!.nonCompliant.length > 1 ? "s" : ""} non-compliant.
              </span>
            </Show>
          </div>

          <div class="flex gap-4 mb-6">
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.active.length}</div>
              <div class="text-text-dim text-xs mt-1">Active Services</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-green">{summary()!.compliant.length}</div>
              <div class="text-text-dim text-xs mt-1">Compliant</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-yellow">{summary()!.atRisk.length}</div>
              <div class="text-text-dim text-xs mt-1">At Risk</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold text-red">{summary()!.nonCompliant.length}</div>
              <div class="text-text-dim text-xs mt-1">Non-Compliant</div>
            </div>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
              <div class="text-3xl font-bold">{summary()!.totalMissing}</div>
              <div class="text-text-dim text-xs mt-1">Total Missing Patches</div>
            </div>
          </div>

          {/* Stability & Integrity Alerts */}
          <Show when={summary()!.stabilityIssues > 0 || summary()!.integrityIssues > 0}>
            <div class="flex gap-4 mb-6">
              <Show when={summary()!.stabilityIssues > 0}>
                <div class="bg-yellow/5 border border-yellow/20 rounded-lg p-4 flex-1">
                  <div class="text-3xl font-bold" style={{ color: "var(--yellow)" }}>{summary()!.stabilityIssues}</div>
                  <div class="text-text-dim text-xs mt-1">Testing Gaps (skipped testing)</div>
                </div>
              </Show>
              <Show when={summary()!.integrityIssues > 0}>
                <div class="bg-orange/5 border border-orange/20 rounded-lg p-4 flex-1">
                  <div class="text-3xl font-bold" style={{ color: "#f97316" }}>{summary()!.integrityIssues}</div>
                  <div class="text-text-dim text-xs mt-1">Tag Mutations (image changed under tag)</div>
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
                    style={{ "background-color": patchingStatusColor(svc.status as PatchingStatus) }}
                  />
                  <span class="font-semibold text-base">{name}</span>
                </div>
                <div class="text-text-dim text-xs font-mono mt-1">{svc.tag}</div>
                <div class="text-sm font-medium mt-2" style={{ color: patchingStatusColor(svc.status as PatchingStatus) }}>
                  {patchingStatusLabel(svc.status as PatchingStatus)}
                </div>
                <div class="text-text-dim text-xs mt-1">{svc.explanation}</div>
                <Show when={svc.missingPatches.length > 0}>
                  <div class="text-text-dim text-xs mt-1">
                    Missing: {svc.missingPatches.join(", ")}
                  </div>
                </Show>
                <Show when={svc.daysUntilDeadline !== null}>
                  <div class="text-text-dim text-xs mt-1">
                    Deadline: {svc.daysUntilDeadline}d remaining
                  </div>
                </Show>
              </A>
            )}
          </For>
        </div>

        {/* Detailed Stability Alerts */}
        <Show when={data()!.testingGaps.length > 0}>
          <h2 class="text-lg font-semibold mb-3">Stability Concerns</h2>
          <p class="text-text-dim text-xs mb-3">Images deployed to staging or production without passing through testing first.</p>
          <div class="bg-yellow/5 border border-yellow/20 rounded-lg p-4 mb-6">
            <For each={data()!.testingGaps}>
              {(gap) => (
                <div class="text-xs text-text-dim mt-1">
                  <span class="font-mono">{gap.tag}</span> ({gap.sha.slice(0, 13)}…) was deployed to{" "}
                  <span class="font-medium" style={{ color: "var(--yellow)" }}>{gap.environments.join(", ")}</span> without passing through testing first.
                </div>
              )}
            </For>
          </div>
        </Show>

        {/* Detailed Integrity Alerts */}
        <Show when={data()!.tagMutations.length > 0}>
          <h2 class="text-lg font-semibold mb-3">Image Integrity Concerns</h2>
          <p class="text-text-dim text-xs mb-3">Tags that had their underlying image change without a tag update — the version tested may not be the version deployed.</p>
          <div class="bg-orange/5 border border-orange/20 rounded-lg p-4 mb-6">
            <For each={data()!.tagMutations}>
              {(mut) => (
                <div class="text-xs text-text-dim mt-1">
                  Tag <span class="font-mono">{mut.oldTag}</span> had its SHA change from{" "}
                  <span class="font-mono">{mut.oldSha}</span> to{" "}
                  <span class="font-mono">{mut.newSha}</span> in snapshot {mut.snapshot}.
                </div>
              )}
            </For>
          </div>
        </Show>


      </Show>
    </>
  );
}
