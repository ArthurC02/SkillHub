import { test, expect, type Page } from "@playwright/test";
import type { SearchCorrection, SearchFilters } from "../src/core/api/types";
import { emptySearchIntent } from "../src/core/api/searchIntent";
import { stubPlatform } from "./stub";

type SearchRequest = SearchCorrection & {
  query: string;
  filters: SearchFilters;
  limit: number;
};

async function stubIntentSearch(page: Page) {
  const requests: { method: string; body: SearchRequest }[] = [];
  await stubPlatform(page);
  await page.route("**/api/skills/search**", async (route, request) => {
    const method = request.method();
    const body: SearchRequest =
      method === "POST"
        ? request.postDataJSON()
        : {
            query: new URL(request.url()).searchParams.get("q") ?? "",
            intent: { ...emptySearchIntent(), input: "invoice" },
            keywords: ["invoice"],
            filters: { category: "documents" },
            limit: 20,
          };
    requests.push({ method, body });
    await route.fulfill({
      json: {
        query: body.query,
        interpretation: {
          status: method === "POST" ? "corrected" : "analyzed",
          intent: body.intent,
          keywords: body.keywords,
          filters: body.filters,
        },
        results: [],
        total: 0,
        limit: 20,
        truncated: false,
        degraded: false,
        partial_index: false,
        no_results: true,
        filtered_out: false,
        query_suggestion: `本次輸出需求：${body.intent.output ?? "未提及"}`,
      },
    });
  });
  return requests;
}

test("corrections survive reload and separate result caches while preserving the original task", async ({
  page,
}) => {
  const requests = await stubIntentSearch(page);
  await page.goto("/?q=原本的請求");
  const interpretation = page.getByRole("region", { name: "搜尋理解" });
  await expect(interpretation.locator("dd")).toHaveText([
    "invoice",
    "未提及",
    "未提及",
    "未提及",
    "未提及",
  ]);
  expect(requests.map((request) => request.method)).toEqual(["GET"]);
  const correction = {
    intent: { ...emptySearchIntent(), input: "invoice", output: "CSV" },
    keywords: ["invoice"],
  };
  await interpretation.getByRole("button", { name: "修正搜尋理解", exact: true }).click();
  await interpretation.getByLabel("輸出", { exact: true }).fill("CSV");
  await interpretation.getByRole("button", { name: "套用修正並搜尋" }).click();
  await expect(page.getByText("本次輸出需求：CSV", { exact: true })).toBeVisible();
  expect(requests.at(-1)).toEqual({
    method: "POST",
    body: {
      query: "原本的請求",
      ...correction,
      filters: { category: "documents" },
      limit: 20,
    },
  });
  await expect(page.getByLabel("任務描述", { exact: true })).toHaveValue("原本的請求");
  const beforeReload = requests.length;
  await page.reload();
  await expect(interpretation.locator("dd")).toHaveText([
    "invoice",
    "CSV",
    "未提及",
    "未提及",
    "未提及",
  ]);
  expect(requests).toHaveLength(beforeReload + 1);
  expect(requests.at(-1)).toEqual(requests.at(-2));
  for (const output of ["PDF", "CSV"]) {
    await interpretation.getByRole("button", { name: "修正搜尋理解", exact: true }).click();
    await interpretation.getByLabel("輸出", { exact: true }).fill(output);
    await interpretation.getByRole("button", { name: "套用修正並搜尋" }).click();
    await expect(page.getByText(`本次輸出需求：${output}`, { exact: true })).toBeVisible();
    await expect(interpretation.locator("dd").nth(1)).toHaveText(output);
  }
  expect(
    requests.some((request) => request.method === "POST" && request.body.intent.output === "PDF"),
  ).toBe(true);
  await interpretation.getByRole("button", { name: "捨棄理解與篩選，以原句搜尋" }).click();
  await expect(page.getByText("本次輸出需求：未提及", { exact: true })).toBeVisible();
  expect(requests.at(-1)).toEqual({
    method: "POST",
    body: {
      query: "原本的請求",
      intent: emptySearchIntent(),
      keywords: [],
      filters: {},
      limit: 20,
    },
  });
  expect(new URL(page.url()).searchParams.has("category")).toBe(false);
  await page.getByLabel("任務描述", { exact: true }).fill("新的任務");
  await page.getByRole("button", { name: "搜尋", exact: true }).click();
  await expect(page.locator("q")).toHaveText("新的任務");
  expect(requests.at(-1)?.method).toBe("GET");
  expect(new URL(page.url()).searchParams.has("correction")).toBe(false);
});

for (const correction of ["{", "7", "true", "[]", "{}", "null"]) {
  test(`invalid correction ${correction} is visible and makes no search request until replaced`, async ({
    page,
  }) => {
    const requests = await stubIntentSearch(page);
    await page.goto(`/?${new URLSearchParams({ q: "原本的請求", correction })}`);
    await expect(page.getByRole("alert")).toContainText("無法讀取搜尋結果");
    expect(requests).toHaveLength(0);
    await page.getByLabel("任務描述", { exact: true }).fill("新的任務");
    await page.getByRole("button", { name: "搜尋", exact: true }).click();
    await expect(page.locator("q")).toHaveText("新的任務");
    await expect(page.getByRole("alert")).toHaveCount(0);
    expect(requests.map((request) => request.method)).toEqual(["GET"]);
  });
}
