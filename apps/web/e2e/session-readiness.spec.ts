import { expect, test } from "@playwright/test";
import { platformResponse, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import type { ImportResult } from "../src/core/api/types";
import AxeBuilder from "@axe-core/playwright";

const draft = "https://github.com/example/private-draft";
const limits = {
  allowed_hosts: ["github.com"],
  max_zip_bytes: 10485760,
  max_unpacked_bytes: 104857600,
  max_files: 100,
  max_file_bytes: 1048576,
  max_path_depth: 8,
  max_skills_per_import: 20,
  note: "",
};
const imported = {
  shape: "skill",
  skills: [
    {
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      path: "skill",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
} satisfies ImportResult;

for (const failure of ["pending", "server", "network", "offline"]) {
  test(`${failure} identity verification blocks import and recovers without replaying the draft`, async ({
    page,
    context,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.route("**/skills/import/limits", (route) => route.fulfill({ json: limits }));
    let checking = false;
    let writes = 0;
    let release!: () => void;
    let started!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const readStarted = new Promise<void>((resolve) => {
      started = resolve;
    });
    await page.route("**/me", async (route) => {
      if (checking && failure === "pending") {
        started();
        await gate;
      }
      if (checking && failure === "server")
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (checking && failure === "network") return route.abort("failed");
      return route.fulfill({ json: platformResponse("/me").body });
    });
    await page.route("**/skills/import/url", (route) => {
      writes += 1;
      return route.fulfill({ status: 201, json: imported });
    });
    try {
      await page.goto("/workspace/import");
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const input = page.getByRole("textbox", { name: "URL", exact: true });
      await input.fill(draft);
      await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeEnabled();
      checking = true;
      if (failure === "offline") await context.setOffline(true);
      await page.evaluate(() =>
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
      );
      if (failure === "pending") await readStarted;
      else if (failure === "offline")
        await expect(page.getByText("等待連線以確認登入狀態…", { exact: true })).toBeVisible();
      else
        await expect(
          page.getByRole("button", { name: "重新確認登入狀態", exact: true }),
        ).toBeVisible();
      await expect(page.locator(".auth-name")).toHaveCount(0);
      await input.focus();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("main").getByRole("alert")).toHaveText(
        "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
      );
      await expect(page.getByRole("main").getByRole("alert")).toBeFocused();
      await expect(page.getByRole("main").getByRole("alert")).toBeInViewport({ ratio: 1 });
      expect(writes).toBe(0);
      await expect(input).toHaveValue(draft);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("session-readiness-blocked.png") });
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      checking = false;
      if (failure === "pending") release();
      else if (failure === "offline") await context.setOffline(false);
      else {
        await page.getByRole("button", { name: "重新確認登入狀態", exact: true }).click();
        await expect(page.getByRole("main")).toBeFocused();
      }
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toHaveCount(0);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(page.getByRole("button", { name: "重新確認登入狀態", exact: true })).toHaveCount(
        0,
      );
      expect(writes).toBe(0);
      await expect(input).toHaveValue(draft);
      await page.getByRole("button", { name: "開始匯入", exact: true }).click();
      await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
      expect(writes).toBe(1);
    } finally {
      release();
      await context.setOffline(false);
    }
  });
}

test("a changed Workspace after a pending check clears the old draft without importing it", async ({
  page,
}) => {
  await stubPlatform(page);
  let checking = false;
  let writes = 0;
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/skills/import/limits", (route) => route.fulfill({ json: limits }));
  let documents = 0;
  let release!: () => void;
  let started!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const readStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  page.on("request", (request) => {
    if (request.resourceType() === "document") documents += 1;
  });
  await page.route("**/me", async (route) => {
    if (checking) {
      started();
      await gate;
    }
    return route.fulfill({
      json: {
        ...(platformResponse("/me").body as object),
        ...(checking ? { workspace_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd" } : {}),
      },
    });
  });
  await page.route("**/skills/import/url", (route) => {
    writes += 1;
    return route.fulfill({ status: 201, json: imported });
  });
  try {
    await page.goto("/workspace/import");
    await expect(page.locator(".auth-name")).toHaveText("tester");
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    await input.fill(draft);
    await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeEnabled();
    checking = true;
    await page.evaluate(() =>
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
    );
    await readStarted;
    await page.keyboard.press("Enter");
    await expect(page.getByRole("main").getByRole("alert")).toHaveText(
      "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
    );
    expect(writes).toBe(0);
    release();
    await expect(input).toHaveValue("");
    expect(documents).toBe(2);
    expect(writes).toBe(0);
  } finally {
    release();
  }
});

test("logout can still end the session while identity verification is pending", async ({
  page,
}) => {
  await stubPlatform(page);
  let checking = false;
  let signedIn = true;
  await page.setViewportSize({ width: 375, height: 900 });
  let logouts = 0;
  let release!: () => void;
  let started!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const readStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route("**/me", async (route) => {
    if (checking && signedIn) {
      started();
      await gate;
    }
    return signedIn
      ? route.fulfill({ json: platformResponse("/me").body })
      : route.fulfill({ status: 401, json: { error: "unauthorized" } });
  });
  await page.route("**/auth/logout", (route) => {
    logouts += 1;
    signedIn = false;
    release();
    return route.fulfill({ status: 204 });
  });
  try {
    await page.goto("/workspace/account");
    await expect(
      page.getByRole("main").getByText("tester@example.com", { exact: true }),
    ).toBeVisible();
    checking = true;
    await page.evaluate(() =>
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
    );
    await readStarted;
    await page.getByRole("button", { name: "登出", exact: true }).click();
    await expect(
      page.getByRole("main").getByText("帳號資料需要登入。", { exact: false }),
    ).toBeVisible();
    expect(logouts).toBe(1);
  } finally {
    release();
  }
});
