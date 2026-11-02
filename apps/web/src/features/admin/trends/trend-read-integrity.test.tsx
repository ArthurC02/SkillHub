import { expect, test } from "vitest";
import { platformResponse } from "../../../testing/fixtures/platform";
import { Locator, renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const kind of ["cost", "journeys"] as const) {
  test(`an incomplete ${kind} trend withholds its conclusion and recovers`, async () => {
    const page = app();
    let failing = false;
    let reads = 0;
    page.route("**/admin/trends/*?*", ({ url }) => {
      const body = platformResponse(url.pathname + url.search).body as {
        buckets: Record<string, unknown>[];
        period?: Record<string, unknown>;
      };
      if (url.pathname === `/admin/trends/${kind}`) {
        reads++;
        if (failing)
          return {
            json:
              kind === "cost"
                ? { ...body, buckets: body.buckets.map((bucket) => ({ ...bucket, total: null })) }
                : { ...body, period: { ...body.period, median_minutes: null } },
          };
      }
      return { json: body };
    });
    await page.goto("/admin/trends?days=7");
    const overview = page.getByRole("region", { name: "本期概覽", exact: true });
    const section = () =>
      new Locator(() => {
        const owner = document.getElementById(`admin-trend-${kind}`)?.parentElement;
        return owner ? [owner] : [];
      });
    await page.eventually(() => {
      expect(overview.count()).toBe(1);
      expect(section().locator("canvas").count()).toBe(kind === "cost" ? 2 : 3);
    });
    failing = true;
    await page.getByRole("button", { name: "重新整理六組趨勢", exact: true }).click();
    await page.eventually(() => {
      expect(section().getByRole("alert").text()).toContain("暫時無法讀取");
      expect(section().locator("canvas, table").count()).toBe(0);
      expect(overview.count()).toBe(kind === "cost" ? 0 : 1);
      expect(page.locator("main").text()).toContain("已取得 5/6 組");
    });
    expect(reads).toBe(2);
    expect(await page.axeViolations("main")).toEqual([]);
    failing = false;
    await page.getByRole("button", { name: "重新整理六組趨勢", exact: true }).click();
    await page.eventually(() => {
      expect(section().getByRole("alert").count()).toBe(0);
      expect(overview.count()).toBe(1);
      expect(page.locator("main").text()).toContain("已取得 6/6 組");
    });
    expect(page.pathname() + page.search()).toBe("/admin/trends?days=7");
    expect(reads).toBe(3);
    expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
