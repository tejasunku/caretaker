#!/usr/bin/env bash
set -euo pipefail

# Start zot registry for local image storage.
# Images are stored in ./registry-data/ (gitignored).
# Registry API available at http://localhost:5000

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BIN_DIR="$PROJECT_ROOT/bin"
CONFIG="$PROJECT_ROOT/config/zot-minimal.json"
DATA_DIR="$PROJECT_ROOT/registry-data"

# Ensure data directory exists
mkdir -p "$DATA_DIR"

# Ensure zot is installed
if [ ! -x "$BIN_DIR/zot" ]; then
    echo "zot not found in $BIN_DIR — run scripts/setup.sh first"
    exit 1
fi

echo "Starting zot registry at http://localhost:5000"
echo "Storage: $DATA_DIR"
echo "Config:  $CONFIG"
echo ""
echo "Push images with:"
echo "  podman push --tls-verify=false localhost:5000/<name>:<tag>"
echo ""
echo "Pull images with:"
echo "  podman pull --tls-verify=false localhost:5000/<name>:<tag>"
echo ""

exec "$BIN_DIR/zot" serve "$CONFIG"
