import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { stubPlatform } from "./stub";

const entry = {
  id: "credit-1",
  kind: "grant",
  label: "測試點數紀錄",
  delta_credits: 40,
  estimated: false,
  created_at: "2026-08-17T09:00:00Z",
};

test("an empty partial statement does not claim there were no transactions", async ({ page }) => {
  await stubPlatform(page);
  let failing = true;
  await page.route("**/me/credits/entries**", (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("before");
    if (cursor && failing) return route.fulfill({ status: 503, json: { error: "unavailable" } });
    return route.fulfill({
      json: cursor
        ? { entries: [entry], note: "" }
        : { entries: [], next_before: "credit-1", note: "" },
    });
  });
  await page.goto("/workspace/account");
  const statement = page.getByRole("region", { name: "點數紀錄", exact: true });
  await expect(statement).toContainText("尚未讀完，無法確認全部紀錄");
  await expect(statement).not.toContainText("還沒有任何點數進出");
  await statement.getByRole("button", { name: "載入更早的紀錄", exact: true }).click();
  await expect(statement.getByRole("alert")).toContainText("尚未讀完");
  await expect(statement).not.toContainText("還沒有任何點數進出");
  failing = false;
  await statement.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(statement.getByText(entry.label, { exact: false })).toBeVisible();
});

for (const owner of ["balance", "empty statement", "populated statement"]) {
  test(`${owner} withdraws cached claims on read failure and recovers independently`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let failing = false;
    let balanceReads = 0;
    let statementReads = 0;
    await page.route("**/me/credits", (route) => {
      balanceReads += 1;
      return failing && owner === "balance"
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: { balance_credits: 120 } });
    });
    await page.route("**/me/credits/entries", (route) => {
      statementReads += 1;
      return failing && owner !== "balance"
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({
            json: { entries: owner === "populated statement" ? [entry] : [], note: "" },
          });
    });
    await page.goto("/workspace/account");
    const balance = page.getByRole("region", { name: "點數餘額", exact: true });
    const statement = page.getByRole("region", { name: "點數紀錄", exact: true });
    await expect(balance).toContainText("目前餘額 120 點");
    await expect(statement).toContainText(
      owner === "populated statement" ? entry.label : "還沒有任何點數進出",
    );
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    failing = true;
    await page.goBack();
    const failed = owner === "balance" ? balance : statement;
    await expect(failed.getByRole("alert")).toContainText("暫時無法讀取");
    await expect(failed).not.toContainText("目前餘額");
    await expect(failed).not.toContainText("還沒有任何點數進出");
    await expect(failed).not.toContainText(entry.label);
    if (owner === "balance") {
      await page.screenshot({
        path: test.info().outputPath("credit-recovery-phone.png"),
        fullPage: true,
      });
    }
    const beforeBalance = balanceReads;
    const beforeStatement = statementReads;
    failing = false;
    await failed.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(balance).toContainText("目前餘額 120 點");
    await expect(statement).toContainText(
      owner === "populated statement" ? entry.label : "還沒有任何點數進出",
    );
    await expect(page.locator("#account-credits-heading")).toBeFocused();
    expect(balanceReads).toBe(beforeBalance + (owner === "balance" ? 1 : 0));
    expect(statementReads).toBe(beforeStatement + (owner === "balance" ? 0 : 1));
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}

for (const status of [503, 403]) {
  test(`older statement ${status} ${status === 503 ? "retains partial evidence and retries its cursor" : "withdraws inaccessible cached entries"}`, async ({
    page,
  }) => {
    await stubPlatform(page);
    const cursors: string[] = [];
    let failing = true;
    await page.route("**/me/credits/entries**", (route) => {
      const cursor = new URL(route.request().url()).searchParams.get("before") ?? "";
      cursors.push(cursor);
      if (cursor && failing) return route.fulfill({ status, json: { error: "unavailable" } });
      return route.fulfill({
        json: {
          entries: cursor ? [{ ...entry, id: "credit-2", label: "更早的點數" }] : [entry],
          next_before: cursor ? undefined : "credit-1",
          note: "",
        },
      });
    });
    await page.goto("/workspace/account");
    const statement = page.getByRole("region", { name: "點數紀錄", exact: true });
    await expect(statement.getByText(entry.label, { exact: false })).toBeVisible();
    await statement.getByRole("button", { name: "載入更早的紀錄", exact: true }).click();
    await expect(statement.getByRole("alert")).toBeVisible();
    if (status === 503) {
      await expect(statement.getByRole("alert")).toContainText("尚未讀完");
      await expect(statement.getByText(entry.label, { exact: false })).toBeVisible();
    } else await expect(statement.getByText(entry.label, { exact: false })).toHaveCount(0);
    await expect(
      statement.getByRole("button", { name: "載入更早的紀錄", exact: true }),
    ).toHaveCount(0);
    const before = cursors.length;
    failing = false;
    await statement.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(
      statement.getByText(status === 503 ? "更早的點數" : entry.label, { exact: false }),
    ).toBeVisible();
    await expect(page.locator("#account-credits-heading")).toBeFocused();
    expect(cursors.slice(before)).toEqual(status === 503 ? ["credit-1"] : [""]);
    if (status === 403) {
      await expect(statement.locator(".download-item")).toHaveCount(1);
      await statement.getByRole("button", { name: "載入更早的紀錄", exact: true }).click();
      await expect(statement.getByText("更早的點數", { exact: false })).toBeVisible();
      expect(cursors.slice(before)).toEqual(["", "credit-1"]);
    }
    await expect(statement.locator(".download-item")).toHaveCount(2);
  });
}
