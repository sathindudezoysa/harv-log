#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGIN_DIR="$PROJECT_DIR/grafana-plugin"
DIST_DIR="$PLUGIN_DIR/dist"
REPOSITORY="${HARV_LOGS_REPOSITORY:-sathindudezoysa/harv-log}"
RELEASE_TAG="${HARV_LOGS_RELEASE_TAG:-latest}"

for command_name in curl jq tar; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Error: '$command_name' is not installed."
    exit 1
  fi
done

if [[ "$RELEASE_TAG" == "latest" ]]; then
  RELEASE_TAG="$(curl --fail --location --show-error \
    "https://api.github.com/repos/$REPOSITORY/releases/latest" | jq -r '.tag_name')"
fi

ASSET_VERSION="${RELEASE_TAG#v}"
DOWNLOAD_URL="https://github.com/$REPOSITORY/releases/download/$RELEASE_TAG/harv-logs-$ASSET_VERSION.tar.gz"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "==> Downloading Harv Logs plugin release $RELEASE_TAG"
curl --fail --location --show-error --retry 3 \
  --output "$TEMP_DIR/harv-logs.tar.gz" "$DOWNLOAD_URL"

tar -xzf "$TEMP_DIR/harv-logs.tar.gz" -C "$TEMP_DIR"
if [[ ! -d "$TEMP_DIR/dist" ]]; then
  echo "Error: release archive does not contain a dist directory."
  exit 1
fi

echo "==> Installing plugin files in $DIST_DIR"
rm -rf "$DIST_DIR"
mv "$TEMP_DIR/dist" "$DIST_DIR"