import { expect, test } from "vitest";
import { EVALUATION, OTHER_RUN, REVISIONS, RUN } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

const PANEL = "section:has(> #evaluation-heading)";

function costOf(page: App) {
  return page.locator(PANEL).locator("h3 + p").first();
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
    test(`an evaluation with ${name} on ${reread ? "reread" : "first read"} keeps Run context and recovers locally`, async () => {
      const page = app();
      let reads = 0;
      let recovered = false;
      const writes: unknown[] = [];
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
      page.route(`**/runs/${RUN}/evaluation`, async () => {
        reads++;
        if (recovered) {
          await gate;
          return { json: restored };
        }
        return { json: reread && reads === 1 ? EVALUATION : invalid };
      });
      page.route(`**/runs/${RUN}/evaluation/feedback`, ({ body }) => {
        writes.push(body);
        return { json: { ...EVALUATION, feedback } };
      });
      try {
        await page.goto(`/runs/${RUN}`);
        if (reread) {
          await page.locator("#feedback-comment").fill(feedback.comment);
          await page.getByRole("button", { name: "有幫助", exact: true }).click();
        }
        const panel = page.locator(PANEL);
        await expect(panel.getByRole("alert")).toContainText("暫時無法讀取評估結果");
        await expect(page.locator("main h1").filter({ hasText: "試跑結果" })).toBeVisible();
        await expect(page.getByRole("heading", { name: "這次試跑", exact: true })).toBeVisible();
        await expect(panel.locator(".verdict")).toHaveCount(0);
        await expect(panel.getByRole("button", { name: "有幫助", exact: true })).toHaveCount(0);
        await expect(panel.getByRole("heading", { name: "改善建議", exact: true })).toHaveCount(0);
        await expect(page.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        expect(reads).toBe(reread ? 2 : 1);
        expect(writes).toHaveLength(reread ? 1 : 0);
        if (reread) expect(writes[0]).toEqual({ helpful: true, comment: feedback.comment });
        expect(await page.axeViolations("body")).toEqual([]);
        recovered = true;
        await panel.getByRole("button", { name: "再試一次", exact: true }).click();
        await page.eventually(() => expect(reads).toBe(reread ? 3 : 2));
        await expect(page.locator("#evaluation-heading")).toBeFocused();
        release();
        await expect(panel.getByRole("alert")).toHaveCount(0);
        await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
        await expect(costOf(page)).toHaveText(reread ? "未測量" : "0 點（模型閘道實付）");
        await expect(page.locator("#feedback-comment")).toHaveValue(reread ? feedback.comment : "");
        expect(writes).toHaveLength(reread ? 1 : 0);
      } finally {
        release();
      }
    });
  }
}

test("a different historical Evaluation cannot replace the requested revision", async () => {
  const page = app();
  let reads = 0;
  page.route(`**/runs/${RUN}/evaluation?revision=eval-1`, () => {
    reads++;
    return {
      json:
        reads === 1
          ? EVALUATION
          : {
              ...EVALUATION,
              evaluation_id: "eval-1",
              overall: "undetermined",
              superseded_at: "2026-10-16T00:00:00Z",
            },
    };
  });
  await page.goto(`/runs/${RUN}?evaluation=eval-1`);
  const panel = page.locator(PANEL);
  await expect(panel.getByRole("alert")).toContainText("暫時無法讀取這份歷史評估");
  await expect(page.locator("main h1").filter({ hasText: "試跑結果" })).toBeVisible();
  await expect(panel.locator(".verdict")).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "有幫助", exact: true })).toHaveCount(0);
  expect(await page.axeViolations("body")).toEqual([]);
  await panel.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(panel.locator(".verdict")).toHaveText("任務判定：無法判斷");
  await expect(panel.getByText("你正在看歷史判定", { exact: false })).toBeVisible();
  await expect(panel.getByText("回饋只能對目前的判定填寫。", { exact: true })).toBeVisible();
  await expect(page.locator("#evaluation-heading")).toBeFocused();
  expect(reads).toBe(2);
  expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
});

test("an unreadable history list keeps the current verdict and retries only history", async () => {
  const page = app();
  let historyReads = 0;
  let evaluationReads = 0;
  page.route(`**/runs/${RUN}/evaluation`, () => {
    evaluationReads++;
    return { json: EVALUATION };
  });
  page.route(`**/runs/${RUN}/evaluation/revisions`, () => {
    historyReads++;
    return {
      json: historyReads === 1 ? { revisions: [REVISIONS.revisions[0], null] } : REVISIONS,
    };
  });
  await page.goto(`/runs/${RUN}`);
  const panel = page.locator(PANEL);
  await expect(panel.getByRole("alert")).toContainText("暫時無法讀取歷史評估版本");
  await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
  await expect(page.locator("main h1").filter({ hasText: "試跑結果" })).toBeVisible();
  await expect(page.locator("#evaluation-revision")).toHaveCount(0);
  expect(await page.axeViolations("body")).toEqual([]);
  await panel.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator("#evaluation-revision")).toBeVisible();
  await expect(panel.getByRole("alert")).toHaveCount(0);
  await expect(page.locator("#evaluation-heading")).toBeFocused();
  expect(historyReads).toBe(2);
  expect(evaluationReads).toBe(1);
});

test("a readable pending evaluation completes without inventing a cost or verdict", async () => {
  const page = app();
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route(`**/runs/${RUN}/evaluation`, async () => {
    reads++;
    if (reads > 1) {
      await gate;
      return {
        json: { ...EVALUATION, cost: { evaluation_credits: null, source: "unreported", note: "" } },
      };
    }
    return {
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
    };
  });
  try {
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator(PANEL);
    await expect(panel.getByRole("status")).toContainText("評估進行中");
    await expect(panel.locator(".verdict")).toHaveCount(0);
    await expect(
      panel.getByRole("heading", { name: "評估本身用掉的點數", exact: true }),
    ).toHaveCount(0);
    await page.eventually(() => expect(reads).toBe(2));
    release();
    await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
    await expect(costOf(page)).toHaveText("未測量");
  } finally {
    release();
  }
});
