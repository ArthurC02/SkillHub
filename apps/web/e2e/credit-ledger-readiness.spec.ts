import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreditLedger } from "../src/features/admin/admin.service";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  await page.route("**/me", (route) => route.fulfill({ json: { ...ME, operator: true } }));
});

async function inspectState(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(label + "-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(label + "-phone.png"), fullPage: true });
}

for (const [amount, action, why, receipt, finalBalance, kind] of [
  [
    25,
    "授予",
    "要等最新餘額讀取完成，才可授予點數。",
    "已授予 25 點，餘額現在是 145 點。",
    145,
    "grant",
  ],
  [
    -25,
    "扣減點數",
    "要等最新餘額讀取完成，才可更正點數。",
    "已扣減 25 點，餘額現在是 95 點。",
    95,
    "adjustment",
  ],
] as const) {
  test(`a cached ledger preserves the ${kind} draft while offline refresh and retry wait for fresh evidence`, async ({
    page,
  }) => {
    let reads = 0;
    let balance = 120;
    const entries: CreditLedger["entries"] = [...ADMIN_LEDGER.entries];
    const writes: Record<string, unknown>[] = [];
    let releaseFailure!: () => void;
    let releaseRecovery!: () => void;
    const failureGate = new Promise<void>((resolve) => {
      releaseFailure = resolve;
    });
    const recoveryGate = new Promise<void>((resolve) => {
      releaseRecovery = resolve;
    });
    await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, async (route) => {
      reads++;
      if (reads === 2) {
        await failureGate;
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      if (reads === 3) await recoveryGate;
      return route.fulfill({ json: { ...ADMIN_LEDGER, balance_credits: balance, entries } });
    });
    await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, (route) => {
      writes.push(route.request().postDataJSON());
      balance = finalBalance;
      entries.unshift({
        kind,
        delta_credits: amount,
        ref_type: "operator_grant",
        estimated: false,
        created_at: "2026-10-15T20:00:00Z",
      });
      return route.fulfill({
        json: {
          workspace_id: ADMIN_ACCOUNT.workspace_id,
          balance_credits: finalBalance,
          amount_credits: amount,
        },
      });
    });
    try {
      await page.goto("/admin/accounts");
      await page.getByLabel("Email", { exact: true }).fill("member@example.com");
      await page.getByRole("button", { name: "查詢", exact: true }).click();
      await page.locator("#admin-grant-amount").fill(String(amount));
      await page.locator("#admin-grant-note").fill("確認本次點數異動");
      const main = page.getByRole("main");
      const submit = main.getByRole("button", { name: action, exact: true });
      const heading = main.getByRole("heading", { name: "點數", exact: true });
      const refresh = main.getByRole("button", { name: "重新整理點數", exact: true });
      await expect(submit).toBeEnabled();
      await setOffline(page, true);
      await refresh.click();
      await expect(submit).toBeDisabled();
      await expect(submit).toHaveAccessibleDescription(why);
      await expect(refresh).toBeDisabled();
      await expect(heading).toBeFocused();
      await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
      await expect(main.getByText("上次讀取的餘額", { exact: false })).toBeVisible();
      await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
      await inspectState(page, "credit-refresh-paused");
      expect(reads).toBe(1);
      expect(writes).toEqual([]);
      await setOffline(page, false);
      await expect.poll(() => reads).toBe(2);
      await expect(main.getByRole("status")).toContainText("載入點數中");
      await expect(submit).toBeDisabled();
      releaseFailure();
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取點數。");
      await expect(main.getByText("上次讀取的餘額", { exact: false })).toHaveCount(0);
      await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
      const retry = main.getByRole("button", { name: "再試一次", exact: true });
      await expect(retry).toBeEnabled();
      await setOffline(page, true);
      await retry.click();
      await expect(retry).toBeDisabled();
      await expect(submit).toBeDisabled();
      await expect(heading).toBeFocused();
      await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
      await inspectState(page, "credit-retry-paused");
      expect(reads).toBe(2);
      expect(writes).toEqual([]);
      await setOffline(page, false);
      await expect.poll(() => reads).toBe(3);
      await expect(main.getByRole("status")).toContainText("載入點數中");
      await expect(retry).toBeDisabled();
      await expect(submit).toHaveAccessibleDescription(why);
      await expect(page.locator("#admin-grant-amount")).toHaveValue(String(amount));
      await expect(page.locator("#admin-grant-note")).toHaveValue("確認本次點數異動");
      expect(writes).toEqual([]);
      releaseRecovery();
      await expect(main.getByText("目前餘額", { exact: false })).toBeVisible();
      await expect(submit).toBeEnabled();
      await expect(heading).toBeFocused();
      expect(reads).toBe(3);
      expect(writes).toEqual([]);
      await submit.click();
      await expect(main.getByRole("status").filter({ hasText: receipt })).toHaveText(receipt);
      await expect(main.getByText("目前餘額", { exact: false })).toContainText(
        String(finalBalance),
      );
      expect(reads).toBe(4);
      expect(writes).toHaveLength(1);
      expect(writes[0]).toMatchObject({ amount_credits: amount, reason: "確認本次點數異動" });
    } finally {
      releaseFailure();
      releaseRecovery();
    }
  });
}

for (const enlarged of [false, true]) {
  test(`an initial ledger read waits for connectivity without claiming a balance or offering a grant${enlarged ? " with enlarged header text" : ""}`, async ({
    page,
  }) => {
    let accountReads = 0;
    let ledgerReads = 0;
    let releaseAccount!: () => void;
    let releaseLedger!: () => void;
    const accountGate = new Promise<void>((resolve) => {
      releaseAccount = resolve;
    });
    const ledgerGate = new Promise<void>((resolve) => {
      releaseLedger = resolve;
    });
    await page.route("**/admin/accounts?email=*", async (route) => {
      accountReads++;
      await accountGate;
      return route.fulfill({ json: ADMIN_ACCOUNT });
    });
    await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, async (route) => {
      ledgerReads++;
      await ledgerGate;
      return route.fulfill({ json: ADMIN_LEDGER });
    });
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() === "POST") writes.push(request.url());
    });
    try {
      await page.goto("/admin/accounts");
      if (enlarged) await page.addStyleTag({ content: ".app-title { font-size: 24px }" });
      await page.getByLabel("Email", { exact: true }).fill("member@example.com");
      await page.getByRole("button", { name: "查詢", exact: true }).click();
      await expect.poll(() => accountReads).toBe(1);
      await setOffline(page, true);
      releaseAccount();
      await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
      await expect(page.getByRole("banner").getByRole("status")).toContainText("等待連線");
      const main = page.getByRole("main");
      await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
      await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
      await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
      expect(ledgerReads).toBe(0);
      const header = await page.getByRole("banner").evaluate((element) => {
        const title = element.querySelector(".app-title")!;
        const text = document.createRange();
        text.selectNodeContents(title);
        const titleBox = title.getBoundingClientRect();
        const authBox = element.querySelector("[data-auth-controls]")!.getBoundingClientRect();
        return {
          lines: new Set([...text.getClientRects()].map((rect) => rect.top)).size,
          titleRight: titleBox.right,
          authLeft: authBox.left,
          viewport: innerWidth,
          content: document.documentElement.scrollWidth,
        };
      });
      expect(header.lines).toBe(1);
      expect(header.titleRight).toBeLessThanOrEqual(header.authLeft);
      expect(header.content).toBeLessThanOrEqual(header.viewport);
      await inspectState(page, "credit-initial-paused");
      await setOffline(page, false);
      await expect.poll(() => ledgerReads).toBe(1);
      await expect(main.getByRole("status")).toContainText("載入點數中");
      releaseLedger();
      await expect(main.getByText("目前餘額", { exact: false })).toBeVisible();
      await expect(page.locator("#admin-grant-amount")).toHaveValue("");
      expect(writes).toEqual([]);
    } finally {
      releaseAccount();
      releaseLedger();
    }
  });
}
