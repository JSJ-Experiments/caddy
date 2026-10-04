#!/usr/bin/env bash
# Userspace UDP impairment; no root privileges or kernel/network changes.
set -euo pipefail
cd "$(dirname "$0")/../.."
for scenario in loss reordered; do
  jitter=0
  [[ "$scenario" != reordered ]] || jitter=2ms
  BBR_TEST_SIZE_MIB=8 BBR_TEST_REPEAT=3 BBR_TEST_PROTOCOLS=h3 \
    BBR_TEST_DELAY=10ms BBR_TEST_JITTER="$jitter" BBR_TEST_LOSS=0.001 \
    BBR_TEST_SCENARIO="$scenario-20ms-0.1pct" scripts/bbrv1/smoke.sh
  cp dist/smoke.jsonl "dist/impair-$scenario.jsonl"
done
