#!/usr/bin/env bash
set -euo pipefail

# Portable binaries and Python environment for the prototype.
# Idempotent: safe to run multiple times. Skips if already installed.
# To update: delete the binary from bin/ or .venv/ and re-run this script.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BIN_DIR="$PROJECT_ROOT/bin"

mkdir -p "$BIN_DIR"

install_helm() {
    local bin="$BIN_DIR/helm"
    if [ -x "$bin" ]; then
        echo "helm: already installed ($("$bin" version --short 2>/dev/null || echo "unknown version"))"
        return 0
    fi
    echo "helm: installing..."
    local version="v3.22.0"
    local os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    local arch="$(uname -m)"
    case "$arch" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
        armv7l)  arch="arm" ;;
    esac
    local url="https://get.helm.sh/helm-${version}-${os}-${arch}.tar.gz"
    local tmpdir=$(mktemp -d)
    curl -fsSL "$url" | tar xz -C "$tmpdir"
    mv "$tmpdir/${os}-${arch}/helm" "$bin"
    chmod +x "$bin"
    rm -rf "$tmpdir"
    echo "helm: installed ($("$bin" version --short 2>/dev/null || echo "done"))"
}

setup_python() {
    local venv="$PROJECT_ROOT/.venv"
    if [ -d "$venv" ]; then
        echo "python: venv already exists at $venv"
        return 0
    fi
    if ! command -v uv &>/dev/null; then
        echo "python: uv not found — install uv first (https://docs.astral.sh/uv/)"
        return 1
    fi
    echo "python: setting up venv..."
    cd "$PROJECT_ROOT"
    uv venv .venv
    uv pip install -r requirements.txt
    echo "python: ready"
}

install_helm
setup_python

echo ""
echo "Setup complete."
echo "  Binaries: $BIN_DIR"
echo "  Python:   $PROJECT_ROOT/.venv"
echo "Run scripts with: $PROJECT_ROOT/.venv/bin/python <script>"
echo "Or activate:      source $PROJECT_ROOT/.venv/bin/activate"
