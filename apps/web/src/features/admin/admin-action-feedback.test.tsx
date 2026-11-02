import { expect, test } from "vitest";
import { ADMIN_LEDGER } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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
  for (const owner of ["submitted form", "header search"]) {
    test(`admin ${outcome.name} feedback respects ${owner} focus`, async () => {
      const page = app();
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
      page.route(`**/admin/credits/${ADMIN_LEDGER.workspace_id}`, () => ({
        json: { ...ADMIN_LEDGER, balance_credits: completed ? 170 : 120 },
      }));
      page.route(`**/admin/credits/${ADMIN_LEDGER.workspace_id}/grants`, async ({ body }) => {
        writes.push(body);
        started();
        await held;
        completed = outcome.status === 200;
        return {
          status: outcome.status,
          json: completed
            ? {
                workspace_id: ADMIN_LEDGER.workspace_id,
                amount_credits: 50,
                balance_credits: 170,
              }
            : { error: "request rejected" },
        };
      });
      try {
        await page.goto("/admin/accounts");
        await page.eventually(() =>
          expect(page.getByLabel("Email", { exact: true }).count()).toBe(1),
        );
        await page.getByLabel("Email", { exact: true }).fill("member@example.com");
        await page.getByLabel("Email", { exact: true }).press("Enter");
        await page.eventually(() =>
          expect(page.getByRole("heading", { name: "封測者甲" }).count()).toBeGreaterThan(0),
        );
        await page.locator("#admin-grant-amount").fill("50");
        await page.locator("#admin-grant-note").fill("checked operator grant");
        await page.getByRole("button", { name: "授予", exact: true }).click();
        await received;
        await page.eventually(() =>
          expect(page.getByRole("button", { name: "送出中…", exact: true }).isDisabled()).toBe(
            true,
          ),
        );
        const search = page.locator('header input[type="search"]');
        if (owner === "header search") await search.fill("下一個任務");
        release();
        const result = page.locator('[role=group][aria-label="操作結果"]');
        const feedback = result.locator(outcome.status === 200 ? "[role=status]" : "[role=alert]");
        await page.eventually(() => expect(feedback.text()).toBe(outcome.message));
        if (owner === "header search") {
          await page.eventually(() => expect(search.isFocused()).toBe(true));
          expect(search.value()).toBe("下一個任務");
          expect(result.isFocused()).toBe(false);
        } else await page.eventually(() => expect(result.isFocused()).toBe(true));
        expect(writes).toHaveLength(1);
        expect(writes[0]).toMatchObject({ amount_credits: 50, reason: "checked operator grant" });
        expect(await page.axeViolations("form:has(#admin-grant-note)")).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
