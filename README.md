# ergo

**A task graph for coding agents. Human-readable.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandover/ergo)](https://goreportcard.com/report/github.com/sandover/ergo)
[![Go Reference](https://pkg.go.dev/badge/github.com/sandover/ergo.svg)](https://pkg.go.dev/github.com/sandover/ergo)

`ergo` gives your AI agents a better place to plan. Tasks and dependencies persist across sessions, stay visible to humans, and are safe for concurrent agents. Data lives in the repo as plain text.

### Why?
Coding agents' plans are ephemeral and not inspectable, or they are sprawling markdown files. `ergo` gives agents a structured place to plan -- with a dependency graph and epics -- while keeping everything visible to humans.

Inspired by [beads (bd)](https://github.com/steveyegge/beads), but simpler and faster.

## Features

- **Repo-local:** state lives in `.ergo/` as append-only JSONL -- inspectable and diffable.
- **Simple:** no daemons, no git hooks, few opinions, easy to reason about.
- **Concurrency-safe:** file lock serializes writes; `next` is race-safe.
- **Unix:** Plain text for pipes, `--json` for scripts.
- **Fast:** 5-15x faster than beads, especially for large projects.

## Quick Start

```bash
# Install
brew install sandover/tap/ergo   # or: go install github.com/sandover/ergo@latest

# Initialize (run once per repo)
ergo init

# Tell your agent
echo "Use 'ergo' for task tracking" >> AGENTS.md
```

## Usage
```bash
# Create an epic (JSON stdin, title + body required)
echo '{"title":"User login","body":"Let users sign in with email and password"}' | ergo new epic
# => created OFKSTE

# Add a task to the epic
echo '{"title":"Password hashing","body":"Use bcrypt with cost=12","epic":"OFKSTE"}' | ergo new task

# Multi-line JSON with heredoc
ergo new task <<'EOF'
{
  "title": "Login endpoint",
  "body": "POST /login:\n1. Validate email\n2. Check password\n3. Return JWT",
  "epic": "OFKSTE"
}
EOF

# Human-only task (agents skip these)
echo '{"title":"Choose session duration","body":"Decide 1h vs 24h tokens","epic":"OFKSTE","worker":"human"}' | ergo new task

# Add dependencies (A depends on B)
ergo dep ABCDEF GHIJKL

# List & inspect tasks
ergo list              # tree view; filter with --ready or --blocked
ergo show ABCDEF       # task details for humans, --json for agents

# Claim and work
ergo next              # claim oldest READY task, set to doing, print body
ergo next --as agent   # skip human-only tasks

# Update task state
echo '{"state":"done"}' | ergo set ABCDEF            # mark complete
```
All mutations use JSON stdin. Run `ergo --help` for syntax or `ergo quickstart` for the complete reference.

## Data Representation

All state lives in `.ergo/` at your repo root:

```
.ergo/
├── events.jsonl   # append-only event log (source of truth)
└── lock           # flock(2) lock file for write serialization
```

**Why append-only JSONL?**
- **Auditable:** Full history of every state change, who made it, when.
- **Inspectable:** `cat .ergo/events.jsonl | jq` — no special tools needed.
- **Recoverable:** Corrupt state? Replay events. Want to undo? Filter events.
- **Diffable:** `git diff` shows exactly what changed.

**Concurrency safety:**
- All writes acquire an exclusive `flock(2)` on `.ergo/lock` before appending.
- `ergo next` is atomic: read → find oldest READY → claim → write, all under lock.
- Multiple agents can safely race to claim work; exactly one wins, others retry.
- Lock timeout is configurable (`--lock-timeout 5s`); default 30s, `0` = fail fast.

**State reconstruction:**
On each command, ergo replays `events.jsonl` to build current state in memory quickly (100 tasks: ~3ms, 1000 tasks: ~15ms) and guarantees consistency. Run `ergo compact` to collapse history if the log grows large. Verify: `go test -bench=. -benchmem`

**Why not SQLite?**
SQLite is great, but binary files don't diff well in git, and concurrent writers from multiple processes need careful handling. JSONL is trivially inspectable (`cat | jq`), merges via normal git workflows, and append-only writes with `flock` are dead simple. For a task graph of a few thousand items, replay is instant; you don't need a query engine.

Add `/.ergo/` to `.gitignore` for local-only use, or commit it for shared state across machines.
