import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The companion's own API is served by the companion binary on loopback
// (main.go's -listen default). `npm run dev` proxies to a running
// `go run ./cmd/companion`; the shipped app never uses a dev server —
// the single-binary, no-installer shape means this bundle is embedded.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8377",
    },
  },
  build: {
    outDir: "dist",
  },
});
