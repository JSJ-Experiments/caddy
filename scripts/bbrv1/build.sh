#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
if [[ $(go env GOOS) != linux || $(go env GOARCH) != arm64 ]]; then
  echo 'This experiment targets linux/arm64.' >&2
  exit 1
fi
mkdir -p dist
sha=$(git rev-parse HEAD)
go build -trimpath -ldflags "-s -w -X github.com/caddyserver/caddy/v2.CustomVersion=v2.11.4-bbrv1-${sha:0:12}" -o dist/caddy-bbrv1 ./cmd/caddy
go build -trimpath -ldflags '-s -w' -o dist/quicprobe ./scripts/bbrv1/quicprobe

# Same upstream version, compiler and build flags; do not use an unknown system
# binary as the baseline. Keep the checkout outside the testbox-synced tree.
stock=$(mktemp -d)
trap 'rm -rf "$stock"' EXIT
git -C "$stock" init -q
git -C "$stock" fetch -q --depth 1 https://github.com/caddyserver/caddy.git e2eee6a7fce366321294c9c2a79f3146891dcbdf
git -C "$stock" checkout -q --detach FETCH_HEAD
out=$PWD/dist/caddy-stock
(cd "$stock" && go build -trimpath -ldflags '-s -w -X github.com/caddyserver/caddy/v2.CustomVersion=v2.11.4-stock-experiment' -o "$out" ./cmd/caddy)
{
  printf 'caddy_fork_commit=%s\n' "$sha"
  printf 'caddy_upstream_commit=e2eee6a7fce366321294c9c2a79f3146891dcbdf\n'
  printf 'quic_fork_commit=25a38bfc5715b8f5fd21be74951176a75d1bf602\n'
  go version
  go version -m dist/caddy-bbrv1
} > dist/build-info.txt
(cd dist && sha256sum caddy-bbrv1 caddy-stock quicprobe build-info.txt > SHA256SUMS)
