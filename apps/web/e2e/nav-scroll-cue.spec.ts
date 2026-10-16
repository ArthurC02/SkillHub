import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { SKILL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const colorScheme of ["light", "dark"] as const) {
  for (const [name, path, selector] of [
    ["main", "/activity", ".app-nav"],
    ["skill", `/skills/${SKILL}`, ".category-nav"],
  ]) {
    test(`${name} navigation keeps its overflow cue readable in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await page.setViewportSize({ width: 320, height: 812 });
      await stubPlatform(page);
      await page.goto(path);
      const nav = page.locator(selector);
      const cue = nav.locator(":scope > .nav-scroll-cue");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      await expect(cue).toBeVisible();
      const report = await new AxeBuilder({ page }).include(selector).analyze();
      await writeFile(test.info().outputPath("navigation-a11y.json"), JSON.stringify(report));
      await page.screenshot({ path: test.info().outputPath("navigation-viewport.png") });
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await nav.locator("a").first().focus();
      const links = await nav.locator("a").count();
      for (let index = 1; index < links; index++) {
        await page.keyboard.press("Tab");
      }
      await expect(nav.locator("a").last()).toBeFocused();
      await expect(cue).toBeHidden();
      await page.screenshot({ path: test.info().outputPath("focused-navigation-viewport.png") });
      for (let index = 1; index < links; index++) {
        await page.keyboard.press("Shift+Tab");
      }
      await expect(nav.locator("a").first()).toBeFocused();
      await expect(cue).toBeVisible();
      await page.setViewportSize({ width: 640, height: 812 });
      await expect(cue).toBeHidden();
      await page.setViewportSize({ width: 320, height: 812 });
      await expect(cue).toBeVisible();
      await nav.evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
      });
      await expect(cue).toBeHidden();
    });
  }
}
