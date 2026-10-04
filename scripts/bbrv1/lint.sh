#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL https://github.com/golangci/golangci-lint/releases/download/v2.14.0/golangci-lint-2.14.0-linux-arm64.tar.gz -o "$tmp/lint.tar.gz"
echo "ee7ec5f3453d15ddf106fae5a4d6c71737712348a979d1fe9cd52ec7ea299bae  $tmp/lint.tar.gz" | sha256sum -c -
tar -xzf "$tmp/lint.tar.gz" -C "$tmp" --strip-components=1
base=e2eee6a7fce366321294c9c2a79f3146891dcbdf
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch -q --depth 1 https://github.com/caddyserver/caddy.git "$base"
fi
"$tmp/golangci-lint" run --timeout 10m --new-from-rev="$base"
