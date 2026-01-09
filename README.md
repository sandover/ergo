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

# Tell your agent (optional but recommended)
echo "Use 'ergo' for task tracking in this repo." >> AGENTS.md

# Create an epic
epic=$(ergo epic new <<'EOF'
Improve Linux support
EOF
)

# Create two tasks and a dependency (A depends B => A waits for B)
t1=$(cat <<'EOF' | ergo task new --epic "$epic"
Locking: add unix lock abstraction for Linux
EOF
)
t2=$(cat <<'EOF' | ergo task new --epic "$epic"
Docs: add Linux notes
EOF
)
ergo link "$t2" depends "$t1"

# See what's READY, then claim one
ergo ls --ready
ergo take
```

## 🛠 Features

- **Repo-local + inspectable:** state lives in `.ergo/` as an append-only JSONL log.
- **Dependency-aware:** tasks become READY automatically when blockers are done/canceled.
- **Concurrency-safe:** writes are serialized with a file lock; `take` is safe under races.
- **Plain output:** intentionally simple text/TSV for piping and shell scripts.
- **No server:** no accounts, no daemon, no external service.

## 📖 Essential Commands

| Command | Action |
| --- | --- |
| `ergo init [dir]` | Create `.ergo/` in the repo (or `dir`). |
| `ergo epic new` | Create an epic (prints id). |
| `ergo task new --epic <epic_id>` | Create a task under an epic (prints id). |
| `ergo ls --ready` | List READY tasks. |
| `ergo take` | Atomically claim oldest READY task and set it to `doing`. |
| `ergo state <id> <state>` | Set state: `todo | doing | done | blocked | canceled`. |
| `ergo link <from> depends <to>` | Add a dependency edge (“from waits for to”). |
| `ergo show <id>` | Show details and body. |
| `ergo plan` | Summarize epics/tasks and READY/BLOCKED. |
| `ergo compact` | Rewrite the log to current state (drops history). |

Run `ergo --help` for the full compact manual.

## 🔗 Workflow (Humans + Agents)

The usual loop:

1. You initialize once (`ergo init`) and write tasks as you think of them.
2. Agents (or other humans) run `ergo ls --ready` to find unblocked work.
3. A worker runs `ergo take` to claim a task (safe under concurrency).
4. When done, they mark it `done`, unblocking dependents automatically.

If you want ergo to be “invisible infrastructure”, keep `.ergo/` uncommitted and use it locally.
If you want shared memory across collaborators/agents, commit `.ergo/` and treat it like project state.

## 👀 What You’ll See

Example `ergo ls --ready` output (TSV):

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

## ✍️ Editing Task Bodies

- If stdin is piped, the body is read from stdin.
- Otherwise, `ergo` opens `$EDITOR` (default `nano`).
- For `edit`, piping empty stdin keeps the existing body.

## 🧭 Design Principles

- Boring storage you can inspect (`.ergo/` on disk).
- Output meant for terminals and pipes.
- Explicit concurrency model (lock + append-only log).
- Easy to reason about; easy to delete.

## 🚫 Non-goals

- Being a hosted PM tool or a full replacement for Jira/Linear.
- Managing your calendar, sprints, or team process.
- Hiding state in a server or requiring a daemon.

## 📝 Documentation

- [Installing](docs/INSTALLING.md) | [Agent Workflow](docs/AGENT_WORKFLOW.md) | [Troubleshooting](docs/TROUBLESHOOTING.md) | [FAQ](docs/FAQ.md)

## ✅ Status

- Tested primarily on macOS; Linux should work; Windows is not supported yet.
- Root discovery is minimal: run commands from the directory containing `.ergo/`.
- Output is intentionally plain text / TSV; there is no JSON output mode yet.
