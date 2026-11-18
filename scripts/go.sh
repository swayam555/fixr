#!/usr/bin/env bash
# Runs the Go toolchain in a container (h3-go needs cgo; the host has no C compiler).
# Usage: scripts/go.sh test ./...   |   scripts/go.sh vet ./...
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
exec docker run --rm --network host \
  -v "$ROOT/services/go:/src" -w /src \
  -v fixr-gomod:/go/pkg/mod -v fixr-gocache:/root/.cache/go-build \
  -e CGO_ENABLED=1 -e GOFLAGS=-buildvcs=false \
  golang:1.26.11-trixie go "$@"
