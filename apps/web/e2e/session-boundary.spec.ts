import { expect, test } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import AxeBuilder from "@axe-core/playwright";

test("signing out withdraws private account state without requiring navigation", async ({
  page,
}) => {
  await stubPlatform(page);
  let signedIn = true;
  await page.route("**/me", (route) =>
    signedIn
      ? route.fulfill({ json: platformResponse("/me").body })
      : route.fulfill({ status: 401, json: { error: "unauthorized" } }),
  );
  await page.route("**/auth/logout", (route) => {
    signedIn = false;
    return route.fulfill({ status: 204 });
  });
  await page.goto("/workspace/account");
  const main = page.getByRole("main");
  await expect(main.getByText("tester@example.com", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "登出", exact: true }).click();
  await expect(main.locator(".account-profile")).toHaveCount(0);
  await expect(main.locator(".account-danger-zone")).toHaveCount(0);
  await expect(main.getByText("帳號資料需要登入。", { exact: false })).toBeVisible();
  await expect(page.locator(".app-sidebar-context-name")).not.toContainText("tester");
  expect(new URL(page.url()).pathname).toBe("/workspace/account");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
});

test("a new offline identity starts with its own account state and no previous operation receipt", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.__SKILLHUB_DEV_LOGIN__ = true;
  });
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  const profile = platformResponse("/me").body as Record<string, unknown>;
  let actor = "tester";
  let cancelled = false;
  let documents = 0;
  page.on("request", (request) => {
    if (request.resourceType() === "document") documents += 1;
  });
  await page.route("**/me", (route) =>
    actor
      ? route.fulfill({
          json: {
            ...profile,
            user_id: actor === "tester" ? profile.user_id : "second-user",
            workspace_id: actor === "tester" ? profile.workspace_id : "second-workspace",
            display_name: actor,
            email: `${actor}@example.com`,
            ...(cancelled || actor !== "tester"
              ? { deletion_requested_at: null, purge_after: null, deletion_scope: null }
              : {}),
          },
        })
      : route.fulfill({ status: 401, json: { error: "unauthorized" } }),
  );
  await page.route("**/me/deletion/cancel", (route) => {
    cancelled = true;
    return route.fulfill({ json: { deletion_requested_at: null } });
  });
  await page.route("**/auth/logout", (route) => {
    actor = "";
    return route.fulfill({ status: 204 });
  });
  await page.route("**/auth/dev/login", (route) => {
    actor = route.request().postDataJSON().user;
    return route.fulfill({ status: 204 });
  });
  await page.goto("/workspace/account");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "取消刪除申請", exact: true }).click();
  await expect(main.getByText("已取消。帳號不會被刪除，資料照舊。", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "登出", exact: true }).click();
  const login = page.locator("[data-auth-controls] form");
  await expect(login).toBeVisible();
  await login.getByRole("textbox").fill("second");
  await login.getByRole("button", { name: "登入", exact: true }).click();
  await expect(main.getByText("second@example.com", { exact: true })).toBeVisible();
  await expect(main.getByText("tester@example.com", { exact: true })).toHaveCount(0);
  await expect(main.getByText("已取消。帳號不會被刪除，資料照舊。", { exact: true })).toHaveCount(
    0,
  );
  await expect(page.locator(".app-sidebar-context-name")).toHaveText("second");
  expect(documents).toBe(3);
  expect(new URL(page.url()).pathname).toBe("/workspace/account");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  await page.screenshot({
    path: test.info().outputPath("second-account-phone.png"),
    fullPage: true,
  });
});

for (const action of ["logout", "login"]) {
  test(`a refused ${action} retains the current task and reports the failure without reloading`, async ({
    page,
  }) => {
    await page.addInitScript(() => {
      window.__SKILLHUB_DEV_LOGIN__ = true;
    });
    await stubPlatform(page);
    if (action === "login")
      await page.route("**/me", (route) =>
        route.fulfill({ status: 401, json: { error: "unauthorized" } }),
      );
    await page.route(action === "logout" ? "**/auth/logout" : "**/auth/dev/login", (route) =>
      route.fulfill({ status: 500, json: { error: "internal error" } }),
    );
    let documents = 0;
    page.on("request", (request) => {
      if (request.resourceType() === "document") documents += 1;
    });
    await page.goto("/workspace/account");
    const controls = page.locator("[data-auth-controls]");
    if (action === "logout")
      await controls.getByRole("button", { name: "登出", exact: true }).click();
    else {
      await controls.getByRole("textbox").fill("second");
      await controls.getByRole("button", { name: "登入", exact: true }).click();
      await expect(controls.getByRole("textbox")).toHaveValue("second");
    }
    await expect(controls.getByRole("alert")).toContainText(
      action === "logout" ? "登出沒有完成" : "登入沒有成功",
    );
    expect(documents).toBe(1);
    if (action === "logout")
      await expect(
        page.getByRole("main").getByText("tester@example.com", { exact: true }),
      ).toBeVisible();
  });
}
