# Ergo for VS Code

This experimental extension provides two read-only ways to browse an Ergo
backlog:

- Click `.ergo/backlog.jsonl` to open a searchable backlog overview. Expand or
  collapse epics, then click any epic or task to open its details.
- Run **Ergo: Backlog** from the Command Palette to select an epic or task
  from a compact native picker.

Both entry points open the existing `ergo show` document in VS Code's native
Markdown preview. The extension invokes the installed Ergo CLI to interpret the
backlog. It does not parse or write JSONL, mutate tasks, or maintain another
task-detail renderer. Use **Reopen Editor With → Text Editor** when you
specifically need to inspect the underlying JSONL.

## Requirements

- VS Code 1.95 or later.
- `ergo` on the extension host's `PATH`.
- An Ergo build whose `ergo list --help` includes `--json`.

Restart VS Code after installing or updating Ergo so the extension host receives
the current `PATH`. In a remote, WSL, or development-container window, install
Ergo in that environment.

Set `ergo.executablePath` to an absolute executable path when VS Code should use
a specific Ergo installation instead of resolving `ergo` from the extension
host's `PATH`.

## Develop locally

Open `editors/vscode` as the VS Code workspace, run `npm ci`, and press `F5`.
The launch configuration builds the extension and opens an Extension
Development Host.

Run the focused checks directly when needed:

```sh
npm test
npm run build
```

## Package and install

Create the experimental package:

```sh
npm ci
npm test
npm run package
```

The package is `ergo-0.0.1.vsix`. In VS Code, run **Extensions: Install from
VSIX...**, select that file, and reload the extension host when prompted. Use
the same procedure on macOS and Windows.

To update the experiment, build and install the replacement VSIX. To remove it,
find **Ergo** in the Extensions view and choose **Uninstall**. VSIX installations
do not update automatically.

## Smoke test

Create a disposable backlog with distinct, realistic work:

On macOS:

```sh
scratch="$(mktemp -d)"
ergo init "$scratch"
epic="$(ergo --dir "$scratch" new task "Create one Citation from text that spans pages")"
ergo --dir "$scratch" new task "Decide what each page highlight should show" --epic "$epic"
task="$(ergo --dir "$scratch" new task "Add support-safe plugin diagnostics to server logs")"
code "$scratch"
```

On Windows PowerShell:

```powershell
$scratch = Join-Path $env:TEMP "ergo-vscode-smoke"
New-Item -ItemType Directory -Path $scratch -Force | Out-Null
ergo init $scratch
$epic = ergo --dir $scratch new task "Create one Citation from text that spans pages"
ergo --dir $scratch new task "Decide what each page highlight should show" --epic $epic
$task = ergo --dir $scratch new task "Add support-safe plugin diagnostics to server logs"
code $scratch
```

In the opened window:

1. Click `.ergo/backlog.jsonl` and confirm that the searchable Ergo backlog
   overview opens instead of raw JSONL.
2. Search for `Citation`, expand the epic, and click its child to inspect the
   Markdown preview.
3. Run **Ergo: Backlog**, search for the root task's six-character ID, and
   inspect its preview.
4. Use **Markdown: Switch to Editor View** and compare the read-only source with
   `ergo --color=never --dir <scratch> show <id>`.
5. Open an ordinary folder without `.ergo`, run **Ergo: Backlog**, and
   confirm that Ergo's missing-backlog error is shown.
