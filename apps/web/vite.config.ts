import { execSync } from "node:child_process";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

/**
 * 資訊架構 IA-11: the page states which build it is (footer, folded). CI has
 * the commit in GITHUB_SHA; a local build asks git and says so, because a
 * local build is not the thing anyone will be asked to reproduce. No git at
 * all is a typed absence, not an empty string (設計 §2.9).
 */
function buildId(): string {
  if (process.env.GITHUB_SHA) return process.env.GITHUB_SHA.slice(0, 12);
  try {
    const sha = execSync("git rev-parse --short=12 HEAD", { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
    return `${sha}（本機建置，未經 CI）`;
  } catch {
    return "未知（建置環境沒有 git 也沒有 GITHUB_SHA）";
  }
}

// No `server.proxy` here on purpose. The obvious dev setup — proxy a list of
// path prefixes to :8080 — cannot work with these routes: the API owns
// /skills/{id}/diff, /skills/{id}/fork and friends, while this app's own router
// owns the page URL /skills/$skillId. Proxying /skills breaks every deep link
// in the browser; not proxying it breaks fetch(). So the API allows this origin
// instead, for development only, via DEV_CORS_ORIGIN (see
// apps/platform/internal/platform/httpx/cors.go). In production the SPA and
// the API share an origin and neither mechanism is involved.

export default defineConfig({
  plugins: [react()],
  define: { "import.meta.env.VITE_BUILD_ID": JSON.stringify(buildId()) },
  test: {
    environment: "jsdom",
    // The e2e directory belongs to Playwright (ADR-036). Vitest's default glob
    // reaches it, and a Playwright spec collected by the wrong runner fails at
    // import with "did not expect test.describe() to be called here" — a red
    // suite whose test count is still green, which reads as flake.
    exclude: ["**/node_modules/**", "**/dist/**", "e2e/**"],
  },
});
