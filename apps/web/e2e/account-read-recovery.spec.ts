import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const profile = platformResponse("/me").body as Record<string, unknown>;
const pending = {
  deletion_requested_at: "2026-08-17T00:00:00Z",
  purge_after: "2099-09-16T00:00:00Z",
  deletion_scope: "帳號、小工具與試跑紀錄會刪除。",
};

for (const action of ["request", "cancel"]) {
  test(`account ${action} operation succeeds but failed state read hides old controls and retries without replay`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let requested = action === "cancel";
    let failing = false;
    let reads = 0;
    const writes: string[] = [];
    await page.route("**/me", (route) => {
      if (route.request().method() === "DELETE") {
        writes.push("DELETE");
        requested = true;
        failing = true;
        return route.fulfill({
          json: { ...pending, cancellable: true, scope: pending.deletion_scope },
        });
      }
      reads += 1;
      return failing
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({
            json: {
              ...profile,
              ...(requested
                ? pending
                : { deletion_requested_at: null, purge_after: null, deletion_scope: null }),
            },
          });
    });
    await page.route("**/me/deletion/cancel", (route) => {
      writes.push(route.request().method());
      requested = false;
      failing = true;
      return route.fulfill({ json: { deletion_requested_at: null } });
    });
    await page.goto("/workspace/account");
    const main = page.getByRole("main");
    if (action === "request") {
      await main.getByRole("button", { name: "刪除我的帳號", exact: true }).click();
      await main.getByRole("button", { name: "確認開始刪除", exact: true }).click();
    } else await main.getByRole("button", { name: "取消刪除申請", exact: true }).click();
    await expect(main.getByRole("alert")).toContainText("操作已送出，但最新帳號狀態暫時無法確認");
    await expect(main.getByRole("heading", { name: "帳號", level: 1, exact: true })).toBeVisible();
    await expect(main.locator(".account-profile")).toHaveCount(0);
    await expect(main.locator(".account-danger-zone")).toHaveCount(0);
    await expect(main.getByText("刪除申請中", { exact: true })).toHaveCount(0);
    await expect(main.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(0);
    await expect(main.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(0);
    await expect(main.getByRole("button", { name: "確認開始刪除", exact: true })).toHaveCount(0);
    expect(writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    await page.screenshot({
      path: test.info().outputPath(`account-${action}-failed-phone.png`),
      fullPage: true,
    });
    const before = reads;
    failing = false;
    await main.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(main.locator(".account-profile")).toBeVisible();
    await expect(
      main.getByRole("button", {
        name: action === "request" ? "取消刪除申請" : "刪除我的帳號",
        exact: true,
      }),
    ).toBeVisible();
    await expect(main.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 1 : 0,
    );
    await expect(main).toBeFocused();
    expect(reads).toBe(before + 1);
    expect(writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}

for (const status of [403, 401]) {
  test(`account cached ${status} withdraws private profile and deletion controls`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let failing = false;
    await page.route("**/me", (route) =>
      failing
        ? route.fulfill({ status, json: { error: "unavailable" } })
        : route.fulfill({ json: { ...profile, ...pending } }),
    );
    await page.goto("/workspace/account");
    const main = page.getByRole("main");
    await expect(main.locator(".account-profile")).toBeVisible();
    await expect(main.getByRole("button", { name: "取消刪除申請", exact: true })).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    failing = true;
    await page.goBack();
    await expect(main.getByRole("heading", { name: "帳號", level: 1, exact: true })).toBeVisible();
    if (status === 401) await expect(main.getByRole("status")).toContainText("帳號資料需要登入");
    else await expect(main.getByRole("alert")).toContainText("暫時無法讀取帳號資料");
    await expect(main.locator(".account-profile")).toHaveCount(0);
    await expect(main.locator(".account-danger-zone")).toHaveCount(0);
    await expect(main.getByText(String(profile.email), { exact: true })).toHaveCount(0);
    await expect(main.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(0);
    await expect(main.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(0);
    if (status === 401)
      await expect(main.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
    else {
      failing = false;
      await main.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(main.locator(".account-profile")).toBeVisible();
      await expect(main).toBeFocused();
    }
  });
}
