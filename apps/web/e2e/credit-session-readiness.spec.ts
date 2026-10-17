import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

for (const phase of ["pending", "offline"]) {
  for (const amount of [25, -25]) {
    test(`a ${amount} credit intent blocked by ${phase} session verification stays editable and waits for explicit submission`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 812 });
      await stubPlatform(page);
      let checking = false;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/me", async (route) => {
        if (checking && phase === "pending") await gate;
        return route.fulfill({ json: { ...ME, operator: true } });
      });
      let balance = 120;
      const writes: unknown[] = [];
      await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, (route) =>
        route.fulfill({ json: { ...ADMIN_LEDGER, balance_credits: balance } }),
      );
      await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, (route) => {
        writes.push(route.request().postDataJSON());
        balance += amount * 2;
        return route.fulfill({
          json: {
            workspace_id: ADMIN_ACCOUNT.workspace_id,
            amount_credits: amount * 2,
            balance_credits: balance,
          },
        });
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
        if (phase === "offline") await setOffline(page, true);
        await page.evaluate(() =>
          document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
        );
        await expect(page.getByRole("banner").getByRole("status")).toContainText(
          phase === "offline" ? "等待連線以確認登入狀態" : "正在確認登入狀態",
        );
        const main = page.getByRole("main");
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
        const report = await new AxeBuilder({ page }).analyze();
        await writeFile(
          test.info().outputPath("credit-not-sent-a11y.json"),
          JSON.stringify(report),
        );
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        await page.screenshot({ path: test.info().outputPath("credit-not-sent-viewport.png") });
        await input.fill(String(amount * 2));
        await note.fill("修訂後的點數草稿");
        await expect(alert).toHaveCount(0);
        expect(writes).toEqual([]);
        checking = false;
        if (phase === "offline") await setOffline(page, false);
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
        await setOffline(page, false);
      }
    });
  }
}
