import * as vscode from "vscode";
import { ErgoCommandError, listArguments, runCompatibleErgo } from "./ergo";
import { runWithCliRecovery } from "./cli_recovery_vscode";
import { ErgoListItem, parseListDocument, toPickerItems } from "./listing";
import { ErgoLiveness } from "./liveness";
import { ErgoPreviewProvider } from "./preview_provider";
import { RefreshGate } from "./refresh_gate";

export async function showLiveBacklogPicker(
  folder: vscode.WorkspaceFolder,
  initialExecutable: string,
  liveness: ErgoLiveness,
  previews: ErgoPreviewProvider,
): Promise<void> {
  const picker = vscode.window.createQuickPick<ErgoQuickPickItem>();
  const gate = new RefreshGate();
  picker.title = "Ergo: Backlog";
  picker.placeholder = "Search by title or ID";
  picker.matchOnDescription = true;
  picker.busy = true;

  let disposed = false;
  let hasItems = false;
  let lastRefreshError = "";
  let executable = initialExecutable;

  const refresh = async (): Promise<boolean> => {
    const activeID = picker.activeItems[0]?.item?.id;
    picker.busy = true;
    const result = await runWithCliRecovery(folder.uri, executable, (selected) =>
      gate.run(
        async () => parseListDocument(
          await runCompatibleErgo(listArguments(folder.uri.fsPath), selected),
        ),
        (document) => {
          const items = quickPickItems(document);
          hasItems = items.some((item) => item.item !== undefined);
          picker.items = items;
          if (activeID) {
            const active = items.find((item) => item.item?.id === activeID);
            picker.activeItems = active ? [active] : [];
          }
          picker.busy = false;
          lastRefreshError = "";
        },
      ),
    );
    if (result.status === "stopped") {
      picker.busy = false;
      lastRefreshError = result.message;
      return false;
    }
    executable = result.executable;
    return true;
  };

  const liveUpdates = liveness.subscribe(folder.uri.fsPath, () => {
    void refresh()
      .then((completed) => {
        if (!completed) {
          picker.hide();
        }
      })
      .catch(async (error: unknown) => {
        picker.busy = false;
        const message = errorMessage(error, "Ergo could not refresh the backlog.");
        if (message !== lastRefreshError) {
          lastRefreshError = message;
          await vscode.window.showErrorMessage(message);
        }
      });
  });
  const accepted = picker.onDidAccept(() => {
    const selected = picker.selectedItems[0] ?? picker.activeItems[0];
    if (!selected?.item) {
      return;
    }
    picker.hide();
    void previews.open(folder.uri.fsPath, selected.item.id, selected.item.kind).catch(
      async (error: unknown) => vscode.window.showErrorMessage(
        errorMessage(error, "Ergo could not open this task."),
      ),
    );
  });
  const hidden = picker.onDidHide(() => {
    if (disposed) {
      return;
    }
    disposed = true;
    gate.dispose();
    liveUpdates.dispose();
    accepted.dispose();
    hidden.dispose();
    picker.dispose();
  });

  picker.show();
  try {
    const refreshed = await refresh();
    if (!refreshed) {
      picker.hide();
      return;
    }
    if (!disposed && !picker.busy && !hasItems) {
      picker.hide();
      await vscode.window.showInformationMessage("No Ergo tasks found.");
    }
  } catch (error) {
    picker.hide();
    throw error;
  }
}

function quickPickItems(document: ReturnType<typeof parseListDocument>): ErgoQuickPickItem[] {
  return toPickerItems(document).map((entry) =>
    entry.type === "separator"
      ? { label: entry.label, kind: vscode.QuickPickItemKind.Separator }
      : {
          label: entry.label,
          description: entry.description,
          item: entry.item,
        },
  );
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof ErgoCommandError || error instanceof Error ? error.message : fallback;
}

type ErgoQuickPickItem = vscode.QuickPickItem & {
  item?: ErgoListItem;
};
