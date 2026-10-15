import { expect, test } from "@playwright/test";
import { stubPlatform } from "./stub";

for (const destination of [
  { label: "工作台", path: "/workspace", scope: "nav.app-nav" },
  { label: "資料保存政策", path: "/policy", scope: ".app-footer" },
  { label: "帳號", path: "/workspace/account", scope: "main" },
]) {
  test(`keyboard navigation to ${destination.path} starts at the new content and browser back restores a content starting point`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.goto("/library");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(main).not.toBeFocused();
    const link = page
      .locator(destination.scope)
      .getByRole("link", { name: destination.label, exact: true });
    await link.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`${destination.path}$`));
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(main).toBeFocused();
    await page.screenshot({ path: test.info().outputPath("navigation-focus.png"), fullPage: true });
    await page.goBack();
    await expect(page).toHaveURL(/\/library$/);
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(main).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(main.locator(":focus")).toHaveCount(1);
  });
}
