#!/usr/bin/env bash
set -euo pipefail

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.64.8}"

printf '%s\n' "[ci] go mod tidy"
go mod tidy

git diff --exit-code

printf '%s\n' "[ci] golangci-lint ${GOLANGCI_LINT_VERSION}"
go run github.com/golangci/golangci-lint/cmd/golangci-lint@"${GOLANGCI_LINT_VERSION}" run ./...

printf '%s\n' "[ci] go test -race ./..."
go test -race ./...
