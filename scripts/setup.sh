#!/usr/bin/env bash
set -euo pipefail

# setup.sh — Download the forge-state-mcp binary from GitHub Releases.
#
# Called automatically by the Claude Code "Setup" hook when the plugin is
# first installed (or when the plugin version changes).
#
# Environment:
#   CLAUDE_PLUGIN_ROOT  — set by Claude Code to the plugin install directory
#
# The binary is placed at $CLAUDE_PLUGIN_ROOT/bin/forge-state-mcp so that
# .mcp.json can reference it via ${CLAUDE_PLUGIN_ROOT}/bin/forge-state-mcp.

REPO="hiromaily/claude-forge"
BINARY_NAME="forge-state-mcp"

die() { echo "setup.sh: $*" >&2; exit 1; }

# --- Resolve plugin root ------------------------------------------------
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
[ -z "$PLUGIN_ROOT" ] && die "CLAUDE_PLUGIN_ROOT is not set"

BIN_DIR="${PLUGIN_ROOT}/bin"
BINARY_PATH="${BIN_DIR}/${BINARY_NAME}"

# --- Detect version from plugin.json ------------------------------------
VERSION=""
if command -v jq >/dev/null 2>&1; then
  VERSION="$(jq -r '.version // empty' "${PLUGIN_ROOT}/.claude-plugin/plugin.json" 2>/dev/null || true)"
fi
if [ -z "$VERSION" ]; then
  # Fallback: grep for version field
  VERSION="$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "${PLUGIN_ROOT}/.claude-plugin/plugin.json" 2>/dev/null \
    | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)"
fi
[ -z "$VERSION" ] && die "Could not determine plugin version from plugin.json"

# --- Skip if already installed at this version ---------------------------
MARKER="${BIN_DIR}/.installed-version"
if [ -f "$MARKER" ] && [ -f "$BINARY_PATH" ]; then
  INSTALLED="$(cat "$MARKER" 2>/dev/null || true)"
  if [ "$INSTALLED" = "$VERSION" ]; then
    exit 0
  fi
fi

# --- Detect platform -----------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       die "Unsupported architecture: $ARCH" ;;
esac

ASSET="forge-state-mcp-${OS}-${ARCH}.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ASSET}"

# --- Download and install ------------------------------------------------
echo "setup.sh: downloading ${BINARY_NAME} v${VERSION} (${OS}/${ARCH})..." >&2

mkdir -p "$BIN_DIR"

HTTP_CODE="$(curl -fsSL -w '%{http_code}' -o "${BINARY_PATH}.gz" "$URL" || true)"
if [ "$HTTP_CODE" != "200" ] || [ ! -f "${BINARY_PATH}.gz" ]; then
  rm -f "${BINARY_PATH}.gz"
  # Fallback: build from source if Go is available
  if command -v go >/dev/null 2>&1 && [ -d "${PLUGIN_ROOT}/mcp-server" ]; then
    echo "setup.sh: release not found, building from source..." >&2
    cd "${PLUGIN_ROOT}/mcp-server"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.appVersion=v${VERSION}" -o "$BINARY_PATH" .
  else
    die "Failed to download ${URL} (HTTP ${HTTP_CODE}) and Go is not available for fallback build"
  fi
else
  gunzip -f "${BINARY_PATH}.gz"
fi

chmod +x "$BINARY_PATH"
echo "$VERSION" > "$MARKER"
echo "setup.sh: installed ${BINARY_NAME} v${VERSION} at ${BINARY_PATH}" >&2
