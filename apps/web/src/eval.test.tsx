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
  vi.useRealTimers();
  container.remove();
  vi.unstubAllGlobals();
});

test("a terminal run polls evaluation from 404 through pending to completed", async () => {
  vi.useFakeTimers();
  let calls = 0;
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/evaluation/revisions")) return json({ revisions: [] });
    if (url.includes("/evaluation")) {
      calls++;
      if (calls === 1) return json({ error: "not found" }, 404);
      if (calls === 2) return json({ ...evaluation, status: "pending", summary: "poll pending" });
      return json({ ...evaluation, status: "completed", summary: "poll complete" });
    }
    return json({ error: "not found" }, 404);
  });

  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <EvaluationPanel runId={RUN} runStatus="succeeded" />
      </QueryClientProvider>,
    );
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(calls).toBe(1);

  await act(async () => vi.advanceTimersByTimeAsync(3000));
  expect(calls).toBe(2);
  await act(async () => vi.advanceTimersByTimeAsync(3000));
  expect(calls).toBe(3);
  expect(queryClient.getQueryData(["evaluation", RUN, "current"])).toMatchObject({
    status: "completed",
    summary: "poll complete",
  });

  await act(async () => vi.advanceTimersByTimeAsync(6000));
  expect(calls).toBe(3);
});

type LinkProps = {
  to: string;
  params?: Record<string, string>;
  search?: Record<string, string | undefined>;
  children?: unknown;
};

// Resolves the destination into an href so a test can assert where a link goes,
// not merely that some link text rendered. EVAL-011 is entirely about the ids in
// that destination, so a mock that dropped them would assert nothing.
vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, params, search, children }: LinkProps) => {
    const path = Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to);
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter((e): e is [string, string] => e[1] !== undefined),
    ).toString();
    return <a href={query ? `${path}?${query}` : path}>{children as never}</a>;
  },
}));

const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const SKILL = "11111111-1111-1111-1111-111111111111";
const TEST_CASE = "44444444-4444-4444-4444-444444444444";
const NEW_VERSION = "55555555-5555-5555-5555-555555555555";

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
        // ADR-043: the judge filed this under `artifact` and the quote was
        // actually in the agent's own output. Mis-filed, not fabricated.
        {
          kind: "agent_output",
          reattributed_from: "artifact",
          match: "normalized",
          available: true,
          excerpt: "已移除 email 欄位",
          excerpt_truncated: false,
        },
        // The manifest row. Proves the file is there and nothing about what is
        // in it — the archive is never opened in the control plane.
        {
          kind: "artifact",
          artifact_path: "out/cleaned.csv",
          match: "not_checked",
          available: true,
          excerpt: "cleaned.csv (2048 bytes, text/csv, sha256:dddd)",
          excerpt_truncated: false,
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
    // 04 丙-10: the judge had a verdict and the platform threw it away because
    // the citation did not re-verify. Marked by the reason's prefix, which is
    // where judge.go's defence 3 puts it.
    {
      criterion_id: "c3",
      text: "報告有貼出正文",
      result: "undetermined",
      source: "model",
      reason:
        "evidence_unverifiable: no trace event 0f0a... was sent to the judge. " +
        "The judge's own reasoning was: the report quotes the body in full.",
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

function stubPlatform(options: {
  evaluated: boolean;
  /** The evaluation row exists but the judge has not finished (04 丙-35). */
  pending?: boolean;
  /** The suggestion arrives already accepted, so the apply button is enabled. */
  accepted?: boolean;
  /** False: the editable test case this run was frozen from no longer resolves. */
  testCase?: boolean;
}) {
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
        ...(options.testCase === false ? {} : { test_case_id: TEST_CASE }),
      });
    }
    if (!options.evaluated) return json({ error: "not found" }, 404);
    if (url.includes("/versions/from-suggestions")) {
      return json(
        {
          skill_id: SKILL,
          version_id: NEW_VERSION,
          version_number: 3,
          content_hash: "sha256:aaaa",
          duplicate: false,
          applied_suggestion_ids: ["s1"],
          rejected_suggestions: [],
        },
        201,
      );
    }
    if (url.includes("/evaluation/revisions")) return json({ revisions: [] });
    if (url.includes("/evaluation"))
      return json(options.pending ? { ...evaluation, status: "pending" } : evaluation);
    if (url.includes("/suggestions/s1/diff")) return json(blockedDiff);
    if (url.includes("/suggestions"))
      return json({
        evaluation_id: "eval-1",
        suggestions: [options.accepted ? { ...suggestion, decision: "accepted" } : suggestion],
      });
    return json({ error: "not found" }, 404);
  });
}

/** Renders, presses 建立新版本, and waits for the result block. */
async function applyAccepted() {
  await render("succeeded");
  await waitFor(() => (container.textContent ?? "").includes("建立新版本"));
  const apply = Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("建立新版本"),
  );
  expect(apply?.disabled).toBe(false);
  await act(async () => apply?.click());
  await waitFor(() => (container.textContent ?? "").includes("已建立新版本"));
}

function rerunLink(): HTMLAnchorElement | undefined {
  return Array.from(container.querySelectorAll("a")).find((a) =>
    a.getAttribute("href")?.startsWith("/lab/run"),
  );
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
  await waitFor(() => container.querySelector("[data-loading]") === null);
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

test("§2.12 a judge still running is 進行中, not a verdict — and says you may leave", async () => {
  // `status: "pending"` is a row saying the judgement is being made right now.
  // It used to go to EvaluationReport like any other answer, which renders a
  // verdict block for a verdict nobody has reached. 進行中 is a third axis.
  stubPlatform({ evaluated: true, pending: true });
  await render("succeeded");

  const text = container.textContent ?? "";
  expect(text).toContain("評估進行中");
  // The three sentences §2.12 asks for.
  expect(text).toContain("會自己完成");
  expect(text).toContain("可以關掉這一頁");
  // And the one it cannot answer, said rather than faked: a judge returns a
  // verdict or fails, so there is no intermediate count to report.
  expect(text).toContain("沒有進度可以報");
  // Not a verdict, and not the absence of one either.
  expect(text).not.toContain("未評估");
});

test("§2.12 未評估 stays 未評估 — a 404 is not evidence that a judge is coming", async () => {
  // useEvaluation polls a 404 too, because a judge that has not started has no
  // row. But an old run nobody ever evaluated 404s forever, and 「評估進行中」
  // there would be a promise with nothing enforcing it (§2.2). What the page may
  // say is what it is actually doing: re-checking.
  stubPlatform({ evaluated: false });
  await render("succeeded");

  const text = container.textContent ?? "";
  expect(text).toContain("未評估");
  expect(text).not.toContain("評估進行中");
  expect(text).toContain("每 3 秒再查一次");
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

test("ADR-043 a citation says whether its quote was verified, and where it was filed", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");
  const text = container.textContent ?? "";

  // 04 丙-41: both fields landed server-side and neither reached the screen.
  // The ADR says it in its own §影響 — an audit field that never reaches the
  // screen is not an audit field.

  // Mis-filed and fabricated are different failures, and the report has to be
  // able to show which one happened. This one is the first.
  expect(text).toContain("Judge 原本標為");
  expect(text).toContain("標錯來源與捏造引文是兩件不同的事");
  // G8: two correct verdicts were lost to a trailing punctuation mark. One
  // glance at this line instead of a regression report.
  expect(text).toContain("正規化後才比對得上");
  // An artifact citation proves the file exists. Saying 「找不到」 would be an
  // accusation about a quote nobody ever checked.
  expect(text).toContain("沒有回驗任何引文");
  expect(text).not.toContain("在本次 Run 的可回驗來源裡找不到");
  // The trace citation in this fixture predates the field entirely. Silence is
  // not a pass: it says the report cannot answer.
  expect(text).toContain("還沒有記錄引文回驗結果");
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

test("EVAL-011 the new version's id is handed to the preflight screen, not to the address bar", async () => {
  stubPlatform({ evaluated: true, accepted: true });
  await applyAccepted();

  // The whole of the gap this test exists for: after a version is built, the
  // three ids a re-run needs are in a link on the screen.
  const href = rerunLink()?.getAttribute("href") ?? "";
  const params = new URLSearchParams(href.slice(href.indexOf("?")));
  expect(params.get("skill")).toBe(SKILL);
  expect(params.get("version")).toBe(NEW_VERSION);
  expect(params.get("test_case")).toBe(TEST_CASE);

  // A link to the permission screen, never a re-run started from here (TEST-009).
  const text = container.textContent ?? "";
  expect(text).toContain("以新版本重跑這個 Test Case");
  expect(text).toContain("執行前權限確認畫面");
});

test("EVAL-011 a run whose test case no longer resolves says so instead of inventing an id", async () => {
  stubPlatform({ evaluated: true, accepted: true, testCase: false });
  await applyAccepted();

  // ADR-003: no link at all rather than one pointing at something that is gone.
  expect(rerunLink()).toBeUndefined();
  expect(container.textContent).toContain("無法從這裡以相同輸入重跑新版本");
  // The version was still built, and the page still says where it lives.
  expect(container.textContent).toContain("已建立新版本");
});

test("丙-10 a verdict downgraded for unverifiable evidence is not shown as a judge who does not know", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");

  const items = Array.from(container.querySelectorAll("li.criterion"));
  const downgraded = items.find((li) => (li.textContent ?? "").includes("報告有貼出正文"));
  const uncertain = items.find((li) => (li.textContent ?? "").includes("沒有刪掉原始列"));

  // Channel one: different words, on the badge and in the sentence beside it.
  expect(downgraded?.querySelector(".badge")?.textContent).toBe("證據無法回驗");
  expect(downgraded?.textContent).toContain("平台降級");
  expect(downgraded?.textContent).toContain("這不是「模型自己說不知道」");

  // Channel two: a class of its own, so the two states differ visually as well
  // (NFR-007 forbids leaning on colour alone, hence both channels).
  expect(downgraded?.className).toContain("criterion-unverifiable");
  expect(uncertain?.className).not.toContain("criterion-unverifiable");

  // The plain undetermined case keeps its own wording and is not relabelled.
  expect(uncertain?.querySelector(".badge")?.textContent).toBe("無法判斷");
  expect(uncertain?.textContent).not.toContain("平台降級");
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
