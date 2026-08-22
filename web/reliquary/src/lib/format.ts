/** Byte sizes as the old page rendered them: one decimal at GB/MB, whole
 * KB below, and never "0 KB" for a bundle that does exist. */
export function fmtBytes(b: number): string {
  if (b >= 1 << 30) return `${(b / (1 << 30)).toFixed(1)} GB`;
  if (b >= 1 << 20) return `${(b / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(b / 1024))} KB`;
}

/** Timestamps in the reader's own locale and zone: custody is a question
 * about *now*, and an ISO string in UTC makes everyone do the arithmetic. */
export function fmtTime(t: string | undefined): string {
  if (!t) return "";
  const d = new Date(t);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString();
}

/** Thousands separators for the catalogue's counts. */
export const fmtCount = (n: number) => n.toLocaleString();

/**
 * Copy text to the clipboard, working on plain-HTTP LAN deployments too:
 * navigator.clipboard only exists in secure contexts (HTTPS/localhost),
 * and reliquary is often reached over a LAN address. Falls back to the
 * deprecated-but-universal execCommand path.
 */
export async function copyText(text: string): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // fall through to the legacy path
    }
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    ta.remove();
  }
}
