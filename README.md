# ergo

**A tiny, repo-local task graph for humans + agents.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/sandover/ergo)](https://github.com/sandover/ergo/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandover/ergo)](https://goreportcard.com/report/github.com/sandover/ergo)

`ergo` is a small CLI that stores tasks and dependencies in an append-only log under `.ergo/`.
It’s meant for “agents in a repo” workflows where multiple processes can safely claim READY work, and humans can inspect and steer with plain, pipe-friendly commands.

## ⚡ Quick Start

```bash
# Install (pick one)
brew install sandover/tap/ergo
# or: go install github.com/sandover/ergo@latest

# Initialize (run once per repo)
ergo init

# (Optional) Tell your agent how to use ergo
# echo "Use 'ergo' for task tracking in this repo." >> AGENTS.md
# echo "Agents: always use --as agent; use --json; never use --as human." >> AGENTS.md

# Create an epic
ergo task new --kind epic "Improve Linux support"
# prints a short ID like "EPIC01"

# Create tasks under that epic
ergo task new --epic EPIC01 "Locking: add unix lock abstraction for Linux"
ergo task new --epic EPIC01 "Docs: add Linux notes"

# Add a dependency (A depends B => A waits for B)
# (replace `<TASK_A>` and `<TASK_B>` with the ids printed by `task new`)
ergo dep <TASK_A> depends <TASK_B>

# See what's READY, then claim one
ergo ready
ergo take
```

Want multi-line bodies? Use `--body-file <path>` (or `--body-file -` for stdin).

### About the ids

Commands like `task new` print an id on success. You only need ids when you want to reference a specific task (e.g. `show`, `state`, `dep …`). If you’re just creating tasks and taking READY work, you can mostly ignore them.

### Human decision points

If a task needs a human decision, create it with `--worker human` (or set it later with `ergo worker <id> human`). Agents should run `ergo ready --as agent` / `ergo take --as agent` so they naturally stop when only human tasks remain.

## 🛠 Features

- **Repo-local + inspectable:** state lives in `.ergo/` as an append-only JSONL log.
- **Dependency-aware:** tasks become READY automatically when blockers are done/canceled.
- **Concurrency-safe:** writes are serialized with a file lock; `take` is safe under races.
- **Plain output:** intentionally simple text for piping and shell scripts.
- **No server:** no accounts, no daemon, no external service.
- **Composable output:** use `--json` for scripts and agents.

## 📖 Essential Commands

| Command | Action |
| --- | --- |
| `ergo init [dir]` | Create `.ergo/` in the repo (or `dir`). |
| `ergo task new --kind epic [title] [--body-file <path|->]` | Create an epic (prints id). |
| `ergo task new --epic <epic_id> [title] [--body-file <path|->]` | Create a task under an epic (prints id). |
| `ergo ready` | List READY work items. |
| `ergo take` | Atomically claim the oldest-created READY item and set it to `doing`. |
| `ergo state <id> <state>` | Set state: `todo | doing | done | blocked | canceled`. |
| `ergo dep <from> depends <to>` | Add a dependency edge (“from waits for to”). |
| `ergo show <id>` | Show details and body. |
| `ergo plan` | Summarize epics/tasks and READY/BLOCKED. |
| `ergo where` | Print the active `.ergo/` directory path. |
| `ergo compact` | Rewrite the log to current state (drops history). |

Run `ergo --help` for command usage.

## 🔗 Workflow (Humans + Agents)

The usual loop:

1. You initialize once (`ergo init`) and write tasks as you think of them.
2. Agents (or other humans) run `ergo ready` to find unblocked tasks.
3. A worker runs `ergo take` to claim a task (safe under concurrency).
4. When done, they mark it `done`, unblocking dependents automatically.

By default, `ready`/`take` return tasks only. Use `--kind any` (or `--kind epic`) to include epics.
Tasks depend on tasks; epics depend on epics. Epics are completed when all their tasks are done|canceled.

If you want ergo to be “invisible infrastructure”, keep `.ergo/` uncommitted and use it locally.
If you want shared memory across collaborators/agents, commit `.ergo/` and treat it like project state.

## 👀 What You’ll See

Example `ergo ready` output:

```text
ABC123  todo   EPIC01  -  Locking: add unix lock abstraction for Linux
```

Example `ergo take` output (prints the task body):

```text
Locking: add unix lock abstraction for Linux
```

## 📦 Installation

- **Homebrew:** `brew install sandover/tap/ergo`
- **Go:** `go install github.com/sandover/ergo@latest`
- **Build from source:** `go build -o ergo .`

**Requirements:** Go 1.21+ (for building). Prebuilt releases are used by Homebrew.

## 🗂 Where Your Data Lives

- `.ergo/events.jsonl` is the source of truth (append-only JSONL).
- `.ergo/lock` is a filesystem lock used to serialize writes.

Recommended `.gitignore` (personal/stealth mode):

```gitignore
/.ergo/
```

## ✍️ Task Bodies

- For `task new`: if you provide a title, it’s used as the body; otherwise use `--body-file` or stdin.
- If stdin is piped, the body is read from stdin.
- Otherwise, `ergo` opens `$EDITOR` (default `nano`).
- Bodies are set at creation time.

## 🧭 Design Principles

- Boring storage you can inspect (`.ergo/` on disk).
- Output meant for terminals and pipes.
- Explicit concurrency model (lock + append-only log).
- Easy to reason about; easy to delete.

## 🚫 Non-goals

- Being a hosted PM tool or a full replacement for Jira/Linear.
- Managing your calendar, sprints, or team process.
- Hiding state in a server or requiring a daemon.

## ✅ Status

- Tested primarily on macOS; Linux should work; Windows is not supported yet.
- Auto-discovers `.ergo/` by walking up directories (like git).
- Default output is intentionally plain text (often tab-separated); use `--json` for scripts/agents.
