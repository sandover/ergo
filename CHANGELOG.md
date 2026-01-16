# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/sandover/ergo/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/sandover/ergo/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/sandover/ergo/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/sandover/ergo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/sandover/ergo/compare/v0.1.3...v0.2.0
[0.1.3]: https://github.com/sandover/ergo/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/sandover/ergo/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/sandover/ergo/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/sandover/ergo/releases/tag/v0.1.0
