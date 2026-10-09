import { expect, test } from "@playwright/test";
import { ADMIN_AUDIT_LOG } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("the audit log continues on a stable cursor without repeating the first page", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  const cursor = "2026-09-10T08:00:00Z_50";
  await page.route(/\/admin\/audit-log\?/, (route) => {
    const url = new URL(route.request().url());
    expect(url.searchParams.get("limit")).toBe("50");
    if (url.searchParams.has("cursor")) {
      expect(url.searchParams.get("cursor")).toBe(cursor);
      expect(url.searchParams.has("offset")).toBe(false);
      return route.fulfill({ json: { events: [ADMIN_AUDIT_LOG.events[1]] } });
    }
    return route.fulfill({
      json: {
        events: Array.from({ length: 50 }, (_, index) => ({
          ...ADMIN_AUDIT_LOG.events[0],
          resource_id: `00000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
        })),
        next_cursor: cursor,
      },
    });
  });

  await page.goto("/admin/audit-log");
  await expect(page.locator("tbody tr")).toHaveCount(50);
  await page.getByRole("button", { name: "載入更多" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(51);
  await expect(page.getByRole("button", { name: "載入更多" })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
