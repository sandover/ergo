# ergo public CLI spec (contracts)

This document defines **stable contracts** for ergo’s CLI behavior.
It is not the primary user manual; users should read `ergo --help` and `ergo quickstart`.

## Scope

This spec covers:
- Output and exit-code guarantees (especially `--json`)
- State machine and claim invariants
- Dependency kind rules and cycle prevention
- Prune/compact semantics and “pruned ID” behavior
- Concurrency and locking behavior (user-visible consequences)

## Output and exit code contracts

### General

- Success returns exit code `0`.
- Failures return a non-zero exit code and print an informative error to stderr.
- When `--json` is set, commands should avoid emitting non-JSON noise on stdout.

### `--json` contract

When `--json` is set and a command succeeds:
- Exactly **one JSON value** is written to stdout (object or array).
- JSON must be parseable by strict decoders (no trailing non-JSON).
- Schema changes should be additive when possible.

Commands that are expected to be machine-used should offer a useful JSON shape:
- `list --json`: array of items
- `show --json`: object (or object-with-children for epics)
- Mutations (`new`, `set`, `dep`, `prune`, `compact`, `claim`): JSON object(s)

### “No ready tasks” for `claim`

When `ergo claim` (oldest-ready mode) finds no claimable tasks:
- This is a **successful** outcome (exit code `0`).
- Human output prints a clear message.
- With `--json`, output is a JSON object that explicitly indicates “no ready”.

## State machine and claim invariants

### States

Tasks have a state in:
`todo | doing | done | blocked | canceled | error`.

Epics are structural and do not have state.

### Claim invariants

- `doing` requires a claim.
- `error` requires a claim (to show who failed).
- `todo`, `done`, `canceled` must have **no** claim.
- `blocked` may have a claim or not.

### Transition rules

Transitions are constrained by a fixed state machine (see code for full matrix).
Notably:
- `done` and `canceled` can be reopened to `todo`.
- `error` can transition to `doing` (retry), `todo` (reassign), or `canceled` (give up).

## IDs and entities

- IDs are 6-character, uppercase, short identifiers.
- Entities:
  - **Task**: can be claimed and has state.
  - **Epic**: cannot be claimed and has no state; acts as a grouping node.

## Dependency rules

- Task→task dependencies are allowed.
- Epic→epic dependencies are allowed.
- Task↔epic dependencies are forbidden.
- Self-dependencies are forbidden.
- Creating a dependency that would introduce a cycle is rejected.

## Prune and compact (deletion model)

ergo deletion is **two-phase**:

1) `prune` performs **logical deletion** (tombstones).
2) `compact` performs **physical deletion** (rewrites the event log).

### Prune policy

Policy-based (no per-ID selection):
- Tasks in `done` or `canceled` are eligible.
- Tasks in `todo`, `doing`, `blocked`, `error` are preserved.
- After pruning eligible tasks, any epic with no remaining children is pruned.

### Pruned ID behavior (tombstones)

When an ID is pruned, it is treated as **non-existent**:
- It does not appear in `list`.
- It cannot be used as a dependency endpoint.
- It cannot be claimed or updated.

Dependencies to/from pruned IDs are dropped; a pruned dependency must not keep other work blocked.

### Post-compact behavior

After `compact`, the history of pruned IDs may be removed from the log:
- `show <id>` for a previously pruned ID may no longer be distinguishable from “never existed”.

## Concurrency and locking (user-visible behavior)

- Mutations are serialized by an advisory lock on `.ergo/lock`.
- Lock acquisition is fail-fast:
  - When the lock is held by another process, mutations fail quickly with a “lock busy” error.
  - Callers should retry.
- `claim` oldest-ready is race-safe: only one process can claim a given task.
