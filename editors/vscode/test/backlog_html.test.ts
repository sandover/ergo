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
  ],
}));

test("renders a clickable searchable overview from realistic backlog data", () => {
  const view = renderBacklog(claimsLibraryBacklog, "vscode-resource:", "fixed-nonce");

  assert.match(view.html, /Add support-safe plugin diagnostics to server logs/);
  assert.match(view.html, /Create one Citation from text that spans pages/);
  assert.match(view.html, /1 ready · 1 waiting · 1 done/);
  assert.match(view.html, /data-id="OKOKSE"/);
  assert.match(view.html, /data-id="BQM4Y5">BQM4Y5<\/button>/);
  assert.doesNotMatch(view.html, /<button[^>]*>Create one Citation/);
  assert.match(view.html, /data-id="EUDZOS">EUDZOS<\/button>/);
  assert.match(view.html, /vscode\.postMessage\(\{ type: "open", id: button\.dataset\.id \}\)/);
  assert.match(view.html, /search\.addEventListener\("input"/);
  assert.match(view.html, /<label class="ready-filter"><input id="readyOnly" type="checkbox">Ready only<\/label>/);
  assert.match(view.html, /data-ready="true"/);
  assert.match(view.html, /data-ready="false"/);
  assert.match(view.html, /group\.tagName === "DETAILS"/);
  assert.match(view.html, /group\.hidden = query && !epicMatches && groupMatches === 0/);
  assert.match(view.html, /const onlyReady = readyOnly\.checked/);
  assert.match(view.html, /readyOnly\.addEventListener\("change", applyFilters\)/);
  assert.deepEqual([...view.itemIds], ["CCKOC2", "BQM4Y5", "OKOKSE", "RKSARF", "EUDZOS"]);
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
