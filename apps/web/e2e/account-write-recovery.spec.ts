import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Route } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const profile = platformResponse("/me").body as Record<string, unknown>;
const pending = {
  deletion_requested_at: "2099-08-17T00:00:00Z",
  purge_after: "2099-09-16T00:00:00Z",
  deletion_scope: String(profile.deletion_scope),
};

async function stubAccount(page: Page, requested: boolean) {
  await stubPlatform(page);
  const scenario = {
    requested,
    committed: true,
    failure: "network" as "network" | "server" | "none",
    readFails: false,
    holdRead: undefined as Promise<void> | undefined,
    reads: 0,
    writes: [] as string[],
  };
  async function write(route: Route, next: boolean) {
    scenario.writes.push(route.request().method());
    if (scenario.committed) scenario.requested = next;
    if (scenario.failure === "network") return route.abort("failed");
    if (scenario.failure === "server") {
      return route.fulfill({
        status: 500,
        json: {
          error: next ? "刪除要求沒有記錄成功，可以再試一次" : "取消刪除沒有記錄成功，可以再試一次",
        },
      });
    }
    return route.fulfill({
      json: next
        ? {
            deletion_requested_at: pending.deletion_requested_at,
            purge_after: pending.purge_after,
            cancellable: true,
            scope: pending.deletion_scope,
          }
        : { deletion_requested_at: null },
    });
  }
  await page.route("**/me", async (route) => {
    if (route.request().method() === "DELETE") return write(route, true);
    scenario.reads += 1;
    if (scenario.readFails && scenario.writes.length) {
      await scenario.holdRead;
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    return route.fulfill({
      json: {
        ...profile,
        ...(scenario.requested
          ? pending
          : { deletion_requested_at: null, purge_after: null, deletion_scope: null }),
      },
    });
  });
  await page.route("**/me/deletion/cancel", (route) => write(route, false));
  return scenario;
}

async function operate(page: Page, action: "request" | "cancel") {
  const main = page.getByRole("main");
  if (action === "cancel") {
    await expect(main.getByRole("button", { name: "取消刪除申請", exact: true })).toBeVisible();
    await main.getByRole("button", { name: "取消刪除申請", exact: true }).click();
    return;
  }
  await expect(main.getByRole("button", { name: "刪除我的帳號", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "刪除我的帳號", exact: true }).click();
  await main.getByRole("button", { name: "確認開始刪除", exact: true }).click();
}

for (const action of ["request", "cancel"] as const) {
  for (const committed of [true, false]) {
    for (const failure of ["network", "server"] as const) {
      test(`${action} ${failure} without a response rereads ${committed ? "changed" : "unchanged"} state without claiming the write was unrecorded`, async ({
        page,
      }) => {
        const scenario = await stubAccount(page, action === "cancel");
        scenario.committed = committed;
        scenario.failure = failure;
        await page.goto("/workspace/account");
        await expect(page.locator(".account-profile")).toBeVisible();
        const before = scenario.reads;
        await operate(page, action);
        const main = page.getByRole("main");
        await expect(main.getByRole("alert")).toContainText(
          "未能取得操作結果，請以最新帳號狀態確認",
        );
        const requested = committed ? action === "request" : action === "cancel";
        await expect(main.getByText("刪除申請中", { exact: true })).toHaveCount(requested ? 1 : 0);
        await expect(main.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(
          requested ? 0 : 1,
        );
        await expect(
          main.getByRole("button", {
            name: requested
              ? "取消刪除申請"
              : action === "request" && !committed
                ? "確認開始刪除"
                : "刪除我的帳號",
            exact: true,
          }),
        ).toBeVisible();
        await expect(main).not.toContainText("沒有記錄成功");
        expect(scenario.reads).toBe(before + 1);
        expect(scenario.writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
      });
    }
  }

  test(`${action} loses both operation response and state read, then retries only the read`, async ({
    page,
  }) => {
    const scenario = await stubAccount(page, action === "cancel");
    await page.setViewportSize({ width: 375, height: 900 });
    let releaseRead: () => void = () => {};
    scenario.holdRead = new Promise<void>((resolve) => {
      releaseRead = resolve;
    });
    scenario.readFails = true;
    await page.goto("/workspace/account");
    await expect(page.locator(".account-profile")).toBeVisible();
    const before = scenario.reads;
    await operate(page, action);
    const main = page.getByRole("main");
    await expect.poll(() => scenario.reads).toBe(before + 1);
    await expect(
      main.getByRole("status").filter({ hasText: "正在確認最新帳號狀態" }),
    ).toBeVisible();
    await expect(main.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(0);
    releaseRead();
    await expect(main.getByRole("alert")).toContainText(
      "未能取得操作結果，最新帳號狀態也暫時無法確認",
    );
    await expect(main.locator(".account-danger-zone")).toHaveCount(0);
    await expect(main).not.toContainText("操作已送出");
    await page.screenshot({
      path: test.info().outputPath(`${action}-unknown-phone.png`),
      fullPage: true,
    });
    scenario.readFails = false;
    await main.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(main.locator(".account-profile")).toBeVisible();
    await expect(main.getByRole("alert")).toContainText("未能取得操作結果，請以最新帳號狀態確認");
    await expect(main.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 1 : 0,
    );
    await expect(main.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(
      action === "request" ? 0 : 1,
    );
    await expect(main).toBeFocused();
    expect(scenario.reads).toBe(before + 2);
    expect(scenario.writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    await page.screenshot({
      path: test.info().outputPath(`${action}-recovered-phone.png`),
      fullPage: true,
    });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });

  test(`${action} unknown result is cleared when the opposite operation later succeeds`, async ({
    page,
  }) => {
    const scenario = await stubAccount(page, action === "cancel");
    await page.goto("/workspace/account");
    await operate(page, action);
    const main = page.getByRole("main");
    await expect(main.getByRole("alert")).toContainText("未能取得操作結果");
    scenario.failure = "none";
    await operate(page, action === "request" ? "cancel" : "request");
    await expect(
      main.getByRole("status").filter({
        hasText:
          action === "request" ? "已取消。帳號不會被刪除，資料照舊。" : pending.deletion_scope,
      }),
    ).toBeVisible();
    await expect(main.getByRole("alert")).toHaveCount(0);
    await expect(main.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 0 : 1,
    );
    expect(scenario.writes).toEqual(action === "request" ? ["DELETE", "POST"] : ["POST", "DELETE"]);
  });
}
