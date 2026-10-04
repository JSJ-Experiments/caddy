#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
go mod verify
go test -race -short ./...
go test -race -short github.com/quic-go/quic-go/internal/congestion github.com/quic-go/quic-go/internal/ackhandler
go vet ./...
