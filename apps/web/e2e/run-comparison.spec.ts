import { expect, test } from "@playwright/test";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  SKILL,
  TEST_CASE,
  VERSION,
  comparisonSide,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [1280, 375]) {
  test(`long Run outputs stay within their own comparison cells at ${width}px`, async ({
    page,
  }) => {
    await stubPlatform(page);
    const outputs = [
      "請提供要輸出的單字，以及要套用的技能名稱；我會依該技能規則直接輸出。".repeat(3),
      `first line\n${"unbroken-output-".repeat(40)}\nlast line`,
    ];
    await page.route(`**/runs/${RUN}/comparison?*`, (route) =>
      route.fulfill({
        json: {
          ...COMPARISON,
          runs: COMPARISON.runs.map((side, i) => ({ ...side, final_output: outputs[i] })),
        },
      }),
    );
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
    const output = page
      .getByRole("table", { name: "Run 任務判定與執行狀態對比" })
      .getByRole("row")
      .filter({ hasText: "最終輸出" })
      .locator("pre");
    await expect(output).toHaveText(outputs);
    for (const pre of await output.all()) {
      const bounds = await pre.evaluate((el) => {
        const range = document.createRange();
        range.selectNodeContents(el);
        const cell = el.closest("td")!.getBoundingClientRect();
        return [...range.getClientRects()].every(
          (rect) => rect.left >= cell.left && rect.right <= cell.right,
        );
      });
      expect(bounds, "output text painted outside its comparison cell").toBe(true);
    }
  });
}

test("populated Run results keep the output and task verdict visible", async ({ page }) => {
  await stubPlatform(page);
  await page.goto(`/runs/${RUN}`);
  await expect(page.getByRole("heading", { name: "Run 結果", exact: true })).toBeVisible();
  await expect(page.getByText("Removed 17 duplicate rows.", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "任務判定", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "與另一個 Run 比較" }).click();
  await expect(page.getByRole("heading", { name: "Run 比較", exact: true })).toBeVisible();
});

test("comparison aligns distinct outputs, verdicts, costs and version links", async ({ page }) => {
  await stubPlatform(page);
  const left = comparisonSide(RUN, true);
  const right = comparisonSide(OTHER_RUN, true);
  await page.route(`**/runs/${RUN}/comparison?*`, (route) =>
    route.fulfill({
      json: {
        ...COMPARISON,
        runs: [
          { ...left, final_output: "ALPHA", errors: [], duration_ms: 12000 },
          {
            ...right,
            skill_version_id: "second-version",
            final_output: "BETA",
            errors: [],
            duration_ms: 15000,
            cost: { ...right.cost, credits: 0 },
            evaluation: {
              ...right.evaluation,
              overall: "met",
              cost: { evaluation_credits: 4, source: "gateway", note: "" },
            },
          },
        ],
        criterion_matrix: [
          {
            criterion_id: "c1",
            text: "最終輸出必須恰好為 BETA。",
            results: [
              { run_id: RUN, result: "failed", source: "model" },
              { run_id: OTHER_RUN, result: "passed", source: "model" },
            ],
          },
        ],
      },
    }),
  );
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const table = page.getByRole("table", { name: "Run 任務判定與執行狀態對比" });
  const cells = (name: string) =>
    table
      .getByRole("row")
      .filter({ has: page.getByRole("rowheader", { name, exact: true }) })
      .getByRole("cell");
  await expect(cells("任務判定")).toHaveText(["未符合", "符合"]);
  await expect(cells("最終輸出")).toHaveText(["ALPHA", "BETA"]);
  await expect(cells("延遲")).toHaveText(["12000 毫秒", "15000 毫秒"]);
  await expect(
    table.getByRole("row").filter({ hasText: "Run 用掉的點數" }).getByRole("cell"),
  ).toHaveText(["169 點", "0 點"]);
  await expect(
    table.getByRole("row").filter({ hasText: "評估用掉的點數" }).getByRole("cell"),
  ).toHaveText(["26 點", "4 點"]);
  await expect(
    page.getByRole("table", { name: "驗收條件判定矩陣對比" }).getByRole("cell"),
  ).toHaveText(["未通過模型評估", "通過模型評估"]);
  await expect(page.getByText("@@ -1 +1 @@\n-old\n+new", { exact: true })).toBeVisible();
  const links = table.getByRole("link", { name: "以相同的 Test Case 與版本重新試跑" });
  await expect(links.nth(0)).toHaveAttribute(
    "href",
    `/lab/run?skill=${SKILL}&version=${VERSION}&test_case=${TEST_CASE}`,
  );
  await expect(links.nth(1)).toHaveAttribute(
    "href",
    `/lab/run?skill=${SKILL}&version=second-version&test_case=${TEST_CASE}`,
  );
});

test("missing evaluation and unknown cost never become pass or zero", async ({ page }) => {
  await stubPlatform(page);
  const missing = comparisonSide(OTHER_RUN, false);
  await page.route(`**/runs/${RUN}/comparison?*`, (route) =>
    route.fulfill({
      json: {
        ...COMPARISON,
        runs: [
          comparisonSide(RUN, true),
          {
            ...missing,
            final_output: undefined,
            duration_ms: undefined,
            cost: { ...missing.cost, credits: null },
          },
        ],
      },
    }),
  );
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const table = page.getByRole("table", { name: "Run 任務判定與執行狀態對比" });
  await expect(table.getByRole("cell", { name: "未評估（不是通過）", exact: true })).toBeVisible();
  await expect(
    table.getByRole("row").filter({ hasText: "Run 用掉的點數" }).getByRole("cell"),
  ).toHaveText(["169 點", "未測量"]);
  await expect(table.getByRole("cell", { name: "未開始執行", exact: true })).toBeVisible();
  await expect(
    table.getByRole("cell", {
      name: "已刪除或已過期，無法以相同輸入重跑；比較內容本身不受影響。",
      exact: true,
    }),
  ).toBeVisible();
  await expect(table.getByRole("link", { name: "以相同的 Test Case 與版本重新試跑" })).toHaveCount(
    1,
  );
  await expect(
    page.getByRole("table", { name: "驗收條件判定矩陣對比" }).getByRole("cell").last(),
  ).toHaveText("未評估");
});
