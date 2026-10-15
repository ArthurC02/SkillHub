import { expect, test, type Page } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function returnToTab(page: Page) {
  await page.bringToFront();
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
  );
}

test("logging out in another tab withdraws the previous account when returning to it", async ({
  page,
  context,
}) => {
  const other = await context.newPage();
  let signedIn = true;
  let documents = 0;
  page.on("request", (request) => {
    if (request.resourceType() === "document") documents += 1;
  });
  for (const tab of [page, other]) {
    await stubPlatform(tab);
    await tab.route("**/me", (route) =>
      signedIn
        ? route.fulfill({ json: platformResponse("/me").body })
        : route.fulfill({ status: 401, json: { error: "unauthorized" } }),
    );
    await tab.route("**/auth/logout", (route) => {
      signedIn = false;
      return route.fulfill({ status: 204 });
    });
    await tab.goto("/workspace/account");
    await expect(
      tab.getByRole("main").getByText("tester@example.com", { exact: true }),
    ).toBeVisible();
  }
  await other.getByRole("button", { name: "登出", exact: true }).click();
  await expect(
    other.getByRole("main").getByText("帳號資料需要登入。", { exact: false }),
  ).toBeVisible();
  await returnToTab(page);
  await expect(page.getByRole("main").locator(".account-profile")).toHaveCount(0);
  await expect(page.getByRole("main").locator(".account-danger-zone")).toHaveCount(0);
  await expect(
    page.getByRole("main").getByText("帳號資料需要登入。", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".app-sidebar-context-name")).not.toContainText("tester");
  expect(documents).toBe(2);
  await page.screenshot({
    path: test.info().outputPath("returned-account-tab.png"),
    fullPage: true,
  });
});

for (const field of ["user_id", "workspace_id"]) {
  test(`a changed ${field} discovered on return clears the previous page draft`, async ({
    page,
    context,
  }) => {
    const other = await context.newPage();
    const initial = platformResponse("/me").body as Record<string, unknown>;
    let profile = initial;
    let documents = 0;
    page.on("request", (request) => {
      if (request.resourceType() === "document") documents += 1;
    });
    for (const tab of [page, other]) {
      await stubPlatform(tab);
      await tab.route("**/me", (route) => route.fulfill({ json: profile }));
    }
    await page.goto("/workspace/account");
    await expect(
      page.getByRole("main").getByText("tester@example.com", { exact: true }),
    ).toBeVisible();
    await page.getByRole("searchbox", { name: "搜尋 Catalog" }).fill("previous private draft");
    const name = field === "user_id" ? "second" : "tester";
    profile = {
      ...initial,
      [field]: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
      display_name: name,
      email: `${name}@example.com`,
    };
    await other.goto("/policy");
    await returnToTab(page);
    await expect(
      page.getByRole("main").getByText(`${name}@example.com`, { exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("searchbox", { name: "搜尋 Catalog" })).toHaveValue("");
    await expect(page.locator(".app-sidebar-context-name")).toHaveText(name);
    expect(documents).toBe(2);
    expect(new URL(page.url()).pathname).toBe("/workspace/account");
  });
}

test("returning with the same identity updates the profile without discarding the current draft", async ({
  page,
}) => {
  await stubPlatform(page);
  const initial = platformResponse("/me").body as Record<string, unknown>;
  let profile = initial;
  let documents = 0;
  page.on("request", (request) => {
    if (request.resourceType() === "document") documents += 1;
  });
  await page.route("**/me", (route) => route.fulfill({ json: profile }));
  await page.goto("/workspace/account");
  await expect(
    page.getByRole("main").getByText("tester@example.com", { exact: true }),
  ).toBeVisible();
  await page.getByRole("searchbox", { name: "搜尋 Catalog" }).fill("my unfinished task");
  profile = { ...initial, display_name: "renamed" };
  await returnToTab(page);
  await expect(page.locator(".app-sidebar-context-name")).toHaveText("renamed");
  await expect(page.getByRole("searchbox", { name: "搜尋 Catalog" })).toHaveValue(
    "my unfinished task",
  );
  expect(documents).toBe(1);
});

for (const failure of ["server", "network"]) {
  test(`an unreadable ${failure} foreground session preserves the draft and recovers without declaring logout`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let failing = false;
    let documents = 0;
    page.on("request", (request) => {
      if (request.resourceType() === "document") documents += 1;
    });
    await page.route("**/me", (route) => {
      if (!failing) return route.fulfill({ json: platformResponse("/me").body });
      return failure === "network"
        ? route.abort("failed")
        : route.fulfill({ status: 503, json: { error: "service unavailable" } });
    });
    await page.goto("/workspace/account");
    const main = page.getByRole("main");
    await expect(main.getByText("tester@example.com", { exact: true })).toBeVisible();
    await page.getByRole("searchbox", { name: "搜尋 Catalog" }).fill("my unfinished task");
    failing = true;
    await returnToTab(page);
    await expect(main.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    await expect(main.getByText("帳號資料需要登入。", { exact: false })).toHaveCount(0);
    await expect(page.getByRole("searchbox", { name: "搜尋 Catalog" })).toHaveValue(
      "my unfinished task",
    );
    expect(documents).toBe(1);
    failing = false;
    await main.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(main.getByText("tester@example.com", { exact: true })).toBeVisible();
    await expect(page.getByRole("searchbox", { name: "搜尋 Catalog" })).toHaveValue(
      "my unfinished task",
    );
    expect(documents).toBe(1);
  });
}

test("a previously signed-out tab starts a fresh page when a login is confirmed on return", async ({
  page,
  context,
}) => {
  const other = await context.newPage();
  let signedIn = false;
  let documents = 0;
  page.on("request", (request) => {
    if (request.resourceType() === "document") documents += 1;
  });
  for (const tab of [page, other]) {
    await stubPlatform(tab);
    await tab.route("**/me", (route) =>
      signedIn
        ? route.fulfill({ json: platformResponse("/me").body })
        : route.fulfill({ status: 401, json: { error: "unauthorized" } }),
    );
  }
  await page.goto("/workspace/account");
  await expect(
    page.getByRole("main").getByText("帳號資料需要登入。", { exact: false }),
  ).toBeVisible();
  await page.getByRole("searchbox", { name: "搜尋 Catalog" }).fill("previous anonymous draft");
  signedIn = true;
  await other.goto("/policy");
  await returnToTab(page);
  await expect(
    page.getByRole("main").getByText("tester@example.com", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "搜尋 Catalog" })).toHaveValue("");
  expect(documents).toBe(2);
});
