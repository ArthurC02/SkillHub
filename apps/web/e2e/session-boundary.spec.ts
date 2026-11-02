import { expect, test } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import AxeBuilder from "./axe";

for (const response of ["lost", "server-error"]) {
  for (const currentSession of ["signed-out"]) {
    test(`an uncertain logout with a ${response} response reconciles a ${currentSession} session without replaying the write`, async ({
      page,
    }) => {
      await stubPlatform(page);
      let attempted = false;
      let writes = 0;
      let reads = 0;
      await page.route("**/me", (route) => {
        if (!attempted) return route.fulfill({ json: platformResponse("/me").body });
        reads += 1;
        return route.fulfill({
          status: currentSession === "signed-out" ? 401 : 503,
          json: { error: currentSession === "signed-out" ? "unauthorized" : "service unavailable" },
        });
      });
      await page.route("**/auth/logout", (route) => {
        attempted = true;
        writes += 1;
        return response === "lost"
          ? route.abort("failed")
          : route.fulfill({ status: 500, json: { error: "internal error" } });
      });
      await page.goto("/workspace/account");
      const main = page.getByRole("main");
      await expect(main.getByText("tester@example.com", { exact: true })).toBeVisible();
      await page.getByRole("button", { name: "登出", exact: true }).click();
      await expect(main.locator(".account-profile")).toHaveCount(0);
      await expect(main.locator(".account-danger-zone")).toHaveCount(0);
      await expect(main.getByText("帳號資料需要登入。", { exact: false })).toBeVisible();
      await expect(page.locator(".app-sidebar-context-name")).not.toContainText("tester");
      expect(reads).toBeGreaterThan(0);
      expect(writes).toBe(1);
      expect(new URL(page.url()).pathname).toBe("/workspace/account");
    });
  }
}

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
            user_id: actor === "tester" ? profile.user_id : "a1111111-1111-4111-8111-111111111111",
            workspace_id:
              actor === "tester" ? profile.workspace_id : "a2222222-2222-4222-8222-222222222222",
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
