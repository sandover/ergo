# Large-backlog performance investigation

Baseline: 2026-09-08. Cache verification: 2026-09-09. Loader refactor verification: 2026-09-13.

Scope: investigate GitHub [issue #13](https://github.com/sandover/ergo/issues/13) on Ergo 6.x, then verify the resulting automatic disposable cache against the original full-replay baseline.

## Ergo model

Ergo is a repository-local, file-backed backlog in the Unix tradition. A command discovers `.ergo`, takes the repository lock, reads JSONL, rebuilds the current graph, performs one operation, renders the result, and exits. `backlog.jsonl` is the event history for task and dependency state. `journal.jsonl` holds work narrative and results. `compact` is the existing snapshot mechanism; it is not a database or a resident service.

The important consequence is that a full-history read pays for the history even when the requested answer is one task. `show`, `claim`, and writes use the same replay path as `list`.

## Fixture

The reusable generator is [generate-large-backlog-fixture.go](/Users/brandonharvey/src/ergo/scripts/generate-large-backlog-fixture.go), backed by [large_backlog_fixture.go](/Users/brandonharvey/src/ergo/internal/ergo/large_backlog_fixture.go). It writes only the requested project directory. The default is deterministic and uses temporary storage; `--tasks` scales it while `--backlog` and `--journal` select `full` or `compacted`.

```sh
fixture_dir=$(mktemp -d)
go run ./scripts/generate-large-backlog-fixture.go \
  --dir "$fixture_dir" --backlog full --journal full
```

Default logical shape:

| Item | Count |
| --- | ---: |
| Tasks | 1,500 |
| Epics | 24 |
| Leaf tasks | 1,476 |
| Live dependencies | 1,500 |
| Backlog transactions in full history | 30,380 |
| Full journal entries | 5,984 |
| Body-change events | 14,000 |
| Title-change events | 6,574 |

The full journal kind mix is `created=1500`, `open=1516`, `claim=1380`, `done=1157`, `cancel=227`, `result=154`, `fail=36`, and `block=14`. Final leaf states are `done=900`, `canceled=180`, `failed=36`, `blocked=14`, `doing=43`, and `todo=303`. Dependency churn includes link/unlink/relink history and leaves 1,500 acyclic live edges.

Physical sizes from the generator on this host:

| Representation | Backlog | Journal | Backlog records | Journal records |
| --- | ---: | ---: | ---: | ---: |
| full backlog + full journal | 26,875,669 B | 590,484 B | 30,380 | 5,984 |
| compacted backlog + full journal | 2,604,086 B | 590,484 B | 3,001 | 5,984 |
| compacted backlog + compacted journal | 2,604,086 B | 191,005 B | 3,001 | 1,654 |

The source journal count is intentionally close to the issue report while using 1,500 created tasks rather than its reported 1,441 `created` entries.

## Reproduction

The focused fixture gate is:

```sh
GOCACHE=/Users/brandonharvey/src/ergo/.gocache/go-build \
GOMODCACHE=/Users/brandonharvey/src/ergo/.gocache/go-mod \
go test ./internal/ergo -run 'TestLargeBacklogFixture' -count=1
```

Internal allocation benchmarks:

```sh
GOCACHE=/Users/brandonharvey/src/ergo/.gocache/go-build \
GOMODCACHE=/Users/brandonharvey/src/ergo/.gocache/go-mod \
go test ./internal/ergo -run '^$' -bench 'BenchmarkLargeBacklog' \
  -benchmem -benchtime=3x -count=1
```

End-to-end CLI benchmarks:

```sh
GOCACHE=/Users/brandonharvey/src/ergo/.gocache/go-build \
GOMODCACHE=/Users/brandonharvey/src/ergo/.gocache/go-mod \
go test ./cmd/ergo -run '^$' \
  -bench 'BenchmarkLargeBacklogCLI|BenchmarkProcessStartup' \
  -benchmem -benchtime=3x -count=1
```

Cache transition and history-scaling benchmarks:

```sh
GOCACHE=/Users/brandonharvey/src/ergo/.gocache/go-build \
GOMODCACHE=/Users/brandonharvey/src/ergo/.gocache/go-mod \
go test ./internal/ergo -run '^$' \
  -bench 'BenchmarkLargeBacklogCacheWarm|BenchmarkFixedStateHistoryCacheWarm|BenchmarkLargeBacklogCacheTransitions' \
  -benchmem -benchtime=3x -count=1
```

The tests use a temporary fixture for each benchmark. `-benchmem` on the CLI benchmark reports allocations in the benchmark parent process; the child process has its own address space. The in-process benchmarks provide the operation allocation figures.

## Measurements

Host: Apple M3, macOS arm64. Values are one run with `-benchtime=3x`; they are stable enough to identify order-of-magnitude costs, not a cross-machine performance promise.

### Internal operations

| Operation | Full | Compacted |
| --- | ---: | ---: |
| Inspect event log | 633.6 ms, 101.76 MB, 911,429 allocs | 156.2 ms, 66.86 MB, 82,398 allocs |
| Replay events | 329.2 ms, 138.24 MB, 630,708 allocs | 1.49 ms, 1.81 MB, 4,327 allocs |
| Read journal | 9.59 ms, 5.37 MB, 53,303 allocs | 2.96 ms, 1.49 MB, 15,805 allocs |
| View with journal | 1,000.1 ms, 247.33 MB, 1,595,617 allocs | 167.3 ms full journal; 159.7 ms compacted journal |
| Graph derivation only | 658 µs, 234,456 B, 73 allocs | 605 µs, 234,456 B, 73 allocs |
| List rendering only | 11.93 ms, 532,768 B, 21,166 allocs | 12.07 ms, 533,394 B, 21,168 allocs |
| One `show` render only | 50.7 µs, 3,648 B, 62 allocs | 40.1 µs, 4,152 B, 64 allocs |

### CLI commands

Raw benchmark lines from the end-to-end run:

```text
BenchmarkLargeBacklogCLI/full-full/list-8                         3 1022389153 ns/op 21114 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/full-full/list-ready-8                   3  986205583 ns/op 21314 B/op 46 allocs/op
BenchmarkLargeBacklogCLI/full-full/list-json-8                    3  971930458 ns/op 21314 B/op 46 allocs/op
BenchmarkLargeBacklogCLI/full-full/list-ready-json-8              3  983066653 ns/op 21448 B/op 46 allocs/op
BenchmarkLargeBacklogCLI/full-full/show-8                         3  975311945 ns/op 21154 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/full-full/claim-8                        3  990899111 ns/op 21266 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/full-full/lifecycle-write-8               3  997325680 ns/op 21640 B/op 48 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/list-8                    3  189799972 ns/op 21109 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/list-ready-8              3  228544764 ns/op 21165 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/list-json-8               3  186446958 ns/op 21165 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/list-ready-json-8         3  200679625 ns/op 21261 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/show-8                    3  203754083 ns/op 21149 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/claim-8                   3  180441722 ns/op 21421 B/op 46 allocs/op
BenchmarkLargeBacklogCLI/compacted-full/lifecycle-write-8          3  245076792 ns/op 21677 B/op 49 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/list-8               3  182408083 ns/op 21125 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/list-ready-8         3  171829931 ns/op 21197 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/list-json-8           3  166107375 ns/op 21197 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/list-ready-json-8     3  166352250 ns/op 21261 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/show-8                3  164505305 ns/op 21170 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/claim-8               3  172050764 ns/op 21277 B/op 45 allocs/op
BenchmarkLargeBacklogCLI/compacted-compacted/lifecycle-write-8      3  274786167 ns/op 21677 B/op 49 allocs/op
BenchmarkProcessStartup-8                                           3    7847639 ns/op 15701 B/op 38 allocs/op
```

The issue’s old `list --ready` outlier does not appear here. In the original baseline, full-history `list`, `list --ready`, JSON forms, `show`, `claim`, and a lifecycle write all remained near 1 second because they shared the full backlog rebuild. Snapshot reads were about 0.16–0.27 seconds. Journal loading was a small secondary cost and is now omitted entirely from list commands.

### Automatic cache results

The cache keeps the full backlog as authority, stores one disposable raw-state checkpoint, and seeks directly to its saved byte offset. These results use the same Apple M3 host and warm operating-system file cache. They are not cold-disk measurements.

Reference 1,500-task / 30,380-transaction in-process transitions:

| State | Time | Allocated | Allocations |
| --- | ---: | ---: | ---: |
| First automatic cache creation | 945.6 ms | 197.09 MB | 1,522,192 |
| Warm cache hit | 39.6 ms | 30.97 MB | 20,917 |
| Warm hit with 64-record tail | 41.9 ms | 32.63 MB | 26,148 |
| Refresh at 256-record tail | 65.4 ms | 47.71 MB | 40,202 |
| Invalid-cache full fallback and rebuild | 948.6 ms | 198.03 MB | 1,522,200 |

The first command costs about the same as the old full replay because it must reconstruct the graph once. Routine warm reads are about 24x faster than the former one-second view, well below the 250 ms investigation target. Refresh adds about 24 ms over the 64-record-tail case at the 256-record threshold. A corrupt or stale cache costs one full replay and then repairs itself for later commands.

Before the loader refactors, the same five-sample warm-cache benchmark took 65.5 ms and made 117,450 allocations. Shared replay machinery first reduced those figures to 56.2 ms and 94,963 allocations. The single-document codec then reached 39.6 ms and 20,917 allocations. Its checksum envelope increases cumulative allocated bytes from 23.13 MB to 30.97 MB while cutting elapsed time by another 30% and allocation count by 78%.

Warm end-to-end CLI measurements on the reference fixture:

| Command | Before | With automatic cache |
| --- | ---: | ---: |
| `list` | 1,022 ms | 60.3 ms |
| `list --ready` | 986 ms | 51.9 ms |
| `list --json` | 972 ms | 49.4 ms |
| `list --ready --json` | 983 ms | 53.9 ms |
| `show` | 975 ms | 64.3 ms |
| `claim` | 991 ms | 70.6 ms |
| lifecycle write | 997 ms | 68.3 ms |

Task-scaled warm cache loads were about 62 ms at 1,500 tasks, 277 ms at 5,000 tasks, and 569 ms at 15,000 tasks. The remaining work scales with current graph size, as expected.

A separate fixed-state fixture kept one current task while growing history. Warm loads were 0.127 ms or less at 30,000, 100,000, and 300,000 transactions. This establishes that warm work no longer grows with represented history. A focused test changes cached-prefix bytes in place while preserving the cheap identity signals and confirms that the loader does not read that prefix; this is the deliberately accepted trust boundary, not a corruption guarantee.

Focused integration tests also establish identical stdout, stderr, and exit status for list, ready list, both JSON projections, show, and literal body output before and after cache creation. Released v1-v4 backlog fixtures round-trip through the cache without semantic change. Cache replacement and Windows support were compile-checked; Windows runtime timing and cold-disk behavior were not measured.

The measured 4 MiB creation and 1 MiB-or-256-record refresh defaults behave well at the reference scale. Keep them fixed and internal. The first-use cost avoids creating cache files for small repositories, while the refresh bounds keep ordinary tails cheap without introducing settings or adaptive machinery.

## Profiles

Profiles were collected with:

```sh
go test ./internal/ergo -run '^$' \
  -bench 'BenchmarkLargeBacklogViewWithJournal/full' \
  -benchmem -benchtime=15s \
  -cpuprofile=/private/tmp/ergo-large-view-long.cpu \
  -memprofile=/private/tmp/ergo-large-view-long.mem -count=1
```

The longer CPU profile showed these cumulative shares:

```text
inspectEventLog                         38.49%
encoding/json.Unmarshal                 26.96%
replayEventsOnto                        20.17%
isReachable                              7.74% flat/cumulative in replay
```

The allocation profile was dominated by:

```text
isReachable                              1,148.83 MB flat, 23.08%
inspectEventLog                           535.87 MB flat, 35.94% cumulative
encoding/json.Unmarshal                   347.55 MB flat, 40.59% cumulative
replayEventsOnto                            34.34 MB flat, 39.49% cumulative
readJournal                                 53.04 MB flat,  1.78% cumulative
```

The profile includes one-time benchmark fixture setup, but the 15-second run makes that a small part of the result. It confirms that JSONL scan/decode and event replay dominate. Dependency reachability checks are a meaningful replay allocation source, especially during the historical link churn, but they are not on the steady-state path after a snapshot is available.

## Conclusion and recommendation

The automatic disposable cache addresses the measured bottleneck without changing Ergo's user model. `backlog.jsonl` remains append-only and authoritative. Commands automatically create and refresh `.ergo/cache.json`, load its current raw graph state, and replay only the bounded tail. Cache absence or failure returns to the old full replay path.

The reference CLI improves from roughly one second to 49-71 ms after first use. Fixed-state history tests show constant warm-load time through 300,000 transactions. Current graph growth still costs time and memory because Ergo must reconstruct and query that graph; this is the correct remaining scaling boundary.

No manual compaction schedule, daemon, database, cache command, setting, status field, or new output is needed. `ergo compact` retains its explicit history-reduction behavior. The cache is an ignored local optimization and can be deleted safely.
