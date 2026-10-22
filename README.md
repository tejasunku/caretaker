# caretaker

Patch container images with [Copacetic](https://github.com/project-copacetic/copacetic) and stage the patched images across Kubernetes clusters.

**Branch note:** the Go rewrite lives on `go-rewrite`. `main` still contains the original Python prototype until the rewrite reaches parity. Nothing is installed *into* a cluster — the CLI drives clusters from your workstation.

## How it works

- **Identity is a digest.** Running images are pinned as `repo:tag@sha256:...`. Tags remain for humans and for base-version lineage.
- **One universal preference map** (`config/preferences.yaml`) says which patched image a service should prefer for a given source version.
- **Per-cluster last-known-good** is recorded on each Deployment as annotations:
  - `caretaker.io/last-known-good-image` — last image that passed verification
  - `caretaker.io/previous-image` — image replaced by the last update
  - `caretaker.io/last-update-at`, `caretaker.io/managed`
- `update` seeds LKG with the running image, swaps to the digest-pinned target, waits for the rollout and optional health check, then advances LKG. On failure it **rolls back automatically**.
- `rollback` returns to `previous-image` (one step back) by default, or to the LKG with `--from-lkg`, or to an explicit `--image`.

## Setup

Prerequisites: Go 1.24+, a container runtime (podman on WSL is what this repo assumes), and `kubectl`/`kind` (installed into `bin/`).

```bash
export PATH="$PWD/bin:$PATH"

make setup                 # installs copa, trivy, zot, kind, kubectl; builds bin/caretaker
./scripts/start-registry.sh    # terminal 1: zot on localhost:5000
./scripts/start-buildkit.sh    # terminal 2: buildkitd on tcp://127.0.0.1:12345
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

## Commands

```bash
caretaker config show                 # effective config (password redacted)
caretaker doctor                      # tools, registry, cluster connectivity
caretaker clusters                    # configured clusters + live deployment counts
caretaker status                      # running image, LKG, preferred patch, stale/ok
caretaker lkg                         # per-cluster last-known-good + previous

caretaker patch image --service api   # patch newest base tag with Copa
caretaker patch image --service api --source localhost:5000/test/api:1.0 --tag 1.0-patched-20261022
caretaker patch all --write-mapping   # patch every service and record preferences
caretaker scan --image localhost:5000/test/api:1.0 --output sbom.cdx.json

caretaker update --clusters kind-caretaker --services api
caretaker update --clusters all --services all --map api=localhost:5000/test/api:1.0-patched-20261022
caretaker update --services api --health-cmd 'curl -sf http://api.caretaker.svc/healthz'
caretaker update --services api --dry-run

caretaker rollback --clusters kind-caretaker --services api     # one step back
caretaker rollback --services api --from-lkg                    # to recorded LKG
caretaker rollback --services api --image localhost:5000/test/api:1.0-patched-20261016

caretaker mapping show
caretaker mapping set --service api --source localhost:5000/test/api:1.0 --target localhost:5000/test/api:1.0-patched-20261022
caretaker mapping merge --file config/preferences.yaml --updates updates.yaml --out merged.yaml
```

`update` accepts either a preferences file (`--preferences`, defaults to `preferences` in `caretaker.yaml`) or ad-hoc strings (`--map service=image`, repeatable).

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

preferences: config/preferences.yaml

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

## Preference map

```yaml
apiVersion: caretaker/v1alpha1
kind: PatchPreferences
services:
  api:
    mappings:
      - source: localhost:5000/test/api:1.0
        target: localhost:5000/test/api:1.0-patched-20261022@sha256:...
```

Lookup order: exact digest, exact tag, then numeric base version. The base-version fallback lets a preference for `1.0` apply to any `1.0-patched-*` build, so clusters on different patch dates converge on the same target.

## Layout

```
cmd/caretaker          entry point
internal/cli           cobra commands (flag parsing + printing only)
internal/config        YAML + env precedence
internal/image         reference parsing, version/tag helpers
internal/registry      OCI client (go-containerregistry)
internal/kube          client-go Cluster interface
internal/catalog       preference map: lookup, merge, save
internal/patch         Copa/Trivy orchestration
internal/update        plan -> swap -> verify -> LKG -> rollback
internal/health        rollout wait + optional health command
internal/runner        exec abstraction
```

## Tests

```bash
make test    # unit tests with fakes; no network, no cluster
make lint    # gofmt + go vet
```

Covered: reference parsing and version ordering, preference lookup/merge (including base-version matching), Copa argument construction, and the update state machine (LKG seeding, verification failure, auto-rollback, explicit mapping, dry run).

Live integration tests (require the registry and kind cluster from Setup):

```bash
CARETAKER_TEST_CONTEXT=kind-caretaker go test -tags integration -v ./test/integration/
# add CARETAKER_TEST_PATCH=1 to exercise a real Copa patch (slow)
```

The update tests mutate one Deployment and restore it on completion.
