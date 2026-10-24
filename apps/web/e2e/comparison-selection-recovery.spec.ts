import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { SEARCH, SKILL, SKILL_B } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const surface of ["catalog", "search"]) {
  for (const outcome of ["populated", "empty", "error"]) {
    test(`${surface} ${outcome} retains one comparison entry and can clear invisible selections`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let reads = 0;
      const pattern =
        surface === "catalog" ? "**/api/skills/catalog?*" : /\/api\/skills\/search(\?|$)/;
      await page.route(pattern, (route) => {
        if (
          surface === "catalog" &&
          new URL(route.request().url()).searchParams.get("limit") !== "100"
        )
          return route.fallback();
        reads += 1;
        if (outcome === "populated") return route.fallback();
        return outcome === "error"
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({
              json:
                surface === "catalog"
                  ? { results: [], total: 0, limit: 100, truncated: false }
                  : { ...SEARCH, results: [], total: 0, no_results: true, filtered_out: false },
            });
      });
      const params = new URLSearchParams({ compare: `${SKILL},${SKILL_B}`, category: "data" });
      if (surface === "search") params.set("q", "pdf 摘要");
      await page.goto(`/?${params}`);
      if (outcome === "error") await expect(page.getByRole("alert")).toContainText("暫時無法讀取");
      else if (outcome === "empty")
        await expect(
          page.getByText(
            surface === "search" ? "沒有夠接近的小工具。" : "沒有小工具符合目前的篩選條件",
            { exact: false },
          ),
        ).toBeVisible();
      else await expect(page.locator(".search-results > li").first()).toBeVisible();
      const comparison = page.getByRole("link", { name: "並排比較這 2 個小工具", exact: true });
      await expect(comparison).toHaveCount(1);
      await expect(comparison).toBeVisible();
      const target = new URL((await comparison.getAttribute("href")) ?? "", page.url());
      expect(target.pathname).toBe("/compare");
      expect(target.searchParams.get("ids")).toBe(`${SKILL},${SKILL_B}`);
      await page.screenshot({
        path: test.info().outputPath("comparison-selection-phone.png"),
        fullPage: false,
      });
      const before = reads;
      await page.getByRole("link", { name: "清除比較選擇", exact: true }).click();
      await expect(page.locator(".compare-bar")).toHaveCount(0);
      const current = new URL(page.url());
      expect(current.searchParams.has("compare")).toBe(false);
      expect(current.searchParams.get("category")).toBe("data");
      expect(current.searchParams.get("q")).toBe(surface === "search" ? "pdf 摘要" : null);
      expect(reads).toBe(before);
      expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    });
  }
}
