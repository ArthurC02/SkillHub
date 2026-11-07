import { act } from "react";
import { expect, test } from "vitest";
import {
  BATCH,
  BATCH_REPORT,
  PREFLIGHT,
  RUN,
  SKILL,
  TEST_CASE,
  TEST_CASE_DRAFT,
  VERSION,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";
import type { BatchReport } from "./batches.model";
import type { BatchReflection } from "./reflection.model";

const app = renderedApp();

const LISTED = {
  ...TEST_CASE_DRAFT,
  skill_name: "PDF Summariser",
  criteria_confirmed: 1,
  criteria_total: 2,
  has_rubric: true,
};

const SECOND_CASE = "44444444-4444-4444-8444-444444444444";
const NEW_VERSION = "77777777-7777-4777-8777-777777777777";
const REFLECTION = "88888888-8888-4888-8888-888888888888";
const BATCH_PAGE = `/skills/${SKILL}/evaluation-batches/${BATCH}`;
const REFLECTIONS = `/skills/${SKILL}/evaluation-batches/${BATCH}/reflections`;

const TWO_CASE_REPORT: BatchReport = {
  ...BATCH_REPORT,
  test_cases: [
    BATCH_REPORT.test_cases[0],
    {
      ...BATCH_REPORT.test_cases[0],
      test_case_id: SECOND_CASE,
      trials: [{ trial: 1, run_id: RUN, outcome: "running" }],
    },
  ],
};

function reflection(overrides: Partial<BatchReflection> = {}): BatchReflection {
  return {
    reflection_id: REFLECTION,
    batch_id: BATCH,
    dev_test_case_ids: [TEST_CASE],
    outcome: "kept",
    runs_read: 2,
    runs_not_read: 0,
    candidates: [],
    evaluation: null,
    created_at: "2026-11-07T01:00:00Z",
    suggestions: [
      {
        suggestion_id: "r1",
        category: "skill",
        problem: "SKILL.md 沒說怎麼判斷重複列。",
        evidence: [],
        target_path: "SKILL.md",
        expected_impact: "下一次會先比對每一欄。",
        decision: "accepted",
      },
    ],
    ...overrides,
  };
}

function withTwoCases(page: ReturnType<typeof app>, reflections: BatchReflection[] = []) {
  page.route(`**/evaluation-batches/${BATCH}`, () => ({ json: TWO_CASE_REPORT }));
  page.route("**/test-cases**", (request) =>
    request.url.pathname === "/test-cases"
      ? {
          json: {
            test_cases: [
              { ...LISTED, name: "去重複列" },
              { ...LISTED, test_case_id: SECOND_CASE, name: "合併儲存格" },
            ],
          },
        }
      : undefined,
  );
  page.route(`**${REFLECTIONS}`, (request) =>
    request.method === "GET" ? { json: { reflections } } : undefined,
  );
}

function reflectPosts(page: ReturnType<typeof app>) {
  return page.requests.filter((r) => r.method === "POST" && r.url.pathname === REFLECTIONS);
}

function reasonOf(page: ReturnType<typeof app>, name: string) {
  const id = page.getByRole("button", { name }).attribute("aria-describedby");
  return id ? document.getElementById(id)?.textContent : undefined;
}

test("finding improvements waits for a development case, and for one left held out", async () => {
  const page = app();
  withTwoCases(page);
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("checkbox", { name: "去重複列" }).count()).toBe(1),
  );
  const find = page.getByRole("button", { name: "找改進" });
  expect(find.attribute("aria-disabled")).toBe("true");
  expect(reasonOf(page, "找改進")).toBe("先勾至少一題開發題。");
  await page.getByRole("checkbox", { name: "去重複列" }).check();
  await page.getByRole("checkbox", { name: "合併儲存格" }).check();
  await page.eventually(() =>
    expect(reasonOf(page, "找改進")).toBe("至少留一題不勾，之後用它檢查改進有沒有用。"),
  );
  await act(async () => find.element().click());
  expect(reflectPosts(page)).toHaveLength(0);
});

test("a development case still running holds the request, a held-out one does not", async () => {
  const page = app();
  withTwoCases(page);
  await page.goto(BATCH_PAGE);
  await page.getByRole("checkbox", { name: "合併儲存格" }).check();
  await page.eventually(() =>
    expect(reasonOf(page, "找改進")).toBe(
      "勾選的開發題還有試跑沒跑完或還沒評估完，等它們都有結果再找改進。",
    ),
  );
  await page.getByRole("checkbox", { name: "合併儲存格" }).click();
  await page.getByRole("checkbox", { name: "去重複列" }).check();
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "找改進" }).attribute("aria-disabled")).toBe("false"),
  );
});

test("finding improvements sends only the development cases and shows what it read", async () => {
  const page = app();
  withTwoCases(page);
  const made: BatchReflection[] = [];
  page.route(`**${REFLECTIONS}`, (request) => {
    if (request.method !== "POST") return { json: { reflections: made } };
    made.unshift(reflection({ runs_read: 8, runs_not_read: 3 }));
    return { status: 201, json: made[0] };
  });
  await page.goto(BATCH_PAGE);
  await page.getByRole("checkbox", { name: "去重複列" }).check();
  await act(async () => page.getByRole("button", { name: "找改進" }).element().click());
  await page.eventually(() => expect(reflectPosts(page)).toHaveLength(1));
  expect(reflectPosts(page)[0].body).toEqual({
    dev_test_case_ids: [TEST_CASE.toLowerCase()],
  });
  await page.eventually(() =>
    expect(
      page
        .getByRole("status")
        .filter({ hasText: "讀了開發題 1 題裡 8 次失敗的試跑；另外 1 題留著檢查，沒有給模型看。" })
        .filter({ hasText: "還有 3 次失敗的試跑沒有讀，這次只看了一部分。" })
        .count(),
    ).toBe(1),
  );
  expect(page.getByText("SKILL.md 沒說怎麼判斷重複列。").count()).toBe(1);
});

test.each([
  [
    "unavailable",
    "這次找改進時模型沒有回應，所以沒有建議；這不代表小工具沒有可以改進的地方，可以再試一次。",
  ],
  ["not_needed", "開發題的試跑都符合，也沒有需要處理的發現，這次沒有呼叫模型。"],
] as const)(
  "an empty %s round says why rather than reading as nothing to fix",
  async (outcome, sentence) => {
    const page = app();
    withTwoCases(page, [reflection({ outcome, suggestions: [] })]);
    await page.goto(BATCH_PAGE);
    await page.eventually(() => expect(page.getByText(sentence).count()).toBe(1));
  },
);

test("a refused round shows the server's reason", async () => {
  const page = app();
  withTwoCases(page);
  page.route(`**${REFLECTIONS}`, (request) =>
    request.method === "POST"
      ? {
          status: 422,
          json: { error: "點數不足，無法開始找改進，沒有呼叫模型。請聯絡管理者為這個帳號加點。" },
        }
      : undefined,
  );
  await page.goto(BATCH_PAGE);
  await page.getByRole("checkbox", { name: "去重複列" }).check();
  await act(async () => page.getByRole("button", { name: "找改進" }).element().click());
  await page.eventually(() =>
    expect(
      page
        .getByRole("alert")
        .filter({ hasText: "點數不足，無法開始找改進，沒有呼叫模型。" })
        .count(),
    ).toBe(1),
  );
});

test("applying a round's suggestions names the round and links a re-run of the held-out cases", async () => {
  const page = app();
  withTwoCases(page, [reflection()]);
  page.route(`**/skills/${SKILL}/versions/from-suggestions`, () => ({
    status: 201,
    json: {
      skill_id: SKILL,
      version_id: NEW_VERSION,
      version_number: 3,
      content_hash: "sha256:new",
      duplicate: false,
      applied_suggestion_ids: ["r1"],
      rejected_suggestions: [],
    },
  }));
  await page.goto(BATCH_PAGE);
  const build = "以已接受的 1 項建議建立新版本";
  await page.eventually(() => expect(page.getByRole("button", { name: build }).count()).toBe(1));
  await act(async () => page.getByRole("button", { name: build }).element().click());
  await page.eventually(() =>
    expect(page.requests.filter((r) => r.url.pathname.endsWith("/from-suggestions"))).toHaveLength(
      1,
    ),
  );
  const sent = page.requests.find((r) => r.url.pathname.endsWith("/from-suggestions"));
  expect(sent?.body).toEqual({ reflection_id: REFLECTION, suggestion_ids: ["r1"] });
  const rerun = page.getByRole("link", { name: "只用沒給模型看的 1 題重跑新版本" });
  await page.eventually(() => expect(rerun.count()).toBe(1));
  const href = new URL(String(rerun.attribute("href")), "http://localhost");
  expect(href.pathname).toBe(`/skills/${SKILL}/evaluation-batches`);
  expect(Object.fromEntries(href.searchParams)).toEqual({
    version: NEW_VERSION,
    rerun: BATCH,
    reflection: REFLECTION,
  });
});

test.each([
  ["not yet evaluated", () => reflection(), true],
  [
    "already evaluated",
    () => evaluated({ candidates: [standing(0)], recommended: 0, adopted: null }),
    false,
  ],
])(
  "building from accepted suggestions points to comparing the candidates only while a round is %s",
  async (_name, round, pointsToCandidates) => {
    const page = app();
    withTwoCases(page, [round()]);
    await page.goto(BATCH_PAGE);
    const build = "以已接受的 1 項建議建立新版本";
    await page.eventually(() => expect(reasonOf(page, build)).toContain("覆寫這一批跑過的版本"));
    expect((reasonOf(page, build) ?? "").includes("想先比較各候選，用下方的「評估候選」")).toBe(
      pointsToCandidates,
    );
  },
);

test("a re-run from a round prefills only the held-out cases", async () => {
  const page = app();
  withTwoCases(page, [reflection()]);
  await page.goto(
    `/skills/${SKILL}/evaluation-batches?version=${VERSION}&rerun=${BATCH}&reflection=${REFLECTION}`,
  );
  const box = (name: string) =>
    (page.getByRole("checkbox", { name }).element() as HTMLInputElement).checked;
  await page.eventually(() => {
    expect(page.getByRole("status").filter({ hasText: "沒給模型看的 1 題" }).count()).toBe(1);
    expect(box("合併儲存格")).toBe(true);
    expect(box("去重複列")).toBe(false);
  });
});

test("a re-run from a round that cannot be found prefills nothing", async () => {
  const page = app();
  withTwoCases(page, []);
  await page.goto(
    `/skills/${SKILL}/evaluation-batches?version=${VERSION}&rerun=${BATCH}&reflection=${REFLECTION}`,
  );
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "找不到那一次找改進的紀錄" }).count()).toBe(1),
  );
  expect(page.getByRole("button", { name: "開始評估批次" }).count()).toBe(0);
});

test("a round of several candidates lists each candidate's suggestions under its focus", async () => {
  const page = app();
  const first = reflection().suggestions[0];
  withTwoCases(page, [
    reflection({
      suggestions: [first, { ...first, suggestion_id: "r2", problem: "Script 沒有處理空白欄。" }],
      candidates: [
        { focus: "instructions", outcome: "kept", suggestion_ids: ["R1"] },
        { focus: "scripts_and_tools", outcome: "kept", suggestion_ids: ["r2"] },
        { focus: "references_and_examples", outcome: "unavailable", suggestion_ids: [] },
      ],
    }),
  ]);
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("heading", { name: "候選 3：先看參考資料與範例" }).count()).toBe(1),
  );
  const headings = [...document.querySelectorAll("h4")].map((h) => h.textContent);
  expect(headings).toEqual([
    "候選 1：先看指示",
    "候選 2：先看 Script 與工具",
    "候選 3：先看參考資料與範例",
  ]);
  const groups = [...document.querySelectorAll("h4")].map(
    (h) => h.parentElement?.textContent ?? "",
  );
  expect(groups[0]).toContain("SKILL.md 沒說怎麼判斷重複列。");
  expect(groups[0]).not.toContain("Script 沒有處理空白欄。");
  expect(groups[1]).toContain("Script 沒有處理空白欄。");
  expect(groups[2]).toContain(
    "這次找改進時模型沒有回應，所以沒有建議；這不代表小工具沒有可以改進的地方，可以再試一次。",
  );
});

test("a round of one candidate lists its suggestions without candidate headings", async () => {
  const page = app();
  withTwoCases(page, [reflection()]);
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(document.body.textContent).toContain("SKILL.md 沒說怎麼判斷重複列。"),
  );
  expect(document.querySelectorAll("h4")).toHaveLength(0);
});

const CANDIDATE_A = "a1a1a1a1-a1a1-4a1a-8a1a-a1a1a1a1a1a1";
const CANDIDATE_B = "b2b2b2b2-b2b2-4b2b-8b2b-b2b2b2b2b2b2";
const EVALUATE = `${REFLECTIONS}/${REFLECTION}/evaluate`;
const ADOPT = `${REFLECTIONS}/${REFLECTION}/adopt`;

const paired = (verdict: string, mean: number | null = 0.25) => ({
  verdict,
  mean_difference: mean,
  low: mean === null ? null : mean - 0.2,
  high: mean === null ? null : mean + 0.2,
});

function standing(candidate: number, overrides: Record<string, unknown> = {}) {
  return {
    candidate,
    skill_version_id: candidate === 0 ? CANDIDATE_A : CANDIDATE_B,
    batch_id: `c${candidate}c${candidate}c${candidate}c${candidate}-0000-4000-8000-00000000000${candidate}`,
    skipped_suggestion_ids: [],
    settled: true,
    kept: true,
    dev: paired("improved"),
    held_out: paired("likely_improved"),
    ...overrides,
  };
}

function evaluated(evaluation: Record<string, unknown>) {
  return reflection({ evaluation: evaluation as BatchReflection["evaluation"] });
}

function postsTo(page: ReturnType<typeof app>, path: string) {
  return page.requests.filter((r) => r.method === "POST" && r.url.pathname === path);
}

test("evaluating names the suggestions it accepts and leaves the rejected ones out", async () => {
  const page = app();
  const base = reflection().suggestions[0];
  const round = reflection({
    suggestions: [
      { ...base, suggestion_id: "p1", decision: "pending" },
      { ...base, suggestion_id: "p2", decision: "pending" },
      { ...base, suggestion_id: "x1", decision: "rejected" },
    ],
  });
  const rounds = [round];
  withTwoCases(page, rounds);
  page.route(`**${EVALUATE}`, () => {
    rounds[0] = evaluated({
      candidates: [standing(0, { settled: false })],
      recommended: null,
      adopted: null,
    });
    return { json: rounds[0] };
  });
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(reasonOf(page, "評估 1 個候選")).toContain("還沒決定的 2 項建議"),
  );
  expect(reasonOf(page, "評估 1 個候選")).toContain("已拒絕的 1 項不會用");
  expect(reasonOf(page, "評估 1 個候選")).toContain("這一批的 2 題");
  await act(async () => page.getByRole("button", { name: "評估 1 個候選" }).element().click());
  await page.eventually(() => expect(postsTo(page, EVALUATE)).toHaveLength(1));
  await page.eventually(() =>
    expect(
      page.getByRole("status").filter({ hasText: "候選的批次跑完後才會給建議。" }).count(),
    ).toBe(1),
  );
});

test("confirming every listed pair sends the hash each pair showed, then evaluates again", async () => {
  const page = app();
  withTwoCases(page, [reflection()]);
  let evaluations = 0;
  page.route(`**${EVALUATE}`, () => {
    evaluations += 1;
    return evaluations === 1
      ? {
          status: 422,
          json: {
            error: "這幾題在候選版本上要先確認試跑權限；候選批次還沒開。",
            unconfirmed_pairs: [
              { skill_version_id: CANDIDATE_A, test_case_id: TEST_CASE },
              { skill_version_id: CANDIDATE_B, test_case_id: SECOND_CASE },
            ],
          },
        }
      : { json: evaluated({ candidates: [standing(0)], recommended: 0, adopted: null }) };
  });
  page.route("**/runs/preflight**", (request) => {
    if (request.url.pathname.endsWith("/confirm"))
      return { json: { confirmed: true, summary_hash: "x" } };
    const version = request.url.searchParams.get("version_id") ?? "";
    const testCase = request.url.searchParams.get("test_case_id") ?? "";
    return {
      json: {
        ...PREFLIGHT,
        summary_hash: `hash-${version.slice(0, 2)}`,
        summary: { ...PREFLIGHT.summary, skill_version_id: version, test_case_id: testCase },
      },
    };
  });
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "評估 1 個候選" }).count()).toBe(1),
  );
  await act(async () => page.getByRole("button", { name: "評估 1 個候選" }).element().click());
  const confirmAll = "我看過了，確認列出的 2 題";
  await page.eventually(() =>
    expect(page.getByRole("button", { name: confirmAll }).attribute("aria-disabled")).toBe("false"),
  );
  await act(async () => page.getByRole("button", { name: confirmAll }).element().click());
  await page.eventually(() => expect(evaluations).toBe(2));
  const confirms = page.requests.filter(
    (r) => r.method === "POST" && r.url.pathname === `/skills/${SKILL}/runs/preflight/confirm`,
  );
  expect(confirms.map((r) => r.body)).toEqual([
    { version_id: CANDIDATE_A, test_case_id: TEST_CASE, summary_hash: "hash-a1" },
    { version_id: CANDIDATE_B, test_case_id: SECOND_CASE, summary_hash: "hash-b2" },
  ]);
});

test("confirming every pair waits until each pair's summary has been shown", async () => {
  const page = app();
  withTwoCases(page, [reflection()]);
  page.route(`**${EVALUATE}`, () => ({
    status: 422,
    json: {
      error: "這幾題在候選版本上要先確認試跑權限；候選批次還沒開。",
      unconfirmed_pairs: [{ skill_version_id: CANDIDATE_A, test_case_id: TEST_CASE }],
    },
  }));
  page.route("**/runs/preflight**", () => ({
    status: 503,
    json: { error: "暫時無法產生權限摘要。" },
  }));
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "評估 1 個候選" }).count()).toBe(1),
  );
  await act(async () => page.getByRole("button", { name: "評估 1 個候選" }).element().click());
  const confirmAll = "我看過了，確認列出的 1 題";
  await page.eventually(() =>
    expect(page.getByRole("button", { name: confirmAll }).count()).toBe(1),
  );
  expect(page.getByRole("button", { name: confirmAll }).attribute("aria-disabled")).toBe("true");
  await act(async () => page.getByRole("button", { name: confirmAll }).element().click());
  expect(page.requests.filter((r) => r.url.pathname.endsWith("/preflight/confirm"))).toHaveLength(
    0,
  );
});

test("the recommended candidate is named, its dev verdict is not the deciding one, and adopting sends it", async () => {
  const page = app();
  const round = evaluated({
    candidates: [
      standing(0),
      standing(1, { kept: false, held_out: paired("no_clear_difference", 0) }),
    ],
    recommended: 0,
    adopted: null,
  });
  const rounds = [round];
  withTwoCases(page, rounds);
  page.route(`**${ADOPT}`, () => {
    rounds[0] = evaluated({
      candidates: [standing(0), standing(1)],
      recommended: 0,
      adopted: { candidate: 0, skill_version_id: CANDIDATE_A, adopted_at: "2026-11-07T02:00:00Z" },
    });
    return { json: rounds[0] };
  });
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("status").filter({ hasText: "建議採用候選 1" }).count()).toBe(1),
  );
  expect(document.body.textContent).toContain("開發題（不作為判斷依據）：比原版好，差距明顯。");
  expect(document.body.textContent).toContain("在每一題開發題上都沒有勝過其他候選與原版，淘汰");
  await act(async () => page.getByRole("button", { name: "採用候選 1" }).element().click());
  await page.eventually(() =>
    expect(postsTo(page, ADOPT).map((r) => r.body)).toEqual([{ candidate: 0 }]),
  );
  await page.eventually(() =>
    expect(
      page
        .getByRole("status")
        .filter({ hasText: "已採用候選 1，它現在是這個小工具正式路線上的最新版本。" })
        .count(),
    ).toBe(1),
  );
  expect(page.getByRole("button", { name: "採用候選 2" }).count()).toBe(0);
});

test.each([
  [
    "every batch settled and none qualifies",
    [standing(0, { held_out: paired("no_clear_difference", 0) })],
    "沒有候選在沒給模型看的題目上明顯變好，建議保留目前的版本。",
  ],
  ["a batch still running", [standing(0, { settled: false })], "候選的批次跑完後才會給建議。"],
])("with no recommendation and %s the page says so", async (_name, candidates, sentence) => {
  const page = app();
  withTwoCases(page, [evaluated({ candidates, recommended: null, adopted: null })]);
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("status").filter({ hasText: sentence }).count()).toBe(1),
  );
});

test("an unavailable candidate says it has no version and offers no adoption", async () => {
  const page = app();
  withTwoCases(page, [
    evaluated({
      candidates: [
        standing(0, { skill_version_id: null, batch_id: null, skipped_suggestion_ids: ["r1"] }),
      ],
      recommended: null,
      adopted: null,
    }),
  ]);
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(document.body.textContent).toContain("沒有可以套用的建議，所以沒有建版本。"),
  );
  expect(document.body.textContent).toContain("有 1 項建議沒有放進這個版本");
  expect(page.getByRole("button", { name: "採用候選 1" }).count()).toBe(0);
});
