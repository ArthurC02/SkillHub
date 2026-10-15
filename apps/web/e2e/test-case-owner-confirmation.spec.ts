import { setOffline } from "./offline";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { platformResponse, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function verifyLayout(page: Page, state: string) {
  const result = await new AxeBuilder({ page }).analyze();
  const path = test.info().outputPath(`${state}-accessibility.json`);
  await writeFile(path, JSON.stringify(result));
  await test.info().attach(`${state}-accessibility`, { path, contentType: "application/json" });
  expect(result.violations).toEqual([]);
  expect(result.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath(`${state}.png`), fullPage: true });
}

for (const offline of [false, true]) {
  for (const absent of [false, true]) {
    test(`test-case ownership waits for a ${offline ? "paused" : "fetching"} empty snapshot read that confirms ${absent ? "absence" : "ownership"}`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let reads = 0;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/skills?*", async (route) => {
        reads += 1;
        if (reads > 1) await gate;
        return route.fulfill({
          json:
            reads === 1 || absent
              ? { skills: [], total: 0, limit: 24, truncated: false }
              : platformResponse("/skills").body,
        });
      });
      await page.route("**/test-cases?*", (route) =>
        route.request().resourceType() === "document"
          ? route.fallback()
          : route.fulfill({ json: { test_cases: [], total: 0, limit: 50, truncated: false } }),
      );
      const missing = page.getByRole("status").filter({ hasText: "這個小工具不在你的工作區" });
      const editor = page.locator("#new-test-case");
      try {
        await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
        await expect(missing).toBeVisible();
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        await setOffline(page, offline);
        await page.goBack();
        if (!offline) await expect.poll(() => reads).toBe(2);
        await expect(missing).toHaveCount(0);
        await expect(
          editor
            .getByRole("status")
            .filter({ hasText: offline ? "等待連線以讀取你的小工具清單" : "載入你的小工具清單中" }),
        ).toBeVisible();
        await editor.getByLabel("名稱", { exact: true }).fill("ownership draft");
        await editor.getByLabel("User Prompt", { exact: true }).fill("keep this task");
        await expect(editor.getByRole("button", { name: "建立", exact: true })).toBeDisabled();
        await verifyLayout(page, "unconfirmed");
        await setOffline(page, false);
        await expect.poll(() => reads).toBe(2);
        release();
        await expect(editor.locator("[data-loading]")).toHaveCount(0);
        if (absent) {
          await expect(missing).toBeVisible();
          await expect(editor.getByLabel("小工具", { exact: true })).toHaveValue("");
          await expect(editor.getByRole("button", { name: "建立", exact: true })).toBeDisabled();
        } else {
          await expect(missing).toHaveCount(0);
          await expect(editor.getByLabel("小工具", { exact: true })).toHaveValue(SKILL);
          await expect(editor.getByRole("button", { name: "建立", exact: true })).toBeEnabled();
        }
        await expect(editor.getByLabel("名稱", { exact: true })).toHaveValue("ownership draft");
        await expect(editor.getByLabel("User Prompt", { exact: true })).toHaveValue(
          "keep this task",
        );
        expect(new URL(page.url()).searchParams.get("skill")).toBe(SKILL);
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        expect(reads).toBe(2);
        await verifyLayout(page, "confirmed");
      } finally {
        release();
        await setOffline(page, false);
      }
    });
  }
}

test("a paused test-case cached owner retry keeps its draft and waits before confirming ownership", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/skills?*", (route) => {
    reads += 1;
    if (reads === 1)
      return route.fulfill({ json: { skills: [], total: 0, limit: 24, truncated: false } });
    return reads === 2
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: platformResponse("/skills").body });
  });
  const editor = page.locator("#new-test-case");
  try {
    await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
    await expect(page.getByText("這個小工具不在你的工作區", { exact: false })).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect(editor.getByRole("alert")).toBeVisible();
    await editor.getByLabel("名稱", { exact: true }).fill("retry draft");
    await editor.getByLabel("User Prompt", { exact: true }).fill("keep retry task");
    await setOffline(page, true);
    await editor.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(editor.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
    await expect(
      editor.getByRole("status").filter({ hasText: "等待連線以讀取你的小工具清單" }),
    ).toBeVisible();
    await expect(page.locator("#create-test-case-heading")).toBeFocused();
    await expect(page.getByText("這個小工具不在你的工作區", { exact: false })).toHaveCount(0);
    expect(reads).toBe(2);
    await verifyLayout(page, "retry-paused");
    await setOffline(page, false);
    await expect(editor.getByLabel("小工具", { exact: true })).toHaveValue(SKILL);
    await expect(editor.getByRole("alert")).toHaveCount(0);
    await expect(editor.getByLabel("名稱", { exact: true })).toHaveValue("retry draft");
    await expect(editor.getByLabel("User Prompt", { exact: true })).toHaveValue("keep retry task");
    expect(reads).toBe(3);
    await verifyLayout(page, "retry-confirmed");
  } finally {
    await setOffline(page, false);
  }
});
