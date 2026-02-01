# ergo release guide

This is the maintainer checklist for shipping a new ergo version.
User-facing docs live in `ergo --help` and `ergo quickstart`; this file is operational.

## Pre-flight (every release)

- Ensure working tree is clean and up to date with `main`.
- Run CI parity locally: `task ci` (tidy, lint, test).
- Verify docs and behavior are coherent:
  - `internal/ergo/help.txt` matches the current CLI surface area.
  - `internal/ergo/quickstart.txt` examples still work and cover new behavior.
  - `--json` outputs remain stable and machine-safe for agents.
- Sanity check install/build:
  - `task build` and smoke-run `./bin/ergo --help`.

## Versioning and changelog

- Prefer SemVer:
  - Patch: bugfixes, perf improvements, doc fixes (no contract changes).
  - Minor: additive features, additive JSON fields, new commands/flags.
  - Major: breaking behavior/JSON changes or removals.
- Update `CHANGELOG.md` with user-impacting changes:
  - Call out any behavior/contract changes explicitly (especially `--json` schemas).
  - Include upgrade notes when needed.

## Cutting a release (tag-driven)

- Choose a version tag (e.g., `v0.7.0`).
- Create and push the git tag.
- Let GitHub Actions / goreleaser build and publish artifacts.

## Post-release verification

- Verify the release artifacts run:
  - `ergo --version` matches the tag.
  - `ergo --help` and `ergo quickstart` render correctly on a real terminal.
- Verify agent-critical surfaces:
  - JSON output remains parseable (no extra stdout noise).
  - Error messages remain actionable (hint text still accurate).

## “Don’t break agents” rules

- Treat `--json` output as a public API:
  - Prefer additive changes (new fields) over shape changes.
  - Update `docs/spec.md` and integration tests for any contract changes.
- Keep `help.txt` and `quickstart.txt` as the complete manual:
  - If a user-visible behavior changes, update them in the same PR.
