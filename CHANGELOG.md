# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-07-20

### Added

- Added direct lifecycle commands: `done`, `block`, `cancel`, and `release`.
- Added direct `title`, `body`, and `move` commands for focused task changes.
- Specific `claim` now resumes blocked, done, canceled, and legacy-error work while reusing the original task ID.
- Claim JSON now returns exact `next_commands` for every valid lifecycle exit.
- Lifecycle commands accept `--result <path>`, optional `--summary <text>`, and an optional piped body in one atomic mutation.

### Changed

- **BREAKING:** Existing-task mutation now uses intent-specific commands instead of positional mutation JSON.
- **BREAKING:** A claim exists exactly while state is `doing`; every forward lifecycle exit clears it.
- **BREAKING:** New commands no longer create `error`. Use `release` for retryable unfinished work or `block` for an identified impediment.
- **BREAKING:** Direct mutation JSON reports the command `kind`, current state, and only fields that actually changed in `updated_fields`. True no-ops return an empty array.
- Lifecycle commands establish their named postcondition directly from any applicable readable state instead of requiring intermediate transitions.

### Removed

- Removed the generic `set` command without an alias.
- Removed public creation of the legacy `error` state.
- There is no `reopen` command or operation that returns done or canceled work to unclaimed `todo`; use a specific `claim` when closed work resumes.

### Upgrade from v1

| V1 intent | V2 command |
| --- | --- |
| Set a claim or move to `doing` | `ergo claim <id> --agent <identity>` |
| Set `done` | `ergo done <id>` |
| Set `blocked` | `ergo block <id>` |
| Set `canceled` | `ergo cancel <id>` |
| Return doing, blocked, or legacy-error work to `todo` | `ergo release <id>` |
| Resume blocked, done, canceled, or legacy-error work | `ergo claim <id> --agent <identity>` |
| Replace `title` | `ergo title <id> <title>` |
| Replace a body from stdin | `printf ... | ergo body <id>` |
| Set or clear `epic` | `ergo move <id> <container-id>` or `ergo move <id> --root` |
| Attach a result while exiting | add `--result <path>` and optional `--summary <text>` to the lifecycle command |
| Attach a late result | repeat the task's current lifecycle command with `--result` |
| Record a retryable failed attempt | `ergo release <id>` with an optional body and result |
| Record a failure that needs intervention | `ergo block <id>` with an optional body and result |

### Compatibility

- Existing `plans.jsonl` and legacy `events.jsonl` repositories open without migration.
- Historical `error` and claimed-blocked tasks remain readable and survive compaction without semantic changes.
- A direct lifecycle command normalizes only the legacy task it addresses; older Ergo binaries may continue appending replayable events.

## [1.2.0] - 2026-07-18

### Added
- Ergo now runs natively on Windows for AMD64 and ARM64 systems.
- GitHub releases now include ZIP archives for both Windows architectures.
- Windows users with Go installed can install Ergo with `go install github.com/sandover/ergo/cmd/ergo@latest`.

### Improved
- File locking and event-log replacement now use platform-specific implementations on Windows and Unix.
- Result paths reject Windows root-relative forms that could escape the project directory.
- Locking and storage integration tests are now portable across supported operating systems.

### Compatibility
- Existing commands, JSON output, and task logs remain compatible.
- macOS and Linux installation and release artifacts are unchanged.

## [1.1.0] - 2026-07-08

### Improved
- Ergo now handles parallel agents more gracefully. When another Ergo process is working in the same repo, commands wait briefly instead of failing immediately.
- Claiming work is safer under contention: parallel agents can race for ready tasks without double-claiming the same item.
- Task updates now apply as one coherent command, so active agents are much less likely to see partial work in progress.
- `list` and `show` now read a consistent snapshot while other Ergo commands are running.

### Compatibility
- No workflow changes are required.
- Existing commands and JSON output shapes remain compatible.

## [1.0.0] - 2026-05-22

### Changed
- **BREAKING**: stdin to `new task` is now the task body (plain text or Markdown); metadata goes in the JSON argument.
- **BREAKING**: stdin to `set <id>` replaces the task body; metadata goes in the JSON argument.
- **BREAKING**: `plan --file <path>` takes a Markdown file split on `---` lines; each chunk becomes a child task.
- **BREAKING**: `result` replaces `result_path` / `result_summary` as the result field. Old logs are still readable.
- `--help` rewritten: state machine intro, "ready" semantics, workflow-first command order.

### Removed
- `ergo new epic` — use `ergo new task` with child tasks to form a container.
- `--body-stdin` flag — stdin is always the body when piped.
- `show --short` flag — use `ergo --json show <id>` for structured output.
- `claim --epic <id>` flag — use `ergo --json list --epic <id> --ready` to scope work.

## [0.11.2] - 2026-05-18

### Changed
- Redesigned `--help` for cold-start model comprehension: mental model up top,
  worked lifecycle example before command reference, trimmed command descriptions.
- Replaced FOR AGENTS prose with a 2-line RULES section (negative constraints only).
- Added "CHOOSE INPUT MODE" section to quickstart for multi-line body guidance.
- Clarified sequential ergo usage in the bundled planning skill.

### Fixed
- Models no longer agonize over heredocs vs printf vs jq for multi-line input;
  `--body-stdin` pattern is now demonstrated inline in `--help`.

## [0.11.1] - 2026-03-01

### Fixed
- `ergo plan` gives better typo hints in JSON input.
- Top-level typos now suggest top-level fields only (`title`, `body`, `tasks`).
- Typos inside task objects can now suggest task fields, including `after`.
- Common transposed typos like `aftre` now get the expected hint: `after`.

### Why this matters
- Less guesswork when writing plan JSON by hand.
- Fewer misleading “did you mean …” messages.

### Compatibility
- No command or flag changes.
- Unknown fields are still rejected; this only improves hint quality.
- Error shape is unchanged (`parse_error` + `invalid` map).

### Tests
- Added regression tests for top-level and nested typo hints so this behavior stays stable.

## [0.11.0] - 2026-02-28

### Added
- New `ergo plan` command to create a full feature plan in one step (epic + tasks + ordering) from a single JSON payload.
- Structured `--json` response for `plan` with the created epic, tasks, and dependency edges.

### Changed
- Multi-event writes now use atomic replacement, which avoids partial writes if something fails mid-operation.
- `compact` now uses the same atomic write path for consistency.
- Task runner descriptions in `Taskfile.yml` are clearer and more intent-focused.

### Fixed
- `ergo plan` now preserves title/body text exactly as provided, matching existing `new task` and `new epic` behavior.

### Why this matters
- You can create and sequence a real feature plan with one command instead of many manual steps.
- Failures are safer: plan creation no longer risks leaving partially-written state.

### Compatibility
- No removals or breaking flag changes in this release.
- `plan` follows strict JSON input validation and returns actionable `parse_error` / `validation_failed` responses.

### Tests
- Added unit, command-path, integration, and storage tests for `plan` behavior, validation failures, dependency semantics, and atomic rollback guarantees.

### Documentation
- Updated `ergo --help`, `ergo quickstart`, and `docs/spec.md` to cover `plan` behavior and JSON/error contracts.

## [0.10.3] - 2026-02-18

### Changed
- `show <id>` human output is Markdown-first while preserving the existing JSON show contract.

## [0.10.2] - 2026-02-18

### Fixed
- `show <task-id>` header metadata is now dense (no blank spacer lines between `epic`, `claim`, `created`, `updated`, dependency lines, and `Results`).

### Tests
- Added regression coverage for dense task-show headers to prevent spacing drift.

## [0.10.1] - 2026-02-18

### Changed
- `show <epic-id>` human output is now document-first: epic body first, compact child task rows second, metadata footer last.
- Epic child ordering in `show` now follows dependency order (matching `list`) instead of lexicographic ID order.

### Documentation
- `ergo quickstart` now explicitly notes that `ergo --json show <epic-id>` returns `epic` plus `children`, and each child includes `body`.

### Tests
- Added regression coverage for epic child ordering in both human and JSON `show` output.
- Added regression coverage for document-first epic layout and for keeping task-level `show` formatting unchanged.

## [0.10.0] - 2026-02-16

### Changed
- Event log file renamed from `events.jsonl` to `plans.jsonl` for clarity.
- Existing repositories with `events.jsonl` continue to work (backwards compatible).
- New repositories initialized with `ergo init` will use `plans.jsonl`.

## [0.9.3] - 2026-02-05

### Fixed
- `list` now shows progress context in active epics.
- `--body-stdin` accepts empty bodies and supports TTY input.

### Documentation
- Add suggested git hooks and pre-push CI parity hook docs.
- Update agent structured-output hints.

## [0.9.2] - 2026-02-03

### Added
- `--body-stdin` mode for `new`/`set`: stdin is treated as literal body text; metadata is passed via flags.
- `claim --json` now includes an additive `reminder` field.

### Changed
- **BREAKING:** Replace `dep` with `sequence` for dependency ordering.
  - Old: `ergo dep A B` (A depends on B)
  - New: `ergo sequence B A` (B then A; same relationship)
  - New: `ergo sequence A B C` for ordered chains
  - New: `ergo sequence rm A B` removes the implied edge
- `claim` output now includes a reminder: “When you have completed this claimed task, you MUST mark it done.”

### Documentation
- Align README, `ergo --help`, and `ergo quickstart` with the supported input patterns (JSON stdin, `--body-stdin`, and flags-only when stdin is a TTY).

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
- `init` is idempotent and now repairs missing `.ergo/lock` or event log file when `.ergo/` already exists.
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

[Unreleased]: https://github.com/sandover/ergo/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/sandover/ergo/compare/v1.2.0...v2.0.0
[1.2.0]: https://github.com/sandover/ergo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/sandover/ergo/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/sandover/ergo/compare/v0.11.2...v1.0.0
[0.11.0]: https://github.com/sandover/ergo/compare/v0.10.3...v0.11.0
[0.10.3]: https://github.com/sandover/ergo/compare/v0.10.2...v0.10.3
[0.10.2]: https://github.com/sandover/ergo/compare/v0.10.1...v0.10.2
[0.10.1]: https://github.com/sandover/ergo/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/sandover/ergo/compare/v0.9.3...v0.10.0
[0.9.3]: https://github.com/sandover/ergo/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/sandover/ergo/compare/v0.9.1...v0.9.2
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
