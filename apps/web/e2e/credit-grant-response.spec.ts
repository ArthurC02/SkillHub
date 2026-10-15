import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreditLedger } from "../src/features/admin/admin.service";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  await page.route("**/me", (route) => route.fulfill({ json: { ...ME, operator: true } }));
});

async function inspectUnconfirmedGrant(page: Page) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(
    test.info().outputPath("credit-grant-unconfirmed-a11y.json"),
    JSON.stringify(report),
  );
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({
    path: test.info().outputPath("credit-grant-unconfirmed-phone.png"),
    fullPage: true,
  });
}

for (const amount of [25, -25]) {
  for (const invalid of ["empty", "null", "foreign Workspace", "different amount"]) {
    test(`a ${amount} credit response with ${invalid} preserves the uncertain intent and retry key`, async ({
      page,
    }) => {
      let balance = 120;
      let reads = 0;
      const entries: CreditLedger["entries"] = [...ADMIN_LEDGER.entries];
      const writes: { amount_credits: number; reason: string; idempotency_key: string }[] = [];
      let release!: () => void;
      const retryGate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, (route) => {
        reads++;
        return route.fulfill({ json: { ...ADMIN_LEDGER, balance_credits: balance, entries } });
      });
      await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, async (route) => {
        writes.push(route.request().postDataJSON());
        if (writes.length !== 2) {
          balance += amount;
          entries.unshift({
            kind: amount > 0 ? "grant" : "adjustment",
            delta_credits: amount,
            ref_type: "operator_grant",
            estimated: false,
            created_at: "2026-10-16T00:00:00Z",
          });
        }
        const receipt = {
          workspace_id: ADMIN_ACCOUNT.workspace_id,
          balance_credits: balance,
          amount_credits: amount,
        };
        if (writes.length === 1) {
          const body =
            invalid === "empty"
              ? {}
              : invalid === "null"
                ? null
                : invalid === "foreign Workspace"
                  ? { ...receipt, workspace_id: ME.workspace_id }
                  : { ...receipt, amount_credits: amount * 5 };
          return route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify(body),
          });
        }
        if (writes.length === 2) await retryGate;
        return route.fulfill({ json: receipt });
      });
      try {
        await page.goto("/admin/accounts");
        await page.getByLabel("Email", { exact: true }).fill("member@example.com");
        await page.getByRole("button", { name: "查詢", exact: true }).click();
        const input = page.locator("#admin-grant-amount");
        const note = page.locator("#admin-grant-note");
        await input.fill(String(amount));
        await note.fill("核對這筆點數異動");
        const main = page.getByRole("main");
        const submit = main.getByRole("button", {
          name: amount > 0 ? "授予" : "扣減點數",
          exact: true,
        });
        await submit.click();
        const failure = main.getByRole("alert");
        await expect(failure).toContainText("無法確認操作是否完成");
        await expect(main.getByRole("status")).toHaveCount(0);
        await expect(input).toHaveAttribute("readonly", "");
        await expect(note).toHaveAttribute("readonly", "");
        await expect(input).toHaveValue(String(amount));
        await expect(note).toHaveValue("核對這筆點數異動");
        await expect(
          main.getByText("結果尚未確認；金額與理由暫時不能更改。", { exact: false }),
        ).toBeVisible();
        expect(writes).toHaveLength(1);
        expect(reads).toBe(1);
        await inspectUnconfirmedGrant(page);
        await main.getByRole("button", { name: "重新整理點數", exact: true }).click();
        await expect(main.getByText("目前餘額", { exact: false })).toContainText(
          String(120 + amount),
        );
        await expect(failure).toContainText("無法確認操作是否完成");
        await expect(input).toHaveAttribute("readonly", "");
        await expect(note).toHaveAttribute("readonly", "");
        await expect(
          main.getByRole("cell", { name: amount > 0 ? "+25" : "-25", exact: true }),
        ).toHaveCount(1);
        expect(writes).toHaveLength(1);
        expect(reads).toBe(2);
        await submit.click();
        await expect.poll(() => writes.length).toBe(2);
        expect(writes[1]).toEqual(writes[0]);
        await expect(input).toHaveAttribute("readonly", "");
        await expect(note).toHaveAttribute("readonly", "");
        release();
        const success = main.getByRole("status");
        await expect(success).toContainText(
          `已${amount > 0 ? "授予" : "扣減"} 25 點，餘額現在是 ${120 + amount} 點。`,
        );
        await expect(failure).toHaveCount(0);
        await expect(submit).toBeDisabled();
        await expect(input).not.toHaveAttribute("readonly", "");
        await expect(note).not.toHaveAttribute("readonly", "");
        expect(writes).toHaveLength(2);
        expect(reads).toBe(3);
        await note.fill("新的點數異動");
        await expect(success).toHaveCount(0);
        await submit.click();
        await expect(success).toContainText(`餘額現在是 ${120 + amount * 2} 點。`);
        expect(writes).toHaveLength(3);
        expect(writes[2].idempotency_key).not.toBe(writes[0].idempotency_key);
        expect(writes[2]).toMatchObject({ amount_credits: amount, reason: "新的點數異動" });
        expect(reads).toBe(4);
        await expect(
          main.getByRole("cell", { name: amount > 0 ? "+25" : "-25", exact: true }),
        ).toHaveCount(2);
      } finally {
        release();
      }
    });
  }
}

for (const amount of [9007199254740992, -9007199254740992]) {
  test(`an unsafe credit amount ${amount} cannot be sent before correction`, async ({ page }) => {
    const writes: unknown[] = [];
    const safeAmount = Math.sign(amount) * Number.MAX_SAFE_INTEGER;
    const balance = amount > 0 ? Number.MAX_SAFE_INTEGER : 0;
    await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, (route) =>
      route.fulfill({
        json: { ...ADMIN_LEDGER, balance_credits: amount > 0 ? 0 : Number.MAX_SAFE_INTEGER },
      }),
    );
    await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, (route) => {
      writes.push(route.request().postDataJSON());
      return route.fulfill({
        json: {
          workspace_id: ADMIN_ACCOUNT.workspace_id,
          balance_credits: balance,
          amount_credits: safeAmount,
        },
      });
    });
    await page.goto("/admin/accounts");
    await page.getByLabel("Email", { exact: true }).fill("member@example.com");
    await page.getByRole("button", { name: "查詢", exact: true }).click();
    const input = page.locator("#admin-grant-amount");
    const note = page.locator("#admin-grant-note");
    await input.fill(String(amount));
    await note.fill("調整點數異動");
    const main = page.getByRole("main");
    const submit = main.getByRole("button", {
      name: amount > 0 ? "授予" : "扣減點數",
      exact: true,
    });
    await expect(submit).toBeDisabled();
    await expect(submit).toHaveAccessibleDescription("點數超出可精確計算的範圍，請調整金額。");
    await input.press("Enter");
    expect(writes).toEqual([]);
    await expect(input).not.toHaveAttribute("readonly", "");
    await expect(note).toHaveValue("調整點數異動");
    await input.fill(String(safeAmount));
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect(main.getByRole("status")).toContainText(
      `${Number.MAX_SAFE_INTEGER} 點，餘額現在是 ${balance} 點。`,
    );
    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({
      amount_credits: safeAmount,
      reason: "調整點數異動",
    });
  });
}
