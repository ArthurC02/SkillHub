import { expect, test } from "@playwright/test";
import { SEARCH, SKILL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  test(`late model reasons keep the active comparison control visible at ${width}px`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const reason = "這個小工具會先檢查文件格式，再整理出可供比對的資訊。".repeat(30);
    await page.route("**/api/skills/search/reasons", async (route) => {
      await held;
      await route.fulfill({ json: { reasons: [{ skill_id: SKILL, reason, fits: true }] } });
    });
    try {
      await page.goto(`/?${new URLSearchParams({ q: SEARCH.query })}`);
      const rows = page.locator(".search-results > li");
      await expect(rows).toHaveCount(2);
      const comparison = rows.last().getByRole("checkbox");
      await comparison.focus();
      await page.keyboard.press("Space");
      await expect(comparison).toBeChecked();
      await expect(comparison).toBeFocused();
      await expect(comparison).toBeInViewport({ ratio: 1 });
      release();
      await expect(rows.first().locator(".match-reason")).toContainText(reason);
      await expect(comparison).toBeFocused();
      await page.screenshot({
        path: info.outputPath("late-reason-comparison.png"),
        fullPage: false,
      });
      await expect(comparison).toBeInViewport({ ratio: 1 });
    } finally {
      release();
    }
  });
}
