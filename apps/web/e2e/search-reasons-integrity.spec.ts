import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { SEARCH, SKILL, SKILL_B } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const MODEL_REASON = "模型判斷這個版本不符合任務需要的格式。";
const VALID_REASON = { reasons: [{ skill_id: SKILL, reason: MODEL_REASON, fits: false }] };
const CASES = [
  { name: "null item", reply: { reasons: [null] }, invalid: true },
  {
    name: "invalid verdict",
    reply: { reasons: [{ skill_id: SKILL, reason: "不應採用的模型說明", fits: "false" }] },
    invalid: true,
  },
  { name: "missing list", reply: { results: [] }, invalid: true },
  { name: "valid negative", reply: VALID_REASON, invalid: false },
];

for (const scenario of CASES) {
  for (const width of [375, 1280]) {
    test(`optional reasons ${scenario.name} retain search decisions at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      const requests: unknown[] = [];
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/api/skills/search/reasons", async (route) => {
        const attempt = requests.push(route.request().postDataJSON());
        if (attempt === 1) await held;
        await route.fulfill({ json: attempt === 1 ? scenario.reply : VALID_REASON });
      });
      const first = page.waitForResponse((reply) =>
        reply.url().endsWith("/api/skills/search/reasons"),
      );
      try {
        await page.goto(`/?${new URLSearchParams({ q: SEARCH.query })}`);
        const main = page.getByRole("main");
        const rows = main.locator(".search-results > li");
        await expect(rows).toHaveCount(2);
        await expect.poll(() => requests.length).toBe(1);
        expect(requests[0]).toEqual({ query: "pdf 摘要", skill_ids: [SKILL, SKILL_B] });
        await rows.first().getByRole("checkbox").check();
        const input = main.getByLabel("任務描述", { exact: true });
        await input.fill("尚未送出的搜尋草稿");
        const selectedURL = page.url();
        release();
        await (await first).finished();
        if (!scenario.invalid)
          await expect(rows.first().getByText(MODEL_REASON, { exact: false })).toBeVisible();
        await expect(rows).toHaveCount(2);
        await expect(input).toHaveValue("尚未送出的搜尋草稿");
        await expect(input).toBeFocused();
        await expect(rows.first().getByRole("checkbox")).toBeChecked();
        expect(page.url()).toBe(selectedURL);
        if (scenario.invalid) {
          await expect(rows.locator(".match-reason .badge-source-model")).toHaveCount(0);
          await page.locator('a[href="/policy"]').first().click();
          await expect(
            page.getByRole("heading", { name: "資料保存政策", exact: true }),
          ).toBeVisible();
          await page.goBack();
          await expect.poll(() => requests.length).toBe(2);
          expect(requests[1]).toEqual(requests[0]);
        }
        await expect(rows.first().locator(".match-reason")).toContainText(MODEL_REASON);
        await expect(rows.first().locator(".match-reason .badge-warning")).toHaveText("可能不合用");
        await expect(rows.last().locator(".match-reason")).toContainText("查詢與文件共同出現：pdf");
        await expect(rows.locator(".result-card-head > a")).toHaveText([
          "PDF Summariser",
          "Doc Splitter",
        ]);
        await expect(rows.first().getByRole("checkbox")).toBeChecked();
        await expect(main.getByRole("alert")).toHaveCount(0);
        expect(errors).toEqual([]);
        const report = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(info.outputPath("search-reasons-a11y.json"), JSON.stringify(report));
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({
          path: info.outputPath("search-reasons-current.png"),
          fullPage: false,
        });
      } finally {
        release();
      }
    });
  }
}
