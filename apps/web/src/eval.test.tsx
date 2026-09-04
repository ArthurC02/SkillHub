import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { EvaluationPanel, MATCH_NOTE } from "./pages/RunEvaluation";
import { RunVerdict } from "./components/RunVerdict";
import { EVALUATION_POLL_MAX_404, EVALUATION_POLL_MAX_PENDING } from "./api/evaluation";
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

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

beforeEach(() => {
  queryClient.clear();
  setSearch({});
  navigations.length = 0;
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
/*
 * The address, standing in for the router. 資訊架構 §0.1 R4 puts the evaluation
 * revision in it — a superseded verdict is a different verdict, not a way of
 * looking at the same one.
 *
 * Reads come from `search`, which a test sets before it renders; writes are
 * recorded rather than applied, because a switcher that keeps its choice in
 * component state performs no navigation at all, and that is the thing that has
 * to fail.
 */
let search: Record<string, string | undefined> = {};
const navigations: Record<string, string | undefined>[] = [];

function setSearch(next: Record<string, string | undefined>) {
  search = next;
}

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, params, search: linkSearch, children }: LinkProps) => {
    const path = Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to);
    const query = new URLSearchParams(
      Object.entries(linkSearch ?? {}).filter((e): e is [string, string] => e[1] !== undefined),
    ).toString();
    return <a href={query ? `${path}?${query}` : path}>{children as never}</a>;
  },
  useSearch: () => search,
  useNavigate: () => (options: { search?: unknown }) => {
    const next =
      typeof options.search === "function"
        ? (options.search as (prev: typeof search) => typeof search)(search)
        : (options.search as typeof search);
    navigations.push({ ...next });
    return Promise.resolve();
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
        // A second `normalized` citation, and the whole point of it: 設計
        // §2.13 去重 1 says the explanation belongs to the list. With one of
        // each kind the dedup assertion below would pass on either shape.
        {
          kind: "agent_output",
          match: "normalized",
          available: true,
          excerpt: "欄位：name, phone",
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
    // The fourth criterion, and the second `model` one: 設計 §2.13 hoists
    // 判定來源 only when it would otherwise be printed more than once, so a
    // three-row fixture could not tell the hoist from the row-by-row version.
    {
      criterion_id: "c4",
      text: "輸出是 UTF-8",
      result: "passed",
      source: "model",
      reason: "檔頭沒有 BOM，內容可解碼。",
      evidence: [
        {
          kind: "agent_output",
          match: "exact",
          available: true,
          excerpt: "encoding: utf-8",
          excerpt_truncated: false,
        },
      ],
    },
  ],
  deterministic_findings: [
    {
      category: "activation",
      severity: "warning",
      message: "沒有出現 Skill 啟用事件。",
      // §2.9 / §2.10 第 10 項: an absence whose type word may not be folded
      // away. It lives on the finding list so the criterion list's legend and
      // this one are two different sets, computed rather than shared.
      evidence: [
        {
          kind: "trace_event",
          match: "not_found",
          available: true,
          excerpt: "skill_activated",
          excerpt_truncated: false,
        },
      ],
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

/** A second suggestion, for the same reason c4 exists: 設計 §2.13 去重 1 is
 *  about a sentence printed N times, and N=1 proves nothing. */
const suggestion2: ImprovementSuggestion = {
  suggestion_id: "s2",
  category: "runtime",
  problem: "宣告的 runtime 版本與實際不符。",
  evidence: [],
  target_path: "SKILL.md",
  expected_impact: "Agent 會在正確的 runtime 上啟用這個 Skill。",
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
        suggestions: [
          options.accepted ? { ...suggestion, decision: "accepted" } : suggestion,
          suggestion2,
        ],
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
  // 設計 §2.13: 「會變的量永遠平鋪，不會變的理由才可以折」. The permission stays
  // flat and in the wording `components/InFlight.tsx` uses for the same queue on
  // the same page; the River/worker derivation behind it is D 類 and is gone
  // rather than said twice.
  expect(text).toContain("可以關掉這一頁（平台在跑，不是你的瀏覽器）");
  expect(text).not.toContain("evaluate_run");
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
  // accusation about a quote nobody ever checked — so the assertion is on that
  // row's own badge, not on the page's text: 找不到 IS on this page now, on the
  // finding that has it, and a page-wide `not.toContain` would only prove the
  // fixture had no such citation anywhere.
  expect(text).toContain("沒有回驗任何引文");
  expect(rowFor("out/cleaned.csv")?.querySelector(".badge")?.textContent).toBe("未回驗引文");
  // The trace citation in this fixture predates the field entirely. Silence is
  // not a pass: it says the report cannot answer.
  expect(text).toContain("還沒有記錄引文回驗結果");
});

/** The `.evidence-list` row whose citation quotes `needle`. */
function rowFor(needle: string): Element | undefined {
  return Array.from(container.querySelectorAll(".evidence-list > li")).find((li) =>
    (li.textContent ?? "").includes(needle),
  );
}

/*
 * 設計 §2.13 — 丙-142 第一批②. Measured 2026-09-03: 232 characters, 19% of this
 * page, were eight citations each carrying one of five fixed paragraphs about
 * re-verification. The result is a STATE, so it is a badge with a word in it,
 * and the paragraph is the LIST's fact.
 *
 * Both halves have to hold at once, which is why this is one test: the words
 * alone must still tell the five results apart (§2.3 — colour is the second
 * channel and these five share three tints), and the paragraph must be printed
 * once for the list however many citations carry it.
 */
test("§2.13 引文回驗結果是徽章，解釋在清單層級只印一次", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");

  // §2.10 第 10 項: 缺席「是哪一型」不可折 — every citation carries its own word,
  // in document order, and the two absences are among them.
  const criteria = container.querySelector("ul.criterion-list")!;
  expect(
    Array.from(criteria.querySelectorAll(".evidence-list > li > .note > .badge")).map(
      (b) => b.textContent,
    ),
  ).toEqual(["回驗結果未記錄", "正規化後比對", "正規化後比對", "未回驗引文", "已逐字回驗"]);
  expect(rowFor("skill_activated")?.querySelector(".badge")?.textContent).toBe("找不到");

  // …and the sentence behind each word is printed once for the whole list. Two
  // `normalized` citations, one explanation — that is the 232 characters.
  const text = container.textContent ?? "";
  expect(occurrences(text, "需要正規化後才比對得上")).toBe(1);
  expect(occurrences(text, "沒有回驗任何引文")).toBe(1);
  expect(occurrences(text, "還沒有記錄引文回驗結果")).toBe(1);

  // Counted from the data and not written down: the finding list holds exactly
  // one kind of citation, so its legend is one line — a hard-coded legend would
  // explain four cases that list does not contain. The same page has shipped
  // that mistake once already (§0 的分類計數).
  const findingLegend = container.querySelector(
    "ul.note + ul.finding-list",
  )!.previousElementSibling;
  expect(Array.from(findingLegend!.querySelectorAll("li")).map((li) => li.textContent)).toEqual([
    `找不到 ${MATCH_NOTE.not_found}`,
  ]);
});

function occurrences(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

/*
 * 設計 §4.7: an icon is the third signal and always rides beside the word, on
 * every badge that lands on a §4.4 row — never as a replacement for the word,
 * never on a badge that is a category. This checks the whole evaluation page's
 * worth of state badges at once (criterion verdicts, downgraded evidence, the
 * five citation re-verification badges) plus the one this suite does not
 * otherwise render: a `met` run's verdict badge, which is the RunVerdict.tsx
 * `pass` branch.
 */
test("§4.7 每個表狀態徽章都帶一個 aria-hidden 圖示，且圖示旁邊仍有詞", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");

  const stateBadges = Array.from(container.querySelectorAll(".badge")).filter((b) =>
    /-(failed|danger|unverifiable|undetermined|unverified|passed)\b/.test(b.className),
  );
  expect(stateBadges.length).toBeGreaterThan(0);
  for (const badge of stateBadges) {
    expect(badge.querySelectorAll('svg[aria-hidden="true"]')).toHaveLength(1);
    expect((badge.textContent ?? "").trim().length).toBeGreaterThan(0);
  }

  const verdictContainer = document.createElement("div");
  document.body.appendChild(verdictContainer);
  const verdictRoot = createRoot(verdictContainer);
  await act(async () => {
    verdictRoot.render(<RunVerdict verdict={{ value: "met", label: "符合", note: "" }} />);
  });
  const verdictBadge = verdictContainer.querySelector(".badge")!;
  expect(verdictBadge.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
  await act(async () => verdictRoot.unmount());
  verdictContainer.remove();
});

/*
 * 設計 §2.13 去重 1, and `components/RunVerdict.tsx` is the precedent its own
 * comment states: 「On a page of fifty rows a sentence per row is noise」.
 */
test("§2.13 判定來源在清單上方講一次，只有來源不同的那一條在列上覆寫", async () => {
  stubPlatform({ evaluated: true });
  await render("succeeded");
  const text = container.textContent ?? "";

  // Two of the three judged criteria are the model's, so that sentence is the
  // list's — once, above it.
  expect(occurrences(text, "模型評估（不是確定事實）")).toBe(1);
  // And the rule-sourced one still says so on its own row, because it differs.
  const uncertain = Array.from(container.querySelectorAll("li.criterion")).find((li) =>
    (li.textContent ?? "").includes("沒有刪掉原始列"),
  );
  expect(uncertain?.textContent).toContain("規則判定");
  // The row that agrees with the list does not repeat it.
  const passed = Array.from(container.querySelectorAll("li.criterion")).find((li) =>
    (li.textContent ?? "").includes("輸出是 UTF-8"),
  );
  expect(passed?.textContent).not.toContain("判定來源");

  // 預期影響's parenthesis is the same shape: C 類, so not a word of it goes —
  // but it belongs to the list of two suggestions, not to each of them.
  await waitFor(() => text.length > 0 && (container.textContent ?? "").includes("預期影響"));
  expect(occurrences(container.textContent ?? "", "模型的預測，不是量測結果")).toBe(1);
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

// --- the two things that used to run forever, and the one that never ran -----

test("EVAL-001 an old run with no evaluation stops being asked about, and says so", async () => {
  // The 404 poll had no bound: a March run nobody ever evaluated asked again
  // every three seconds for as long as the tab stayed open, under a line
  // promising 「結果會自己出現在這裡」. When the asking stops the sentence has to.
  vi.useFakeTimers();
  let calls = 0;
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/evaluation/revisions")) return json({ revisions: [] });
    if (url.includes("/evaluation")) calls++;
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
  await act(async () => vi.advanceTimersByTimeAsync(0));
  expect(calls).toBe(1);
  expect(container.textContent).toContain("結果會自己出現在這裡");

  // One short of the bound: still asking, still promising.
  await act(async () => vi.advanceTimersByTimeAsync(3000 * (EVALUATION_POLL_MAX_404 - 2)));
  expect(calls).toBe(EVALUATION_POLL_MAX_404 - 1);
  expect(container.textContent).toContain("結果會自己出現在這裡");

  await act(async () => vi.advanceTimersByTimeAsync(3000));
  expect(calls).toBe(EVALUATION_POLL_MAX_404);

  // And it stays there however long the tab is left open.
  await act(async () => vi.advanceTimersByTimeAsync(3000 * 100));
  expect(calls).toBe(EVALUATION_POLL_MAX_404);

  // 未評估 is still the answer — nothing failed. What changed is the promise.
  expect(container.textContent).toContain("未評估");
  expect(container.textContent).toContain("已經停止再查");
  expect(container.textContent).not.toContain("結果會自己出現在這裡");
});

test("EVAL-001 a judge that never finishes stops being polled, and the promise stops with it", async () => {
  // The other unbounded poll. A row stuck at `pending` — a worker that died, a
  // job nobody will retry — was asked for every three seconds until the tab
  // closed, under 「它會自己完成」. Counted on `dataUpdateCount`, because a pending
  // read is a success and never moves the `errorUpdateCount` the 404 bound uses.
  vi.useFakeTimers();
  let calls = 0;
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/evaluation/revisions")) return json({ revisions: [] });
    if (url.includes("/evaluation")) {
      calls++;
      return json({ ...evaluation, status: "pending" });
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
  await act(async () => vi.advanceTimersByTimeAsync(0));
  expect(calls).toBe(1);
  expect(container.textContent).toContain("每 3 秒自己查一次");

  // One short of the bound: still asking, still promising.
  await act(async () => vi.advanceTimersByTimeAsync(3000 * (EVALUATION_POLL_MAX_PENDING - 2)));
  expect(calls).toBe(EVALUATION_POLL_MAX_PENDING - 1);
  expect(container.textContent).toContain("每 3 秒自己查一次");

  await act(async () => vi.advanceTimersByTimeAsync(3000));
  expect(calls).toBe(EVALUATION_POLL_MAX_PENDING);

  // And it stays there however long the tab is left open.
  await act(async () => vi.advanceTimersByTimeAsync(3000 * 200));
  expect(calls).toBe(EVALUATION_POLL_MAX_PENDING);

  // 進行中 is still what the row says — this page just stopped watching it, and
  // says which of the two stopped.
  expect(container.textContent).toContain("評估進行中");
  expect(container.textContent).toContain("已經停止再查");
  expect(container.textContent).not.toContain("每 3 秒自己查一次");
  expect(container.textContent).not.toContain("會自己完成");
});

test("EVAL-002 a re-evaluation landing while the page is open brings the revision switcher with it", async () => {
  // ADR-026 / 設計 §2.5: two verdicts have to be distinguishable. `useEvaluation`
  // polls and the revision list did not, so the new verdict replaced the old
  // content with nothing on screen saying which one was being read.
  vi.useFakeTimers();
  let evaluations = 0;
  let revisionReads = 0;
  const second = { ...evaluation, evaluation_id: "eval-2", summary: "重評之後的判定。" };
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/evaluation/revisions")) {
      revisionReads++;
      return json({
        revisions:
          evaluations >= 2
            ? [revisionOf(second, null), revisionOf(evaluation, "2026-08-25T00:00:00Z")]
            : [revisionOf(evaluation, null)],
      });
    }
    if (url.includes("/evaluation")) {
      evaluations++;
      return json(evaluations === 1 ? { ...evaluation, status: "pending" } : second);
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
  // One verdict, one revision: nothing to choose between, so no switcher.
  expect(container.querySelector("#evaluation-revision")).toBeNull();
  const readsBefore = revisionReads;

  // Bounded rather than exactly one tick: which fake-clock tick the poll lands
  // on depends on how many renders happen before the interval is armed, and that
  // count is not what this test is about. The assertion is still 「the second
  // verdict arrives while the page is open」 — if it never arrives, this loop
  // runs out and the expect below fails.
  for (let i = 0; i < 4 && !container.textContent?.includes("重評之後的判定。"); i++) {
    await act(async () => vi.advanceTimersByTimeAsync(3000));
  }
  await act(async () => vi.advanceTimersByTimeAsync(0));

  expect(container.textContent).toContain("重評之後的判定。");
  expect(revisionReads).toBeGreaterThan(readsBefore);
  const picker = container.querySelector<HTMLSelectElement>("#evaluation-revision");
  expect(picker).not.toBeNull();
  expect(picker!.querySelectorAll("option")).toHaveLength(3); // 目前的判定 + 兩版
  expect(container.textContent).toContain("已被取代");
});

function revisionOf(source: Evaluation, supersededAt: string | null) {
  return {
    evaluation_id: source.evaluation_id,
    judge_prompt_version: source.judge_prompt_version,
    rubric_version: source.rubric_version,
    overall: source.overall,
    evaluated_at: source.evaluated_at,
    superseded_at: supersededAt,
  };
}

/*
 * 資訊架構 §0.1 R4 — 「你在看哪一份東西」進網址.
 *
 * IA §4 records `/runs/$runId` as carrying no search params, and justifies that
 * solely by the 一般／進階 reading-mode toggle (IA-4, 2026-08-23). The same page
 * also chooses between immutable `evaluation_id`s (ADR-003 / ADR-026), and that
 * control was never examined: a superseded verdict could not be linked or
 * reloaded into, and the address always snapped back to 目前的判定 — which for a
 * re-evaluated run is a DIFFERENT verdict from the one under discussion.
 */
test("R4: the evaluation revision comes from the address, and the switcher writes it back", async () => {
  const superseded = { ...evaluation, evaluation_id: "eval-1", summary: "被取代的那一份判定。" };
  const current = { ...evaluation, evaluation_id: "eval-2", summary: "重評之後的判定。" };
  const asked: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    asked.push(url);
    if (url.includes("/evaluation/revisions")) {
      return json({
        revisions: [revisionOf(current, null), revisionOf(superseded, "2026-08-25T00:00:00Z")],
      });
    }
    if (url.includes("/evaluation")) {
      return json(url.includes(`revision=${superseded.evaluation_id}`) ? superseded : current);
    }
    return json({ error: "not found" }, 404);
  });

  // An address somebody was sent, naming the verdict being discussed.
  setSearch({ evaluation: superseded.evaluation_id });
  await render("succeeded");

  // Read: the page opened on that revision rather than on 目前的判定.
  expect(asked.some((url) => url.includes(`revision=${superseded.evaluation_id}`))).toBe(true);
  expect(container.textContent).toContain("被取代的那一份判定。");
  const picker = container.querySelector<HTMLSelectElement>("#evaluation-revision")!;
  expect(picker.value).toBe(superseded.evaluation_id);

  // Write: switching back to 目前的判定 clears the parameter rather than leaving
  // the address naming a verdict nobody is reading.
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(picker, "");
    picker.dispatchEvent(new Event("change", { bubbles: true }));
  });
  expect(navigations).toHaveLength(1);
  expect(navigations[0].evaluation).toBeUndefined();
});
