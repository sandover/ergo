import assert from "node:assert/strict";
import test from "node:test";
import { renderBacklog } from "../src/backlog_html";
import { parseListDocument } from "../src/listing";

const claimsLibraryBacklog = parseListDocument(JSON.stringify({
  version: 1,
  items: [
    {
      id: "CCKOC2",
      title: "Add support-safe plugin diagnostics to server logs",
      kind: "task",
      state: "todo",
      ready: true,
    },
    {
      id: "BQM4Y5",
      title: "Create one Citation from text that spans pages",
      kind: "epic",
    },
    {
      id: "OKOKSE",
      title: "Decide what each page highlight should show",
      kind: "task",
      state: "todo",
      ready: true,
      epic_id: "BQM4Y5",
    },
    {
      id: "RKSARF",
      title: "Define and store multi-page Citation locations",
      kind: "task",
      state: "todo",
      ready: false,
      epic_id: "BQM4Y5",
    },
    {
      id: "EUDZOS",
      title: "Capture multi-page text selections in Acrobat",
      kind: "task",
      state: "done",
      ready: false,
      epic_id: "BQM4Y5",
    },
	{
	  id: "FAIL01",
	  title: "Verify the Windows package",
	  kind: "task",
	  state: "failed",
	  ready: false,
	  epic_id: "BQM4Y5",
	},
  ],
}));

test("renders a clickable searchable overview from realistic backlog data", () => {
  const view = renderBacklog(claimsLibraryBacklog, "vscode-resource:", "fixed-nonce");

  assert.match(view.html, /Add support-safe plugin diagnostics to server logs/);
  assert.match(view.html, /Create one Citation from text that spans pages/);
	assert.match(view.html, /1 ready · 1 waiting · 1 failed · 1 done/);
	assert.match(view.html, /<span class="id-control-group"><button class="item id" type="button" data-id="CCKOC2">CCKOC2<\/button><button class="copy-id" type="button" data-copy-id="CCKOC2" title="Copy task ID CCKOC2" aria-label="Copy task ID CCKOC2"/);
	assert.match(view.html, /<span class="id-control-group"><button class="item id" type="button" data-id="BQM4Y5">BQM4Y5<\/button><button class="copy-id" type="button" data-copy-id="BQM4Y5" title="Copy epic ID BQM4Y5" aria-label="Copy epic ID BQM4Y5"/);
  assert.match(view.html, /data-id="OKOKSE"/);
  assert.match(view.html, /data-id="BQM4Y5">BQM4Y5<\/button>/);
  assert.doesNotMatch(view.html, /<button[^>]*>Create one Citation/);
  assert.match(view.html, /data-id="EUDZOS">EUDZOS<\/button>/);
  assert.match(view.html, /vscode\.postMessage\(\{ type: "open", id: button\.dataset\.id \}\)/);
  assert.match(view.html, /vscode\.postMessage\(\{ type: "copyId", id: copyButton\.dataset\.copyId \}\)/);
  assert.match(view.html, /\.id-control-group:hover \.copy-id, \.id-control-group:focus-within \.copy-id, \.copy-id:focus-visible \{ opacity: 1; \}/);
  assert.match(view.html, /search\.addEventListener\("input"/);
  assert.match(view.html, /<label class="ready-filter"><input id="readyOnly" type="checkbox">Ready only<\/label>/);
  assert.match(view.html, /data-ready="true"/);
  assert.match(view.html, /data-ready="false"/);
  assert.match(view.html, /group\.tagName === "DETAILS"/);
  assert.match(view.html, /group\.hidden = query && !epicMatches && groupMatches === 0/);
  assert.match(view.html, /const onlyReady = readyOnly\.checked/);
	assert.match(view.html, /readyOnly\.addEventListener\("change", \(\) => \{ applyFilters\(\); saveState\(\); \}\)/);
	assert.match(view.html, /const saved = vscode\.getState\(\) \|\| \{\}/);
	assert.match(view.html, /data-epic-id="BQM4Y5"/);
	assert.match(view.html, /window\.scrollTo\(0, saved\.scrollY\)/);
	assert.match(view.html, /--surface: #0f181e/);
	assert.match(view.html, /--text: #8f9598/);
	assert.match(view.html, /--accent: #8298a0/);
	assert.deepEqual([...view.itemIds], ["CCKOC2", "BQM4Y5", "OKOKSE", "RKSARF", "EUDZOS", "FAIL01"]);
});

test("escapes task content before inserting it into HTML", () => {
  const hostile = parseListDocument(JSON.stringify({
    version: 1,
    items: [{
      id: "SAFE01",
      title: "<img src=x onerror=\"alert(1)\">",
      kind: "task",
      state: "todo",
      ready: true,
    }],
  }));

  const view = renderBacklog(hostile, "vscode-resource:", "fixed-nonce");
  assert.doesNotMatch(view.html, /<img src=x/);
  assert.match(view.html, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("marks a finished epic failed when any child failed", () => {
	const document = parseListDocument(JSON.stringify({
	  version: 1,
	  items: [
		{ id: "EPIC01", title: "Ship release", kind: "epic" },
		{ id: "DONE01", title: "Build", kind: "task", state: "done", ready: false, epic_id: "EPIC01" },
		{ id: "FAIL01", title: "Verify", kind: "task", state: "failed", ready: false, epic_id: "EPIC01" },
	  ],
	}));
	const view = renderBacklog(document, "vscode-resource:", "fixed-nonce");
	assert.match(view.html, /data-state="failed" title="failed">✗<\/span> Ship release/);
});

test("renders draft state in the backlog and epic progress", () => {
  const document = parseListDocument(JSON.stringify({
    version: 1,
    items: [
      { id: "EPIC01", title: "Stage release", kind: "epic" },
      { id: "DRAFT01", title: "Configure the release", kind: "task", state: "draft", ready: false, epic_id: "EPIC01" },
    ],
  }));
  const view = renderBacklog(document, "vscode-resource:", "fixed-nonce");
  assert.match(view.html, /1 draft/);
  assert.match(view.html, /grid-template-columns: auto minmax\(0, 1fr\) auto/);
  assert.match(view.html, /<span class="epic-title">Stage release<\/span>\s*<span class="epic-progress">1 draft<\/span>/);
  assert.match(view.html, /data-state="draft" data-tooltip="draft" aria-label="draft"><span class="state-symbol" aria-hidden="true">◌<\/span><\/span>/);
  assert.match(view.html, /data-ready="false"/);
});

test("keeps compact task states accessible without visible text labels", () => {
  const document = parseListDocument(JSON.stringify({
    version: 1,
    items: [
      { id: "READY01", title: "Ready", kind: "task", state: "todo", ready: true },
      { id: "WAIT01", title: "Waiting", kind: "task", state: "todo", ready: false },
      { id: "DRAFT01", title: "Draft", kind: "task", state: "draft", ready: false },
      { id: "DOING01", title: "Doing", kind: "task", state: "doing", ready: false },
      { id: "BLOCK01", title: "Blocked", kind: "task", state: "blocked", ready: false },
      { id: "FAIL01", title: "Failed", kind: "task", state: "failed", ready: false },
      { id: "DONE01", title: "Done", kind: "task", state: "done", ready: false },
      { id: "CANCEL01", title: "Canceled", kind: "task", state: "canceled", ready: false },
      { id: "ERROR01", title: "Legacy error", kind: "task", state: "error", ready: false },
    ],
  }));
  const view = renderBacklog(document, "vscode-resource:", "fixed-nonce");

  for (const status of ["ready", "waiting", "draft", "in progress", "blocked", "failed", "done", "canceled", "legacy error"]) {
    assert.match(view.html, new RegExp(`aria-label="${status.replaceAll(" ", "\\s+")}"`));
  }
  assert.match(view.html, /grid-template-columns: auto auto minmax\(0, 1fr\)/);
  assert.doesNotMatch(view.html, /state-label/);
  assert.match(view.html, /\.state \{[^}]*cursor: default;/);
  assert.match(view.html, /\.state\[data-tooltip\]:hover::after \{ opacity: 1;/);
});

test("highlights search matches in visible IDs and titles", () => {
  const view = renderBacklog(parseListDocument(JSON.stringify({
    version: 1,
    items: [{ id: "SEARCH", title: "Find this task", kind: "task", state: "todo", ready: true }],
  })), "vscode-resource:", "fixed-nonce");

  assert.match(view.html, /mark\.search-hit/);
  assert.match(view.html, /querySelectorAll\("\.item\.id, \.epic-title, \.task-title"\)/);
  assert.match(view.html, /highlightSearchHits\(query\)/);
});
