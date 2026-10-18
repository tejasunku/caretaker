import { For, Show, createEffect, createMemo, createSignal, onCleanup } from "solid-js";
import { createAsync } from "@solidjs/router";
import { getPatchTargets } from "./data";
import { getJob, startPatchJob, type JobView } from "~/lib/jobs";

export default function Patch() {
  const targets = createAsync(() => getPatchTargets());

  const [selected, setSelected] = createSignal<Set<string>>(new Set());
  const [dryRun, setDryRun] = createSignal(false);
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
      setSelected(new Set(t.services.map((s) => s.name)));
    }
  });

  onCleanup(() => {
    if (timer) clearTimeout(timer);
  });

  createEffect(() => {
    job()?.lines.length;
    if (logEl) logEl.scrollTop = logEl.scrollHeight;
  });

  const toggle = (name: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });

  const allSelected = createMemo(() => {
    const t = targets();
    return !!t && t.services.length > 0 && selected().size === t.services.length;
  });

  function toggleAll() {
    const t = targets();
    if (!t) return;
    setSelected(allSelected() ? new Set<string>() : new Set(t.services.map((s) => s.name)));
  }

  async function start() {
    setError(null);
    setJob(null);
    setSubmitting(true);
    try {
      const data = await startPatchJob({ services: [...selected()], dryRun: dryRun() });
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
  const canRun = createMemo(() => selected().size > 0 && !running());

  return (
    <>
      <h1 class="text-2xl font-semibold mb-2">Patch Images</h1>
      <p class="text-text-dim text-sm mb-6">
        Build patched images with Copa and push them to the registry. Patching all images is
        safe and has no effect on running clusters — updating clusters is a separate step.
      </p>

      <Show when={targets()}>
        <div class="bg-card-bg border border-card-border rounded-lg p-4 mb-6">
          <div class="flex items-center justify-between mb-3">
            <h2 class="text-sm font-semibold">Images to patch</h2>
            <button type="button" class="text-xs text-accent hover:underline" onClick={toggleAll}>
              {allSelected() ? "Clear all" : "Select all"}
            </button>
          </div>
          <div class="grid grid-cols-2 gap-1">
            <For each={targets()!.services}>
              {(svc) => (
                <label class="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-border cursor-pointer">
                  <input
                    type="checkbox"
                    checked={selected().has(svc.name)}
                    onChange={() => toggle(svc.name)}
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
                      ? `${svc.staleClusters}/${svc.totalClusters} clusters behind`
                      : "clusters up to date"}
                  </span>
                </label>
              )}
            </For>
          </div>
        </div>

        <div class="flex items-center gap-4 mb-6">
          <button
            type="button"
            class="bg-accent text-white px-4 py-2 rounded-md text-sm font-medium disabled:opacity-40"
            disabled={!canRun()}
            onClick={start}
          >
            {running()
              ? "Patching…"
              : allSelected()
                ? "Patch all images"
                : `Patch ${selected().size} selected`}
          </button>
          <label class="flex items-center gap-2 text-sm text-text-dim cursor-pointer">
            <input
              type="checkbox"
              checked={dryRun()}
              onChange={(e) => setDryRun(e.currentTarget.checked)}
            />
            Dry run (show what would happen)
          </label>
          <span class="text-xs text-text-dim ml-auto">{selected().size} of {targets()!.services.length} image(s)</span>
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
          Patched tags are written as{" "}
          <span class="font-mono">&lt;version&gt;-patched-&lt;YYYYMMDD&gt;</span>. Patching runs Copa
          against BuildKit (about 25–55s per image), so a full run takes a few minutes. Once images
          are patched, use <span class="font-mono">Update Clusters</span> to roll them out.
        </p>
      </Show>
    </>
  );
}
