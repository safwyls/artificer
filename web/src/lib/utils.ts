import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Whether a host is only reachable from the same network — the RFC1918
 * ranges, loopback, link-local, .local names and IPv6 unique-local. Flamekeeper
 * usually reaches a server on one of these, which makes it the wrong thing
 * to hand to someone joining from outside, so the dashboard says so.
 */
export function isPrivateHost(host: string): boolean {
  const h = host.trim().toLowerCase().replace(/^\[|\]$/g, "");
  if (!h) return false;
  if (h === "localhost" || h.endsWith(".local") || h.endsWith(".lan") || h.endsWith(".home.arpa")) return true;
  if (h === "::1" || /^f[cd][0-9a-f]{2}:/.test(h)) return true;
  const v4 = h.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/);
  if (!v4) return false;
  const [a, b] = [Number(v4[1]), Number(v4[2])];
  return a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254);
}

/**
 * The address to hand players: the configured public one, else the
 * management host. A join address without a port gets the game port
 * appended, so "play.example.com" and "play.example.com:8211" both work.
 */
export function joinAddressFor(server: { host: string; gamePort: number; joinAddress?: string }): string {
  const custom = server.joinAddress?.trim();
  if (!custom) return `${server.host}:${server.gamePort}`;
  // A bare IPv6 literal is all colons; only "]:port" or a single colon
  // counts as a port already being there.
  const hasPort = custom.includes("]") ? /\]:\d+$/.test(custom) : /^[^:]+:\d+$/.test(custom);
  return hasPort ? custom : `${custom}:${server.gamePort}`;
}

/**
 * Copy text to the clipboard, working on plain-HTTP LAN deployments too:
 * navigator.clipboard only exists in secure contexts (HTTPS/localhost),
 * which is exactly where flamekeeper usually isn't. Falls back to the
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
