export const supportedListVersion = 1;

export type ErgoItemKind = "epic" | "task";

export interface ErgoListItem {
  id: string;
  title: string;
  kind: ErgoItemKind;
  state?: string;
  ready?: boolean;
  epic_id?: string;
}

export interface ErgoListDocument {
  version: number;
  items: ErgoListItem[];
}

export interface PickerSeparator {
  type: "separator";
  label: string;
}

export interface PickerItem {
  type: "item";
  label: string;
  description: string;
  item: ErgoListItem;
}

export type PickerEntry = PickerSeparator | PickerItem;

export function parseListDocument(input: string): ErgoListDocument {
  let value: unknown;
  try {
    value = JSON.parse(input);
  } catch {
    throw new Error("Ergo returned an invalid task listing.");
  }
  if (!isObject(value) || value.version !== supportedListVersion || !Array.isArray(value.items)) {
    if (isObject(value) && typeof value.version === "number") {
      throw new Error(
        `This extension does not support Ergo task listing version ${value.version}.`,
      );
    }
    throw new Error("Ergo returned an invalid task listing.");
  }
  const items = value.items.map(parseListItem);
  return { version: supportedListVersion, items };
}

export function toPickerItems(document: ErgoListDocument): PickerEntry[] {
  const entries: PickerEntry[] = [];
  const rootTasks = document.items.filter(
    (item) => item.kind === "task" && !item.epic_id,
  );
  const epics = document.items.filter((item) => item.kind === "epic");
  const knownEpics = new Set(epics.map((item) => item.id));
  const ungroupedTasks = document.items.filter(
    (item) =>
      item.kind === "task" && item.epic_id !== undefined && !knownEpics.has(item.epic_id),
  );

  if (rootTasks.length > 0 || ungroupedTasks.length > 0) {
    entries.push({ type: "separator", label: "ROOT TASKS" });
    for (const item of [...rootTasks, ...ungroupedTasks]) {
      entries.push(taskPickerItem(item, false));
    }
  }

  if (epics.length > 0) {
    entries.push({ type: "separator", label: "EPICS" });
    for (const epic of epics) {
      const children = document.items.filter((item) => item.epic_id === epic.id);
      const state = derivedEpicState(children);
      entries.push({
        type: "item",
        label: `${state === "failed" ? statusIcon(state) : "$(symbol-structure)"} ${epic.title}`,
        description: `${epic.id} · ${children.length} ${children.length === 1 ? "task" : "tasks"}${state === "failed" ? " · failed" : ""}`,
        item: epic,
      });
      for (const child of children) {
        entries.push(taskPickerItem(child, true));
      }
    }
  }

  return entries;
}

export function derivedEpicState(children: ErgoListItem[]): string {
  if (
    children.length === 0 ||
    !children.every((child) => ["done", "failed", "canceled"].includes(child.state ?? ""))
  ) {
    return "active";
  }
  return children.some((child) => child.state === "failed") ? "failed" : "done";
}

function taskPickerItem(item: ErgoListItem, child: boolean): PickerItem {
  const status =
    item.state === "todo" ? (item.ready ? "ready" : "waiting") : item.state ?? "task";
  const prefix = child ? "    ↳ " : "";
  return {
    type: "item",
    label: `${prefix}${statusIcon(status)} ${item.title}`,
    description: `${item.id} · ${status}`,
    item,
  };
}

function statusIcon(status: string): string {
  switch (status) {
    case "ready":
      return "$(circle-outline)";
    case "waiting":
      return "$(clock)";
    case "doing":
      return "$(sync)";
    case "blocked":
      return "$(error)";
    case "failed":
      return "$(close)";
    case "done":
      return "$(check)";
    case "canceled":
      return "$(circle-slash)";
    case "error":
      return "$(warning)";
    default:
      return "$(circle-outline)";
  }
}

function parseListItem(value: unknown): ErgoListItem {
  if (
    !isObject(value) ||
    typeof value.id !== "string" ||
    typeof value.title !== "string" ||
    (value.kind !== "epic" && value.kind !== "task")
  ) {
    throw new Error("Ergo returned an invalid task listing.");
  }
  const item: ErgoListItem = {
    id: value.id,
    title: value.title,
    kind: value.kind,
  };
  if (value.kind === "task") {
    if (typeof value.state !== "string" || typeof value.ready !== "boolean") {
      throw new Error("Ergo returned an invalid task listing.");
    }
    item.state = value.state;
    item.ready = value.ready;
    if (value.epic_id !== undefined) {
      if (typeof value.epic_id !== "string") {
        throw new Error("Ergo returned an invalid task listing.");
      }
      item.epic_id = value.epic_id;
    }
  }
  return item;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
