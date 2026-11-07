import { act } from "react";
import { expect, test } from "vitest";
import {
  BATCH,
  BATCH_REPORT,
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
