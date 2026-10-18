import { For, Show, createEffect, createMemo, createSignal, onCleanup } from "solid-js";
import { createAsync } from "@solidjs/router";
import { getPatchTargets } from "./data";
import { getJob, startUpdateJob, type JobView } from "~/lib/jobs";

function stageClass(stage: string): string {
  if (stage === "testing") return "bg-green/15 text-green";
  if (stage === "staging") return "bg-yellow/15 text-yellow";
  return "bg-blue/15 text-blue";
}

export default function Update() {
  const targets = createAsync(() => getPatchTargets());

  const [clusters, setClusters] = createSignal<Set<string>>(new Set());
  const [services, setServices] = createSignal<Set<string>>(new Set());
  const [dryRun, setDryRun] = createSignal(false);
  const [force, setForce] = createSignal(false);
  const [job, setJob] = createSignal<JobView | null>(null);
  const [submitting, setSubmitting] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  let initialized = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let logEl: HTMLPreElement | undefined;

  createEffect(() => {
    const t = targets();
    if (t && !initialized) {
      initialized = true;
      setClusters(new Set(t.clusters.map((c) => c.id)));
      setServices(new Set(t.services.map((s) => s.name)));
    }
  });

  onCleanup(() => {
    if (timer) clearTimeout(timer);
  });

  createEffect(() => {
    job()?.lines.length;
    if (logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  const toggle = (set: typeof setClusters, name: string) =>
    set((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });

  const allClusters = createMemo(() => {
    const t = targets();
    return !!t && t.clusters.length > 0 && clusters().size === t.clusters.length;
  });
  const allServices = createMemo(() => {
    const t = targets();
    return !!t && t.services.length > 0 && services().size === t.services.length;
  });

  function toggleAllClusters() {
    const t = targets();
    if (!t) return;
    setClusters(allClusters() ? new Set<string>() : new Set(t.clusters.map((c) => c.id)));
  }
  function toggleAllServices() {
    const t = targets();
    if (!t) return;
    setServices(allServices() ? new Set<string>() : new Set(t.services.map((s) => s.name)));
  }

  async function start() {
    setError(null);
    setJob(null);
    setSubmitting(true);
    try {
      const data = await startUpdateJob({
        services: [...services()],
        clusters: [...clusters()],
        dryRun: dryRun(),
        force: force(),
      });
      poll(data.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  async function poll(id: string) {
    try {
      const data = await getJob(id);
      setJob(data);
      if (data.status === "running") timer = setTimeout(() => poll(id), 1000);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  const running = createMemo(() => job()?.status === "running" || submitting());
  const canRun = createMemo(() => clusters().size > 0 && services().size > 0 && !running());

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Update Clusters</h1>
      <p class="text-text-dim text-sm mb-6">
        Hot-swap the latest patched image into running clusters. Only the clusters and services you
        select are touched. Clusters running newer versions are left alone.
      </p>

      <Show when={targets()}>
        <div class="grid grid-cols-2 gap-4 mb-6">
          <div class="bg-card-bg border border-card-border rounded-lg p-4">
            <div class="flex items-center justify-between mb-3">
              <h2 class="text-sm font-semibold">Clusters to update</h2>
              <button type="button" class="text-xs text-accent hover:underline" onClick={toggleAllClusters}>
                {allClusters() ? "Clear all" : "Select all"}
              </button>
            </div>
            <div class="flex flex-col gap-1">
              <For each={targets()!.clusters}>
                {(c) => (
                  <label class="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-border cursor-pointer">
                    <input
                      type="checkbox"
                      checked={clusters().has(c.id)}
                      onChange={() => toggle(setClusters, c.id)}
                    />
                    <span class="font-mono text-sm flex-1">{c.id}</span>
                    <span class="text-xs px-1.5 py-0.5 rounded" classList={{ [stageClass(c.stage)]: true }}>
                      {c.stage}
                    </span>
                    <span
                      class="text-xs px-1.5 py-0.5 rounded"
                      classList={{
                        "bg-yellow/15 text-yellow": c.stale > 0,
                        "bg-green/15 text-green": c.stale === 0,
                      }}
                    >
                      {c.stale > 0 ? `${c.stale}/${c.services} behind` : "up to date"}
                    </span>
                  </label>
                )}
              </For>
            </div>
          </div>

          <div class="bg-card-bg border border-card-border rounded-lg p-4">
            <div class="flex items-center justify-between mb-3">
              <h2 class="text-sm font-semibold">Services to update</h2>
              <button type="button" class="text-xs text-accent hover:underline" onClick={toggleAllServices}>
                {allServices() ? "Clear all" : "Select all"}
              </button>
            </div>
            <div class="flex flex-col gap-1">
              <For each={targets()!.services}>
                {(svc) => (
                  <label class="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-border cursor-pointer">
                    <input
                      type="checkbox"
                      checked={services().has(svc.name)}
                      onChange={() => toggle(setServices, svc.name)}
                    />
                    <span class="font-mono text-sm flex-1">{svc.name}</span>
                    <span
                      class="text-xs px-1.5 py-0.5 rounded"
                      classList={{
                        "bg-yellow/15 text-yellow": svc.staleClusters > 0,
                        "bg-green/15 text-green": svc.staleClusters === 0,
                      }}
                    >
                      {svc.staleClusters > 0
                        ? `${svc.staleClusters}/${svc.totalClusters} behind`
                        : "up to date"}
                    </span>
                  </label>
                )}
              </For>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-4 mb-6 flex-wrap">
          <button
            type="button"
            class="bg-accent text-white px-4 py-2 rounded-md text-sm font-medium disabled:opacity-40"
            disabled={!canRun()}
            onClick={start}
          >
            {running()
              ? "Updating…"
              : `Update ${clusters().size} cluster(s)`}
          </button>
          <label class="flex items-center gap-2 text-sm text-text-dim cursor-pointer">
            <input
              type="checkbox"
              checked={dryRun()}
              onChange={(e) => setDryRun(e.currentTarget.checked)}
            />
            Dry run (show what would happen)
          </label>
          <label class="flex items-center gap-2 text-sm text-text-dim cursor-pointer">
            <input
              type="checkbox"
              checked={force()}
              onChange={(e) => setForce(e.currentTarget.checked)}
            />
            Force (override newer-version guard)
          </label>
          <span class="text-xs text-text-dim ml-auto">
            {clusters().size} of {targets()!.clusters.length} cluster(s) · {services().size} of{" "}
            {targets()!.services.length} service(s)
          </span>
        </div>

        <Show when={error()}>
          <div class="border border-red/30 bg-red/5 text-red rounded-lg p-3 mb-4 text-sm">
            {error()}
          </div>
        </Show>

        <Show when={job()}>
          <div class="mb-3 flex items-center gap-3">
            <h2 class="text-sm font-semibold">Log</h2>
            <span
              class="text-xs px-2 py-0.5 rounded font-medium"
              classList={{
                "bg-blue/15 text-blue": job()!.status === "running",
                "bg-green/15 text-green": job()!.status === "done",
                "bg-red/15 text-red": job()!.status === "error",
              }}
            >
              {job()!.status}
            </span>
            <span class="text-xs text-text-dim font-mono">{job()!.createdAt}</span>
          </div>
          <pre
            ref={logEl}
            class="bg-card-bg border border-card-border rounded-lg p-4 text-xs font-mono overflow-auto whitespace-pre-wrap mb-6"
            style={{ "max-height": "420px" }}
          >
            {job()!.lines.length ? job()!.lines.join("\n") : "Waiting for output…"}
          </pre>
        </Show>

        <p class="text-text-dim text-xs">
          A deployment is updated by pinning it to the latest{" "}
          <span class="font-mono">&lt;version&gt;-patched-&lt;YYYYMMDD&gt;</span> image
          (<span class="font-mono">kubectl set image</span> for real clusters). Run{" "}
          <span class="font-mono">Patch Images</span> first to produce patched images.
        </p>
      </Show>
    </>
  );
}
