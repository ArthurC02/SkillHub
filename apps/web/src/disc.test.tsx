import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { LicenseBadge, LicenseNotes } from "./components/LicenseBadge";
import { RiskIndicator } from "./components/RiskIndicator";
import type {
  PublicSearchResponse,
  PublicSearchResult,
  SkillDetail,
  SkillFiles,
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
  // The router is a module singleton and only observes history while it is
  // mounted, so a test that navigated leaves it pointing at that page even
  // after beforeEach resets window.location — the address bar moves, the router
  // does not. Send it home now that it is mounted again. Skipped when the tree
  // under test is a bare component and no router is on screen.
  if (container.querySelector(".app-shell") && router.state.location.pathname !== "/") {
    await act(async () => {
      await router.navigate({ to: "/", search: {} });
    });
  }
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
    disclosures: [],
    note: "以上為靜態掃描結果。",
  },
  dependencies: [],
  compatibility: {
    spec_validation: { value: "passed", label: "通過", note: "" },
    capability: { value: "unverified", label: "未驗證", note: "" },
    runtime: { value: "unverified", label: "未驗證", note: "" },
    note: "尚未試跑。",
  },
} satisfies Pick<PublicSearchResult, "tier" | "risk" | "dependencies" | "compatibility">;

const EMPTY: PublicSearchResponse = {
  query: "",
  results: [],
  degraded: false,
  partial_index: false,
  no_results: false,
  filtered_out: false,
  limit: 20,
  truncated: false,
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

test("DISC-002: a truncated result page says so, and says how many it is showing", async () => {
  // ADR-042 決策 3. The cap (20 by default) has always been here; result 21 did
  // not exist as far as the page could tell, and a list that is quietly short
  // reads as the whole answer. Deliberately worded apart from the two notices
  // below: those say how well the search could look, this says how much of what
  // it found is on the page.
  stubSearch({
    ...EMPTY,
    query: "pdf",
    limit: 20,
    truncated: true,
    results: [
      {
        ...HIT_FACETS,
        skill_id: "77777777-7777-7777-7777-777777777777",
        name: "PDF 一號",
        summary: "摘要",
        rank: 0.9,
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  expect(container.textContent).toContain("只列出最接近的 20 個");
  // Not the degraded copy: recall being lower and the page being cut are
  // different facts with different fixes.
  expect(container.textContent).not.toContain("召回率明顯較低");
});

test("DISC-005: degraded and partial_index are separate, non-blocking notices", async () => {
  stubSearch({
    ...EMPTY,
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
    filtered_out: false,
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
  disclosures: [
    {
      code: "embedded-script",
      label: "SKILL.md 內含可執行程式碼",
      note: "程式碼寫在 SKILL.md 裡面,不是獨立檔案,所以看檔案清單看不出來。",
    },
  ],
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
    redistribution: {
      value: "unknown",
      label: "可散布性未確認",
      note: "沒有人確認過這個 Skill 可不可以再散布。",
    },
    derivation: { is_fork: false, label: "來源關係", note: "非 Fork。" },
    risk: {
      scan_status: "scanned",
      counts: { errors: 0, warnings: 0, infos: 0 },
      highlights: [],
      info_counts: {},
      disclosures: [],
      note: "以上為靜態掃描結果。",
    },
    compatibility: {
      spec_validation: { value: "passed", label: "通過", note: "" },
      capability: { value: "unverified", label: "未驗證", note: "" },
      runtime: { value: "unverified", label: "未驗證", note: "" },
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
  const tier = compareRow("來源層級");
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
        disclosures: [],
        note: "以上為靜態掃描結果。",
      }}
    />,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("未知");
  expect(text).not.toContain("未發現錯誤或警告");
});

// --- DISC-003: structured filters -------------------------------------------

/** Picks the <select> inside the filter-bar label whose text starts with `label`. */
function filterSelect(label: string): HTMLSelectElement {
  const group = [...container.querySelectorAll(".filter-bar label")].find((l) =>
    l.textContent?.startsWith(label),
  );
  if (!group) throw new Error(`no filter labelled ${label}; DOM: ${container.textContent}`);
  return group.querySelector("select")!;
}

async function chooseFilter(label: string, value: string) {
  const select = filterSelect(label);
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => !container.textContent?.includes("搜尋中…"));
}

test("DISC-003: a chosen filter reaches the request and the shareable URL", async () => {
  const calls = stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");
  await chooseFilter("是否包含 Script", "yes");
  await chooseFilter("驗證狀態", "passed");

  // The request carries both dimensions...
  const last = calls[calls.length - 1];
  expect(last).toContain("script=yes");
  expect(last).toContain("validation=passed");
  // ...and so does the address bar, which is what makes the page shareable.
  const url = new URLSearchParams(window.location.search);
  expect(url.get("q")).toBe("pdf");
  expect(url.get("script")).toBe("yes");
  expect(url.get("validation")).toBe("passed");

  // Clearing a dimension removes it rather than sending an empty value: an
  // unset filter must be absent from a shared link, not present and blank.
  await chooseFilter("是否包含 Script", "");
  expect(new URLSearchParams(window.location.search).has("script")).toBe(false);
  expect(calls[calls.length - 1]).not.toContain("script=");
});

// DISC-002 lists Agent 相容 as an M2 dimension「依 Sandbox 實測」. It is the same
// contract as the two above — reaches the request, reaches the URL, clears to
// absent — and it is asserted separately because the value it carries is not a
// yes/no: `transpiled` is a third answer, and a filter that folded it into "not
// native" would quietly also return everything nobody has measured.
test("DISC-003: the Agent 相容 filter reaches the request and the shareable URL", async () => {
  const calls = stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  await chooseFilter("Agent 相容", "transpiled");
  expect(calls[calls.length - 1]).toContain("agent=transpiled");
  expect(new URLSearchParams(window.location.search).get("agent")).toBe("transpiled");

  await chooseFilter("Agent 相容", "");
  expect(new URLSearchParams(window.location.search).has("agent")).toBe(false);
  expect(calls[calls.length - 1]).not.toContain("agent=");
});

test("DISC-003: the filters the platform has no data for are disabled and say why", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  // The three dimensions with per-row data are usable. Agent 相容 joined them
  // with the M2 baseline measurements (0022); the assertion below is what would
  // catch it silently reverting to a dead control.
  expect(filterSelect("是否包含 Script").disabled).toBe(false);
  expect(filterSelect("驗證狀態").disabled).toBe(false);
  expect(filterSelect("Agent 相容").disabled).toBe(false);

  // The three without are present, disabled, and each states its own reason —
  // not hidden, and never offered as a control that accepts a value and
  // narrows nothing.
  for (const label of ["類別", "來源層級", "需要 MCP"]) {
    expect(filterSelect(label).disabled).toBe(true);
  }
  const text = container.querySelector(".filter-bar")!.textContent ?? "";
  expect(text).toContain("只存在於策展清單");
  expect(text).toContain("人工精選審查尚未開始");
  expect(text).toContain("沒有記錄是否需要 MCP");
  // The reason a dimension is dead must not read as "nothing matched".
  expect(text).toContain("不是因為所有 Skill 都不符合");
});

test("DISC-003: filtered-to-empty and the no-results refusal never share copy", async () => {
  // The catalog had matches; the filters removed them. The fix is the filter.
  stubSearch({ ...EMPTY, query: "pdf", filtered_out: true });
  await render(<App />);
  await submitSearch("pdf");

  let text = container.textContent ?? "";
  expect(text).toContain("全部被目前的篩選條件排除");
  // The DISC-005 refusal copy must not appear: telling someone to reword a
  // query that did match is advice about the wrong thing.
  expect(text).not.toContain("沒有夠接近的 Skill");

  // The other empty state, same page, entirely different copy.
  await act(async () => root.unmount());
  queryClient.clear();
  window.history.pushState({}, "", "/");
  stubSearch({
    ...EMPTY,
    query: "pdf",
    no_results: true,
    query_suggestion: "Try naming the file format you have.",
  });
  await render(<App />);
  await submitSearch("pdf");

  text = container.textContent ?? "";
  expect(text).toContain("沒有夠接近的 Skill");
  expect(text).toContain("Try naming the file format you have.");
  expect(text).not.toContain("全部被目前的篩選條件排除");
});

test("DISC-002: a result row carries all seven columns, and infers none of them", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        skill_id: "55555555-5555-5555-5555-555555555555",
        name: "PDF Summariser",
        summary: "把 PDF 整理成摘要",
        rank: 0.42,
        risk: {
          scan_status: "scanned",
          level: "disclosed",
          warnings: 0,
          disclosures: [
            { code: "script-file", label: "含可執行 Script 檔案", note: "平台不曾執行它們。" },
          ],
          note: "靜態掃描。",
        },
        dependencies: ["pypdf"],
        verified_at: "2026-08-01T10:00:00Z",
      },
      {
        ...HIT_FACETS,
        skill_id: "66666666-6666-6666-6666-666666666666",
        name: "Unscanned Skill",
        summary: "沒有掃描紀錄",
        rank: 0.3,
        // The server's own sentence, verbatim (catalog.searchRiskUnknown). The row
        // used to print a near-copy of it from the client as well, so an unscanned
        // hit said the same thing twice in two spellings; the note is now the only
        // place that sentence comes from.
        risk: {
          scan_status: "unavailable",
          level: "none",
          warnings: 0,
          disclosures: [],
          note: "此結果尚無掃描紀錄,狀態未知——不代表已通過檢查。",
        },
        dependencies: [],
        compatibility: {
          spec_validation: { value: "unverified", label: "未驗證", note: "" },
          capability: { value: "unverified", label: "未驗證", note: "" },
          runtime: { value: "unverified", label: "未驗證", note: "" },
          note: "尚未試跑。",
        },
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const rows = container.querySelectorAll(".search-result");
  expect(rows).toHaveLength(2);

  const scanned = rows[0].textContent ?? "";
  expect(scanned).toContain("PDF Summariser"); // 名稱
  expect(scanned).toContain("把 PDF 整理成摘要"); // 白話摘要
  expect(scanned).toContain("已收錄"); // 來源層級 (server-owned copy)
  expect(scanned).toContain("規格驗證：通過"); // 相容狀態
  // 設計 §4.4: the search row and the detail view used to word this boolean
  // differently — 「含 Script 檔案」 here, 「含可執行 Script 檔案」 one component over —
  // and 可執行 is the word doing the work. One list now serves both.
  expect(scanned).toContain("含可執行 Script 檔案"); // 風險提示
  expect(scanned).toContain("pypdf"); // 依賴
  expect(scanned).toContain("2026-08-01"); // 最近驗證時間
  // 沒有驗證證據的 Skill 必須明確標記「尚未試跑」.
  expect(scanned).toContain("尚未試跑");

  // Nothing is inferred on the row that has no evidence: an unscanned package
  // is unknown rather than clean, and an empty dependency list says it was not
  // extracted rather than that there are none (02:DISC-004).
  const unscanned = rows[1].textContent ?? "";
  expect(unscanned).toContain("尚無掃描紀錄");
  expect(unscanned).not.toContain("未發現警告");
  expect(unscanned).toContain("未擷取到依賴資訊");
  expect(unscanned).toContain("規格驗證：未驗證");
});

test("DISC-006: the general detail view answers all nine required facts", async () => {
  const skill = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-000000000001",
    name: "PDF Summariser",
    summary: "把 PDF 整理成摘要",
    enrichment: {
      status: "enriched",
      summary: "讀 PDF，輸出重點摘要。",
      tags: { inputs: ["pdf"], outputs: ["markdown"], tools: [], dependencies: ["pypdf"] },
      note: "本區塊由模型產生。",
    },
    limitations: [
      { text: "不處理掃描件的手寫字。", source: "model" },
      { text: "套件內含可執行 Script。", source: "scan" },
    ],
    allowed_tools: ["Bash"],
    source: {
      type: "git",
      url: "https://github.com/example/pdf",
      source_version: "abc123",
      fetched_at: "2026-08-01T10:00:00Z",
      content_hash: "sha256:beef",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
  });
  stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes("PDF Summariser"));

  const text = container.textContent ?? "";
  // 02:DISC-003 一般模式: 功能、限制、輸入、輸出、依賴、權限、來源、License、相容性.
  expect(text).toContain("把 PDF 整理成摘要"); // 功能
  expect(text).toContain("不處理掃描件的手寫字。"); // 限制 (model)
  expect(text).toContain("套件內含可執行 Script。"); // 限制 (scan)
  expect(text).toContain("輸入：");
  expect(text).toContain("pdf");
  expect(text).toContain("輸出：");
  expect(text).toContain("markdown");
  expect(text).toContain("依賴：");
  expect(text).toContain("pypdf");
  expect(text).toContain("套件宣告可用的工具"); // 權限
  expect(text).toContain("https://github.com/example/pdf"); // 來源
  expect(text).toContain("License 未知"); // License
  expect(text).toContain("規格驗證"); // 相容性
  // ADR-013: the model half of 限制 is marked as model-written, the scan half is
  // not allowed to borrow that label.
  expect(container.querySelectorAll(".badge-source-model").length).toBeGreaterThan(0);
});

test("DISC-006: an unenriched skill reads as unknown, never as 'needs nothing'", async () => {
  const skill = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-000000000002",
    name: "Bare Skill",
    // Pending enrichment and an empty limitations list: the state a skill sits
    // in between import and backfill, which is where an omitted row silently
    // turns "not extracted" into "none".
  });
  stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes("Bare Skill"));

  const text = container.textContent ?? "";
  expect(text).toContain("依賴：");
  expect(text).toContain("未知");
  expect(text).toContain("不代表這個 Skill 沒有限制");
});

// 02:SEC-007 / ADR-027 決策 4: three states, and two of them refuse. `unknown`
// is not a pending state that will resolve itself — it blocks exactly like
// `blocked` — so the screen has to say so in its own words rather than leaving a
// reader to assume the download is on its way.
test("SEC-007: the redistribution verdict shows all three states and only `allowed` opens packaging", async () => {
  const cases = [
    { value: "allowed", label: "可再散布", opens: true },
    { value: "blocked", label: "不可再散布", opens: false },
    { value: "unknown", label: "可散布性未確認", opens: false },
  ];
  for (const c of cases) {
    const skill = detailFixture({
      skill_id: `dddddddd-0000-0000-0000-00000000000${cases.indexOf(c) + 3}`,
      name: `Redistribution ${c.value}`,
      redistribution: { value: c.value, label: c.label, note: `${c.value} 的說明。` },
      version: {
        version_id: "v1",
        version_number: 1,
        content_hash: "sha256:aa",
        created_at: "2026-08-01T00:00:00Z",
      },
    });
    stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
    await render(<App />);
    await act(async () => {
      await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
    });
    await waitFor(() => (container.textContent ?? "").includes(skill.name));

    const text = container.textContent ?? "";
    expect(text).toContain(c.label);
    expect(text).toContain(`${c.value} 的說明。`);

    const link = [...container.querySelectorAll("a")].find((a) =>
      (a.getAttribute("href") ?? "").includes("/package"),
    );
    const refusal = [...container.querySelectorAll("button")].find((b) =>
      (b.textContent ?? "").includes("打包並下載"),
    );
    if (c.opens) {
      expect(link).toBeDefined();
      expect(refusal).toBeUndefined();
    } else {
      expect(link).toBeUndefined();
      expect(refusal?.disabled).toBe(true);
    }
    // Cleanup between iterations: this test renders the app three times.
    await act(async () => root?.unmount());
    container.innerHTML = "";
    queryClient.clear();
  }
});

test("DISC-007: advanced mode shows SKILL.md in full and marks every script", async () => {
  const skillId = "eeeeeeee-0000-0000-0000-000000000001";
  const files: SkillFiles = {
    skill_id: skillId,
    version_id: "v1",
    version_number: 1,
    skill_md: "---\nname: pdf\n---\n\n用法說明。",
    skill_md_truncated: false,
    tree: [
      { path: "SKILL.md", size: 42, is_script: false },
      { path: "scripts/run.py", size: 17, is_script: true },
      { path: "reference/notes.md", size: 9, is_script: false },
    ],
    // SKILL-003: the tree is exactly what cannot show code living inside the
    // document, so the disclosure travels beside it.
    embedded_script_note: "SKILL.md 內含可執行程式碼。",
    note: "tree 為套件內檔案清單與大小。",
  };
  vi.stubGlobal("fetch", (input: string) => {
    if (String(input).includes(`/api/skills/${skillId}/files`)) {
      return Promise.resolve(new Response(JSON.stringify(files), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ error: "not found" }), { status: 404 }));
  });

  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId/files", params: { skillId } });
  });
  await waitFor(() => (container.textContent ?? "").includes("scripts/run.py"));

  const text = container.textContent ?? "";
  expect(text).toContain("用法說明。"); // SKILL.md 全文
  expect(text).toContain("SKILL.md 內含可執行程式碼。"); // SKILL-003 disclosure

  // 「Script 必須有明確標示」: the marker sits on the script entry and only on it.
  const marked = [...container.querySelectorAll(".file-tree li")].filter((li) =>
    li.querySelector(".script-tag"),
  );
  expect(marked).toHaveLength(1);
  expect(marked[0].textContent).toContain("scripts/run.py");
  // A way back to the general mode, so the two modes are navigable both ways.
  expect(text).toContain("一般模式");
});

// The owner's 方案 C decision (m2/anthropic-sa-license-memo.md) as the reader
// meets it: the page still describes the skill, says why the materials are not
// shown, and does not offer a link into the view that would refuse.
test("a licensing hold explains itself and takes the advanced link with it", async () => {
  const held = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-00000000beef",
    name: "Docx Editor",
    summary: "編輯 Word 文件",
    version: {
      version_id: "v1",
      version_number: 1,
      content_hash: "sha256:cafe",
      created_at: "2026-08-01T00:00:00Z",
    },
    access_restriction: {
      reason: "license-review",
      note: "此 Skill 的來源授權正在審查中:不提供 SKILL.md 全文與檔案樹。",
    },
  });
  stubSearchAndDetails(EMPTY, { [held.skill_id]: held });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: held.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes("Docx Editor"));

  const text = container.textContent ?? "";
  // The listing survives — that is the half of 方案 C that is not a removal.
  expect(text).toContain("編輯 Word 文件");
  expect(text).toContain("授權審查中");
  expect(text).toContain("來源授權正在審查中");
  // …and the door that would 403 is not shown as a door.
  expect(text).not.toContain("查看 SKILL.md 與檔案樹");
});
