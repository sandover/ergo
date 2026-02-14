# ergo

**A fast, minimal planning CLI tool for Claude Code and Codex.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandover/ergo)](https://goreportcard.com/report/github.com/sandover/ergo)
[![Go Reference](https://pkg.go.dev/badge/github.com/sandover/ergo.svg)](https://pkg.go.dev/github.com/sandover/ergo)

`ergo` gives your AI agents a better way to plan -- by storing epics & tasks in your repo, in a compact, git-friendly format.

Plans are persistent across agent sessions, they are easy for humans to read and reason about, and they can be shared by different agents (from different model providers).

## Why?
In software projects, even with AI, it really helps to clearly separate WHAT you want to build from HOW the implementation happens. Traditionally, this was the difference between a spec document and a work backlog.

Coding agents' own planning modes tend to result in a mess of markdown files that blur the distinction between the spec and the backlog and become hard to manage.

`ergo` is a tool your AI agent uses to write out a backlog of tasks. Then during implementation, your agent claims tasks, works on them, and marks them done. Tasks can be sequenced (A must happen before B), and they can be grouped into epics. 

The task collection is stored in the repo as JSONL file, not inside the agent or its harness. You can keep the plan in git. The plan is agnostic about which agent you use to work on it. You could even use more than one agent at the same time -- ergo is built for concurrency.

Inspired by [beads (bd)](https://github.com/steveyegge/beads), but simpler, sounder, and faster.

## Features

- **Simple:** no daemons, no git hooks, few opinions, easy to reason about.
- **Fast:** 5-15x faster than beads, especially for large projects.
- **Tasks live in the repo:** state lives in `.ergo/` as append-only JSONL.
- **Safe for multiple agents:** a plain old file lock serializes writes
- **Unixy:** text or JSON on stdin and stdout.

---

## Quick Start

#### Step 1 (in the terminal)
```bash
# Install on Mac -- get homebrew first (https://brew.sh/)
brew install sandover/tap/ergo 
```
Or, on Linux: `go install github.com/sandover/ergo@latest`

#### Step 2
Add this instruction into your `AGENTS.md` or `CLAUDE.md` file
> Use 'ergo' for all feature planning, run "ergo --help to learn it

#### Step 3
Once you have a description of what you want to build (your spec), use a prompt like this:

> Use ergo to plan the implementation of this spec. Each task should have a goal, description, definition of done, and automated validation.

_Pro tip_: after that, tell the agent to *review and improve upon its own plan*. This leads to improvements every time. Measure twice, cut once!

#### Step 4
Tell your agent to implement the plan.

---

## How humans use ergo

These three commands are typically the only ones used by humans.

### `ergo list` -- to see the plan your agent wrote

![Example output of ergo list](docs/img/ergo-list-screenshot.jpg)

Legend:
- `○` ready (todo + all deps satisfied)
- `◐` in progress (doing)
- `·` blocked (blocked or todo with unmet deps)
- `✓` done
- `✗` canceled
- `⚠` error
- `@agent-id` claimed by
- `⧗ …` blocked by (dependency summary)

### `ergo show` -- to see details of a task

```bash
ergo show GQUJPG
```

![Example output of ergo show](docs/img/ergo-show-screenshot.jpg)

### `ergo prune` -- to remove completed tasks

```bash
ergo prune
```

Removes completed (done or canceled) tasks from the plan.

---

## How coding agents use ergo

Run `ergo --help` for syntax and `ergo quickstart` for the complete reference.

### Planning

```bash
# Create an epic
ergo new epic --title "User login" --body "Let users sign in with email+pw."
# => ABCDEF

# Add tasks to it
ergo new task --title "Password hashing" --body "Use bcrypt with cost=12" --epic ABCDEF
# => GHIJKL

ergo new task --title "Session tokens" --body "1h access, 24h refresh" --epic ABCDEF
# => MNOPQR

# Enforce order
ergo sequence GHIJKL MNOPQR
```

### Execution

```bash
# Find actionable work
ergo --json list --ready

# Claim a task (--agent identifies the caller)
ergo claim GHIJKL --agent sonnet@hostname

# Mark it done
ergo set GHIJKL --state done
```

> **Tip:** For multi-line task bodies or automation, pipe JSON to stdin. See `ergo quickstart` for patterns.

---

## Data Representation

All state lives in `.ergo/` at your repo root:

```
.ergo/
├── plans.jsonl    # append-only event log (source of truth)
└── lock           # flock(2) lock file for write serialization
```

(For backwards compatibility, `events.jsonl` is also supported if it already exists.)

**Why append-only JSONL?**
- **Auditable:** Full history of every state change, who made it, when.
- **Inspectable:** `cat .ergo/plans.jsonl | jq` — no special tools needed.
- **Recoverable:** Corrupt state? Replay events. Want to undo? Filter events.
- **Diffable:** `git diff` shows exactly what changed.

**Concurrency safety:**
- All writes acquire an exclusive `flock(2)` on `.ergo/lock` before appending.
- `ergo claim` is atomic: read → find oldest READY → claim → write, all under lock.
- Multiple agents can safely race to claim work; exactly one wins, others fail fast and should retry.

**State reconstruction:**
On each command, ergo replays `plans.jsonl` to build current state in memory quickly (100 tasks: ~3ms, 1000 tasks: ~15ms) and guarantees consistency. Run `ergo compact` to collapse history if the log grows large. To verify: `go test -bench=. -benchmem`

**Why not SQLite?**
SQLite is great, but binary files don't diff well in git. JSONL is trivially inspectable (`cat | jq`), merges via git, and append-only writes with `flock` are simple. For a few thousand tasks, replay is instant.

## Is it any good?

Yes.
