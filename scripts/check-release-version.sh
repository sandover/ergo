#!/usr/bin/env bash
# Validate Ergo's Go module identity and its public references.
# An optional version may include the leading v used by Git tags.
# Go 2 and later require the matching /vN semantic import suffix.
# Current Go imports and README links must use the declared module path.
# This check performs no writes and must run before an immutable tag is pushed.

set -euo pipefail

if [[ "$#" -gt 1 ]]; then
  printf '%s\n' "usage: $0 [version]" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

module_path="$(go list -m -f '{{.Path}}')"
module_base="github.com/sandover/ergo"

if [[ "${module_path}" != "${module_base}" && ! "${module_path}" =~ ^${module_base}/v([2-9]|[1-9][0-9]+)$ ]]; then
  printf '%s\n' "error: go.mod declares invalid Ergo module path ${module_path}" >&2
  exit 1
fi

release_version=""
if [[ "$#" -eq 1 ]]; then
  release_version="${1#v}"
  if [[ ! "${release_version}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([-+][0-9A-Za-z.-]+)?$ ]]; then
    printf '%s\n' "error: release version must be semantic, for example 6.0.4 or v7.0.0-rc.1" >&2
    exit 1
  fi

  release_major="${BASH_REMATCH[1]}"
  expected_module="${module_base}"
  if (( release_major >= 2 )); then
    expected_module="${expected_module}/v${release_major}"
  fi

  if [[ "${module_path}" != "${expected_module}" ]]; then
    printf '%s\n' \
      "error: release ${release_version} requires module ${expected_module}; go.mod declares ${module_path}" >&2
    exit 1
  fi
fi

stale_imports="$(
  GIT_WORK_TREE="${repo_root}" git grep -n -E '"github\.com/sandover/ergo(/v[0-9]+)?/' -- '*.go' \
    | grep -v -F "\"${module_path}/" || true
)"
if [[ -n "${stale_imports}" ]]; then
  printf '%s\n%s\n' "error: first-party Go imports do not match ${module_path}:" "${stale_imports}" >&2
  exit 1
fi

readme_modules="$(grep -oE 'github\.com/sandover/ergo/v[0-9]+' README.md | sort -u || true)"
expected_readme_modules=""
if [[ "${module_path}" != "${module_base}" ]]; then
  expected_readme_modules="${module_path}"
fi
if [[ "${readme_modules}" != "${expected_readme_modules}" ]]; then
  printf '%s\n%s\n' "error: README module references must all be ${module_path}; found:" "${readme_modules}" >&2
  exit 1
fi
if ! grep -Fq "go install ${module_path}/cmd/ergo@latest" README.md; then
  printf '%s\n' "error: README must install ${module_path}/cmd/ergo@latest" >&2
  exit 1
fi

if [[ -n "${release_version}" ]]; then
  printf '%s\n' "Release preflight passed: ${release_version} matches ${module_path}."
else
  printf '%s\n' "Module path check passed: source and README use ${module_path}."
fi
