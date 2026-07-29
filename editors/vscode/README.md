# Ergo Backlog

**See the work behind your code.**

Ergo Backlog turns a repository's dependency-aware [Ergo](https://github.com/sandover/ergo) backlog into a clear, navigable view inside VS Code. Find the next ready task, understand how work fits into an epic, and open readable details without inspecting JSONL or leaving the editor.

## What you can do

- **Scan the active backlog.** See tasks grouped under their epics, with compact status and dependency cues.
- **Focus on ready work.** Filter out blocked and in-progress tasks while keeping their epic context visible.
- **Find anything quickly.** Search tasks and epics by title or six-character Ergo ID.
- **Read task and epic details.** Open the selected item as a formatted Markdown preview in its own editor tab.
- **Open the backlog naturally.** Select `.ergo/backlog.jsonl` in the Explorer to see the backlog view instead of the raw event log.

Ergo Backlog is read-only. Planning and lifecycle changes remain explicit CLI actions, while VS Code provides a comfortable place to browse and understand the current plan.

## Get started

1. Install [Ergo 4.2.0 or later](https://github.com/sandover/ergo).
2. Open a folder that contains an Ergo backlog.
3. Open the Command Palette and run **Ergo: Backlog**.

Search for a task or epic and select it to open its details. You can also open `.ergo/backlog.jsonl` from the Explorer for a browsable overview of the whole backlog.

### Install Ergo

On macOS:

```sh
brew install sandover/tap/ergo
```

On Windows and Linux, download the appropriate archive from the [latest Ergo release](https://github.com/sandover/ergo/releases/latest) and place `ergo.exe` or `ergo` on `PATH`.

Ergo Backlog uses the Ergo CLI from the VS Code extension host environment. Remote, WSL, SSH, and development-container windows therefore need Ergo installed in that environment.

## How the backlog view works

The overview preserves the structure that helps you choose and understand work:

- Epics remain visible as stable landmarks.
- Tasks appear beneath their epic or at the top level.
- Status glyphs distinguish ready, blocked, active, and completed work.
- The **Ready tasks only** filter narrows the task list without hiding its epic context.
- Clicking an Ergo ID opens that task or epic in a readable detail tab.

Use **Reopen Editor With → Text Editor** when you specifically want to inspect the underlying JSONL event log.

## Configure the Ergo executable

The extension normally resolves `ergo` from the extension host's `PATH`. To use a specific installation, set **Ergo: Executable Path** in VS Code Settings to an absolute path such as:

```text
/opt/homebrew/bin/ergo
```

If Ergo is missing or older than 4.2.0, the extension reports the path it tried and explains how to install or select a compatible executable.

## Read-only by design

Ergo Backlog asks the installed Ergo CLI to interpret the repository's event log. It does not parse or write `.ergo/backlog.jsonl`, mutate tasks, invoke commands through a shell, add telemetry, or send backlog content to an Ergo service.

Use the Ergo CLI to create, claim, complete, block, reorder, or otherwise change work.

## Preview

Ergo Backlog is an early Preview focused on fast, dependable backlog reading. The current release provides the backlog overview, ready-work filter, task and epic search, and formatted detail previews.

Found a problem or have an idea? Open an issue in the [Ergo issue tracker](https://github.com/sandover/ergo/issues). Include your operating system, VS Code version, extension version, and `ergo --version`. Do not attach a private backlog.

## About Ergo

[Ergo](https://github.com/sandover/ergo) is a fast, repository-local, dependency-aware backlog for coding agents and humans. It keeps tasks, epics, dependencies, and lifecycle history close to the code so the plan can travel with the repository.
