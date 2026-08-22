import { test } from "@playwright/test";
import { OTHER_RUN, RUN, SKILL, SKILL_B, TEST_CASE, VERSION } from "../src/fixtures/platform";
import { stubPlatform } from "./stub";

/**
 * Not a test — a way to look at the app.
 *
 * `SHOT_DIR=… npx playwright test e2e/shots.spec.ts --project=chromium` writes a
 * full-page PNG of every route the router declares, at desktop and phone widths
 * and in both themes.
 *
 * It exists because the UI work before ADR-036 was done without ever seeing the
 * result. A search box too narrow for its own placeholder, a filter bar wrapping
 * into 340px of void, a `<caption>` centred in a left-aligned document and every
 * list in the app sinking below its neighbours in a table cell all survived 138
 * jsdom tests and several careful readings of the CSS. They lasted one look each.
 *
 * Deliberately not `toHaveScreenshot`. Baselines are sensitive to font and
 * scrollbar differences, and CI now runs two operating systems and three
 * engines — that is six sets of images to keep true, for a kind of change that a
 * person has to judge anyway. These are for reading, not for asserting.
 *
 * Skipped unless SHOT_DIR is set, so CI neither runs it nor needs anywhere to
 * put the output.
 */
const OUT = process.env.SHOT_DIR;

test.skip(!OUT, "set SHOT_DIR to write screenshots");

/** Every route in router.tsx, with the ids the shared fixtures answer to. */
const ROUTES: [name: string, url: string][] = [
  ["home-empty", "/"],
  ["home-results", "/?q=pdf+%E6%91%98%E8%A6%81"],
  ["compare", `/compare?ids=${SKILL},${SKILL_B}`],
  ["policy", "/policy"],
  ["skill-detail", `/skills/${SKILL}`],
  ["skill-files", `/skills/${SKILL}/files`],
  ["packaging", `/skills/${SKILL}/package?version=${VERSION}`],
  ["lab-run", `/lab/run?skill=${SKILL}&version=${VERSION}&test_case=${TEST_CASE}`],
  ["lab-datasets", `/lab/datasets?test_case=${TEST_CASE}`],
  ["lab-test-cases", "/lab/test-cases"],
  ["lab-test-case-detail", `/lab/test-cases/${TEST_CASE}`],
  ["run-trace", `/runs/${RUN}`],
  ["run-compare", `/runs/${RUN}/compare?against=${OTHER_RUN}`],
  ["workspace-account", "/workspace/account"],
  ["workspace-downloads", "/workspace/downloads"],
  ["workspace-import", "/workspace/import"],
  ["workspace-runs", "/workspace/runs"],
  ["workspace-skills", "/workspace/skills"],
];

const VIEWS: [suffix: string, width: number, dark: boolean][] = [
  ["desktop-light", 1280, false],
  ["desktop-dark", 1280, true],
  ["mobile-light", 375, false],
];

for (const [name, url] of ROUTES) {
  for (const [suffix, width, dark] of VIEWS) {
    test(`shot ${name} ${suffix}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
      await page.setViewportSize({ width, height: 900 });
      await stubPlatform(page);
      await page.goto(url);
      // Long enough for the queries this page fans out to settle; these are all
      // answered from memory, so there is nothing slower than a microtask here.
      await page.waitForTimeout(500);
      await page.screenshot({ path: `${OUT}/${name}--${suffix}.png`, fullPage: true });
    });
  }
}
