import { expect, test } from "@playwright/test";
import { stubPlatform } from "./stub";

test("search failure alert and retry sit inside the first phone screen", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let failing = false;
  await page.route(/\/api\/skills\/search(\?|$)/, (route) =>
    failing
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fallback(),
  );
  await page.goto("/?q=pdf");
  const main = page.getByRole("main");
  await expect(main.locator(".search-results > li").first()).toBeVisible();
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
  failing = true;
  await page.goBack();
  await expect(main.getByRole("alert")).toContainText("暫時無法讀取搜尋結果");
  await page.evaluate(() => scrollTo(0, 0));
  await expect(main.getByRole("alert")).toBeInViewport({ ratio: 1 });
  await expect(main.getByRole("button", { name: "再試一次", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("catalog failure alert and retry sit inside the first phone screen", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/api/skills/catalog?*", (route) => {
    if (new URL(route.request().url()).searchParams.get("limit") !== "100") return route.fallback();
    reads += 1;
    return reads === 1
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fallback();
  });
  await page.goto("/?category=data");
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取目錄");
  await page.evaluate(() => scrollTo(0, 0));
  await expect(page.getByRole("alert")).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeInViewport({
    ratio: 1,
  });
});
