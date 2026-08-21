import { defineConfig, devices } from "@playwright/test";

/**
 * 03:QA-008 — the browser tier, decided in ADR-036.
 *
 * This suite is deliberately narrow. `src/*.test.tsx` already renders every
 * route under jsdom and scans it with 88 axe rules, and re-running that in three
 * engines would triple the bill for a second copy of the same answer. What is
 * here is only what jsdom structurally cannot decide: composite pixels, real
 * layout, and the real Tab key. The header of each spec says which one it is.
 *
 * The API is intercepted rather than served. Nothing in this tier needs
 * Postgres, SeaweedFS or a model key, which is what keeps it runnable on a
 * laptop and cheap in CI (see `task dev` — that stack is for the Run path, not
 * for the front end).
 *
 * The production build is what gets driven, not the dev server: CSS is minified
 * on the way out and the artefact that ships is the one worth asserting on.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://localhost:4173",
    trace: "on-first-retry",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
  ],
  webServer: {
    // VITE_API_BASE_URL empty makes the client call same-origin paths, which is
    // the production shape ("the SPA and the API share an origin" — see
    // vite.config.ts). It also removes CORS from this tier entirely: apiFetch
    // sends `credentials: "include"`, and a credentialed response may not carry
    // `Access-Control-Allow-Origin: *`, so the dev cross-origin setup would
    // have every stub rejected by the browser before any assertion ran.
    command: "npm run build && npm run preview -- --port 4173 --strictPort",
    env: { VITE_API_BASE_URL: "" },
    url: "http://localhost:4173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
