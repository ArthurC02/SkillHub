import { expect, test } from "vitest";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL,
  SKILL_VERSIONS,
  TEST_CASE,
  VERSION,
  comparisonSide,
} from "../../../testing/fixtures/platform";
import { Locator, renderedApp } from "../../../testing/app";

const app = renderedApp();

const STATUS_TABLE = "試跑紀錄任務判定與執行狀態對比";
const MATRIX_TABLE = "驗收條件判定矩陣對比";

function table(page: ReturnType<typeof app>, caption: string): Locator {
  return page.locator("table").filter({ hasText: caption });
}

function rowCells(scope: Locator, header: string): Locator {
  return new Locator(() =>
    scope
      .locator("tr")
      .all()
      .filter((row) => row.querySelector("th")?.textContent?.includes(header))
      .flatMap((row) => [...row.querySelectorAll("td")]),
  );
}

function cellTexts(cells: Locator): string[] {
  return cells.all().map((cell) => (cell.textContent ?? "").replace(/\s+/g, " ").trim());
}

test("populated run results keep identity, sources, output and task verdict visible", async () => {
  const page = app();
  await page.goto(`/runs/${RUN}`);
  const identityText = page.getByText(`試跑紀錄 ID：${RUN}`, { exact: true });
  await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toHaveCount(1);
  await expect(identityText).toHaveCount(0);
  const identity = page.getByText("試跑識別碼", { exact: true });
  await identity.click();
  await expect(identityText).toHaveCount(1);
  const source = page.getByRole("navigation", { name: "這次試跑的來源" });
  await page.eventually(() => {
    expect(source.locator("a").filter({ hasText: "來源小工具" }).attribute("href")).toBe(
      `/skills/${SKILL}`,
    );
    expect(source.locator("a").filter({ hasText: "來源 Version" }).attribute("href")).toBe(
      `/skills/${SKILL}/versions/${VERSION}`,
    );
    expect(source.locator("a").filter({ hasText: "來源測試題" }).attribute("href")).toBe(
      `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
    );
    expect(page.getByText("Removed 17 duplicate rows.", { exact: true }).count()).toBe(1);
    expect(page.getByRole("heading", { name: "任務判定", exact: true }).count()).toBe(1);
  });
  await page.getByRole("link", { name: "與另一個試跑比較" }).click();
  await page.eventually(() => {
    expect(page.pathname()).toBe(`/runs/${RUN}/compare`);
    expect(page.getByRole("heading", { name: "試跑比較", exact: true }).count()).toBe(1);
  });
});

test("candidate runs expose their immutable Versions before comparison on a phone", async () => {
  const page = app();
  const older = SKILL_VERSIONS.versions[1];
  page.route("**/runs?*", () => ({
    json: {
      runs: [
        RUNS.runs[0],
        { ...RUNS.runs[1], skill_version_id: older.version_id },
        {
          ...RUNS.runs[1],
          run_id: "44444444-4444-4444-8444-444444444444",
          skill_version_id: VERSION,
        },
      ],
    },
  }));
  await page.goto(`/runs/${RUN}/compare`);
  const olderRun = page.getByRole("button", {
    name: new RegExp(`^與這一次比較（建立於 .+）：v1，試跑紀錄 ID ${OTHER_RUN}$`),
  });
  await page.eventually(() => {
    expect(page.getByRole("link", { name: "v1", exact: true }).attribute("href")).toBe(
      `/skills/${SKILL}/versions/${older.version_id}`,
    );
    expect(page.getByRole("link", { name: "v2", exact: true }).attribute("href")).toBe(
      `/skills/${SKILL}/versions/${VERSION}`,
    );
    expect(olderRun.count()).toBe(1);
  });
  await olderRun.focus();
  await expect(olderRun).toBeFocused();
  await olderRun.click();
  await page.eventually(() => expect(page.search()).toMatch(new RegExp(`against=${OTHER_RUN}$`)));
});

test("comparison aligns distinct outputs, verdicts, costs and version links", async () => {
  const page = app();
  const left = comparisonSide(RUN, true);
  const right = comparisonSide(OTHER_RUN, true);
  page.route(`**/runs/${RUN}/comparison?*`, () => ({
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
  }));
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const statuses = table(page, STATUS_TABLE);
  const links = statuses.locator("a").filter({ hasText: "以相同的測試題與版本重新試跑" });
  await page.eventually(() => {
    expect(cellTexts(rowCells(statuses, "任務判定"))).toEqual(["未符合", "符合"]);
    expect(cellTexts(rowCells(statuses, "最終輸出"))).toEqual(["ALPHA", "BETA"]);
    expect(cellTexts(rowCells(statuses, "延遲"))).toEqual(["12000 毫秒", "15000 毫秒"]);
    expect(cellTexts(rowCells(statuses, "試跑用掉的點數"))).toEqual(["169 點", "0 點"]);
    expect(cellTexts(rowCells(statuses, "評估用掉的點數"))).toEqual(["26 點", "4 點"]);
    expect(cellTexts(table(page, MATRIX_TABLE).locator("td"))).toEqual([
      "未通過模型評估",
      "通過模型評估",
    ]);
    expect(page.getByText("@@ -1 +1 @@ -old +new", { exact: true }).count()).toBe(1);
    expect(links.nth(0).attribute("href")).toBe(
      `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`,
    );
    expect(links.nth(1).attribute("href")).toBe(
      `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=second-version`,
    );
  });
});

test("missing evaluation and unknown cost never become pass or zero", async () => {
  const page = app();
  const missing = comparisonSide(OTHER_RUN, false);
  page.route(`**/runs/${RUN}/comparison?*`, () => ({
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
  }));
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const statuses = table(page, STATUS_TABLE);
  await page.eventually(() => {
    const cells = statuses.locator("td");
    expect(cellTexts(cells)).toContain("未評估（不是通過）");
    expect(cellTexts(rowCells(statuses, "試跑用掉的點數"))).toEqual(["169 點", "未測量"]);
    expect(cellTexts(cells)).toContain("未開始執行");
    expect(cellTexts(cells)).toContain(
      "已刪除或已過期，無法以相同輸入重跑；比較內容本身不受影響。",
    );
    expect(statuses.locator("a").filter({ hasText: "以相同的測試題與版本重新試跑" }).count()).toBe(
      1,
    );
    expect(cellTexts(table(page, MATRIX_TABLE).locator("td")).at(-1)).toBe("未評估");
  });
});
