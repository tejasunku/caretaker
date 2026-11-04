# caretaker

Patch container images with [Copacetic](https://github.com/project-copacetic/copacetic) and stage the patched images across Kubernetes clusters.

**Branch note:** the Go rewrite lives on `go-rewrite`. `main` still contains the original Python prototype until the rewrite reaches parity. Nothing is installed *into* a cluster — the CLI drives clusters from your workstation.

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
- `update` seeds LKG with the running image, swaps to the digest-pinned target, waits for the rollout and optional health check, then advances LKG. On failure it **rolls back automatically**.
- `rollback` returns to `previous-image` (one step back) by default, or to the LKG with `--from-lkg`, or to an explicit `--image`.
- **Optional Deployment webhook**: a mutating admission webhook rewrites container images to the preference-map target on create/update. It runs on your workstation and is registered with the cluster by the CLI.

## Setup

Prerequisites: Go 1.24+, a container runtime (podman on WSL is what this repo assumes), and `kubectl`/`kind` (installed into `bin/`).

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
caretaker clusters                    # configured clusters + live deployment counts
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

caretaker update --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...  # base case: one cluster, one service, explicit digest
caretaker update --clusters all --services all          # no --map: preference map, then lineage index
caretaker update --services api --health-cmd 'curl -sf http://api.caretaker.svc/healthz'
caretaker update --services api --dry-run

caretaker rollback --clusters kind-caretaker --services api     # one step back
caretaker rollback --services api --from-lkg                    # to recorded LKG
caretaker rollback --services api --image localhost:5000/test/api@sha256:cccccccc...

caretaker mapping show
caretaker mapping set --service api --base localhost:5000/test/api@sha256:04e9733f... --target localhost:5000/test/api@sha256:b5bc47de...
caretaker mapping set --service api --base localhost:5000/test/api@sha256:04e9733f... --target localhost:5000/test/api@sha256:b5bc47de... --alias localhost:5000/test/api@sha256:9f2c1a... --base-version 1.0
caretaker mapping set --service api --base localhost:5000/test/api:1.0 --target localhost:5000/test/api:1.0-patched-20261022 --allow-tags
caretaker mapping merge --file config/preferences.yaml --updates updates.yaml --out merged.yaml

caretaker webhook serve                 # run the HTTPS admission server
caretaker webhook install --cluster kind-caretaker
caretaker webhook status --cluster kind-caretaker
caretaker webhook uninstall --cluster kind-caretaker
```

`patch` uses the base (feature build) itself whenever it still exists in the
registry; if the base has been deleted it falls back to the **oldest** surviving
patched child found in the lineage index. One behavior, no flags.

`update` accepts either a preferences file (`--preferences`, defaults to `preferences` in `caretaker.yaml`) or ad-hoc strings (`--map service=image`, repeatable; digests by default, tags with `--allow-tags`).

Target selection is digest-only and never lists tags: `--map`, then the
preference map, then the per-service `newest` policy, then the lineage index
(the newest patched child of the deployment's base that still exists in the
registry). The base comes from `caretaker.io/base-digest`, the running image's
own lineage referrer, or the running digest itself, so an update still finds a
target after the base manifest has been deleted.

### Updating one cluster's service (base case)

The most common deployment is pinning one service in one cluster to an image
named by digest in the command:

```bash
caretaker update --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...
```

`--map` is repeatable, so several services can be pinned in one command. The
given digest wins over the preference map and the lineage index, but the rest of
the flow is unchanged: LKG is seeded, the image is swapped as `repo@digest`,
the rollout is waited on, LKG advances on success, and a failed verification
rolls back automatically. `--dry-run` previews the swap, `--no-rollback` keeps
a failed image in place, and `--health-cmd` adds a post-rollout check.

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
  host: 10.89.2.1       # how the API server reaches the workstation
  port: 8443
  certDir: .caretaker/webhook
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
| `CARETAKER_WEBHOOK_HOST` / `_PORT` / `_CERT_DIR` / `_FAILURE_POLICY` | webhook registration |

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
rollback that deploys an older patched image listed as an alias is still
recognized and bumped to the newest target. Lookup is digest-only: the running
image is first tried as a feature build (or alias) itself, then looked up as a
mapping target to recover its base, then the recorded `caretaker.io/base-digest`
is used. `--write-mapping` after a patch
and `mapping set --base ... --target ...` both key the row by the base digest;
tags are accepted as input only with `--allow-tags` and are resolved to digests
before storing. `mapping set` only overwrites the fields you pass — `--note`,
`--base-version`, and `--alias` (repeatable; replaces the alias list) are
preserved when omitted, so a plain base/target update cannot drop them (a
changed target does clear the stale `patchedAt` timestamp). Images with no
mapping are ignored by lookups (the update path
then falls through to the lineage index, and the webhook passes them through).

## Deployment webhook

`caretaker webhook serve` runs an HTTPS mutating admission server on the workstation. `caretaker webhook install --cluster kind-caretaker` creates or updates a `MutatingWebhookConfiguration` named `caretaker-deployments`, pointing at `https://<webhook.host>:<port>/mutate` with a generated CA bundle.

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

Set `caretaker.io/skip-webhook: "true"` on a Deployment to bypass it. The default `failurePolicy` is `Ignore` so a stopped webhook never blocks rollouts; use `--failure-policy Fail` when you want enforcement. Certificates are written to `.caretaker/webhook/` (gitignored) and reused unless `--force-certs` is passed.

Because the API server calls back to the host, `webhook.host` must be reachable from the cluster. In this kind setup the host is `10.89.2.1` (the kind network gateway, same address used for the registry mirror).

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
internal/lineage       OCI referrer lineage: parent/child metadata
internal/patch         Copa/Trivy orchestration
internal/update        plan -> swap -> verify -> LKG -> rollback
internal/health        rollout wait + optional health command
internal/webhook       admission webhook: mutation, TLS certs, install/status
internal/runner        exec abstraction
dockerfiles/           demo images (one per service, unique content)
patch-manifests/       sample Copa reports for --report
scripts/               registry, BuildKit, kind, and image setup
test/integration/      live update/rollback/patch tests
```

## Tests

```bash
make test    # unit tests with fakes; no network, no cluster
make lint    # gofmt + go vet
```

Covered: reference parsing and version ordering, digest preference lookup/merge, Copa argument construction, the update state machine (LKG seeding, verification failure, auto-rollback, explicit mapping, dry run), webhook mutation, and certificate generation.

Live integration tests (require the registry and kind cluster from Setup):

```bash
CARETAKER_TEST_CONTEXT=kind-caretaker go test -tags integration -v ./test/integration/
# add CARETAKER_TEST_PATCH=1 to exercise a real Copa patch (slow)
```

The update tests mutate one Deployment and restore it on completion.
