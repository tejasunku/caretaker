# Caretaker walkthrough report

> **Update:** issues #1–#9 below have since been fixed (uncommitted): false
> base-digest stamping, `--allow-tags` dead code, rollback bookkeeping, alias
> CLI (`--alias`) + field-preserving `mapping set`, `apiVersion` validation
> (bumped to `caretaker/v1alpha2`), semantic `doctor` checks, `webhook status`
> namespaces, and the `status` NOTE column.

**Date:** 2026-11-04 · **Branch:** `go-rewrite` @ `1734b99` · **Stack:** kind `caretaker`, zot `:5000`, BuildKit `:12345`

Goal of the project: demonstrate patch velocity — assess an image for CVEs, patch it with Copa,
record provenance, choose a preferred patch per feature build, roll it out to clusters with
last-known-good tracking, and catch non-caretaker deploys at admission. This walkthrough exercised
that full product lifecycle end to end on the live dev stack.

## What was exercised and worked

| Phase | Feature | Result |
|---|---|---|
| A | `config show`, `clusters`, `status`, `lkg`, `mapping show` | ok — inventory clean |
| B | `scan --image <cache base>` | ok — CycloneDX 1.7 SBOM, 114 components, digest-pinned subject |
| C | `patch image --dry-run`, `patch all --dry-run` | ok — both plan |
| C | real `patch image --service cache --write-mapping` | ok — 42s, pushed `70c40b5b…`, wrote mapping (the `--write-mapping` fix holds) |
| C | `lineage log` / `children` / `show` | ok — base f94f9680 → child 70c40b5b, DEPTH 1, correct parent/source |
| D | `update --map` dry-run + real | ok — swapped, LKG/previous/skip-webhook annotations correct |
| D | `rollback` | ok — restored previous, `skip-webhook=true` (webhook-safe) |
| D | `update --health-cmd false` | ok — detected failure, auto-rolled-back, exit 1 |
| E | webhook install/serve/status/uninstall | ok |
| E | webhook: base digest under a **different container/Deployment name** | ok — rewritten + annotated |
| E | webhook: **tag-pinned** base, no `--allow-tags` | ok — resolved and rewritten |
| E | webhook: unknown image (busybox) | ok — clean pass-through, zero annotations |
| F | `patch image --skip-if-clean` | ok — Trivy pre-scan ran (23s); auth has vulns → planned, not skipped |
| F | final `status` | **6/6 ok** |

Live fixes applied during the walkthrough (annotations only): cache and web Deployments were
manually annotated with their true `caretaker.io/base-digest` + `base-version`, which immediately
restored preference resolution for both. That experiment is what isolated issue #1 below.

## Unexpected issues

### #1 — `update` stamps a FALSE `caretaker.io/base-digest` when the true base is unknown · HIGH

`Updater.baseDigest()` (`internal/update/update.go:301-325`) falls back to
`d.Ref.Digest` — the *running image's own digest* — when there is no annotation and no lineage
metadata, and `updateOne` then writes that value into `caretaker.io/base-digest` as if it were the
feature build (`update.go:138-139`).

Observed: cache's `base-digest` became `sha256:de7a79b2…` — the **old patched image**, not the real
base `sha256:f94f9680…`.

Why it is worse than a cosmetic wrong label: the annotation poisons every later lookup. After it is
set, `status` shows `-` and `update` says `no preferred mapping` **permanently**, because lookup
treats the running digest as a base, finds no mapping keyed by it, and `ChildrenForBase` on a
non-base returns nothing — even though the correct base and target sit in the preferences file.
Manually annotating the true base made both `status` and `update` work again instantly.

**Fix direction:** never record the running digest as the base. Only write `base-digest` when it is
genuinely known (annotation → lineage metadata → `BaseForTarget`/mapping); otherwise write nothing.

### #2 — `--allow-tags` is dead code for `update` on digest-pinned deployments · HIGH

In `baseDigest()`, the `if u.AllowTags { resolve repo:<version> }` branch (`update.go:316-323`)
comes **after** `if d.Ref.Digest != "" { return d.Ref.Digest }` (`update.go:313-315`). Every
deployment we manage is digest-pinned, so the tag path can never execute. Live proof:
`update --services cache --allow-tags --dry-run` still reported `no preferred mapping`.

**Fix direction:** attempt version-tag resolution when the digest-derived candidates do not match a
mapping, not unconditionally before them.

### #3 — `Rollback` loses bookkeeping · MED

Rollback correctly sets `skip-webhook=true` and the image, but it does **not** update
`previous-image` and it **overwrites** `last-known-good-image` with the rollback target
(`update.go:436-437`). Consequences: a second `rollback` is `already at target` (skipped), a
rollback cannot be undone with `rollback`, and the LKG of the good image is lost.

**Fix direction:** on rollback, record the abandoned image as `previous-image` and keep LKG unless
explicitly asked to demote it.

### #4 — Webhook cannot catch a rollback to an older patch · HIGH

This is the headline admission use case, and it failed: deploying `cache@de7a79b2` (a real older
patch of base `f94f9680`, whose mapping now targets `70c40b5b`) passed through the webhook
**unmutated** — it is not a base, not the current target, and a fresh Deployment carries no
`base-digest` annotation. Recognition of older patches depends entirely on the preference-map
`aliases` feature … which nothing populates: `mapping set` has no `--alias` flag (and would wipe
existing aliases, see #5).

**Fix direction:** add alias support to `mapping set` (e.g. `--alias` repeatable), and populate
aliases for known older patches — likely part of the registry-backed prefs work.

### #5 — `mapping set` silently drops `baseVersion`, `aliases`, `patchedAt` · MED

Empirically confirmed on a scratch file: `mapping set` replaces the whole mapping with
`{Base, Target, Note}`, discarding the other fields. This is why `api` has no `baseVersion` (its
`patch all` tag becomes `image-patched-…` instead of `1.0-patched-…`) and it is a data-loss hazard
for aliases once #4 exists.

**Fix direction:** `Set`/`SetMapping` must preserve unspecified fields on replace.

### #6 — `doctor` has no semantic checks · HIGH

`internal/cli/doctor.go:49-73` only verifies *existence*: config file, prefs file, copa, trivy,
registry reachable, cluster reachable. During the whole walkthrough it never noticed that `cache`
and `web` resolved to nothing, that `base-digest` was missing/false, that mapped targets might not
exist in the registry, or any stale-patch anomaly. It would have reported all-green while two
services were unresolvable.

**Fix direction:** add semantic checks — per service: preference resolves a target, target exists
in the registry, deployment's `base-digest` (if present) matches the mapping's base, and patch age
vs. Trivy scan (display-only anomaly, per the standing "no gating, just display" rule).

### #7 — `catalog.Load` never validates `apiVersion`/`kind` · LOW

`internal/catalog/catalog.go:93-98` silently defaults empty values. An old `source:`-keyed
preferences file loads with `Base:""` on every mapping, so **all lookups fail forever with no
error** — a silent failure mode exactly like the one that motivated digest-first.

**Fix direction:** reject unknown or empty `apiVersion`/`kind` with a clear error naming the file.

### #8 — `webhook status` shows `NAMESPACES -` when cluster-wide · LOW

Install prints "all namespaces", status shows `-`. Cosmetic.

### #9 — `status` gives no reason for `-` · LOW

`status`'s PREFERRED/STATE column shows `-` with no explanation (unlike `update`'s NOTE column),
which made #1 hard to spot. Also, `status` is preference-map-driven only, so it cannot reflect
lineage-index-only targets (known PINS limitation).

### #10 — `api` mapping has no `baseVersion` · INFO

Consequence of #5. Cosmetic: `patch all` generates `image-patched-<date>-<digest>` for api while
other services get `1.0-patched-…`.

## Proposed fix order (one by one)

1. **#1 + #2** — same function (`baseDigest`): stop stamping a false base, make `--allow-tags` reachable. Data corruption first.
2. **#7** — reject unknown `apiVersion` (tiny, prevents a whole class of silent failure).
3. **#5 + #4** — `mapping set` preserves fields, then add `--alias` (unblocks the webhook rollback case).
4. **#3** — rollback bookkeeping.
5. **#6** — doctor semantic checks (would have caught #1; user-flagged).
6. **#8 + #9** — display polish.
7. **Registry-backed preferences refactor** (design agreed: referrers index `caretaker-preferences-index`, append-only signed entries, `mapping push/pull`, webhook reads registry with TTL cache) — last, as it is the large change.

After each batch: `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `make build`.
Commits only on request.

## Environment notes (post-restart)

No WSL socket on this box; `podman` works natively. Bring-up:

```bash
export PATH="$PWD/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=podman
setsid --fork ./bin/zot serve ./config/zot-minimal.json >/tmp/zot.log 2>&1 </dev/null
podman start buildkitd
podman start caretaker-control-plane
```

`./scripts/start-buildkit.sh` defaults `DOCKER_HOST` to the WSL path (wrong here) — use
`podman start buildkitd`. `pkill -f "webhook serve"` hangs the shell tool; run it alone in its own
call (it does work).
