import { expect, test } from "vitest";
import type { SearchCorrection, SearchFilters } from "../../core/api/types";
import { emptySearchIntent } from "../../core/api/searchIntent";
import { queryClient } from "../../core/api/queryClient";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

type SearchRequest = SearchCorrection & {
  query: string;
  filters: SearchFilters;
  limit: number;
};

function stubIntentSearch(page: App) {
  const requests: { method: string; body: SearchRequest }[] = [];
  page.route(/\/api\/skills\/search(\?|$)/, ({ url, method, body: posted }) => {
    const body: SearchRequest =
      method === "POST"
        ? (posted as SearchRequest)
        : {
            query: url.searchParams.get("q") ?? "",
            intent: { ...emptySearchIntent(), input: "invoice" },
            keywords: ["invoice"],
            filters: { category: "documents" },
            limit: 20,
          };
    requests.push({ method, body });
    return {
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
    };
  });
  return requests;
}

function interpretationValues(page: App) {
  return page
    .getByRole("region", { name: "搜尋理解" })
    .locator("dd")
    .all()
    .map((node) => node.textContent);
}

async function correctOutput(page: App, output: string) {
  await page.getByRole("button", { name: "修正搜尋理解", exact: true }).click();
  await page.getByLabel("輸出", { exact: true }).fill(output);
  await page.getByRole("button", { name: "套用修正並搜尋" }).click();
  await expect(page.getByText(`本次輸出需求：${output}`, { exact: true })).toHaveCount(1);
}

test("corrections survive reload and separate result caches while preserving the original task", async () => {
  const page = app();
  const requests = stubIntentSearch(page);
  await page.goto("/?q=原本的請求");
  await page.eventually(() =>
    expect(interpretationValues(page)).toEqual(["invoice", "未提及", "未提及", "未提及", "未提及"]),
  );
  expect(requests.map((request) => request.method)).toEqual(["GET"]);
  const correction = {
    intent: { ...emptySearchIntent(), input: "invoice", output: "CSV" },
    keywords: ["invoice"],
  };
  await correctOutput(page, "CSV");
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
  queryClient.clear();
  await page.goto("/policy");
  await page.eventually(() => expect(page.pathname()).toBe("/policy"));
  await page.goBack();
  await page.eventually(() =>
    expect(interpretationValues(page)).toEqual(["invoice", "CSV", "未提及", "未提及", "未提及"]),
  );
  expect(requests).toHaveLength(beforeReload + 1);
  expect(requests.at(-1)).toEqual(requests.at(-2));
  for (const output of ["PDF", "CSV"]) {
    await correctOutput(page, output);
    await page.eventually(() => expect(interpretationValues(page)[1]).toBe(output));
  }
  expect(
    requests.some((request) => request.method === "POST" && request.body.intent.output === "PDF"),
  ).toBe(true);
  expect(new URLSearchParams(page.search()).get("category")).toBe("documents");
  await page.getByRole("button", { name: "捨棄理解與篩選，以原句搜尋" }).click();
  await expect(page.getByText("本次輸出需求：未提及", { exact: true })).toHaveCount(1);
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
  expect(new URLSearchParams(page.search()).has("category")).toBe(false);
  await page.getByLabel("任務描述", { exact: true }).fill("新的任務");
  await page.getByRole("button", { name: "搜尋", exact: true }).click();
  await expect(page.locator("q")).toHaveText("新的任務");
  expect(requests.at(-1)?.method).toBe("GET");
  expect(new URLSearchParams(page.search()).has("correction")).toBe(false);
});

for (const correction of ["{", "7", "true", "[]", "{}", "null"]) {
  test(`invalid correction ${correction} is visible and makes no search request until replaced`, async () => {
    const page = app();
    const requests = stubIntentSearch(page);
    await page.goto(`/?${new URLSearchParams({ q: "原本的請求", correction })}`);
    await expect(page.getByRole("alert")).toContainText("無法讀取搜尋結果");
    expect(requests).toHaveLength(0);
    await page.getByLabel("任務描述", { exact: true }).fill("新的任務");
    await page.getByRole("button", { name: "搜尋", exact: true }).click();
    await page.eventually(() => {
      expect(page.locator("q").text()).toBe("新的任務");
      expect(page.getByRole("alert").count()).toBe(0);
    });
    expect(requests.map((request) => request.method)).toEqual(["GET"]);
  });
}
