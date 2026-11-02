import { expect, test } from "vitest";
import type { CreditLedger } from "../admin.service";
import { renderedApp } from "../../../testing/app";
import { ADMIN_ACCOUNT, ADMIN_LEDGER } from "../../../testing/fixtures/platform";

const app = renderedApp();

const LEDGER_URL = `**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`;

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
  test(`a cached ledger preserves the ${kind} draft while offline refresh and retry wait for fresh evidence`, async () => {
    const page = app();
    let reads = 0;
    let balance = 120;
    const entries: CreditLedger["entries"] = [...ADMIN_LEDGER.entries];
    const writes: unknown[] = [];
    let releaseFailure!: () => void;
    let releaseRecovery!: () => void;
    const failureGate = new Promise<void>((resolve) => {
      releaseFailure = resolve;
    });
    const recoveryGate = new Promise<void>((resolve) => {
      releaseRecovery = resolve;
    });
    page.route(LEDGER_URL, async () => {
      reads++;
      if (reads === 2) {
        await failureGate;
        return { status: 503, json: { error: "service unavailable" } };
      }
      if (reads === 3) await recoveryGate;
      return { json: { ...ADMIN_LEDGER, balance_credits: balance, entries } };
    });
    page.route(`${LEDGER_URL}/grants`, (request) => {
      writes.push(request.body);
      balance = finalBalance;
      entries.unshift({
        kind,
        delta_credits: amount,
        ref_type: "operator_grant",
        estimated: false,
        created_at: "2026-10-15T20:00:00Z",
      });
      return {
        json: {
          workspace_id: ADMIN_ACCOUNT.workspace_id,
          balance_credits: finalBalance,
          amount_credits: amount,
        },
      };
    });
    try {
      await page.goto("/admin/accounts");
      await page.getByLabel("Email", { exact: true }).fill("member@example.com");
      await page.getByLabel("Email", { exact: true }).press("Enter");
      await expect(
        page.getByRole("heading", { name: ADMIN_ACCOUNT.display_name, exact: true }),
      ).toBeVisible();
      await page.locator("#admin-grant-amount").fill(String(amount));
      await page.locator("#admin-grant-note").fill("確認本次點數異動");
      const main = page.locator("main");
      const submit = main.getByRole("button", { name: action, exact: true });
      const heading = main.getByRole("heading", { name: "點數", exact: true });
      const refresh = main.getByRole("button", { name: "重新整理點數", exact: true });
      await expect(submit).toBeEnabled();
      await page.setOffline(true);
      await refresh.click();
      await expect(submit).toBeDisabled();
      await expect(submit).toHaveAccessibleDescription(why);
      await expect(refresh).toBeDisabled();
      await expect(heading).toBeFocused();
      await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
      await expect(main.getByText("上次讀取的餘額", { exact: false })).toBeVisible();
      await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
      expect(await page.axeViolations("body")).toEqual([]);
      expect(reads).toBe(1);
      expect(writes).toEqual([]);
      await page.setOffline(false);
      await page.eventually(() => expect(reads).toBe(2));
      await expect(main.getByRole("status")).toContainText("載入點數中");
      await expect(submit).toBeDisabled();
      releaseFailure();
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取點數。");
      await expect(main.getByText("上次讀取的餘額", { exact: false })).toHaveCount(0);
      await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
      const retry = main.getByRole("button", { name: "再試一次", exact: true });
      await expect(retry).toBeEnabled();
      await page.setOffline(true);
      await retry.click();
      await expect(retry).toBeDisabled();
      await expect(submit).toBeDisabled();
      await expect(heading).toBeFocused();
      await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
      expect(await page.axeViolations("body")).toEqual([]);
      expect(reads).toBe(2);
      expect(writes).toEqual([]);
      await page.setOffline(false);
      await page.eventually(() => expect(reads).toBe(3));
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
      await page.setOffline(false);
    }
  });
}

test("an initial ledger read waits for connectivity without claiming a balance or offering a grant", async () => {
  const page = app();
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
  page.route("**/admin/accounts?email=*", async () => {
    accountReads++;
    await accountGate;
    return { json: ADMIN_ACCOUNT };
  });
  page.route(LEDGER_URL, async () => {
    ledgerReads++;
    await ledgerGate;
    return { json: ADMIN_LEDGER };
  });
  const writes = () => page.requests.filter((request) => request.method === "POST");
  try {
    await page.goto("/admin/accounts");
    await page.getByLabel("Email", { exact: true }).fill("member@example.com");
    await page.getByLabel("Email", { exact: true }).press("Enter");
    await page.eventually(() => expect(accountReads).toBe(1));
    await page.setOffline(true);
    releaseAccount();
    document.dispatchEvent(new Event("visibilitychange"));
    await expect(page.locator(".app-header").getByRole("status")).toContainText("等待連線");
    const main = page.locator("main");
    await expect(main.getByRole("status")).toContainText("等待連線以讀取點數");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
    await expect(main.getByText("目前餘額", { exact: false })).toHaveCount(0);
    expect(ledgerReads).toBe(0);
    expect(await page.axeViolations("body")).toEqual([]);
    await page.setOffline(false);
    await page.eventually(() => expect(ledgerReads).toBe(1));
    await expect(main.getByRole("status")).toContainText("載入點數中");
    releaseLedger();
    await expect(main.getByText("目前餘額", { exact: false })).toBeVisible();
    await expect(page.locator("#admin-grant-amount")).toHaveValue("");
    expect(writes()).toEqual([]);
  } finally {
    releaseAccount();
    releaseLedger();
    await page.setOffline(false);
  }
});
