import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies /api to the Go backend so `npm run dev` works against
// a locally running `go run ./cmd/reliquary` without CORS headaches. The
// proxy must also carry the SSE stream (/api/sync/events) — hence ws/
// buffering left at Vite's defaults, which pass event streams through.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  build: {
    outDir: "dist",
  },
});
