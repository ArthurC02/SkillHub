import { expect, test } from "vitest";
import { SEARCH, SKILL, SKILL_B } from "../../../testing/fixtures/platform";
import { emptySearchIntent } from "../../../core/api/searchIntent";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const corrected of [false, true]) {
  for (const empty of [false, true]) {
    test(`${corrected ? "corrected" : "ordinary"} search withdraws cached ${empty ? "absence" : "results"} and interpretation, then retries the same request without clearing input`, async () => {
      const page = app();
      let failing = false;
      page.route(/\/api\/skills\/search(\?|$)/, () =>
        failing
          ? { status: 503, json: { error: "service unavailable" } }
          : {
              json: {
                ...SEARCH,
                results: empty ? [] : SEARCH.results,
                total: empty ? 0 : 2,
                no_results: empty,
                interpretation: {
                  status: corrected ? "corrected" : "analyzed",
                  intent: { ...emptySearchIntent(), input: "PDF" },
                  keywords: ["PDF"],
                  filters: { category: "data", script: "yes" },
                },
              },
            },
      );
      const searches = () =>
        page.requests
          .filter((request) => request.url.pathname === "/api/skills/search")
          .map((request) => ({
            method: request.method,
            query: request.url.search,
            body: request.method === "POST" ? request.body : null,
          }));
      const params = new URLSearchParams({
        q: "pdf 摘要",
        category: "data",
        compare: `${SKILL},${SKILL_B}`,
      });
      if (corrected)
        params.set(
          "correction",
          JSON.stringify({ intent: { ...emptySearchIntent(), input: "PDF" }, keywords: ["PDF"] }),
        );
      await page.goto(`/?${params}`);
      const main = page.locator("main");
      const understanding = main.getByRole("region", { name: "搜尋理解", exact: true });
      const rows = main.locator(".search-results > li");
      const noMatch = main.getByText("沒有夠接近的小工具。", { exact: true });
      await page.eventually(() => {
        expect(understanding.count()).toBe(1);
        if (empty) expect(noMatch.count()).toBe(1);
        else expect(rows.count()).toBe(2);
      });
      const original = searches().at(-1);
      const originalURL = page.pathname() + page.search();
      await page.locator('a[href="/policy"]').first().click();
      await page.eventually(() =>
        expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1),
      );
      failing = true;
      await page.goBack();
      const scriptFilter = main.getByRole("combobox", { name: "是否包含 Script" });
      await page.eventually(() => {
        expect(main.getByRole("alert").text()).toContain("暫時無法讀取搜尋結果");
        expect(main.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(1);
        expect(understanding.count()).toBe(0);
        expect(rows.count()).toBe(0);
        expect(noMatch.count()).toBe(0);
        expect(main.getByRole("button", { name: "修正搜尋理解", exact: true }).count()).toBe(0);
        expect(scriptFilter.value()).toBe("");
      });
      await page.getByLabel("任務描述", { exact: true }).fill("尚未送出的搜尋草稿");
      const before = searches().length;
      failing = false;
      await main.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(understanding.count()).toBe(1);
        expect(scriptFilter.value()).toBe("yes");
        expect(page.getByLabel("任務描述", { exact: true }).value()).toBe("尚未送出的搜尋草稿");
        expect(page.locator("#catalog-search-heading").isFocused()).toBe(true);
      });
      expect(searches()).toHaveLength(before + 1);
      expect(searches().at(-1)).toEqual(original);
      expect(page.pathname() + page.search()).toBe(originalURL);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}

test("catalog failure retries its read without resubmitting the unsubmitted search draft", async () => {
  const page = app();
  let reads = 0;
  page.route("**/api/skills/catalog?*", ({ url }) => {
    if (url.searchParams.get("limit") !== "100") return undefined;
    reads += 1;
    return reads === 1 ? { status: 503, json: { error: "service unavailable" } } : undefined;
  });
  await page.goto("/?category=data");
  await page.eventually(() => expect(page.getByRole("alert").text()).toContain("暫時無法讀取目錄"));
  await page.getByLabel("任務描述", { exact: true }).fill("保留這段任務");
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await page.eventually(() => {
    expect(page.locator(".catalog-gallery > li").count()).toBe(1);
    expect(page.getByLabel("任務描述", { exact: true }).value()).toBe("保留這段任務");
    expect(page.locator("#catalog-search-heading").isFocused()).toBe(true);
  });
  expect(reads).toBe(2);
  expect(new URLSearchParams(page.search()).get("category")).toBe("data");
});
