# Ergo Backlog

Ergo Backlog makes a repository's dependency-aware Ergo backlog readable
inside VS Code. Search tasks and epics, scan the active backlog, and open
details without reading JSONL or leaving the editor.

The extension is a Preview. It is intentionally read-only and delegates
backlog interpretation to the installed Ergo CLI.

## Requirements

- VS Code 1.95 or later
- Ergo 4.2.0 or later in the extension host environment
- A workspace containing an Ergo backlog

Install Ergo on macOS:

```sh
brew install sandover/tap/ergo
```

Windows and Linux archives are available from the
[Ergo 4.2.0 GitHub Release](https://github.com/sandover/ergo/releases/tag/v4.2.0).
Place `ergo` or `ergo.exe` on the extension host's `PATH`.

Remote, WSL, and development-container windows need Ergo installed in that
environment. Set `ergo.executablePath` to an absolute executable path when VS
Code should use a specific installation. The extension reports the attempted
path and corrective installation guidance when Ergo is missing or older than
4.2.0.

## Browse the backlog

Run **Ergo: Backlog** from the Command Palette to open a searchable native
picker. Search by title or six-character ID, select a task or epic, and inspect
the exact readable `ergo show` document in VS Code's Markdown preview.

Opening `.ergo/backlog.jsonl` presents a read-only backlog overview instead of
raw JSONL. Filter the overview, expand an epic, and select any ID to open its
details. Use **Reopen Editor With → Text Editor** when you specifically need
the underlying event log.

## Boundaries and privacy

The extension invokes the configured Ergo executable with argument arrays and
without a shell. It does not parse or write the JSONL event log, mutate tasks,
add telemetry, or send backlog content to an Ergo service. VS Code and
installed extensions remain subject to their own privacy behavior.

The extension does not create, claim, complete, reorder, or otherwise change
backlog work. Use the Ergo CLI for mutations.

## Support

Report defects and feature requests in the
[Ergo issue tracker](https://github.com/sandover/ergo/issues). Include VS Code,
operating-system, extension, and `ergo --version` information. Do not attach a
private backlog.

## Remove the extension

Find **Ergo Backlog** in the Extensions view and choose **Uninstall**. Removing
the extension does not remove Ergo or change repository backlogs.

## Develop and package

Open `editors/vscode` as the VS Code workspace, run `npm ci`, and press `F5` to
open an Extension Development Host.

```sh
npm test
npm run package
```

The package command creates `ergo-backlog-0.1.0.vsix`. The repository CI runs
the same locked build and test workflow on Linux, macOS, and Windows and proves
the package on Linux.

## Maintainer smoke procedure

Create a disposable backlog with distinct work:

```sh
scratch="$(mktemp -d)"
ergo init "$scratch"
epic="$(ergo --dir "$scratch" new task "Create one citation from text spanning pages")"
ergo --dir "$scratch" new task "Define each page highlight" --epic "$epic"
ergo --dir "$scratch" new task "Add support-safe diagnostics"
code "$scratch"
```

In the opened window:

1. Open `.ergo/backlog.jsonl`, filter for `citation`, and open the epic child.
2. Run **Ergo: Backlog**, search for a six-character ID, and open its preview.
3. Compare the source view with
   `ergo --color=never --dir <scratch> show <id>`.
4. Confirm that a folder without `.ergo` reports a missing backlog.
5. Configure a missing executable and an Ergo version older than 4.2.0 and
   confirm that each produces distinct corrective guidance.
