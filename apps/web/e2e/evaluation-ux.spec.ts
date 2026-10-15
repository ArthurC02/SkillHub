import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import {
  EVALUATION,
  REVISIONS,
  RUN,
  SUGGESTIONS,
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("a criterion reveals executed check nodes without replacing its final verdict", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route(`**/runs/${RUN}/evaluation`, (route) =>
    route.fulfill({
      json: {
        ...EVALUATION,
        criterion_results: [
          {
            ...EVALUATION.criterion_results[0],
            steps: [
              { path: "0.0", kind: "contains", result: "failed", reason: "找不到 email 欄位" },
              {
                path: "0",
                kind: "sequence",
                result: "failed",
                reason: "前置檢查未通過，後續評審未執行",
              },
            ],
          },
        ],
      },
    }),
  );
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator(".verdict")).toHaveText("任務判定：未符合");
  const reveal = page.getByText("檢查節點結果（2 項）", { exact: true });
  await expect(reveal).toBeVisible();
  await expect(page.getByText("找不到 email 欄位", { exact: false })).toBeHidden();
  await reveal.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText("前置檢查未通過，後續評審未執行", { exact: false })).toBeVisible();
  await expect(
    page.getByText("只列出執行過的檢查節點；最終判定以上方的驗收條件為準。", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".criterion-list > li .badge-criterion-failed")).toHaveCount(3);
  await expect(page.locator(".verdict")).toHaveText("任務判定：未符合");
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("criterion-check-nodes-phone.png") });
});

test("a failed evaluation reread hides cached verdicts and recovers in place", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  await page.route(`**/runs/${RUN}/evaluation?*`, (route) =>
    route.fulfill({
      json: {
        ...EVALUATION,
        evaluation_id: "eval-1",
        overall: "undetermined",
        summary: "歷史判定",
        superseded_at: "2026-08-17T02:00:00Z",
      },
    }),
  );
  await page.route(`**/runs/${RUN}/evaluation`, async (route) => {
    reads += 1;
    if (reads === 1)
      return route.fulfill({
        json: {
          ...EVALUATION,
          overall: "met",
          summary: "先前的判定",
          evidence_complete: true,
          deterministic_findings: [],
          criterion_results: [
            {
              ...EVALUATION.criterion_results[0],
              result: "passed",
              reason: "輸出的 CSV 含有 email 欄位",
              evidence: [
                {
                  kind: "agent_output",
                  available: true,
                  match: "exact",
                  excerpt: "header: name,email,phone",
                  excerpt_truncated: false,
                },
              ],
            },
          ],
        },
      });
    if (reads === 2) return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    await held;
    return route.fulfill({ json: EVALUATION });
  });
  await page.goto(`/runs/${RUN}`);
  const panel = page.locator("#run-decision");
  await expect(panel.locator(".verdict")).toHaveText("任務判定：符合");
  await page.getByLabel("評估版本").selectOption("eval-1");
  await expect(page.getByText("歷史判定", { exact: true })).toBeVisible();
  await panel.getByRole("button", { name: "回到目前的判定" }).click();
  await expect(panel.getByRole("alert")).toContainText("暫時無法讀取評估結果");
  await expect(panel.locator(".verdict")).toHaveCount(0);
  await expect(page.locator("#feedback-comment")).toHaveCount(0);
  await expect(panel.getByRole("button", { name: /建立新版本/ })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("evaluation-read-failed-phone.png") });
  await panel.getByRole("button", { name: "再試一次" }).focus();
  await page.keyboard.press("Enter");
  try {
    await expect.poll(() => reads).toBe(3);
    await expect(panel.getByRole("status").filter({ hasText: "載入評估結果中" })).toBeVisible();
    await expect(panel.locator(".verdict")).toHaveCount(0);
    await expect(page.locator("#evaluation-heading")).toBeFocused();
  } finally {
    finishRead();
  }
  await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
  await expect(page.locator("#feedback-comment")).toHaveValue("沒說到重點。");
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("evaluation-recovered-phone.png") });
});

test("an unavailable historical evaluation always offers the current verdict", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route(`**/runs/${RUN}/evaluation?*`, (route) =>
    route.fulfill({ status: 404, json: { error: "not found" } }),
  );
  await page.route(`**/runs/${RUN}/evaluation/revisions`, (route) =>
    route.fulfill({ status: 503, json: { error: "service unavailable" } }),
  );
  await page.goto(`/runs/${RUN}?evaluation=eval-gone&events=0,100`);
  const panel = page.locator("#run-decision");
  await expect(panel.getByRole("alert").filter({ hasText: "這份歷史評估" })).toBeVisible();
  await expect(panel.getByText("未評估", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("評估版本")).toHaveCount(0);
  const current = panel.getByRole("button", { name: "回到目前的判定" });
  await current.focus();
  await page.keyboard.press("Enter");
  await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
  expect(new URL(page.url()).searchParams.has("evaluation")).toBe(false);
  expect(new URL(page.url()).searchParams.get("events")).toBe("0,100");
  await expect(page.locator("#evaluation-heading")).toBeFocused();
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("historical-evaluation-recovery-phone.png") });
});

test("suggestions from another evaluation cannot be accepted before a matching reread", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.route(`**/runs/${RUN}/evaluation`, (route) =>
    route.fulfill({ json: { ...EVALUATION, evaluation_id: "eval-3" } }),
  );
  await page.route(`**/runs/${RUN}/evaluation/revisions`, (route) =>
    route.fulfill({
      json: { revisions: [{ ...REVISIONS.revisions[0], evaluation_id: "eval-3" }] },
    }),
  );
  let reads = 0;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  await page.route(`**/runs/${RUN}/suggestions`, async (route) => {
    reads += 1;
    if (reads === 1) return route.fulfill({ json: SUGGESTIONS });
    await held;
    return route.fulfill({ json: { ...SUGGESTIONS, evaluation_id: "eval-3" } });
  });
  await page.goto(`/runs/${RUN}`);
  const panel = page.locator("#run-decision");
  await expect(panel.getByRole("status").filter({ hasText: "屬於另一份評估" })).toBeVisible();
  await expect(panel.getByRole("button", { name: "接受", exact: true })).toHaveCount(0);
  await expect(panel.getByRole("button", { name: /建立新版本/ })).toHaveCount(0);
  await panel.getByRole("button", { name: "重新讀取改善建議" }).click();
  try {
    await expect.poll(() => reads).toBe(2);
    await expect(panel.getByRole("button", { name: "重新讀取中…" })).toBeDisabled();
    await expect(panel.getByRole("button", { name: "接受", exact: true })).toHaveCount(0);
  } finally {
    finishRead();
  }
  await expect(panel.getByRole("button", { name: "以已接受的 1 項建議建立新版本" })).toBeEnabled();
  await expect(panel.getByRole("status").filter({ hasText: "屬於另一份評估" })).toHaveCount(0);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("matching-suggestions-desktop.png"),
    fullPage: true,
  });
});

test("editing feedback clears the last submission confirmation", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let current = EVALUATION;
  await page.route(`**/runs/${RUN}/evaluation`, (route) => route.fulfill({ json: current }));
  let writes = 0;
  let finishWrite!: () => void;
  const held = new Promise<void>((resolve) => {
    finishWrite = resolve;
  });
  await page.route(`**/runs/${RUN}/evaluation/feedback`, async (route) => {
    writes += 1;
    expect(route.request().postDataJSON()).toEqual({ helpful: true, comment: "reviewer note" });
    await held;
    current = {
      ...EVALUATION,
      feedback: { helpful: true, comment: "reviewer note", submitted_at: "2026-10-13T12:00:00Z" },
    };
    return route.fulfill({ json: current });
  });
  await page.goto(`/runs/${RUN}`);
  const comment = page.locator("#feedback-comment");
  await comment.fill("reviewer note");
  await page.getByRole("button", { name: "有幫助", exact: true }).click();
  try {
    await expect.poll(() => writes).toBe(1);
    await expect(comment).toHaveAttribute("readonly", "");
    await expect(page.getByRole("button", { name: "沒幫助", exact: true })).toBeDisabled();
  } finally {
    finishWrite();
  }
  await expect(page.getByText("已送出回饋。", { exact: true })).toBeVisible();
  await comment.fill("new unsent note");
  await expect(page.getByText("已送出回饋。", { exact: true })).toHaveCount(0);
  expect(writes).toBe(1);
});

for (const scope of ["workspace", "platform"] as const) {
  test(`${scope} grader revision keeps its result after a failed list reread`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    const grader = scope === "workspace" ? WORKSPACE_GRADER : PLATFORM_GRADER;
    const path = scope === "workspace" ? "/evaluation/graders" : "/admin/evaluation/graders";
    const next = {
      ...grader,
      version: grader.version + 1,
      version_id: "11111111-2222-4333-8444-555555555555",
      instructions: "必須友善且簡短",
      revised_at: "2026-10-13T12:00:00Z",
    };
    let revised = false;
    let unavailable = false;
    let writes = 0;
    let finishWrite!: () => void;
    const held = new Promise<void>((resolve) => {
      finishWrite = resolve;
    });
    await page.route(`**${path}`, (route) =>
      route.fulfill(
        unavailable
          ? { status: 503, json: { error: "service unavailable" } }
          : {
              json: {
                graders:
                  scope === "workspace"
                    ? [PLATFORM_GRADER, revised ? next : grader]
                    : [revised ? next : grader],
              },
            },
      ),
    );
    await page.route(`**${path}/${grader.id}/versions`, async (route) => {
      writes += 1;
      expect(route.request().postDataJSON()).toEqual({
        instructions: "必須友善且簡短",
        reason: "clarifies the expected tone",
      });
      await held;
      revised = true;
      unavailable = true;
      return route.fulfill({ json: next });
    });
    await page.goto(scope === "workspace" ? "/lab/graders" : "/admin/graders");
    await expect(page.getByRole("heading", { name: "現有的評分器" })).toBeInViewport();
    await expect(
      page.getByRole("heading", {
        name: `${grader.name} v${scope === "workspace" ? 2 : 1}`,
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.locator("#grader-create-name")).toBeHidden();
    const instructions = page.locator(`#grader-${grader.id}-instructions`);
    await expect(instructions).toBeHidden();
    await page.screenshot({ path: testInfo.outputPath(`${scope}-grader-overview-phone.png`) });
    const edit = page.getByText(`修改「${grader.name}」的評分指示`, { exact: true });
    await edit.focus();
    await page.keyboard.press("Enter");
    await instructions.fill("必須友善且簡短");
    await page.locator(`#grader-${grader.id}-reason`).fill("clarifies the expected tone");
    await page.getByRole("button", { name: "存成新版本", exact: true }).click();
    try {
      await expect.poll(() => writes).toBe(1);
      await expect(instructions).toHaveAttribute("readonly", "");
      await expect(page.getByRole("button", { name: "存成新版本中…" })).toBeDisabled();
    } finally {
      finishWrite();
    }
    const result = page.locator("#grader-result");
    await expect(result).toContainText(`已儲存「${grader.name}」v${scope === "workspace" ? 3 : 2}`);
    await expect(result).toBeFocused();
    await expect(page.getByRole("alert")).toContainText("暫時無法讀取評分器");
    await expect(instructions).toBeHidden();
    await expect(page.locator(".grader-list")).toHaveAttribute("inert", "");
    await expect(page.getByRole("button", { name: "存成新版本", exact: true })).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath(`${scope}-grader-reread-failed-phone.png`) });
    unavailable = false;
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("heading", {
        name: `${grader.name} v${scope === "workspace" ? 3 : 2}`,
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.locator("#grader-list-heading")).toBeFocused();
    await expect(result).toBeVisible();
    await expect(instructions).toBeVisible();
    await expect(instructions).toHaveValue("必須友善且簡短");
    await expect(page.locator(`#grader-${grader.id}-reason`)).toHaveValue("");
    await page.locator(`#grader-${grader.id}-reason`).fill("new unsent reason");
    await expect(result).toHaveCount(0);
    expect(writes).toBe(1);
    const accessibility = await new AxeBuilder({ page }).include("main").analyze();
    expect(accessibility.violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`${scope}-grader-recovered-phone.png`),
      fullPage: true,
    });
  });
}
