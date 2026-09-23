import { randomBytes } from "node:crypto";
import { copyIdButton, copyIdStyles } from "./copy_id";
import { PreviewKind } from "./preview";

type MarkdownRenderer = {
  render(source: string): string;
  validateLink(url: string): boolean;
};

type MarkdownConstructor = new (options: {
  breaks: boolean;
  html: boolean;
  linkify: boolean;
  typographer: boolean;
}) => MarkdownRenderer;

// markdown-it ships as JavaScript. Keeping this small type at the boundary avoids
// adding a second package solely for declarations.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const MarkdownIt = require("markdown-it") as MarkdownConstructor;

const markdown = new MarkdownIt({
  breaks: false,
  html: false,
  linkify: true,
  typographer: true,
});
const validatesWebLink = markdown.validateLink.bind(markdown);
markdown.validateLink = (url: string): boolean =>
  validatesWebLink(url) || url.startsWith("file:///");

export function renderPreview(
  source: string,
  title: string,
  id: string,
  kind: PreviewKind,
  cspSource: string,
  nonce = randomBytes(16).toString("base64"),
): string {
  const idLabel = kind === "epic" ? "Epic ID" : "Task ID";
  const document = previewDocument(source);
  const parentLink = kind === "task" && document.parent
    ? `<button class="epic-link" type="button" data-open-epic="${attribute(document.parent)}" title="Open epic ${attribute(document.parent)}" aria-label="Open parent epic ${attribute(document.parent)}"><span class="epic-link-label">Part of epic</span><strong>${text(document.parent)}</strong><svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M5.5 3.5h7v7h-1v-5.3l-7.15 7.15-.7-.7L10.8 4.5H5.5v-1Z"></path></svg></button>`
    : "";
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource} 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
  <title>${text(title)}</title>
  <style nonce="${nonce}">
    :root {
      color-scheme: dark;
      --surface: #0f181e;
      --surface-raised: #161e25;
      --surface-soft: #182228;
      --surface-hover: #1d292f;
      --border: #242f35;
      --text: #8f9598;
      --text-strong: #9da4a7;
      --text-muted: #656b70;
      --accent: #8298a0;
      --accent-soft: #8098a1;
    }
    * { box-sizing: border-box; }
    html { background: var(--surface); }
    body {
      background: var(--surface);
      color: var(--text);
      font-family: var(--vscode-font-family, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif);
      font-size: 15px;
      line-height: 1.6;
      margin: 0 auto;
      max-width: 780px;
      padding: 36px 40px 80px;
    }
    h1, h2, h3, h4, h5, h6 { color: var(--text-strong); line-height: 1.28; margin: 1.65em 0 .65em; }
    h1 { color: #98aab2; font-size: 1.8em; margin-top: 0; }
    h2 { color: #a79d87; font-size: 1.42em; }
    h3 { color: #adb0ab; font-size: 1.18em; }
    h4 { color: #adb0ab; font-size: 1.08em; }
    h5, h6 { font-size: 1em; }
    body.kind-epic h3 { border-top: 1px solid #344149; margin-top: 2.7em; padding-top: 1.25em; }
    p, ul, ol, blockquote, pre, table { margin: 0 0 1em; }
    li + li { margin-top: .18em; }
    li::marker { color: var(--text-muted); }
    strong { color: var(--text-strong); font-weight: 650; }
    em { color: #8f9598; }
    a { color: var(--accent-soft); text-decoration-thickness: 1px; text-underline-offset: .15em; }
    a:hover { color: #b0b8b9; }
    blockquote { border-left: 2px solid var(--accent); color: var(--text); margin-left: 0; padding: .1em 1em; }
    code, pre { font-family: var(--vscode-editor-font-family, "SFMono-Regular", Consolas, monospace); }
    :not(pre) > code { background: var(--surface-soft); border: 1px solid var(--border); border-radius: 3px; color: #969f92; font-size: .9em; padding: .12em .32em; }
    pre { background: var(--surface-raised); border: 1px solid var(--border); border-radius: 5px; color: var(--text); overflow-x: auto; padding: 14px 16px; }
    pre code { color: inherit; }
    hr { border: 0; border-top: 1px solid var(--border); margin: 2em 0; }
    table { border-collapse: collapse; width: 100%; }
    th, td { border: 1px solid var(--border); padding: 7px 10px; text-align: left; }
    th { background: var(--surface-raised); color: var(--text-strong); }
    tr:nth-child(even) { background: color-mix(in srgb, var(--text) 3%, transparent); }
    .document-tools { align-items: center; display: flex; gap: 12px; justify-content: flex-end; margin: 0 0 14px; min-height: 24px; }
    .document-tools.has-parent { justify-content: space-between; }
    .document-id { color: var(--text-muted); font-family: var(--vscode-editor-font-family, "SFMono-Regular", Consolas, monospace); font-size: 12px; }
    .document-tools .copy-id { opacity: .72; }
    .epic-link { align-items: center; background: transparent; border: 1px solid var(--border); border-radius: 999px; color: var(--text-muted); cursor: pointer; display: inline-flex; font: inherit; font-size: 12px; gap: 6px; min-height: 26px; padding: 2px 9px 2px 10px; transition: background-color .12s ease, border-color .12s ease, color .12s ease; }
    .epic-link strong { color: var(--accent-soft); font-family: var(--vscode-editor-font-family, "SFMono-Regular", Consolas, monospace); font-size: 11px; font-weight: 500; }
    .epic-link svg { fill: currentColor; height: 13px; width: 13px; }
    .epic-link:hover { background: var(--surface-hover); border-color: #344149; color: var(--text-strong); }
    .epic-link:focus-visible { border-color: var(--accent); outline: 1px solid var(--accent); outline-offset: 1px; }
    ${copyIdStyles}
    @media (prefers-reduced-motion: reduce) { .epic-link { transition: none; } }
    @media (max-width: 600px) {
      body { padding: 24px 20px 56px; }
      .document-tools.has-parent { align-items: flex-start; flex-direction: column; }
    }
  </style>
</head>
<body class="kind-${kind}">
  <div class="document-tools${parentLink ? " has-parent" : ""}"><span class="id-control-group"><span class="document-id">${idLabel} ${text(id)}</span>${copyIdButton(id, kind)}</span>${parentLink}</div>
  ${markdown.render(document.body)}
  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi();
    const saved = vscode.getState() || {};
    let scrollFrame;
    window.addEventListener("scroll", () => {
      if (scrollFrame) cancelAnimationFrame(scrollFrame);
      scrollFrame = requestAnimationFrame(() => vscode.setState({ scrollY: window.scrollY }));
    }, { passive: true });
    requestAnimationFrame(() => {
      if (typeof saved.scrollY === "number") window.scrollTo(0, saved.scrollY);
    });
    document.addEventListener("click", (event) => {
      const copyButton = event.target.closest("button[data-copy-id]");
      if (copyButton) {
        event.preventDefault();
        event.stopPropagation();
        vscode.postMessage({ type: "copyId", id: copyButton.dataset.copyId });
        return;
      }
      const epicLink = event.target.closest("button[data-open-epic]");
      if (epicLink) {
        vscode.postMessage({ type: "openEpic", id: epicLink.dataset.openEpic });
        return;
      }
      const link = event.target.closest('a[href^="file:///"]');
      if (!link) return;
      event.preventDefault();
      vscode.postMessage({ type: "openFile", uri: link.getAttribute("href") });
    });
  </script>
</body>
</html>`;
}

export function renderPreviewNotice(
  message: string,
  cspSource: string,
  nonce = randomBytes(16).toString("base64"),
): string {
  return `<!doctype html>
<html lang="en"><head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${cspSource} 'nonce-${nonce}';">
  <style nonce="${nonce}">
    :root { color-scheme: dark; }
    body { background: #0f181e; color: #8f9598; font: 15px/1.6 var(--vscode-font-family, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif); margin: 0 auto; max-width: 780px; padding: 36px 40px; }
  </style>
</head><body><p>${text(message)}</p></body></html>`;
}

function previewDocument(source: string): { body: string; parent?: string } {
  if (!source.startsWith("---\n") && !source.startsWith("---\r\n")) {
    return { body: source };
  }
  const match = /^---\r?\n[\s\S]*?\r?\n---\r?\n/.exec(source);
  if (!match) {
    return { body: source };
  }
  const parent = /^parent:\s*["']?([A-Z0-9]{6})["']?\s*$/m.exec(match[0])?.[1];
  return { body: source.slice(match[0].length), parent };
}

function text(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function attribute(value: string): string {
  return text(value).replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}
