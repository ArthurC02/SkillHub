import { afterAll, beforeAll, expect, test } from "vitest";
import {
  EVALUATION,
  REVISIONS,
  RUN,
  SUGGESTIONS,
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});
afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

test("a criterion reveals executed check nodes without replacing its final verdict", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/evaluation`, () => ({
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
  }));
  await page.goto(`/runs/${RUN}`);
  const reveal = page.getByText("檢查節點結果（2 項）", { exact: true });
  await page.eventually(() => {
    expect(page.locator(".verdict").text()).toBe("任務判定：未符合");
    expect(reveal.count()).toBe(1);
  });
  await expect(page.getByText("找不到 email 欄位")).toHaveCount(0);
  await reveal.click();
  await page.eventually(() => {
    expect(page.getByText("前置檢查未通過，後續評審未執行").count()).toBe(1);
    expect(
      page
        .getByText("只列出執行過的檢查節點；最終判定以上方的驗收條件為準。", { exact: true })
        .count(),
    ).toBe(1);
    expect(page.locator(".criterion-list > li .badge-criterion-failed").count()).toBe(3);
    expect(page.locator(".verdict").text()).toBe("任務判定：未符合");
  });
  expect(await page.axeViolations("main")).toEqual([]);
});

test("a failed evaluation reread hides cached verdicts and recovers in place", async () => {
  const page = app();
  let reads = 0;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  page.route(`**/runs/${RUN}/evaluation?*`, () => ({
    json: {
      ...EVALUATION,
      evaluation_id: "eval-1",
      overall: "undetermined",
      summary: "歷史判定",
      superseded_at: "2026-08-17T02:00:00Z",
    },
  }));
  page.route(`**/runs/${RUN}/evaluation`, async () => {
    reads += 1;
    if (reads === 1)
      return {
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
      };
    if (reads === 2) return { status: 503, json: { error: "service unavailable" } };
    await held;
    return { json: EVALUATION };
  });
  try {
    await page.goto(`/runs/${RUN}`);
    const verdict = page.locator("#run-decision .verdict");
    await expect(verdict).toHaveText("任務判定：符合");
    await page.getByLabel("評估版本").select("eval-1");
    await expect(page.getByText("歷史判定", { exact: true })).toHaveCount(1);
    await page.getByRole("button", { name: "回到目前的判定" }).click();
    await expect(page.locator("#run-decision [role=alert]")).toContainText("暫時無法讀取評估結果");
    await expect(verdict).toHaveCount(0);
    await expect(page.locator("#feedback-comment")).toHaveCount(0);
    await expect(page.getByRole("button", { name: /建立新版本/ })).toHaveCount(0);
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(reads).toBe(3);
      expect(
        page.getByRole("status").filter({ hasText: "載入評估結果中" }).count(),
      ).toBeGreaterThan(0);
      expect(verdict.count()).toBe(0);
      expect(page.locator("#evaluation-heading").isFocused()).toBe(true);
    });
  } finally {
    finishRead();
  }
  await page.eventually(() => {
    expect(page.locator("#run-decision .verdict").text()).toBe("任務判定：未符合");
    expect(page.locator("#feedback-comment").value()).toBe("沒說到重點。");
  });
  expect(await page.axeViolations("main")).toEqual([]);
});

test("an unavailable historical evaluation always offers the current verdict", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/evaluation?*`, () => ({
    status: 404,
    json: { error: "not found" },
  }));
  page.route(`**/runs/${RUN}/evaluation/revisions`, () => ({
    status: 503,
    json: { error: "service unavailable" },
  }));
  await page.goto(`/runs/${RUN}?evaluation=eval-gone&events=0,100`);
  await expect(page.getByRole("alert").filter({ hasText: "這份歷史評估" })).toBeVisible();
  await expect(page.getByText("未評估", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("評估版本")).toHaveCount(0);
  const current = page.getByRole("button", { name: "回到目前的判定" });
  await current.focus();
  await page.press("Enter");
  await page.eventually(() => {
    expect(page.locator("#run-decision .verdict").text()).toBe("任務判定：未符合");
    expect(page.locator("#evaluation-heading").isFocused()).toBe(true);
  });
  const params = new URLSearchParams(page.search());
  expect(params.has("evaluation")).toBe(false);
  expect(params.get("events")).toBe("0,100");
  expect(await page.axeViolations("main")).toEqual([]);
});

test("suggestions from another evaluation cannot be accepted before a matching reread", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/evaluation`, () => ({
    json: { ...EVALUATION, evaluation_id: "eval-3" },
  }));
  page.route(`**/runs/${RUN}/evaluation/revisions`, () => ({
    json: { revisions: [{ ...REVISIONS.revisions[0], evaluation_id: "eval-3" }] },
  }));
  let reads = 0;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  page.route(`**/runs/${RUN}/suggestions`, async () => {
    reads += 1;
    if (reads === 1) return { json: SUGGESTIONS };
    await held;
    return { json: { ...SUGGESTIONS, evaluation_id: "eval-3" } };
  });
  const mismatch = page.getByRole("status").filter({ hasText: "屬於另一份評估" });
  const accept = page.getByRole("button", { name: "接受", exact: true });
  try {
    await page.goto(`/runs/${RUN}`);
    await expect(mismatch).toBeVisible();
    await expect(accept).toHaveCount(0);
    await expect(page.getByRole("button", { name: /建立新版本/ })).toHaveCount(0);
    await page.getByRole("button", { name: "重新讀取改善建議" }).click();
    await page.eventually(() => {
      expect(reads).toBe(2);
      expect(page.getByRole("button", { name: "重新讀取中…" }).isDisabled()).toBe(true);
      expect(accept.count()).toBe(0);
    });
  } finally {
    finishRead();
  }
  await page.eventually(() => {
    expect(page.getByRole("button", { name: "以已接受的 1 項建議建立新版本" }).isDisabled()).toBe(
      false,
    );
    expect(mismatch.count()).toBe(0);
  });
  expect(await page.axeViolations("main")).toEqual([]);
});

test("editing feedback clears the last submission confirmation", async () => {
  const page = app();
  let current = EVALUATION;
  page.route(`**/runs/${RUN}/evaluation`, () => ({ json: current }));
  const bodies: unknown[] = [];
  let finishWrite!: () => void;
  const held = new Promise<void>((resolve) => {
    finishWrite = resolve;
  });
  page.route(`**/runs/${RUN}/evaluation/feedback`, async ({ body }) => {
    bodies.push(body);
    await held;
    current = {
      ...EVALUATION,
      feedback: { helpful: true, comment: "reviewer note", submitted_at: "2026-10-13T12:00:00Z" },
    };
    return { json: current };
  });
  await page.goto(`/runs/${RUN}`);
  const comment = page.locator("#feedback-comment");
  await expect(comment).toHaveCount(1);
  await comment.fill("reviewer note");
  await page.getByRole("button", { name: "有幫助", exact: true }).click();
  try {
    await page.eventually(() => {
      expect(bodies).toEqual([{ helpful: true, comment: "reviewer note" }]);
      expect(comment.attribute("readonly")).toBe("");
      expect(page.getByRole("button", { name: "沒幫助", exact: true }).isDisabled()).toBe(true);
    });
  } finally {
    finishWrite();
  }
  const confirmation = page.getByText("已送出回饋。", { exact: true });
  await expect(confirmation).toHaveCount(1);
  await comment.fill("new unsent note");
  await expect(confirmation).toHaveCount(0);
  expect(bodies).toHaveLength(1);
});

for (const scope of ["workspace", "platform"] as const) {
  test(`${scope} grader revision keeps its result after a failed list reread`, async () => {
    const page = app();
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
    const bodies: unknown[] = [];
    let finishWrite!: () => void;
    const held = new Promise<void>((resolve) => {
      finishWrite = resolve;
    });
    page.route(`**${path}`, () =>
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
    );
    page.route(`**${path}/${grader.id}/versions`, async ({ body }) => {
      bodies.push(body);
      await held;
      revised = true;
      unavailable = true;
      return { json: next };
    });
    const instructionsSelector = `#grader-${grader.id}-instructions`;
    const instructions = page.locator(instructionsSelector);
    const reason = page.locator(`#grader-${grader.id}-reason`);
    const result = page.locator("#grader-result");
    const save = page.getByRole("button", { name: "存成新版本", exact: true });
    try {
      await page.goto(scope === "workspace" ? "/lab/graders" : "/admin/graders");
      await page.eventually(() => {
        expect(
          page
            .getByRole("heading", {
              name: `${grader.name} v${scope === "workspace" ? 2 : 1}`,
              exact: true,
            })
            .count(),
        ).toBe(1);
        expect(page.getByRole("heading", { name: "現有的評分器" }).count()).toBe(1);
      });
      await expect(page.locator("#grader-create-name")).toHaveCount(0);
      await expect(instructions).toHaveCount(0);
      await page.getByText(`修改「${grader.name}」的評分指示`, { exact: true }).click();
      await instructions.fill("必須友善且簡短");
      await reason.fill("clarifies the expected tone");
      await save.click();
      await page.eventually(() => {
        expect(bodies).toEqual([
          { instructions: "必須友善且簡短", reason: "clarifies the expected tone" },
        ]);
        expect(instructions.attribute("readonly")).toBe("");
        expect(page.getByRole("button", { name: "存成新版本中…" }).isDisabled()).toBe(true);
      });
    } finally {
      finishWrite();
    }
    await page.eventually(() => {
      expect(result.text()).toContain(`已儲存「${grader.name}」v${scope === "workspace" ? 3 : 2}`);
      expect(result.isFocused()).toBe(true);
      expect(page.getByRole("alert").text()).toContain("暫時無法讀取評分器");
    });
    await expect(instructions).toHaveCount(0);
    expect(document.querySelector(".grader-list")?.getAttribute("inert")).toBe("");
    await expect(save).toHaveCount(0);
    unavailable = false;
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(
        page
          .getByRole("heading", {
            name: `${grader.name} v${scope === "workspace" ? 3 : 2}`,
            exact: true,
          })
          .count(),
      ).toBe(1);
      expect(page.locator("#grader-list-heading").isFocused()).toBe(true);
      expect(result.count()).toBe(1);
      expect(instructions.value()).toBe("必須友善且簡短");
      expect(reason.value()).toBe("");
    });
    await reason.fill("new unsent reason");
    await expect(result).toHaveCount(0);
    expect(bodies).toHaveLength(1);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
