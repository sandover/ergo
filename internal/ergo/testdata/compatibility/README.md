# Released backlog compatibility fixtures

These files are immutable examples of backlogs written by released Ergo
binaries. They are the compatibility boundary for the event reader: current
Ergo must read each file and preserve its semantic state through compaction.
Tests may add separate synthetic records for malformed input, but must not
replace these fixtures with records produced through current Go event types.

Each fixture was captured by building the named release tag and running that
binary against an empty temporary repository. The scenario creates
an epic with two tasks, orders the second after the first, completes the first
with `go.mod` as a result, and leaves the second unfinished. Releases that
support lifecycle messages attach one to each lifecycle operation.

| Fixture | Release commit | Filename selected by release | Deliberate coverage |
| --- | --- | --- | --- |
| `v1.0.0-events.jsonl` | `3c3c762d3323c1677435b7c81b0a8e51233effc9` | `events.jsonl` | Oldest supported filename, retained `error` state and claim |
| `v2.0.0-plans.jsonl` | `18b629f3525756adb109a4f9a23ad1553890e6d9` | `plans.jsonl` | Lifecycle postconditions, result summary, unclaim |
| `v3.0.0-plans.jsonl` | `ebda77a425fdf53e3556bef8bb365be19fe51d6a` | `plans.jsonl` | Lifecycle messages and path-identified results |
| `v4.0.0-backlog.jsonl` | `f0c401ff428612dbb49e1b582a0407e5fa24da45` | `backlog.jsonl` | Current filename and title-based creation surface |

For the v1 capture, `plans.jsonl` was renamed to the already-supported
`events.jsonl` immediately after `ergo init`; all records were then appended by
the unmodified v1.0.0 binary. This proves the released writer's behavior on the
old filename rather than merely copying current constructors.

The support policy is release-major based: Ergo reads backlog records written
by every released major version beginning with v1. Compatibility may normalize
legacy state only through an explicit lifecycle command. Opening, listing,
showing, or compacting a supported backlog preserves its current meaning.
