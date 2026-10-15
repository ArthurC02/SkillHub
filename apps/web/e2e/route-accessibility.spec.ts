import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";
import { ROUTES } from "./routes";
import { stubPlatform } from "./stub";

for (const [name, url] of ROUTES) {
  for (const view of [
    { name: "desktop-light", width: 1280, colorScheme: "light" },
    { name: "phone-dark", width: 375, colorScheme: "dark" },
  ] as const) {
    test(`${name} has accessible rendered content in ${view.name}`, async ({ page }) => {
      await stubPlatform(page);
      await page.emulateMedia({ colorScheme: view.colorScheme, reducedMotion: "reduce" });
      await page.setViewportSize({ width: view.width, height: 900 });
      await page.goto(url);
      await expect(page.locator("main h1")).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      await page.evaluate(() => document.fonts.ready);
      const results = await new AxeBuilder({ page }).analyze();
      const report = test.info().outputPath("accessibility-results.json");
      await writeFile(report, JSON.stringify(results));
      await test.info().attach("accessibility-results", {
        path: report,
        contentType: "application/json",
      });
      if (results.incomplete.some(({ id }) => id === "color-contrast")) {
        const screenshot = test.info().outputPath("accessibility-review.png");
        await page.screenshot({ path: screenshot, fullPage: true });
        await test.info().attach("accessibility-review", {
          path: screenshot,
          contentType: "image/png",
        });
      }
      expect(await page.getByRole("group", { name: "目前 Workspace", exact: true }).count()).toBe(
        1,
      );
      expect(
        results.violations.map(({ id, nodes }) => ({
          id,
          nodes: nodes.map(({ target, failureSummary }) => ({ target, failureSummary })),
        })),
      ).toEqual([]);
    });
  }
}
