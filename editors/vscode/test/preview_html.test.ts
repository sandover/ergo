import assert from "node:assert/strict";
import test from "node:test";
import { renderPreview, renderPreviewNotice } from "../src/preview_html";

test("renders task Markdown with the scoped Solar Eclipse Ashen palette", () => {
  const html = renderPreview(
    "# Gentle heading\n\nBody with **weight** and `code`.",
    "Ergo task ABC123",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.match(html, /<h1>Gentle heading<\/h1>/);
  assert.match(html, /<strong>weight<\/strong>/);
  assert.match(html, /<code>code<\/code>/);
  assert.match(html, /--surface: #0f181e/);
  assert.match(html, /--text: #8f9598/);
  assert.match(html, /--accent: #8298a0/);
  assert.match(html, /nonce="fixed-nonce"/);
  assert.match(html, /const saved = vscode\.getState\(\) \|\| \{\}/);
  assert.match(html, /window\.scrollTo\(0, saved\.scrollY\)/);
});

test("does not allow task Markdown to inject HTML", () => {
  const html = renderPreview(
    "<script>alert('no')</script>",
    "<unsafe>",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;alert/);
  assert.match(html, /<title>&lt;unsafe&gt;<\/title>/);
});

test("keeps result-file links clickable through the guarded extension handler", () => {
  const html = renderPreview(
    "[report](file:///repo/docs/report.md)",
    "Ergo task ABC123",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.match(html, /<a href="file:\/\/\/repo\/docs\/report\.md">report<\/a>/);
  assert.match(html, /vscode\.postMessage\(\{ type: "openFile", uri: link\.getAttribute\("href"\) \}\)/);
});

test("renders an accessible copy control near the detail heading", () => {
  const html = renderPreview(
    "# Epic title",
    "Ergo epic EPIC01",
    "EPIC01",
    "epic",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.match(html, /<div class="document-tools"><span class="id-control-group"><span class="document-id">Epic ID EPIC01<\/span><button class="copy-id" type="button" data-copy-id="EPIC01" title="Copy epic ID EPIC01" aria-label="Copy epic ID EPIC01"/);
  assert.match(html, /vscode\.postMessage\(\{ type: "copyId", id: copyButton\.dataset\.copyId \}\)/);
  assert.match(html, /<body class="kind-epic">/);
  assert.match(html, /body\.kind-epic h3 \{ border-top: 1px solid #344149; margin-top: 2\.7em; padding-top: 1\.25em; \}/);
  assert.doesNotMatch(html, /h1, h2 \{ border-bottom:/);
});

test("omits CLI front matter from the readable detail", () => {
  const html = renderPreview(
    '---\nid: "ABC123"\ntitle: "Internal metadata"\n---\n\n# Human title\n\nUseful detail.',
    "Ergo task ABC123",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.match(html, /<h1>Human title<\/h1>/);
  assert.match(html, /<p>Useful detail\.<\/p>/);
  assert.doesNotMatch(html, /Internal metadata/);
});

test("links a task to its parent epic without exposing CLI metadata", () => {
  const html = renderPreview(
    '---\nid: "ABC123"\nparent: "EPIC01"\n---\n\n# Child task',
    "Ergo task ABC123",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.match(html, /<div class="document-tools has-parent">/);
  assert.match(html, /class="epic-link"[^>]+data-open-epic="EPIC01"/);
  assert.match(html, /<span class="epic-link-label">Part of epic<\/span><strong>EPIC01<\/strong>/);
  assert.match(html, /vscode\.postMessage\(\{ type: "openEpic", id: epicLink\.dataset\.openEpic \}\)/);
  assert.doesNotMatch(html, /parent:|id: &quot;ABC123&quot;/);
});

test("does not invent an epic link for a root task", () => {
  const html = renderPreview(
    '---\nid: "ABC123"\n---\n\n# Root task',
    "Ergo task ABC123",
    "ABC123",
    "task",
    "vscode-resource:",
    "fixed-nonce",
  );

  assert.doesNotMatch(html, /class="epic-link"/);
  assert.doesNotMatch(html, /<span class="epic-link-label">Part of epic<\/span>/);
});

test("renders a calm escaped notice instead of a blank detail panel", () => {
  const html = renderPreviewNotice("Could not load <task>.", "vscode-resource:", "fixed-nonce");

  assert.match(html, /background: #0f181e/);
  assert.match(html, /Could not load &lt;task&gt;\./);
  assert.match(html, /nonce="fixed-nonce"/);
});
