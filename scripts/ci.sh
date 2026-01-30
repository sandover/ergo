#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOCACHE="${GOCACHE:-"${ROOT}/.gocache/go-build"}"
export GOMODCACHE="${GOMODCACHE:-"${ROOT}/.gocache/go-mod"}"
mkdir -p "${GOCACHE}" "${GOMODCACHE}"

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.64.8}"

printf '%s\n' "[ci] go mod tidy"
go mod tidy

git diff --exit-code -- go.mod go.sum

printf '%s\n' "[ci] golangci-lint ${GOLANGCI_LINT_VERSION}"
ERGO_LINT_QUIET=1 GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION}" ./scripts/golangci-lint.sh run ./...

printf '%s\n' "[ci] go test -race ./..."
go test -race ./...
