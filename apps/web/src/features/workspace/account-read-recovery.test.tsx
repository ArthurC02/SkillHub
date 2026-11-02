import { expect, test } from "vitest";
import { platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const profile = platformResponse("/me").body as Record<string, unknown>;
const pending = {
  deletion_requested_at: "2026-08-17T00:00:00Z",
  purge_after: "2099-09-16T00:00:00Z",
  deletion_scope: "帳號、小工具與試跑紀錄會刪除。",
};

for (const action of ["request", "cancel"]) {
  test(`account ${action} operation succeeds but failed state read hides old controls and retries without replay`, async () => {
    const page = app();
    let requested = action === "cancel";
    let failing = false;
    let reads = 0;
    const writes: string[] = [];
    page.route("**/me", ({ method }) => {
      if (method === "DELETE") {
        writes.push("DELETE");
        requested = true;
        failing = true;
        return { json: { ...pending, cancellable: true, scope: pending.deletion_scope } };
      }
      reads += 1;
      return failing
        ? { status: 503, json: { error: "service unavailable" } }
        : {
            json: {
              ...profile,
              ...(requested
                ? pending
                : { deletion_requested_at: null, purge_after: null, deletion_scope: null }),
            },
          };
    });
    page.route("**/me/deletion/cancel", ({ method }) => {
      writes.push(method);
      requested = false;
      failing = true;
      return { json: { deletion_requested_at: null } };
    });
    await page.goto("/workspace/account");
    if (action === "request") {
      await expect(page.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(1);
      await page.getByRole("button", { name: "刪除我的帳號", exact: true }).click();
      await page.getByRole("button", { name: "確認開始刪除", exact: true }).click();
    } else {
      await expect(page.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(1);
      await page.getByRole("button", { name: "取消刪除申請", exact: true }).click();
    }
    await expect(page.locator("main [role=alert]")).toContainText(
      "操作已送出，但最新帳號狀態暫時無法確認",
    );
    await expect(page.getByRole("heading", { name: "帳號", exact: true })).toHaveCount(1);
    await expect(page.locator("main .account-profile")).toHaveCount(0);
    await expect(page.locator("main .account-danger-zone")).toHaveCount(0);
    await expect(page.getByText("刪除申請中", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "確認開始刪除", exact: true })).toHaveCount(0);
    expect(writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    const before = reads;
    failing = false;
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(page.locator("main .account-profile")).toHaveCount(1);
    await expect(
      page.getByRole("button", {
        name: action === "request" ? "取消刪除申請" : "刪除我的帳號",
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 1 : 0,
    );
    await expect(page.locator("main")).toBeFocused();
    expect(reads).toBe(before + 1);
    expect(writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

for (const status of [403, 401]) {
  test(`account cached ${status} withdraws private profile and deletion controls`, async () => {
    const page = app();
    let failing = false;
    page.route("**/me", () =>
      failing ? { status, json: { error: "unavailable" } } : { json: { ...profile, ...pending } },
    );
    await page.goto("/workspace/account");
    await expect(page.locator("main .account-profile")).toHaveCount(1);
    await expect(page.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(1);
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
    failing = true;
    await page.goBack();
    await expect(page.getByRole("heading", { name: "帳號", exact: true })).toHaveCount(1);
    if (status === 401)
      await expect(
        page.locator("main [role=status]").filter({ hasText: "帳號資料需要登入" }),
      ).toHaveCount(1);
    else await expect(page.locator("main [role=alert]")).toContainText("暫時無法讀取帳號資料");
    await expect(page.locator("main .account-profile")).toHaveCount(0);
    await expect(page.locator("main .account-danger-zone")).toHaveCount(0);
    await expect(page.getByText(String(profile.email), { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(0);
    if (status === 401)
      await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
    else {
      failing = false;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(page.locator("main .account-profile")).toHaveCount(1);
      await expect(page.locator("main")).toBeFocused();
    }
  });
}
