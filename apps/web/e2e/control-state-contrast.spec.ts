import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";
import { stubPlatform } from "./stub";

for (const colorScheme of ["light", "dark"] as const) {
  test(`a primary action remains readable while becoming enabled in ${colorScheme} mode`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.emulateMedia({ colorScheme });
    await page.setViewportSize({ width: 375, height: 900 });
    await page.goto("/lab/test-cases");
    await expect(page.getByRole("status").filter({ hasText: "已載入 1 個小工具" })).toBeVisible();
    const button = page.locator("#new-test-case button.action");
    await expect(button).toBeDisabled();
    await button.evaluate(
      async (element, fraction) => {
        const control = element as HTMLButtonElement;
        void getComputedStyle(control).backgroundColor;
        control.disabled = false;
        void getComputedStyle(control).backgroundColor;
        for (const animation of control.getAnimations()) {
          const duration = animation.effect?.getComputedTiming().duration;
          if (typeof duration === "number") {
            animation.pause();
            animation.currentTime = duration * fraction;
          }
        }
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      },
      colorScheme === "light" ? 0.25 : 0.5,
    );
    const result = await new AxeBuilder({ page }).include("#new-test-case button.action").analyze();
    const path = test.info().outputPath("midpoint-accessibility.json");
    await writeFile(path, JSON.stringify(result));
    await test.info().attach("midpoint-accessibility", { path, contentType: "application/json" });
    expect(result.violations).toEqual([]);
    expect(result.incomplete).toEqual([]);
    await page.screenshot({ path: test.info().outputPath("enabled-midpoint.png"), fullPage: true });
  });
}
