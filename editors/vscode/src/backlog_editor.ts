import * as path from "node:path";
import * as vscode from "vscode";
import { listArguments, runCompatibleErgo } from "./ergo";
import { renderBacklog } from "./backlog_html";
import { copyIdToClipboard } from "./copy_id_host";
import { isCopyIdMessage } from "./copy_id";
import { parseListDocument } from "./listing";
import { ErgoLiveness } from "./liveness";
import { RefreshGate } from "./refresh_gate";
import { PreviewKind } from "./preview";
import { ErgoPreviewProvider } from "./preview_provider";

export const backlogViewType = "ergo.backlog";

export class ErgoBacklogEditor implements vscode.CustomReadonlyEditorProvider {
  constructor(
    private readonly previews: ErgoPreviewProvider,
    private readonly liveness: ErgoLiveness,
  ) {}

  openCustomDocument(uri: vscode.Uri): vscode.CustomDocument {
    return { uri, dispose: () => undefined };
  }

  async resolveCustomEditor(
    document: vscode.CustomDocument,
    panel: vscode.WebviewPanel,
  ): Promise<void> {
    const folder = path.dirname(path.dirname(document.uri.fsPath));
    panel.webview.options = { enableScripts: true };
    panel.webview.html = noticeHtml("Loading Ergo backlog…", panel.webview.cspSource);
    let itemKinds = new Map<string, PreviewKind>();
    const refreshGate = new RefreshGate();

    const refresh = async (): Promise<void> => {
      try {
        const executable = vscode.workspace
          .getConfiguration("ergo", document.uri)
          .get<string>("executablePath", "ergo");
        await refreshGate.run(
          async () => parseListDocument(
            await runCompatibleErgo(listArguments(folder), executable),
          ),
          (listing) => {
            const view = renderBacklog(listing, panel.webview.cspSource);
            itemKinds = new Map(listing.items.map((item) => [item.id, item.kind]));
            panel.webview.html = view.html;
          },
        );
      } catch (error) {
        const message = error instanceof Error ? error.message : "Ergo could not open this backlog.";
        panel.webview.html = noticeHtml(message, panel.webview.cspSource);
      }
    };

    const messages = panel.webview.onDidReceiveMessage(async (message: unknown) => {
      if (isCopyIdMessage(message)) {
        if (!itemKinds.has(message.id)) {
          return;
        }
        try {
          await copyIdToClipboard(message.id);
        } catch {
          await vscode.window.showErrorMessage(`Could not copy ID ${message.id}.`);
        }
        return;
      }
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
    const liveUpdates = this.liveness.subscribe(folder, refresh);
    panel.onDidDispose(() => {
      refreshGate.dispose();
      messages.dispose();
      liveUpdates.dispose();
    });

    await refresh();
  }
}

function isOpenMessage(value: unknown): value is { type: "open"; id: string } {
  return typeof value === "object" && value !== null &&
    (value as { type?: unknown }).type === "open" &&
    typeof (value as { id?: unknown }).id === "string";
}

function noticeHtml(message: string, cspSource: string): string {
  const safe = message.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
  return `<!doctype html><html><head><meta charset="UTF-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource} 'unsafe-inline';"><style>body{background:#0f181e;color:#8f9598;font:15px/1.6 var(--vscode-font-family,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif);margin:0 auto;max-width:960px;padding:28px 32px}</style></head><body><p>${safe}</p></body></html>`;
}
