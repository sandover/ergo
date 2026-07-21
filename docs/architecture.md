# Ergo architecture

This document is a maintainer reference. `ergo --help` and `ergo quickstart`
are the user manuals. `docs/spec.md` is the stable public contract.

## Design

Ergo stores task graph history as append-only JSONL and rebuilds current state
by replaying events. The design favors plain text, explicit inputs, and one
serialized write path over a database or daemon.

The active repository contains:

```text
.ergo/
├── plans.jsonl
└── lock
```

If a repository already has `.ergo/events.jsonl`, Ergo continues to use it.
Opening a repository does not rename or rewrite the log.

## Event storage and replay

Normal mutations append events. Replay constructs tasks, dependency maps,
reverse dependency maps, metadata timestamps, results, and tombstones in memory.
It tolerates a truncated final line but reports malformed complete lines with
the file and line number.

Prune appends tombstones. Compact is the only routine that replaces the event
file. It writes one lossless representation of the live graph and removes
pruned history. It preserves unresolved legacy error and claimed-blocked state
exactly.

Containers are derived. A root task becomes a container when another task's
parent ID points to it. Historical `new_epic` events remain replayable so empty
legacy containers stay visible.

## Locking

Reads and writes acquire an advisory lock on `.ergo/lock`. A mutation loads,
validates, builds its full event batch, appends, and reloads while holding the
lock. This prevents partial command effects and interleaved batches.

Oldest-ready claim selects and writes under the same lock. Concurrent agents can
race to claim; only one claims each task. List and show also lock so their
snapshots do not split a mutation batch.

The lock file is a synchronization inode, not state. Do not delete it during
contention. The operating system releases the lock when its process exits.

## Domain invariants

Forward states are todo, doing, blocked, done, and canceled. Historical error
is readable but never a new mutation target.

The mutation core enforces a postcondition instead of a transition table:

```text
state=doing  <=>  claimed_by is nonempty
```

Done, block, cancel, and release state their target condition directly. Claim
states doing plus an owner. This lets a direct command normalize legacy state
without intermediate events or command choreography.

A mutation request can include target state and claim, body replacement, result
attachment, title, or placement. Validation finishes before append. Same-value
content and same-state lifecycle calls suppress redundant events. Result append
and legacy claim cleanup still produce events when the state itself is unchanged.

## Tasks and containers

Task is the only stored entity. A leaf task carries lifecycle state. A container
has children and no direct lifecycle, claim, or result behavior. Completion is
derived when all children are done or canceled.

Placement validation keeps containers at one root level. A clean root todo task
may be promoted by receiving its first child. Containers cannot move or nest.
Moves also reject dependency edges between the prospective container and child.

## Dependencies and readiness

The graph stores directed `depends` edges. It rejects self edges, ancestry edges,
and cycles. A container dependency completes when every child is done or
canceled. A child also inherits dependencies assigned to its container.

Readiness is derived for todo leaf tasks whose direct and inherited dependencies
are complete. Explicit blocked is separate from todo work waiting on dependencies.

## Results

Result paths are validated relative to the project root while the mutation lock
is held. A path must remain inside the project, refer to a regular file, and
stay outside `.ergo/`. Attachment captures SHA-256, mtime, and the current git
commit when available. Replay orders results newest first.

## Code organization

- `model.go`: domain types and small invariants.
- `storage.go`: discovery, event I/O, and result provenance.
- `graph.go`: replay, derivation, readiness, and compaction.
- `mutation.go`: the shared atomic mutation path.
- `commands_*.go`: intent-specific command behavior.
- `output.go` and rendering files: stable JSON and human output.
- `cmd/ergo`: Cobra routing, global flags, and process-level errors.

Command handlers state intent. They do not duplicate event, claim, result, or
lock mechanics. Placement and lifecycle validation stay inside the locked path.

## Public surfaces

Agents depend on JSON output. Successful `--json` calls emit one value to stdout.
Mutation objects report only fields that changed. Output shape changes require
integration tests, manual updates, and a changelog note.

The documentation sources have distinct roles:

- `internal/ergo/help.txt`: complete compact command and flag reference.
- `internal/ergo/quickstart.txt`: complete example-led manual.
- `docs/spec.md`: stable behavioral contract.
- `docs/architecture.md`: implementation constraints.

Behavior, tests, and these sources must change together.
