import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { LicenseBadge, LicenseNotes } from "./components/LicenseBadge";
import { RiskIndicator } from "./components/RiskIndicator";
import type {
  PublicSearchResponse,
  PublicSearchResult,
  SkillDetail,
  SkillLicense,
  SkillRisk,
} from "./api/types";

// Renders against mocked fetch; no backend needed. The DOM plumbing below is
// deliberately hand-rolled — @testing-library is not a dependency of this app
// and these four assertions do not justify adding one.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  // The router is a module singleton shared across tests, so a test that
  // navigated has to be sent home before the next one mounts it. pushState is
  // patched by @tanstack/history, so this moves the router's location too.
  window.history.pushState({}, "", "/");
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function render(node: React.ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

/** Answers only the search call; /me stays 401 so the page renders logged-out. */
function stubSearch(body: PublicSearchResponse) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    calls.push(String(input));
    if (String(input).includes("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });
  return calls;
}

async function submitSearch(text: string) {
  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await waitFor(() => !container.textContent?.includes("搜尋中…"));
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (done()) return;
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

/**
 * The DISC-002 per-result columns every hit now carries. Spread into fixtures
 * so a test only spells out the field it is actually about.
 */
const HIT_FACETS = {
  tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
  risk: {
    scan_status: "scanned",
    level: "none",
    warnings: 0,
    note: "以上為靜態掃描結果。",
  },
  dependencies: [],
  compatibility: {
    spec_validation: "passed",
    capability: "unverified",
    runtime: "unverified",
    note: "尚未試跑。",
  },
} satisfies Pick<PublicSearchResult, "tier" | "risk" | "dependencies" | "compatibility">;

const EMPTY: PublicSearchResponse = {
  query: "",
  results: [],
  degraded: false,
  partial_index: false,
  no_results: false,
};

test("DISC-001: search hits the public endpoint, which needs no session", async () => {
  const calls = stubSearch({ ...EMPTY, query: "pdf 摘要", no_results: true });
  await render(<App />);
  await submitSearch("pdf 摘要");

  expect(calls.some((url) => url.includes("/api/skills/search?q=pdf"))).toBe(true);
  // The workspace-scoped route (which requires a session) must not be used.
  expect(calls.some((url) => /\/skills\/search\?/.test(url) && !url.includes("/api/"))).toBe(false);
});

test("DISC-005: no_results shows the server's query_suggestion, not hardcoded copy", async () => {
  stubSearch({
    ...EMPTY,
    query: "asdf",
    no_results: true,
    query_suggestion: "Try naming the file format you have.",
  });
  await render(<App />);
  await submitSearch("asdf");

  expect(container.textContent).toContain("Try naming the file format you have.");
});

test("DISC-002: each candidate shows its match reason, labelled by provenance", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        skill_id: "11111111-1111-1111-1111-111111111111",
        name: "PDF Summariser",
        summary: "把 PDF 轉成摘要",
        rank: 0.82,
        match_reason: "這個 Skill 直接處理 PDF 並輸出摘要。",
        match_reason_source: "model",
      },
      {
        ...HIT_FACETS,
        skill_id: "22222222-2222-2222-2222-222222222222",
        name: "Doc Splitter",
        summary: "切分文件",
        rank: 0.4,
        match_reason: "查詢與文件共同出現：pdf",
        match_reason_source: "template",
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(text).toContain("這個 Skill 直接處理 PDF 並輸出摘要。");
  expect(text).toContain("查詢與文件共同出現：pdf");
  // ADR-013: model-written copy carries a visible marker; template copy does not
  // borrow it.
  expect(container.querySelectorAll(".badge-source-model")).toHaveLength(1);
  expect(container.querySelectorAll(".badge-source-template")).toHaveLength(1);
});

test("DISC-005: degraded and partial_index are separate, non-blocking notices", async () => {
  stubSearch({
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        skill_id: "33333333-3333-3333-3333-333333333333",
        name: "Lexical Hit",
        summary: "只靠關鍵字命中",
        // Null, not the lexical score: a real FTS-only answer returned 1.4,
        // which is not the 0..1 cosine similarity the schema documents, so the
        // server withholds it and says why instead.
        rank: null,
        rank_note: "此頁改用關鍵字比對排序，未計算語意相似度。",
      },
    ],
    degraded: true,
    degraded_reason: "embedding unavailable; lexical search only",
    partial_index: true,
    no_results: false,
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(container.querySelectorAll(".notice")).toHaveLength(2);
  expect(text).toContain("embedding unavailable; lexical search only");
  // Non-blocking: the result is still listed.
  expect(text).toContain("Lexical Hit");
  // The out-of-range lexical score is never shown, nor called a similarity.
  expect(text).not.toMatch(/相似度\s*[\d.]/);
  expect(text).not.toContain("1.4");
  expect(text).toContain("此頁改用關鍵字比對排序，未計算語意相似度。");
});

test("DISC-002: an unranked hit on the hybrid path reads as unscored, not 0.00", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        skill_id: "44444444-4444-4444-4444-444444444444",
        name: "Pending Enrichment",
        summary: "尚未建立索引",
        rank: null,
        rank_note: "尚未建立語意索引，未評分。",
      },
    ],
    partial_index: true,
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(text).toContain("未評分");
  expect(text).not.toContain("相似度 0.00");
});

test("DISC-008: license shows both axes — expression and provenance tier", async () => {
  const license: SkillLicense = {
    expression: "MIT",
    source: "repo-license-file",
    source_note: "來自 repo 根目錄的 LICENSE，涵蓋整個 repo。",
    status: { value: "declared", label: "License 已宣告", note: "尚未經人工核對。" },
  };
  await render(
    <>
      <LicenseBadge license={license} />
      <LicenseNotes license={license} />
    </>,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("MIT");
  expect(text).toContain("已宣告"); // status axis: declared, not confirmed
  expect(text).toContain("repo 根目錄 LICENSE"); // provenance axis
  expect(text).toContain("涵蓋整個 repo");
});

test("DISC-008: unknown license never shows a name or implies permissiveness", async () => {
  const license: SkillLicense = {
    status: { value: "unknown", label: "License 未知", note: "未宣告 License，依規則不可下載。" },
  };
  await render(<LicenseBadge license={license} />);

  expect(container.textContent).toContain("License 未知");
  expect(container.querySelector(".license-expression")).toBeNull();
  expect(container.querySelector(".badge-license-source")).toBeNull();
});

const RISK: SkillRisk = {
  scan_status: "scanned",
  counts: { errors: 0, warnings: 1, infos: 321 },
  highlights: [
    {
      severity: "warning",
      code: "embedded-script",
      path: "SKILL.md",
      message: "SKILL.md 內含可執行程式碼區塊。",
    },
  ],
  info_counts: { "external-url": 320, "large-file": 1 },
  has_embedded_script: true,
  note: "以上為靜態掃描結果。",
};

test("DISC-008: warnings are up front, info findings aggregate behind a disclosure", async () => {
  await render(<RiskIndicator risk={RISK} />);

  const text = container.textContent ?? "";
  // Errors/warnings verbatim and not hidden.
  expect(text).toContain("SKILL.md 內含可執行程式碼區塊。");
  expect(container.querySelector(".risk-list .badge-risk")).not.toBeNull();
  // The embedded-script flag is its own visible marker (SKILL-003).
  expect(text).toContain("SKILL.md 內含可執行程式碼");

  // 321 info findings collapse to per-code counts inside <details>.
  const details = container.querySelector("details.risk-infos")!;
  expect(details).not.toBeNull();
  expect((details as HTMLDetailsElement).open).toBe(false);
  expect(details.textContent).toContain("external-url");
  expect(details.textContent).toContain("320");
  expect(details.querySelectorAll("li")).toHaveLength(2);
});

// ---- DISC-004: the ranking rule is explained, and matches what the server does ----

const TWO_HITS: PublicSearchResponse = {
  ...EMPTY,
  query: "pdf",
  results: [
    {
      ...HIT_FACETS,
      skill_id: "aaaaaaaa-0000-0000-0000-000000000001",
      name: "A",
      summary: "甲",
      rank: 0.7,
    },
    {
      ...HIT_FACETS,
      skill_id: "aaaaaaaa-0000-0000-0000-000000000002",
      name: "B",
      summary: "乙",
      rank: 0.6,
    },
    {
      ...HIT_FACETS,
      skill_id: "aaaaaaaa-0000-0000-0000-000000000003",
      name: "C",
      summary: "丙",
      rank: 0.5,
    },
    {
      ...HIT_FACETS,
      skill_id: "aaaaaaaa-0000-0000-0000-000000000004",
      name: "D",
      summary: "丁",
      rank: 0.4,
    },
  ],
};

test("DISC-004: the ranking rule is explained on demand and matches the pipeline", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");

  const explainer = container.querySelector<HTMLDetailsElement>("details.ranking-explainer")!;
  expect(explainer).not.toBeNull();
  // Progressive disclosure: available, not shouted at every searcher.
  expect(explainer.open).toBe(false);

  const text = explainer.textContent ?? "";
  // The four facts the implementation actually has: vector distance ranks,
  // the lexical leg only widens candidates, the cut-off hides the rest, and
  // popularity is not an input (02:DISC-002 排序不得只使用 Star).
  expect(text).toContain("語意相似度");
  expect(text).toContain("關鍵字命中只用來多找候選，不會改變名次");
  expect(text).toContain("低於 0.25");
  expect(text).toContain("不看 Star 數");
  // Both exceptions are described even when neither is live right now...
  expect(text).toContain("只能用關鍵字比對時");
  expect(text).toContain("還沒建立語意索引");
  // ...but nothing claims one of them is happening.
  expect(text).not.toContain("目前這次搜尋就是這個狀態");
  expect(text).not.toContain("這頁就有這種結果");
});

test("DISC-004: a degraded answer marks the exception that is actually in force", async () => {
  stubSearch({ ...TWO_HITS, degraded: true, degraded_reason: "embedding unavailable" });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.querySelector("details.ranking-explainer")?.textContent ?? "";
  expect(text).toContain("目前這次搜尋就是這個狀態");
  expect(text).not.toContain("這頁就有這種結果");
});

// ---- DISC-009: selecting 2–3 candidates, then comparing them side by side ----

function compareLink() {
  return container.querySelector<HTMLAnchorElement>(".compare-bar a");
}

function pick(index: number) {
  const boxes = container.querySelectorAll<HTMLInputElement>(".compare-pick input");
  return act(async () => {
    boxes[index].click();
  });
}

test("DISC-009: comparison needs two candidates and accepts at most three", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");

  expect(compareLink()).toBeNull();
  expect(container.textContent).toContain("勾選 2 至 3 個 Skill");

  await pick(0);
  expect(compareLink()).toBeNull(); // one is not a comparison

  await pick(1);
  expect(compareLink()?.getAttribute("href")).toContain(
    "ids=aaaaaaaa-0000-0000-0000-000000000001%2Caaaaaaaa-0000-0000-0000-000000000002",
  );

  await pick(2);
  expect(compareLink()?.textContent).toContain("3 個");
  // The fourth box is refused rather than silently dropping an earlier pick.
  const boxes = container.querySelectorAll<HTMLInputElement>(".compare-pick input");
  expect(boxes[3].disabled).toBe(true);
  expect(boxes[0].disabled).toBe(false);

  await pick(0); // deselecting frees the slot again
  expect(container.querySelectorAll<HTMLInputElement>(".compare-pick input")[3].disabled).toBe(
    false,
  );
});

function detailFixture(overrides: Partial<SkillDetail>): SkillDetail {
  return {
    skill_id: "unset",
    name: "unset",
    summary: "把 PDF 轉成摘要",
    scope: "catalog",
    tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
    enrichment: { status: "pending", note: "尚未產生白話摘要。" },
    limitations: [],
    license: { status: { value: "unknown", label: "License 未知", note: "未宣告 License。" } },
    derivation: { is_fork: false, label: "來源關係", note: "非 Fork。" },
    risk: {
      scan_status: "scanned",
      counts: { errors: 0, warnings: 0, infos: 0 },
      highlights: [],
      info_counts: {},
      note: "以上為靜態掃描結果。",
    },
    compatibility: {
      spec_validation: "passed",
      capability: "unverified",
      runtime: "unverified",
      note: "尚未試跑。",
    },
    ...overrides,
  };
}

/** Answers the search call and GET /api/skills/{id} from fixtures. */
function stubSearchAndDetails(search: PublicSearchResponse, details: Record<string, SkillDetail>) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(search), { status: 200 }));
    }
    const id = url.match(/\/api\/skills\/([^/?]+)$/)?.[1];
    if (id && details[id]) {
      return Promise.resolve(new Response(JSON.stringify(details[id]), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ error: "not found" }), { status: 404 }));
  });
}

function compareRow(label: string) {
  return [...container.querySelectorAll("tbody tr")].find((tr) =>
    tr.querySelector("th")?.textContent?.startsWith(label),
  )!;
}

test("DISC-009: the table highlights differing rows and never invents a missing one", async () => {
  const left = detailFixture({
    skill_id: "id-left",
    name: "左邊的 Skill",
    enrichment: {
      status: "enriched",
      summary: "把 PDF 轉成摘要。",
      tags: { inputs: ["PDF"], outputs: [], tools: [], dependencies: [] },
      note: "由模型於索引時產生。",
    },
    license: {
      expression: "MIT",
      source: "manifest",
      status: { value: "declared", label: "License 已宣告", note: "尚未人工核對。" },
    },
    version: {
      version_id: "v1",
      version_number: 1,
      content_hash: "abc",
      created_at: "2026-08-01T00:00:00Z",
    },
  });
  const right = detailFixture({ skill_id: "id-right", name: "右邊的 Skill" });
  stubSearchAndDetails(
    {
      ...EMPTY,
      query: "pdf",
      results: [
        { ...HIT_FACETS, skill_id: "id-left", name: "左邊的 Skill", summary: "甲", rank: 0.7 },
        { ...HIT_FACETS, skill_id: "id-right", name: "右邊的 Skill", summary: "乙", rank: 0.6 },
      ],
    },
    { "id-left": left, "id-right": right },
  );

  // Driven the way a user gets there: search, pick two, follow the link. That
  // also covers the router wiring of /compare?ids=…
  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);
  await act(async () => compareLink()!.click());
  await waitFor(() => container.querySelector("table.compare-table") !== null);

  // Both skills are columns of one table (DISC-004 至少兩個候選的靜態比較).
  expect(container.textContent).toContain("左邊的 Skill");
  expect(container.textContent).toContain("右邊的 Skill");

  // Same value on both sides: no highlight, no noise.
  const tier = compareRow("類別");
  expect(tier.className).not.toContain("compare-differs");
  expect(tier.textContent).not.toContain("有差異");

  // Different value: highlighted, and the reason is spelled out rather than
  // being carried by colour alone.
  const license = compareRow("License");
  expect(license.className).toContain("compare-differs");
  expect(license.textContent).toContain("有差異");
  expect(license.textContent).toContain("MIT");
  expect(license.textContent).toContain("License 未知");

  // A field neither skill declared reads 未知 on every column — never blank,
  // never a pass (02:DISC-004 不得自行推定為通過).
  // Declared on one side only: the value renders, the other side reads 未知,
  // and an empty bucket is 未知 too rather than an implied "takes nothing".
  const inputs = compareRow("輸入");
  expect(inputs.textContent).toContain("PDF");
  expect(inputs.querySelectorAll(".compare-unknown")).toHaveLength(1);
  expect(compareRow("輸出").querySelectorAll(".compare-unknown")).toHaveLength(2);
  expect(compareRow("限制").querySelectorAll(".compare-unknown")).toHaveLength(2);

  // Missing on one side only: still 未知 there, and the row counts as a difference.
  const version = compareRow("版本與時間");
  expect(version.className).toContain("compare-differs");
  expect(version.querySelectorAll(".compare-unknown")).toHaveLength(1);
  expect(version.textContent).toContain("v1");

  // Evidence rows are present and honest about what was never verified.
  expect(compareRow("相容性（驗證證據）").textContent).toContain("未驗證");
});

test("DISC-004: an unreadable package is reported as unknown, never as a clean scan", async () => {
  await render(
    <RiskIndicator
      risk={{
        scan_status: "unavailable",
        counts: { errors: 0, warnings: 0, infos: 0 },
        highlights: [],
        info_counts: {},
        note: "以上為靜態掃描結果。",
      }}
    />,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("未知");
  expect(text).not.toContain("未發現錯誤或警告");
});
