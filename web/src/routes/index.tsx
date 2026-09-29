import { For, Show, createResource } from "solid-js";
import { A, useSearchParams } from "@solidjs/router";
import { Title } from "@solidjs/meta";
import {
  loadAllSnapshots,
  diffSnapshots,
  type SnapshotData,
} from "~/server/snapshots";
import {
  resolveImagePatchStatus,
  type ImagePatchReport,
} from "~/lib/queries";
import { MOCK_PATCHES } from "~/lib/mock-data";
import {
  computeCompliance,
  statusColor,
  statusLabel,
  type ComplianceStatus,
  DEFAULT_CONFIG,
} from "~/lib/compliance";

async function fetchOverviewData() {
  const snapshots = loadAllSnapshots();
  if (snapshots.length === 0) return { services: {}, snapshots: [] };

  const latest = snapshots[snapshots.length - 1];
  const diffs = snapshots.slice(1).map((s, i) => ({
    timestamp: s.timestamp,
    folder: s.folder,
    diff: diffSnapshots(snapshots[i], s),
  }));

  // Compute compliance per service
  const services: Record<string, {
    tag: string;
    sha: string;
    status: ComplianceStatus;
    explanation: string;
    missingPatches: string[];
    daysUntilDeadline: number | null;
    latestSnapshot: string;
    firstSeen: string;
  }> = {};

  // Determine all services that have ever appeared
  const allServices = new Set<string>();
  for (const snap of snapshots) {
    for (const name of Object.keys(snap.services)) {
      allServices.add(name);
    }
  }

  // Find the first and last snapshot each service appears in
  const serviceLifecycle: Record<string, { first: string; last: string }> = {};
  for (const name of allServices) {
    let first = "";
    let last = "";
    for (const snap of snapshots) {
      if (name in snap.services) {
        if (!first) first = snap.folder;
        last = snap.folder;
      }
    }
    serviceLifecycle[name] = { first, last };
  }

  const latestFolder = latest.folder;

  for (const name of allServices) {
    const lifecycle = serviceLifecycle[name];
    const isActive = name in latest.services;
    const isNewService = lifecycle.first === latestFolder;

    if (!isActive) {
      services[name] = {
        tag: latest.services[name]?.tag ?? "—",
        sha: latest.services[name]?.sha ?? "",
        status: "inactive",
        explanation: `Last seen in snapshot ${lifecycle.last}`,
        missingPatches: [],
        daysUntilDeadline: null,
        latestSnapshot: lifecycle.last,
        firstSeen: lifecycle.first,
      };
      continue;
    }

    const svcData = latest.services[name];
    const report = resolveImagePatchStatus(
      svcData.tag,
      svcData.sha,
      MOCK_PATCHES,
      []
    );

    const missingPatches = report.missingPatches.map((p) => p.id);
    const hasViablePatchedVersion = report.missingPatches.every(
      (p) => p.hasPatchAvailable
    );

    const compliance = computeCompliance(
      svcData.tag,
      report.missingPatches.map((p) => ({
        id: p.id,
        reconciliationWindowDays: 7,
        releasedAt: p.releasedAt,
      })),
      hasViablePatchedVersion,
      false,
      isNewService,
      DEFAULT_CONFIG
    );

    services[name] = {
      tag: svcData.tag,
      sha: svcData.sha,
      status: compliance.status,
      explanation: compliance.explanation,
      missingPatches,
      daysUntilDeadline: compliance.daysUntilDeadline,
      latestSnapshot: latestFolder,
      firstSeen: lifecycle.first,
    };
  }

  return { services, snapshots: diffs };
}

export default function Overview() {
  const [data] = createResource(fetchOverviewData);

  const statusOrder: ComplianceStatus[] = [
    "non_compliant",
    "at_risk_next_window",
    "at_risk_staging",
    "barely",
    "compliant",
    "new",
    "inactive",
  ];

  const sortedServices = () => {
    const d = data();
    if (!d) return [];
    return Object.entries(d.services).sort(([, a], [, b]) => {
      return statusOrder.indexOf(a.status) - statusOrder.indexOf(b.status);
    });
  };

  return (
    <>
      <Title>Caretaker — Overview</Title>
      <h1>Service Overview</h1>
      <p class="subtitle">
        Compliance status for all services based on patch reconciliation windows
      </p>

      <Show when={data()} fallback={<p>Loading snapshots...</p>}>
        <div class="status-grid">
          <For each={sortedServices()}>
            {([name, svc]) => (
              <A href={`/service/${name}`} class="status-card">
                <div style={{ "margin-bottom": "8px" }}>
                  <span
                    class="status-indicator"
                    style={{ "background-color": statusColor(svc.status) }}
                  />
                  <span class="service-name">{name}</span>
                </div>
                <div class="service-tag">{svc.tag}</div>
                <div
                  class="status-label"
                  style={{ color: statusColor(svc.status) }}
                >
                  {statusLabel(svc.status)}
                </div>
                <div class="status-explanation">{svc.explanation}</div>
                <Show when={svc.missingPatches.length > 0}>
                  <div
                    class="status-explanation"
                    style={{ "margin-top": "4px" }}
                  >
                    {svc.missingPatches.length} missing patch(es)
                  </div>
                </Show>
              </A>
            )}
          </For>
        </div>

        <h2>Snapshot Timeline</h2>
        <div class="timeline">
          <For each={data()!.snapshots}>
            {(entry) => (
              <div class="timeline-entry">
                <div class="timeline-date">{entry.folder}</div>
                <div class="timeline-changes">
                  <For each={Object.keys(entry.diff.added)}>
                    {(name) => (
                      <div class="change-item">
                        <span class="change-badge added">added</span>
                        {name}: {entry.diff.added[name].tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.removed)}>
                    {(name) => (
                      <div class="change-item">
                        <span class="change-badge removed">removed</span>
                        {name}: {entry.diff.removed[name].tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.changed)}>
                    {(name) => (
                      <div class="change-item">
                        <span class="change-badge changed">
                          {entry.diff.changed[name].diffType}
                        </span>
                        {name}: {entry.diff.changed[name].prev.tag} →{" "}
                        {entry.diff.changed[name].curr.tag}
                      </div>
                    )}
                  </For>
                  <For each={Object.keys(entry.diff.unchanged)}>
                    {(name) => (
                      <div class="change-item">
                        <span class="change-badge unchanged">unchanged</span>
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
