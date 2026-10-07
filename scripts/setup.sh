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

install_copa() {
    local bin="$BIN_DIR/copa"
    if [ -x "$bin" ]; then
        echo "copa: already installed ($("$bin" -v 2>/dev/null || echo "unknown version"))"
        return 0
    fi
    echo "copa: installing..."
    local version="v0.15.0"
    local arch="$(uname -m)"
    case "$arch" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
    esac
    local url="https://github.com/project-copacetic/copacetic/releases/download/${version}/copa_${version#v}_linux_${arch}.tar.gz"
    local tmpdir=$(mktemp -d)
    curl -fsSL "$url" | tar xz -C "$tmpdir"
    mv "$tmpdir/copa" "$bin"
    chmod +x "$bin"
    rm -rf "$tmpdir"
    echo "copa: installed ($("$bin" -v 2>/dev/null || echo "done"))"
}

install_trivy() {
    local bin="$BIN_DIR/trivy"
    if [ -x "$bin" ]; then
        echo "trivy: already installed ($("$bin" --version 2>/dev/null | head -1 || echo "unknown version"))"
        return 0
    fi
    echo "trivy: installing..."
    local version="v0.75.0"
    local arch="$(uname -m)"
    case "$arch" in
        x86_64)  arch="64bit" ;;
        aarch64) arch="ARM64" ;;
    esac
    local url="https://github.com/aquasecurity/trivy/releases/download/${version}/trivy_${version#v}_Linux-${arch}.tar.gz"
    local tmpdir=$(mktemp -d)
    curl -fsSL "$url" | tar xz -C "$tmpdir"
    mv "$tmpdir/trivy" "$bin"
    chmod +x "$bin"
    rm -rf "$tmpdir"
    echo "trivy: installed ($("$bin" --version 2>/dev/null | head -1 || echo "done"))"
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
install_copa
install_trivy
setup_python

echo ""
echo "Setup complete."
echo "  Binaries: $BIN_DIR"
echo "  Python:   $PROJECT_ROOT/.venv"
echo "  PATH:     export PATH=\"$BIN_DIR:\$PATH\""
echo "Run scripts with: $PROJECT_ROOT/.venv/bin/python <script>"
echo "Or activate:      source $PROJECT_ROOT/.venv/bin/activate"
