import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const entry = {
  id: "credit-1",
  kind: "grant",
  label: "測試點數紀錄",
  delta_credits: 40,
  estimated: false,
  created_at: "2026-08-17T09:00:00Z",
};

test("an empty partial statement does not claim there were no transactions", async () => {
  const page = app();
  let failing = true;
  page.route("**/me/credits/entries**", (request) => {
    const cursor = request.url.searchParams.get("before");
    if (cursor && failing) return { status: 503, json: { error: "unavailable" } };
    return {
      json: cursor
        ? { entries: [entry], note: "" }
        : { entries: [], next_before: "credit-1", note: "" },
    };
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
  test(`${owner} withdraws cached claims on read failure and recovers independently`, async () => {
    const page = app();
    let failing = false;
    let balanceReads = 0;
    let statementReads = 0;
    page.route("**/me/credits", () => {
      balanceReads += 1;
      return failing && owner === "balance"
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: { balance_credits: 120 } };
    });
    page.route("**/me/credits/entries", () => {
      statementReads += 1;
      return failing && owner !== "balance"
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: { entries: owner === "populated statement" ? [entry] : [], note: "" } };
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
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

for (const status of [503, 403]) {
  test(`older statement ${status} ${status === 503 ? "retains partial evidence and retries its cursor" : "withdraws inaccessible cached entries"}`, async () => {
    const page = app();
    const cursors: string[] = [];
    let failing = true;
    page.route("**/me/credits/entries**", (request) => {
      const cursor = request.url.searchParams.get("before") ?? "";
      cursors.push(cursor);
      if (cursor && failing) return { status, json: { error: "unavailable" } };
      return {
        json: {
          entries: cursor ? [{ ...entry, id: "credit-2", label: "更早的點數" }] : [entry],
          next_before: cursor ? undefined : "credit-1",
          note: "",
        },
      };
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
