---
name: caretaker
description: Overview of caretaker — a Go CLI that patches container images with Copacetic and stages digest-pinned updates across Kubernetes clusters. Covers the preference map, per-cluster last-known-good annotations, the update/verify/rollback flow, the OCI Referrers-based lineage log, configuration/env vars, and the demo kind + zot + BuildKit stack.
---

# Caretaker Skill

Caretaker is a workstation CLI that patches container images with Copacetic and drives
digest-pinned image updates across Kubernetes clusters. Nothing runs inside the cluster; the
tool reads and mutates Deployments through client-go.

It demonstrates:
1. **Patch production:** Copa builds a patched image and pushes it to the registry.
2. **Preferred-patch mapping:** one file maps a service + base version to the chosen patch.
3. **Staged rollout:** per-cluster update, verification, last-known-good tracking, and rollback.

## Layout

```
cmd/caretaker              entry point
internal/cli               cobra commands (flags + printing only)
internal/config            YAML + env precedence
internal/image             reference parsing, version/tag helpers
internal/registry          OCI client (go-containerregistry)
internal/lineage           referrer-based lineage: record, read, base lookup
internal/kube              client-go Cluster interface
internal/catalog           preference map: lookup, merge, save
internal/patch             Copa/Trivy orchestration
internal/update            plan -> swap -> verify -> LKG -> rollback
internal/health            rollout wait + optional health command
internal/webhook           admission webhook: mutation, TLS certs, install/status
internal/runner            exec abstraction
dockerfiles/               six demo images, one unique Dockerfile per service
patch-manifests/           sample Copa reports for --report
scripts/                   registry, BuildKit, kind, image setup
test/integration/          live update/rollback/patch tests (build tag)
```

## Nomenclature

One vocabulary everywhere (code, annotations, docs, CLI output):

| Term | Meaning | Where it lives |
|---|---|---|
| **base** (a.k.a. **feature build**) | The image built by a service's repo/Dockerfile — the unpatched root of a patch chain. | `caretaker.io/base-digest`, `caretaker.io/base-version` |
| **parent** (a.k.a. **source**) | The immediate image handed to Copa (`-i`). Equal to the base when the source is unpatched; a patched child when we patch a patch. | `caretaker.io/parent-digest`, `caretaker.io/source-digest` |
| **child** | The patched image Copa produced. | `caretaker.io/child-digest`, `lineage show` |
| **descendant** | Any child of a base, direct or through intermediate patched children (reported as `DEPTH` by `lineage children`). | `caretaker-lineage-index` log |

Rules that follow from this:

- Never call a patched image the base, and never call the root a parent. In
  `root -> child1 -> child2` there is one base, two parents, two children.
- Identity is always a digest. Tags are labels only (`--allow-tags` to opt in).
  Discovery of "what descends from this digest" comes from the registry lineage
  index, not from tag scans.
- Parent-side referrers die with the parent, so the index log and the child's
  own metadata referrer are what keep a chain readable after pruning.

## Data model

- **Identity is a digest.** Running images are `repo@sha256:...`; tags are labels.
- **Universal preference map** — `config/preferences.yaml`, keyed by feature-build (base) digest:
  ```yaml
  apiVersion: caretaker/v1alpha1
  kind: PatchPreferences
  services:
    api:
      policy: pinned          # or newest
      mappings:
        - base: localhost:5000/test/api@sha256:04e9733f...   # feature build digest
          aliases:                                            # optional older patched digests of the same feature set
            - localhost:5000/test/api@sha256:9f2c1a...
          target: localhost:5000/test/api@sha256:33e60967...  # preferred patched digest
          baseVersion: "1.0"
  ```
  One row per feature build. The optional `aliases` list holds other digests
  (typically older patched versions of the same feature set) that also resolve to
  `target`, so a rollback to an older patched image is still recognized and bumped
  forward — this is how feature sets are grouped now that tags aren't assumed to
  exist. Lookup is digest-only: the running image as a
  feature build or alias, then as a mapping target (to recover its base), then the
  recorded `caretaker.io/base-digest`. Tags are accepted as input only with
  `--allow-tags` and are resolved to digests before storing. Unmapped images
  are ignored (update falls through to the lineage index; the webhook passes
  them through).
- **Per-cluster LKG** is stored as Deployment annotations:
  - `caretaker.io/last-known-good-image` — last image that passed verification
  - `caretaker.io/previous-image` — image replaced by the last update
  - `caretaker.io/base-digest` — digest of the base (feature build) for the running image
  - `caretaker.io/base-version` — `1.0`, the base's human version
  - `caretaker.io/last-update-at`, `caretaker.io/managed`
  - `caretaker.io/webhook` — set when the admission webhook rewrote the image

## Update flow

`caretaker update`:
1. Choose a target, digest-only and never by listing tags: `--map service=image`,
   the preference map, the per-service `newest` policy, then the lineage index
   (the newest surviving patched child of the deployment's base). The base
   comes from `caretaker.io/base-digest`, the running image's own lineage
   referrer, or the running digest itself.
2. Resolve the target to a digest and pin it as `repo@digest`.
3. Seed LKG with the running image (only if absent) and record `previous-image`.
4. Swap the Deployment image.
5. Wait for rollout; optionally run `--health-cmd`.
6. On success, advance LKG to the new image. On failure, roll back to the old image automatically
   (`--no-rollback` disables this).

`caretaker rollback` returns to `previous-image` (one step back), `--from-lkg`, or an explicit
`--image`. Batch rollback skips services with no recorded target.

## Quick start

```bash
export PATH="$PWD/bin:$PATH"
export DOCKER_HOST="unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock"
export CONTAINER_HOST="$DOCKER_HOST"
export KIND_EXPERIMENTAL_PROVIDER=podman

make setup                     # copa, trivy, zot, kubectl, kind + bin/caretaker
./scripts/start-registry.sh    # terminal 1: zot :5000
./scripts/start-buildkit.sh    # terminal 2: buildkitd :12345
./scripts/push-images.sh       # build + push the six images at 1.0
./scripts/kind-setup.sh        # kind cluster + Deployments + registry mirror
./bin/caretaker doctor
```

## Commands

```bash
caretaker doctor
caretaker config show
caretaker clusters
caretaker status [--preferences FILE]
caretaker lkg

caretaker patch image --service api [--source REF] [--tag TAG] [--base-digest D] [--base-version V] \
  [--report FILE] [--cve-only] [--skip-if-clean] [--write-mapping] [--force] [--dry-run] [--preferences FILE]
caretaker patch all [--services S] [--source REF] [--concurrency N] [--skip-if-clean] \
  [--cve-only --report FILE] [--write-mapping] [--force] [--dry-run]
caretaker scan --image REF --output sbom.cdx.json

caretaker lineage log [--service S | --repo R]
caretaker lineage children <image> [--service S | --repo R]   # direct + indirect descendants
caretaker lineage show <image> [--service S | --repo R]       # base/parent of one image

# base case: one cluster, one service, explicit digest (wins over prefs/index)
caretaker update --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...
caretaker update --clusters all --services all [--preferences FILE] [--map svc=image] \
  [--dry-run] [--no-verify] [--no-rollback] [--timeout 5m] [--health-cmd CMD]
caretaker rollback --clusters all --services all [--from-lkg | --image REF] [--dry-run]

caretaker mapping show [--file FILE]
caretaker mapping set --service api --base BASE --target TGT
caretaker mapping merge --file BASE --updates UPDATES --out OUT

caretaker webhook serve
caretaker webhook install --cluster kind-caretaker [--namespaces ns1,ns2] [--failure-policy Ignore|Fail]
caretaker webhook status --cluster kind-caretaker
caretaker webhook uninstall --cluster kind-caretaker
```

Patch source selection (no flags): the base (feature build) itself when it still
exists in the registry, otherwise the **oldest** patched child recorded for it in
the lineage index. Parent-side referrers are gone with a deleted base, so the
index log is what makes that fallback possible.

Copa invocation: `copa patch -i <source> -t <tag> --addr <buildkit> --platform <plat> --push`
(plus `-r <report>` when configured). The push tag is generated and unique:
`<base-version>-patched-<YYYYMMDD>-<HHMMSS>-<12-char-source-digest>`; the digest is
the identity and the tag is only a retention/UX alias.

## Deployment webhook

`caretaker webhook serve` runs an HTTPS mutating admission server on the workstation (default
`:8443`). `caretaker webhook install` registers `caretaker-deployments` with the cluster, pointing at
`https://<webhook.host>:<port>/mutate` and embedding the generated CA. On create/update it resolves
each container image to a digest (a tag is resolved against the registry first), then recognizes the
image **by that digest across every service in the preference map — independent of the container or
Deployment name** — and rewrites it to the digest-only preferred target, adding
`caretaker.io/base-digest`, `caretaker.io/base-version`, `caretaker.io/managed`, and
`caretaker.io/webhook`. Unmapped images pass through untouched. Recognition fires for any deployer
(kubectl, GitOps, caretaker), so a non-caretaker manifest that pins a known feature build is still
caught. `failurePolicy` defaults
to `Ignore`; `caretaker.io/skip-webhook: "true"` bypasses mutation. Certificates live in
`.caretaker/webhook/`. The API server must reach `webhook.host` (in this kind setup, `10.89.2.1`).

## OCI Referrers API

Lineage lives in the registry as **referrer artifacts**, not in the image manifest (changing
annotations would change the image digest) and not in a per-cluster ConfigMap (which drifts).

**Mechanics.** A referrer is an ordinary OCI artifact manifest whose `subject` field points at a
digest. The registry exposes a reverse index for it:

```http
GET /v2/<name>/referrers/<digest>?artifactType=<mediaType>
```

The response is an image index whose manifests each carry `subject`, `artifactType`, and
`annotations`. Three properties matter here:

- **It is an index, not a mutation.** Pushing referrers never touches the subject manifest, so the
  subject digest — and every signature over it — stays valid.
- **`artifactType` is an arbitrary string** and is the only server-side filter (exact match on the
  query parameter). Annotations are returned with each entry but are **not queryable**; zot's search
  extension has no annotation filter. So all filtering that matters must be done client-side after
  fetching the list.
- **Lifecycle is bound to the subject.** The reverse index keys off the subject digest: delete the
  subject and its referrers list goes with it. Untagged referrers are garbage-collection eligible
  (zot: `deleteUntagged` defaults true, `deleteReferrers` defaults false).

**Implemented design.** Referrers-on-the-child alone are not enough, because when the base image is
deleted we no longer know which children to query. So caretaker anchors a durable log:

1. Bootstrap a stable, immutable **index manifest** per image repository, tagged
   `caretaker-lineage-index`. Because it is tagged it stays live, so it survives base deletion and
   gives the log a permanent subject anchor.
2. Push **every lineage event as a referrer of that index manifest** with
   `artifactType: application/vnd.caretaker.lineage.v1+json`. Annotations carry `base-digest`,
   `parent-digest`, `source-digest`, `child-digest`, `child-repository`, `service`, `base-version`,
   `patched-at`; the full record also lives in the config blob.
3. Query: resolve the index tag → `GET /v2/<repo>/referrers/<index-digest>?artifactType=...` →
   filter by base digest client-side.

The result is an append-only, race-free log (no read-modify-write, no external DB, no per-child
tags) that answers "what patched children exist for base X?" even after X is deleted.

Also keep the fast path: `application/vnd.caretaker.lineage.metadata.v1+json` referrers on the
**child** so `lineage.BaseFor(child)` is one `Referrers` call.

**Current code.** `internal/registry` (`Referrers`/`ReferrersOfType`, `PushArtifact(subjectRef,
artifactType, annotations)`, `EnsureArtifactTag(repository, tag, artifactType, annotations)`,
`ArtifactAnnotations` via `go-containerregistry`) and `internal/lineage` (`Index`, `Record`, `Log`,
`Metadata`, `Children`, `ChildrenForBase`, `BaseFor`). Media types live in `internal/lineage`:
`MediaTypeIndex`, `MediaTypeEntry` (`application/vnd.caretaker.lineage.v1+json`), `MediaTypeMetadata`,
`MediaTypeChild`, and `IndexTag = "caretaker-lineage-index"`.

`mutate` has no `ArtifactType` helper, so the artifact's `artifactType` is carried by the **config
media type** (the fallback OCI defines), and the manifest is forced to `types.OCIManifestSchema1`
because `subject` does not exist on a Docker schema2 manifest. Reads never create the index: `Log`
returns an empty slice when the index tag is absent (`ErrNoIndex` internally), while `Record` calls
`Index` and creates it. Verified on zot: entries come back with the expected `artifactType` and
inline annotations, and the log still resolves children after the base manifest is deleted.

**Annotations in use** (`internal/lineage/lineage.go`): `caretaker.io/lineage` (`metadata` | `child`),
`caretaker.io/service`, `caretaker.io/base-digest`, `caretaker.io/base-version`,
`caretaker.io/source-digest`, `caretaker.io/parent-digest`, `caretaker.io/child-digest`,
`caretaker.io/patched-at`.

**Signing.** A signature is itself a referrer (cosign v3 and Notation default to OCI 1.1 referrers).
Signing a manifest digest covers its config and annotations; you cannot sign annotations alone.
- Sign a lineage entry directly: `cosign sign <repo>@sha256:<entry-digest>` (older cosign needs
  `--registry-referrers-mode=oci-1-1`) or `notation sign ...`.
- Sign the record *and* tie it to the image: a cosign in-toto attestation on the child with a
  lineage predicate.
- `scripts/setup.sh` installs cosign v3.1.4 into `bin/`; notation is not installed. Key-based
  cosign is what the prototype uses (`test/integration/referrers_test.go`
  `TestLineageEntrySignature`).
- cosign v3 refuses `--tlog-upload=false` alongside its default signing config. For an offline,
  tlog-free sign, pass `--signing-config <file>` where the file is
  `{"mediaType":"application/vnd.dev.sigstore.signingconfig.v0.2+json","rekorTlogConfig":{},"tsaConfig":{}}`
  (`cosign signing-config create --out sc.json` with no services emits exactly that), then verify
  with `cosign verify --key <pub> --insecure-ignore-tlog=true --insecure-ignore-sct=true`.
- A signed lineage entry becomes a referrer of the entry itself (`artifactType`
  `application/vnd.dev.sigstore.bundle.v0.3+json`), so signatures are readable through the same
  referrers path.

**Deliberately out of scope:** a signed log head / completeness proof (Rekor) for proving the registry
did not omit entries. Noted as acceptable to skip — a stale-patch anomaly view is sufficient.

## Configuration

`caretaker.yaml` holds registry, Copa/Trivy paths, BuildKit address, platform, services,
preferences path, and cluster contexts. Env overrides (highest precedence):
`CARETAKER_CONFIG`, `CARETAKER_REGISTRY_*`, `CARETAKER_KUBECONFIG`, `CARETAKER_PREFERENCES`,
`CARETAKER_SERVICES`, `CARETAKER_COPA`, `CARETAKER_TRIVY`, `CARETAKER_BUILDKIT_ADDR`,
`CARETAKER_PLATFORM`, `CARETAKER_TAG_SUFFIX`, `CARETAKER_REPORT`, and
`CARETAKER_WEBHOOK_HOST` / `_PORT` / `_CERT_DIR` / `_FAILURE_POLICY`.

## Tests

```bash
make test                      # unit tests with fakes, no network
make lint                      # gofmt + go vet
CARETAKER_TEST_CONTEXT=kind-caretaker go test -tags integration -v ./test/integration/
# CARETAKER_TEST_PATCH=1 exercises a real Copa patch (slow)
```

## Demo stack notes

- **Zot** binds `0.0.0.0:5000` (`config/zot-minimal.json`) so the kind node can reach it.
- **Registry mirror**: kind-setup adds a containerd mirror for `localhost:5000` pointing at the
  kind network gateway; without it, digest-pinned refs fail with `ImagePullBackOff`.
- **BuildKit** must run with `--network host` so it can reach `localhost:5000`.
- **Kind on podman** requires `KIND_EXPERIMENTAL_PROVIDER=podman`.
- **`pkill` stalls**: check `ss -tlnp | grep :5000` before killing a registry process.

## Related skills

- **copacetic**: Copa modes, BuildKit, targeted patching.
- **trivy**: vulnerability scans, SBOM formats and reports.
