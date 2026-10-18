export interface JobView {
  id: string;
  kind: "patch" | "update";
  status: "running" | "done" | "error";
  exitCode: number | null;
  lines: string[];
  createdAt: string;
}

async function postJob(path: string, body: unknown): Promise<JobView> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as JobView;
}

export function startPatchJob(body: {
  services: string[];
  dryRun: boolean;
  force?: boolean;
}): Promise<JobView> {
  return postJob("/api/patch", body);
}

export function startUpdateJob(body: {
  services: string[];
  clusters: string[];
  dryRun: boolean;
  force?: boolean;
}): Promise<JobView> {
  return postJob("/api/update", body);
}

export async function getJob(id: string): Promise<JobView> {
  const res = await fetch(`/api/jobs/${id}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as JobView;
}
