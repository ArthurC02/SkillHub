import { expect, test } from "@playwright/test";
import { ROUTES } from "./routes";
import { stubPlatform } from "./stub";

const OUT = process.env.SHOT_DIR;

test.skip(!OUT, "set SHOT_DIR to write screenshots");

const VIEWS: [suffix: string, width: number, dark: boolean][] = [
  ["desktop-light", 1280, false],
  ["desktop-dark", 1280, true],
  ["mobile-light", 375, false],
  ["mobile-dark", 375, true],
];

for (const [name, url] of ROUTES) {
  for (const [suffix, width, dark] of VIEWS) {
    test(`shot ${name} ${suffix}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
      await page.setViewportSize({ width, height: 900 });
      await stubPlatform(page);
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      await expect(page.locator("main h1")).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      await page.evaluate(() => document.fonts.ready);
      await page.screenshot({ path: `${OUT}/${name}--${suffix}.png`, fullPage: true });
    });
  }
}
