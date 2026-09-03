import { readFileSync } from "node:fs";
import { join } from "node:path";
import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { LicenseBadge, LicenseNotes } from "./components/LicenseBadge";
import { RiskIndicator } from "./components/RiskIndicator";
import type {
  CatalogResponse,
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

function stubCatalog(body: CatalogResponse, status = 200) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    calls.push(String(input));
    if (String(input).includes("/api/skills/catalog")) {
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
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

test("DISC-001 keeps the search draft in sync with URL navigation", async () => {
  stubSearch(EMPTY);
  await render(<App />);

  await act(async () => {
    await router.navigate({ to: "/", search: { q: "first" } });
  });
  expect(container.querySelector<HTMLInputElement>('input[aria-label="任務描述"]')?.value).toBe(
    "first",
  );

  await act(async () => {
    await router.navigate({ to: "/", search: { q: "second" } });
  });
  expect(container.querySelector<HTMLInputElement>('input[aria-label="任務描述"]')?.value).toBe(
    "second",
  );
});

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
 *
 * `summary_source` joined the list when 117ba44 made it required and updated the
 * two fixtures that assert the badge, leaving twelve that do not — which is what
 * a shared base is for. It sits here as `package`, the value that renders no
 * badge, so a test that says nothing about the summary's provenance keeps
 * asserting nothing about it.
 */
const HIT_FACETS = {
  summary_source: "package",
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
} satisfies Pick<
  PublicSearchResult,
  "summary_source" | "tier" | "risk" | "dependencies" | "compatibility"
>;

const EMPTY: PublicSearchResponse = {
  query: "",
  results: [],
  degraded: false,
  partial_index: false,
  no_results: false,
  filtered_out: false,
  limit: 20,
  truncated: false,
  total: 0,
};

test("DISC-006: catalog serializes filters and explains one truncated result list once", async () => {
  const rankNote = "server-owned-catalog-order-sentinel";
  const calls = stubCatalog({
    results: [
      {
        ...HIT_FACETS,
        skill_id: "catalog-one",
        name: "Catalog One",
        summary: "A browsable skill.",
        rank: null,
        rank_note: rankNote,
        verified_at: "2026-09-03T00:00:00Z",
        match_reason: "",
        match_reason_source: "template",
      },
    ],
    limit: 20,
    total: 3,
    truncated: true,
  });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { script: "no" } });
  });
  await waitFor(() => container.textContent?.includes("Catalog One") ?? false);

  expect(calls.some((url) => url.includes("/api/skills/catalog?limit=20&script=no"))).toBe(true);
  expect(container.textContent).toContain("目錄共 3 個 Skill，這裡列出 1 個");
  expect(container.textContent?.split(rankNote).length - 1).toBe(1);
  expect(container.textContent).not.toContain("未計算語意相似度");
});

const CATALOG_ROW = {
  ...HIT_FACETS,
  skill_id: "catalog-one",
  name: "Catalog One",
  summary: "A browsable skill.",
  rank: null,
  rank_note: "目錄依收錄時間排序。",
  verified_at: "2026-09-03T00:00:00Z",
  match_reason: "",
  match_reason_source: "template" as const,
};

/**
 * 「那你就把有的都給我看」這個手勢。
 *
 * `browsing` 的判準是 `q === undefined`，而**空字串是字串**：清空搜尋框再按搜尋，
 * 送出的是 `q=""`，`validateSearch` 原樣收下，伺服器對空查詢走 `no_results`，畫面
 * 回「沒有夠接近的 Skill。」——而目錄就在同一個位址上。這一頁的搜尋態全部只有三個
 * 連結，沒有一個回得去目錄，唯一的出口是頁首的產品標題。
 *
 * 押的是「目錄那一列出現了」，也就是 `browsing` 真的翻回來了。把 `|| undefined`
 * 拿掉就變紅。
 */
test("DISC-006: 把搜尋框清空再按搜尋，回到目錄而不是「沒有夠接近的 Skill」", async () => {
  const calls = stubCatalog({ results: [CATALOG_ROW], limit: 20, total: 1, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "沒有這種東西" } });
  });
  // 搜尋在這個替身上回 401；重點不是它回什麼，是接下來那個手勢去了哪裡。
  await submitSearch("   ");
  await waitFor(() => container.textContent?.includes("Catalog One") ?? false);

  expect(calls.some((url) => url.includes("/api/skills/catalog"))).toBe(true);
  expect(container.textContent).not.toContain("沒有夠接近的 Skill");
});

/**
 * 設計 §2.4 第 3 項，而這一格是 2026-09-03 目錄批帶進來的迴歸：目錄與搜尋共用
 * `SearchResultRow`，那五顆來源徽章的但書卻只寫在搜尋那一半。於是**落地首頁的
 * 預設狀態**上，「作者原文」「AI 改寫」「來源未標示」的限定語退回成只有 `title=`
 * ——手機上不存在。§0 把這一族排在順位 1，它不能只在其中一種狀態下成立。
 */
test("DISC-006: 目錄那一半也要有來源標記的但書，不只搜尋那一半", async () => {
  stubCatalog({ results: [CATALOG_ROW], limit: 20, total: 1, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  await waitFor(() => container.textContent?.includes("Catalog One") ?? false);

  expect(container.textContent).toContain("標記說明");
  expect(container.textContent).toContain("「作者原文」是套件的 frontmatter description");
});

test("DISC-006: an empty catalog is distinct from a failed catalog read", async () => {
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  await waitFor(() => container.textContent?.includes("目錄現在是空的") ?? false);
  expect(container.textContent).toContain("這不是讀取失敗");
  expect(container.textContent).not.toContain("清掉篩選條件");
  await act(async () => root.unmount());
  queryClient.clear();
  container.replaceChildren();
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false }, 503);
  await render(<App />);
  await waitFor(() => container.textContent?.includes("無法讀取目錄") ?? false);
  expect(container.textContent).not.toContain("目錄現在是空的");
});

test("DISC-006: an empty filtered catalog explains how to recover", async () => {
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { tier: "curated" } });
  });
  await waitFor(() => container.textContent?.includes("沒有 Skill 符合目前的篩選條件") ?? false);
  expect(container.textContent).toContain("清掉篩選條件");
  expect(container.textContent).not.toContain("部署還沒有匯入任何 Skill");
});

test("returning to browse does not render a cached search beside the catalog", async () => {
  const searchHit: PublicSearchResult = {
    ...HIT_FACETS,
    skill_id: "cached-search",
    name: "Cached Search Only",
    summary: "search result",
    rank: 0.9,
    verified_at: "2026-09-03T00:00:00Z",
    match_reason: "matched",
    match_reason_source: "template",
  };
  const catalogHit: PublicSearchResult = {
    ...searchHit,
    skill_id: "catalog-only",
    name: "Catalog Only",
    summary: "catalog result",
    rank: null,
    rank_note: "catalog order",
  };
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            ...EMPTY,
            results: [searchHit],
            total: 1,
          }),
          { status: 200 },
        ),
      );
    }
    if (url.includes("/api/skills/catalog")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            results: [catalogHit],
            limit: 20,
            total: 1,
            truncated: false,
          }),
          { status: 200 },
        ),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });

  await render(<App />);
  await act(async () => router.navigate({ to: "/", search: { q: "" } }));
  await waitFor(() => container.textContent?.includes("Cached Search Only") ?? false);
  await act(async () => router.navigate({ to: "/", search: {} }));
  await waitFor(() => container.textContent?.includes("Catalog Only") ?? false);

  expect(container.textContent).not.toContain("Cached Search Only");
});

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
        summary_source: "model",
        rank: 0.82,
        match_reason: "這個 Skill 直接處理 PDF 並輸出摘要。",
        match_reason_source: "model",
      },
      {
        ...HIT_FACETS,
        skill_id: "22222222-2222-2222-2222-222222222222",
        name: "Doc Splitter",
        summary: "切分文件",
        summary_source: "package",
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
  // borrow it. Scoped to .match-reason since the summary carries the same marker
  // now — an unscoped count would let one badge stand in for the other, and the
  // whole point is that they are two separate claims about two separate strings.
  expect(container.querySelectorAll(".match-reason .badge-source-model")).toHaveLength(1);
  expect(container.querySelectorAll(".match-reason .badge-source-template")).toHaveLength(1);

  // ADR-013's other half, and the one that was missing. The summary is the
  // sentence a reader decides on, and it is the model's rewrite for every
  // enriched skill — 45/45 of the catalogue. The badge was on the match reason
  // three lines below and not on this.
  expect(container.querySelectorAll(".badge-source-package")).toHaveLength(1);
  expect(text).toContain("AI 改寫");
  expect(text).toContain("作者原文");
  // A server that did not answer must not be answered for: defaulting to
  // 作者原文 would print the author's name over the model's sentence, which is
  // the failure this badge exists to prevent, reintroduced as a fallback.
  expect(container.querySelectorAll(".badge-source-unknown")).toHaveLength(0);
});

test("DISC-002: a summary with no stated source says so rather than crediting the author", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        // The one fixture that must NOT inherit the shared base's
        // `summary_source`: this test is about a server that did not send the
        // field. The view-model type says it always does and the wire is not
        // bound by that, so the cast is where the disagreement is written down
        // rather than assumed away.
        summary_source: undefined,
        skill_id: "33333333-3333-3333-3333-333333333333",
        name: "Mystery",
        summary: "來源不明的摘要",
        rank: 0.5,
        match_reason: "查詢與文件共同出現：pdf",
        match_reason_source: "template",
      } as unknown as PublicSearchResult,
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  expect(container.querySelectorAll(".badge-source-unknown")).toHaveLength(1);
  // On the BADGE, not on the page: 2026-08-29 added a 標記說明 line above the
  // list that names all five markers once as visible text (設計 §2.4 第 3 項
  // — they were `title=` only), so 「作者原文」 appears there legitimately. The
  // fact under test is that this row is not badged with it.
  expect(container.querySelectorAll(".badge-source-package")).toHaveLength(0);
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
    total: 47,
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

  expect(container.textContent).toContain("只列出最接近的 1 個");
  // 設計系統 §4.3 wants 「共 N 筆，這裡顯示 M 筆，因為 X」. The population, and not
  // the page size a second time: until 2026-08-25 this notice read 「超過 20 個」,
  // which is the cap talking about itself. A reader could not tell 21 from 2100.
  expect(container.textContent).toContain("共 47 個");
  expect(container.textContent).not.toContain("超過");
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

/**
 * 04 丙-136。`compareRoute` 的註解逐字寫著「the selection lives in the URL so a
 * comparison is linkable and survives a reload」——那個裁定在 `/compare` 上成立，
 * 在**產生**那份選擇的這一步上以前不成立：`useState` 撐不過一次導覽，於是
 * DISC-009 自己的工作流「比較 → 上一頁 → 換掉一筆 → 再比較」每走一次都要重新勾兩個。
 *
 * 押的是「網址說了算」：帶著 `compare=` 到站，兩個框就是勾的、比較連結就在。
 */
test("DISC-009: 勾選住在網址上，所以它撐得過一次導覽", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await act(async () => {
    await router.navigate({
      to: "/",
      search: {
        q: "pdf",
        compare: "aaaaaaaa-0000-0000-0000-000000000001,aaaaaaaa-0000-0000-0000-000000000002",
      },
    });
  });
  await waitFor(() => compareLink() !== null);

  const boxes = container.querySelectorAll<HTMLInputElement>(".compare-pick input");
  expect(boxes[0].checked).toBe(true);
  expect(boxes[1].checked).toBe(true);
  expect(compareLink()?.getAttribute("href")).toContain(
    "ids=aaaaaaaa-0000-0000-0000-000000000001%2Caaaaaaaa-0000-0000-0000-000000000002",
  );
});

/**
 * 選擇住到網址上之後，「換一個問題就把它丟掉」這件事換了負責人：以前是一個
 * `useEffect`，現在是 `submitSearch` 裡明寫的 `compare: undefined`——**因為那個函式
 * 淺層合併**，不明寫就會把舊的勾選帶過新的查詢，去比較兩個已經不在這一頁上的 Skill。
 * 下面那一支測的是整份 search 被替換的路徑；這一支測的是合併的那一條。
 */
test("DISC-009: 用表單換一個問題，勾選要跟著走掉（合併路徑）", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);
  expect(compareLink()).not.toBeNull();

  await submitSearch("another-task");
  await waitFor(() => compareLink() === null);
  expect(container.querySelectorAll<HTMLInputElement>(".compare-pick input")[0].checked).toBe(
    false,
  );
});

test("DISC-009: direct URL navigation clears selections from the previous result state", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);
  expect(compareLink()).not.toBeNull();

  await act(async () => {
    await router.navigate({ to: "/", search: { q: "another-task" } });
  });
  await waitFor(() => compareLink() === null);
  expect(container.textContent).toContain("勾選 2 至 3 個 Skill");
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
/**
 * `owner` 開著，代表「這個 Skill 在你的工作區裡」：`/me` 給一個 session，
 * `GET /skills/{id}/versions` 給一份非空的清單。**兩者都是打包入口現在讀的訊號**
 * ——`SkillDetail` 的 `PackagingEntry` 不再用 `skill.version` 判擁有權，因為那個
 * 欄位來自 Skill 自己的工作區（`discovery/detail.go` 的 `LatestVersion(ctx,
 * skill.WorkspaceID, …)`），對每一個訪客都有值。要單獨測授權那道閘門，就得先把
 * 擁有權那道打開，否則測到的是兩道閘門的交集。
 */
function stubSearchAndDetails(
  search: PublicSearchResponse,
  details: Record<string, SkillDetail>,
  owner = false,
) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(search), { status: 200 }));
    }
    if (owner && url.replace(/^https?:[/][/][^/]+/, "").split("?")[0] === "/me") {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            user_id: "u-1",
            email: "t@example.com",
            display_name: "tester",
            workspace_id: "ws-1",
            deletion_requested_at: null,
            purge_after: null,
            deletion_scope: null,
          }),
          { status: 200 },
        ),
      );
    }
    if (owner && url.includes("/versions")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            versions: [
              {
                version_id: "v1",
                version_number: 1,
                content_hash: "sha256:aa",
                created_at: "2026-08-01T00:00:00Z",
              },
            ],
          }),
          { status: 200 },
        ),
      );
    }
    // The id, whatever query string follows it: 並排比較 now reads with
    // `?view=embedded` so the server does not count it as a page view.
    const id = url.match(/\/api\/skills\/([^/?]+)(?:\?|$)/)?.[1];
    if (id && details[id]) {
      return Promise.resolve(new Response(JSON.stringify(details[id]), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ error: "not found" }), { status: 404 }));
  });
  return calls;
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
  const calls = stubSearchAndDetails(
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

  // O11Y-004: comparing is not opening. Both reads are marked embedded, so the
  // server records no skill_detail_viewed for them — 01 §11.2's first segment
  // counts sessions that opened a skill detail, and this table opened none.
  // Without the marker a three-way comparison minted three of those events
  // (adversarial review, 2026-08-24).
  const detailReads = calls.filter((url) => /\/api\/skills\/id-(left|right)/.test(url));
  expect(detailReads.length).toBeGreaterThan(0);
  for (const url of detailReads) {
    expect([url, url.includes("view=embedded")]).toEqual([url, true]);
  }
});

test("DISC-009: a repeated URL id is still only one comparison candidate", async () => {
  const skill = detailFixture({ skill_id: "id-left", name: "Only Skill" });
  const calls = stubSearchAndDetails(EMPTY, { "id-left": skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/compare", search: { ids: "id-left,id-left" } });
  });
  await waitFor(() => calls.some((url) => url.includes("/api/skills/id-left?")));

  expect(container.textContent).toContain("請從首頁的搜尋結果或目錄選擇 2 到 3 個 Skill");
  expect(container.querySelector("table.compare-table")).toBeNull();
});

test("DISC-009: a failed read on /compare says so at once, not after seven seconds of 載入中", async () => {
  // `useQueries` here was the one place in the app without `retry: false`. Three
  // retries keep `fetchStatus` at "fetching" for the whole backoff, so the page
  // kept promising 「載入中…（2 個裡讀到 0 個）」 about two reads that had already
  // failed — 設計 §2.1: the screen may say 未知, it may not claim progress that is
  // not happening.
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            ...EMPTY,
            query: "pdf",
            results: [
              {
                ...HIT_FACETS,
                skill_id: "id-left",
                name: "左邊的 Skill",
                summary: "甲",
                rank: 0.7,
              },
              {
                ...HIT_FACETS,
                skill_id: "id-right",
                name: "右邊的 Skill",
                summary: "乙",
                rank: 0.6,
              },
            ],
          }),
          { status: 200 },
        ),
      );
    }
    return Promise.resolve(new Response(`{"error":"boom"}`, { status: 500 }));
  });

  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);
  await act(async () => compareLink()!.click());

  // Well inside the first retry delay (1s), which is the window the reader used
  // to spend looking at nothing.
  await waitFor(() => (container.textContent ?? "").includes("讀取失敗"), 300);
  expect(container.textContent).toContain("有 2 個 Skill 讀取失敗");
});

/**
 * `level: "unknown"` — the fourth value, and the only one that is not a finding.
 *
 * The other three answer 「掃過了，結果是什麼」. `unknown` answers 「沒掃」, and
 * without a word of its own it falls through to the shape a scanned-clean row
 * uses — which is the OpenSSF-empty-repo failure 設計 §2.11(a) is built around
 * and what 02:DISC-004「不得自行推定為通過」 forbids. 未掃描 takes
 * `--accent-border` (§4.4's 未知／未驗證／未檢查) and deliberately not `--danger`:
 * 沒掃 is not 不通過.
 */
test("DISC-004: a risk level of unknown reads as 未掃描, not as a clean row", async () => {
  const { RiskSummary } = await import("./components/RiskIndicator");
  await act(async () => {
    root = createRoot(container);
    root.render(
      <RiskSummary
        risk={{
          scan_status: "unavailable",
          level: "unknown",
          warnings: 0,
          disclosures: [],
          note: "尚無掃描紀錄，狀態未知——不代表已通過檢查。",
        }}
      />,
    );
  });

  const text = container.textContent ?? "";
  expect(text).toContain("未掃描");
  // §4.4 規則 1: 未執行 names the check that did not run. A bare 「未掃描」 is a
  // state with no subject.
  expect(text).toContain("沒有靜態掃描結果可讀");
  expect(text).toContain("那不是「掃過了、沒發現」");
  // The sentence the untinted clean-scan branch prints. It must not appear: a
  // package nobody scanned did not fail to find warnings, it was never looked at.
  expect(text).not.toContain("靜態掃描未發現警告");
  // Not green: the badge carries the 未知 tint, never the plain `.badge`.
  expect(container.querySelector(".badge-unverified")).not.toBeNull();
  expect(container.querySelector(".badge-risk")).toBeNull();
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

test("DISC-003: history navigation to a different active filter reopens its controls", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf", tier: "curated" } });
  });
  const details = container.querySelector<HTMLDetailsElement>("details.filter-disclosure")!;
  await waitFor(() => details.open);
  await act(async () => {
    details.open = false;
    details.dispatchEvent(new Event("toggle", { bubbles: true }));
  });
  expect(details.open).toBe(false);

  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf", tier: "indexed" } });
  });
  await waitFor(() => details.open);
});

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

  // The four dimensions with per-row data are usable. Agent 相容 joined them
  // with the M2 baseline measurements (0022) and 來源層級 with migration 0042;
  // the assertions below are what would catch either silently reverting to a
  // dead control.
  expect(filterSelect("是否包含 Script").disabled).toBe(false);
  expect(filterSelect("驗證狀態").disabled).toBe(false);
  expect(filterSelect("Agent 相容").disabled).toBe(false);
  expect(filterSelect("來源層級").disabled).toBe(false);

  // The two without are present, disabled, and each states its own reason —
  // not hidden, and never offered as a control that accepts a value and
  // narrows nothing.
  for (const label of ["類別", "需要 MCP"]) {
    expect(filterSelect(label).disabled).toBe(true);
  }
  const text = container.querySelector(".filter-bar")!.textContent ?? "";
  expect(text).toContain("只存在於策展清單");
  expect(text).toContain("沒有記錄是否需要 MCP");
  // 來源層級 is no longer one of them: its old excuse must be gone from the
  // bar, not merely outvoted by a live control sitting next to it.
  expect(text).not.toContain("人工精選審查尚未開始");
  // The reason a dimension is dead must not read as "nothing matched".
  expect(text).toContain("不是因為所有 Skill 都不符合");
});

/**
 * 設計 §0 的裁定: 「數量留在外面，段落收進去」 — so the two numbers on the summary
 * have to be the two numbers in the bar.
 *
 * This is the assertion that stops the disclosure becoming a lie. The paragraph
 * saying WHY a dimension is dead is now one click away on a phone, and the only
 * thing left outside is a count; a count that stops describing the controls
 * beneath it is worse than no count, because the reader would have no reason to
 * open the bar and look. A seventh filter, or a dead one going live, changes
 * what is rendered — and this fails until the summary is changed with it.
 */
test("設計 §0: the filter summary counts the filters that are actually there", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  const selects = [...container.querySelectorAll<HTMLSelectElement>(".filter-bar select")];
  const dead = selects.filter((s) => s.disabled).length;
  const live = selects.length - dead;
  expect(live, "no live filters found — the picker below is measuring nothing").toBeGreaterThan(0);
  expect(dead, "no dead filters found — 「N 項目前無法篩選」 would be a claim about nothing").toBe(
    2,
  );

  const summary = container
    .querySelector(".filter-disclosure > summary")!
    .textContent!.replace(/\s+/g, "");
  expect(summary).toContain(`${live}項可用`);
  expect(summary).toContain(`${dead}項目前無法篩選`);
});

/**
 * 設計 §0 ＋ §2.2: the bar starts open when it is doing something, and shut when
 * it is not.
 *
 * Both halves are load-bearing and they fail in opposite directions, so both are
 * asserted. Shut-when-idle is 義務 1.2 — measured 2026-09-03, the six controls
 * and their six paragraphs put the first result at y958 in a 900px window with
 * the bar open, and y613 with it shut. Open-when-active is §2.2「會擋住人的東西
 * 必須在他撞上之前顯示」: a filter is removing rows from the answer, and a reader
 * who followed a shared ?tier=curated link would otherwise see a short result
 * list with nothing on screen saying why.
 */
test("設計 §0: the filter bar starts shut when idle and open when it is narrowing", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");
  expect(
    container.querySelector<HTMLDetailsElement>(".filter-disclosure")!.open,
    "no filter is set, yet the bar opens above the answer",
  ).toBe(false);

  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf", tier: "curated" } });
  });
  await waitFor(() => !container.textContent?.includes("搜尋中…"));
  expect(
    container.querySelector<HTMLDetailsElement>(".filter-disclosure")!.open,
    "a filter is narrowing the results and nothing on screen says so",
  ).toBe(true);
});

// DISC-002 來源層級, live since migration 0042 gave `skills.curation_tier` a
// second value. Same contract as the three filters above.
test("DISC-003: the 來源層級 filter reaches the request and the shareable URL", async () => {
  const calls = stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  await chooseFilter("來源層級", "curated");
  expect(calls[calls.length - 1]).toContain("tier=curated");
  expect(new URLSearchParams(window.location.search).get("tier")).toBe("curated");

  await chooseFilter("來源層級", "");
  expect(new URLSearchParams(window.location.search).has("tier")).toBe(false);
  expect(calls[calls.length - 1]).not.toContain("tier=");
});

test("DISC-003: 來源層級 offers the two tiers a row can carry, and says what 已索引 means", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  // `external` means "never imported", so no row carries it and it must not be
  // offered — a third option would promise a page that cannot exist.
  const options = Array.from(filterSelect("來源層級").options).map((o) => o.value);
  expect(options).toEqual(["", "curated", "indexed"]);

  // 精選 survives only while the reviewed version is still the newest one, so a
  // curated skill drops back to 已索引 on its next release. Copy calling 已索引
  // "never reviewed" would be false for exactly those rows.
  const text = container.querySelector("#filter-why-tier")!.textContent ?? "";
  expect(text).toContain("沒有帶著人工審查結論");
  expect(text).not.toContain("未經人工審查");
});

test("DISC-002: the tier badge is the server's value, not a front-end guess", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        ...HIT_FACETS,
        // The only row on this page that is not the fixture's 已索引 default.
        tier: { value: "curated", label: "精選", note: "已完成人工檢視。" },
        skill_id: "11111111-1111-1111-1111-111111111111",
        name: "PDF Summariser",
        summary: "把 PDF 轉成摘要",
        rank: 0.82,
        // Absent, not null: the contract marks both optional, and `null` is a
        // third thing the type does not admit.
        match_reason: undefined,
        match_reason_source: undefined,
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const badge = container.querySelector(".search-result .result-facets")!.textContent ?? "";
  expect(badge).toContain("精選");
  // The fixture default, which is what a hardcoded tierLabel() would still print.
  expect(badge).not.toContain("已收錄");
});

test("DISC-003: 清除所有篩選 clears every filter, not the two somebody remembered", async () => {
  // Arrive with all three filters on the URL. The button's own copy is the
  // claim under test: it says every filter, so every filter has to go.
  window.history.pushState({}, "", "/?q=pdf&script=none&validation=validated&agent=native");
  stubSearch({ ...EMPTY, query: "pdf", filtered_out: true });
  await render(<App />);
  await waitFor(() => (container.textContent ?? "").includes("清除所有篩選"));

  const clear = Array.from(container.querySelectorAll("button")).find((b) =>
    b.textContent?.includes("清除所有篩選"),
  )!;
  await act(async () => clear.click());
  await waitFor(() => !container.textContent?.includes("搜尋中…"));

  // Everything except the question, rather than a list of the filters that
  // existed when this test was written — a list is what let `agent` survive the
  // clear in the first place.
  const left = [...new URLSearchParams(window.location.search).keys()];
  expect(left).toEqual(["q"]);
  expect(new URLSearchParams(window.location.search).get("q")).toBe("pdf");
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
  // 最近驗證時間。斷言移到 `<time dateTime>` 上而不是渲染出來的文字：這一格原本
  // 印的是伺服器 UTC 字串的前十碼（`.slice(0, 10)`），對 UTC+8 的讀者少報一天，
  // 現在走 `<Timestamp>`，人看到的是自己的時鐘、機器看到的是原值。斷言那個原值
  // 比斷言任何一種在地化字串都穩，而且它就是 §1.1 說「證據要查得動」的那一半。
  expect([...container.querySelectorAll("time")].map((t) => t.getAttribute("dateTime"))).toContain(
    "2026-08-01T10:00:00Z",
  );
  // 沒有驗證證據的 Skill 必須明確標記「尚未試跑」.
  expect(scanned).toContain("尚未試跑");

  // Nothing is inferred on the row that has no evidence: an unscanned package
  // is unknown rather than clean, and an empty dependency list says it was not
  // extracted rather than that there are none (02:DISC-004).
  const unscanned = rows[1].textContent ?? "";
  expect(unscanned).toContain("尚無掃描紀錄");
  expect(unscanned).not.toContain("未發現警告");
  // 設計 §2.9 的表是封閉的六個詞。這一格以前寫「未擷取到依賴資訊」——意思對，
  // 詞不在表上，而表的用處正是讓「0」與「沒量到」在同一張截圖上長得不一樣。
  // 兩半都釘：表上的型別詞，以及「這不是零」那一句。
  expect(unscanned).toContain("未測量");
  expect(unscanned).toContain("不等於沒有依賴");
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
    stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill }, true);
    await render(<App />);
    await act(async () => {
      await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
    });
    await waitFor(() => (container.textContent ?? "").includes(skill.name));
    // 等打包入口自己安定下來，而不是等它上面的標題。`allowed` 這一格要等
    // workspace-scoped 的版本清單答完（`PackagingEntry`），另外兩格的停用鈕由
    // 授權閘門當場決定、不等任何請求。
    await waitFor(() =>
      c.opens
        ? Boolean(
            [...container.querySelectorAll("a")].find((a) =>
              (a.getAttribute("href") ?? "").includes("/package"),
            ),
          )
        : Boolean(
            [...container.querySelectorAll("button")].find((b) =>
              (b.textContent ?? "").includes("打包並下載"),
            ),
          ),
    );

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

/**
 * 丙-116 的另一半，而它躺了整整兩天。
 *
 * 那次修的是「試跑」：非擁有者按下去會走進一條三個畫面都各自正確、合起來卻沒有
 * 一句話說「這還不是你的」的走廊。**打包那一半沒有一起修。** 打包入口當時判斷
 * 擁有權用的是 `skill.version`，而同一個檔案在三十行外的註解逐字寫著
 * 「**`skill.version` is NOT the signal** … keying off it calls every visitor an
 * owner」——`GET /api/skills/{id}` 的 `version` 來自 `LatestVersion(ctx,
 * skill.WorkspaceID, …)`，那是 **Skill 自己的**工作區。
 *
 * 後果不是少一個連結，是多一條死路：`.action` 是全 app 唯一的強調樣式，2026-09-03
 * 的重排又把它移到整頁第二個區塊，所以**訪客在目錄頁上最顯眼的動作**是「打包並
 * 下載這個版本」，按下去落在 workspace-scoped 的 preview，回
 * `404 {"error":"skill version not found"}`，畫面印出一句英文，而且說的還不是
 * 真正的原因。
 *
 * 這支測試押的是**沒有那條連結、而且有那句話**。把 `PackagingEntry` 的擁有權判斷
 * 換回 `skill.version`，它就變紅。
 */
test("SEC-007: 目錄裡別人的 Skill 不給打包 CTA，而是說要先 Fork", async () => {
  const skill = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-000000000009",
    name: "別人的 Skill",
    // 授權那道閘門是開的，所以擋下來的只可能是擁有權那道。
    redistribution: { value: "allowed", label: "可再散布", note: "可再散布。" },
    version: {
      version_id: "v1",
      version_number: 1,
      content_hash: "sha256:aa",
      created_at: "2026-08-01T00:00:00Z",
    },
  });
  // `owner` 不開：`/me` 與 `/skills/{id}/versions` 都回 404，也就是一個沒登入的
  // 訪客在目錄裡看別人的東西——首頁最常見的那條路。
  stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes(skill.name));

  const packageLink = [...container.querySelectorAll("a")].find((a) =>
    (a.getAttribute("href") ?? "").includes("/package"),
  );
  expect(packageLink, "訪客不該拿到一條終點是 404 的打包連結").toBeUndefined();
  // 而且不是靜靜地消失：§2.4 要求被拿掉的控制項說出原因，§2.2 第三向要求那個原因
  // 帶著下一步。下一步是 Fork，而 Fork 就在同一頁上。
  expect(container.textContent ?? "").toContain("打包與下載需要登入");
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

// 02:GEN-004 names two screens — the detail view and the workspace list — and
// they have to agree. The detail page used to key the disclosure on the
// VERSION's source (`source.type === "generated"`), which is `upload` for any
// version the user saved themselves. So the first time somebody added their own
// version 2 to a generated skill, the two absences vanished from the detail page
// while the list, which reads the skill row's `redistribution`, went on showing
// them — and `redistribution` is what GEN-007's search exclusion keys on, so the
// skill was still the unreviewed, never-run, unfindable thing the sentence is
// about. Both fixtures below are that skill; both must say so.
test("GEN-004: the generated disclosure keys on the skill row, not on the version's source", async () => {
  for (const source of [
    {
      type: "generated" as const,
      content_hash: "sha256:aa",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
    // The version a user uploaded onto their own generated skill.
    {
      type: "upload" as const,
      content_hash: "sha256:bb",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
  ]) {
    const skill = detailFixture({
      skill_id: "eeeeeeee-0000-0000-0000-000000000001",
      name: "Generated Extractor",
      redistribution: { value: "generated", label: "平台為你生成的內容", note: "平台生成。" },
      source,
    });
    stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
    await render(<App />);
    await act(async () => {
      await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
    });
    await waitFor(() => (container.textContent ?? "").includes(skill.name));

    const text = container.textContent ?? "";
    expect(text).toContain("沒有經過任何人工檢視，沒有任何試跑證據");
    expect(text).toContain("那不是品質、可用性或安全的結論");
  }
});

// The other direction, so the criterion is not just "always true": an ordinary
// uploaded skill must not be told it was written by a model.
test("GEN-004: a self-supplied skill gets no generated disclosure", async () => {
  const skill = detailFixture({
    skill_id: "eeeeeeee-0000-0000-0000-000000000002",
    name: "Hand Written",
    redistribution: { value: "self_supplied", label: "你自己帶進來的", note: "自帶內容。" },
    source: {
      type: "upload",
      content_hash: "sha256:cc",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
  });
  stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes(skill.name));

  expect(container.textContent ?? "").not.toContain("沒有經過任何人工檢視");
});

/**
 * 04 丙-29 ④ / 設計 §4.4: **all twelve disclosure codes reach the screen, and
 * the words are the server's.**
 *
 * `skillpkg.DisclosureCodes` (apps/platform/internal/shared/skillpkg/skillpkg.go)
 * is the whole set and it went 6 → 12: `symlink-entry`,
 * `undeclared-dependency`, `file-not-scanned`, `package-dependencies` and
 * `entry-path-escape` were all being found by the scanner and none of them had
 * a word. The Go side asserts its catalogue covers the list; this is the same
 * assertion on the renderer.
 *
 * WHAT IS BEING ASSERTED, and it is not what it would have been a month ago.
 * There is **no code→中文 map in `apps/web`** any more: `RiskIndicator` renders
 * `disclosure.label`, from one catalogue both endpoints read (设计 §4.4 records
 * the merge, and the two divergent client-side lists it replaced — one of which
 * silently dropped 「可執行」, a smaller claim rather than a shorter label). The
 * property that buys is that **a code this build has never seen still renders**,
 * which no `keyof` union can have — and that property had no test. This is it.
 *
 * So the failure this catches is a regression to a client-side subset: reinstate
 * a local map, filter on a known-codes list, drop `label` for `note`, and this
 * goes red with the codes that vanished named in the diff.
 *
 * The list is READ FROM THE GO SOURCE, not copied: a copy here would be the
 * thirteenth hand-written list in a finding whose whole history is hand-written
 * lists falling behind the scanner.
 */
function disclosureCodes(): string[] {
  const go = readFileSync(
    join(
      import.meta.dirname,
      "..",
      "..",
      "platform",
      "internal",
      "shared",
      "skillpkg",
      "skillpkg.go",
    ),
    "utf8",
  );
  // `Code<Name> = "kebab-case"` — the constants, which is what the exported
  // `DisclosureCodes` slice is spelled in terms of. Taken from the slice's own
  // body so a constant that exists but is deliberately NOT a disclosure (the
  // spec and licence verdicts, which the Go header enumerates) stays out.
  const slice = /var DisclosureCodes = \[\]string\{([\s\S]*?)\n\}/.exec(go);
  expect(slice, "skillpkg.go has no DisclosureCodes slice — the parse broke").toBeTruthy();
  const names = [...slice![1].matchAll(/\b(Code\w+)\b/g)].map((m) => m[1]);
  return names.map((name) => {
    const decl = new RegExp(`\\b${name}\\s*=\\s*"([^"]+)"`).exec(go);
    expect(decl, `skillpkg.go declares ${name} in DisclosureCodes but nowhere else`).toBeTruthy();
    return decl![1];
  });
}

test("04 丙-29 ④: every disclosure code the scanner can emit reaches the screen", async () => {
  const codes = disclosureCodes();
  // Sentinel tied to the finding: the set went 6 → 12, so a parse that returns
  // fewer than twelve is a broken parse rather than a shrunken catalogue.
  expect(codes.length, "fewer than twelve disclosure codes parsed").toBeGreaterThanOrEqual(12);

  await act(async () => {
    root = createRoot(container);
    root.render(
      <RiskIndicator
        risk={{
          scan_status: "scanned",
          counts: { errors: 0, warnings: 0, infos: 0 },
          highlights: [],
          info_counts: {},
          // The server's wording, one entry per code. The labels below are this
          // test's own strings on purpose: what is under test is that the
          // renderer prints what it was given, for every code, not that it
          // agrees with a second copy of the catalogue.
          disclosures: codes.map((code) => ({
            code,
            label: `標籤：${code}`,
            note: `但書：${code}`,
          })),
          note: "來自匯入時的靜態掃描。",
        }}
      />,
    );
  });

  const text = container.textContent ?? "";
  for (const code of codes) {
    expect(text, `no label rendered for disclosure code ${code}`).toContain(`標籤：${code}`);
    // 設計 §2.4 第 3 項: the qualifier is visible text here, not a tooltip.
    expect(text, `no visible note rendered for disclosure code ${code}`).toContain(`但書：${code}`);
  }
  // And the clean-scan sentence is absent: twelve disclosures is not 「未發現」.
  expect(text).not.toContain("靜態掃描未發現錯誤或警告");
});
