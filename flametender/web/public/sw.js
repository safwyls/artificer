// Flametender's service worker.
//
// It exists for two reasons, in this order:
//
//  1. Installability. Chrome dropped the service-worker requirement for
//     *menu* installation, but the install prompt — the omnibox install
//     icon and the `beforeinstallprompt` event this app listens for — is
//     still gated on a worker with a real fetch handler. An empty one
//     doesn't count, and shouldn't: it would be a lie told to a browser
//     check.
//  2. Opening at all when the network is bad. A phone on a weak signal
//     gets the shell from cache and honest "unreachable" states inside
//     it, rather than a blank browser error page.
//
// What it deliberately does NOT do is make this console work offline.
// Every number on screen is live state read from a game server through an
// agent; serving yesterday's player count from a cache would be worse
// than showing nothing. So the API is never cached — not once, not for a
// second — and only the shell and content-hashed assets are.
const VERSION = "v1";
const SHELL_CACHE = `flametender-shell-${VERSION}`;
const ASSET_CACHE = `flametender-assets-${VERSION}`;

// The shell is cached under this one key regardless of which route was
// requested: every client-side route is served the same index.html, so
// caching per-URL would store N copies of one document.
const SHELL_KEY = "/";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.add(SHELL_KEY))
      // A failed precache must not fail the install: the fetch handler
      // populates the cache on the first successful navigation anyway.
      .catch(() => {}),
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      // Drop caches from older versions; asset filenames are content
      // hashed, so nothing here is worth migrating.
      const names = await caches.keys();
      await Promise.all(
        names.filter((n) => n !== SHELL_CACHE && n !== ASSET_CACHE).map((n) => caches.delete(n)),
      );
      await self.clients.claim();
    })(),
  );
});

/** Only same-origin 200s that came from this server get stored. A
 * redirect is how Cloudflare Access answers an expired session; caching
 * one would pin a login bounce into the app. */
function isCacheable(response) {
  return response && response.ok && response.type === "basic" && !response.redirected;
}

const ASSET_PATTERN = /\.(?:js|css|png|jpg|jpeg|svg|webp|ico|woff2?)$/;

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  // Live state, and the endpoints that authenticate it. Both must reach
  // the network every time.
  if (url.pathname.startsWith("/api/")) return;
  if (url.pathname.startsWith("/cdn-cgi/")) return;

  if (request.mode === "navigate") {
    event.respondWith(shellNetworkFirst(request));
    return;
  }
  if (ASSET_PATTERN.test(url.pathname) || url.pathname === "/manifest.json") {
    event.respondWith(assetCacheFirst(request));
  }
});

/** Navigations go to the network first so a deploy is picked up on the
 * next load, and fall back to the cached shell only when the network
 * actually fails. */
async function shellNetworkFirst(request) {
  try {
    const response = await fetch(request);
    if (isCacheable(response)) {
      const cache = await caches.open(SHELL_CACHE);
      await cache.put(SHELL_KEY, response.clone());
    }
    return response;
  } catch (err) {
    const cached = await caches.match(SHELL_KEY);
    if (cached) return cached;
    throw err;
  }
}

/** Assets are content-hashed by the build, so a hit is always the right
 * file and a changed file is a different URL. Cache-first is safe and
 * makes a reinstall-free app open instantly. */
async function assetCacheFirst(request) {
  const cached = await caches.match(request);
  if (cached) return cached;
  const response = await fetch(request);
  if (isCacheable(response)) {
    const cache = await caches.open(ASSET_CACHE);
    await cache.put(request, response.clone());
  }
  return response;
}
