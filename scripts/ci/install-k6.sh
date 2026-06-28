#!/usr/bin/env bash
set -Eeuo pipefail

version_file="${K6_VERSION_FILE:-performance/k6/K6_VERSION}"
cache_root="${K6_CACHE_DIR:-$HOME/.cache/k6}"
install_dir="${K6_INSTALL_DIR:-/usr/local/bin}"

if [ ! -f "$version_file" ]; then
  echo "Missing k6 version file: $version_file" >&2
  exit 1
fi

k6_version="$(tr -d '[:space:]' < "$version_file")"
case "$k6_version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *)
    echo "Invalid k6 version: $k6_version" >&2
    exit 1
    ;;
esac

if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
  echo "Unsupported k6 install target: $(uname -s) $(uname -m)" >&2
  exit 1
fi

cache_dir="$cache_root/$k6_version"
cache_bin="$cache_dir/k6"
mkdir -p "$cache_dir"

k6_actual_version() {
  "$1" version | awk 'NR == 1 { print $2 }'
}

assert_k6_version() {
  local binary="$1"
  local actual

  actual="$(k6_actual_version "$binary")"
  if [ "$actual" != "$k6_version" ]; then
    echo "k6 version mismatch for $binary: expected $k6_version, got $actual" >&2
    exit 1
  fi
}

if [ -x "$cache_bin" ]; then
  assert_k6_version "$cache_bin"
else
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  archive="$tmp_dir/k6.tar.gz"
  curl -fsSL "https://github.com/grafana/k6/releases/download/${k6_version}/k6-${k6_version}-linux-amd64.tar.gz" -o "$archive"
  tar -xzf "$archive" -C "$tmp_dir"
  install -m 0755 "$tmp_dir/k6-${k6_version}-linux-amd64/k6" "$cache_bin"
  assert_k6_version "$cache_bin"
fi

mkdir -p "$install_dir" 2>/dev/null || true
if [ -w "$install_dir" ]; then
  install -m 0755 "$cache_bin" "$install_dir/k6"
else
  sudo install -m 0755 "$cache_bin" "$install_dir/k6"
fi

assert_k6_version "$install_dir/k6"
"$install_dir/k6" version
