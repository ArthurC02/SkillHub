import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { EVALUATION, OTHER_RUN, REVISIONS, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
});

async function inspectState(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(label + "-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(label + "-viewport.png") });
  await page.screenshot({ path: test.info().outputPath(label + "-full.png"), fullPage: true });
}

for (const [name, invalid] of [
  ["another Run", { ...EVALUATION, run_id: OTHER_RUN }],
  ["missing criteria", { ...EVALUATION, criterion_results: undefined }],
  [
    "missing criterion reason",
    {
      ...EVALUATION,
      criterion_results: [{ ...EVALUATION.criterion_results[1], reason: undefined }],
    },
  ],
  ["unknown verdict", { ...EVALUATION, overall: "unknown" }],
] as const) {
  for (const reread of [false, true]) {
    test(`an evaluation with ${name} on ${reread ? "reread" : "first read"} keeps Run context and recovers locally`, async ({
      page,
    }) => {
      let reads = 0;
      let recovered = false;
      let documents = 0;
      const writes: unknown[] = [];
      page.on("domcontentloaded", () => documents++);
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const feedback = {
        helpful: true,
        comment: "這份評估的回饋",
        submitted_at: "2026-10-16T00:00:00Z",
      };
      const restored = {
        ...EVALUATION,
        feedback: reread ? feedback : undefined,
        cost: {
          evaluation_credits: reread ? null : 0,
          source: reread ? "unreported" : "gateway",
          note: "",
        },
      };
      await page.route(`**/runs/${RUN}/evaluation`, async (route) => {
        reads++;
        if (recovered) {
          await gate;
          return route.fulfill({ json: restored });
        }
        return route.fulfill({ json: reread && reads === 1 ? EVALUATION : invalid });
      });
      await page.route(`**/runs/${RUN}/evaluation/feedback`, (route) => {
        writes.push(route.request().postDataJSON());
        return route.fulfill({ json: { ...EVALUATION, feedback } });
      });
      try {
        await page.goto(`/runs/${RUN}`);
        if (reread) {
          await page.locator("#feedback-comment").fill(feedback.comment);
          await page.getByRole("button", { name: "有幫助", exact: true }).click();
        }
        const main = page.getByRole("main");
        const panel = page.locator("#evaluation-heading").locator("..");
        await expect(panel.getByRole("alert")).toContainText("暫時無法讀取評估結果");
        await expect(
          main.getByRole("heading", { level: 1, name: "試跑結果", exact: true }),
        ).toBeVisible();
        await expect(main.getByRole("heading", { name: "這次試跑", exact: true })).toBeVisible();
        await expect(panel.locator(".verdict")).toHaveCount(0);
        await expect(panel.getByRole("button", { name: "有幫助", exact: true })).toHaveCount(0);
        await expect(panel.getByRole("heading", { name: "改善建議", exact: true })).toHaveCount(0);
        await expect(main.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        expect(reads).toBe(reread ? 2 : 1);
        expect(writes).toHaveLength(reread ? 1 : 0);
        if (reread) expect(writes[0]).toEqual({ helpful: true, comment: feedback.comment });
        await inspectState(page, "evaluation-unconfirmed");
        recovered = true;
        await panel.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect.poll(() => reads).toBe(reread ? 3 : 2);
        await expect(page.locator("#evaluation-heading")).toBeFocused();
        release();
        await expect(panel.getByRole("alert")).toHaveCount(0);
        await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
        const cost = panel
          .getByRole("heading", { name: "評估本身用掉的點數", exact: true })
          .locator("xpath=following-sibling::p[1]");
        await expect(cost).toHaveText(reread ? "未測量" : "0 點（模型閘道實付）");
        await expect(page.locator("#feedback-comment")).toHaveValue(reread ? feedback.comment : "");
        expect(documents).toBe(1);
        expect(writes).toHaveLength(reread ? 1 : 0);
      } finally {
        release();
      }
    });
  }
}

test("a different historical Evaluation cannot replace the requested revision", async ({
  page,
}) => {
  let reads = 0;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await page.route(`**/runs/${RUN}/evaluation?revision=eval-1`, (route) => {
    reads++;
    return route.fulfill({
      json:
        reads === 1
          ? EVALUATION
          : {
              ...EVALUATION,
              evaluation_id: "eval-1",
              overall: "undetermined",
              superseded_at: "2026-10-16T00:00:00Z",
            },
    });
  });
  await page.goto(`/runs/${RUN}?evaluation=eval-1`);
  const panel = page.locator("#evaluation-heading").locator("..");
  await expect(panel.getByRole("alert")).toContainText("暫時無法讀取這份歷史評估");
  await expect(
    page.getByRole("heading", { level: 1, name: "試跑結果", exact: true }),
  ).toBeVisible();
  await expect(panel.locator(".verdict")).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "有幫助", exact: true })).toHaveCount(0);
  await inspectState(page, "evaluation-history-unconfirmed");
  await panel.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(panel.locator(".verdict")).toHaveText("任務判定：無法判斷");
  await expect(panel.getByText("你正在看歷史判定", { exact: false })).toBeVisible();
  await expect(panel.getByText("回饋只能對目前的判定填寫。", { exact: true })).toBeVisible();
  await expect(page.locator("#evaluation-heading")).toBeFocused();
  expect(reads).toBe(2);
  expect(writes).toEqual([]);
});

test("an unreadable history list keeps the current verdict and retries only history", async ({
  page,
}) => {
  let historyReads = 0;
  let evaluationReads = 0;
  await page.route(`**/runs/${RUN}/evaluation`, (route) => {
    evaluationReads++;
    return route.fulfill({ json: EVALUATION });
  });
  await page.route(`**/runs/${RUN}/evaluation/revisions`, (route) => {
    historyReads++;
    return route.fulfill({
      json: historyReads === 1 ? { revisions: [REVISIONS.revisions[0], null] } : REVISIONS,
    });
  });
  await page.goto(`/runs/${RUN}`);
  const panel = page.locator("#evaluation-heading").locator("..");
  await expect(panel.getByRole("alert")).toContainText("暫時無法讀取歷史評估版本");
  await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
  await expect(
    page.getByRole("heading", { level: 1, name: "試跑結果", exact: true }),
  ).toBeVisible();
  await expect(page.locator("#evaluation-revision")).toHaveCount(0);
  await inspectState(page, "evaluation-history-list-unconfirmed");
  await panel.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator("#evaluation-revision")).toBeVisible();
  await expect(panel.getByRole("alert")).toHaveCount(0);
  await expect(page.locator("#evaluation-heading")).toBeFocused();
  expect(historyReads).toBe(2);
  expect(evaluationReads).toBe(1);
});

test("a readable pending evaluation completes without inventing a cost or verdict", async ({
  page,
}) => {
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/runs/${RUN}/evaluation`, async (route) => {
    reads++;
    if (reads > 1) {
      await gate;
      return route.fulfill({
        json: { ...EVALUATION, cost: { evaluation_credits: null, source: "unreported", note: "" } },
      });
    }
    return route.fulfill({
      json: {
        ...EVALUATION,
        status: "pending",
        overall: "undetermined",
        evaluated_at: null,
        criterion_results: [],
        deterministic_findings: [],
        judge_model: "",
        judge_prompt_version: "",
        feedback: undefined,
        cost: { evaluation_credits: null, source: "unreported", note: "" },
      },
    });
  });
  try {
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator("#evaluation-heading").locator("..");
    await expect(panel.getByRole("status")).toContainText("評估進行中");
    await expect(panel.locator(".verdict")).toHaveCount(0);
    await expect(
      panel.getByRole("heading", { name: "評估本身用掉的點數", exact: true }),
    ).toHaveCount(0);
    await expect.poll(() => reads).toBe(2);
    release();
    await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
    await expect(
      panel
        .getByRole("heading", { name: "評估本身用掉的點數", exact: true })
        .locator("xpath=following-sibling::p[1]"),
    ).toHaveText("未測量");
  } finally {
    release();
  }
});
