import { expect, test } from "vitest";
import { platformResponse } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

const profile = platformResponse("/me").body as Record<string, unknown>;
const pending = {
  deletion_requested_at: "2099-08-17T00:00:00Z",
  purge_after: "2099-09-16T00:00:00Z",
  deletion_scope: String(profile.deletion_scope),
};

function stubAccount(page: App, requested: boolean) {
  const scenario = {
    requested,
    committed: true,
    failure: "network" as "network" | "server" | "none",
    readFails: false,
    holdRead: undefined as Promise<void> | undefined,
    reads: 0,
    writes: [] as string[],
  };
  function write(method: string, next: boolean) {
    scenario.writes.push(method);
    if (scenario.committed) scenario.requested = next;
    if (scenario.failure === "network") return "network-error" as const;
    if (scenario.failure === "server") {
      return {
        status: 500,
        json: {
          error: next ? "刪除要求沒有記錄成功，可以再試一次" : "取消刪除沒有記錄成功，可以再試一次",
        },
      };
    }
    return {
      json: next
        ? {
            deletion_requested_at: pending.deletion_requested_at,
            purge_after: pending.purge_after,
            cancellable: true,
            scope: pending.deletion_scope,
          }
        : { deletion_requested_at: null },
    };
  }
  page.route("**/me", async ({ method }) => {
    if (method === "DELETE") return write(method, true);
    scenario.reads += 1;
    if (scenario.readFails && scenario.writes.length) {
      await scenario.holdRead;
      return { status: 503, json: { error: "service unavailable" } };
    }
    return {
      json: {
        ...profile,
        ...(scenario.requested
          ? pending
          : { deletion_requested_at: null, purge_after: null, deletion_scope: null }),
      },
    };
  });
  page.route("**/me/deletion/cancel", ({ method }) => write(method, false));
  return scenario;
}

async function operate(page: App, action: "request" | "cancel") {
  if (action === "cancel") {
    await expect(page.getByRole("button", { name: "取消刪除申請", exact: true })).toHaveCount(1);
    await page.getByRole("button", { name: "取消刪除申請", exact: true }).click();
    return;
  }
  await expect(page.getByRole("button", { name: "刪除我的帳號", exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: "刪除我的帳號", exact: true }).click();
  await page.getByRole("button", { name: "確認開始刪除", exact: true }).click();
}

for (const action of ["request", "cancel"] as const) {
  for (const committed of [true, false]) {
    for (const failure of ["network", "server"] as const) {
      test(`${action} ${failure} without a response rereads ${committed ? "changed" : "unchanged"} state without claiming the write was unrecorded`, async () => {
        const page = app();
        const scenario = stubAccount(page, action === "cancel");
        scenario.committed = committed;
        scenario.failure = failure;
        await page.goto("/workspace/account");
        await expect(page.locator(".account-profile")).toHaveCount(1);
        const before = scenario.reads;
        await operate(page, action);
        await expect(page.locator("main [role=alert]")).toContainText(
          "未能取得操作結果，請以最新帳號狀態確認",
        );
        const requested = committed ? action === "request" : action === "cancel";
        const buttonName = requested
          ? "取消刪除申請"
          : action === "request" && !committed
            ? "確認開始刪除"
            : "刪除我的帳號";
        await expect(page.getByText("刪除申請中", { exact: true })).toHaveCount(requested ? 1 : 0);
        await expect(page.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(
          requested ? 0 : 1,
        );
        await expect(page.getByRole("button", { name: buttonName, exact: true })).toHaveCount(1);
        await expect(page.locator("main")).not.toContainText("沒有記錄成功");
        expect(scenario.reads).toBe(before + 1);
        expect(scenario.writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
      });
    }
  }

  test(`${action} loses both operation response and state read, then retries only the read`, async () => {
    const page = app();
    const scenario = stubAccount(page, action === "cancel");
    let releaseRead: () => void = () => {};
    scenario.holdRead = new Promise<void>((resolve) => {
      releaseRead = resolve;
    });
    scenario.readFails = true;
    await page.goto("/workspace/account");
    await expect(page.locator(".account-profile")).toHaveCount(1);
    const before = scenario.reads;
    await operate(page, action);
    await page.eventually(() => expect(scenario.reads).toBe(before + 1));
    await expect(
      page.locator("main [role=status]").filter({ hasText: "正在確認最新帳號狀態" }),
    ).toHaveCount(1);
    await expect(page.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(0);
    releaseRead();
    await expect(page.locator("main [role=alert]")).toContainText(
      "未能取得操作結果，最新帳號狀態也暫時無法確認",
    );
    await expect(page.locator("main .account-danger-zone")).toHaveCount(0);
    await expect(page.locator("main")).not.toContainText("操作已送出");
    scenario.readFails = false;
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(page.locator("main .account-profile")).toHaveCount(1);
    await expect(page.locator("main [role=alert]")).toContainText(
      "未能取得操作結果，請以最新帳號狀態確認",
    );
    await expect(page.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 1 : 0,
    );
    await expect(page.getByText("目前沒有刪除申請。", { exact: true })).toHaveCount(
      action === "request" ? 0 : 1,
    );
    await expect(page.locator("main")).toBeFocused();
    expect(scenario.reads).toBe(before + 2);
    expect(scenario.writes).toEqual(action === "request" ? ["DELETE"] : ["POST"]);
    expect(await page.axeViolations("main")).toEqual([]);
  });

  test(`${action} unknown result is cleared when the opposite operation later succeeds`, async () => {
    const page = app();
    const scenario = stubAccount(page, action === "cancel");
    await page.goto("/workspace/account");
    await operate(page, action);
    await expect(page.locator("main [role=alert]")).toContainText("未能取得操作結果");
    scenario.failure = "none";
    await operate(page, action === "request" ? "cancel" : "request");
    await expect(
      page.locator("main [role=status]").filter({
        hasText:
          action === "request" ? "已取消。帳號不會被刪除，資料照舊。" : pending.deletion_scope,
      }),
    ).toBeVisible();
    await expect(page.locator("main [role=alert]")).toHaveCount(0);
    await expect(page.getByText("刪除申請中", { exact: true })).toHaveCount(
      action === "request" ? 0 : 1,
    );
    expect(scenario.writes).toEqual(action === "request" ? ["DELETE", "POST"] : ["POST", "DELETE"]);
  });
}
