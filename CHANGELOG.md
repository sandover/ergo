# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.1] - 2026-02-02

### Changed
- `list` summaries now match the active view scope (e.g., `--ready` only reports ready counts).
- `list --epic <id>` now includes done/canceled children by default (unless `--ready` is set).
- `list --epics` output uses the same visual language as the main list view.

### Fixed
- `list` now renders consistent empty-state messages for view-scoped filters.
- `list` rejects conflicting flags (e.g., `--ready` + `--all`) with a clear error.

### Documentation
- Refresh list examples and screenshots to match the updated list rendering.

### Tests
- Expand `list` integration coverage for empty states, summaries, and `--json` variants.
## [0.9.0] - 2026-01-31

### Added
- `prune` command to remove closed work (logical delete). Default is dry-run; run with `--yes` to apply. Only `done`/`canceled` tasks are pruned, then any now-empty epics.
- Human-friendly prune output with color, icons (✓ ✗ Ⓔ), and clear messaging matching `ergo list` style.

### Changed
- Clarify retention story: `prune` is logical removal; `compact` physically rewrites the log.

## [0.8.0] - 2026-01-30

### Changed
- **BREAKING:** Remove `worker` from the domain model, CLI I/O, and tree rendering.
- **BREAKING:** Remove `list --blocked` flag; use `list` or `list --ready`.
- JSON stdin parsing is strict (unknown keys rejected) and enforces single-value input.
- `claim` supports `--epic` filtering when claiming oldest ready work.

### Fixed
- `list --ready` no longer shows completed tasks in human output.

### Tests
- Add coverage for epic-filtered claiming and strict JSON parsing edge cases.

## [0.7.2] - 2026-01-30

### Fixed
- `init` is idempotent and now repairs missing `.ergo/lock` or `.ergo/events.jsonl` when `.ergo/` already exists.
- `new`/`set`/other write commands auto-create missing `.ergo/lock` on demand.
- Event log parsing tolerates a truncated final line and reports corruption with file+line context (including git conflict marker hints).

### Tests
- Reduce flakiness in concurrent-claim tests by retrying only on actual lock contention.

## [0.7.1] - 2026-01-30

### Documentation
- Clarify that agent claimant identity should be <model>@<hostname>.
- Remove personal identifiers from docs and examples.

### Tests
- Use generic example claimant identities in fixtures.

## [0.7.0] - 2026-01-30

### Changed
- **BREAKING:** Removed `--lock-timeout`; locking is now fail-fast.
- **BREAKING:** Removed `--readonly` flag and read-only guardrails.
- `claim`: simplify flow and provenance in CLI output.
- `show`: display epic children in output.

### Documentation
- Add file headers for navigation.

### Refactor
- Extract helpers in `show` command.

## [0.6.0] - 2026-01-29

### Changed
- **BREAKING:** Title and body are separate fields (no implicit title-from-body).
- **BREAKING:** Removed `next`; use `claim` (no args) to claim oldest READY task.
- Legacy archives without titles are auto-migrated from the first non-empty, non-heading body line.
- JSON outputs now carry explicit title/body fields where applicable.

### Fixed
- List rendering: added proper spacing for epic titles

## [0.5.8] - 2026-01-19

### Fixed
- List rendering now skips heading-only first lines and falls back to the first meaningful line.
- Tree view layout now uses a single build/measure/truncate pipeline for stable ID alignment.
- Tree view alignment refactor with explicit ID layout contract and consistent margins

## [0.5.7] - 2026-01-19

### Fixed
- CI lint now uses pinned golangci-lint; JSON error output checks WriteJSON errors
- Tree view: collapsed epic IDs align and render in default color

### Documentation
- Clarify that epics have no intrinsic state (derived from child tasks)

## [0.5.6] - 2026-01-18

### Added
- `set`: implicit claim when transitioning to `doing`/`error`

### Changed
- `list`: default to active tasks; leaner JSON output

## [0.5.5] - 2026-01-18

### Changed
- `show`: render Markdown in TTY output (via Glamour)

## [0.5.4] - 2026-01-16

### Added
- `Taskfile.yml` for common dev workflows

### Changed
- CI linting modernized; quickstart tightened for agent consumption

## [0.5.3] - 2026-01-16

### Fixed
- CLI: enable Cobra `--version` flag; CI build path updated

## [0.5.2] - 2026-01-16

### Changed
- Adopt Cobra CLI and standard Go project layout; update goreleaser config

## [0.5.1] - 2026-01-16

### Fixed
- `compact` command now preserves body updates (previously only kept original body text)
- `compact` now preserves `updated_at` even when the last event was a no-op (state/body/worker unchanged)
- `compact` now preserves epic assignment changes (avoids treating the latest epic as the created epic)

## [0.5.0] - 2026-01-15

### Changed
- **BREAKING:** `list --json` now returns bare array `[...]` instead of `{"tasks":[...]}`
  - Consistent with `show --json` which returns bare object
  - Simpler pattern: list returns array, show returns object
  - Update your scripts: `jq '.tasks[]'` → `jq '.[]'`
- Help text now includes JSON OUTPUT section showing list and show response structures
- Help text compressed for better information density (consolidated examples, compacted schema)

## [0.4.5] - 2026-01-15

### Fixed
- Tree view: Long annotations (e.g., `@claimer` with long hostname) no longer cause task IDs to wrap to next line

## [0.4.4] - 2026-01-15

### Changed
- **Derived epic state:** Epics no longer have intrinsic state; state is computed from child tasks
- **Tree view cleaner:** Canceled tasks hidden by default; done epics collapse to `✓ Epic [N tasks]`
- `list --all` shows everything (canceled tasks, expanded done epics)
- JSON output always includes all tasks (agents filter themselves)

### Fixed
- `set` now rejects state/worker/claim on epics with clear error messages
- JSON `list` was returning empty array when tasks existed (filtering bug)

### Added
- `--all` flag for `list` command
- Unit tests for derived epic state, filtering/collapsing logic
- Integration tests for epic validation and JSON output completeness

## [0.4.2] - 2026-01-15

### Fixed
- **Title not stored:** Tasks/epics now correctly store title as first line of body (was storing body only, losing title)
- **`ergo set` silent:** Now prints task ID on success for agent confirmation

### Changed
- `ergo list --epics` now shows only epics (simple list format) instead of full tree
- Consolidated duplicate `extractTitle`/`firstLine` functions

### Added
- "FOR AGENTS" section in `--help` with explicit `--json` guidance
- Heredoc warning in `--help` for multi-line JSON bodies (prevents shell corruption)
- Test coverage for title+body storage and set output

## [0.4.1] - 2026-01-14

### Added
- Performance benchmarks and regression guards (`go test -bench=.`)
- CI improvements: multi-OS testing, race detection, linting

### Changed
- Documentation rewrite: agent-first framing, cleaner README, deduplicated quickstart

## [0.4.0] - 2026-01-14

### Changed
- **BREAKING:** `body` field is now required for `new task` and `new epic`

### Added
- Multi-line body examples in documentation

## [0.3.1] - 2026-01-14

### Fixed
- Documentation examples now use correct 6-char base32 ID format

## [0.3.0] - 2026-01-14

### Changed
- **BREAKING:** All input now via JSON stdin only (removed positional args, `--file`, `-`)
- Simplified API: one way to do things

### Removed
- Positional argument input for titles
- `--file` flag
- `-` stdin shorthand

## [0.2.0] - 2026-01-13

### Added
- Tree view for `list` command with hierarchical display
- Aligned columns and visual hierarchy in output

## [0.1.3] - 2026-01-13

### Fixed
- CI: pass `HOMEBREW_TAP_GITHUB_TOKEN` to goreleaser

## [0.1.2] - 2026-01-13

### Fixed
- Version injection via ldflags at build time

## [0.1.1] - 2026-01-13

### Changed
- Extract help/quickstart text to embedded `.txt` files

## [0.1.0] - 2026-01-13

### Added
- Initial release
- Tasks and epics with dependencies
- Append-only JSONL storage in `.ergo/`
- Concurrent-safe claims with `flock(2)`
- `list`, `show`, `new`, `set`, `next`, `dep` commands
- Human vs agent task filtering (`--as`)
- Task results/artifacts as project file references
- State machine with enforced transitions
- Epic-to-epic dependencies

[Unreleased]: https://github.com/sandover/ergo/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/sandover/ergo/compare/v0.9.0...v0.9.1
[0.8.0]: https://github.com/sandover/ergo/compare/v0.7.2...v0.8.0
[0.7.2]: https://github.com/sandover/ergo/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/sandover/ergo/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/sandover/ergo/compare/v0.5.8...v0.7.0
[0.6.0]: https://github.com/sandover/ergo/compare/v0.5.8...v0.6.0
[0.5.8]: https://github.com/sandover/ergo/compare/v0.5.7...v0.5.8
[0.5.7]: https://github.com/sandover/ergo/compare/v0.5.6...v0.5.7
[0.5.6]: https://github.com/sandover/ergo/compare/v0.5.5...v0.5.6
[0.5.5]: https://github.com/sandover/ergo/compare/v0.5.4...v0.5.5
[0.5.4]: https://github.com/sandover/ergo/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/sandover/ergo/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/sandover/ergo/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/sandover/ergo/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/sandover/ergo/compare/v0.4.5...v0.5.0
[0.4.0]: https://github.com/sandover/ergo/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/sandover/ergo/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/sandover/ergo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/sandover/ergo/compare/v0.1.3...v0.2.0
[0.1.3]: https://github.com/sandover/ergo/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/sandover/ergo/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/sandover/ergo/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/sandover/ergo/releases/tag/v0.1.0
