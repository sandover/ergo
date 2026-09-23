import * as vscode from "vscode";
import { backlogViewType, ErgoBacklogEditor } from "./backlog_editor";
import {
  clearCompatibilityCache,
  ErgoCommandError,
} from "./ergo";
import { ErgoLiveness } from "./liveness";
import { showLiveBacklogPicker } from "./picker";
import { ErgoPreviewProvider } from "./preview_provider";
import { ErgoStatusBar } from "./status_bar";

export function activate(context: vscode.ExtensionContext): void {
  const liveness = new ErgoLiveness();
  const status = new ErgoStatusBar(liveness);
  const previews = new ErgoPreviewProvider(liveness);
  context.subscriptions.push(
    liveness,
    status,
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("ergo.executablePath")) {
        clearCompatibilityCache();
        status.refreshForConfigurationChange();
      }
    }),
    vscode.window.registerCustomEditorProvider(
      backlogViewType,
      new ErgoBacklogEditor(previews, liveness),
      {
        webviewOptions: { retainContextWhenHidden: true },
        supportsMultipleEditorsPerDocument: false,
      },
    ),
    vscode.commands.registerCommand("ergo.listTasks", async () => {
      try {
        const folder = await chooseWorkspaceFolder();
        if (!folder) {
          return;
        }
        await showLiveBacklogPicker(
          folder,
          ergoExecutable(folder.uri),
          liveness,
          previews,
        );
      } catch (error) {
        const message =
          error instanceof ErgoCommandError || error instanceof Error
            ? error.message
            : "Ergo could not list tasks.";
        await vscode.window.showErrorMessage(message);
      }
    }),
  );
}

async function chooseWorkspaceFolder(): Promise<vscode.WorkspaceFolder | undefined> {
  const folders = vscode.workspace.workspaceFolders;
  if (!folders || folders.length === 0) {
    await vscode.window.showErrorMessage("Open an Ergo project folder first.");
    return undefined;
  }
  if (folders.length === 1) {
    return folders[0];
  }
  const active = vscode.window.activeTextEditor?.document.uri;
  if (active) {
    const activeFolder = vscode.workspace.getWorkspaceFolder(active);
    if (activeFolder) {
      return activeFolder;
    }
  }
  return vscode.window.showWorkspaceFolderPick({
    placeHolder: "Choose the Ergo project to inspect",
  });
}

export function deactivate(): void {}

function ergoExecutable(resource: vscode.Uri): string {
  return vscode.workspace
    .getConfiguration("ergo", resource)
    .get<string>("executablePath", "ergo");
}
