"""Core cluster model and providers for caretaker.

Production is modelled as independent clusters, each running some set of
services.  The lowest unit of deployment is (cluster, service) -- NOT a helm
chart.  Clusters are versioned independently: two prod clusters may run
different images of the same service at the same time.

Providers abstract how live state is read and mutated.  Caretaker is meant to
sit *beside* whatever tool created the cluster; it only reads running state and
performs in-place image replacement (hot swap).

  * SimulatedProvider -- file-backed JSON state under clusters/ (fast, offline)
  * KindProvider      -- real Kubernetes via `kubectl set image`
"""

from __future__ import annotations

import json
import subprocess
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CLUSTERS_DIR = ROOT / "clusters"
HISTORY_PATH = ROOT / "history" / "events.jsonl"
REGISTRY_PATH = ROOT / "config" / "clusters.json"

# Services that make up the demo application.
SERVICES = ["api", "auth", "cache", "payments", "web", "worker"]

# Where service images live in the local registry.
IMAGE_NAMESPACE = "localhost:5000/test"


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def record_event(
    cluster: "Cluster",
    service: str,
    action: str,
    from_tag: str,
    from_digest: str,
    to_tag: str,
    to_digest: str,
) -> None:
    """Append a patch/deploy event to the central history log (JSONL).

    The log is provider-agnostic so simulated and real (kind) hot swaps share
    one audit trail.  This is the source of truth for the UI history page.
    """
    HISTORY_PATH.parent.mkdir(parents=True, exist_ok=True)
    event = {
        "at": now_iso(),
        "cluster": cluster.id,
        "stage": cluster.stage,
        "provider": cluster.provider,
        "service": service,
        "action": action,
        "fromTag": from_tag,
        "fromDigest": from_digest,
        "toTag": to_tag,
        "toDigest": to_digest,
    }
    with HISTORY_PATH.open("a") as fh:
        fh.write(json.dumps(event) + "\n")


def load_events() -> list[dict]:
    """Read the central history log (oldest first).  Returns [] if absent."""
    if not HISTORY_PATH.exists():
        return []
    events: list[dict] = []
    for line in HISTORY_PATH.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return events


@dataclass
class Cluster:
    id: str
    stage: str  # testing | staging | production
    provider: str = "simulated"
    namespace: str = "default"

    @property
    def is_production(self) -> bool:
        return self.stage == "production"


@dataclass
class Deployment:
    """A single service running inside a single cluster."""

    service: str
    container: str
    repository: str
    tag: str
    digest: str = ""
    replicas: int = 1

    @property
    def image_ref(self) -> str:
        base = f"{self.repository}:{self.tag}"
        return f"{base}@{self.digest}" if self.digest else base

    @property
    def tag_ref(self) -> str:
        return f"{self.repository}:{self.tag}"


class Provider:
    """Abstract interface every cluster provider must implement."""

    name = "abstract"

    def list_deployments(self, cluster_id: str) -> list[Deployment]:
        raise NotImplementedError

    def get_deployment(self, cluster_id: str, service: str) -> Deployment | None:
        for d in self.list_deployments(cluster_id):
            if d.service == service:
                return d
        return None

    def set_image(
        self, cluster_id: str, service: str, repository: str, tag: str, digest: str = ""
    ) -> Deployment:
        """Replace the running image in place (hot swap)."""
        raise NotImplementedError


class SimulatedProvider(Provider):
    """File-backed fake cluster.

    Each cluster is a JSON document under ``clusters/<id>.json``.  Hot swaps
    mutate the document and append to an ``events`` log so the operation is
    auditable even without a real orchestrator.
    """

    name = "simulated"

    def __init__(self, state_dir: Path = CLUSTERS_DIR):
        self.state_dir = Path(state_dir)

    def _path(self, cluster_id: str) -> Path:
        return self.state_dir / f"{cluster_id}.json"

    def _load(self, cluster_id: str) -> dict:
        path = self._path(cluster_id)
        if not path.exists():
            raise FileNotFoundError(
                f"no simulated state for cluster {cluster_id!r} (expected {path})"
            )
        return json.loads(path.read_text())

    def _save(self, cluster_id: str, state: dict) -> None:
        self._path(cluster_id).write_text(json.dumps(state, indent=2) + "\n")

    def list_deployments(self, cluster_id: str) -> list[Deployment]:
        state = self._load(cluster_id)
        deployments = []
        for _svc, entry in sorted(state.get("deployments", {}).items()):
            image = entry.get("image", {})
            deployments.append(
                Deployment(
                    service=entry["service"],
                    container=entry.get("container", entry["service"]),
                    repository=image.get("repository", ""),
                    tag=image.get("tag", ""),
                    digest=image.get("digest", ""),
                    replicas=entry.get("replicas", 1),
                )
            )
        return deployments

    def set_image(
        self, cluster_id: str, service: str, repository: str, tag: str, digest: str = ""
    ) -> Deployment:
        state = self._load(cluster_id)
        entry = state.get("deployments", {}).get(service)
        if entry is None:
            raise KeyError(f"service {service!r} is not deployed in cluster {cluster_id!r}")

        before = dict(entry.get("image", {}))
        entry["image"] = {"repository": repository, "tag": tag, "digest": digest}
        state.setdefault("events", []).append(
            {
                "at": now_iso(),
                "type": "set_image",
                "service": service,
                "from": before,
                "to": dict(entry["image"]),
            }
        )
        self._save(cluster_id, state)

        updated = self.get_deployment(cluster_id, service)
        assert updated is not None
        return updated


class KindProvider(Provider):
    """Real Kubernetes cluster (kind) driven by ``kubectl``.

    Each caretaker cluster maps to a kube-context (name == cluster id).  A hot
    swap is exactly ``kubectl set image deployment/<svc> <svc>=<repo>@<digest>``.
    """

    name = "kind"

    def __init__(self, kubeconfig: str | None = None, namespace: str = "default"):
        self.kubeconfig = kubeconfig
        self.namespace = namespace

    def _kubectl(self, context: str, *args: str) -> str:
        cmd = ["kubectl"]
        if self.kubeconfig:
            cmd += ["--kubeconfig", self.kubeconfig]
        cmd += ["--context", context, "-n", self.namespace, *args]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            raise RuntimeError(
                f"kubectl {' '.join(args)} failed: {result.stderr.strip()}"
            )
        return result.stdout

    def list_deployments(self, cluster_id: str) -> list[Deployment]:
        raw = self._kubectl(cluster_id, "get", "deployments", "-o", "json")
        payload = json.loads(raw)
        deployments = []
        for item in payload.get("items", []):
            name = item["metadata"]["name"]
            spec = item["spec"]
            for container in spec["template"]["spec"]["containers"]:
                image = container["image"]
                repository, tag, digest = _split_image_ref(image)
                deployments.append(
                    Deployment(
                        service=name,
                        container=container["name"],
                        repository=repository,
                        tag=tag,
                        digest=digest,
                        replicas=spec.get("replicas", 1),
                    )
                )
        return deployments

    def set_image(
        self, cluster_id: str, service: str, repository: str, tag: str, digest: str = ""
    ) -> Deployment:
        image_ref = f"{repository}:{tag}@{digest}" if digest else f"{repository}:{tag}"
        self._kubectl(
            cluster_id, "set", "image", f"deployment/{service}", f"{service}={image_ref}"
        )
        return self.get_deployment(cluster_id, service)


def _split_image_ref(image: str) -> tuple[str, str, str]:
    """Split ``repo:tag@sha256:...`` into (repository, tag, digest)."""
    digest = ""
    if "@" in image:
        image, digest = image.split("@", 1)
    repository, tag = image, "latest"
    slash = repository.rfind("/")
    colon = repository.rfind(":")
    if colon > slash:
        repository, tag = repository[:colon], repository[colon + 1 :]
    return repository, tag, digest


def load_clusters(path: Path = REGISTRY_PATH) -> list[Cluster]:
    data = json.loads(Path(path).read_text())
    return [Cluster(**c) for c in data["clusters"]]


def get_cluster(cluster_id: str, path: Path = REGISTRY_PATH) -> Cluster | None:
    for cluster in load_clusters(path):
        if cluster.id == cluster_id:
            return cluster
    return None


def provider_for(cluster: Cluster) -> Provider:
    if cluster.provider == "simulated":
        return SimulatedProvider()
    if cluster.provider == "kind":
        return KindProvider(namespace=cluster.namespace)
    raise ValueError(f"unknown provider {cluster.provider!r} for cluster {cluster.id!r}")
