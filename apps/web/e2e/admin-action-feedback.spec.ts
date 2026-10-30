import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_LEDGER } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const outcomes = [
  {
    name: "uncertain",
    status: 500,
    message: "無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。",
  },
  { name: "refused", status: 403, message: "操作未被接受；目前沒有執行權限，請確認登入身分。" },
  { name: "completed", status: 200, message: "已授予 50 點，餘額現在是 170 點。" },
];

for (const outcome of outcomes) {
  for (const width of [375, 1280]) {
    for (const owner of ["submitted form", "header search"]) {
      test(`admin ${outcome.name} feedback respects ${owner} focus at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        let release!: () => void;
        let started!: () => void;
        const held = new Promise<void>((resolve) => {
          release = resolve;
        });
        const received = new Promise<void>((resolve) => {
          started = resolve;
        });
        const writes: unknown[] = [];
        let completed = false;
        await page.route(`**/admin/credits/${ADMIN_LEDGER.workspace_id}`, (route) =>
          route.fulfill({ json: { ...ADMIN_LEDGER, balance_credits: completed ? 170 : 120 } }),
        );
        await page.route(`**/admin/credits/${ADMIN_LEDGER.workspace_id}/grants`, async (route) => {
          writes.push(route.request().postDataJSON());
          started();
          await held;
          completed = outcome.status === 200;
          return route.fulfill({
            status: outcome.status,
            json: completed
              ? {
                  workspace_id: ADMIN_LEDGER.workspace_id,
                  amount_credits: 50,
                  balance_credits: 170,
                }
              : { error: "request rejected" },
          });
        });
        try {
          await page.goto("/admin/accounts");
          await page.getByLabel("Email", { exact: true }).fill("member@example.com");
          await page.getByLabel("Email", { exact: true }).press("Enter");
          await expect(page.getByRole("heading", { name: "封測者甲" })).toBeVisible();
          await page.locator("#admin-grant-amount").fill("50");
          await page.locator("#admin-grant-note").fill("checked operator grant");
          const grant = page.getByRole("button", { name: "授予", exact: true });
          await grant.evaluate((element) => {
            window.scrollTo({
              top: scrollY + element.getBoundingClientRect().bottom - innerHeight + 16,
              behavior: "instant",
            });
          });
          await grant.click();
          await received;
          await expect(page.getByRole("button", { name: "送出中…", exact: true })).toBeDisabled();
          const search = page.locator('header input[type="search"]');
          if (owner === "header search") await search.fill("下一個任務");
          release();
          const result = page.getByRole("group", { name: "操作結果", exact: true });
          const feedback = result.getByRole(outcome.status === 200 ? "status" : "alert");
          await expect(feedback).toHaveText(outcome.message);
          if (owner === "header search") {
            await expect(search).toBeFocused();
            await expect(search).toHaveValue("下一個任務");
            await expect(result).not.toBeFocused();
          } else {
            await expect(result).toBeFocused();
            await expect(feedback).toBeInViewport({ ratio: 1 });
          }
          expect(writes).toHaveLength(1);
          expect(writes[0]).toMatchObject({ amount_credits: 50, reason: "checked operator grant" });
          const report = await new AxeBuilder({ page })
            .include("form:has(#admin-grant-note)")
            .analyze();
          await writeFile(info.outputPath("action-feedback-a11y.json"), JSON.stringify(report));
          expect(report.violations).toEqual([]);
          expect(report.incomplete).toEqual([]);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          await page.screenshot({ path: info.outputPath("action-feedback.png") });
        } finally {
          release();
        }
      });
    }
  }
}
