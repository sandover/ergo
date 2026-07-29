import * as path from "node:path";
import * as vscode from "vscode";
import { listArguments, runCompatibleErgo } from "./ergo";
import { renderBacklog } from "./backlog_html";
import { parseListDocument } from "./listing";
import { PreviewKind } from "./preview";
import { ErgoPreviewProvider } from "./preview_provider";

export const backlogViewType = "ergo.backlog";

export class ErgoBacklogEditor implements vscode.CustomReadonlyEditorProvider {
  constructor(private readonly previews: ErgoPreviewProvider) {}

  openCustomDocument(uri: vscode.Uri): vscode.CustomDocument {
    return { uri, dispose: () => undefined };
  }

  async resolveCustomEditor(
    document: vscode.CustomDocument,
    panel: vscode.WebviewPanel,
  ): Promise<void> {
    const folder = path.dirname(path.dirname(document.uri.fsPath));
    panel.webview.options = { enableScripts: true };
    let itemKinds = new Map<string, PreviewKind>();

    const refresh = async (): Promise<void> => {
      try {
        const executable = vscode.workspace
          .getConfiguration("ergo", document.uri)
          .get<string>("executablePath", "ergo");
        const output = await runCompatibleErgo(listArguments(folder), executable);
        const listing = parseListDocument(output);
        const view = renderBacklog(listing, panel.webview.cspSource);
        itemKinds = new Map(listing.items.map((item) => [item.id, item.kind]));
        panel.webview.html = view.html;
      } catch (error) {
        const message = error instanceof Error ? error.message : "Ergo could not open this backlog.";
        panel.webview.html = errorHtml(message, panel.webview.cspSource);
      }
    };

    const messages = panel.webview.onDidReceiveMessage(async (message: unknown) => {
      if (!isOpenMessage(message)) {
        return;
      }
      const kind = itemKinds.get(message.id);
      if (!kind) {
        return;
      }
      try {
        await this.previews.open(folder, message.id, kind);
      } catch (error) {
        const message = error instanceof Error ? error.message : "Ergo could not open this task.";
        await vscode.window.showErrorMessage(message);
      }
    });
    const watcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(path.dirname(document.uri.fsPath), path.basename(document.uri.fsPath)),
    );
    watcher.onDidChange(refresh);
    watcher.onDidCreate(refresh);
    panel.onDidDispose(() => {
      messages.dispose();
      watcher.dispose();
    });

    await refresh();
  }
}

function isOpenMessage(value: unknown): value is { type: "open"; id: string } {
  return typeof value === "object" && value !== null &&
    (value as { type?: unknown }).type === "open" &&
    typeof (value as { id?: unknown }).id === "string";
}

function errorHtml(message: string, cspSource: string): string {
  const safe = message.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
  return `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource};"></head><body><p>${safe}</p></body></html>`;
}
