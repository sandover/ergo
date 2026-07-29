export const previewScheme = "ergo-task";

export function showArguments(folder: string, id: string): string[] {
  return ["--color=never", "--dir", folder, "show", id];
}

export type PreviewKind = "task" | "epic";

export function previewIdentity(folder: string, id: string, kind: PreviewKind): string {
  return JSON.stringify([folder, id, kind]);
}

export function previewName(id: string, kind: PreviewKind): string {
  return `Ergo ${kind} ${id}`;
}

export class PreviewStore {
  private readonly content = new Map<string, string>();

  set(identity: string, source: string): void {
    this.content.set(identity, source);
  }

  get(identity: string): string | undefined {
    return this.content.get(identity);
  }
}
