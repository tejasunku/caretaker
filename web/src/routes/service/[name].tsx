import { For, Show, createResource } from "solid-js";
import { useParams, A } from "@solidjs/router";
import { Title } from "@solidjs/meta";
import {
  loadAllSnapshots,
  diffSnapshots,
  type SnapshotData,
} from "~/server/snapshots";
import { resolveImagePatchStatus } from "~/lib/queries";
import { MOCK_PATCHES } from "~/lib/mock-data";
import {
  computeCompliance,
  statusColor,
  statusLabel,
  DEFAULT_CONFIG,
} from "~/lib/compliance";

async function fetchServiceData(name: string) {
  const snapshots = loadAllSnapshots();
  if (snapshots.length === 0) return null;

  // Build per-snapshot history for this service
  const history: {
    snapshot: SnapshotData;
    present: boolean;
    tag: string;
    sha: string;
    replicaCount: number;
    resourceTier: string;
    diffType: string | null;
  }[] = [];

  for (let i = 0; i < snapshots.length; i++) {
    const snap = snapshots[i];
    const svc = snap.services[name];
    const prev = i > 0 ? snapshots[i - 1] : null;
    let diffType: string | null = null;

    if (prev && prev.services[name] && svc) {
      const prevSvc = prev.services[name];
      if (prevSvc.tag !== svc.tag || prevSvc.sha !== svc.sha) {
        diffType = "tag_update";
      } else if (
        prevSvc.replicaCount !== svc.replicaCount ||
        prevSvc.resourceTier !== svc.resourceTier
      ) {
        diffType = "infra_change";
      } else {
        diffType = "unchanged";
      }
    } else if (!prev && svc) {
      diffType = "initial";
    } else if (prev && prev.services[name] && !svc) {
      diffType = "removed";
    }

    history.push({
      snapshot: snap,
      present: !!svc,
      tag: svc?.tag ?? "—",
      sha: svc?.sha ?? "",
      replicaCount: svc?.replicaCount ?? 0,
      resourceTier: svc?.resourceTier ?? "",
      diffType,
    });
  }

  // Compute compliance for latest version
  const latest = snapshots[snapshots.length - 1];
  const latestSvc = latest.services[name];
  let compliance = null;

  if (latestSvc) {
    const report = resolveImagePatchStatus(
      latestSvc.tag,
      latestSvc.sha,
      MOCK_PATCHES,
      []
    );
    const hasViable = report.missingPatches.every((p) => p.hasPatchAvailable);
    compliance = computeCompliance(
      latestSvc.tag,
      report.missingPatches.map((p) => ({
        id: p.id,
        reconciliationWindowDays: 7,
        releasedAt: p.releasedAt,
      })),
      hasViable,
      false,
      false,
      DEFAULT_CONFIG
    );
  }

  return { name, history, compliance, latestSvc };
}

export default function ServiceDetail() {
  const params = useParams();
  const [data] = createResource(() => params.name, fetchServiceData);

  return (
    <>
      <Title>Caretaker — {params.name}</Title>
      <A href="/" style={{ "text-decoration": "none", color: "var(--text-dim)", "font-size": "13px" }}>
        ← Back to overview
      </A>

      <Show when={data()} fallback={<p>Loading...</p>}>
        <h1 style={{ "margin-top": "12px" }}>{data()!.name}</h1>

        <Show when={data()!.compliance}>
          <div class="stats-row">
            <div class="stat-box">
              <div
                class="stat-value"
                style={{ color: statusColor(data()!.compliance!.status) }}
              >
                {statusLabel(data()!.compliance!.status)}
              </div>
              <div class="stat-label">Current Status</div>
            </div>
            <div class="stat-box">
              <div class="stat-value">{data()!.latestSvc?.tag ?? "—"}</div>
              <div class="stat-label">Current Tag</div>
            </div>
            <Show when={data()!.compliance!.daysUntilDeadline !== null}>
              <div class="stat-box">
                <div
                  class="stat-value"
                  style={{
                    color:
                      data()!.compliance!.daysUntilDeadline! <= 2
                        ? "var(--red)"
                        : "var(--text)",
                  }}
                >
                  {data()!.compliance!.daysUntilDeadline}d
                </div>
                <div class="stat-label">Until Deadline</div>
              </div>
            </Show>
          </div>
          <div style={{ "margin-bottom": "24px", "font-size": "14px", color: "var(--text-dim)" }}>
            {data()!.compliance!.explanation}
          </div>
        </Show>

        <h2>Deployment History</h2>
        <div class="timeline">
          <For each={data()!.history}>
            {(entry) => (
              <div
                class="timeline-entry"
                classList={{
                  added: entry.diffType === "initial" || entry.diffType === "tag_update",
                  removed: entry.diffType === "removed",
                  changed: entry.diffType === "infra_change",
                }}
              >
                <div class="timeline-date">{entry.snapshot.folder}</div>
                <Show
                  when={entry.present}
                  fallback={
                    <div class="change-item">
                      <span class="change-badge removed">removed</span>
                      Service no longer deployed
                    </div>
                  }
                >
                  <div class="timeline-changes">
                    <div class="change-item">
                      <span class="change-badge changed">{entry.diffType}</span>
                      <span style={{ "font-family": "monospace" }}>{entry.tag}</span>
                    </div>
                    <div
                      class="change-item"
                      style={{ color: "var(--text-dim)", "font-size": "12px" }}
                    >
                      SHA: {entry.sha} · {entry.replicaCount} replicas · {entry.resourceTier}
                    </div>
                  </div>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>
    </>
  );
}
