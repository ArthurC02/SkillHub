import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { SKILL, VERSION, TEST_CASE, PREFLIGHT } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const owner of ["version", "test case"] as const) {
  for (const width of [375, 1280]) {
    test(`a summary for another ${owner} requires a matching reread before confirmation at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      let reads = 0;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() === "POST") writes.push(request.url());
      });
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route(`**/skills/${SKILL}/runs/preflight?*`, async (route) => {
        const url = new URL(route.request().url());
        expect(url.searchParams.get("version_id")).toBe(VERSION);
        expect(url.searchParams.get("test_case_id")).toBe(TEST_CASE);
        reads += 1;
        if (reads === 1)
          return route.fulfill({
            json: {
              ...PREFLIGHT,
              summary: {
                ...PREFLIGHT.summary,
                ...(owner === "version"
                  ? { skill_version_id: "55555555-5555-4555-8555-555555555555" }
                  : { test_case_id: "66666666-6666-4666-8666-666666666666" }),
              },
            },
          });
        await held;
        return route.fulfill({ json: PREFLIGHT });
      });
      try {
        await page.goto(`/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`);
        const main = page.getByRole("main");
        const confirm = page.getByRole("button", {
          name: "我確認以上權限,開始試跑",
          exact: true,
        });
        await expect(main.getByRole("alert")).toContainText("暫時無法讀取權限摘要");
        await expect(confirm).toHaveCount(0);
        await expect(main.locator(".preflight-facts")).toHaveCount(0);
        expect(writes).toEqual([]);
        const failureAxe = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          info.outputPath("preflight-context-failure-a11y.json"),
          JSON.stringify(failureAxe),
        );
        expect(failureAxe.violations).toEqual([]);
        expect(failureAxe.incomplete).toEqual([]);
        await page.screenshot({ path: info.outputPath("foreign-summary.png") });
        await main.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect.poll(() => reads).toBe(2);
        const heading = page.getByRole("heading", {
          name: "執行前權限確認",
          exact: true,
        });
        await expect(heading).toBeFocused();
        await expect(confirm).toHaveCount(0);
        release();
        await expect(confirm).toBeEnabled();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(main.locator(".preflight-facts")).toContainText("rows.csv");
        await expect(page.getByLabel("小工具 Version", { exact: true })).toHaveValue(VERSION);
        await expect(heading).toBeFocused();
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        expect(writes).toEqual([]);
        expect(reads).toBe(2);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        const axe = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(info.outputPath("preflight-context-a11y.json"), JSON.stringify(axe));
        expect(axe.violations).toEqual([]);
        expect(axe.incomplete).toEqual([]);
        await page.screenshot({ path: info.outputPath("matching-summary.png") });
      } finally {
        release();
      }
    });
  }
}
