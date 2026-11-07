# Caretaker Walkthrough

Caretaker patches container images with Copacetic (Copa) and rolls the patched
digests out to Kubernetes. This guide follows the product lifecycle:

**assess → patch → map → set target → verify/fallback/restore → enforce at admission**

Everything here is digest-first. Tags appear only where Copa needs a push tag or
where a command explicitly demonstrates tag support (`--allow-tags`).

Verified on: Kubernetes `v1.37.0` (kind `v0.34.0-alpha`), zot `v2.1.22`,
Copa `v0.15.0`, cosign `v3.1.4`.

> Digest examples are shortened for readability (`sha256:04e9733f...`).
> Replace them with the full digest printed by `patch`, `status`, `mapping show`,
> or `lineage`; the shortened form is not a valid image reference.

---

## 1. What you need (dependencies)

### On your workstation

| Dependency | Why | How this repo gets it |
|---|---|---|
| Go 1.24+ | build `caretaker` | `make build` |
| A container runtime | build/push images, run kind | podman (or Docker) |
| `kind` | demo cluster | `make setup` installs to `bin/` |
| `kubectl` | inspect the cluster | `make setup` installs to `bin/` |
| `copa` | patches images | `make setup` installs to `bin/` |
| `trivy` | scans / SBOMs / `--skip-if-clean` | `make setup` installs to `bin/` |
| `cosign` | optional preference signing | `make setup` installs to `bin/` |
| BuildKit daemon | Copa's build engine | `./scripts/start-buildkit.sh` |
| A registry | stores base + patched images and artifacts | `./scripts/start-registry.sh` (zot) |

### In the cluster

- A kube-context caretaker can reach (the demo uses `kind-caretaker`).
- The cluster must be able to pull from the registry (the demo adds a
  containerd mirror for `localhost:5000`).
- Kubernetes **1.36+** only if you want the `MutatingAdmissionPolicy`
  alternative to the webhook. The tool prints the server version so you can
  tell.

### Registry

- Writable repositories for your service images.
- Space for two kinds of metadata: the per-repo `caretaker-lineage-index` and
  the `caretaker-preferences` repository (optional, only if you push
  preferences).
- No database, no in-cluster agent, no control-plane changes.

---

## 2. Actions only you can do

These are the things the tool cannot do for you:

1. **Provide a container runtime and, on Windows/WSL, a running podman machine.**
2. **Expose a registry that both your workstation and the cluster can reach.**
   In the kind demo the host gateway is `10.89.2.1`, which is why the in-cluster
   webhook is given `--registry-url 10.89.2.1:5000` (inside a pod,
   `localhost:5000` is the pod, not your host).
3. **Start BuildKit** somewhere Copa can reach (`tcp://127.0.0.1:12345` by
   default).
4. **Create or choose the cluster** and make sure the kube-context is in
   `caretaker.yaml`.
5. **Decide whether to install the admission webhook** (optional) and whether
   to use the in-cluster webhook or the `MutatingAdmissionPolicy` variant.
6. **Supply signing keys** if you want signed preference versions.

---

## 3. Environment variables

```bash
export PATH="$PWD/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=podman
```

On the WSL/podman-machine setup used for this demo, the podman-backed scripts
also need:

```bash
export DOCKER_HOST="unix:///mnt/wsl/podman-sockets/podman-machine-default/podman-root.sock"
export CONTAINER_HOST="$DOCKER_HOST"
```

On a native Linux box with a local Docker/podman socket, omit those two.

Caretaker itself reads its config from `caretaker.yaml` (or
`--config`/`CARETAKER_CONFIG`). Environment overrides exist for registry,
tools, and cluster access; see `caretaker config show` and the README.

---

## 4. Bring the demo stack up

```bash
make setup                       # installs copa, trivy, cosign, zot, kind, kubectl; builds bin/caretaker
./scripts/start-registry.sh      # zot on :5000
./scripts/start-buildkit.sh      # buildkitd on :12345
./scripts/push-images.sh         # six base images at 1.0
./scripts/kind-setup.sh          # kind cluster + Deployments + registry mirror
```

Check the stack:

```bash
./bin/caretaker doctor
./bin/caretaker clusters
```

`clusters` now includes a **K8S** column. If it shows `v1.36.0` or newer, the
MutatingAdmissionPolicy option is available on that cluster.

`doctor` also reports admission state per cluster:

- `webhook/<cluster>` — deployed webhook, Service/Deployment readiness, and
  whether the served preferences ConfigMap matches `config/preferences.yaml`.
- `policymap/<cluster>` — MutatingAdmissionPolicy state and whether its
  `caretaker-image-map` ConfigMap is stale.
- `admission/<cluster>` — warns if both mechanisms are installed.

A stale-preferences warning means a `patch --write-mapping` or `mapping set`
changed only the local file. Push the served copy before `set-target`.

If the machine was restarted, the persistent pieces can be restarted without
recreating anything:

```bash
setsid --fork ./bin/zot serve ./config/zot-minimal.json >/tmp/zot.log 2>&1 </dev/null
podman start buildkitd
podman start caretaker-control-plane
```

---

## 5. Assess

```bash
./bin/caretaker status
./bin/caretaker lkg
./bin/caretaker mapping show

# SBOM for one image (CycloneDX)
./bin/caretaker scan --image localhost:5000/test/api@sha256:04e9733f... \
  --output /tmp/api-sbom.cdx.json
```

---

## 6. Patch one image

```bash
# Plan first
./bin/caretaker patch image --service api \
  --source localhost:5000/test/api@sha256:04e9733f... \
  --base-version 1.0 --write-mapping --dry-run

# Real run
./bin/caretaker patch image --service api \
  --source localhost:5000/test/api@sha256:04e9733f... \
  --base-version 1.0 --write-mapping
```

What happens:

1. Copa builds and pushes a patched image to the same repository. The digest is
   the identity; the generated tag is only an alias
   (`1.0-patched-YYYYMMDD-HHMMSS-<12-char-source-digest>`).
2. Caretaker records lineage in the registry as OCI referrers:
   child metadata on the child, a child pointer on the parent, and an
   append-only entry in `caretaker-lineage-index`.
3. `--write-mapping` records the preference (feature-build digest → target) in
   `config/preferences.yaml`.

Inspect lineage:

```bash
./bin/caretaker lineage log --service api
./bin/caretaker lineage children localhost:5000/test/api@sha256:04e9733f... --service api
./bin/caretaker lineage show localhost:5000/test/api@sha256:33e60967...
```

`lineage children` returns direct and indirect descendants, with `DEPTH`.

### Optional patch flags

- `--skip-if-clean` — Trivy first; skip Copa when no vulnerabilities exist.
- `--cve-only --report trivy.json` — pass Copa a report (Copa's own CVE-only
  mode).
- `--concurrency N` — `patch all` only; see §11.
- `--allow-tags` — opt into tag input. Not needed for normal digest workflows.

---

## 7. Set the target (roll out)

Base case — one cluster, one service, explicit digest:

```bash
./bin/caretaker set-target --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967... --dry-run

./bin/caretaker set-target --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967...
```

Without `--map`, caretaker chooses in this order:

1. your preference map,
2. the per-service `newest` policy,
3. the newest surviving child of the deployment's base from the lineage index.

Every target is pinned to `repo@digest` before the swap. Caretaker writes
`caretaker.io/base-digest`, seeds `last-known-good-image`, records
`previous-image`, swaps, waits for rollout, and advances LKG on success. A
failed verification falls back to the LKG automatically.

`set-target` is also how you deliberately move to an older image: it is the
desired state, not a restore. Use `restore` only for the break-glass path in §8.

If the webhook or MAP is installed and you just changed preferences with
`patch --write-mapping` or `mapping set`, push the served copy first:

```bash
./bin/caretaker webhook prefs push --cluster kind-caretaker
# or, with MutatingAdmissionPolicy:
./bin/caretaker webhook map prefs push --cluster kind-caretaker --file config/preferences.yaml
```

Otherwise admission can rewrite the new target back to the stale served target.
`doctor` warns about this.

```bash
./bin/caretaker status
```

`update` remains available as an alias, and `--no-rollback` as a deprecated
alias for `--no-fallback`.

---

## 8. Verify, automatic fallback, and restore

### 8.1 Automatic fallback (failed verification)

Choose a target and fail the health command. Caretaker waits for the rollout,
runs the check, then returns to the LKG on its own:

```bash
./bin/caretaker set-target --clusters kind-caretaker --services api \
  --map api=localhost:5000/test/api@sha256:33e60967... \
  --health-cmd "false"
```

Expect `ACTION=fell-back`. The LKG stays on the last image that passed
verification; `previous-image` becomes the abandoned target so you can inspect
or manually toggle later. `--no-fallback` leaves the failed image in place
instead.

Automatic fallback sets `caretaker.io/skip-webhook: "true"` before swapping back,
so the in-cluster webhook cannot immediately re-apply the target. MAP ignores
that annotation — see §11.

### 8.2 Manual restore (break-glass)

`restore` is the intentional escape hatch. It returns to the image replaced by
the last `set-target` by default:

```bash
./bin/caretaker restore --clusters kind-caretaker --services api --dry-run
./bin/caretaker restore --clusters kind-caretaker --services api

# Or the recorded LKG
./bin/caretaker restore --clusters kind-caretaker --services api --lkg

# Or an explicit image
./bin/caretaker restore --clusters kind-caretaker --services api \
  --image localhost:5000/test/api@sha256:04e9733f...
```

`restore` sets `caretaker.io/skip-webhook: "true"` before the swap, waits for the
rollout, and records the abandoned image in `previous-image` so a second restore
toggles back. It does not run health checks and bypasses admission by design.

To deliberately choose an older image as desired state while keeping
preferences and admission in effect, use `set-target` with the older digest —
that is not a restore.

`rollback` remains available as an alias, and `--from-lkg` as a deprecated alias
for `--lkg`.

---

## 9. Preferences, aliases, and registry-stored copies

`config/preferences.yaml` (v1alpha2):

```yaml
apiVersion: caretaker/v1alpha2
kind: PatchPreferences
services:
  api:
    policy: pinned            # or newest
    mappings:
      - base: localhost:5000/test/api@sha256:04e9733f...   # feature build
        aliases:                                            # older patches of the same feature set
          - localhost:5000/test/api@sha256:b5bc47de...
        target: localhost:5000/test/api@sha256:33e60967...  # preferred patch
        baseVersion: "1.0"
        patchedAt: "2026-10-29T03:02:52Z"
```

- `base` is the key. `aliases` let a Deployment that pins an older patched
  digest still be recognized by admission and moved to the preferred target.
- Add an alias without destroying other fields:
  `caretaker mapping set --service api --base <base> --target <target> --alias <older>`
- Push/pull the whole document as a signed, digest-addressed registry artifact:

```bash
./bin/caretaker mapping push --sign
./bin/caretaker mapping pull --digest sha256:... --verify --file /tmp/prefs.yaml
```

---

## 10. Admission webhook — in-cluster (recommended)

This is the "simple, removable" shape: a Deployment + Service, preferences in a
ConfigMap, TLS in a Secret, and a `MutatingWebhookConfiguration` that points at
the Service. The API server never calls your workstation.

### 10.1 Build and push the webhook image

```bash
./scripts/build-webhook.sh
# prints a digest-pinned ref, e.g.
# localhost:5000/test/caretaker-webhook@sha256:5b05ecfc...
```

### 10.2 Deploy

```bash
./bin/caretaker webhook deploy --cluster kind-caretaker \
  --image localhost:5000/test/caretaker-webhook@sha256:5b05ecfc... \
  --registry-url 10.89.2.1:5000 --registry-insecure
```

What it creates:

| Object | Name | Purpose |
|---|---|---|
| Namespace | `caretaker-system` | holds the webhook |
| ConfigMap | `caretaker-preferences` | your preferences document |
| Secret | `caretaker-webhook` | `tls.crt`, `tls.key`, `ca.crt` |
| Deployment | `caretaker-webhook` | runs `caretaker webhook serve` |
| Service | `caretaker-webhook` | port 443 → 8443 |
| MutatingWebhookConfiguration | `caretaker-deployments` | points at the Service |

**You must** pass a registry URL reachable from pods. In kind that is
`10.89.2.1:5000`; do not use `localhost:5000` (that is the pod itself).

Check it:

```bash
./bin/kubectl --context kind-caretaker -n caretaker-system get pods
./bin/caretaker webhook status --cluster kind-caretaker
```

`webhook status` shows the Kubernetes version and whether
MutatingAdmissionPolicy is available.

### 10.3 Prove it mutates with no host process

Make sure nothing is listening on `:8443` locally, then deploy a base digest:

```bash
./bin/kubectl --context kind-caretaker -n default apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: base-test
spec:
  replicas: 0
  selector: {matchLabels: {app: base-test}}
  template:
    metadata: {labels: {app: base-test}}
    spec:
      containers:
        - name: app
          image: localhost:5000/test/api@sha256:04e9733f...
EOF

./bin/kubectl --context kind-caretaker -n default get deploy base-test \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

The image comes back as the patched target, and the Deployment carries
`caretaker.io/base-digest`, `caretaker.io/managed`, and `caretaker.io/webhook`.

Recognition is by digest and is independent of the container or Deployment
name, so it also catches manifests deployed by kubectl, GitOps, or Helm.

### 10.4 Push preferences live (required after local edits)

```bash
# edit config/preferences.yaml, or run patch --write-mapping / mapping set, then:
./bin/caretaker webhook prefs push --cluster kind-caretaker
```

The webhook rereads the mounted file on every admission request; the kubelet
refreshes ConfigMap volumes within about a minute. This is required when an
admission mechanism is installed: `patch --write-mapping` and `mapping set`
change only the local file. If the served ConfigMap still has the old target, a
following `set-target` can be rewritten back to it. `doctor` reports the stale
preferences (`preferences stale vs ...; run webhook prefs push`).

### 10.5 Remove it

```bash
./bin/caretaker webhook undeploy --cluster kind-caretaker
# optionally:
./bin/kubectl --context kind-caretaker delete namespace caretaker-system
```

Nothing else needs cleanup: images and annotations are inert, and the registry
artifacts are only read by caretaker commands.

---

## 11. Admission policy (MutatingAdmissionPolicy, alpha)

> **Alpha — not recommended as the primary admission mechanism.** MAP works for
> steady-state image rewriting, but it ignores `caretaker.io/skip-webhook`. A
> manual `restore` or an automatic fallback to a mapped base/alias can be
> rewritten straight back to the preferred target. Use the in-cluster webhook
> (§10) when you need a reliable break-glass path.

On **Kubernetes 1.36+** (this cluster is 1.37) MAP does the same image rewriting
with no webhook server, image, certificate, or host process at all. Kubernetes
evaluates a CEL expression at admission time and looks the image up in a
ConfigMap parameter.

```bash
# install policy + binding + parameter ConfigMap
./bin/caretaker webhook map install --cluster kind-caretaker \
  --preferences config/preferences.yaml

# check it
./bin/caretaker webhook map status --cluster kind-caretaker

# update the digest map later (no reinstall needed)
./bin/caretaker webhook map prefs push --cluster kind-caretaker \
  --file config/preferences.yaml

# remove it
./bin/caretaker webhook map uninstall --cluster kind-caretaker
```

What it creates:

| Object | Name | Scope |
|---|---|---|
| MutatingAdmissionPolicy | `caretaker-images` | cluster |
| MutatingAdmissionPolicyBinding | `caretaker-images` | cluster |
| ConfigMap (parameter) | `caretaker-image-map` in `caretaker-system` | namespace |

Behavior details:

- Rewrites both `containers` and `initContainers`; unmapped digests are left
  untouched (the expression emits a no-op `test` op for them).
- **It does not add caretaker annotations** — only the image is rewritten. The
  in-cluster webhook is the mode that annotates `caretaker.io/base-digest` and
  friends.
- **It ignores `caretaker.io/skip-webhook`.** `restore` and automatic fallback
  are not protected: if the target image is a mapped base or alias, MAP can
  rewrite it back after the swap. This is the main alpha limitation.
- Preferences are served from the `caretaker-image-map` ConfigMap. After
  `patch --write-mapping` or `mapping set`, run `webhook map prefs push` before
  `set-target`; otherwise admission can restore the stale target. `doctor`
  reports `image map stale vs ...`.
- The policy is cluster-wide. There is no per-service opt-out beyond the
  namespace binding.
- The binding uses `parameterNotFoundAction: Allow`, so a missing parameter
  ConfigMap never blocks admissions; `failurePolicy` defaults to `Ignore`, and
  `reinvocationPolicy` is `IfNeeded`.
- The ConfigMap is keyed by a sanitized `<repo>_<digest>` string (ConfigMap keys
  cannot contain `:`, `@`, or `/`); values are full digest-pinned target refs.
- `map status` refuses on clusters older than 1.36; `webhook status` tells you
  whether MAP is available at all.

`webhook deploy` and `map install` are **alternatives** — pick one. Do not enable
both, or an image could be rewritten twice (which is harmless but confusing).

---

## 12. Parallel patching (what it actually is)

`patch all --concurrency N` starts **N Copa processes**, one image each, behind
a semaphore. It is not Copa's bulk mode.

```bash
./bin/caretaker patch all --concurrency 4 --write-mapping
```

Dependencies per worker: the same BuildKit daemon, registry push bandwidth, CPU,
and disk. The practical limit is BuildKit, not the number of goroutines.

Copa v0.15.0 does have native bulk (`copa patch --config file.yaml --push`) with
a worker pool sized to `runtime.NumCPU()`. Caretaker does not use it because its
config is tag-oriented and it provides no per-image hook for lineage/preference
recording. Concurrency is also settable with `patch.concurrency` or
`CARETAKER_PATCH_CONCURRENCY` (default 1).

---

## 13. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `ImagePullBackOff` for the webhook | image digest not in the registry | use the digest printed by `build-webhook.sh` (it reads `Docker-Content-Digest`) |
| `write CA: ... read-only file system` | Secret missing `ca.crt`, or cert SANs do not cover the Service | redeploy with the current code (`ca.crt` is included) |
| Webhook does nothing | `failurePolicy: Ignore` and the backend is down | check `webhook status`, pod logs, and that the API server can reach the Service |
| `no preferred mapping` | base digest unknown or preferences stale | annotate `caretaker.io/base-digest`, fix the mapping, or push preferences |
| `set-target` is rewritten back to the old target | served preferences/image map is stale | run `webhook prefs push` or `webhook map prefs push`; `doctor` reports the staleness |
| `restore` is immediately re-patched (webhook) | `caretaker.io/skip-webhook` did not survive | inspect the annotation; a later `set-target` clears it back to `false` |
| `restore` is immediately re-patched (MAP) | MAP ignores `skip-webhook` | expected alpha limitation; use the webhook mode for reliable restores |

---

## 14. Minimal command reference

```bash
caretaker doctor
caretaker clusters                 # includes K8S version
caretaker status
caretaker lkg
caretaker scan --image REF --output sbom.cdx.json
caretaker patch image --service S --source REF@sha256:... [--write-mapping] [--skip-if-clean] [--dry-run]
caretaker patch all [--concurrency N] [--write-mapping]
caretaker lineage log|children|show ...
caretaker set-target --clusters C --services S [--map svc=ref] [--health-cmd C] [--force] [--dry-run] [--no-fallback]
caretaker restore --clusters C --services S [--lkg|--image REF] [--dry-run]
# aliases: update -> set-target, rollback -> restore; --from-lkg -> --lkg; --no-rollback -> --no-fallback
caretaker mapping show|set|merge|push|pull
caretaker webhook deploy|undeploy|prefs push|status|uninstall
caretaker webhook map install|uninstall|prefs push|status
```
