# ergo

**A tiny, repo-local task graph for humans + agents.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)

`ergo` is a small CLI that stores tasks and dependencies in an append-only log under `.ergo/`.  Claude Code or Codex use it to write out their plans in a form that's persistent and inspectable by humans or other agents. Ready for all your agent swarm needs!

### Problem to solve
Coding agents make plans, but they are either internal to the agent's own "plan" tool (ephemeral and not inspectable), or sprawl into markdown files which have to be managed ad hoc over time, which gets messy.

### Solution
A fast CLI tool for planning inside the repo, with support for dependencies and epics -- like a micro-Jira. Agents write plans into it and claim tasks from it. Plans persists over time so various agents can use it (even in parallel). 

One implementation of this idea is [beads](https://github.com/steveyegge/beads). **ergo** is inspired by **bd**, but seeks to be simpler and faster.

## Features

- **Repo-local:** state lives in `.ergo/` as append-only JSONL -- inspectable and diffable.
- **Simple:** no daemon, no git hooks, few opinions, easy to reason about.
- **Concurrency-safe:** file lock serializes writes; `next` is race-safe.
- **For humans + agents:** mark tasks `worker: human`; filter with `--as agent`.
- **Unix:**: Plain text for pipes, `--json` for scripts.

## Quick Start

```bash
# Install
brew install sandover/tap/ergo   # or: go install github.com/sandover/ergo@latest

# Initialize (run once per repo)
ergo init

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

# List tasks
ergo list              # tree view of all tasks
ergo list --ready      # only unblocked, claimable tasks
ergo list --blocked    # tasks waiting on dependencies

# Claim and work
ergo next              # claim oldest READY task, set to doing, print body
ergo next --peek       # see what's next without claiming
ergo next --as agent   # skip human-only tasks

# Update task state
echo '{"state":"done"}' | ergo set ABCDEF            # mark complete
echo '{"state":"todo"}' | ergo set ABCDEF            # reopen a task
echo '{"claim":"agent-2"}' | ergo set GHIJKL         # reassign to different worker

# Inspect
ergo show ABCDEF         # human-readable details
ergo show ABCDEF --json  # structured output for agents
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
On each command, ergo replays `events.jsonl` from the top to build current state in memory. This is fast (typically <10ms for thousands of events) and guarantees consistency. Run `ergo compact` to collapse history into current state if the log grows large.

**Why not SQLite?**
SQLite is great, but binary files don't diff well in git, and concurrent writers from multiple processes need careful handling. JSONL is trivially inspectable (`cat | jq`), merges via normal git workflows, and append-only writes with `flock` are dead simple. For a task graph of a few thousand items, replay is instant; you don't need a query engine.

Add `/.ergo/` to `.gitignore` for local-only use, or commit it for shared state across machines.
