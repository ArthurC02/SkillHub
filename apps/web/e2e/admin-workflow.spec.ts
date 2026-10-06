import { expect, test } from "@playwright/test";
import { ADMIN_ACCOUNT, ADMIN_LEDGER } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("operator can find an account and verify a credit grant in the refreshed ledger", async ({
  page,
}) => {
  await stubPlatform(page);
  const lookedUpEmails: string[] = [];
  const grants: { amount_credits: number; reason: string }[] = [];
  await page.route("**/admin/accounts?*", async (route) => {
    lookedUpEmails.push(new URL(route.request().url()).searchParams.get("email") ?? "");
    await route.fulfill({ json: ADMIN_ACCOUNT });
  });
  await page.route("**/admin/credits/ws-2**", async (route) => {
    if (route.request().method() === "POST") {
      const body = route.request().postDataJSON() as {
        amount_credits: number;
        reason: string;
      };
      grants.push({ amount_credits: body.amount_credits, reason: body.reason });
      await route.fulfill({
        json: { balance_credits: 145, amount_credits: body.amount_credits },
      });
      return;
    }
    await route.fulfill({
      json: {
        ...ADMIN_LEDGER,
        balance_credits: grants.length ? 145 : 120,
        entries: grants.length
          ? [
              {
                kind: "grant",
                delta_credits: 25,
                ref_type: null,
                estimated: false,
                created_at: "2026-09-12T08:00:00Z",
              },
              ...ADMIN_LEDGER.entries,
            ]
          : ADMIN_LEDGER.entries,
      },
    });
  });

  await page.goto("/admin");
  await page.locator(".admin-home-list").getByRole("link", { name: "帳號與點數" }).click();
  await page.getByLabel("Email").fill("member@example.com");
  await page.getByRole("button", { name: "查詢" }).click();
  await expect(page.getByText("目前餘額 120 點")).toBeVisible();
  expect(new URL(page.url()).searchParams.has("email")).toBe(false);
  expect(lookedUpEmails).toContain("member@example.com");

  await page.getByLabel("點數（整數，不能是 0；負數是更正）").fill("25");
  await page.getByLabel("理由（必填，會寫進動作紀錄）").fill("封測補點");
  await page.getByRole("button", { name: "授予", exact: true }).click();
  await expect(page.getByText("目前餘額 145 點")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "已授予 25 點" })).toBeVisible();
  expect(grants).toEqual([{ amount_credits: 25, reason: "封測補點" }]);
});
