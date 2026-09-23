type ItemKind = "task" | "epic";

export const copyIdStyles = `
    .id-control-group { align-items: center; display: inline-flex; gap: 1px; min-width: 0; }
    .copy-id { align-items: center; background: transparent; border: 1px solid transparent; border-radius: 3px; color: var(--text-muted); cursor: pointer; display: inline-flex; flex: none; height: 20px; justify-content: center; opacity: 0; padding: 0; transition: background-color .12s ease, color .12s ease, opacity .12s ease; width: 20px; }
    .id-control-group:hover .copy-id, .id-control-group:focus-within .copy-id, .copy-id:focus-visible { opacity: 1; }
    .copy-id:hover { background: var(--surface-hover); color: var(--accent); }
    .copy-id:focus-visible { border-color: var(--accent); outline: 1px solid var(--accent); outline-offset: 1px; }
    .copy-id svg { display: block; fill: currentColor; height: 13px; width: 13px; }
    @media (prefers-reduced-motion: reduce) { .copy-id { transition: none; } }
`;

export function copyIdButton(id: string, kind: ItemKind): string {
  const label = `Copy ${kind} ID ${id}`;
  return `<button class="copy-id" type="button" data-copy-id="${attribute(id)}" title="${attribute(label)}" aria-label="${attribute(label)}"><svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path fill-rule="evenodd" clip-rule="evenodd" d="M5 5h7v7H5V5Zm1 1v5h5V6H6Z"></path><path d="M4 9H3V3h6v1H4v5Z"></path></svg></button>`;
}

export function isCopyIdMessage(value: unknown): value is { type: "copyId"; id: string } {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const message = value as Record<string, unknown>;
  return Object.keys(message).length === 2 &&
    message.type === "copyId" &&
    typeof message.id === "string" &&
    isErgoId(message.id);
}

export function isErgoId(value: string): boolean {
  return /^[A-Z0-9]{6}$/.test(value);
}

function attribute(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}
