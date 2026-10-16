import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

async function scrollKey(page: Page, region: Locator, key: "ArrowLeft" | "ArrowRight") {
  await region.evaluate((element) => {
    Reflect.set(
      element,
      "ledgerScrollEnd",
      new Promise<void>((resolve) => {
        element.addEventListener("scrollend", () => resolve(), { once: true });
      }),
    );
  });
  await page.keyboard.press(key);
  await region.evaluate((element) => Reflect.get(element, "ledgerScrollEnd"));
}

test.afterEach(async ({ page }) => {
  if (test.info().status === test.info().expectedStatus || page.isClosed()) return;
  const state = await page.evaluate(() => {
    const region = document.querySelector(".credit-ledger-table")?.parentElement;
    return {
      focus: {
        tag: document.activeElement?.tagName,
        id: document.activeElement?.id,
        role: document.activeElement?.getAttribute("role"),
      },
      scrollLeft: region?.scrollLeft,
      width: region?.clientWidth,
      scrollWidth: region?.scrollWidth,
      visibility: document.visibilityState,
      documentFocus: document.hasFocus(),
    };
  });
  await writeFile(test.info().outputPath("ledger-keyboard-state.json"), JSON.stringify(state));
});

for (const width of [320, 375]) {
  for (const colorScheme of ["light", "dark"] as const) {
    test(`credit ledger columns stay readable at ${width}px in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 812 });
      await page.emulateMedia({ colorScheme });
      await stubPlatform(page);
      const writes: string[] = [];
      page.on("request", (request) => {
        if (
          request.method() === "POST" &&
          new URL(request.url()).pathname.startsWith("/admin/credits/")
        ) {
          writes.push(request.url());
        }
      });
      await page.goto("/admin/accounts");
      await expect(page.getByRole("heading", { level: 1, name: "帳號與點數" })).toBeVisible();
      await page.getByLabel("Email", { exact: true }).fill("member@example.com");
      await page.getByRole("button", { name: "查詢", exact: true }).click();
      const table = page.getByRole("table", { name: "最近 50 筆分錄，新的在上面", exact: true });
      const region = table.locator("..");
      const source = table.getByRole("cell", { name: "不適用", exact: true });
      await expect(source).toBeVisible();
      const layout = await source.evaluate((cell) => {
        const range = document.createRange();
        range.selectNodeContents(cell);
        return { lines: range.getClientRects().length, width: cell.getBoundingClientRect().width };
      });
      await writeFile(test.info().outputPath("source-layout.json"), JSON.stringify(layout));
      expect(layout.lines).toBe(1);
      await expect(region).toHaveAttribute("role", "region");
      await expect(region).toHaveAccessibleName("點數分錄，可左右捲動");
      await expect(page.getByText("左右捲動查看全部欄位。", { exact: true })).toBeVisible();
      expect(await region.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
        true,
      );
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      await page.getByRole("button", { name: "重新整理點數", exact: true }).focus();
      await page.keyboard.press("Tab");
      await expect(region).toBeFocused();
      await scrollKey(page, region, "ArrowRight");
      await expect.poll(() => region.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
      const right = await region.evaluate((element) => element.scrollLeft);
      await scrollKey(page, region, "ArrowLeft");
      await expect.poll(() => region.evaluate((element) => element.scrollLeft)).toBeLessThan(right);
      await region.evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
      });
      await expect
        .poll(() =>
          source.evaluate((cell) => {
            const bounds = cell.closest(".table-scroll")!.getBoundingClientRect();
            const range = document.createRange();
            range.selectNodeContents(cell);
            const text = range.getBoundingClientRect();
            return text.left >= bounds.left && text.right <= bounds.right;
          }),
        )
        .toBe(true);
      await page.screenshot({ path: test.info().outputPath("ledger-source-viewport.png") });
      await page.setViewportSize({ width: 1280, height: 900 });
      await expect(page.getByText("左右捲動查看全部欄位。", { exact: true })).toBeHidden();
      await expect
        .poll(() => region.evaluate((element) => element.scrollWidth === element.clientWidth))
        .toBe(true);
      await page.evaluate(() => window.scrollTo(0, 0));
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(test.info().outputPath("ledger-desktop-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(writes).toEqual([]);
    });
  }
}
