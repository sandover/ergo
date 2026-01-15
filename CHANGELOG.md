# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
