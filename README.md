# ergo

**A fast, minimal, dependency-aware backlog for coding agents.**

[![License](https://img.shields.io/github/license/sandover/ergo)](LICENSE)
[![CI](https://github.com/sandover/ergo/actions/workflows/ci.yml/badge.svg)](https://github.com/sandover/ergo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sandover/ergo)](https://goreportcard.com/report/github.com/sandover/ergo)
[![Go Reference](https://pkg.go.dev/badge/github.com/sandover/ergo.svg)](https://pkg.go.dev/github.com/sandover/ergo)

Ergo keeps an implementation backlog in the repository. Agents create tasks,
order them with dependencies, claim ready work, and record outcomes through
direct commands. Humans see the same backlog. A repository lock keeps concurrent
claims and mutations safe.

Ergo is deliberately small: tasks, epics, dependencies, lifecycle state, and
results. Its transaction and snapshot records are plain, git-friendly JSONL.

Inspired by [beads (bd)](https://github.com/steveyegge/beads), with a smaller
command and storage model.

## Install

macOS with Homebrew:

```sh
brew install sandover/tap/ergo
```

Any supported platform with Go:

```sh
go install github.com/sandover/ergo/cmd/ergo@latest
```

Add a short repository instruction for your coding agent:

> Use Ergo to manage the implementation backlog. Run `ergo --help` and
> `ergo quickstart` to learn it.

The repository also ships an
[Ergo feature-planning skill](skills/ergo-feature-planning/SKILL.md) for shaping
and executing larger backlogs.

## Start

```sh
ergo init
ergo new task "Add login"
# => ABCDEF

ergo list --ready
ergo claim ABCDEF --agent model@host
ergo done ABCDEF -m "Implemented and verified"
```

Interactive output is colored automatically; pipes and redirects stay plain.
Force color for a capable viewer or suppress it explicitly:

```sh
ergo --color=always list --ready | less -R
ergo --color=never show ABCDEF
```

Ergo also honors `NO_COLOR` and `TERM=dumb`. `ergo quickstart` describes the
complete presentation policy.

Use a concise title. Pipe longer context into the initial body:

```sh
printf '%s\n' 'Use bcrypt with cost 12.' |
  ergo new task "Add password hashing"
```

## Create an epic

An epic is a root task with children. Create one from a Markdown file:

```sh
cat > tasks.md <<'EOF'
# Password hashing
Use bcrypt with cost 12.
---
# Session tokens
Use 1-hour access and 24-hour refresh tokens.
EOF

ergo new epic "User login" --file tasks.md
```

Each `# Title` chunk becomes a child task. File order does not create
dependencies. Add order explicitly:

```sh
ergo sequence TASK_HASHING TASK_TOKENS
```

Optional piped stdin becomes free-form context on the epic.

You can also build an epic incrementally:

```sh
EPIC_ID=$(ergo new task "User login")
ergo new task "Password hashing" --epic "$EPIC_ID"
```

The first child promotes a clean root todo task to an epic.

## Work with the backlog

```sh
ergo list
ergo list --ready
ergo list --epic ABCDEF
ergo show ABCDEF
```

![Example output of ergo list](docs/img/ergo-list-screenshot.jpg)

Claim a known task or the oldest ready task:

```sh
ergo claim ABCDEF --agent model@host
ergo claim --agent model@host
```

Finish the attempt with the command that states the outcome:

```sh
ergo done ABCDEF -m "Implemented and verified" --result docs/verification.md
ergo block ABCDEF -m "Waiting for the staging credential"
ergo cancel ABCDEF -m "Requirement withdrawn"
ergo release ABCDEF -m "Ready for another agent"
```

Lifecycle messages append. Results refer to existing project-relative files.
Lifecycle commands clear the claim and never replace the task body.

Use focused commands to edit existing work:

```sh
ergo title ABCDEF "Clarify authentication failure"
printf '%s\n' '## Goal' '- Clarify the failure' | ergo body ABCDEF
ergo move ABCDEF GHIJKL
ergo move ABCDEF --root
```

For a lossless body edit, project the stored body to a temporary file before
writing it back:

```sh
tmp=$(mktemp) || exit
trap 'rm -f "$tmp"' 0
ergo show ABCDEF --body >"$tmp" || exit
${EDITOR:-vi} "$tmp" || exit
ergo body ABCDEF <"$tmp"
```

`ergo quickstart` explains the full projection and empty-body semantics.

## Storage

```text
.ergo/
├── backlog.jsonl  # transactions and compacted snapshots
└── lock           # write and coherent-read serialization
```

New repositories use `backlog.jsonl`; an existing `plans.jsonl` or
`events.jsonl` remains in place. Exactly one supported log may exist. Repository
reads load a coherent graph under the lock. Mutations validate their complete
event batch and append it as one transaction record under that same lock.
Ready-task selection and claim are one locked update, so concurrent agents
cannot claim the same task.

Run `ergo --help` for the front door and `ergo quickstart` for the complete
guide. Each command also supports `--help` for syntax and options.
