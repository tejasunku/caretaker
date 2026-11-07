# caretaker

Patch container images with [Copacetic](https://github.com/project-copacetic/copacetic) and stage the patched images across Kubernetes clusters.

**Branch note:** the Go rewrite lives on `main` (formerly `go-rewrite`). The CLI drives clusters from your workstation; the only in-cluster options are the optional admission mechanisms (`caretaker webhook deploy` or the serverless `caretaker webhook map install`). See `docs/walkthrough.md` for the full guided tour, including everything you must do by hand.

## How it works

- **Identity is a digest.** Running images are pinned as `repo@sha256:...`, preferences store digests, and tags are never assumed to exist. Tag input is opt-in with `--allow-tags` (or `allowTags: true`) and is resolved to a digest before use.
- **One universal preference map** (`config/preferences.yaml`) says which patched image a service should prefer for a given **feature build** (base) digest. Each service's mappings are keyed by the base digest; the target is the preferred patched digest.
- **Patched images carry lineage referrers.** Each patched child records `baseDigest`, `sourceDigest`, `parentDigest`, and `patchedAt`; parents record child pointers. This is how parent fallback works when an old image was garbage-collected.
- **Nomenclature** (same words in code, annotations, and docs): the **base** — a.k.a. the **feature build** — is the image built by a service's repo and is the unpatched root of a chain; the **parent** (a.k.a. source) is the immediate image given to Copa, equal to the base unless we are patching a patch; the **child** is the image Copa produced; a **descendant** is any child of a base, direct or indirect (`lineage children` reports the depth).
- **Every repository has a lineage index** (`caretaker-lineage-index` tag) whose referrers are the append-only log of every patch recorded there. It outlives the base image, so "what was patched from this digest" stays answerable after pruning. Read it with `caretaker lineage log|children|show`.
- **Copa's build backend is pluggable.** `patch.buildkitAddr` is passed to Copa as `--addr`. Besides a local Docker/Podman/BuildKit daemon, Copa accepts `kubepod://<pod>?context=<ctx>&namespace=<ns>` to use BuildKit running in a Kubernetes pod; caretaker still runs on the workstation.
- **Per-cluster last-known-good** is recorded on each Deployment as annotations:
  - `caretaker.io/last-known-good-image` — last image that passed verification
  - `caretaker.io/previous-image` — image replaced by the last update
  - `caretaker.io/base-digest` — digest of the unpatched base image
  - `caretaker.io/base-version` — human-readable version such as `1.0`
  - `caretaker.io/last-update-at`, `caretaker.io/managed`
- `set-target` (alias: `update`) seeds LKG with the running image, swaps to the digest-pinned target, waits for the rollout and optional health check, then advances LKG. On failure it **falls back automatically**; `--no-fallback` leaves the new image in place. `--force` bypasses admission.
- `restore` (alias: `rollback`) is the break-glass path: by default it returns to `previous-image` (one step back), or to the LKG with `--lkg`, or to an explicit `--image`. It sets `caretaker.io/skip-webhook=true` before the swap so the admission webhook cannot immediately undo it. To deliberately choose an older image as desired state, use `set-target` instead.
- **Optional Deployment webhook**: a mutating admission webhook rewrites container images to the preference-map target on create/update. It runs **only inside the cluster** (`caretaker webhook deploy`) as a Deployment/Service fed by a preferences ConfigMap — admission never calls back to your workstation. On Kubernetes ≥ 1.36 there is also a serverless alternative: `caretaker webhook map install` installs a MutatingAdmissionPolicy + ConfigMap parameter.

## Setup

Prerequisites: Go 1.24+, a container runtime (podman on WSL is what this repo assumes), and `kubectl`/`kind` (installed into `bin/`). The full list of dependencies — and every step that is yours alone to perform — is in `docs/walkthrough.md`.

```bash
export PATH="$PWD/bin:$PATH"

make setup                 # installs copa, trivy, zot, kind, kubectl, cosign; builds bin/caretaker
./scripts/start-registry.sh    # terminal 1: zot on localhost:5000
./scripts/start-buildkit.sh    # terminal 2: buildkitd on tcp://127.0.0.1:12345
./scripts/push-images.sh       # build + push the six demo images at tag 1.0
./scripts/kind-setup.sh        # once: creates the kind cluster + 6 Deployments
./bin/caretaker doctor
```

`kind-setup.sh` also configures the node's containerd mirror so `localhost:5000` resolves from inside the kind node.

`make setup` expects the WSL podman socket environment used by the existing scripts:

```bash
export DOCKER_HOST="unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock"
export CONTAINER_HOST="$DOCKER_HOST"
export KIND_EXPERIMENTAL_PROVIDER=podman
```

### Demo assets

- `dockerfiles/<service>.Dockerfile` + `dockerfiles/content/*.txt` — six tiny, distinct images.
- `scripts/push-images.sh` — builds and pushes them at tag `1.0`.
- `scripts/kind-setup.sh` — creates the cluster, loads images, and applies the Deployments.
- `patch-manifests/*.json` — sample Copa reports for targeted patching (`--report`).

## Commands

```bash
caretaker config show                 # effective config (password redacted)
caretaker doctor                      # tools, registry, mapping targets, per-deployment resolution
caretaker clusters                    # configured clusters + K8S version + live deployment counts
caretaker status                      # running image, LKG, preferred patch, stale/ok
caretaker lkg                         # per-cluster last-known-good + previous

caretaker patch image --service api   # resolve source from config/preferences
caretaker patch image --service api --source localhost:5000/test/api@sha256:04e9733f...
caretaker patch image --service api --source localhost:5000/test/api:1.0 --allow-tags
caretaker patch image --service api --cve-only --report trivy.json # Copa CVE-only mode
caretaker patch all --write-mapping   # patch every service and record preferences
caretaker patch all --skip-if-clean --concurrency 4 --write-mapping
caretaker scan --image localhost:5000/test/api@sha256:04e9733f... --output sbom.cdx.json

caretaker lineage log --service api    # every patch recorded for a repository
caretaker lineage children localhost:5000/test/api@sha256:04e9733f...  # direct + indirect children
caretaker lineage show localhost:5000/test/api@sha256:33e60967...      # base/parent of one image

caretaker set-target --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...  # base case: one cluster, one service, explicit digest
caretaker set-target --clusters all --services all      # no --map: preference map, then lineage index
caretaker set-target --services api --health-cmd 'curl -sf http://api.caretaker.svc/healthz'
caretaker set-target --services api --dry-run
caretaker set-target --services api --force             # bypass admission

caretaker restore --clusters kind-caretaker --services api      # break-glass: one step back
caretaker restore --services api --lkg                          # restore recorded LKG
caretaker restore --services api --image localhost:5000/test/api@sha256:cccccccc...

caretaker mapping show
caretaker mapping set --service api --base localhost:5000/test/api@sha256:04e9733f... --target localhost:5000/test/api@sha256:b5bc47de...
caretaker mapping set --service api --base localhost:5000/test/api@sha256:04e9733f... --target localhost:5000/test/api@sha256:b5bc47de... --alias localhost:5000/test/api@sha256:9f2c1a... --base-version 1.0
caretaker mapping set --service api --base localhost:5000/test/api:1.0 --target localhost:5000/test/api:1.0-patched-20261022 --allow-tags
caretaker mapping merge --file config/preferences.yaml --updates updates.yaml --out merged.yaml
caretaker mapping push [--file config/preferences.yaml] [--sign --key .caretaker/cosign.key]
caretaker mapping pull --digest sha256:... [--file config/preferences.yaml] [--verify --pub-key .caretaker/cosign.pub]

caretaker webhook deploy --cluster kind-caretaker \
  --image localhost:5000/test/caretaker-webhook@sha256:5b05ecfc...
caretaker webhook prefs push --cluster kind-caretaker         # write prefs ConfigMap
caretaker webhook status --cluster kind-caretaker             # endpoint + K8S version + MAP availability
caretaker webhook undeploy --cluster kind-caretaker           # remove everything it created

caretaker webhook map install --cluster kind-caretaker         # MutatingAdmissionPolicy (K8s >= 1.36)
caretaker webhook map prefs push --cluster kind-caretaker      # update the policy's ConfigMap
caretaker webhook map status --cluster kind-caretaker
caretaker webhook map uninstall --cluster kind-caretaker
```

`patch` uses the base (feature build) itself whenever it still exists in the
registry; if the base has been deleted it falls back to the **oldest** surviving
patched child found in the lineage index. One behavior, no flags.

`set-target` (alias `update`) accepts either a preferences file (`--preferences`, defaults to `preferences` in `caretaker.yaml`) or ad-hoc strings (`--map service=image`, repeatable; digests by default, tags with `--allow-tags`).

Target selection is digest-only and never lists tags: `--map`, then the
preference map, then the per-service `newest` policy, then the lineage index
(the newest patched child of the deployment's base that still exists in the
registry). The base comes from `caretaker.io/base-digest`, the running image's
own lineage referrer, or the running digest itself, so set-target still finds a
target after the base manifest has been deleted.

### Setting one cluster's target (base case)

The most common deployment is pinning one service in one cluster to an image
named by digest in the command:

```bash
caretaker set-target --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...
```

`--map` is repeatable, so several services can be pinned in one command. The
given digest wins over the preference map and the lineage index, but the rest of
the flow is unchanged: LKG is seeded, the image is swapped as `repo@digest`,
the rollout is waited on, LKG advances on success, and a failed verification
falls back to the LKG automatically. `--dry-run` previews the swap,
`--no-fallback` keeps a failed image in place, `--force` bypasses admission, and
`--health-cmd` adds a post-rollout check.

`restore` is not a target-setting command: it is the break-glass path back to
`previous-image`, the recorded LKG, or an explicit image, and it bypasses
admission. If you deliberately want an older image as desired state, use
`set-target` with that digest.

## Configuration

`caretaker.yaml` (see the repo root):

```yaml
registry:
  url: localhost:5000
  namespace: test
  insecure: true

patch:
  copa: ./bin/copa
  trivy: ./bin/trivy
  buildkitAddr: tcp://127.0.0.1:12345
  platform: linux/amd64
  tagSuffix: patched
  skipIfClean: false            # scan with Trivy and skip clean images
  concurrency: 1                # images patched at once by "patch all"
  sources:                      # optional explicit source digest per service
    api: localhost:5000/test/api@sha256:04e9733f...

allowTags: false                # tag input is opt-in; digest is the default
preferences: config/preferences.yaml

webhook:
  port: 8443            # container listen port (Service maps 443 -> 8443)
  certDir: .caretaker/webhook  # where the CLI writes the generated certs before sealing them in a Secret
  failurePolicy: Ignore # or Fail to block unmutated deployments

services: [api, auth, cache, payments, web, worker]

clusters:
  - id: kind-caretaker
    context: kind-caretaker
    namespace: caretaker
    stage: production
```

Environment overrides (highest precedence):

| Variable | Purpose |
|---|---|
| `CARETAKER_CONFIG` | config file path |
| `CARETAKER_REGISTRY_URL` / `_NAMESPACE` / `_INSECURE` | registry |
| `CARETAKER_REGISTRY_USERNAME` / `_PASSWORD` | registry auth |
| `CARETAKER_KUBECONFIG` | kubeconfig path |
| `CARETAKER_PREFERENCES` | preferences file |
| `CARETAKER_SERVICES` | comma-separated service list |
| `CARETAKER_COPA` / `CARETAKER_TRIVY` | tool paths |
| `CARETAKER_BUILDKIT_ADDR` / `CARETAKER_PLATFORM` | Copa settings |
| `CARETAKER_TAG_SUFFIX` / `CARETAKER_REPORT` | tag suffix and optional Trivy report |
| `CARETAKER_ALLOW_TAGS` | opt in to tag input/lookup (default false) |
| `CARETAKER_SKIP_IF_CLEAN` | scan with Trivy and skip images with no vulnerabilities |
| `CARETAKER_PATCH_CONCURRENCY` | images patched at once by `patch all` (default 1) |
| `CARETAKER_WEBHOOK_PORT` / `_CERT_DIR` / `_FAILURE_POLICY` | in-cluster webhook |

## Preference map

Per service, a flat list of **feature build digest → preferred patched digest**:

```yaml
apiVersion: caretaker/v1alpha2
kind: PatchPreferences
services:
  api:
    mappings:
      - base: localhost:5000/test/api@sha256:04e9733f...    # feature build (base) digest
        aliases:                                             # optional: older patched digests of the same feature set
          - localhost:5000/test/api@sha256:9f2c1a...
        target: localhost:5000/test/api@sha256:33e60967...   # preferred patched digest
        baseVersion: "1.0"
        patchedAt: "2026-10-29T03:02:52Z"
```

One row per feature build; later rows for a new base version coexist with older
ones. The optional `aliases` list holds other digests — typically older patched
versions of the same feature set — that should also resolve to `target`. Because
tags aren't assumed to exist, this is how you group a feature set by digest: a
Deployment that pins an older patched image listed as an alias is still
recognized by admission and moved to the newest target. Lookup is digest-only: the running
image is first tried as a feature build (or alias) itself, then looked up as a
mapping target to recover its base, then the recorded `caretaker.io/base-digest`
is used. `--write-mapping` after a patch
and `mapping set --base ... --target ...` both key the row by the base digest;
tags are accepted as input only with `--allow-tags` and are resolved to digests
before storing. `mapping set` only overwrites the fields you pass — `--note`,
`--base-version`, and `--alias` (repeatable; replaces the alias list) are
preserved when omitted, so a plain base/target update cannot drop them (a
changed target does clear the stale `patchedAt` timestamp). Images with no
mapping are ignored by lookups (the set-target path
then falls through to the lineage index, and the webhook passes them through).

### Registry-stored preferences

`mapping push` stores the whole preferences document in the OCI registry as one
immutable artifact version and prints its digest:

```bash
caretaker mapping push --sign
# pushed localhost:5000/test/caretaker-preferences@sha256:0dbf8638...
# signed localhost:5000/test/caretaker-preferences@sha256:0dbf8638...
```

The document is encoded as a single JSON blob in the artifact's layer —
deliberately not as a set of references to images. The digests named inside it
are plain strings, so the stored copy survives even after every image it
mentions is deleted from the registry. Each version is a referrer of a tagged,
immutable index manifest (`caretaker-preferences-index`, in the
`caretaker-preferences` repository), mirroring the lineage index, so the history
is append-only and discoverable through the Referrers API. Every version carries
`caretaker.io/set-at` (RFC3339Nano) and `caretaker.io/writer` annotations so
versions can be ordered and attributed without downloading their blobs.

`mapping pull --digest ...` downloads one version and writes it back to the
preferences file; `--verify` checks its cosign signature first:

```bash
caretaker mapping pull --digest sha256:0dbf8638... --verify
```

Signing is opt-in. `--sign` uses cosign with `.caretaker/cosign.key`
(generate one with `cosign generate-key-pair --output-key-prefix .caretaker/cosign`),
passes a no-services signing config so no transparency log is needed, and the
signature lands as a referrer of the version artifact. Verification uses
`--pub-key`. The local file is still what `set-target`, `status`, `patch`, and
the webhook read; pushing to and pulling from the registry is an explicit sync
step for now. Lineage and preferences stay separate — cross-checking them is
left to `doctor` later.

## Deployment webhook

`caretaker webhook deploy` installs the mutating admission webhook *inside* the
cluster; admission never calls back to the host. It creates, in namespace
`caretaker-system` by default:

| Resource | Purpose |
|---|---|
| Deployment `caretaker-webhook` | 2 replicas of the webhook binary, HTTPS readiness probe |
| Service `caretaker-webhook` | 443 → 8443, the endpoint the API server calls |
| Secret `caretaker-webhook` | `tls.crt` / `tls.key` / `ca.crt` (mounted read-only at `/certs`) |
| ConfigMap `caretaker-preferences` | the preferences document, mounted at `/etc/caretaker/preferences.yaml` |
| MutatingWebhookConfiguration | service-mode registration with the CA bundle |

On Deployment create/update it:
1. Resolves each container image to a digest — a tag is resolved against the registry first, so `repo:1.0` and `repo@sha256:...` are treated identically.
2. Recognizes the image **by its digest across every service in the preference map**, independent of the container or Deployment name: if the digest is a feature build, it is rewritten to that build's preferred patched target; if it is already a preferred target, it is left in place.
3. Rewrites the image to the digest-only target and adds `caretaker.io/base-digest`, `caretaker.io/base-version`, `caretaker.io/managed`, and `caretaker.io/webhook`.

**Images with no mapping pass through untouched** — the webhook only rewrites
containers whose image digest (or recorded base) appears in the preference map;
everything else is admitted exactly as submitted, with no annotations added.
Recognition is keyed by digest, not by the container or Deployment name, so any
manifest that pins a known feature-build image is caught no matter what it is
called or whether the deployer is using caretaker.

A container image that pins a tag is resolved to its digest before recognition,
so a tag-specified base is rewritten just like a digest-specified one. The
legacy `<version>-patched-<date>` tag heuristic (inferring the base version from
a patched tag) still runs only with `--allow-tags`, because those tags may be
absent or stale.

Set `caretaker.io/skip-webhook: "true"` on a Deployment to bypass it. The default `failurePolicy` is `Ignore` so a stopped webhook never blocks rollouts; use `--failure-policy Fail` when you want enforcement.

Build the image with `./scripts/build-webhook.sh` (it prints the digest-pinned ref — always deploy by digest, never by tag). Push the preferences into the cluster with `caretaker webhook prefs push --cluster kind-caretaker` (the mounted ConfigMap refreshes in pods within about a minute). `caretaker webhook status` shows the endpoint (`service://...`), the cluster's Kubernetes version, and whether `MutatingAdmissionPolicy` is available as an alternative (Kubernetes ≥ 1.36). `caretaker webhook undeploy` removes everything it created.

Gotchas: in-kind pods cannot reach `localhost:5000` — pass `--registry-url 10.89.2.1:5000 --registry-insecure` so the webhook can resolve images; the `/certs` mount is read-only, so the Secret must include `ca.crt`.

### MutatingAdmissionPolicy mode (alpha, Kubernetes 1.36+)

`caretaker webhook map install --cluster kind-caretaker` is an experimental, serverless alternative to the webhook. It installs a cluster-scoped `MutatingAdmissionPolicy` + `MutatingAdmissionPolicyBinding` and a `caretaker-image-map` ConfigMap parameter in `caretaker-system`. Kubernetes evaluates the policy's CEL expression at admission time and rewrites mapped digests in both `containers` and `initContainers`; unmapped images pass through untouched.

```bash
caretaker webhook map install --cluster kind-caretaker --preferences config/preferences.yaml
caretaker webhook map prefs push --cluster kind-caretaker   # update the digest map live
caretaker webhook map status --cluster kind-caretaker       # policy / binding / configmap / mapping count
caretaker webhook map uninstall --cluster kind-caretaker
```

No image, Service, Secret, certificate, or host process is involved. The ConfigMap is keyed by a sanitized `<repo>_<digest>` string (ConfigMap keys cannot contain `:`, `@`, or `/`) and values are digest-pinned target refs. The binding uses `parameterNotFoundAction: Allow` and `reinvocationPolicy: IfNeeded`; `failurePolicy` defaults to `Ignore`.

> **Alpha — not recommended as the primary admission mechanism.** MAP works for steady-state rewriting, but it has no per-Deployment escape hatch: it ignores `caretaker.io/skip-webhook`. A manual `restore` or an automatic fallback to a mapped base or alias can therefore be rewritten straight back to the preferred target. Use the in-cluster webhook when you need a reliable break-glass path.

Known quirks:

- Ignores `caretaker.io/skip-webhook` (the webhook honors it).
- Adds no `caretaker.io/*` annotations; only the image is rewritten.
- Preferences live in a separate ConfigMap. After `patch --write-mapping` or `mapping set`, run `webhook map prefs push` before `set-target`, or admission can rewrite the new target back to the stale served target. `doctor` warns when the map is stale.
- The policy is cluster-wide; there is no per-service opt-out beyond its namespace binding.
- Requires Kubernetes ≥ 1.36. Do not run MAP and the in-cluster webhook at the same time.

`mapping set` and `--map` accept tags as input only with `--allow-tags`, and resolve them to digests before storing anything.

## Layout

```
cmd/caretaker          entry point
internal/cli           cobra commands (flag parsing + printing only)
internal/config        YAML + env precedence
internal/image         reference parsing, version/tag helpers
internal/registry      OCI client (go-containerregistry)
internal/kube          client-go Cluster interface
internal/catalog       preference map: digest-first lookup, merge, save
internal/prefs         registry-stored preference versions: push, pull, sign
internal/lineage       OCI referrer lineage: parent/child metadata
internal/patch         Copa/Trivy orchestration
internal/update        plan -> swap -> verify -> LKG -> fallback/restore
internal/health        rollout wait + optional health command
internal/webhook       admission webhook: mutation, TLS certs, install/deploy/status
internal/runner        exec abstraction
Dockerfile.webhook     in-cluster webhook image (distroless)
docs/walkthrough.md    guided tour: dependencies, manual actions, full lifecycle
dockerfiles/           demo images (one per service, unique content)
patch-manifests/       sample Copa reports for --report
scripts/               registry, BuildKit, kind, and image setup
test/integration/      live set-target/restore/patch tests
```

## Tests

```bash
make test    # unit tests with fakes; no network, no cluster
make lint    # gofmt + go vet
```

Covered: reference parsing and version ordering, digest preference lookup/merge, Copa argument construction, the set-target state machine (LKG seeding, automatic fallback, forced targets, explicit mapping, dry run), restore, webhook mutation, and certificate generation.

Live integration tests (require the registry and kind cluster from Setup):

```bash
CARETAKER_TEST_CONTEXT=kind-caretaker go test -tags integration -v ./test/integration/
# add CARETAKER_TEST_PATCH=1 to exercise a real Copa patch (slow)
```

The update tests mutate one Deployment and restore it on completion. Restore tests cover break-glass toggling.
