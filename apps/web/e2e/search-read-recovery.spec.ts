import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { SEARCH, SKILL, SKILL_B } from "../src/testing/fixtures/platform";
import { emptySearchIntent } from "../src/core/api/searchIntent";
import { stubPlatform } from "./stub";

for (const corrected of [false, true]) {
  for (const empty of [false, true]) {
    test(`${corrected ? "corrected" : "ordinary"} search withdraws cached ${empty ? "absence" : "results"} and interpretation, then retries the same request without clearing input`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let failing = false;
      const requests: { method: string; query: string; body: unknown }[] = [];
      await page.route(/\/api\/skills\/search(\?|$)/, (route) => {
        const request = route.request();
        requests.push({
          method: request.method(),
          query: new URL(request.url()).search,
          body: request.method() === "POST" ? request.postDataJSON() : null,
        });
        return failing
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({
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
            });
      });
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
      const main = page.getByRole("main");
      await expect(main.getByRole("region", { name: "搜尋理解", exact: true })).toBeVisible();
      if (empty)
        await expect(main.getByText("沒有夠接近的小工具。", { exact: true })).toBeVisible();
      else await expect(main.locator(".search-results > li")).toHaveCount(2);
      const original = requests.at(-1);
      const originalURL = page.url();
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
      await page.screenshot({
        path: test.info().outputPath("search-failure-first-screen.png"),
        fullPage: false,
      });
      await expect(main.getByRole("region", { name: "搜尋理解", exact: true })).toHaveCount(0);
      await expect(main.locator(".search-results > li")).toHaveCount(0);
      await expect(main.getByText("沒有夠接近的小工具。", { exact: true })).toHaveCount(0);
      await expect(main.getByRole("button", { name: "修正搜尋理解", exact: true })).toHaveCount(0);
      await expect(
        main.getByRole("combobox", { name: "是否包含 Script", exact: true }),
      ).toHaveValue("");
      await main.getByLabel("任務描述", { exact: true }).fill("尚未送出的搜尋草稿");
      const before = requests.length;
      failing = false;
      await main.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(main.getByRole("region", { name: "搜尋理解", exact: true })).toBeVisible();
      await expect(
        main.getByRole("combobox", { name: "是否包含 Script", exact: true }),
      ).toHaveValue("yes");
      await expect(main.getByLabel("任務描述", { exact: true })).toHaveValue("尚未送出的搜尋草稿");
      await expect(page.locator("#catalog-search-heading")).toBeFocused();
      expect(requests.length).toBe(before + 1);
      expect(requests.at(-1)).toEqual(original);
      expect(page.url()).toBe(originalURL);
      expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({
        path: test.info().outputPath("search-recovered-phone.png"),
        fullPage: false,
      });
    });
  }
}

test("catalog failure retries its read without resubmitting the unsubmitted search draft", async ({
  page,
}) => {
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
  await page.getByLabel("任務描述", { exact: true }).fill("保留這段任務");
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator(".catalog-gallery > li")).toHaveCount(1);
  await expect(page.getByLabel("任務描述", { exact: true })).toHaveValue("保留這段任務");
  await expect(page.locator("#catalog-search-heading")).toBeFocused();
  expect(reads).toBe(2);
  expect(new URL(page.url()).searchParams.get("category")).toBe("data");
});
