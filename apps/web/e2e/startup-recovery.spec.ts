import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const resource of ["entry", "dependency"] as const) {
  for (const dark of [false, true]) {
    test(`startup ${resource} failure offers keyboard recovery in ${dark ? "desktop dark" : "phone light"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: dark ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
      await stubPlatform(page);
      const path = `/lab/test-cases/${TEST_CASE}?version=${VERSION}`;
      const response = await page.request.get(path);
      expect(response.status()).toBe(200);
      const html = await response.text();
      const entry = html.match(/<script\b[^>]*type="module"[^>]*src="([^"]+)"/);
      expect(entry).not.toBeNull();
      const script = await page.request.get(entry![1]);
      expect(script.status()).toBe(200);
      const dependency = (await script.text()).match(/\bfrom["']([^"']+\.js)["']/);
      expect(dependency).not.toBeNull();
      const asset =
        resource === "entry"
          ? entry![1]
          : new URL(dependency![1], new URL(entry![1], response.url())).pathname;
      const blocked: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      let failing = true;
      await page.route("**/assets/*.js", (route) => {
        if (failing && new URL(route.request().url()).pathname === asset) {
          blocked.push(asset);
          return route.abort("connectionrefused");
        }
        return route.fallback();
      });
      await page.goto(path);
      await expect.poll(() => blocked.length).toBeGreaterThan(0);
      const recovery = page.getByRole("link", { name: "重新載入頁面", exact: true });
      await expect(recovery).toBeVisible();
      await expect(page.getByRole("status")).toHaveText("頁面尚未完成載入。請重新載入後再試。");
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      const waitingAccessibility = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("startup-waiting-a11y.json"),
        JSON.stringify(waitingAccessibility, null, 2),
      );
      expect(waitingAccessibility.violations).toEqual([]);
      expect(waitingAccessibility.incomplete).toEqual([]);
      await page.keyboard.press("Tab");
      await expect(recovery).toBeFocused();
      await page.screenshot({ path: test.info().outputPath("startup-recovery.png") });
      const firstBlocked = blocked.length;
      await page.keyboard.press("Enter");
      await expect.poll(() => blocked.length).toBeGreaterThan(firstBlocked);
      await expect(recovery).toBeVisible();
      expect(new URL(page.url()).pathname + new URL(page.url()).search).toBe(path);
      failing = false;
      await recovery.focus();
      await page.keyboard.press("Enter");
      await expect(
        page
          .locator(".test-case-history-panel")
          .getByRole("heading", { name: "執行歷史", exact: true }),
      ).toBeVisible();
      await expect(page.locator(".test-case-detail-layout")).toHaveCSS("display", "grid");
      expect(new URL(page.url()).pathname + new URL(page.url()).search).toBe(path);
      await expect(recovery).toHaveCount(0);
      expect(new Set(blocked)).toEqual(new Set([asset]));
      expect(writes).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      const accessibility = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("startup-recovered-a11y.json"),
        JSON.stringify(accessibility, null, 2),
      );
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
    });
  }
}

for (const dark of [false, true]) {
  test(`account page module failure reloads the same task in ${dark ? "desktop dark" : "phone light"}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: dark ? 1280 : 375, height: 900 });
    await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
    await stubPlatform(page);
    const path = "/admin/accounts";
    let failing = true;
    const blocked: string[] = [];
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() !== "GET") writes.push(request.url());
    });
    await page.route("**/assets/AdminAccounts.page-*.js", (route) => {
      if (failing) {
        blocked.push(route.request().url());
        return route.fulfill({ status: 503, body: "" });
      }
      return route.fallback();
    });
    await page.goto(path);
    await expect(
      page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }),
    ).toBeVisible();
    expect(blocked.length).toBeGreaterThan(0);
    failing = false;
    await page.getByRole("button", { name: "重新載入頁面", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(page.getByLabel("Email", { exact: true })).toBeVisible();
    const url = new URL(page.url());
    expect(url.pathname + url.search + url.hash).toBe(path);
    await expect(
      page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }),
    ).toHaveCount(0);
    expect(writes).toEqual([]);
    const recovered = await new AxeBuilder({ page }).analyze();
    await writeFile(
      test.info().outputPath("account-module-recovered-a11y.json"),
      JSON.stringify(recovered, null, 2),
    );
    expect(recovered.violations).toEqual([]);
    expect(recovered.incomplete).toEqual([]);
    await page.screenshot({ path: test.info().outputPath("account-module-recovered.png") });
  });
}
