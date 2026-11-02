import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../../../testing/fixtures/platform";

const app = renderedApp();

for (const [name, invalid] of [
  ["an unreadable balance", { ...ADMIN_LEDGER, balance_credits: "not a measured balance" }],
  ["another Workspace", { ...ADMIN_LEDGER, workspace_id: ME.workspace_id }],
  ["an unreadable entry", { ...ADMIN_LEDGER, entries: [ADMIN_LEDGER.entries[0], null] }],
] as const) {
  for (const reread of [false, true]) {
    test(`a ledger from ${name} on ${reread ? "reread" : "first read"} stays unconfirmed and recovers without replaying a grant`, async () => {
      const page = app();
      let reads = 0;
      let recovered = false;
      let release!: () => void;
      const recoveryGate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, async () => {
        reads++;
        if (recovered) {
          await recoveryGate;
          return { json: { ...ADMIN_LEDGER, balance_credits: 0, entries: [] } };
        }
        return { json: reread && reads === 1 ? ADMIN_LEDGER : invalid };
      });
      const writes = () => page.requests.filter((request) => request.method === "POST");
      try {
        await page.goto("/admin/accounts");
        await page.getByLabel("Email", { exact: true }).fill("member@example.com");
        await page.getByLabel("Email", { exact: true }).press("Enter");
        const main = page.locator("main");
        if (reread) {
          await page.locator("#admin-grant-amount").fill("25");
          await page.locator("#admin-grant-note").fill("保留這次的授予草稿");
          await expect(main.getByRole("button", { name: "授予", exact: true })).toBeEnabled();
          await main.getByRole("button", { name: "重新整理點數", exact: true }).click();
        }
        await expect(main.getByRole("alert")).toContainText("暫時無法讀取點數。");
        await expect(main.locator("h1")).toHaveText("帳號與點數");
        await expect(main.getByRole("heading", { name: "封測者甲", exact: true })).toBeVisible();
        await expect(page.getByLabel("Email", { exact: true })).toHaveValue("member@example.com");
        await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
        await expect(main.getByText("這個帳戶的分錄：0 筆。", { exact: true })).toHaveCount(0);
        await expect(main.getByRole("table")).toHaveCount(0);
        if (reread) {
          const grant = main.getByRole("button", { name: "授予", exact: true });
          await expect(grant).toBeDisabled();
          await expect(grant).toHaveAccessibleDescription("要等最新餘額讀取完成，才可授予點數。");
          await expect(page.locator("#admin-grant-amount")).toHaveValue("25");
          await expect(page.locator("#admin-grant-note")).toHaveValue("保留這次的授予草稿");
        } else {
          await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
        }
        expect(reads).toBe(reread ? 2 : 1);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("body")).toEqual([]);
        recovered = true;
        await main.getByRole("button", { name: "再試一次", exact: true }).click();
        await page.eventually(() => expect(reads).toBe(reread ? 3 : 2));
        await expect(main.getByRole("status")).toContainText("載入點數中");
        if (reread)
          await expect(main.getByRole("button", { name: "授予", exact: true })).toBeDisabled();
        expect(writes()).toEqual([]);
        release();
        await expect(main.getByText("目前餘額", { exact: false })).toContainText("0");
        await expect(main.getByText("這個帳戶的分錄：0 筆。", { exact: true })).toBeVisible();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(main.getByRole("heading", { name: "點數", exact: true })).toBeFocused();
        if (!reread) {
          await expect(page.locator("#admin-grant-amount")).toHaveValue("");
          await page.locator("#admin-grant-amount").fill("25");
          await page.locator("#admin-grant-note").fill("新的授予草稿");
        } else {
          await expect(page.locator("#admin-grant-amount")).toHaveValue("25");
          await expect(page.locator("#admin-grant-note")).toHaveValue("保留這次的授予草稿");
        }
        await expect(main.getByRole("button", { name: "授予", exact: true })).toBeEnabled();
        expect(writes()).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
