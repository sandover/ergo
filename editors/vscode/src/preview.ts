export function showArguments(folder: string, id: string): string[] {
  return ["--color=never", "--dir", folder, "show", id];
}

export type PreviewKind = "task" | "epic";

export function previewName(id: string, kind: PreviewKind): string {
  return `Ergo ${kind} ${id}`;
}
