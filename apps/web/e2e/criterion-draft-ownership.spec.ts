import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, TEST_CASE_DRAFT } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const scenarios = [
  { mode: "pristine-reread", expected: "伺服器已更新的驗收條件", dirty: false, enabled: false },
  { mode: "canonical-save", expected: "已儲存的內容", dirty: false, enabled: false },
  { mode: "dirty-reread", expected: "未送出的自己的草稿", dirty: true, enabled: true },
  { mode: "pending-typing", expected: "正在編輯的下一稿", dirty: true, enabled: true },
  { mode: "pending-revert", expected: "輸出的列數少於輸入", dirty: true, enabled: true },
  { mode: "empty-reread", expected: "", dirty: true, enabled: false },
  { mode: "unchanged-reread", expected: "輸出的列數少於輸入", dirty: false, enabled: false },
  { mode: "failed-save", expected: "  已儲存的內容  ", dirty: true, enabled: true },
];

for (const scenario of scenarios) {
  for (const width of [375, 1280]) {
    test(`${scenario.mode} preserves criterion draft ownership at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const current = structuredClone(TEST_CASE_DRAFT);
      const writes: { method: string; criterion: string; body: unknown }[] = [];
      let reads = 0;
      let release!: () => void;
      let started!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      const received = new Promise<void>((resolve) => {
        started = resolve;
      });
      await page.route(`**/test-cases/${TEST_CASE}`, (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        reads += 1;
        return route.fulfill({ json: current });
      });
      await page.route(`**/test-cases/${TEST_CASE}/criteria/*`, async (route) => {
        const request = route.request();
        const criterion = new URL(request.url()).pathname.split("/").at(-1)!;
        const body: unknown = request.postDataJSON();
        writes.push({ method: request.method(), criterion, body });
        expect(request.method()).toBe("PATCH");
        if (criterion === "c1") {
          expect(body).toEqual({ text: "  已儲存的內容  " });
          if (scenario.mode === "failed-save") {
            return route.fulfill({ status: 503, json: { error: "暫時無法更新驗收條件" } });
          }
          if (scenario.mode.startsWith("pending-")) {
            started();
            await held;
          }
          current.acceptance_criteria[0].text = "已儲存的內容";
          current.acceptance_criteria[0].confirmed_at = null;
        } else {
          expect(criterion).toBe("c2");
          expect(body).toEqual({ confirmed: true });
          current.acceptance_criteria[1].confirmed_at = "2026-08-18T01:00:00Z";
          if (scenario.mode !== "unchanged-reread") {
            current.acceptance_criteria[0].text = "伺服器已更新的驗收條件";
            current.acceptance_criteria[0].confirmed_at = null;
          }
        }
        current.updated_at = "2026-08-18T01:00:00Z";
        return route.fulfill({ json: current });
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}`);
        const field = page.locator("#criterion-c1");
        const first = page.locator("li.criterion").filter({ has: field });
        const second = page.locator("li.criterion").filter({ has: page.locator("#criterion-c2") });
        const save = first.getByRole("button", { name: "儲存文字", exact: true });
        await expect(field).toHaveValue("輸出的列數少於輸入");
        await expect(save).toBeDisabled();
        await expect(second).toContainText("尚未確認");
        const saving =
          scenario.mode === "canonical-save" ||
          scenario.mode.startsWith("pending-") ||
          scenario.mode === "failed-save";
        if (saving) {
          await field.fill("  已儲存的內容  ");
          await save.click();
          if (scenario.mode.startsWith("pending-")) {
            await received;
            await field.fill(scenario.expected);
            release();
          }
          if (scenario.mode === "failed-save") {
            await expect(first.getByRole("alert")).toHaveText(
              "操作結果尚未確認；請先確認最新資料，再決定是否重送。",
            );
          } else {
            await expect(first).toContainText("狀態：尚未確認");
          }
        } else {
          if (scenario.dirty) await field.fill(scenario.expected);
          await second.getByRole("button", { name: "確認", exact: true }).click();
          await expect(second).toContainText("狀態：已確認");
        }
        await expect(save).toBeVisible();
        await expect(field).toHaveValue(scenario.expected);
        if (scenario.enabled) await expect(save).toBeEnabled();
        else await expect(save).toBeDisabled();
        await expect(first.locator("#criterion-edited-c1")).toHaveCount(scenario.dirty ? 1 : 0);
        if (scenario.mode === "empty-reread") {
          await expect(save).toHaveAccessibleDescription("驗收條件不能是空白");
        }
        await expect.poll(() => reads).toBe(2);
        expect(writes).toEqual([
          saving
            ? { method: "PATCH", criterion: "c1", body: { text: "  已儲存的內容  " } }
            : { method: "PATCH", criterion: "c2", body: { confirmed: true } },
        ]);
        await field.click();
        await expect(field).toBeFocused();
        await expect(field).toBeInViewport({ ratio: 1 });
        const report = await new AxeBuilder({ page })
          .include("li.criterion:has(#criterion-c1)")
          .analyze();
        await writeFile(info.outputPath("criterion-a11y.json"), JSON.stringify(report));
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("criterion.png") });
      } finally {
        release();
      }
    });
  }
}
