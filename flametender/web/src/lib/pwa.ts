import { useEffect, useState } from "react";

/**
 * Installing Flametender as an app.
 *
 * Browsers decide *whether* a site may be installed; this decides whether
 * anyone finds out. Chrome fires `beforeinstallprompt` once the criteria
 * are met (manifest, HTTPS, a service worker with a real fetch handler —
 * see public/sw.js) and hides its own affordance behind a menu most
 * people never open. Catching the event lets the console offer the install
 * where someone is already looking.
 *
 * The event is fired once, early, and often *before* React mounts, so it
 * is captured at module load and replayed to whoever asks. Without that,
 * the button would simply never appear on a cold load.
 */

/** The non-standard event Chromium fires when a site becomes installable. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

let deferred: BeforeInstallPromptEvent | null = null;
const listeners = new Set<(available: boolean) => void>();

function announce() {
  for (const listener of listeners) listener(deferred !== null);
}

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (event) => {
    // Chrome's own mini-infobar is suppressed by this, which is the point:
    // the app shows the offer in its own chrome instead.
    event.preventDefault();
    deferred = event as BeforeInstallPromptEvent;
    announce();
  });
  window.addEventListener("appinstalled", () => {
    // The prompt is single-use, and an installed app has nothing to offer.
    deferred = null;
    announce();
  });
}

/** Reports whether the app is already running as an installed app, where
 * offering to install it again would be nonsense. */
function isStandalone(): boolean {
  if (typeof window === "undefined") return false;
  return (
    window.matchMedia?.("(display-mode: standalone)").matches ||
    // iOS Safari predates display-mode and reports it here instead.
    ("standalone" in window.navigator && Boolean((window.navigator as { standalone?: boolean }).standalone))
  );
}

export interface PwaInstall {
  /** True when the browser has offered an install and the app isn't
   * already installed. */
  available: boolean;
  /** Shows the browser's install dialog. Resolves once the person has
   * chosen; the offer disappears either way, since the event cannot be
   * replayed. */
  install: () => Promise<void>;
}

export function usePwaInstall(): PwaInstall {
  const [available, setAvailable] = useState(deferred !== null && !isStandalone());

  useEffect(() => {
    const listener = (nowAvailable: boolean) => setAvailable(nowAvailable && !isStandalone());
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, []);

  return {
    available,
    install: async () => {
      const event = deferred;
      if (!event) return;
      await event.prompt();
      await event.userChoice;
      deferred = null;
      announce();
    },
  };
}

/**
 * Registers the service worker, which is what makes the browser consider
 * this installable at all.
 *
 * Production only: under `vite dev` a worker would serve cached assets
 * over the dev server's own hot updates, which is a confusing way to
 * spend an afternoon.
 */
export function registerServiceWorker() {
  if (!import.meta.env.PROD) return;
  if (!("serviceWorker" in navigator)) return;
  // After load: registration competes with the app's first data fetches
  // otherwise, and nothing here is needed for the first paint.
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch((err) => {
      // Not fatal — the console works fine unregistered, it just can't be
      // installed. Worth a line in the console for whoever wonders why.
      console.warn("service worker registration failed", err);
    });
  });
}
