# PINS.md — Deferred Decisions and Known Gaps

This is the live register of wrinkles we know about and are not currently fixing.
It is intentionally blunt: each entry says what is missing, what happens today,
and what would have to change to close it. When an item is fixed, delete it or
move it to "Resolved" with the commit that closed it.

Current model: a Go CLI (`caretaker`) that reads a digest-first preference file,
patches with Copa, and either hot-swaps Deployments directly or lets an optional
mutating webhook rewrite images on admission. There is no GitOps/snapshot layer.

Status legend:
- **Planned** — next work; design is understood.
- **Pinned** — consciously deferred; valid but out of scope now.
- **Known gap** — current behavior is limited; we are accepting the risk.
- **Intentional** — behaves this way on purpose; documented so we do not "fix" it.
- **Checked** — looked like a gap, but current code already handles it.

## Highest-leverage open items

1. Preferences are global, not per-cluster (§2.7).
2. Parent fallback depends on registry referrers being present (§1.3).
3. Rollback protection is annotation-based; a manual image change can still be
   rewritten by the webhook (§2.1, §3.7).

---

## 1. Patching

### 1.1 Skip images with no vulnerabilities — Checked (implemented)

`--skip-if-clean` (or `patch.skipIfClean`) scans the source with Trivy JSON
before invoking Copa. Zero findings skips the build/push with
`no vulnerabilities found`; a scan failure fails open and patches anyway. No
severity filter yet; every reported vulnerability triggers patching.

Pin: Trivy cannot reliably tell fixable from unfixable findings. We assume every
reported vulnerability is patchable; if Copa cannot patch it, report the failure
and continue. We cannot take a service down for an unfixable CVE, so that edge
case is unsolvable here.

### 1.2 Fixable vs unfixable CVE classification — Pinned

We do not model "has a fix" separately. The planned zero-vuln check is the only
gate. A future version could compare installed vs fixed versions or use Trivy's
`FixedVersion` field, but that still does not guarantee Copa can patch it.

Open: severity floor, OS vs library scope, and whether unfixable findings should
block a "clean" result.

### 1.3 Parent image missing or deleted — Checked (implemented)

`patch` resolves the source digest from `--source`, `patch.sources`, or the
latest preference mapping, and `--parent-policy`
(`prefer-parent|oldest-child|newest-child|none`, default `prefer-parent`) can
fall back to an existing child when the parent is gone. Children come from OCI
lineage referrers (`internal/lineage`) and from preference mappings.

Every patch records `baseDigest`, `sourceDigest`, `parentDigest`, and
`patchedAt` as a referrer on the child, so provenance survives deletion of the
parent. Patching a child still compounds patches; the record now makes that
visible.

Open: fallback quality depends on referrers surviving registry GC, and there is
no retention policy of our own.

### 1.4 Parent selection is policy-driven — Checked

Source selection is explicit (`--source`), config (`patch.sources`), the latest
preference mapping, or the newest tag only with `--allow-tags`. `--parent-policy`
controls fallback. There is still no manifest health check on the chosen parent;
Copa reports a failure if it is broken.

### 1.5 Sequential patching — Checked (implemented)

`patch all --concurrency N` (or `patch.concurrency`) patches up to N images at
once while keeping results in request order and continuing past errors. The
default is 1. The practical limit is BuildKit and registry push throughput;
start with 2–4. Tests use a mutex-protected runner fake.

### 1.6 Patched tag collisions — Resolved

Generated tags are `<base>-<suffix>-YYYYMMDD-HHMMSS-<12-char-digest>`
(`internal/patch/patch.go`), so repeated patches of the same base no longer
collide. The tag is only a push handle; the result is identified by digest.
`--tag` still allows an explicit override.

### 1.7 No post-patch verification — Pinned

We trust Copa's exit code. We do not re-scan the patched image to confirm the
CVEs disappeared, and we do not keep the SBOM/report tied to the patch result. A
"successful" patch can still leave vulnerabilities.

Open: should `patch` optionally re-scan and report before/after counts? Where
should the SBOM/report live?

### 1.8 No package selection — Pinned

Copa has an internal patch-spec/`--config` format, but caretaker exposes no
package allowlist. Implementing `--packages` ourselves would mean filtering or
generating a Trivy report. Deferred as likely redundant.

### 1.9 Multi-arch not modeled — Pinned

`patch.platform` is a single string (default `linux/amd64`), and digest
resolution assumes one manifest. There is no handling of an OCI index or
per-platform digests.

### 1.10 Signing and signature verification — Pinned

No cosign. We have not defined an extension point yet. Proposed: a `hooks`
config with `afterPatch` and `beforeUpdate` commands receiving image/digest via
environment variables, so signing and verification can be added without a hard
dependency.

### 1.11 Registry retention / GC — Known gap

Registry cleanup is external. We do not check that a parent still exists before
patching, and we do not implement retention. This is the root cause of §1.3.

### 1.12 No patch run history — Known gap

Preferences are updated only with `--write-mapping`; results are printed and then
lost. There is no event log, job record, or audit trail for patch runs.

### 1.13 Copa failure handling — Known gap

No `--ignore-errors` passthrough, no retry, and `patch all` continues after a
failure. `--write-mapping` records only successful results. A failed image is
reported in the final error count but not persisted.

### 1.14 Broken newest tag — Known gap

If the highest base tag resolves to a missing or broken manifest, `PatchService`
fails rather than trying the next tag. The parent policy in §1.3 should cover
this.

---

## 2. Update and rollback

### 2.1 Webhook can undo rollback — Resolved

`Updater.Rollback` and the automatic `recover` path now set
`caretaker.io/skip-webhook: "true"` before changing the image, so the admission
webhook leaves the rollback alone. A subsequent normal update sets the annotation
to `"false"`, re-enabling mutation.

Residual: a manual image edit outside caretaker is still subject to the webhook.
Use the skip annotation for deliberate manual rollbacks.

### 2.2 `newest` policy for digest-only Deployments — Resolved

`chooseTarget` now uses `catalog.LatestTarget` instead of registry tags, so the
`newest` policy works for digest-only Deployments without a tag.

### 2.3 LKG is one step deep — Known gap

We keep `last-known-good-image` and `previous-image` only. There is no history,
so rollback beyond one step requires `--image`. Repeated updates overwrite the
previous slot.

### 2.4 Rollback does not run health checks — Known gap

`Rollback` and `recover` only call `WaitForRollout`
(`internal/update/update.go:186`, `:356`). They do not call `Health.Verify`, so a
healthy rollout can still be a broken application. Update verifies health;
rollback does not.

### 2.5 Batch updates are not atomic — Pinned

`UpdateAll` is sequential; a failure or abort leaves earlier services updated.
There is no batch-level transaction or batch rollback. Per your call, we are not
fixing abort semantics now.

### 2.6 Explicit `--map` bypasses lineage — Intentional

`--map` is an operator override and can force a 2.0 patched image onto a 1.9
Deployment. The normal mapping and `newest` paths are base-version scoped, so 1.9
will not silently receive 2.0 patches.

### 2.7 Preferences are global — Pinned

One `config/preferences.yaml` applies to every cluster. There is no per-cluster
preference or service override in `caretaker.yaml`. Divergence requires `--map`
or a separate file. You decided not to change this for now.

### 2.8 No update history / audit — Known gap

State lives in Deployment annotations and the preference file. There is no
central log of who updated what, when, or why.

### 2.9 No retries or backoff — Known gap

Transient registry or API errors fail the service immediately. There is no retry
policy beyond client-go's conflict retry on `SetImage` and `Annotate`.

### 2.10 Sequential updates — Known gap

Clusters and services are processed one at a time. This is fine at the current
scale but slow for many clusters.

### 2.11 Health is optional — Known gap

If `--health-cmd` is not set, verification is rollout-only. There is no default
HTTP/TCP probe or readiness check beyond Kubernetes rollout status.

---

## 3. Webhook

### 3.1 Only regular containers — Known gap

`Mutate` iterates `spec.template.spec.containers` only; `initContainers` and
ephemeral containers are not rewritten.

### 3.2 `failurePolicy: Ignore` — Accepted

If the webhook is down, the API server admits Deployments unpatched. This is
intentional for availability, but it means the webhook is not a guarantee.

### 3.3 Registry dependency at admission — Known gap

Mutation resolves the base digest from `caretaker.io/base-digest`, the running
digest, or a preference mapping. Registry calls happen only on the opt-in tag
path (`--allow-tags`). If the registry is unavailable, `preferred` returns no
match and the image is admitted unchanged (fail-open).

### 3.4 Self-signed certs, no rotation — Known gap

Certs are generated into `.caretaker/webhook/` (gitignored). There is no
rotation, expiry monitoring, or automatic CA bundle refresh.

### 3.5 Per-cluster only — Pinned

`MutatingWebhookConfiguration` is cluster-scoped. Each cluster needs its own
install; one server URL can serve several, but there is no multi-cluster
management command. CLI updates remain the primary path.

### 3.6 No signature verification — Pinned

The webhook does not verify image signatures (see §1.10).

### 3.7 Skip annotation is manual — Narrowed

Rollback and automatic recovery set `caretaker.io/skip-webhook` automatically,
and normal updates clear it. There is still no standalone helper command for a
manual one-off mutation; set the annotation directly if needed.

### 3.8 Primary-container assumption in CLI — Known gap

`kube.SetImage` updates the container named after the service, or index 0 if none
matches. Sidecar images are not touched, unlike the webhook, which iterates all
regular containers.

---

## 4. Configuration and state

### 4.1 Manual cluster list — Accepted

Clusters are declared in `caretaker.yaml`; there is no discovery or
auto-registration. That is the intended workflow for now.

### 4.2 Stale preference mappings — Known gap

Mappings are never garbage-collected. Entries can point at deleted images or
obsolete digests; lookup simply fails or returns a dead target.

### 4.3 No schema validation or migration — Known gap

`catalog.Load` defaults `apiVersion`/`kind` and unmarshals; unknown fields are
ignored and there is no migration path for older files.

### 4.4 No central state store — Known gap

The source of truth is split between Deployment annotations and the preference
file. There is no database or event log.

### 4.5 Registry auth is basic-auth only — Known gap

`registry.New` supports anonymous or username/password from config. There is no
credential helper, keychain, workload identity, or per-registry auth.

### 4.6 Workstation-only credentials — Known gap

Both CLI and webhook use the local kubeconfig and registry credentials. There is
no in-cluster service-account mode.

---

## 5. Testing and operations

### 5.1 No CI pipeline — Known gap

Unit tests, `go vet`, and `make build` are run manually. There is no automated
gate on pushes.

### 5.2 Integration tests need a live stack — Known gap

`test/integration` requires a kind context, registry, and BuildKit
(`CARETAKER_TEST_CONTEXT`), and restores Deployments on cleanup. It is not
hermetic and is not run by default.

### 5.3 No metrics or structured audit — Known gap

Output is human-readable tables and logs. There are no metrics, traces, or
durable structured events.

### 5.4 Parallel patch test infrastructure — Planned

A concurrency test needs a thread-safe runner; the current `runner.Print` fake
appends without a lock.

---

## 6. Checked and safe (no action)

### 6.1 Feature-version rollback cannot cross lineages — Checked

`catalog.Lookup` is digest-only by default, and each mapping carries its
`baseDigest`/`baseVersion` lineage. A 1.9 Deployment cannot match a 2.0 mapping
unless its digest is explicitly listed as the source, and `--allow-tags` is
required for legacy tag-only mappings. The only bypass is an explicit `--map`.
Regression tests cover digest identity, tag rejection, and that a newer
base-version mapping is not selected for an older Deployment.

### 6.2 Abort and rollback to LKG — Checked

Automatic rollback on failed verification and `caretaker rollback` are
implemented. Batch abort semantics are intentionally out of scope (§2.5).

### 6.3 Digest identity — Checked

Running images are pinned by digest and preferences are digest-only. Tags are
accepted only with `--allow-tags` (or `config.allowTags`) and are resolved to
digests before storage and lookup. The remaining tag uses are the generated push
handle and explicit opt-in discovery.

---

## Deferred feature requests

| Request | Status | Where |
|---|---|---|
| Do not create a patched image if there are no CVEs | Checked (implemented) | §1.1 |
| Rollback feature build | Checked / no change | §6.1 |
| Abort patching, roll back to LKG | Checked | §6.2 |
| Create a patch even if the parent/feature image is missing | Checked (implemented) | §1.3 |
| Signing and verification | Pinned | §1.10 |
| Manually create a Helm chart for patched images | Pinned | below |
| Create patches in parallel | Checked (implemented) | §1.5 |
| Take packages as input | Pinned | §1.8 |

**Helm chart snapshot — Pinned.** This conflicts with the direct-cluster model.
If it returns, treat it as a read-only materialization of the current desired
state for a cluster or group, not as an alternative deploy path. Generating a
chart that someone then applies would reintroduce drift between the chart and
what caretaker actually did.

**Per-cluster preferences — Pinned.** See §2.7. The current global file is a
deliberate simplification.

**Multi-cluster webhook management — Pinned.** See §3.5. The CLI is the primary
update mechanism; the webhook is optional and per-cluster.

---

## Resolved in the digest-first refactor

- **Tag identity removed.** `image.SameImage` is digest-first, `catalog` stores
  and looks up digests only, and tag refs are rejected unless `--allow-tags`
  (or `config.allowTags`) is set. Tag refs are resolved to digests before use.
- **`caretaker.io/base-digest`** is recorded and used by update and webhook
  resolution. `base-version` is still recorded for display and opt-in fallback.
- **Lineage referrers.** Patched images carry `baseDigest`, `sourceDigest`,
  `parentDigest`, and `patchedAt`; parents carry child pointers.
- **Unique patched tags** (§1.6) and **digest-only `newest`** (§2.2).
- **Parent fallback** via `--parent-policy` (§1.3, §1.4).
- **Clean-image skip** via `--skip-if-clean` (§1.1).
- **Concurrent patching** via `patch all --concurrency` (§1.5).
- **Webhook-safe rollback** via `skip-webhook` (§2.1, §3.7).

## Maintenance

- When an item is fixed, delete it from its section and add a one-line entry
  under "Resolved" with the commit that closed it.
- If an item moves from Pinned to Planned, update the status here and mention it
  in the commit message.
- Keep this file honest. A gap that is not listed is a surprise waiting to
  happen.
