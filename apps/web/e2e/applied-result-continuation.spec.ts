import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const NEW_VERSION = "99999999-1111-4111-8111-111111111111";

for (const hasTestCase of [true, false]) {
  for (const leavesForm of [false, true]) {
    for (const width of [375, 1280]) {
      test(`an applied ${hasTestCase ? "rerun" : "version"} continuation ${leavesForm ? "preserves header focus" : "reveals the next action"} at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.route(`**/runs/${RUN}`, (route) => {
          if (route.request().isNavigationRequest()) return route.fallback();
          return route.fulfill({
            json: {
              ...(platformResponse(route.request().url()).body as object),
              ...(!hasTestCase ? { test_case_id: undefined } : {}),
            },
          });
        });
        let release!: () => void;
        let received!: () => void;
        const held = new Promise<void>((resolve) => {
          release = resolve;
        });
        const started = new Promise<void>((resolve) => {
          received = resolve;
        });
        const writes: unknown[] = [];
        await page.route(`**/skills/${SKILL}/versions/from-suggestions`, async (route) => {
          writes.push(route.request().postDataJSON());
          received();
          await held;
          return route.fulfill({
            status: 201,
            json: {
              skill_id: SKILL,
              version_id: NEW_VERSION,
              version_number: 3,
              content_hash: "sha256:cccc",
              duplicate: false,
              applied_suggestion_ids: ["s1"],
              rejected_suggestions: [],
            },
          });
        });
        try {
          await page.goto(`/runs/${RUN}`);
          const apply = page.getByRole("button", {
            name: "以已接受的 1 項建議建立新版本",
            exact: true,
          });
          await expect(apply).toBeEnabled();
          await apply.evaluate((element) => {
            window.scrollBy(0, element.getBoundingClientRect().bottom - innerHeight + 8);
            element.focus();
          });
          await expect(apply).toBeInViewport({ ratio: 1 });
          await apply.click();
          await started;
          const search = page.getByRole("banner").getByRole("searchbox");
          if (leavesForm) await search.fill("保留目前搜尋草稿");
          release();
          const continuation = page.getByRole("link", {
            name: hasTestCase ? "以新版本重跑這個測試題" : "開啟剛建立的版本",
            exact: true,
          });
          await expect(apply).toBeEnabled();
          if (leavesForm) {
            await expect(search).toBeFocused();
            await expect(search).toHaveValue("保留目前搜尋草稿");
          } else {
            await expect(continuation).toBeFocused();
            await expect(continuation).toBeInViewport({ ratio: 1 });
          }
          expect(writes).toEqual([{ evaluation_id: "eval-2", suggestion_ids: ["s1"] }]);
          const axe = await new AxeBuilder({ page })
            .include('section[aria-labelledby="suggestions-heading"]')
            .analyze();
          await writeFile(info.outputPath("applied-continuation-a11y.json"), JSON.stringify(axe));
          expect(axe.violations).toEqual([]);
          expect(axe.incomplete).toEqual([]);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          await page.screenshot({ path: info.outputPath("applied-continuation.png") });
        } finally {
          release();
        }
      });
    }
  }
}
