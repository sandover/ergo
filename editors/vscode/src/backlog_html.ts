// This module owns the read-only HTML projection of Ergo's list JSON for VS Code.
// The CLI listing is authoritative; this renderer may add presentation but not backlog state.
// IDs stay clickable and escaped, and filtering continues to operate on rendered rows.

import { randomBytes } from "node:crypto";
import {
  derivedEpicState,
  ErgoListDocument,
  ErgoListItem,
  taskStatusDescription,
  taskStatusWord,
} from "./listing";
import { copyIdButton, copyIdStyles } from "./copy_id";

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
  const itemsById = new Map(document.items.map((item) => [item.id, item]));

  const roots = rootTasks.length
    ? `<section>
        <div class="rows">${rootTasks.map((item) => renderTask(item, itemsById)).join("")}</div>
      </section>`
    : "";
  const epicSections = epics.map((epic) => {
    const children = document.items.filter((item) => item.epic_id === epic.id);
    return renderEpic(epic, children, itemsById);
  }).join("");

  const html = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource} 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
  <title>Ergo Backlog</title>
  <style nonce="${nonce}">
    :root { color-scheme: dark; --surface: #0f181e; --surface-raised: #161e25; --surface-hover: #182228; --border: #242f35; --text: #8f9598; --text-strong: #9da4a7; --text-muted: #656b70; --accent: #8298a0; --success: #6e9ba4; --warning: #649796; --error: #6a93ac; }
    body { color: var(--text); background: var(--surface); font-family: var(--vscode-font-family); font-size: var(--vscode-font-size); font-weight: var(--vscode-font-weight); line-height: 1.35; margin: 0 auto; max-width: 920px; padding: 20px 24px 64px; }
    header { margin-bottom: 14px; }
    h1 { font-size: 20px; line-height: 1.2; margin: 0; }
    h2 { color: var(--text-muted); font-size: 11px; letter-spacing: .09em; margin: 34px 8px 14px; text-transform: uppercase; }
    .meta { color: var(--text-muted); }
    .filters { border-bottom: 1px solid var(--border); display: grid; gap: 8px; padding-bottom: 14px; }
    input[type="search"] { background: var(--surface-raised); border: 1px solid var(--border); color: var(--text-strong); font: inherit; padding: 7px 9px; width: 100%; }
    input[type="search"]:focus { border-color: var(--accent); outline: none; }
    .ready-filter { align-items: center; cursor: pointer; display: inline-flex; gap: 7px; justify-self: start; }
    .ready-filter input { accent-color: var(--vscode-checkbox-background); cursor: pointer; margin: 0; }
    main { margin-top: 18px; }
    section { margin-bottom: 18px; }
    .row { align-items: center; border-radius: 3px; display: grid; gap: 12px; grid-template-columns: auto auto minmax(0, 1fr); padding: 3px 8px; position: relative; }
    .row:hover, .row:focus-within { background: var(--surface-hover); }
    button.item { background: none; border: 0; color: var(--accent); cursor: pointer; font: inherit; min-width: 0; overflow: hidden; padding: 0; text-align: left; text-overflow: ellipsis; white-space: nowrap; }
    button.item:hover { color: var(--text-strong); text-decoration: underline; }
    button.item:focus { outline: 1px solid var(--accent); outline-offset: 2px; }
    .state { align-items: center; color: var(--text-muted); cursor: default; display: inline-flex; font-size: 12px; gap: 5px; line-height: 1; white-space: nowrap; }
    .state[data-tooltip]::after { background: var(--surface-raised); border: 1px solid var(--border); border-radius: 4px; bottom: calc(100% + 7px); box-sizing: border-box; box-shadow: 0 3px 10px rgb(0 0 0 / 28%); color: var(--text-strong); content: attr(data-tooltip); font-size: 12px; font-weight: 400; left: 8px; max-width: min(360px, calc(100% - 16px)); opacity: 0; overflow-wrap: anywhere; padding: 5px 7px; pointer-events: none; position: absolute; text-transform: none; transform: translateY(2px); transition: opacity .1s ease, transform .1s ease; visibility: hidden; white-space: normal; width: max-content; z-index: 2; }
    .state[data-tooltip]:hover::after { opacity: 1; transform: translateY(0); visibility: visible; }
    .state-symbol { font-size: 14px; }
    .state[data-state="draft"] { color: var(--text-muted); }
    .state[data-state="ready"], .state[data-state="done"] { color: var(--success); }
    .state[data-state="in progress"] { color: var(--warning); }
    .state[data-state="blocked"], .state[data-state="failed"], .state[data-state="legacy error"] { color: var(--error); font-weight: 600; }
    .item.id { color: var(--text-muted); font-family: var(--vscode-editor-font-family); font-size: 12px; }
    details { margin: 0 0 18px; }
    details > summary { border-radius: 3px; cursor: pointer; list-style-position: outside; padding: 5px 8px; }
    details > summary:hover, details > summary:focus { background: var(--surface-hover); outline: none; }
    details > summary::marker { color: var(--text-muted); }
    .epic-heading { align-items: baseline; display: grid; gap: 12px; grid-template-columns: auto minmax(0, 1fr) auto; }
    .epic-title, .task-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .epic-title { font-weight: 600; }
    .epic-progress { color: var(--text-muted); font-size: 12px; text-align: right; white-space: nowrap; }
    .children { margin: 3px 0 0; }
    mark.search-hit { background: color-mix(in srgb, var(--accent) 28%, transparent); border-radius: 2px; color: var(--text-strong); padding: 0 1px; }
    .empty { color: var(--text-muted); padding: 28px 0; text-align: center; }
    @media (max-width: 600px) {
      .epic-heading { grid-template-columns: auto minmax(0, 1fr); }
      .epic-progress { grid-column: 2; text-align: left; }
    }
    ${copyIdStyles}
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
    const saved = vscode.getState() || {};
    search.value = typeof saved.search === "string" ? saved.search : "";
    readyOnly.checked = saved.readyOnly === true;
    if (saved.epics && typeof saved.epics === "object") {
      for (const details of document.querySelectorAll("details[data-epic-id]")) {
        const open = saved.epics[details.dataset.epicId];
        if (typeof open === "boolean") details.open = open;
      }
    }
    function saveState() {
      const epics = {};
      for (const details of document.querySelectorAll("details[data-epic-id]")) {
        epics[details.dataset.epicId] = details.open;
      }
      const active = document.activeElement;
      const focus = active === search ? "search" : active === readyOnly ? "readyOnly" : undefined;
      vscode.setState({ search: search.value, readyOnly: readyOnly.checked, epics, scrollY: window.scrollY, focus });
    }
    document.addEventListener("click", (event) => {
      const copyButton = event.target.closest("button[data-copy-id]");
      if (copyButton) {
        event.preventDefault();
        event.stopPropagation();
        vscode.postMessage({ type: "copyId", id: copyButton.dataset.copyId });
        return;
      }
      const button = event.target.closest("button[data-id]");
      if (button) {
        event.preventDefault();
        event.stopPropagation();
        vscode.postMessage({ type: "open", id: button.dataset.id });
      }
    });
    function clearSearchHits() {
      for (const hit of document.querySelectorAll("mark.search-hit")) {
        hit.replaceWith(document.createTextNode(hit.textContent || ""));
      }
    }
    function highlightSearchHits(query) {
      clearSearchHits();
      if (!query) return;
      for (const element of document.querySelectorAll(".item.id, .epic-title, .task-title")) {
        const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
        const nodes = [];
        while (walker.nextNode()) nodes.push(walker.currentNode);
        for (const node of nodes) {
          const value = node.nodeValue || "";
          const lower = value.toLocaleLowerCase();
          let start = 0;
          let match;
          const fragment = document.createDocumentFragment();
          while ((match = lower.indexOf(query, start)) !== -1) {
            fragment.append(value.slice(start, match));
            const hit = document.createElement("mark");
            hit.className = "search-hit";
            hit.textContent = value.slice(match, match + query.length);
            fragment.append(hit);
            start = match + query.length;
          }
          if (start > 0) {
            fragment.append(value.slice(start));
            node.replaceWith(fragment);
          }
        }
      }
    }
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
      highlightSearchHits(query);
    }
    search.addEventListener("input", () => { applyFilters(); saveState(); });
    readyOnly.addEventListener("change", () => { applyFilters(); saveState(); });
    document.addEventListener("toggle", saveState, true);
    document.addEventListener("focusin", saveState);
    let scrollFrame;
    window.addEventListener("scroll", () => {
      if (scrollFrame) cancelAnimationFrame(scrollFrame);
      scrollFrame = requestAnimationFrame(saveState);
    }, { passive: true });
    applyFilters();
    requestAnimationFrame(() => {
      if (typeof saved.scrollY === "number") window.scrollTo(0, saved.scrollY);
      if (saved.focus === "search" || saved.focus === "readyOnly") {
        document.getElementById(saved.focus).focus({ preventScroll: true });
      }
    });
  </script>
</body>
</html>`;

  return { html, itemIds };
}

function renderEpic(
  epic: ErgoListItem,
  children: ErgoListItem[],
  itemsById: ReadonlyMap<string, ErgoListItem>,
): string {
  const counts = new Map<string, number>();
  for (const child of children) {
    const state = taskStatusWord(child);
    counts.set(state, (counts.get(state) ?? 0) + 1);
  }
  const epicState = derivedEpicState(children);
  const progress = ["ready", "draft", "in progress", "waiting", "blocked", "failed", "done", "canceled", "legacy error"]
    .flatMap((state) => {
      const count = counts.get(state);
      return count ? [`${count} ${state}`] : [];
    })
    .join(" · ");
  return `<details open data-epic-id="${attribute(epic.id)}" data-epic-search="${searchText([epic])}">
    <summary>
      <span class="epic-heading">
        <span class="id-control-group"><button class="item id" type="button" data-id="${attribute(epic.id)}">${text(epic.id)}</button>${copyIdButton(epic.id, "epic")}</span>
        <span class="epic-title">${epicState === "failed" ? '<span class="state" data-state="failed" title="failed">✗</span> ' : ""}${text(epic.title)}</span>
        ${progress ? `<span class="epic-progress">${progress}</span>` : ""}
      </span>
    </summary>
    <div class="children">${children.map((item) => renderTask(item, itemsById)).join("")}</div>
  </details>`;
}

function renderTask(item: ErgoListItem, itemsById: ReadonlyMap<string, ErgoListItem>): string {
  const state = taskStatusWord(item);
  const description = taskStatusDescription(item, itemsById);
  return `<div class="row" data-search="${searchText([item])}" data-ready="${item.ready === true}">
    <span class="id-control-group"><button class="item id" type="button" data-id="${attribute(item.id)}">${text(item.id)}</button>${copyIdButton(item.id, "task")}</span>
    <span class="state" data-state="${attribute(state)}" data-tooltip="${attribute(description)}" aria-label="${attribute(description)}"><span class="state-symbol" aria-hidden="true">${stateSymbol(state)}</span></span>
    <span class="task-title">${text(item.title)}</span>
  </div>`;
}

function stateSymbol(state: string): string {
  return (
    ({
      ready: "○",
      draft: "◌",
      waiting: "◷",
      "in progress": "↻",
      blocked: "!",
      failed: "✗",
      done: "✓",
      canceled: "–",
      "legacy error": "⚠",
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
