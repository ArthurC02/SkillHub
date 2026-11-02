import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, platformResponse } from "../../../testing/fixtures/platform";

const app = renderedApp();

for (const phase of ["pending", "offline"]) {
  for (const amount of [25, -25]) {
    test(`a ${amount} credit intent blocked by ${phase} session verification stays editable and waits for explicit submission`, async () => {
      const page = app();
      let checking = false;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/me", async () => {
        if (checking && phase === "pending") await gate;
        return { json: { ...(platformResponse("/me").body as object), operator: true } };
      });
      let balance = 120;
      const writes: unknown[] = [];
      page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, () => ({
        json: { ...ADMIN_LEDGER, balance_credits: balance },
      }));
      page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, (request) => {
        writes.push(request.body);
        balance += amount * 2;
        return {
          json: {
            workspace_id: ADMIN_ACCOUNT.workspace_id,
            amount_credits: amount * 2,
            balance_credits: balance,
          },
        };
      });
      try {
        await page.goto("/admin/accounts");
        await page.getByLabel("Email", { exact: true }).fill("member@example.com");
        await page.getByLabel("Email", { exact: true }).press("Enter");
        await expect(page.getByRole("heading", { name: "封測者甲", exact: true })).toBeVisible();
        const input = page.locator("#admin-grant-amount");
        const note = page.locator("#admin-grant-note");
        await input.fill(String(amount));
        await note.fill("尚未送出的點數草稿");
        checking = true;
        if (phase === "offline") await page.setOffline(true);
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
        await expect(page.locator(".app-header").getByRole("status")).toContainText(
          phase === "offline" ? "等待連線以確認登入狀態" : "正在確認登入狀態",
        );
        const main = page.locator("main");
        const submit = main.getByRole("button", {
          name: amount > 0 ? "授予" : "扣減點數",
          exact: true,
        });
        await submit.click();
        const alert = main.getByRole("alert");
        await expect(alert).toContainText("這次操作尚未送出");
        await expect(alert).toBeFocused();
        await expect(input).not.toHaveAttribute("readonly", "");
        await expect(note).not.toHaveAttribute("readonly", "");
        await expect(
          main.getByText("結果尚未確認；金額與理由暫時不能更改。", { exact: false }),
        ).toHaveCount(0);
        expect(writes).toEqual([]);
        expect(await page.axeViolations("body")).toEqual([]);
        await input.fill(String(amount * 2));
        await note.fill("修訂後的點數草稿");
        await expect(alert).toHaveCount(0);
        expect(writes).toEqual([]);
        checking = false;
        if (phase === "offline") await page.setOffline(false);
        else release();
        await expect(page.locator(".auth-name")).toHaveText("tester");
        await expect(input).toHaveValue(String(amount * 2));
        await expect(note).toHaveValue("修訂後的點數草稿");
        expect(writes).toEqual([]);
        await submit.click();
        await expect(main.getByRole("status")).toContainText(
          `已${amount > 0 ? "授予" : "扣減"} 50 點，餘額現在是 ${120 + amount * 2} 點。`,
        );
        expect(writes).toHaveLength(1);
        expect(writes[0]).toMatchObject({
          amount_credits: amount * 2,
          reason: "修訂後的點數草稿",
          idempotency_key: expect.stringMatching(/^[0-9a-f-]{36}$/),
        });
        await expect(submit).toBeDisabled();
      } finally {
        release();
        await page.setOffline(false);
      }
    });
  }
}
