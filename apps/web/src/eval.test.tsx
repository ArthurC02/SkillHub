import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { EvaluationPanel } from "./pages/RunEvaluation";
import type { Evaluation, ImprovementSuggestion, SuggestionDiff } from "./api/evaluation";

// 02:EVAL-001 / EVAL-002. Four assertions, one per rule the view exists to keep:
// a run's terminal state is never a task verdict, no evaluation is 未評估 and not
// a pass, expired evidence shows its excerpt and says the original is gone, and a
// blocked suggestion states which rule blocked it.
//
// Same hand-rolled DOM plumbing as trace.test.tsx: @testing-library is not a
// dependency and these assertions do not justify adding one.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children?: unknown }) => children,
}));

const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const SKILL = "11111111-1111-1111-1111-111111111111";

const evaluation: Evaluation = {
  evaluation_id: "eval-1",
  run_id: RUN,
  status: "completed",
  // The workload finished; the task was not done. An ordinary consistent state.
  overall: "not_met",
  summary: "產出的檔案缺少要求的欄位。",
  criterion_results: [
    {
      criterion_id: "c1",
      text: "輸出的 CSV 含有 email 欄位",
      result: "failed",
      source: "model",
      reason: "最終輸出的表頭沒有 email。",
      evidence: [
        {
          kind: "trace_event",
          trace_event_id: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
          occurred_at: "2026-08-16T09:12:04.002Z",
          // The trace partition this cited has been dropped.
          available: false,
          excerpt: "header: name,phone",
          excerpt_truncated: true,
        },
      ],
    },
    {
      criterion_id: "c2",
      text: "沒有刪掉原始列",
      result: "undetermined",
      source: "rule",
      reason: "Trace 有缺漏，證據不足以判定。",
      evidence: [],
    },
  ],
  deterministic_findings: [
    {
      category: "activation",
      severity: "warning",
      message: "沒有出現 Skill 啟用事件。",
      evidence: [],
    },
  ],
  judge_model: "gpt-5.6-terra",
  judge_prompt_version: "judge-2026-08-17",
  evidence_complete: false,
  cost: { evaluation_usd: 0.0212, source: "gateway", note: "閘道對這次評估的實付。" },
  evaluated_at: "2026-08-17T02:00:00Z",
  superseded_at: null,
};

const suggestion: ImprovementSuggestion = {
  suggestion_id: "s1",
  category: "skill",
  problem: "SKILL.md 沒有交代輸出欄位。",
  evidence: [],
  target_path: "SKILL.md",
  expected_impact: "模型會照著列出的欄位輸出。",
  decision: "pending",
};

const blockedDiff: SuggestionDiff = {
  target_path: "SKILL.md",
  applicable: false,
  blocked_reason: "target_changed",
};

function stubPlatform(options: { evaluated: boolean }) {
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    // The run read, which is where the apply action gets the skill to build the
    // new version under — the page no longer takes it from its own URL.
    if (url.endsWith("/runs/" + RUN)) {
      return json({
        run_id: RUN,
        skill_id: SKILL,
        skill_version_id: "22222222-2222-2222-2222-222222222222",
        test_case_snapshot_id: "33333333-3333-3333-3333-333333333333",
        test_case_id: "44444444-4444-4444-4444-444444444444",
      });
    }
    if (!options.evaluated) return json({ error: "not found" }, 404);
    if (url.includes("/evaluation/revisions")) return json({ revisions: [] });
    if (url.includes("/evaluation")) return json(evaluation);
    if (url.includes("/suggestions/s1/diff")) return json(blockedDiff);
    if (url.includes("/suggestions"))
      return json({ evaluation_id: "eval-1", suggestions: [suggestion] });
    return json({ error: "not found" }, 404);
  });
}

async function render(runStatus: string) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <EvaluationPanel runId={RUN} runStatus={runStatus} />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => !(container.textContent ?? "").includes("載入評估結果中"));
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

test("ADR-025 a succeeded run whose task failed reads as 執行完成 plus 未符合, never as a pass", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");

  const text = container.textContent ?? "";
  // Two statements, two rows: the execution outcome is worded as execution and
  // the task verdict is its own field. Collapsing them is the failure this test
  // exists to catch.
  expect(text).toContain("執行完成");
  expect(text).toContain("任務判定");
  expect(text).toContain("未符合");
  // The per-criterion verdicts keep their own words, and a model verdict is
  // labelled as a model's (EVAL-001 第 5 條).
  expect(text).toContain("未通過");
  expect(text).toContain("無法判斷");
  expect(text).toContain("模型評估");
  expect(text).toContain("規則判定");
  // Incomplete evidence is stated, not smoothed over.
  expect(text).toContain("材料不完整");
});

test("ADR-025 a run with no evaluation says 未評估 and does not imply a pass", async () => {
  stubPlatform({ evaluated: false });
  await render("succeeded");

  const text = container.textContent ?? "";
  expect(text).toContain("未評估");
  expect(text).toContain("未評估不等於通過");
  expect(text).not.toContain("符合");
});

test("ADR-026 expired evidence shows the excerpt kept at judgement time and says the original is gone", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");

  const text = container.textContent ?? "";
  expect(text).toContain("原始資料已過期或已刪除");
  // Neither blanked out nor presented as though the trace event were still there.
  expect(container.querySelector("pre")?.textContent).toContain("header: name,phone");
  expect(text).toContain("摘要已截斷");
});

test("EVAL-002 the apply action is offered on a run reached without a skill in its URL", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");
  await waitFor(() => (container.textContent ?? "").includes("建立新版本"));

  // The skill came from GET /runs/{id}, so the action does not depend on how the
  // page was reached. The old `?skill=` stopgap and its explanation are gone.
  const apply = Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("建立新版本"),
  );
  expect(apply).toBeDefined();
  expect(container.textContent).not.toContain("?skill=");
});

test("EVAL-002 a suggestion that cannot be applied names the rule that blocked it", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");
  await waitFor(() => (container.textContent ?? "").includes("查看差異"));

  const button = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "查看差異",
  );
  expect(button).toBeDefined();
  await act(async () => button?.click());
  await waitFor(() => (container.textContent ?? "").includes("目前無法套用"));

  expect(container.textContent).toContain("目標檔案已經和建議產生當時不同");
});
