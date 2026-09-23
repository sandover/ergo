import * as vscode from "vscode";
import { ErgoCommandError, listArguments, runCompatibleErgo } from "./ergo";
import { ErgoLiveness } from "./liveness";
import { parseListDocument } from "./listing";
import { countWorkspaceBacklog, formatBacklogStatus, formatBacklogTooltip } from "./status_counts";

export class ErgoStatusBar implements vscode.Disposable {
  private readonly item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 10);
  private readonly subscriptions: vscode.Disposable[] = [];
  private generation = 0;
  private disposed = false;

  constructor(private readonly liveness: ErgoLiveness) {
    this.item.command = "ergo.listTasks";
    this.subscriptions.push(
      vscode.workspace.onDidChangeWorkspaceFolders(() => {
        this.rebuildSubscriptions();
        void this.refresh();
      }),
    );
    this.rebuildSubscriptions();
    void this.refresh();
  }

  dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.generation++;
    for (const subscription of this.subscriptions) {
      subscription.dispose();
    }
    this.item.dispose();
  }

  refreshForConfigurationChange(): void {
    if (!this.disposed) {
      void this.refresh();
    }
  }

  private rebuildSubscriptions(): void {
    for (const subscription of this.subscriptions.splice(1)) {
      subscription.dispose();
    }
    for (const folder of vscode.workspace.workspaceFolders ?? []) {
      this.subscriptions.push(
        this.liveness.subscribe(folder.uri.fsPath, () => void this.refresh()),
      );
    }
  }

  private async refresh(): Promise<void> {
    const generation = ++this.generation;
    const folders = [...(vscode.workspace.workspaceFolders ?? [])];
    try {
      const workspaces = [];
      for (const folder of folders) {
        if (!(await hasBacklog(folder.uri))) {
          continue;
        }
        const executable = vscode.workspace
          .getConfiguration("ergo", folder.uri)
          .get<string>("executablePath", "ergo");
        const output = await runCompatibleErgo(
          listArguments(folder.uri.fsPath),
          executable,
        );
        workspaces.push(
          countWorkspaceBacklog(folder.name, parseListDocument(output)),
        );
      }
      if (this.disposed || generation !== this.generation) {
        return;
      }
      if (workspaces.length === 0) {
        this.item.hide();
        return;
      }
      this.item.text = formatBacklogStatus(workspaces);
      this.item.tooltip = formatBacklogTooltip(workspaces);
      this.item.show();
    } catch (error) {
      if (this.disposed || generation !== this.generation) {
        return;
      }
      // A failed refresh must never leave old counts visible.
      this.item.hide();
      if (error instanceof ErgoCommandError) {
        console.warn(`Ergo status refresh failed: ${error.message}`);
      } else if (error instanceof Error) {
        console.warn(`Ergo status refresh failed: ${error.message}`);
      }
    }
  }
}

async function hasBacklog(folder: vscode.Uri): Promise<boolean> {
  try {
    const stat = await vscode.workspace.fs.stat(vscode.Uri.joinPath(folder, ".ergo", "backlog.jsonl"));
    return (stat.type & vscode.FileType.File) !== 0;
  } catch {
    return false;
  }
}
