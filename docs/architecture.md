# ergo architecture (maintainer reference)

This document explains how ergo is built and what invariants maintainers must preserve.
It is not a user manual—**the user manual is `ergo --help` and `ergo quickstart`**.

## Goals

- Keep the codebase small, legible, and mapped to the domain (task graphs for agents).
- Provide strong robustness guarantees: no silent corruption, helpful error messages, and safe concurrent use.
- Keep the CLI surfaces stable for agents (especially `--json`).
- Ensure documentation stays coherent: behavior, tests, and manuals must match.

## High-level mental model

ergo stores task/epic state as an **append-only JSONL event log** in `.ergo/events.jsonl`.
Every command rebuilds current state by **replaying** events into an in-memory graph, then performs a read or appends new events.

This design is intentionally “boring”:
- Plain text storage is diffable and debuggable with common tools.
- Replay is fast enough for the intended scale (hundreds to low thousands of items).
- Concurrency safety is centralized in a single file lock.

## Data and persistence

### `.ergo/` layout

- `.ergo/events.jsonl`: append-only event log (source of truth).
- `.ergo/lock`: advisory lock file used to serialize writes.

### Event log invariants

- **Append-only**: mutations append events; they do not rewrite existing lines.
- **Replayable**: state is reconstructed from events; the current state is not stored separately.
- **Tombstones**: prune writes tombstone events; pruned IDs are treated as non-existent.
- **Corruption tolerance**: replay tolerates a truncated final line (common after crashes/partial writes).

### Locking and concurrency

All write operations are performed under an exclusive `flock` on `.ergo/lock`.
The lock is intentionally non-blocking (fail-fast): callers can retry on “lock busy”.

This yields two key properties:
- **No interleaved writes**: events remain line-delimited JSON objects.
- **Race-safe claiming**: “claim oldest ready” selects and writes under the same lock, so only one process wins.

## Core domain model

### Entities

- **Task**: unit of work, has state and (optional) claim.
- **Epic**: grouping/structure only (no state/claim), can have dependencies on other epics.

### State machine

The allowed transitions and claim invariants live in code (the model layer) and must be enforced consistently:
- `doing` and `error` require a claim (to identify the agent).
- `todo`, `done`, `canceled` must have no claim.
- `blocked` may have a claim or not.

### Dependencies

- Task-to-task dependencies are allowed.
- Epic-to-epic dependencies are allowed.
- Cross-kind dependencies (task↔epic) are forbidden.
- Cycles are forbidden.

## Code organization

The code is intentionally layered:

- **Model**: types and invariants (states, transitions, dep rules).
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
