import { For, Show, Suspense, createMemo } from "solid-js";
import { useParams, A, createAsync } from "@solidjs/router";
import { getServiceDetail } from "../data";

function shortDigest(digest: string): string {
  if (!digest) return "—";
  return digest.slice(0, 13) + "…";
}

export default function ServiceDetail() {
  const params = useParams();
  const data = createAsync(() => getServiceDetail(params.name ?? ""));

  return (
    <>
      <A href="/" class="text-text-dim text-xs no-underline">
        ← Back to overview
      </A>

      <Suspense fallback={<p>Loading...</p>}>
        <Show when={data()}>
          <h1 class="text-2xl font-semibold mt-3 mb-1">{data()!.name}</h1>
          <p class="text-text-dim text-sm mb-6">
            Current state and git history for this service.
          </p>

          {/* Current State */}
          <div class="bg-card-bg border border-card-border rounded-lg p-4 mb-6">
            <h2 class="text-lg font-semibold mb-4">Current State</h2>
            <div class="grid grid-cols-2 gap-4 text-sm">
              <div>
                <div class="text-text-dim text-xs">Repository</div>
                <div class="font-mono text-sm mt-1">{data()!.state.repository}</div>
              </div>
              <div>
                <div class="text-text-dim text-xs">Tag</div>
                <div class="font-mono text-sm mt-1">{data()!.state.tag}</div>
              </div>
            </div>
          </div>

          {/* Environment States */}
          <h2 class="text-lg font-semibold mb-3">Environments</h2>
          <div class="grid grid-cols-3 gap-4 mb-6">
            <For each={["testing", "staging", "production"]}>
              {(env) => {
                const envState = data()!.state.environments[env];
                return (
                  <div class="bg-card-bg border border-card-border rounded-lg p-4">
                    <div class="text-xs uppercase tracking-wide text-text-dim mb-3 font-semibold">
                      {env}
                    </div>
                    <Show when={envState} fallback={<div class="text-text-dim text-sm">Not configured</div>}>
                      <div class="space-y-2">
                        <div>
                          <div class="text-text-dim text-xs">Tag</div>
                          <div class="font-mono text-sm">{envState.tag}</div>
                        </div>
                        <div>
                          <div class="text-text-dim text-xs">Digest</div>
                          <div class="font-mono text-xs">{shortDigest(envState.digest)}</div>
                        </div>
                        <div>
                          <div class="text-text-dim text-xs">Replicas</div>
                          <div class="text-sm">{envState.replicaCount}</div>
                        </div>
                      </div>
                    </Show>
                  </div>
                );
              }}
            </For>
          </div>

          {/* Infrastructure */}
          <h2 class="text-lg font-semibold mb-3">Infrastructure</h2>
          <div class="bg-card-bg border border-card-border rounded-lg p-4 mb-6">
            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              <For each={Object.entries(data()!.state.infra)}>
                {([key, value]) => (
                  <div>
                    <div class="text-text-dim text-xs">{key}</div>
                    <div class="font-mono text-sm mt-1">{value}</div>
                  </div>
                )}
              </For>
            </div>
          </div>

          {/* All Digests */}
          <Show when={data()!.allDigests.length > 0}>
            <h2 class="text-lg font-semibold mb-3">Digests</h2>
            <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden mb-6">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-card-border text-text-dim text-xs">
                    <th class="text-left p-3">Digest</th>
                    <th class="text-left p-3">Tag</th>
                    <th class="text-left p-3">In Environments</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={data()!.allDigests}>
                    {(img) => (
                      <tr class="border-b border-card-border last:border-b-0">
                        <td class="p-3 font-mono text-xs">{shortDigest(img.digest)}</td>
                        <td class="p-3 font-mono text-xs">{img.tag}</td>
                        <td class="p-3">
                          <div class="flex gap-1">
                            {img.environments.includes("production") && (
                              <span class="text-xs px-1.5 py-0.5 rounded bg-blue/15 text-blue">prod</span>
                            )}
                            {img.environments.includes("staging") && (
                              <span class="text-xs px-1.5 py-0.5 rounded bg-yellow/15 text-yellow">staging</span>
                            )}
                            {img.environments.includes("testing") && (
                              <span class="text-xs px-1.5 py-0.5 rounded bg-green/15 text-green">testing</span>
                            )}
                            {img.environments.length === 0 && (
                              <span class="text-xs text-text-dim">base</span>
                            )}
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Git History */}
          <h2 class="text-lg font-semibold mb-3">Git History</h2>
          <div class="bg-card-bg border border-card-border rounded-lg overflow-hidden">
            <Show when={data()!.history.length > 0} fallback={<div class="p-4 text-text-dim text-sm">No history</div>}>
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-card-border text-text-dim text-xs">
                    <th class="text-left p-3">Commit</th>
                    <th class="text-left p-3">Date</th>
                    <th class="text-left p-3">Message</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={data()!.history}>
                    {(commit) => (
                      <tr class="border-b border-card-border last:border-b-0">
                        <td class="p-3 font-mono text-xs text-accent">{commit.sha.slice(0, 7)}</td>
                        <td class="p-3 text-xs">{commit.date.split(" ")[0]}</td>
                        <td class="p-3 text-xs">{commit.message}</td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </Show>
          </div>
        </Show>
      </Suspense>
    </>
  );
}
