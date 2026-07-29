import * as vscode from "vscode";
import { runCompatibleErgo } from "./ergo";
import {
  previewIdentity,
  previewName,
  previewScheme,
  PreviewKind,
  PreviewStore,
  showArguments,
} from "./preview";

export class ErgoPreviewProvider implements vscode.TextDocumentContentProvider {
  private readonly store = new PreviewStore();
  private readonly changes = new vscode.EventEmitter<vscode.Uri>();

  readonly onDidChange = this.changes.event;

  dispose(): void {
    this.changes.dispose();
  }

  provideTextDocumentContent(uri: vscode.Uri): string {
    return this.store.get(uri.toString()) ?? "";
  }

  async open(folder: string, id: string, kind: PreviewKind): Promise<void> {
    const executable = vscode.workspace
      .getConfiguration("ergo", vscode.Uri.file(folder))
      .get<string>("executablePath", "ergo");
    const source = await runCompatibleErgo(showArguments(folder, id), executable);
    const uri = previewURI(folder, id, kind);
    this.store.set(uri.toString(), source);
    this.changes.fire(uri);
    const document = await vscode.workspace.openTextDocument(uri);
    if (document.languageId !== "markdown") {
      await vscode.languages.setTextDocumentLanguage(document, "markdown");
    }
    await vscode.commands.executeCommand(
      "vscode.openWith",
      uri,
      "vscode.markdown.preview.editor",
    );
  }
}

export function previewURI(folder: string, id: string, kind: PreviewKind): vscode.Uri {
  const identity = previewIdentity(folder, id, kind);
  return vscode.Uri.from({
    scheme: previewScheme,
    authority: "show",
    path: `/${previewName(id, kind)}`,
    query: new URLSearchParams({ identity }).toString(),
  });
}
