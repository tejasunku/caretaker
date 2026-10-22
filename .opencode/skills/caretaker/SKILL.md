---
name: caretaker
description: Overview of caretaker — a Go CLI that patches container images with Copacetic and stages digest-pinned updates across Kubernetes clusters. Covers the preference map, per-cluster last-known-good annotations, the update/verify/rollback flow, configuration/env vars, and the demo kind + zot + BuildKit stack.
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

## Data model

- **Identity is a digest.** Running images are `repo@sha256:...`; tags are labels.
- **Universal preference map** — `config/preferences.yaml`:
  ```yaml
  apiVersion: caretaker/v1alpha1
  kind: PatchPreferences
  services:
    api:
      policy: pinned          # or newest
      mappings:
        - source: localhost:5000/test/api@sha256:04e9733f...
          target: localhost:5000/test/api@sha256:b5bc47de...
  ```
  Lookup is digest-only: the running image digest first, then the digest of its base version
  (`repo:1.0`). The base version comes from `caretaker.io/base-version` or a `-patched-` tag.
- **Per-cluster LKG** is stored as Deployment annotations:
  - `caretaker.io/last-known-good-image` — last image that passed verification
  - `caretaker.io/previous-image` — image replaced by the last update
  - `caretaker.io/base-version` — `1.0`, used to resolve the base image digest
  - `caretaker.io/last-update-at`, `caretaker.io/managed`
  - `caretaker.io/webhook` — set when the admission webhook rewrote the image

## Update flow

`caretaker update`:
1. Choose a target: `--map service=image`, the preference map, or the `newest` policy.
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

caretaker patch image --service api [--source REF] [--tag TAG] [--report FILE] [--write-mapping]
caretaker patch all [--services all] [--write-mapping]
caretaker scan --image REF --output sbom.cdx.json

caretaker update --clusters all --services all [--preferences FILE] [--map svc=image] \
  [--dry-run] [--no-verify] [--no-rollback] [--timeout 5m] [--health-cmd CMD]
caretaker rollback --clusters all --services all [--from-lkg | --image REF] [--dry-run]

caretaker mapping show [--file FILE]
caretaker mapping set --service api --source SRC --target TGT
caretaker mapping merge --file BASE --updates UPDATES --out OUT

caretaker webhook serve
caretaker webhook install --cluster kind-caretaker [--namespaces ns1,ns2] [--failure-policy Ignore|Fail]
caretaker webhook status --cluster kind-caretaker
caretaker webhook uninstall --cluster kind-caretaker
```

Copa invocation: `copa patch -i <source> -t <tag> --addr <buildkit> --platform <plat> --push`
(plus `-r <report>` when configured). The patched image is tagged `<base>-patched-<YYYYMMDD>`.

## Deployment webhook

`caretaker webhook serve` runs an HTTPS mutating admission server on the workstation (default
`:8443`). `caretaker webhook install` registers `caretaker-deployments` with the cluster, pointing at
`https://<webhook.host>:<port>/mutate` and embedding the generated CA. The webhook rewrites Deployment
container images to the digest-only preference target on create/update, and adds
`caretaker.io/base-version`, `caretaker.io/managed`, and `caretaker.io/webhook`. `failurePolicy` defaults
to `Ignore`; `caretaker.io/skip-webhook: "true"` bypasses mutation. Certificates live in
`.caretaker/webhook/`. The API server must reach `webhook.host` (in this kind setup, `10.89.2.1`).

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
