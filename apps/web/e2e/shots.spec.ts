import { test, type Page } from "@playwright/test";
import { RETENTION, SEARCH } from "./fixtures";

/**
 * Not a test — a way to look at the app. `npx playwright test e2e/shots.spec.ts`
 * with SHOT_DIR set writes full-page PNGs of the screens this tier has fixtures
 * for, at desktop and phone widths in both themes.
 *
 * It exists because the UI work before ADR-036 was done without ever seeing the
 * result: the filter bar's ragged wrapping, a search box too narrow for its own
 * placeholder, and a `<caption>` sitting centred in a left-aligned document were
 * all invisible to 138 jsdom tests and to every reading of the CSS. They took
 * one screenshot each.
 *
 * Skipped unless SHOT_DIR is set, so CI neither runs it nor needs somewhere to
 * put the output.
 */
const OUT = process.env.SHOT_DIR;

test.skip(!OUT, "set SHOT_DIR to write screenshots");

async function stub(page: Page) {
  await page.route("**/*", (route, request) => {
    const kind = request.resourceType();
    if (kind === "document" || kind === "stylesheet" || kind === "script" || kind === "font") {
      return route.continue();
    }
    return route.fulfill({ status: 401, json: { error: "unstubbed" } });
  });
  await page.route("**/me", (r) => r.fulfill({ status: 401, json: { error: "anonymous" } }));
  await page.route("**/api/skills/search**", (r) => r.fulfill({ json: SEARCH }));
  await page.route("**/policy/data-retention", (r) => r.fulfill({ json: RETENTION }));
}

const SHOTS: [name: string, route: string, width: number, dark: boolean][] = [
  ["home-desktop-light", "/?q=pdf", 1280, false],
  ["home-desktop-dark", "/?q=pdf", 1280, true],
  ["home-empty-light", "/", 1280, false],
  ["home-mobile-light", "/?q=pdf", 375, false],
  ["policy-desktop-light", "/policy", 1280, false],
  ["policy-mobile-light", "/policy", 375, false],
];

for (const [name, route, width, dark] of SHOTS) {
  test(`shot ${name}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
    await page.setViewportSize({ width, height: 900 });
    await stub(page);
    await page.goto(route);
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
  });
}
