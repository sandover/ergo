import * as vscode from "vscode";
import { isErgoId } from "./copy_id";

export async function copyIdToClipboard(id: string): Promise<void> {
  if (!isErgoId(id)) {
    throw new Error("Ergo can only copy valid six-character IDs.");
  }
  await vscode.env.clipboard.writeText(id);
  vscode.window.setStatusBarMessage(`Copied ${id}`, 1500);
}
