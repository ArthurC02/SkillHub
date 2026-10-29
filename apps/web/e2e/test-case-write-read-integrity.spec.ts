import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const operation of ["prompt", "criterion"]) {
    test(`an unreadable ${operation} write response rereads without replay and preserves the draft at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      let written = false;
      let failing = true;
      let reads = 0;
      const writes: string[] = [];
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const stored = {
        ...TEST_CASE_DRAFT,
        name: "server name after recovery",
        acceptance_criteria: TEST_CASE_DRAFT.acceptance_criteria.map((criterion) => ({
          ...criterion,
          text: "server criterion after recovery",
        })),
      };
      await page.route(`**/test-cases/${TEST_CASE}`, async (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        if (route.request().method() === "PATCH") {
          written = true;
          writes.push(route.request().url());
          return route.fulfill({ status: 500, json: { error: "讀取失敗" } });
        }
        reads++;
        if (!written) return route.fulfill({ json: TEST_CASE_DRAFT });
        if (!failing) return route.fulfill({ json: stored });
        await gate;
        return route.fulfill({ status: 500, json: { error: "讀取失敗" } });
      });
      await page.route(`**/test-cases/${TEST_CASE}/criteria/c1`, (route) => {
        written = true;
        writes.push(route.request().url());
        return route.fulfill({ status: 500, json: { error: "更新驗收條件失敗" } });
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
        const field =
          operation === "prompt"
            ? page.getByLabel("名稱", { exact: true })
            : page.locator("#criterion-c1");
        const draft = operation === "prompt" ? "kept name draft" : "kept criterion draft";
        await field.fill(draft);
        const submit =
          operation === "prompt"
            ? page.getByRole("button", { name: "儲存", exact: true })
            : page.getByRole("button", { name: "儲存文字", exact: true }).first();
        await submit.click();
        await expect.poll(() => writes.length).toBe(1);
        const unknown = page
          .getByRole("alert")
          .filter({ hasText: "操作結果尚未確認；請先確認最新資料，再決定是否重送。" });
        await expect(unknown).toBeVisible();
        await expect.poll(() => reads).toBe(2);
        await expect(field).toHaveValue(draft);
        await expect(submit).toBeEnabled();
        const audit = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          info.outputPath("test-case-write-unknown-a11y.json"),
          JSON.stringify(audit),
        );
        expect(audit.violations).toEqual([]);
        expect(audit.incomplete).toEqual([]);
        await unknown.scrollIntoViewIfNeeded();
        await page.screenshot({ path: info.outputPath("test-case-write-unknown.png") });
        release();
        await expect(page.getByRole("alert")).toContainText("目前無法確認最新內容，輸入已保留");
        await expect(page.locator(".test-case-detail-page")).toBeHidden();
        await expect(page.locator(".test-case-detail-page")).toHaveAttribute("inert", "");
        failing = false;
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        await retry.focus();
        await retry.press("Enter");
        await expect(field).toHaveValue(draft);
        await expect(unknown).toBeVisible();
        await expect(page.getByText("已儲存。", { exact: true })).toHaveCount(0);
        expect(writes).toHaveLength(1);
        expect(reads).toBe(3);
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        await expect(page.locator("#main-content")).toBeFocused();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("test-case-write-recovered.png") });
      } finally {
        release();
      }
    });
  }
}
