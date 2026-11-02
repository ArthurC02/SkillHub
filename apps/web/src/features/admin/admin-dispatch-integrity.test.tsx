import { expect, test } from "vitest";
import { ADMIN_DISPATCH } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const entry of ["/admin", "/admin/dispatch"]) {
  test(`${entry} keeps unreadable dispatch unknown and recovers`, async () => {
    const page = app();
    let current: unknown = { dispatching: "false", halts: [] };
    let reads = 0;
    const writes = () =>
      page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
    page.route("**/admin/dispatch", () => {
      reads++;
      return { json: current };
    });
    await page.goto(entry);
    const home = entry === "/admin";
    const signal = home
      ? page.locator('.admin-home-priorities a[href="/admin/dispatch"] strong')
      : page
          .getByRole("region", { name: "目前派送", exact: true })
          .locator("[role=status], output");
    const failure = page
      .getByRole("alert")
      .filter({ hasText: home ? "1 項狀態無法取得" : "暫時無法讀取派送狀態" });
    const refresh = page.getByRole("button", {
      name: home ? "重新整理狀態" : "重新整理派送狀態",
      exact: true,
    });
    await expect(failure).toBeVisible();
    if (home) {
      await expect(signal).toHaveText("無法取得");
      await expect(page.locator(".admin-home-summary")).toHaveCount(0);
    } else {
      await expect(signal).toHaveCount(0);
      await expect(page.locator("#admin-halt-lift-target")).toHaveCount(0);
      await page.locator("#admin-halt-declare-note").fill("保留維運判斷草稿");
    }
    current = { dispatching: true, halts: [] };
    await refresh.click();
    await expect(signal).toHaveText("正在派送");
    await expect(failure).toHaveCount(0);
    current = { dispatching: true, halts: [ADMIN_DISPATCH.halts[0], { target: "node-3" }] };
    await refresh.click();
    await expect(failure).toBeVisible();
    if (home) {
      await expect(signal).toHaveText("無法取得");
      await expect(page.locator(".admin-home-summary")).toHaveCount(0);
    } else {
      await expect(signal).toHaveCount(0);
      await expect(page.locator("#admin-halt-lift-target")).toHaveCount(0);
      await expect(page.locator("#admin-halt-declare-note")).toHaveValue("保留維運判斷草稿");
    }
    expect(await page.axeViolations("main")).toEqual([]);
    current = { dispatching: false, halts: [] };
    await refresh.click();
    await expect(signal).toHaveText("停止派送");
    await expect(failure).toHaveCount(0);
    if (!home)
      await expect(page.locator("#admin-halt-declare-note")).toHaveValue("保留維運判斷草稿");
    expect(reads).toBe(4);
    expect(writes()).toEqual([]);
  });
}
