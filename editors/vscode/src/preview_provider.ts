import * as path from "node:path";
import * as vscode from "vscode";
import { isCopyIdMessage, isErgoId } from "./copy_id";
import { copyIdToClipboard } from "./copy_id_host";
import { runCompatibleErgo } from "./ergo";
import { runWithCliRecovery } from "./cli_recovery_vscode";
import { ErgoLiveness } from "./liveness";
import { RefreshGate } from "./refresh_gate";
import {
  previewName,
  PreviewKind,
  showArguments,
} from "./preview";
import { renderPreview, renderPreviewNotice } from "./preview_html";

export class ErgoPreviewProvider {
  private readonly panels = new Map<string, PreviewPanel>();

  constructor(private readonly liveness: ErgoLiveness) {}

  async open(folder: string, id: string, kind: PreviewKind): Promise<void> {
    const title = previewName(id, kind);
    const key = JSON.stringify([folder, id, kind]);
    const existing = this.panels.get(key);
    if (existing) {
      await existing.refresh();
      existing.panel.reveal(vscode.ViewColumn.Active);
      return;
    }
    const panel = vscode.window.createWebviewPanel("ergo.detail", title, vscode.ViewColumn.Active, {
      enableScripts: true,
    });
    panel.webview.html = renderPreviewNotice("Loading Ergo detail…", panel.webview.cspSource);
    const refreshGate = new RefreshGate();
    const refresh = async (): Promise<void> => {
      const executable = vscode.workspace
        .getConfiguration("ergo", vscode.Uri.file(folder))
        .get<string>("executablePath", "ergo");
      try {
        const result = await runWithCliRecovery(vscode.Uri.file(folder), executable, (selected) =>
          refreshGate.run(
            () => runCompatibleErgo(showArguments(folder, id), selected),
            (source) => {
              panel.title = title;
              panel.webview.html = renderPreview(source, title, id, kind, panel.webview.cspSource);
            },
          ),
        );
        if (result.status === "stopped") {
          panel.webview.html = renderPreviewNotice(result.message, panel.webview.cspSource);
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : "Ergo could not open this detail.";
        panel.webview.html = renderPreviewNotice(message, panel.webview.cspSource);
        throw error;
      }
    };
    const liveUpdates = this.liveness.subscribe(folder, () => {
      void refresh().catch(() => undefined);
    });
    this.panels.set(key, { panel, refresh });
    panel.onDidDispose(() => {
      refreshGate.dispose();
      liveUpdates.dispose();
      this.panels.delete(key);
    });
    panel.webview.onDidReceiveMessage(async (message: unknown) => {
      if (isCopyIdMessage(message)) {
        if (message.id !== id) {
          return;
        }
        try {
          await copyIdToClipboard(message.id);
        } catch {
          await vscode.window.showErrorMessage(`Could not copy ID ${message.id}.`);
        }
        return;
      }
      if (isOpenEpicMessage(message)) {
        try {
          await this.open(folder, message.id, "epic");
        } catch {
          await vscode.window.showErrorMessage(`Ergo could not open epic ${message.id}.`);
        }
        return;
      }
      if (!isOpenFileMessage(message)) {
        return;
      }
      try {
        const uri = vscode.Uri.parse(message.uri);
        if (uri.scheme !== "file" || !isWithin(folder, uri.fsPath)) {
          await vscode.window.showErrorMessage("Ergo can only open result files from this workspace.");
          return;
        }
        const document = await vscode.workspace.openTextDocument(uri);
        await vscode.window.showTextDocument(document, { preview: true });
      } catch {
        await vscode.window.showErrorMessage("Ergo could not open that result file.");
      }
    });
    await refresh();
  }
}

interface PreviewPanel {
  panel: vscode.WebviewPanel;
  refresh(): Promise<void>;
}

function isOpenEpicMessage(value: unknown): value is { type: "openEpic"; id: string } {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const message = value as Record<string, unknown>;
  return Object.keys(message).length === 2 &&
    message.type === "openEpic" &&
    typeof message.id === "string" &&
    isErgoId(message.id);
}

function isOpenFileMessage(value: unknown): value is { type: "openFile"; uri: string } {
  return typeof value === "object" && value !== null &&
    (value as { type?: unknown }).type === "openFile" &&
    typeof (value as { uri?: unknown }).uri === "string";
}

function isWithin(folder: string, file: string): boolean {
  const relative = path.relative(folder, file);
  return relative !== "" && relative !== ".." && !relative.startsWith(`..${path.sep}`) &&
    !path.isAbsolute(relative);
}
