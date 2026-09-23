import { ErgoListDocument, ErgoListItem } from "./listing";

export interface WorkspaceBacklogCounts {
  name: string;
  ready: number;
  blocked: number;
}

export function countWorkspaceBacklog(
  name: string,
  document: ErgoListDocument,
): WorkspaceBacklogCounts {
  return {
    name,
    ready: document.items.filter(isReadyTask).length,
    blocked: document.items.filter(isBlockedTask).length,
  };
}

export function formatBacklogStatus(workspaces: WorkspaceBacklogCounts[]): string {
  const ready = workspaces.reduce((total, workspace) => total + workspace.ready, 0);
  const blocked = workspaces.reduce((total, workspace) => total + workspace.blocked, 0);
  return `Ergo: ${ready} ready · ${blocked} blocked`;
}

export function formatBacklogTooltip(workspaces: WorkspaceBacklogCounts[]): string {
  if (workspaces.length <= 1) {
    return "Open the Ergo backlog";
  }
  return [
    "Open the Ergo backlog",
    "",
    ...workspaces.map(
      (workspace) => `${workspace.name}: ${workspace.ready} ready · ${workspace.blocked} blocked`,
    ),
  ].join("\n");
}

function isReadyTask(item: ErgoListItem): boolean {
  return item.kind === "task" && item.state === "todo" && item.ready === true;
}

function isBlockedTask(item: ErgoListItem): boolean {
  return item.kind === "task" && item.state === "blocked";
}
