import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ME } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [name, url, module] of [
  ["Catalog", "/", "Home.page"],
  ["Admin accounts", "/admin/accounts", "AdminAccounts.page"],
] as const) {
  test(`${name} keeps navigation and a page loading status while its route module is pending`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await stubPlatform(page);
    await page.route("**/me", (route) => route.fulfill({ json: { ...ME, operator: true } }));
    let release!: () => void;
    let started!: () => void;
    let finished!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      started = resolve;
    });
    const continued = new Promise<void>((resolve) => {
      finished = resolve;
    });
    await page.route(`**/assets/${module}-*.js`, async (route) => {
      started();
      try {
        await gate;
        await route.continue();
      } finally {
        finished();
      }
    });
    const navigation = page.goto(url, { waitUntil: "domcontentloaded" });
    try {
      await pending;
      await navigation;
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      const main = page.getByRole("main");
      await expect(main.getByRole("status")).toHaveText("載入頁面中…");
      await expect(page.getByRole("link", { name: "資產庫", exact: true })).toBeVisible();
      if (url !== "/")
        await page
          .getByRole("searchbox", { name: "搜尋 Catalog", exact: true })
          .fill("pending search draft");
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("navigation-pending-a11y.json"),
        JSON.stringify(report),
      );
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await page.screenshot({
        path: test.info().outputPath("navigation-pending-phone.png"),
        fullPage: true,
      });
      release();
      await continued;
      await page.waitForLoadState("load");
      expect(await page.evaluate(() => document.readyState)).toBe("complete");
      await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
      await expect(main.locator("[data-loading]")).toHaveCount(0);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      if (url !== "/")
        await expect(
          page.getByRole("searchbox", { name: "搜尋 Catalog", exact: true }),
        ).toHaveValue("pending search draft");
      expect(new URL(page.url()).pathname).toBe(url);
    } finally {
      release();
      await continued;
      await navigation;
    }
  });
}
