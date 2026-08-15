import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// No `server.proxy` here on purpose. The obvious dev setup — proxy a list of
// path prefixes to :8080 — cannot work with these routes: the API owns
// /skills/{id}/diff, /skills/{id}/fork and friends, while this app's own router
// owns the page URL /skills/$skillId. Proxying /skills breaks every deep link
// in the browser; not proxying it breaks fetch(). So the API allows this origin
// instead, for development only, via DEV_CORS_ORIGIN (see
// services/platform/internal/platform/httpx/cors.go). In production the SPA and
// the API share an origin and neither mechanism is involved.

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
  },
});
