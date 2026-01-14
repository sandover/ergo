# ergo

**A tiny, repo-local task graph for humans + agents.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)

`ergo` is a small CLI that stores tasks and dependencies in an append-only log under `.ergo/`.
It's meant for "agents in a repo" workflows where multiple processes can safely claim READY work, and humans can inspect and steer with plain, pipe-friendly commands.

## ⚡ Quick Start

```bash
# Install
brew install sandover/tap/ergo
# or: go install github.com/sandover/ergo@latest

# Initialize (run once per repo)
ergo init

# Create an epic and tasks (JSON stdin)
echo '{"title":"Improve Linux support"}' | ergo new epic          # prints ID like OFKSTE
echo '{"title":"Add unix lock abstraction","epic":"OFKSTE"}' | ergo new task
echo '{"title":"Add Linux docs","epic":"OFKSTE"}' | ergo new task

# Add a dependency (A depends on B => A waits for B)
ergo dep ABCDEF GHIJKL

# See what's ready, then claim one
ergo list --ready
ergo next
```

## 🛠 Features

- **Repo-local:** state lives in `.ergo/` as append-only JSONL—inspectable, diffable.
- **Dependency-aware:** tasks become READY when blockers are done/canceled.
- **Concurrency-safe:** file lock serializes writes; `next` is race-safe.
- **Human + agent:** use `--worker human` for decision points; agents use `--as agent`.
- **No server:** no accounts, no daemon, no external service.

## 📖 Essential Commands

| Command | Action |
| --- | --- |
| `ergo init` | Create `.ergo/` in the repo. |
| `echo '{"title":"..."}' \| ergo new epic` | Create an epic (prints id). |
| `echo '{"title":"...","epic":"ID"}' \| ergo new task` | Create a task (prints id). |
| `ergo list [--ready\|--blocked]` | List tasks (filter by status). |
| `ergo next [--peek]` | Claim oldest READY task, set to `doing`. |
| `echo '{"state":"done"}' \| ergo set <id>` | Update task fields (state, claim, etc). |
| `ergo dep <A> <B>` | A depends on B. |
| `ergo show <id>` | Show task details. |

**All mutations use JSON stdin.** Run `ergo --help` for syntax or `ergo quickstart` for complete examples.

## 🔗 Workflow

1. Initialize once (`ergo init`) and create tasks as you go.
2. Workers run `ergo next` to claim a READY task (safe under concurrency).
3. Mark it `done` when finished—dependents unblock automatically.

Tasks depend on tasks; epics depend on epics. Use `--as agent` so agents skip `--worker human` tasks.

## 📦 Installation

- **Homebrew:** `brew install sandover/tap/ergo`
- **Go:** `go install github.com/sandover/ergo@latest`
- **Source:** `go build -o ergo .`

## 🗂 Data

- `.ergo/events.jsonl` — append-only event log (source of truth)
- `.ergo/lock` — filesystem lock for writes

Add `/.ergo/` to `.gitignore` for local-only use, or commit it for shared state.

## 🧭 Philosophy

- Boring, inspectable storage on disk
- Plain text output for pipes; `--json` for scripts
- Easy to reason about; easy to delete
- Not a PM tool—no sprints, no calendar, no server

Inspired by [beads](https://github.com/beads-ai/beads-spec).
