import type { Plugin } from "vite";
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { homedir } from "node:os";
import { resolve } from "node:path";

// The web UI runs as a SolidStart SPA (ssr: false), so file-based API routes are
// not available. Instead we expose the maintenance backend as a Vite dev-server
// middleware:
//
//   POST /api/patch   { services[], dryRun }   -> patch images with Copa
//   POST /api/update  { services[], clusters[], dryRun } -> hot-swap clusters
//   GET  /api/jobs/<id>                        -> job status + captured log
//
// Patching/building an image takes ~25-55s, so jobs run in the background and the
// UI polls /api/jobs/<id> for progress.

type JobKind = "patch" | "update";

interface Job {
  id: string;
  kind: JobKind;
  status: "running" | "done" | "error";
  exitCode: number | null;
  lines: string[];
  createdAt: string;
}

const jobs = new Map<string, Job>();
const MAX_LINES = 5000;
const DOCKER_SOCKET =
  "unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock";

function sendJson(res: any, status: number, body: unknown): void {
  res.statusCode = status;
  res.setHeader("content-type", "application/json");
  res.end(JSON.stringify(body));
}

function readJson(req: any): Promise<any> {
  return new Promise((resolvePromise, reject) => {
    let raw = "";
    req.on("data", (chunk: Buffer) => {
      raw += chunk;
      if (raw.length > 1_000_000) req.destroy();
    });
    req.on("end", () => {
      try {
        resolvePromise(raw ? JSON.parse(raw) : {});
      } catch (err) {
        reject(err);
      }
    });
    req.on("error", reject);
  });
}

function asList(value: unknown): string {
  return Array.isArray(value) && value.length ? value.join(",") : "all";
}

export function caretakerPatchApi(): Plugin {
  return {
    name: "caretaker-patch-api",
    apply: "serve",
    configureServer(server) {
      const repoRoot = resolve(server.config.root, "..");

      function startJob(kind: JobKind, script: string, args: string[]): Job {
        const job: Job = {
          id: randomUUID(),
          kind,
          status: "running",
          exitCode: null,
          lines: [],
          createdAt: new Date().toISOString(),
        };
        jobs.set(job.id, job);

        const env = {
          ...process.env,
          DOCKER_HOST: process.env.DOCKER_HOST || DOCKER_SOCKET,
          CONTAINER_HOST: process.env.CONTAINER_HOST || DOCKER_SOCKET,
          KIND_EXPERIMENTAL_PROVIDER:
            process.env.KIND_EXPERIMENTAL_PROVIDER || "podman",
          PATH: `${homedir()}/bin:${resolve(repoRoot, "bin")}:${process.env.PATH ?? ""}`,
        };

        const child = spawn("python3", [resolve(repoRoot, "scripts", script), ...args], {
          cwd: repoRoot,
          env,
        });

        const capture = (buf: Buffer) => {
          for (const line of String(buf).split("\n")) {
            if (line.length) job.lines.push(line);
          }
          if (job.lines.length > MAX_LINES) {
            job.lines.splice(0, job.lines.length - MAX_LINES);
          }
        };
        child.stdout.on("data", capture);
        child.stderr.on("data", capture);
        child.on("error", (err) => {
          job.lines.push(`[error] ${err.message}`);
          job.status = "error";
        });
        child.on("close", (code) => {
          job.exitCode = code;
          job.status = code === 0 ? "done" : "error";
        });

        return job;
      }

      server.middlewares.use("/api", async (req, res, next) => {
        const path = (req.url ?? "/").split("?")[0];

        // GET /api/jobs/<id>
        if (req.method === "GET" && path.startsWith("/jobs/")) {
          const id = path.slice("/jobs/".length).split("/")[0];
          const job = jobs.get(id);
          if (!job) return sendJson(res, 404, { error: "job not found" });
          return sendJson(res, 200, job);
        }

        // POST /api/patch -> patch images only
        if (req.method === "POST" && path === "/patch") {
          let body: any;
          try {
            body = await readJson(req);
          } catch {
            return sendJson(res, 400, { error: "invalid JSON body" });
          }
          const args = ["--services", asList(body.services)];
          if (body.dryRun) args.push("--dry-run");
          if (body.force) args.push("--force");
          return sendJson(res, 202, startJob("patch", "patch_batch.py", args));
        }

        // POST /api/update -> hot-swap clusters
        if (req.method === "POST" && path === "/update") {
          let body: any;
          try {
            body = await readJson(req);
          } catch {
            return sendJson(res, 400, { error: "invalid JSON body" });
          }
          const args = [
            "--services",
            asList(body.services),
            "--clusters",
            asList(body.clusters),
          ];
          if (body.dryRun) args.push("--dry-run");
          if (body.force) args.push("--force");
          return sendJson(res, 202, startJob("update", "update_batch.py", args));
        }

        return next();
      });
    },
  };
}
