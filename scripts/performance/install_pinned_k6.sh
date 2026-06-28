#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
K6_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/performance/k6/K6_VERSION")"
INSTALL_DIR="$ROOT_DIR/.tools/k6/$K6_VERSION"
TMP_DIR="${TMPDIR:-/tmp}/aandi-k6-$K6_VERSION"
BASE_URL="https://github.com/grafana/k6/releases/download/$K6_VERSION"

case "$(uname -s)" in
  Darwin) os="macos" ;;
  Linux) os="linux" ;;
  *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [[ "$os" == "macos" ]]; then
  archive="k6-$K6_VERSION-$os-$arch.zip"
else
  archive="k6-$K6_VERSION-$os-$arch.tar.gz"
fi

mkdir -p "$INSTALL_DIR" "$TMP_DIR"
if [[ -x "$INSTALL_DIR/k6" ]]; then
  "$INSTALL_DIR/k6" version
  echo "$INSTALL_DIR/k6"
  exit 0
fi

curl -fsSL "$BASE_URL/$archive" -o "$TMP_DIR/$archive"
curl -fsSL "$BASE_URL/k6-$K6_VERSION-checksums.txt" -o "$TMP_DIR/checksums.txt"

if command -v shasum >/dev/null 2>&1; then
  (cd "$TMP_DIR" && grep "  $archive$" checksums.txt | shasum -a 256 -c -)
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP_DIR" && grep "  $archive$" checksums.txt | sha256sum -c -)
else
  echo "Missing checksum command: shasum or sha256sum" >&2
  exit 1
fi

if [[ "$archive" == *.zip ]]; then
  unzip -q -o "$TMP_DIR/$archive" -d "$TMP_DIR"
else
  tar -xzf "$TMP_DIR/$archive" -C "$TMP_DIR"
fi

found=""
while IFS= read -r candidate; do
  if [[ -x "$candidate" ]]; then
    found="$candidate"
    break
  fi
done < <(find "$TMP_DIR" -type f -name k6)
if [[ -z "$found" ]]; then
  found="$(find "$TMP_DIR" -type f -name k6 | head -n1)"
fi
if [[ -z "$found" ]]; then
  echo "Downloaded archive did not contain a k6 binary." >&2
  exit 1
fi

cp "$found" "$INSTALL_DIR/k6"
chmod +x "$INSTALL_DIR/k6"
"$INSTALL_DIR/k6" version
echo "$INSTALL_DIR/k6"
