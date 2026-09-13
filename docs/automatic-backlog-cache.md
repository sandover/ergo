# Automatic backlog cache

Ergo automatically maintains `.ergo/cache.json` to avoid replaying the complete backlog history on every command. The cache is a disposable performance aid. `.ergo/backlog.jsonl` remains the only authority for task and dependency state.

The feature has no command, flag, setting, status output, or maintenance workflow. Deleting the cache is safe. A missing, stale, unreadable, or corrupt cache causes one normal full replay and a best-effort rebuild.

## Trust boundary

A cache hit validates:

- the cache format and its whole-file integrity checksum;
- the selected backlog basename;
- the identity of the opened backlog file;
- the saved byte offset, source size, and record boundary; and
- the tail records appended after that offset.

Apart from a one-byte record-boundary check, a hit does not read or hash backlog bytes before the saved offset. This is what removes historical I/O as well as historical JSON decoding and replay.

Equivalence to full replay assumes the represented prefix has not changed under the same file identity. An external edit preserving size and modification time can escape detection. A rewrite followed by append, or truncation followed by regrowth beyond the saved offset, can also escape detection without preserving either size or modification time. Native file identity reuse is another limitation where generation identity is unavailable. These cases can affect reads and subsequent mutations until the cache is invalidated or deleted. Normal appends work; replacement with a different identity, a file shorter than the saved offset, compaction, invalid boundaries, corrupt cache data, and malformed tails cause a miss or authoritative full-replay error.

Ergo's repository lock coordinates Ergo processes. It does not make an editor or Git operation that ignores the lock safe during a command.

## Stored state

`cache.json` contains one versioned checkpoint inside a small checksum envelope. The checkpoint directly represents the raw reducer state and source position. Its SHA-256 checksum detects damage even when the file remains valid JSON.

The checkpoint records:

- selected source basename;
- platform file identity from the opened source descriptor;
- captured EOF byte offset;
- physical line and nonblank record counts;
- captured modification time; and
- the state needed to continue replay.

The stored graph preserves:

- every task scalar, including claims and timestamps;
- backlog-origin results and lifecycle messages in their stored order;
- live dependency edges;
- tombstones; and
- the complete legacy explicit-epic set.

The checkpoint is captured before legacy title migration and before journal hydration. This matters for a legacy blank-title task whose body changes in the later tail: full replay derives the title from the final body, so checkpoint-plus-tail replay must do the same. Derived indexes, readiness, rendered output, absolute paths, and journal-hydrated evidence are not stored.

The cache has one syntax-and-semantics version. A future incompatible change increments it. Cache documents are never migrated because rebuilding from the backlog is simpler and safer.

## Loading

Repository loading keeps its existing shared or exclusive lock and selects the authoritative backlog path before consulting the cache.

On a possible hit, Ergo:

1. Opens and fully validates the cache.
2. Opens the selected backlog and compares its descriptor identity with the checkpoint.
3. Confirms that the file still reaches the saved offset and that the preceding byte ends a physical record.
4. Seeks directly to the saved offset.
5. Uses the decoded checkpoint as its owned raw graph and applies the appended tail, if any.
6. Retains a separate raw checkpoint only if publication is due, then finalizes the command graph with legacy migration and index rebuilding once.

Full reads and tails use one JSONL scanner, with explicit starting coordinates and permission to read an initial snapshot. Tail line numbers, record counts, valid-byte offsets, and interrupted-write repair coordinates remain whole-file coordinates. Parser output carries source/repair metadata; the repository load result separately carries an optional publication checkpoint.

Any accelerated-path error discards the candidate and invokes the unchanged full loader. The full loader decides authoritative errors whenever the cache path fails. Cheap checks cannot detect corruption hidden by an undetected prefix rewrite; this is part of the trust boundary above.

## Creation and refresh

Successful reads and writes maintain the cache under the lock they already hold. There is no lock upgrade, second lock, daemon, election, or background worker.

Ergo creates a cache after a complete, newline-terminated backlog reaches 4 MiB. It refreshes a valid cache when the uncached tail reaches either 1 MiB or 256 physical records. These fixed internal thresholds keep small repositories free of cache work and bound ordinary tail replay without creating a user setting.

A writer publishes the successfully loaded input checkpoint after its authoritative work succeeds. Its newly appended transaction may remain in the tail until a later refresh. Cache publication is skipped after an authoritative write or journal failure.

Publication writes a complete cache to a uniquely created `cache.tmp-*` file with mode `0600`, closes it, and atomically replaces `cache.json`. Disposable cache files and their directory are not synced. The backlog and journal keep their existing durability rules. Concurrent readers may encode equivalent candidates; either complete atomic replacement is valid.

Every cache operation is best-effort. Permission, encoding, write, close, or replacement failure does not change command output or exit status. `ergo compact` keeps its explicit behavior and removes the old cache after successfully replacing the backlog.

## Git and compatibility

Before automatic cache publication, Ergo adds these rules to `.ergo/.gitignore`:

```gitignore
# Ergo performance cache
/cache.json
/cache.tmp-*
```

Existing unrelated rules and bytes are preserved. Ergo does not invoke Git, alter the index, or untrack a file that was already added explicitly. Ignore maintenance is best-effort and cannot block backlog work.

Older Ergo binaries ignore the cache and continue appending to the same authoritative backlog. A newer binary accepts those supported tail records. Alternating cache versions may rebuild the disposable file but cannot change task state.

The backlog, journal, transaction, snapshot, and command-output formats do not change. The cache is never considered by backlog-file selection.

## Verified performance

On the Apple M3 reference fixture with 1,500 tasks and 30,380 backlog transactions:

- the first automatic cache creation takes about 946 ms, similar to the former full replay;
- warm internal graph loads take about 40 ms;
- warm end-to-end commands take about 49-71 ms instead of roughly one second;
- a 64-record tail remains about 42 ms; and
- a 256-record refresh takes about 65 ms.

With one current task and 30,000, 100,000, or 300,000 historical transactions, warm loads remain at or below 0.127 ms. Warm work now scales with current graph size and the uncached tail, not represented history. See [large-backlog-performance.md](large-backlog-performance.md) for commands, raw measurements, and limits.

Focused tests cover cache integrity, raw-state fidelity, released backlog compatibility, normal append, replacement, truncation, invalid tails, interrupted-write repair, concurrent publication, compaction, ignore preservation, and unchanged CLI output. Windows builds are compile-checked. Windows runtime performance and cold-disk behavior remain unmeasured.
