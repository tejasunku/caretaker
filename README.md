# caretaker

Patch container images with [Copacetic](https://github.com/project-copacetic/copacetic) and stage the patched images across Kubernetes clusters.

**Branch note:** the Go rewrite lives on `go-rewrite`. `main` still contains the original Python prototype until the rewrite reaches parity. Nothing is installed *into* a cluster — the CLI drives clusters from your workstation.

## How it works

- **Identity is a digest.** Running images are pinned as `repo@sha256:...`, preferences store digests, and tags are never assumed to exist. Tag input is opt-in with `--allow-tags` (or `allowTags: true`) and is resolved to a digest before use.
- **One universal preference map** (`config/preferences.yaml`) says which patched image a service should prefer for a given source digest, with `baseDigest`/`baseVersion` lineage.
- **Patched images carry lineage referrers.** Each patched child records `baseDigest`, `sourceDigest`, `parentDigest`, and `patchedAt`; parents record child pointers. This is how parent fallback works when an old image was garbage-collected.
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

make setup                 # installs copa, trivy, zot, kind, kubectl; builds bin/caretaker
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
caretaker doctor                      # tools, registry, cluster connectivity
caretaker clusters                    # configured clusters + live deployment counts
caretaker status                      # running image, LKG, preferred patch, stale/ok
caretaker lkg                         # per-cluster last-known-good + previous

caretaker patch image --service api   # resolve source from config/preferences
caretaker patch image --service api --source localhost:5000/test/api@sha256:04e9733f...
caretaker patch image --service api --source localhost:5000/test/api:1.0 --allow-tags
caretaker patch image --service api --parent-policy oldest-child   # parent was GC'd
caretaker patch image --service api --cve-only --report trivy.json # Copa CVE-only mode
caretaker patch all --write-mapping   # patch every service and record preferences
caretaker patch all --skip-if-clean --concurrency 4 --write-mapping
caretaker scan --image localhost:5000/test/api@sha256:04e9733f... --output sbom.cdx.json

caretaker update --clusters kind-caretaker --services api
caretaker update --clusters all --services all --map api=localhost:5000/test/api@sha256:b5bc47de...
caretaker update --services api --health-cmd 'curl -sf http://api.caretaker.svc/healthz'
caretaker update --services api --dry-run

caretaker rollback --clusters kind-caretaker --services api     # one step back
caretaker rollback --services api --from-lkg                    # to recorded LKG
caretaker rollback --services api --image localhost:5000/test/api@sha256:cccccccc...

caretaker mapping show
caretaker mapping set --service api --source localhost:5000/test/api@sha256:04e9733f... --target localhost:5000/test/api@sha256:b5bc47de...
caretaker mapping set --service api --source localhost:5000/test/api:1.0 --target localhost:5000/test/api:1.0-patched-20261022 --allow-tags
caretaker mapping merge --file config/preferences.yaml --updates updates.yaml --out merged.yaml

caretaker webhook serve                 # run the HTTPS admission server
caretaker webhook install --cluster kind-caretaker
caretaker webhook status --cluster kind-caretaker
caretaker webhook uninstall --cluster kind-caretaker
```

`update` accepts either a preferences file (`--preferences`, defaults to `preferences` in `caretaker.yaml`) or ad-hoc strings (`--map service=image`, repeatable; digests by default, tags with `--allow-tags`).

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
  parentPolicy: prefer-parent   # prefer-parent | oldest-child | newest-child | none
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
| `CARETAKER_PARENT_POLICY` | `prefer-parent` (default), `oldest-child`, `newest-child`, `none` |
| `CARETAKER_SKIP_IF_CLEAN` | scan with Trivy and skip images with no vulnerabilities |
| `CARETAKER_PATCH_CONCURRENCY` | images patched at once by `patch all` (default 1) |
| `CARETAKER_WEBHOOK_HOST` / `_PORT` / `_CERT_DIR` / `_FAILURE_POLICY` | webhook registration |

## Preference map

```yaml
apiVersion: caretaker/v1alpha1
kind: PatchPreferences
services:
  api:
    mappings:
      - source: localhost:5000/test/api@sha256:04e9733f...
        target: localhost:5000/test/api@sha256:b5bc47de...
        baseDigest: sha256:04e9733f...
        baseVersion: "1.0"
        patchedAt: "2026-10-22T12:00:00Z"
```

Lookup is digest-only: caretaker tries the running image's digest, then the
mapping that produced it, then the recorded `caretaker.io/base-digest`. A
mapping's `baseDigest`/`baseVersion` scope it to the lineage that produced the
child. Tag-only mappings are ignored unless `--allow-tags` is set; `mapping set`
and `--map` reject tag refs without it and canonicalize everything to digests.

## Deployment webhook

`caretaker webhook serve` runs an HTTPS mutating admission server on the workstation. `caretaker webhook install --cluster kind-caretaker` creates or updates a `MutatingWebhookConfiguration` named `caretaker-deployments`, pointing at `https://<webhook.host>:<port>/mutate` with a generated CA bundle.

On Deployment create/update it:
1. Resolves each container image against the preference map (exact digest, mapping source, then `caretaker.io/base-digest`).
2. Rewrites the image to the digest-only target.
3. Adds `caretaker.io/base-digest`, `caretaker.io/base-version`, `caretaker.io/managed`, and `caretaker.io/webhook`.

Tag resolution on the base version runs only with `--allow-tags`, because tags may be absent or stale.

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
