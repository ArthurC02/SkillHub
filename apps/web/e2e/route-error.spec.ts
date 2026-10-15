import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ME } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [failure, recovery] of [
  ["module evaluation", "reload"],
  ["page render", "reload"],
  ["unavailable module", "reload"],
  ["page render", "navigation"],
] as const) {
  test(`${failure} offers readable ${recovery} recovery without showing technical error details`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await stubPlatform(page);
    await page.route("**/me", (route) => route.fulfill({ json: { ...ME, operator: true } }));
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() === "POST") writes.push(request.url());
    });
    let documents = 0;
    page.on("domcontentloaded", () => documents++);
    await page.route("**/assets/AdminAccounts.page-*.js", (route) => {
      if (failure === "unavailable module") return route.fulfill({ status: 503, body: "" });
      const body =
        failure === "module evaluation"
          ? 'throw new Error("route-private-error-detail"); export const AdminAccounts = () => null;'
          : 'export function AdminAccounts() { throw new Error("route-private-error-detail"); }';
      return route.fulfill({ contentType: "text/javascript", body });
    });
    const url = "/admin/accounts#main-content";
    await page.goto(url);
    const main = page.getByRole("main");
    await expect(
      main.getByRole("heading", { level: 1, name: "這一頁暫時無法顯示", exact: true }),
    ).toBeVisible();
    await expect(main.getByRole("alert")).toContainText("載入或顯示頁面時發生問題");
    const reload = main.getByRole("button", { name: "重新載入頁面", exact: true });
    await expect(reload).toHaveAccessibleDescription("重新載入會清除尚未送出的內容。");
    await expect(page.locator("body")).not.toContainText("route-private-error-detail");
    await expect(page.locator("body")).not.toContainText("/assets/AdminAccounts.page-");
    await expect(page.getByRole("button", { name: "Show Error", exact: true })).toHaveCount(0);
    await expect(page.locator(".app-nav a").first()).toBeVisible();
    expect(writes).toEqual([]);
    const report = await new AxeBuilder({ page }).analyze();
    await writeFile(test.info().outputPath("route-error-a11y.json"), JSON.stringify(report));
    expect(report.violations).toEqual([]);
    expect(report.incomplete).toEqual([]);
    await page.screenshot({
      path: test.info().outputPath("route-error-phone.png"),
      fullPage: true,
    });
    if (recovery === "navigation") {
      const search = page.getByRole("searchbox", { name: "搜尋 Catalog", exact: true });
      await search.fill("preserve the header draft");
      await page.getByRole("link", { name: "資產庫", exact: true }).click();
      await expect(
        main.getByRole("heading", { level: 1, name: "資產庫", exact: true }),
      ).toBeVisible();
      await expect(search).toHaveValue("preserve the header draft");
      expect(new URL(page.url()).pathname).toBe("/library");
      expect(writes).toEqual([]);
      return;
    }
    const loadedDocuments = documents;
    await reload.focus();
    await page.keyboard.press("Enter");
    await expect.poll(() => documents).toBeGreaterThan(loadedDocuments);
    await expect(
      main.getByRole("heading", { level: 1, name: "這一頁暫時無法顯示", exact: true }),
    ).toBeVisible();
    expect(new URL(page.url()).pathname + new URL(page.url()).hash).toBe(url);
    await expect(main.getByRole("alert")).toContainText("載入或顯示頁面時發生問題");
    expect(writes).toEqual([]);
  });
}
