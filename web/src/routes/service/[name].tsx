import { For, Show, Suspense, createMemo } from "solid-js";
import { useParams, A, createAsync } from "@solidjs/router";
import { getServiceDetail } from "../data";
import { patchingStatusColor, patchingStatusLabel, type PatchingStatus } from "~/lib/compliance";

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

function formatDurationMs(ms: number | null): string {
  if (ms === null) return "—";
  const hours = Math.floor(ms / (1000 * 60 * 60));
  const days = Math.floor(hours / 24);
  const remainHours = hours % 24;
  if (days > 0) return `${days}d ${remainHours}h`;
  return `${hours}h`;
}

function severityColor(sev: string): string {
  switch (sev) {
    case "critical": return "var(--red)";
    case "high": return "#f97316";
    case "medium": return "var(--yellow)";
    case "low": return "var(--text-dim)";
    default: return "var(--text)";
  }
}

interface DeploymentRecord {
  tag: string;
  sha: string;
  deployedAt: string;
  status: string;
  endedAt: string | null;
  duration: string | null;
  isCurrent: boolean;
}

export default function ServiceDetail() {
  const params = useParams();
  const data = createAsync(() => getServiceDetail(params.name));

  const timeline = createMemo(() => {
    const d = data();
    if (!d) return null;

    const envs = ["testing", "staging", "production"] as const;
    const now = d.referenceDate;

    const result: Record<string, DeploymentRecord[]> = {};

    for (const env of envs) {
      const deploys = d.deployments
        .filter((dep: any) => dep.environment === env && dep.tag.startsWith(d.name + ":"))
        .sort((a: any, b: any) => new Date(a.deployedAt).getTime() - new Date(b.deployedAt).getTime());

      const records: DeploymentRecord[] = deploys.map((dep: any, i: number) => {
        const nextDeploy = deploys[i + 1];
        const isLast = i === deploys.length - 1;
        const endedAt = isLast ? null : nextDeploy.deployedAt;
        const duration = endedAt
          ? formatDuration(dep.deployedAt, endedAt)
          : formatDuration(dep.deployedAt, now);

        return {
          tag: dep.tag,
          sha: dep.sha,
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

  const summaryStats = createMemo(() => {
    const t = timeline();
    if (!t) return null;
    const prod = t.production?.find((r) => r.isCurrent);
    const staging = t.staging?.find((r) => r.isCurrent);
    const testing = t.testing?.find((r) => r.isCurrent);
    return { prod, staging, testing };
  });

  const patchStats = createMemo(() => {
    const d = data();
    if (!d) return null;
    const applied = d.patchDetails.filter((p) => p.applied).length;
    const missing = d.patchDetails.filter((p) => !p.applied).length;
    const overdue = d.patchDetails.filter((p) => !p.applied && p.overdue).length;
    return { applied, missing, overdue, total: d.patchDetails.length };
  });

  return (
    <>
      <A href="/" class="text-text-dim text-xs no-underline">
        ← Back to overview
      </A>

      <Suspense fallback={<p>Loading...</p>}>
        <Show when={data()}>
          <h1 class="text-2xl font-semibold mt-3 mb-1">{data()!.name}</h1>
          <p class="text-text-dim text-xs mb-1">
            Given image tag "{data()!.latestSvc?.tag ?? '—'}", which patches does it have and which is it missing?
          </p>
          <p class="text-text-dim text-xs mb-6 font-mono">
            Reference date: {data()!.referenceDate} · Current week: {data()!.currentWeek.label}
          </p>

          {/* Patching Status */}
          <Show when={data()!.compliance}>
            <div class="flex gap-4 mb-6">
              <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
                <div
                  class="text-3xl font-bold"
                  style={{ color: patchingStatusColor(data()!.compliance!.status as PatchingStatus) }}
                >
                  {patchingStatusLabel(data()!.compliance!.status as PatchingStatus)}
                </div>
                <div class="text-text-dim text-xs mt-1">Current Status</div>
              </div>
              <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
                <div class="text-3xl font-bold font-mono">{data()!.latestSvc?.tag ?? "—"}</div>
                <div class="text-text-dim text-xs mt-1">Current Tag</div>
              </div>
              <Show when={data()!.compliance!.daysUntilDeadline !== null}>
                <div class="bg-card-bg border border-card-border rounded-lg p-4 flex-1">
                  <div
                    class="text-3xl font-bold"
                    style={{
                      color:
                        data()!.compliance!.daysUntilDeadline! <= 2
                          ? "var(--red)"
                          : "var(--text)",
                    }}
                  >
                    {data()!.compliance!.daysUntilDeadline}d
                  </div>
                  <div class="text-text-dim text-xs mt-1">Until Deadline</div>
                </div>
              </Show>
            </div>
            <div class="mb-6 text-sm text-text-dim">
              {data()!.compliance!.explanation}
            </div>
          </Show>

          {/* Testing Gap Alerts */}
          <Show when={data()!.testingGaps.length > 0}>
            <div class="bg-red/5 border border-red/20 rounded-lg p-4 mb-6">
              <div class="text-sm font-semibold text-red mb-2">⚠ Testing Gaps Detected</div>
              <For each={data()!.testingGaps}>
                {(gap) => (
                  <div class="text-xs text-text-dim mt-1">
                    <span class="font-mono">{gap.tag}</span> ({gap.sha.slice(0, 13)}…) was deployed to{" "}
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
                    Tag <span class="font-mono">{mut.oldTag}</span> had its SHA change from{" "}
                    <span class="font-mono">{mut.oldSha}</span> to{" "}
                    <span class="font-mono">{mut.newSha}</span> in snapshot {mut.snapshot}.
                    This means the image content changed under the same tag.
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* Stability Pipeline Flow */}
          <Show when={data()!.stabilityImages.length > 0}>
            <h2 class="text-lg font-semibold mb-3">Stability Pipeline</h2>
            <p class="text-text-dim text-xs mb-4">
              How images move through environments. Bold borders = met required time (2d testing, 2d staging). Dashed = didn't meet.
            </p>
            <div class="flex flex-col gap-4 mb-6">
              <For each={data()!.stabilityImages}>
                {(image) => (
                  <div class="bg-card-bg border border-card-border rounded-lg p-4">
                    <div class="flex items-center gap-2 mb-3">
                      <span class="font-mono text-sm font-medium">{image.tag}</span>
                      <span class="text-text-dim text-xs">({image.label})</span>
                      <Show when={image.isStable}>
                        <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">stable</span>
                      </Show>
                      <Show when={!image.isStable}>
                        <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">unstable</span>
                      </Show>
                    </div>
                    <div class="grid grid-cols-[1fr_auto_1fr_auto_1fr] gap-3 items-center">
                      {/* Testing */}
                      <div
                        class="p-3 rounded-md text-sm"
                        classList={{
                          "border-2 border-solid border-green/40 bg-green/5": image.testing?.metRequired,
                          "border-2 border-dashed border-yellow/40 bg-yellow/5": image.testing && !image.testing.metRequired,
                          "border border-gray/20 bg-gray/5": !image.testing,
                        }}
                      >
                        <div class="text-xs uppercase tracking-wide text-text-dim mb-1">Testing</div>
                        <Show when={image.testing} fallback={<div class="text-text-dim text-xs">—</div>}>
                          <div class="font-mono text-xs">{formatDurationMs(image.testing!.durationMs)}</div>
                          <div class="text-xs text-text-dim mt-0.5">
                            {image.testing!.metRequired ? "✓ met" : "✗ not met"}
                          </div>
                        </Show>
                      </div>
                      {/* Arrow */}
                      <div class="flex items-center justify-center text-text-dim">→</div>
                      {/* Staging */}
                      <div
                        class="p-3 rounded-md text-sm"
                        classList={{
                          "border-2 border-solid border-green/40 bg-green/5": image.staging?.metRequired,
                          "border-2 border-dashed border-yellow/40 bg-yellow/5": image.staging && !image.staging.metRequired,
                          "border border-gray/20 bg-gray/5": !image.staging,
                        }}
                      >
                        <div class="text-xs uppercase tracking-wide text-text-dim mb-1">Staging</div>
                        <Show when={image.staging} fallback={<div class="text-text-dim text-xs">—</div>}>
                          <div class="font-mono text-xs">{formatDurationMs(image.staging!.durationMs)}</div>
                          <div class="text-xs text-text-dim mt-0.5">
                            {image.staging!.metRequired ? "✓ met" : "✗ not met"}
                          </div>
                        </Show>
                      </div>
                      {/* Arrow */}
                      <div class="flex items-center justify-center text-text-dim">→</div>
                      {/* Production */}
                      <div
                        class="p-3 rounded-md text-sm border border-card-border bg-card-bg"
                      >
                        <div class="text-xs uppercase tracking-wide text-text-dim mb-1">Production</div>
                        <Show when={image.production} fallback={<div class="text-text-dim text-xs">—</div>}>
                          <div class="font-mono text-xs">{formatDurationMs(image.production!.durationMs)}</div>
                          <div class="text-xs text-text-dim mt-0.5">current</div>
                        </Show>
                      </div>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* Reconciliation Windows */}
          <Show when={data()!.reconciliationWindows.length > 0}>
            <h2 class="text-lg font-semibold mb-3">Reconciliation Windows</h2>
            <p class="text-text-dim text-xs mb-4">
              For each patch, which feature lines have a compliant image within the reconciliation window.
            </p>
            <div class="flex flex-col gap-4 mb-6">
              <For each={data()!.reconciliationWindows}>
                {(rw) => (
                  <div
                    class="bg-card-bg border rounded-lg p-4"
                    classList={{
                      "border-green/20": rw.status === "compliant",
                      "border-red/20": rw.status === "non_compliant",
                    }}
                  >
                    <div class="flex items-center gap-3 mb-3">
                      <span class="font-mono text-sm font-medium">{rw.patch.id}</span>
                      <span
                        class="text-xs px-1.5 py-0.5 rounded"
                        style={{ color: severityColor(rw.patch.severity), "background-color": severityColor(rw.patch.severity) + "15" }}
                      >
                        {rw.patch.severity}
                      </span>
                      <span class="text-text-dim text-xs">{rw.patch.package}</span>
                      <span class="text-text-dim text-xs">v{rw.patch.fixedVersion}</span>
                      <Show when={rw.status === "compliant"}>
                        <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">compliant</span>
                      </Show>
                      <Show when={rw.status === "non_compliant"}>
                        <span class="text-xs px-1.5 py-0.5 rounded bg-red/15 text-red">non-compliant</span>
                      </Show>
                    </div>
                    <div class="text-xs text-text-dim mb-3 font-mono">
                      Introduced: {rw.introducedWeek.label} · Due by end of: {rw.deadlineWeek.label}
                    </div>
                    <div class="text-xs text-text-dim mb-2">{rw.explanation}</div>
                    <div class="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-2">
                      <For each={rw.featureLines}>
                        {(fl) => (
                          <div
                            class="p-2 rounded text-xs border"
                            classList={{
                              "bg-green/5 border-green/20": fl.patched,
                              "bg-red/5 border-red/20": !fl.patched,
                            }}
                          >
                            <div class="font-mono font-medium">{fl.featureTag}</div>
                            <div class="text-text-dim mt-0.5">
                              latest: {fl.latestImage.tag.split(".").pop()}
                            </div>
                            <div class="text-text-dim">
                              {fl.withinWindow ? "within window" : "outside window"}
                            </div>
                            <div class={fl.patched ? "text-green" : "text-red"}>
                              {fl.patched ? "✓ patched" : "✗ not patched"}
                            </div>
                            <Show when={fl.isCurrentInProd}>
                              <div class="text-accent text-xs mt-0.5">← in production</div>
                            </Show>
                          </div>
                        )}
                      </For>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* Deployment Timeline */}
          <Show when={timeline()}>
            <h2 class="text-lg font-semibold mb-3">Deployment Timeline</h2>
            <div class="flex flex-col gap-4">
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

          {/* Image Details */}
          <Show when={data()!.latestSvc}>
            <h2 class="text-lg font-semibold mb-3 mt-6">Image Details</h2>
            <div class="bg-card-bg border border-card-border rounded-lg p-4 mb-6">
              <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <span class="text-text-dim">Tag:</span>{" "}
                  <span class="font-mono">{data()!.latestSvc!.tag}</span>
                </div>
                <div>
                  <span class="text-text-dim">SHA:</span>{" "}
                  <span class="font-mono">{data()!.latestSvc!.sha}</span>
                </div>
              </div>
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
                  <th class="text-left p-3">Replicas</th>
                  <th class="text-left p-3">Tier</th>
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
                      <td class="p-3">{entry.present ? entry.replicaCount : "—"}</td>
                      <td class="p-3">{entry.present ? entry.resourceTier : "—"}</td>
                      <td class="p-3">
                        {entry.diffType === "initial" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">initial</span>
                        )}
                        {entry.diffType === "tag_update" && (
                          <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">tag update</span>
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
