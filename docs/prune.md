<!--
Title: ergo prune (public spec)
Purpose: Specify the v1 user-facing semantics and output contract for the destructive `ergo prune` command.
Role: Source-of-truth for implementation, tests, help/quickstart, and release review.
Invariants: Prune is append-only (logical delete); `compact` is the only physical delete; pruned IDs are treated as non-existent.
-->

# ergo prune (public spec)

`ergo prune` is the **public**, **destructive** command that **logically deletes** items from the task collection (tasks and epics) using a fixed policy.
It is append-only: history remains in the event log until `ergo compact` rewrites the log.

## Goals

- Provide a minimal, deterministic deletion primitive that composes cleanly with replay, `list`, `show`, `claim`, `set`, `dep`, and `compact`.
- Make accidental deletion hard (explicit confirmation).
- Keep concurrency safety identical to other write commands (single append under the existing lock).

## Non-goals (v1)

- Secure delete / privacy guarantees (pruned events remain in the log until `compact`).
- Undelete / restoration.
- Automatic retention policies (prune-by-age/size, background compaction, etc.).
- Per-ID targeting (task IDs, epic IDs) or any other selection flags.

## Syntax

Collection-wide policy only (v1):

    ergo prune [--yes] [--json]

## Safety / Confirmation

- With `--yes`: performs the prune (writes).
- Without `--yes`: performs a **dry-run** (no writes) that explains what *would* be pruned and how to actually run it (`--yes`).
- There is no interactive prompt in v1; agents can rely on a stable non-interactive contract.

## Flags

- `--yes`: required to perform the prune (write). Without it, prune is a dry-run.
- `--json`: machine-readable output contract (see below).

## Prune policy (v1)

Prune touches only “closed” work:

- Tasks in `done` or `canceled` are pruned.
- Tasks in `todo` or `blocked` are preserved.
- Tasks in `doing` or `error` are preserved and **must not be pruned**.

Epics are structural:

- Any epic that has **no remaining children** after the task pruning pass is pruned.
- No explicit `--cascade` option exists; this is the default behavior.

## Invariants (v1)

- Prune is append-only logical deletion; `compact` is the only physical deletion.
- Default is dry-run; `--yes` is required to write.
- Eligible tasks: `done`, `canceled`. Preserved: `todo`, `blocked`, `doing`, `error`.
- After pruning eligible tasks, any now-empty epics are pruned.
- Pruned IDs are treated as non-existent across `list`/`show`/`claim`/`set`/`dep`.
- Dependency edges to/from pruned IDs are dropped; dependents are not blocked by pruned IDs.
- Replay tolerates delete marker before create (delete wins).

## Entities after prune

When an item is pruned, it becomes **non-existent**:

- It does not appear in `list`.
- It cannot be claimed, set, or used as a dependency endpoint.

## Dependency behavior

When an ID is pruned, it is treated as **non-existent**:

- Outbound edges from pruned nodes are dropped (node removed).
- Inbound edges to pruned nodes are dropped. Dependents are **not blocked** by pruned nodes.

Rationale: a deleted node should not keep other work blocked forever; dropping edges matches the non-existent model.

## Behavior across commands

- `ergo list` / `ergo list --json`: never shows pruned IDs.
- `ergo show <id>`:
  - If `<id>` is pruned and prune history is still present, returns an error that explicitly indicates the ID was pruned.
  - If prune history is not present (e.g. after `compact`), returns the normal not-found error.
- `ergo claim [<id>]`: refuses to claim pruned tasks; pruned tasks are excluded from oldest-ready selection.
- `ergo set <id>`: fails for pruned IDs (no resurrection).
- `ergo dep <A> <B>` / `ergo dep rm <A> <B>`: fails if either endpoint is pruned.

## Idempotency and not-found semantics

- Prune is policy-based and may be a no-op (nothing eligible).
- Distinguishing “never existed” vs “was pruned” is still relevant for downstream commands like `show`:
  - If the prune marker is still present in the log, errors should say “pruned”.
  - After `compact`, prune markers may be gone; errors may only be “not found”.

## Output contract

### Human output (default)

- Dry-run (no `--yes`): prints a summary of what would be pruned and how to actually run prune (`--yes`).
- Execution (`--yes`): prints a summary of what was pruned.
- Each item line includes ID, status (task state or `epic`), and title.

Exact wording is not specified here; it must be stable enough for humans but agents should prefer `--json`.

### JSON output (`--json`)

`--json` prints exactly one JSON object to stdout on success:

- `kind`: `"prune"`
- `dry_run`: boolean
- `pruned_ids`: JSON array of IDs that were (or would be, for dry-run) pruned (tasks and epics)

Notes:

- `pruned_ids` may be empty.
- Ordering of `pruned_ids` is unspecified.

## Event / replay semantics (implementation-facing)

- Prune is represented as a single append-only delete marker event targeting an ID.
- Pruned IDs are treated as non-existent during materialization.
- Replay must tolerate a delete marker appearing before a create (delete wins).

## `compact` interaction (physical cleanup)

- `ergo prune` is **logical deletion**.
- `ergo compact` is **physical deletion**: it rewrites the log without pruned entities or their historical events.

Privacy note: prune is not a privacy delete until compact runs; sensitive data may remain in history.

## Compatibility note

`ergo prune` is the only public CLI surface for deletion.
