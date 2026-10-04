# Experimental QUIC BBRv1 build — not a production release

AI-assisted implementation using OpenAI Codex. No upstream PR is being submitted.

## Pinned sources

- Caddy v2.11.4: `e2eee6a7fce366321294c9c2a79f3146891dcbdf` (same release as this machine).
- quic-go v0.59.1: `438abf0e467326af9fd964636b4cc18cfbaf5298`.
- BBRv1 proof of concept from tdragoun/quic-go, commits
  `bfd220a12dfa85240ed893be3ae2ea75bbe22d98`,
  `9cc335e6f4df63b949b4998ddd063ed52f49847f`,
  `a07eb48492755adb24d4f278a92f5e054f1eccad`.
- Those patches were cherry-picked onto the exact upstream quic-go release in
  https://github.com/JSJ-Experiments/caddy-quic-go/tree/caddy-bbrv1.
  Unlike replacing with the original POC branch, this retains the release's
  HTTP/3 trailer-validation fix. The fork uses quic-go's MIT license; Caddy
  remains Apache-2.0. The port attributes the original QUICHE/For-ACGN code.

The POC is not audited or supported by upstream Caddy/quic-go. Its factory
API, bandwidth sampling, RTT handling, fairness, and connection migration
need further review before production deployment. Passing a download benchmark
does not establish safety or compatibility with every workload.

## Switching algorithms

`CADDY_QUIC_CONGESTION=bbrv1` enables the experimental controller for HTTP/3.
Unset the variable, or use `reno`, for the default NewReno controller.
Unknown values fail listener creation rather than silently falling back.
Selection is logged as `experimental QUIC congestion control`, `bbrv1=true/false`.
Each QUIC connection gets a distinct controller. TCP settings are untouched.

This must be configured in `NetworkAddress.ListenQUIC`, not just
`http3.Server.QUICConfig`: Caddy supplies an already-created QUIC listener.
A full **process restart** is required to switch; a config reload can reuse
the old listener. Do not replace `/usr/bin/caddy` or edit the live service yet.

## Build and test (Blacksmith, native Linux ARM64)

The manual-only `build-bbrv1.yml` workflow downloads/verifies dependencies,
runs race-enabled short tests, builds the experimental binary and an exact
stock baseline, and smoke-tests actual HTTP/3/HTTP/2 transfers and ranges with
SHA-256 integrity checks. Go 1.26.3 matches the installed production compiler.
Artifacts include binaries, checksums, source/compiler provenance and logs.
Upstream multi-platform/release workflows are deliberately disabled on this branch.

Local entry points (prefer running these through the Blacksmith testbox):

```sh
scripts/bbrv1/check.sh
scripts/bbrv1/build.sh
scripts/bbrv1/smoke.sh
```

`dist/quicprobe` enforces the requested protocol and streams bytes into SHA-256
without buffering the file. Use `-insecure` only with isolated self-signed test
certificates. It does not redirect or silently fall back between protocols.
Loopback results do not predict a phone's real-network performance; compare
on the affected client/network before drawing conclusions.

No live Caddy configuration, binary, certificates or WebClip data are changed
by these scripts. Smoke tests bind loopback port 18443 and disable the admin API.
