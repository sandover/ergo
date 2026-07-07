# ergo architecture (maintainer reference)

This document explains how ergo is built and what invariants maintainers must preserve.
It is not a user manual—**the user manual is `ergo --help` and `ergo quickstart`**.

## Goals

- Keep the codebase small, legible, and mapped to the domain (task graphs for agents).
- Provide strong robustness guarantees: no silent corruption, helpful error messages, and safe concurrent use.
- Keep the CLI surfaces stable for agents (especially `--json`).
- Ensure documentation stays coherent: behavior, tests, and manuals must match.

## High-level mental model

ergo stores task graph state as an **append-only JSONL event log** in `.ergo/plans.jsonl`.
Every command rebuilds current state by **replaying** events into an in-memory graph, then performs a read or appends new events.

(For backwards compatibility, `.ergo/events.jsonl` is also supported if it already exists.)

This design is intentionally “boring”:
- Plain text storage is diffable and debuggable with common tools.
- Replay is fast enough for the intended scale (hundreds to low thousands of items).
- Concurrency safety is centralized in a single file lock.

## Data and persistence

### `.ergo/` layout

- `.ergo/plans.jsonl`: append-only event log (source of truth).
  - For backwards compatibility, `.ergo/events.jsonl` is also supported if it already exists.
- `.ergo/lock`: advisory lock file used to serialize commands and hold best-effort writer diagnostics while locked.

### Event log invariants

- **Append-only**: mutations append events; they do not rewrite existing lines.
- **Replayable**: state is reconstructed from events; the current state is not stored separately.
- **Tombstones**: prune writes tombstone events; pruned IDs are treated as non-existent.
- **Corruption tolerance**: replay tolerates a truncated final line (common after crashes/partial writes).

### Locking and concurrency

Commands that mutate or read the task graph acquire an exclusive `flock` on `.ergo/lock`.
Mutations validate and append their full event batch while holding that lock.
`list` and `show` also hold the lock while replaying the log, so they read a coherent snapshot.

Lock acquisition waits up to `--lock-timeout` (30s by default).
`--lock-timeout 0` is the explicit fail-fast mode for hooks and scripts.
When a command times out, the error includes best-effort holder metadata from `.ergo/lock` when available.

The lock file must not be deleted to recover from contention.
`flock` releases when the owning process exits; deleting the file can split processes across different inodes.

This yields two key properties:
- **No interleaved command batches**: each command emits its logical event set together.
- **Race-safe claiming**: “claim oldest ready” selects and writes under the same lock, so only one process wins.

## Core domain model

### Entities

- **Task**: the only stored entity type.
- **Leaf task**: a task with no children; it has state and may be claimed.
- **Container task**: a task with children. Containers are derived from child assignment, have no direct state/claim/results semantics, and complete when all children are done or canceled.

The CLI still uses `--epic <id>` as the parent-assignment flag for compatibility with existing ergonomics. In current behavior, that value is a container task ID.

### State machine

The allowed transitions and claim invariants live in code (the model layer) and must be enforced consistently:
- `doing` and `error` require a claim (to identify the agent).
- `todo`, `done`, `canceled` must have no claim.
- `blocked` may have a claim or not.

### Dependencies

- Dependencies are allowed between any two non-ancestor tasks.
- A task cannot depend on its own container, and a container cannot depend on one of its own children.
- Cycles are forbidden.
- If a dependency points at a container, the dependency is complete only when all children of that container are done or canceled.

## Code organization

The code is intentionally layered:

- **Model**: types and invariants (states, transitions, dependency rules).
- **Storage**: `.ergo` discovery + reading/writing events.
- **Replay/graph**: materialize state from events; readiness/blocking/compaction logic.
- **Commands**: implement CLI operations by combining the above layers.
- **Output**: stable JSON shapes and formatting helpers.
- **CLI wiring**: cobra wiring in `cmd/ergo`.

The most important architectural constraint is that **domain invariants are enforced in one place** (model/command logic),
and **tests assert the public behavior**.

## Documentation architecture (the “manual” contract)

ergo has two documentation surfaces, each with a distinct purpose:

### `ergo --help` (`internal/ergo/help.txt`)

The quick reference:
- One-screen overview for users who already know ergo.
- Command syntax + terse descriptions.
- Minimal prose and minimal scrolling.

### `ergo quickstart` (`internal/ergo/quickstart.txt`)

The complete reference manual:
- Teaches by runnable examples.
- Covers the entire surface area: rules, edge cases, workflows.
- The place agents consult when implementing or debugging.

### Documentation invariants

- **If it’s not in `help.txt` or `quickstart.txt`, it’s undocumented.**
- `help.txt` optimizes for brevity; `quickstart.txt` optimizes for completeness.
- Any behavior change must update: implementation + tests + manuals together.

## JSON contracts

Agents rely on `--json` for machine-safe automation.
When `--json` is set:
- Commands emit a single JSON value to stdout (object or array) on success.
- Error paths should be deterministic and human-actionable.
- Schema evolution should be additive where possible.

If you must change JSON shape semantics, treat it like a public API change:
update tests, update manuals, and call out the change clearly.

## Working agreements for contributors

- Keep changes focused. Prefer small, high-signal diffs.
- Avoid hidden state and environment-variable configuration (except secrets).
- Prefer pure functions for logic that can be isolated and tested.
- When in doubt, update the manuals early—agents are primary users.
