import * as vscode from "vscode";
import { backlogViewType, ErgoBacklogEditor } from "./backlog_editor";
import {
  clearCompatibilityCache,
  ErgoCommandError,
  listArguments,
  runCompatibleErgo,
} from "./ergo";
import { ErgoListItem, parseListDocument, toPickerItems } from "./listing";
import { previewScheme } from "./preview";
import { ErgoPreviewProvider } from "./preview_provider";

export function activate(context: vscode.ExtensionContext): void {
  const previews = new ErgoPreviewProvider();
  context.subscriptions.push(
    previews,
    vscode.workspace.registerTextDocumentContentProvider(previewScheme, previews),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("ergo.executablePath")) {
        clearCompatibilityCache();
      }
    }),
    vscode.window.registerCustomEditorProvider(
      backlogViewType,
      new ErgoBacklogEditor(previews),
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
        const output = await runCompatibleErgo(
          listArguments(folder.uri.fsPath),
          ergoExecutable(folder.uri),
        );
        const entries = toPickerItems(parseListDocument(output));
        if (entries.length === 0) {
          await vscode.window.showInformationMessage("No Ergo tasks found.");
          return;
        }
        const items: ErgoQuickPickItem[] = entries.map((entry) =>
          entry.type === "separator"
            ? { label: entry.label, kind: vscode.QuickPickItemKind.Separator }
            : {
                label: entry.label,
                description: entry.description,
                item: entry.item,
              },
        );
        const selected = await vscode.window.showQuickPick(items, {
          title: "Ergo: Backlog",
          placeHolder: "Search by title or ID",
          matchOnDescription: true,
        });
        if (selected?.item) {
          await previews.open(folder.uri.fsPath, selected.item.id, selected.item.kind);
        }
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

type ErgoQuickPickItem = vscode.QuickPickItem & {
  item?: ErgoListItem;
};
