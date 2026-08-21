import { randomBytes } from "node:crypto";
import { derivedEpicState, ErgoListDocument, ErgoListItem } from "./listing";

export interface BacklogView {
  html: string;
  itemIds: Set<string>;
}

export function renderBacklog(
  document: ErgoListDocument,
  cspSource: string,
  nonce = randomBytes(16).toString("base64"),
): BacklogView {
  const rootTasks = document.items.filter(
    (item) => item.kind === "task" && !item.epic_id,
  );
  const epics = document.items.filter((item) => item.kind === "epic");
  const itemIds = new Set(document.items.map((item) => item.id));

  const roots = rootTasks.length
    ? `<section>
        <div class="rows">${rootTasks.map(renderTask).join("")}</div>
      </section>`
    : "";
  const epicSections = epics.map((epic) => {
    const children = document.items.filter((item) => item.epic_id === epic.id);
    return renderEpic(epic, children);
  }).join("");

  const html = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource} 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
  <title>Ergo Backlog</title>
  <style nonce="${nonce}">
    :root { color-scheme: light dark; }
    body { color: var(--vscode-foreground); background: var(--vscode-editor-background); font-family: var(--vscode-font-family); font-size: var(--vscode-font-size); font-weight: var(--vscode-font-weight); line-height: 1.35; margin: 0 auto; max-width: 920px; padding: 20px 24px 64px; }
    header { margin-bottom: 14px; }
    h1 { font-size: 20px; line-height: 1.2; margin: 0; }
    h2 { color: var(--vscode-descriptionForeground); font-size: 11px; letter-spacing: .09em; margin: 34px 8px 14px; text-transform: uppercase; }
    .meta { color: var(--vscode-descriptionForeground); }
    .filters { border-bottom: 1px solid var(--vscode-widget-border, color-mix(in srgb, var(--vscode-foreground) 14%, transparent)); display: grid; gap: 8px; padding-bottom: 14px; }
    input[type="search"] { background: var(--vscode-input-background); border: 1px solid var(--vscode-input-border, transparent); color: var(--vscode-input-foreground); font: inherit; padding: 7px 9px; width: 100%; }
    input[type="search"]:focus { border-color: var(--vscode-focusBorder); outline: none; }
    .ready-filter { align-items: center; cursor: pointer; display: inline-flex; gap: 7px; justify-self: start; }
    .ready-filter input { accent-color: var(--vscode-checkbox-background); cursor: pointer; margin: 0; }
    main { margin-top: 18px; }
    section { margin-bottom: 18px; }
    .row { align-items: center; border-radius: 3px; display: grid; gap: 12px; grid-template-columns: 18px minmax(0, 1fr) auto; padding: 3px 8px; }
    .row:hover, .row:focus-within { background: var(--vscode-list-hoverBackground); }
    button.item { background: none; border: 0; color: var(--vscode-textLink-foreground); cursor: pointer; font: inherit; min-width: 0; overflow: hidden; padding: 0; text-align: left; text-overflow: ellipsis; white-space: nowrap; }
    button.item:hover { color: var(--vscode-textLink-activeForeground); text-decoration: underline; }
    button.item:focus { outline: 1px solid var(--vscode-focusBorder); outline-offset: 2px; }
    .state { color: var(--vscode-descriptionForeground); font-size: 14px; line-height: 1; text-align: center; }
    .state[data-state="doing"] { color: var(--vscode-progressBar-background); }
    .state[data-state="blocked"] { color: var(--vscode-errorForeground); font-weight: 600; }
    .state[data-state="failed"], .state[data-state="error"] { color: var(--vscode-errorForeground); font-weight: 600; }
    .item.id { color: var(--vscode-descriptionForeground); font-family: var(--vscode-editor-font-family); font-size: 12px; }
    details { margin: 0 0 18px; }
    details > summary { border-radius: 3px; cursor: pointer; list-style-position: outside; padding: 5px 8px; }
    details > summary:hover, details > summary:focus { background: var(--vscode-list-hoverBackground); outline: none; }
    details > summary::marker { color: var(--vscode-descriptionForeground); }
    .epic-heading { align-items: baseline; display: grid; gap: 12px; grid-template-columns: minmax(0, 1fr) auto; }
    .epic-title { font-weight: 600; }
    .epic-progress { color: var(--vscode-descriptionForeground); display: block; font-size: 12px; margin-top: 3px; }
    .children { margin: 3px 0 0 22px; }
    .empty { color: var(--vscode-descriptionForeground); padding: 28px 0; text-align: center; }
    [hidden] { display: none !important; }
  </style>
</head>
<body>
  <header><h1>Ergo backlog</h1></header>
  <div class="filters">
    <input id="search" type="search" aria-label="Search tasks" placeholder="Search by title or ID">
    <label class="ready-filter"><input id="readyOnly" type="checkbox">Ready only</label>
  </div>
  <main>${roots}${epicSections || (!roots ? '<p class="empty">No tasks found.</p>' : "")}</main>
  <p id="noMatches" class="empty" hidden>No matching tasks.</p>
  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi();
    const search = document.getElementById("search");
    const readyOnly = document.getElementById("readyOnly");
    const groups = Array.from(document.querySelectorAll("section, details"));
    document.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-id]");
      if (button) {
        event.preventDefault();
        event.stopPropagation();
        vscode.postMessage({ type: "open", id: button.dataset.id });
      }
    });
    function applyFilters() {
      const query = search.value.trim().toLocaleLowerCase();
      const onlyReady = readyOnly.checked;
      let visibleItems = 0;
      let visibleGroups = 0;
      for (const group of groups) {
        const rows = Array.from(group.querySelectorAll("[data-search]"));
        let groupMatches = 0;
        for (const row of rows) {
          const matchesQuery = !query || row.dataset.search.includes(query);
          const matchesReady = !onlyReady || row.dataset.ready === "true";
          const visible = matchesQuery && matchesReady;
          row.hidden = !visible;
          if (visible) groupMatches++;
        }
        if (group.tagName === "DETAILS") {
          const epicMatches = !query || group.dataset.epicSearch.includes(query);
          group.hidden = query && !epicMatches && groupMatches === 0;
          if (!group.hidden) visibleGroups++;
          if ((epicMatches || groupMatches) && query) group.open = true;
        } else {
          group.hidden = groupMatches === 0;
          if (!group.hidden) visibleGroups++;
        }
        visibleItems += groupMatches;
      }
      document.getElementById("noMatches").hidden = visibleItems !== 0 || visibleGroups !== 0;
    }
    search.addEventListener("input", applyFilters);
    readyOnly.addEventListener("change", applyFilters);
  </script>
</body>
</html>`;

  return { html, itemIds };
}

function renderEpic(epic: ErgoListItem, children: ErgoListItem[]): string {
  const counts = new Map<string, number>();
  for (const child of children) {
    const state = displayState(child);
    counts.set(state, (counts.get(state) ?? 0) + 1);
  }
  const epicState = derivedEpicState(children);
  const progress = ["ready", "doing", "waiting", "blocked", "failed", "done", "canceled", "error"]
    .flatMap((state) => {
      const count = counts.get(state);
      return count ? [`${count} ${state}`] : [];
    })
    .join(" · ");
  return `<details open data-epic-search="${searchText([epic])}">
    <summary>
      <span class="epic-heading">
        <span class="epic-title">${epicState === "failed" ? '<span class="state" data-state="failed" title="failed">✗</span> ' : ""}${text(epic.title)}</span>
        <button class="item id" data-id="${attribute(epic.id)}">${text(epic.id)}</button>
      </span>
      ${progress ? `<span class="epic-progress">${progress}</span>` : ""}
    </summary>
    <div class="children">${children.map(renderTask).join("")}</div>
  </details>`;
}

function renderTask(item: ErgoListItem): string {
  const state = displayState(item);
  return `<div class="row" data-search="${searchText([item])}" data-ready="${item.ready === true}">
    <span class="state" data-state="${attribute(state)}" title="${attribute(state)}">${stateSymbol(state)}</span>
    <span>${text(item.title)}</span>
    <button class="item id" data-id="${attribute(item.id)}">${text(item.id)}</button>
  </div>`;
}

function displayState(item: ErgoListItem): string {
  if (item.state === "todo") {
    return item.ready ? "ready" : "waiting";
  }
  return item.state ?? "task";
}

function stateSymbol(state: string): string {
  return (
    ({
      ready: "○",
      waiting: "◷",
      doing: "↻",
      blocked: "!",
      failed: "✗",
      done: "✓",
      canceled: "–",
      error: "⚠",
    } as Record<string, string>)[state] ?? "○"
  );
}

function searchText(items: ErgoListItem[]): string {
  return attribute(items.map((item) => `${item.title} ${item.id}`).join(" ").toLocaleLowerCase());
}

function text(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function attribute(value: string): string {
  return text(value).replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}
