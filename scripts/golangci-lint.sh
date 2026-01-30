#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.64.8}"
TOOLS_DIR="${ROOT}/.gocache/tools"
TOOLS_BIN="${TOOLS_DIR}/bin"
TOOLS_MOD="${TOOLS_DIR}/mod"
STAMP="${TOOLS_DIR}/golangci-lint.version"
BIN="${TOOLS_BIN}/golangci-lint"

export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-"${ROOT}/.gocache/golangci-lint-cache"}"

mkdir -p "${TOOLS_BIN}" "${TOOLS_MOD}"
mkdir -p "${GOLANGCI_LINT_CACHE}"

if [[ "${ERGO_LINT_QUIET:-0}" != "1" ]]; then
  printf '%s\n' "[lint] golangci-lint ${GOLANGCI_LINT_VERSION}"
fi

if [[ ! -x "${BIN}" ]] || [[ ! -f "${STAMP}" ]] || [[ "$(cat "${STAMP}")" != "${GOLANGCI_LINT_VERSION}" ]]; then
  printf '%s\n' "[lint] installing golangci-lint ${GOLANGCI_LINT_VERSION} (cached in .gocache/)"
  GOMODCACHE="${TOOLS_MOD}" GOBIN="${TOOLS_BIN}" \
    go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
  printf '%s' "${GOLANGCI_LINT_VERSION}" > "${STAMP}"
fi

exec "${BIN}" "$@"
