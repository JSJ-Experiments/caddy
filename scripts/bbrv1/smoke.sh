#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
size_mib=${BBR_TEST_SIZE_MIB:-16}
repeat=${BBR_TEST_REPEAT:-2}
scenario=${BBR_TEST_SCENARIO:-loopback}
probe_args=()
if [[ -n ${BBR_TEST_DELAY:-} ]]; then
  probe_args=(-delay "$BBR_TEST_DELAY" -jitter "${BBR_TEST_JITTER:-0}" -loss "${BBR_TEST_LOSS:-0}")
fi
tmp=$(mktemp -d)
pid=''
cleanup() {
  if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 \
  -keyout "$tmp/key.pem" -out "$tmp/cert.pem" -subj '/CN=localhost' \
  -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' >/dev/null 2>&1
dd if=/dev/urandom of="$tmp/payload.bin" bs=1M count="$size_mib" status=none
digest=$(sha256sum "$tmp/payload.bin" | cut -d' ' -f1)
range_digest=$(head -c 1048576 "$tmp/payload.bin" | sha256sum | cut -d' ' -f1)
cat > "$tmp/Caddyfile" <<EOF
{
  admin off
  auto_https off
  grace_period 1s
}
https://127.0.0.1:18443 {
  bind 127.0.0.1
  tls $tmp/cert.pem $tmp/key.pem
  root * $tmp
  file_server
}
EOF
mkdir -p dist
: > dist/smoke.jsonl
for mode in stock reno bbrv1; do
  binary=dist/caddy-bbrv1
  [[ "$mode" != stock ]] || binary=dist/caddy-stock
  CADDY_QUIC_CONGESTION="$mode" "$binary" run --config "$tmp/Caddyfile" --adapter caddyfile >"$tmp/server.log" 2>&1 &
  pid=$!
  ready=false
  for attempt in $(seq 1 100); do
    if curl -ksf https://127.0.0.1:18443/payload.bin -H 'Range: bytes=0-0' -o /dev/null; then ready=true; break; fi
    if ! kill -0 "$pid" 2>/dev/null; then cat "$tmp/server.log" >&2; exit 1; fi
    sleep .1
  done
  if [[ "$ready" != true ]]; then cat "$tmp/server.log" >&2; exit 1; fi
  if [[ "$mode" != stock ]]; then
    expected=false; [[ "$mode" != bbrv1 ]] || expected=true
    grep -q "\"bbrv1\":$expected" "$tmp/server.log"
  fi
  cp "$tmp/server.log" "dist/smoke-$mode.log"
  echo "Smoke-testing $mode (${BBR_TEST_PROTOCOLS:-h3 h2}, ranges and SHA-256)"
  for proto in ${BBR_TEST_PROTOCOLS:-h3 h2}; do
    dist/quicprobe -url https://127.0.0.1:18443/payload.bin -insecure -protocol "$proto" \
      -label "$scenario/$mode" -bytes "$((size_mib * 1048576))" -sha256 "$digest" -repeat "$repeat" "${probe_args[@]}" | tee -a dist/smoke.jsonl
    dist/quicprobe -url https://127.0.0.1:18443/payload.bin -insecure -protocol "$proto" \
      -label "$scenario/$mode" -range bytes=0-1048575 -bytes 1048576 -sha256 "$range_digest" "${probe_args[@]}" | tee -a dist/smoke.jsonl
  done
  kill "$pid"; wait "$pid" || true; pid=''
done

# A typo must not silently select Reno or leave a running listener behind.
if CADDY_QUIC_CONGESTION=invalid timeout 10s dist/caddy-bbrv1 run \
  --config "$tmp/Caddyfile" --adapter caddyfile > "$tmp/invalid.log" 2>&1; then
  echo 'Invalid congestion control unexpectedly succeeded.' >&2
  exit 1
fi
grep -q 'invalid CADDY_QUIC_CONGESTION' "$tmp/invalid.log"
cp "$tmp/invalid.log" dist/smoke-invalid.log
