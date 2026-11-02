import { expect, test } from "vitest";
import { SEARCH, SKILL, SKILL_B } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const surface of ["catalog", "search"] as const) {
  for (const outcome of ["populated", "empty", "error"] as const) {
    test(`${surface} ${outcome} retains one comparison entry and can clear invisible selections`, async () => {
      const page = app();
      let reads = 0;
      page.route(
        (url) =>
          surface === "catalog"
            ? url.pathname === "/api/skills/catalog" && url.searchParams.get("limit") === "100"
            : url.pathname === "/api/skills/search",
        () => {
          reads += 1;
          if (outcome === "populated") return undefined;
          if (outcome === "error") return { status: 503, json: { error: "service unavailable" } };
          return {
            json:
              surface === "catalog"
                ? { results: [], total: 0, limit: 100, truncated: false }
                : { ...SEARCH, results: [], total: 0, no_results: true, filtered_out: false },
          };
        },
      );
      const params = new URLSearchParams({ compare: `${SKILL},${SKILL_B}`, category: "data" });
      if (surface === "search") params.set("q", "pdf 摘要");
      await page.goto(`/?${params}`);
      if (outcome === "error") await expect(page.getByRole("alert")).toContainText("暫時無法讀取");
      else if (outcome === "empty")
        await expect(
          page.getByText(
            surface === "search" ? "沒有夠接近的小工具。" : "沒有小工具符合目前的篩選條件",
          ),
        ).toBeVisible();
      else await expect(page.locator(".search-results > li").first()).toBeVisible();
      const comparison = page.getByRole("link", { name: "並排比較這 2 個小工具", exact: true });
      await expect(comparison).toHaveCount(1);
      await expect(comparison).toBeVisible();
      const target = new URL(comparison.attribute("href") ?? "", "http://127.0.0.1");
      expect(target.pathname).toBe("/compare");
      expect(target.searchParams.get("ids")).toBe(`${SKILL},${SKILL_B}`);
      const before = reads;
      await page.getByRole("link", { name: "清除比較選擇", exact: true }).click();
      await expect(page.locator(".compare-bar")).toHaveCount(0);
      const current = new URLSearchParams(page.search());
      expect(current.has("compare")).toBe(false);
      expect(current.get("category")).toBe("data");
      expect(current.get("q")).toBe(surface === "search" ? "pdf 摘要" : null);
      expect(reads).toBe(before);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}
