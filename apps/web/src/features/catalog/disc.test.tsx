import { ME as SESSION, platformResponse } from "../../testing/fixtures/platform";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, expect, test, vi } from "vitest";
import App from "../../app/App";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { onlineManager } from "@tanstack/react-query";
import { router } from "../../app/router";
import { LicenseBadge, LicenseNotes } from "../../shared/ui/LicenseBadge";
import { RiskIndicator } from "../../shared/ui/RiskIndicator";
import type {
  CatalogResponse,
  PublicSearchResponse,
  PublicSearchResult,
  SkillDetail,
  SkillFiles,
  SkillLicense,
  SkillRisk,
} from "../../core/api/types";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { preloadEveryPage } from "../../testing/pages";

beforeAll(preloadEveryPage);

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
  // The router is a module singleton that keeps its last location across
  // tests even after window.location resets, so send it home again once it
  // is remounted.
  if (container.querySelector(".app-shell") && router.state.location.pathname !== "/") {
    await act(async () => {
      await router.navigate({ to: "/", search: {} });
    });
  }
}

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

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs, { flushBeforeFirstCheck: true });
}

const HIT_FACETS = {
  summary_source: "package",
  tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
  category: { value: "documents", label: "文件", note: "由策展判定。" },
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
  "summary_source" | "tier" | "category" | "risk" | "dependencies" | "compatibility"
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
        skill_id: "8f6b4724-c3e9-5df0-82e0-5dd4096af602",
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

  expect(calls.some((url) => url.includes("/api/skills/catalog?limit=100&script=no"))).toBe(true);
  expect(container.textContent).toContain("目錄共 3 個小工具，這裡列出 1 個");
  expect(container.textContent?.split(rankNote).length - 1).toBe(1);
  expect(container.textContent).not.toContain("未計算語意相似度");
});

const CATALOG_ROW = {
  ...HIT_FACETS,
  skill_id: "8f6b4724-c3e9-5df0-82e0-5dd4096af602",
  name: "Catalog One",
  summary: "A browsable skill.",
  rank: null,
  rank_note: "目錄依收錄時間排序。",
  verified_at: "2026-09-03T00:00:00Z",
  match_reason: "",
  match_reason_source: "template" as const,
};

test("DISC-006: 把搜尋框清空再按搜尋，回到目錄而不是「沒有夠接近的小工具」", async () => {
  const calls = stubCatalog({ results: [CATALOG_ROW], limit: 20, total: 1, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "沒有這種東西" } });
  });
  await submitSearch("   ");
  await waitFor(() => container.textContent?.includes("Catalog One") ?? false);

  expect(calls.some((url) => url.includes("/api/skills/catalog"))).toBe(true);
  expect(container.textContent).not.toContain("沒有夠接近的小工具");
});

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

type CategoryPage = { results: PublicSearchResult[]; total: number };

function stubCategoryCatalog(pages: Record<string, CategoryPage>) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    if (url.includes("/api/skills/catalog")) {
      const category = new URLSearchParams(url.split("?")[1] ?? "").get("category") ?? "";
      const { results, total } = pages[category];
      return Promise.resolve(
        new Response(
          JSON.stringify({ results, limit: 100, total, truncated: total > results.length }),
          {
            status: 200,
          },
        ),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });
  return calls;
}

const CURATED = { value: "curated", label: "精選", note: "已完成人工檢視，不代表安全保證。" };

const SHELF_ROWS: PublicSearchResult[] = [
  {
    ...CATALOG_ROW,
    skill_id: "b8e28da6-94be-564b-8d1a-2847dec59c01",
    name: "Doc Curated",
    tier: CURATED,
    category: { value: "documents", label: "文件", note: "由策展判定。" },
  },
  {
    ...CATALOG_ROW,
    skill_id: "316e1f8c-dd2f-55a5-9bce-fdfcbb0d014b",
    name: "Doc Plain",
    category: { value: "documents", label: "文件", note: "由策展判定。" },
  },
  {
    ...CATALOG_ROW,
    skill_id: "45f29452-6430-55ba-b61a-4c4a8221df64",
    name: "Write Plain",
    category: { value: "writing", label: "寫作", note: "由策展判定。" },
  },
  {
    ...CATALOG_ROW,
    skill_id: "1b5f0d4e-5477-5b5e-a49a-838b293c4ef8",
    name: "Data Plain",
    category: { value: "data", label: "資料", note: "由策展判定。" },
  },
];

const SHELF_PAGES: Record<string, CategoryPage> = {
  "": { results: SHELF_ROWS, total: 40 },
  documents: { results: SHELF_ROWS.slice(0, 2), total: 12 },
  writing: { results: SHELF_ROWS.slice(2, 3), total: 7 },
  data: { results: SHELF_ROWS.slice(3, 4), total: 21 },
};

function chips(): string[] {
  return [...container.querySelectorAll(".category-nav .chip")].map((a) =>
    (a.textContent ?? "").replace(/\s+/g, ""),
  );
}

function currentChips(): string[] {
  return [...container.querySelectorAll(".category-nav .chip[aria-current]")].map((a) =>
    (a.textContent ?? "").replace(/（.*/s, "").trim(),
  );
}

async function browseCatalogue() {
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  await waitFor(() => container.textContent?.includes("Doc Curated") ?? false);
  await waitFor(() => !chips().some((c) => c.includes("…")));
}

test("Catalog landing leads with the 小工具 gallery and keeps creation outside the hero", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const hero = container.querySelector(".hero")!;
  expect(hero.querySelector("h1")?.textContent).toBe("探索能直接採用的 Agent 小工具");
  expect(hero.textContent).not.toContain("自己做一個小工具");
  expect(container.querySelector("#catalog-heading")?.textContent).toBe("小工具探索畫廊");
  expect(container.querySelectorAll(".catalog-gallery .catalog-skill-card")).toHaveLength(
    SHELF_ROWS.length,
  );
  expect(container.querySelectorAll(".catalog-gallery")).toHaveLength(1);

  const create = [...container.querySelectorAll("a")].find((link) =>
    link.textContent?.includes("前往資產庫匯入或建立"),
  )!;
  expect(create.closest(".hero")).toBe(null);
  expect(create.getAttribute("href")).toContain("/library#create");

  const lastGallery = [...container.querySelectorAll(".catalog-gallery")].at(-1)!;
  const supportingNotes = container.querySelector(".catalog-supporting-notes")!;
  expect(
    lastGallery.compareDocumentPosition(supportingNotes) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
});

test("DISC-002 類別: each chip carries the count the server gives for that category", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  expect(chips()).toEqual(["全部（40）", "文件（12）", "寫作（7）", "資料（21）"]);
  expect(currentChips()).toEqual(["全部"]);
  const nav = container.querySelector(".category-nav")!.textContent ?? "";
  expect(nav).not.toMatch(/下載|星|使用人數|熱門/);
});

test("DISC-002 類別: a chip narrows the catalogue through the URL, and 全部 clears it", async () => {
  const calls = stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const writing = [...container.querySelectorAll<HTMLAnchorElement>(".category-nav .chip")].find(
    (a) => a.textContent?.startsWith("寫作"),
  )!;
  await act(async () => writing.click());
  await waitFor(() => !container.textContent?.includes("Doc Curated"));

  expect(new URLSearchParams(window.location.search).get("category")).toBe("writing");
  expect(calls.some((url) => /catalog\?limit=100[^"]*category=writing/.test(url))).toBe(true);
  expect(container.textContent).toContain("Write Plain");
  expect(writing.getAttribute("aria-current")).toBe("page");
  expect(currentChips()).toEqual(["寫作"]);

  const all = [...container.querySelectorAll<HTMLAnchorElement>(".category-nav .chip")].find((a) =>
    a.textContent?.startsWith("全部"),
  )!;
  await act(async () => all.click());
  await waitFor(() => container.textContent?.includes("Doc Curated") ?? false);
  expect(new URLSearchParams(window.location.search).has("category")).toBe(false);
});

test("DISC-002 類別: the chip row and the filter select write the same URL param", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  await chooseFilter("類別", "data");
  await waitFor(() => container.textContent?.includes("Data Plain") ?? false);
  expect(new URLSearchParams(window.location.search).get("category")).toBe("data");

  expect(currentChips()).toEqual(["資料"]);

  await chooseFilter("類別", "");
  expect(new URLSearchParams(window.location.search).has("category")).toBe(false);
});

test("DISC-002 類別: chip 數量的請求失敗時印「測量失敗」，不是停在「…」", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/api/skills/catalog")) {
      const params = new URLSearchParams(url.split("?")[1] ?? "");
      if (params.get("limit") === "1") {
        return Promise.resolve(new Response(JSON.stringify({ error: "boom" }), { status: 500 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({ results: [CATALOG_ROW], limit: 100, total: 1, truncated: false }),
          { status: 200 },
        ),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  await waitFor(() => container.textContent?.includes("Catalog One") ?? false);
  await waitFor(() => chips().some((c) => c.includes("測量失敗")));

  expect(chips().every((c) => c.includes("測量失敗"))).toBe(true);
  expect(chips().some((c) => c.includes("…"))).toBe(false);
});

for (const cachedTotal of [0, 40]) {
  test(`category counts recover from cached ${cachedTotal} without losing the draft or focus`, async () => {
    let phase: "initial" | "failed" | "recovered" = "initial";
    const countCalls: string[] = [];
    vi.stubGlobal("fetch", async (input: string) => {
      const url = String(input);
      if (!url.includes("/api/skills/catalog")) {
        const { body, status } = platformResponse(url);
        return new Response(JSON.stringify(body), { status });
      }
      const params = new URLSearchParams(url.split("?")[1] ?? "");
      const category = params.get("category") ?? "";
      const page = SHELF_PAGES[category];
      if (params.get("limit") === "1") {
        countCalls.push(url);
        if (phase === "failed" && (cachedTotal !== 0 || category === "documents"))
          return new Response(null, { status: 503 });
        const total =
          phase !== "recovered"
            ? cachedTotal === 0
              ? 0
              : page.total
            : cachedTotal === 0
              ? page.total
              : 0;
        return new Response(JSON.stringify({ total }), { status: 200 });
      }
      return new Response(
        JSON.stringify({ ...page, limit: 100, truncated: page.total > page.results.length }),
        {
          status: 200,
        },
      );
    });
    await browseCatalogue();
    await waitFor(() => countCalls.length === 4);
    expect(chips()).toEqual(
      cachedTotal === 0 ? [] : ["全部（40）", "文件（12）", "寫作（7）", "資料（21）"],
    );
    const input = container.querySelector<HTMLInputElement>('input[aria-label="任務描述"]')!;
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    await act(async () => {
      setValue.call(input, "保留我的任務草稿");
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.focus();
    });
    const location = window.location.href;
    const filters = [
      {},
      { category: "documents" },
      { category: "writing" },
      { category: "data" },
    ] as const;
    phase = "failed";
    await act(async () => {
      await Promise.all(
        filters.map((filter) =>
          queryClient.refetchQueries({
            queryKey: queryKeys.skills.catalogTotal(filter),
            exact: true,
          }),
        ),
      );
    });
    await waitFor(() => chips().some((chip) => chip.includes("測量失敗")));
    expect(chips()).toEqual(
      cachedTotal === 0
        ? ["全部（0）", "文件（測量失敗）", "寫作（0）", "資料（0）"]
        : ["全部（測量失敗）", "文件（測量失敗）", "寫作（測量失敗）", "資料（測量失敗）"],
    );
    const main = container.querySelector("main")!;
    expect(main.querySelectorAll('[role="alert"]')).toHaveLength(1);
    expect(main.textContent).toContain("暫時無法讀取分類數量");
    expect(input.value).toBe("保留我的任務草稿");
    expect(document.activeElement).toBe(input);
    const retry = [...main.querySelectorAll("button")].find(
      (button) => button.textContent === "再試一次",
    )!;
    onlineManager.setOnline(false);
    try {
      phase = "recovered";
      await act(async () => retry.click());
      await waitFor(() => main.textContent?.includes("等待連線以讀取分類數量") ?? false);
      expect(retry.disabled).toBe(true);
      expect(retry.textContent).toBe("重新讀取中…");
      expect(document.activeElement?.id).toBe("catalog-search-heading");
      expect(countCalls).toHaveLength(8);
    } finally {
      await act(async () => onlineManager.setOnline(true));
    }
    await waitFor(
      () =>
        countCalls.length === 12 &&
        main.querySelectorAll('[role="alert"]').length === 0 &&
        !chips().some((chip) => chip.includes("…") || chip.includes("測量失敗")) &&
        !main.textContent?.includes("等待連線以讀取分類數量"),
    );
    expect(chips()).toEqual(
      cachedTotal === 0 ? ["全部（40）", "文件（12）", "寫作（7）", "資料（21）"] : [],
    );
    expect(input.value).toBe("保留我的任務草稿");
    expect(document.activeElement?.id).toBe("catalog-search-heading");
    expect(window.location.href).toBe(location);
  });
}

test("category count failures prioritize one login notice over another category error", async () => {
  vi.stubGlobal("fetch", async (input: string) => {
    const url = String(input);
    if (!url.includes("/api/skills/catalog")) {
      const { body, status } = platformResponse(url);
      return new Response(JSON.stringify(body), { status });
    }
    const params = new URLSearchParams(url.split("?")[1] ?? "");
    if (params.get("limit") === "1")
      return new Response(null, { status: params.get("category") === "documents" ? 401 : 503 });
    return new Response(JSON.stringify({ ...SHELF_PAGES[""], limit: 100, truncated: true }), {
      status: 200,
    });
  });
  await browseCatalogue();
  const main = container.querySelector("main")!;
  await waitFor(() => main.textContent?.includes("分類數量需要登入") ?? false);
  expect(
    [...main.querySelectorAll('[role="status"]')].filter((status) =>
      status.textContent?.includes("分類數量"),
    ),
  ).toHaveLength(1);
  expect(main.querySelectorAll('[role="alert"]')).toHaveLength(0);
  expect(main.textContent).not.toContain("暫時無法讀取分類數量");
});

test("DISC-001 搜尋文字超過 2000 字：送出前擋下並說明，不打伺服器", async () => {
  const calls = stubSearch(EMPTY);
  await render(<App />);

  await submitSearch("a".repeat(2001));

  const alert = container.querySelector('[role="alert"]');
  const input = container.querySelector<HTMLInputElement>('input[aria-label="任務描述"]')!;
  expect(alert?.textContent).toBe("搜尋文字最多 2000 字，目前 2001 字。");
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(input.getAttribute("aria-describedby")).toBe(alert?.id);
  expect(calls.some((url) => url.includes("/api/skills/search"))).toBe(false);
});

test("the catalogue presents every product in one compact scan surface", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const galleries = container.querySelectorAll(".catalog-gallery");
  const cards = [...container.querySelectorAll(".catalog-skill-card")];

  expect(galleries).toHaveLength(1);
  expect(cards).toHaveLength(SHELF_ROWS.length);
  expect(container.querySelector(".catalog-total")?.textContent).toContain("共 40 個，顯示前 4 個");
  expect(container.querySelector(".curated-shelf")).toBeNull();
  expect(container.querySelector("#rest-heading")).toBeNull();

  for (const [index, card] of cards.entries()) {
    const row = SHELF_ROWS[index];
    const text = card.textContent?.replace(/\s+/g, "") ?? "";
    expect(card.querySelector("h3")?.textContent).toBe(row.name);
    expect(text).toContain(row.summary.replace(/\s+/g, ""));
    expect(text).toContain(row.tier.label);
    expect(text).toContain(row.category.label);
    expect(text).toContain("規格驗證：通過");
    expect(text).toContain("尚未試跑");
    expect(text).toContain("依賴未提供");
    expect(text).toContain("掃描未見警告≠安全");
    expect(card.querySelector('time[datetime="2026-09-03T00:00:00Z"]')).not.toBeNull();
    expect(card.querySelector(".result-facets")).toBeNull();
  }
});

test("the curated review explanation follows the unified product gallery", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const gallery = container.querySelector(".catalog-gallery")!;
  const warning = container.querySelector(".catalog-curation-warning")!;
  expect(warning.textContent?.replace(/\s+/g, "")).toContain("不代表安全或推薦");
  expect(warning.compareDocumentPosition(gallery) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
    Node.DOCUMENT_POSITION_FOLLOWING,
  );
  const noteElement = container.querySelector(".catalog-curation-note")!;
  const note = noteElement.textContent!.replace(/\s+/g, "");
  expect(note).toContain("這一版由我們逐份讀過");
  expect(note).toContain("九項人工檢視");
  for (const item of [
    "來源可追溯",
    "License實查",
    "規格驗證",
    "Script逐行審閱",
    "無疑似Secret",
    "白話摘要",
    "至少一次平台基準試跑符合",
  ]) {
    expect(note).toContain(item);
  }
  expect(note).toContain("這不是安全保證，也不是推薦");
  expect(note).toContain("審查綁在這一版的位元組上");
  expect(note).toContain("平台未執行套件程式碼");
  expect(note).toContain("更新後若未重審，就會掉回「已索引」");
  expect(precedes(gallery, noteElement)).toBe(true);
  expect(note).toContain("不是從沒被審過");
  expect(container.textContent).not.toMatch(/官方推薦|已認證|Verified/);
  for (const el of container.querySelectorAll("[title]")) {
    expect(el.getAttribute("title")).not.toContain("九項人工檢視");
  }
});

function precedes(first: Element, second: Element): boolean {
  return Boolean(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING);
}

function noteElement(scope: Element, text: string): Element {
  const note = [...scope.querySelectorAll("p.note")].find((item) =>
    item.textContent?.includes(text),
  );
  expect(note, `找不到可見但書「${text}」`).toBeDefined();
  expect(note!.closest("details, [hidden]")).toBeNull();
  return note!;
}

test("目錄：先顯示卡片，必要警語、facet 與詞彙說明緊接在畫廊之後", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const cards = [...container.querySelectorAll(".catalog-skill-card")];
  expect(cards).toHaveLength(SHELF_ROWS.length);
  expect(precedes(cards.at(-1)!, noteElement(container, "未經人工核對"))).toBe(true);
  expect(
    precedes(cards.at(-1)!, noteElement(container, "你的 Agent 讀的是套件自己的 description")),
  ).toBe(true);
  expect(precedes(cards.at(-1)!, noteElement(container, "類別：由策展判定。"))).toBe(true);
  expect(precedes(cards.at(-1)!, noteElement(container, "標記說明："))).toBe(true);
  expect(noteElement(container, "標記說明：").textContent).toContain("來源未標示");
});

test("設計 §0: the catalogue lands with the filter bar shut, at every width", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();
  expect(
    container.querySelector<HTMLDetailsElement>(".filter-disclosure")!.open,
    "no filter is set, yet the bar opens above the catalogue",
  ).toBe(false);
});

function noteTexts(): string[] {
  return [...container.querySelectorAll(".note")].map((n) => (n.textContent ?? "").trim());
}

test("設計 §2.13: 逐位元相同的 note 提到清單層級，會分辨列的留在列上", async () => {
  stubCategoryCatalog(SHELF_PAGES);
  await browseCatalogue();

  const distinct = (pick: (r: PublicSearchResult) => string | undefined) =>
    new Set(SHELF_ROWS.map((r) => pick(r) ?? ""));
  expect(distinct((r) => r.tier.note).size, "the fixture must span two tier notes").toBe(2);
  const rows = [...container.querySelectorAll(".catalog-skill-card")];
  expect(rows).toHaveLength(SHELF_ROWS.length);
  const rowNotes = (row: Element) =>
    [...row.querySelectorAll(".catalog-card-facts .note")].map((n) => (n.textContent ?? "").trim());

  for (const [line, sentence] of [
    ["類別：由策展判定。", "由策展判定。"],
    ["相容狀態：尚未試跑。", "尚未試跑。"],
    ["風險提示：以上為靜態掃描結果。", "以上為靜態掃描結果。"],
  ] as const) {
    expect(
      noteTexts().filter((n) => n === line),
      `「${line}」 is not stated exactly once for the whole list`,
    ).toHaveLength(1);
    for (const row of rows) {
      expect(rowNotes(row), `「${sentence}」 is still repeated on a row`).not.toContain(sentence);
    }
  }

  for (const line of [
    "來源層級「精選」：已完成人工檢視，不代表安全保證。",
    "來源層級「已收錄」：收錄不等於精選。",
  ]) {
    expect(
      noteTexts().filter((n) => n === line),
      `「${line}」 is not stated once`,
    ).toHaveLength(1);
  }
  expect(new Set(SHELF_ROWS.map((r) => r.tier.label)).size).toBe(2);

  for (const [i, row] of rows.entries()) {
    expect(rowNotes(row), "a lifted tier note is still repeated on a row").not.toContain(
      SHELF_ROWS[i].tier.note,
    );
    const tierBadge = row.querySelector(".catalog-card-facts [class*='badge-tier-']");
    expect(tierBadge?.textContent).toBe(SHELF_ROWS[i].tier.label);
    expect(row.querySelector(".catalog-card-facts [class*='badge-category-']")?.textContent).toBe(
      SHELF_ROWS[i].category.label,
    );
  }
});

test("設計 §2.11(c): 去重之後，每一列的 facet 仍然帶著逐列不同的但書", async () => {
  const disclosed = {
    scan_status: "scanned" as const,
    level: "disclosed" as const,
    warnings: 0,
    disclosures: [
      { code: "script-file", label: "含可執行 Script 檔案", note: "平台不曾執行它們。" },
    ],
    note: "來自匯入時的靜態掃描。",
  };
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 2,
    results: [
      {
        ...HIT_FACETS,
        skill_id: "99999999-9999-9999-9999-999999999999",
        name: "PDF Summariser",
        summary: "把 PDF 整理成摘要",
        rank: 0.82,
        risk: disclosed,
      },
      {
        ...HIT_FACETS,
        skill_id: "99999999-9999-9999-9999-999999999998",
        name: "PDF Splitter",
        summary: "切分 PDF",
        rank: 0.6,
        risk: disclosed,
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const withNote = [...container.querySelectorAll(".search-result .result-facets dd")].filter(
    (dd) => dd.querySelector(".note"),
  );
  expect(
    withNote.length,
    "no facet row carries a qualifier any more — e2e's `checked === 0` would fail",
  ).toBeGreaterThan(0);
  expect(withNote.map((dd) => dd.textContent ?? "").join("")).toContain("平台不曾執行它們。");
  expect(container.textContent).toContain("風險提示：來自匯入時的靜態掃描。");
});

test("DISC-006: an empty catalog is distinct from a failed catalog read", async () => {
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: {} });
  });
  await waitFor(() => container.textContent?.includes("目錄裡還沒有任何東西") ?? false);
  expect(container.textContent).toContain("先把現成套件匯入自己的資產庫");
  expect(container.textContent).not.toContain("還沒有匯入過任何小工具");
  expect(container.querySelector('[role="alert"]')).toBeNull();
  expect(container.textContent).not.toContain("清掉篩選條件");
  await act(async () => root.unmount());
  queryClient.clear();
  container.replaceChildren();
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false }, 503);
  await render(<App />);
  await waitFor(() => container.textContent?.includes("無法讀取目錄") ?? false);
  expect(container.textContent).not.toContain("目錄裡還沒有任何東西");
});

test("DISC-006: an empty filtered catalog explains how to recover", async () => {
  stubCatalog({ results: [], limit: 20, total: 0, truncated: false });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/", search: { tier: "curated" } });
  });
  await waitFor(() => container.textContent?.includes("沒有小工具符合目前的篩選條件") ?? false);
  expect(container.textContent).toContain("清掉篩選條件");
  expect(container.textContent).not.toContain("還沒有匯入過任何小工具");
});

test("returning to browse does not render a cached search beside the catalog", async () => {
  const searchHit: PublicSearchResult = {
    ...HIT_FACETS,
    skill_id: "e47f9fcb-2205-5460-9c3a-126c6b5f1ada",
    name: "Cached Search Only",
    summary: "search result",
    rank: 0.9,
    verified_at: "2026-09-03T00:00:00Z",
    match_reason: "matched",
    match_reason_source: "template",
  };
  const catalogHit: PublicSearchResult = {
    ...searchHit,
    skill_id: "871898b7-70e1-5cdc-baf9-0e3212732daa",
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
    total: 2,
    results: [
      {
        ...HIT_FACETS,
        skill_id: "11111111-1111-1111-1111-111111111111",
        name: "PDF Summariser",
        summary: "把 PDF 轉成摘要",
        summary_source: "model",
        rank: 0.82,
        match_reason: "這個小工具直接處理 PDF 並輸出摘要。",
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
  expect(text).toContain("這個小工具直接處理 PDF 並輸出摘要。");
  expect(text).toContain("查詢與文件共同出現：pdf");
  expect(container.querySelectorAll(".match-reason .badge-source-model")).toHaveLength(1);
  expect(container.querySelectorAll(".match-reason .badge-source-template")).toHaveLength(1);

  expect(container.querySelectorAll(".badge-source-package")).toHaveLength(1);
  expect(text).toContain("AI 改寫");
  expect(text).toContain("作者原文");
  expect(container.querySelectorAll(".badge-source-unknown")).toHaveLength(0);
});

function reasonHit(n: number): PublicSearchResult {
  return {
    ...HIT_FACETS,
    skill_id: `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`,
    name: `Skill ${n}`,
    summary: "把 PDF 轉成摘要",
    summary_source: "model",
    rank: 0.8,
    match_reason: `查詢與文件共同出現：pdf ${n}`,
    match_reason_source: "template",
  };
}

function stubSearchThenReasons(search: PublicSearchResponse, reasons: () => Promise<Response>) {
  const reasonBodies: { query: string; skill_ids: string[] }[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/api/skills/search/reasons")) {
      reasonBodies.push(JSON.parse(String(init?.body)));
      return reasons();
    }
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(search), { status: 200 }));
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });
  return reasonBodies;
}

test("DISC-002: results show before the model reasons, which then replace the template ones", async () => {
  const [first, second] = [reasonHit(1), reasonHit(2)];
  let answer!: (r: Response) => void;
  const bodies = stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf 摘要", results: [first, second], total: 2 },
    () => new Promise((resolve) => (answer = resolve)),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => bodies.length === 1);
  expect(bodies[0]).toEqual({ query: "pdf 摘要", skill_ids: [first.skill_id, second.skill_id] });
  expect(container.textContent).toContain("查詢與文件共同出現：pdf 1");
  expect(container.querySelectorAll(".match-reason .badge-source-model")).toHaveLength(0);

  await act(async () =>
    answer(
      new Response(
        JSON.stringify({
          reasons: [{ skill_id: first.skill_id, reason: "它把 PDF 直接整理成摘要。", fits: true }],
        }),
        { status: 200 },
      ),
    ),
  );
  await waitFor(() => container.textContent?.includes("它把 PDF 直接整理成摘要。") ?? false);
  expect(container.textContent).not.toContain("查詢與文件共同出現：pdf 1");
  expect(container.textContent).toContain("查詢與文件共同出現：pdf 2");
  expect(container.querySelectorAll(".match-reason .badge-source-model")).toHaveLength(1);
  expect(container.querySelectorAll(".match-reason .badge-source-template")).toHaveLength(1);
});

test("DISC-002: a result the model judges unfit is labelled in place, never moved", async () => {
  const hits = [reasonHit(1), reasonHit(2), reasonHit(3)];
  stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf", results: hits, total: 3 },
    () =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            reasons: [
              { skill_id: hits[0].skill_id, reason: "它處理試算表，不讀 PDF。", fits: false },
              { skill_id: hits[1].skill_id, reason: "它把 PDF 直接整理成摘要。", fits: true },
              { skill_id: hits[2].skill_id, reason: "它也能讀 PDF。", fits: true },
            ],
          }),
          { status: 200 },
        ),
      ),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => container.textContent?.includes("它處理試算表，不讀 PDF。") ?? false);
  const rows = [...container.querySelectorAll(".search-result")];
  expect(rows.map((row) => row.querySelector("a")?.textContent)).toEqual([
    "Skill 1",
    "Skill 2",
    "Skill 3",
  ]);
  const reasons = rows.map((row) => row.querySelector(".match-reason"));
  expect(reasons[0]?.querySelector(".badge-warning")?.textContent).toBe("可能不合用");
  expect(reasons[0]?.textContent).not.toContain("符合原因：");
  for (const fitting of reasons.slice(1)) {
    expect(fitting?.querySelector(".badge-warning")).toBeNull();
    expect(fitting?.textContent).toContain("符合原因：");
  }
});

test("DISC-002: a failed reasons call keeps the template reasons and raises no alert", async () => {
  const bodies = stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf", results: [reasonHit(1)], total: 1 },
    () => Promise.resolve(new Response(JSON.stringify({ error: "boom" }), { status: 500 })),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => bodies.length === 1);
  await act(async () => {});
  expect(container.textContent).toContain("查詢與文件共同出現：pdf 1");
  expect(container.querySelectorAll(".match-reason .badge-source-template")).toHaveLength(1);
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("DISC-002: a reasons answer without a reasons list leaves the results and their templates intact", async () => {
  const bodies = stubSearchThenReasons(
    {
      ...EMPTY,
      query: "pdf",
      reason_query: "pdf",
      results: [reasonHit(1), reasonHit(2)],
      total: 2,
    },
    () =>
      Promise.resolve(new Response(JSON.stringify({ query: "pdf", results: [] }), { status: 200 })),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => bodies.length === 1);
  await act(async () => {});
  expect(container.querySelectorAll(".search-results > li")).toHaveLength(2);
  expect(container.textContent).toContain("查詢與文件共同出現：pdf 2");
});

const REASON_ID = "abcdefab-1234-4abc-8def-123456789abc";
const REASON_ROW = { skill_id: REASON_ID, reason: "不應採用的模型說明", fits: true };

for (const [name, payload] of [
  ["null envelope", null],
  ["missing list", {}],
  ["non-array list", { reasons: {} }],
  ["null item", { reasons: [null] }],
  ["object reason", { reasons: [{ ...REASON_ROW, reason: {} }] }],
  ["blank reason", { reasons: [{ ...REASON_ROW, reason: "   " }] }],
  ["missing verdict", { reasons: [{ skill_id: REASON_ID, reason: REASON_ROW.reason }] }],
  ["invalid verdict", { reasons: [{ ...REASON_ROW, fits: "false" }] }],
  ["unrequested identity", { reasons: [{ ...REASON_ROW, skill_id: reasonHit(3).skill_id }] }],
  [
    "ambiguous identity",
    { reasons: [REASON_ROW, { ...REASON_ROW, skill_id: REASON_ID.toUpperCase(), fits: false }] },
  ],
] as const) {
  test(`malformed match reasons ${name} retain results and are not cached as successful`, async () => {
    const first = { ...reasonHit(1), skill_id: REASON_ID };
    const second = reasonHit(2);
    stubSearchThenReasons(
      { ...EMPTY, query: "pdf", reason_query: "pdf", results: [first, second], total: 2 },
      () => Promise.resolve(new Response(JSON.stringify(payload), { status: 200 })),
    );
    await render(<App />);
    await submitSearch("pdf");
    const key = queryKeys.skills.matchReasons("pdf", [first.skill_id, second.skill_id]);
    await waitFor(() => queryClient.getQueryState(key)?.fetchStatus === "idle");
    expect(queryClient.getQueryState(key)?.status).toBe("error");
    expect(queryClient.getQueryData(key)).toBeUndefined();
    expect(container.querySelectorAll(".search-results > li")).toHaveLength(2);
    expect(container.textContent).toContain("查詢與文件共同出現：pdf 1");
    expect(container.textContent).toContain("查詢與文件共同出現：pdf 2");
    expect(container.querySelectorAll(".match-reason .badge-source-model")).toHaveLength(0);
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });
}

test("a case-equivalent reason identity labels the requested result without changing its order", async () => {
  const first = { ...reasonHit(1), skill_id: REASON_ID };
  const second = reasonHit(2);
  stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf", results: [first, second], total: 2 },
    () =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            reasons: [
              { skill_id: REASON_ID.toUpperCase(), reason: "這個版本的格式不適合。", fits: false },
            ],
          }),
          { status: 200 },
        ),
      ),
  );
  await render(<App />);
  await submitSearch("pdf");
  await waitFor(() => container.textContent?.includes("這個版本的格式不適合。") ?? false);
  const rows = [...container.querySelectorAll(".search-results > li")];
  expect(rows.map((row) => row.querySelector(".result-card-head > a")?.textContent)).toEqual([
    "Skill 1",
    "Skill 2",
  ]);
  expect(rows[0].querySelector(".match-reason .badge-warning")?.textContent).toBe("可能不合用");
  expect(rows[1].querySelector(".match-reason")?.textContent).toContain(
    "查詢與文件共同出現：pdf 2",
  );
});

test("empty model reasons are a successful optional read that retains both templates", async () => {
  const hits = [reasonHit(1), reasonHit(2)];
  stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf", results: hits, total: 2 },
    () => Promise.resolve(new Response(JSON.stringify({ reasons: [] }), { status: 200 })),
  );
  await render(<App />);
  await submitSearch("pdf");
  const key = queryKeys.skills.matchReasons(
    "pdf",
    hits.map((hit) => hit.skill_id),
  );
  await waitFor(() => queryClient.getQueryState(key)?.status === "success");
  expect(queryClient.getQueryData(key)).toEqual({ reasons: [] });
  expect(container.querySelectorAll(".search-results > li")).toHaveLength(2);
  expect(container.querySelectorAll(".match-reason .badge-source-template")).toHaveLength(2);
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test.each([
  [10, 10],
  [11, 10],
])("DISC-002: %i results ask for reasons on the first %i", async (shown, asked) => {
  const results = Array.from({ length: shown }, (_, i) => reasonHit(i + 1));
  const bodies = stubSearchThenReasons(
    { ...EMPTY, query: "pdf", reason_query: "pdf", results, total: shown },
    () => Promise.resolve(new Response(JSON.stringify({ reasons: [] }), { status: 200 })),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => bodies.length === 1);
  expect(bodies[0].skill_ids).toEqual(results.slice(0, asked).map((r) => r.skill_id));
});

test("DISC-002: a search without reason_query never asks for reasons", async () => {
  const bodies = stubSearchThenReasons(
    { ...EMPTY, query: "pdf", results: [reasonHit(1)], total: 1 },
    () => Promise.resolve(new Response(JSON.stringify({ reasons: [] }), { status: 200 })),
  );
  await render(<App />);
  await submitSearch("pdf");

  await waitFor(() => container.textContent?.includes("查詢與文件共同出現：pdf 1") ?? false);
  await act(async () => {});
  expect(bodies).toHaveLength(0);
});

test("DISC-002: a search with missing summary provenance is a read failure rather than author credit", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 1,
    results: [
      {
        ...HIT_FACETS,
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

  expect(container.textContent).toContain("暫時無法讀取搜尋結果");
  expect(container.querySelectorAll(".search-result")).toHaveLength(0);
  expect(container.querySelectorAll(".badge-source-package")).toHaveLength(0);
});

test("DISC-002: a truncated result page says so, and says how many it is showing", async () => {
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
  expect(container.textContent).toContain("共 47 個");
  expect(container.textContent).not.toContain("超過");
  expect(container.textContent).not.toContain("召回率明顯較低");
});

test("DISC-005: degraded and partial_index are separate, non-blocking notices", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 1,
    results: [
      {
        ...HIT_FACETS,
        skill_id: "33333333-3333-3333-3333-333333333333",
        name: "Lexical Hit",
        summary: "只靠關鍵字命中",
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
  expect(text).not.toContain("embedding unavailable; lexical search only");
  expect(text).toContain("目前只用關鍵字比對搜尋");
  expect(text).toContain("Lexical Hit");
  expect(text).not.toMatch(/相似度\s*[\d.]/);
  expect(text).not.toContain("1.4");
  expect(text).toContain("此頁改用關鍵字比對排序，未計算語意相似度。");
});

test("DISC-002: an unranked hit on the hybrid path reads as unscored, not 0.00", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 1,
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
  expect(text).toContain("已宣告");
  expect(text).toContain("repo 根目錄 LICENSE");
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
  expect(text).toContain("SKILL.md 內含可執行程式碼區塊。");
  expect(container.querySelector(".risk-list .badge-risk")).not.toBeNull();
  expect(text).toContain("SKILL.md 內含可執行程式碼");

  const details = container.querySelector("details.risk-infos")!;
  expect(details).not.toBeNull();
  expect((details as HTMLDetailsElement).open).toBe(false);
  expect(details.textContent).toContain("external-url");
  expect(details.textContent).toContain("320");
  expect(details.querySelectorAll("li")).toHaveLength(2);
});

const TWO_HITS: PublicSearchResponse = {
  ...EMPTY,
  query: "pdf",
  total: 4,
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

test("搜尋：標題、計數、必要警語與 facet 說明先於首卡，排序與詞彙說明在末卡之後", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");

  const cards = [...container.querySelectorAll(".search-result")];
  expect(cards).toHaveLength(TWO_HITS.results.length);
  expect(precedes(container.querySelector("#results-heading")!, cards[0])).toBe(true);
  expect(precedes(noteElement(container, "找到 4 個小工具。"), cards[0])).toBe(true);
  expect(precedes(noteElement(container, "未經人工核對"), cards[0])).toBe(true);
  expect(
    precedes(noteElement(container, "你的 Agent 讀的是套件自己的 description"), cards[0]),
  ).toBe(true);
  expect(precedes(noteElement(container, "類別：由策展判定。"), cards[0])).toBe(true);
  expect(precedes(cards.at(-1)!, container.querySelector("details.ranking-explainer")!)).toBe(true);
  expect(precedes(cards.at(-1)!, noteElement(container, "標記說明："))).toBe(true);
  expect(noteElement(container, "標記說明：").textContent).toContain("作者原文");
});

test("DISC-004: the ranking rule is explained on demand and matches the pipeline", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");

  const explainer = container.querySelector<HTMLDetailsElement>("details.ranking-explainer")!;
  expect(explainer).not.toBeNull();
  expect(explainer.open).toBe(false);

  const text = explainer.textContent ?? "";
  expect(text).toContain("語意相似度");
  expect(text).toContain("關鍵字命中只用來多找候選，不會改變名次");
  expect(text).toContain("低於 0.25");
  expect(text).toContain("不看 Star 數");
  expect(text).toContain("只能用關鍵字比對時");
  expect(text).toContain("還沒建立語意索引");
  expect(text).not.toContain("目前只用關鍵字比對搜尋");
  expect(text).not.toContain("部分小工具尚未建立語意索引");
});

test("DISC-004: 降級自述在 details 外面平鋪，不在裡面當徽章", async () => {
  stubSearch({ ...TWO_HITS, degraded: true, degraded_reason: "embedding unavailable" });
  await render(<App />);
  await submitSearch("pdf");

  const notices = [...container.querySelectorAll(".notice")].map((n) => n.textContent ?? "");
  expect(
    notices.some((n) => n.includes("目前只用關鍵字比對搜尋")),
    "the degraded self-report is not stated in the flat, always-visible layer",
  ).toBe(true);
  const explainer = container.querySelector<HTMLDetailsElement>("details.ranking-explainer")!;
  expect(explainer.open, "the explainer is the collapsed layer this fact may not live in").toBe(
    false,
  );
  expect(explainer.textContent ?? "").not.toContain("目前只用關鍵字比對搜尋");
});

function compareLink() {
  return container.querySelector<HTMLAnchorElement>('.compare-bar a[href^="/compare"]');
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

  await waitFor(() => container.querySelector(".compare-bar") === null);
  expect(compareLink()).toBeNull();

  await pick(0);
  expect(compareLink()).toBeNull();
  expect(container.textContent).toContain("再選 1 個小工具");

  await pick(1);
  expect(compareLink()?.getAttribute("href")).toContain(
    "ids=aaaaaaaa-0000-0000-0000-000000000001%2Caaaaaaaa-0000-0000-0000-000000000002",
  );

  await pick(2);
  expect(compareLink()?.textContent).toContain("3 個");
  const boxes = container.querySelectorAll<HTMLInputElement>(".compare-pick input");
  expect(boxes[3].disabled).toBe(true);
  expect(boxes[0].disabled).toBe(false);

  await pick(0);
  expect(container.querySelectorAll<HTMLInputElement>(".compare-pick input")[3].disabled).toBe(
    false,
  );
});

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

test("DISC-009: narrowing the same results keeps the comparison candidates", async () => {
  stubSearch(TWO_HITS);
  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);

  await chooseFilter("來源層級", "curated");
  await waitFor(() => new URLSearchParams(window.location.search).get("tier") === "curated");

  const params = new URLSearchParams(window.location.search);
  expect(params.get("compare")).toBe(
    "aaaaaaaa-0000-0000-0000-000000000001,aaaaaaaa-0000-0000-0000-000000000002",
  );
  expect(compareLink()).not.toBeNull();
  const boxes = container.querySelectorAll<HTMLInputElement>(".compare-pick input");
  expect(boxes[0].checked).toBe(true);
  expect(boxes[1].checked).toBe(true);
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
  await waitFor(() => container.querySelector(".compare-bar") === null);
  expect(compareLink()).toBeNull();
});

function detailFixture(overrides: Partial<SkillDetail>): SkillDetail {
  return {
    skill_id: "unset",
    name: "unset",
    summary: "把 PDF 轉成摘要",
    scope: "catalog",
    tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
    category: { value: "documents", label: "文件", note: "由策展判定。" },
    enrichment: { status: "pending", note: "尚未產生白話摘要。" },
    limitations: [],
    license: { status: { value: "unknown", label: "License 未知", note: "未宣告 License。" } },
    redistribution: {
      value: "unknown",
      label: "可散布性未確認",
      note: "沒有人確認過這個小工具可不可以再散布。",
    },
    derivation: { is_fork: false, label: "來源關係", note: "不是複製品。" },
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
            ...SESSION,
            email: "t@example.com",
            display_name: "tester",
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
                version_id: "22222222-2222-2222-2222-222222222222",
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
    skill_id: "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190",
    name: "左邊的小工具",
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
  const right = detailFixture({
    skill_id: "097339be-b801-5bdb-ad79-424f562272bd",
    name: "右邊的小工具",
  });
  const calls = stubSearchAndDetails(
    {
      ...EMPTY,
      query: "pdf",
      total: 2,
      results: [
        {
          ...HIT_FACETS,
          skill_id: "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190",
          name: "左邊的小工具",
          summary: "甲",
          rank: 0.7,
        },
        {
          ...HIT_FACETS,
          skill_id: "097339be-b801-5bdb-ad79-424f562272bd",
          name: "右邊的小工具",
          summary: "乙",
          rank: 0.6,
        },
      ],
    },
    { "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190": left, "097339be-b801-5bdb-ad79-424f562272bd": right },
  );

  await render(<App />);
  await submitSearch("pdf");
  await pick(0);
  await pick(1);
  await act(async () => compareLink()!.click());
  await waitFor(() => container.querySelector("table.compare-table") !== null);

  expect(container.textContent).toContain("左邊的小工具");
  expect(container.textContent).toContain("右邊的小工具");

  const tier = compareRow("來源層級");
  expect(tier.className).not.toContain("compare-differs");
  expect(tier.textContent).not.toContain("有差異");

  const license = compareRow("License");
  expect(license.className).toContain("compare-differs");
  expect(license.textContent).toContain("有差異");
  expect(license.textContent).toContain("MIT");
  expect(license.textContent).toContain("License 未知");

  const inputs = compareRow("輸入");
  expect(inputs.textContent).toContain("PDF");
  expect(inputs.querySelectorAll(".compare-unknown")).toHaveLength(1);
  const outputs = compareRow("輸出");
  expect(outputs.textContent, "左邊量過了，而且沒有輸出").toContain("0 項");
  expect(outputs.querySelectorAll(".compare-unknown"), "右邊還沒有標籤").toHaveLength(1);
  const limits = compareRow("限制");
  expect(limits.textContent, "左邊模型與掃描都跑過，兩邊都沒有發現").toContain("0 項");
  expect(limits.querySelectorAll(".compare-unknown"), "右邊還在處理").toHaveLength(1);

  const version = compareRow("版本與時間");
  expect(version.className).toContain("compare-differs");
  expect(version.querySelectorAll(".compare-unknown")).toHaveLength(1);
  expect(version.textContent).toContain("v1");

  expect(compareRow("相容性（驗證證據）").textContent).toContain("未驗證");

  const detailReads = calls.filter((url) =>
    [left.skill_id, right.skill_id].some((id) => url.includes(`/api/skills/${id}`)),
  );
  expect(detailReads.length).toBeGreaterThan(0);
  for (const url of detailReads) {
    expect([url, url.includes("view=embedded")]).toEqual([url, true]);
  }
});

test("DISC-009: a repeated URL id is still only one comparison candidate", async () => {
  const skill = detailFixture({
    skill_id: "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190",
    name: "Only 小工具",
  });
  const calls = stubSearchAndDetails(EMPTY, { "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190": skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({
      to: "/compare",
      search: { ids: "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190,09f5b9b7-4cc8-5fd2-b82e-059d12ab0190" },
    });
  });
  await waitFor(() =>
    calls.some((url) => url.includes("/api/skills/09f5b9b7-4cc8-5fd2-b82e-059d12ab0190?")),
  );

  expect(container.textContent).toContain("請從首頁的搜尋結果或目錄選擇 2 到 3 個小工具");
  expect(container.querySelector("table.compare-table")).toBeNull();
});

test("DISC-009: a failed read on /compare says so at once, not after seven seconds of 載入中", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/api/skills/search")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            ...EMPTY,
            query: "pdf",
            total: 2,
            results: [
              {
                ...HIT_FACETS,
                skill_id: "09f5b9b7-4cc8-5fd2-b82e-059d12ab0190",
                name: "左邊的小工具",
                summary: "甲",
                rank: 0.7,
              },
              {
                ...HIT_FACETS,
                skill_id: "097339be-b801-5bdb-ad79-424f562272bd",
                name: "右邊的小工具",
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

  await waitFor(() => (container.textContent ?? "").includes("讀取失敗"), 300);
  expect(container.textContent).toContain("有 2 個小工具讀取失敗");
});

test("DISC-004: a risk level of unknown reads as 未掃描, not as a clean row", async () => {
  const { RiskSummary } = await import("../../shared/ui/RiskIndicator");
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
  expect(text).toContain("沒有靜態掃描結果可讀");
  expect(text).toContain("那不是「掃過了、沒發現」");
  expect(text).not.toContain("靜態掃描未發現警告");
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

test("NFR-001: a clean scan says what the scan found and carries the rider that it is not safety", async () => {
  await render(
    <RiskIndicator
      risk={{
        scan_status: "scanned",
        counts: { errors: 0, warnings: 0, infos: 0 },
        highlights: [],
        info_counts: {},
        disclosures: [],
        note: "以上為靜態掃描結果。",
      }}
    />,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("靜態掃描未發現錯誤或警告");
  expect(text).toContain("這不等於安全");
  expect(text).not.toContain("掃描通過");
});

test("NFR-001: a clean row in a list carries the same rider as the detail page", async () => {
  const { RiskSummary } = await import("../../shared/ui/RiskIndicator");
  await render(
    <RiskSummary
      risk={{
        scan_status: "scanned",
        level: "none",
        warnings: 0,
        disclosures: [],
        note: "以上為靜態掃描結果。",
      }}
    />,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("靜態掃描未發現警告");
  expect(text).toContain("這不等於安全");
  expect(text).not.toContain("掃描通過");
});

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

  const last = calls[calls.length - 1];
  expect(last).toContain("script=yes");
  expect(last).toContain("validation=passed");
  const url = new URLSearchParams(window.location.search);
  expect(url.get("q")).toBe("pdf");
  expect(url.get("script")).toBe("yes");
  expect(url.get("validation")).toBe("passed");

  await chooseFilter("是否包含 Script", "");
  expect(new URLSearchParams(window.location.search).has("script")).toBe(false);
  expect(calls[calls.length - 1]).not.toContain("script=");
});

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

  expect(filterSelect("是否包含 Script").disabled).toBe(false);
  expect(filterSelect("驗證狀態").disabled).toBe(false);
  expect(filterSelect("Agent 相容").disabled).toBe(false);
  expect(filterSelect("來源層級").disabled).toBe(false);
  expect(filterSelect("類別").disabled).toBe(false);

  expect(filterSelect("需要 MCP").disabled).toBe(true);
  const text = container.querySelector(".filter-bar")!.textContent ?? "";
  expect(text).toContain("沒有記錄是否需要 MCP");
  expect(text).not.toContain("只存在於策展清單");
  expect(text).not.toContain("人工精選審查尚未開始");

  const deadSelects = [
    ...container.querySelectorAll<HTMLSelectElement>(".filter-bar select"),
  ].filter((s) => s.disabled);
  expect(deadSelects.length, "no dead control to check a reason on").toBeGreaterThan(0);
  for (const select of deadSelects) {
    const id = select.getAttribute("aria-describedby");
    const reason = id ? container.querySelector(`#${id}`) : null;
    expect(reason?.className, "the reason is not visible text").toContain("note");
    expect(
      (reason?.textContent ?? "").replace(/\s+/g, "").length,
      `a disabled filter whose stated reason is too short to be one`,
    ).toBeGreaterThan(15);
    expect(reason?.textContent ?? "").toContain("平台");
  }
});

test("設計 §0: the filter summary counts the filters that are actually there", async () => {
  stubSearch({ ...EMPTY, query: "pdf", no_results: true });
  await render(<App />);
  await submitSearch("pdf");

  const selects = [...container.querySelectorAll<HTMLSelectElement>(".filter-bar select")];
  const dead = selects.filter((s) => s.disabled).length;
  const live = selects.length - dead;
  expect(live, "no live filters found — the picker below is measuring nothing").toBeGreaterThan(0);
  expect(dead, "no dead filters found — 「N 項目前無法篩選」 would be a claim about nothing").toBe(
    1,
  );
  expect(live, "the summary's 「N 項可用」 has stopped counting the live controls").toBe(5);

  const summary = container
    .querySelector(".filter-disclosure > summary")!
    .textContent!.replace(/\s+/g, "");
  expect(summary).toContain(`${live}項可用`);
  expect(summary).toContain(`${dead}項目前無法篩選`);
});

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

  const options = Array.from(filterSelect("來源層級").options).map((o) => o.value);
  expect(options).toEqual(["", "curated", "indexed"]);

  const text = container.querySelector("#filter-why-tier")!.textContent ?? "";
  expect(text).toContain("沒有帶著人工審查結論");
  expect(text).not.toContain("未經人工審查");
});

test("DISC-002: the tier badge is the server's value, not a front-end guess", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 1,
    results: [
      {
        ...HIT_FACETS,
        tier: { value: "curated", label: "精選", note: "已完成人工檢視。" },
        skill_id: "11111111-1111-1111-1111-111111111111",
        name: "PDF Summariser",
        summary: "把 PDF 轉成摘要",
        rank: 0.82,
        match_reason: undefined,
        match_reason_source: undefined,
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const badge = container.querySelector(".search-result .result-facets")!.textContent ?? "";
  expect(badge).toContain("精選");
  expect(badge).not.toContain("已收錄");
});

test("DISC-003: 清除所有篩選 clears every filter, not the two somebody remembered", async () => {
  window.history.pushState({}, "", "/?q=pdf&script=none&validation=validated&agent=native");
  stubSearch({ ...EMPTY, query: "pdf", filtered_out: true });
  await render(<App />);
  await waitFor(() => (container.textContent ?? "").includes("清除所有篩選"));

  const clear = Array.from(container.querySelectorAll("button")).find((b) =>
    b.textContent?.includes("清除所有篩選"),
  )!;
  await act(async () => clear.click());
  await waitFor(() => !container.textContent?.includes("搜尋中…"));

  const left = [...new URLSearchParams(window.location.search).keys()];
  expect(left).toEqual(["q"]);
  expect(new URLSearchParams(window.location.search).get("q")).toBe("pdf");
});

test("DISC-003: filtered-to-empty and the no-results refusal never share copy", async () => {
  stubSearch({ ...EMPTY, query: "pdf", filtered_out: true });
  await render(<App />);
  await submitSearch("pdf");

  let text = container.textContent ?? "";
  expect(text).toContain("全部被目前的篩選條件排除");
  expect(text).not.toContain("沒有夠接近的小工具");

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
  expect(text).toContain("沒有夠接近的小工具");
  expect(text).toContain("Try naming the file format you have.");
  expect(text).not.toContain("全部被目前的篩選條件排除");
});

test("DISC-002: a result row carries all seven columns, and infers none of them", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    total: 2,
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
        name: "Unscanned 小工具",
        summary: "沒有掃描紀錄",
        rank: 0.3,
        risk: {
          scan_status: "unavailable",
          level: "unknown",
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
  expect(scanned).toContain("PDF Summariser");
  expect(scanned).toContain("把 PDF 整理成摘要");
  expect(scanned).toContain("已收錄");
  expect(scanned).toContain("規格驗證：通過");
  expect(scanned).toContain("含可執行 Script 檔案");
  expect(scanned).toContain("pypdf");
  expect([...container.querySelectorAll("time")].map((t) => t.getAttribute("dateTime"))).toContain(
    "2026-08-01T10:00:00Z",
  );
  expect(scanned).toContain("尚未試跑");

  const unscanned = rows[1].textContent ?? "";
  expect(unscanned).toContain("尚無掃描紀錄");
  expect(unscanned).not.toContain("未發現警告");
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
  expect(text).toContain("把 PDF 整理成摘要");
  expect(text).toContain("不處理掃描件的手寫字。");
  expect(text).toContain("套件內含可執行 Script。");
  expect(text).toContain("輸入：");
  expect(text).toContain("pdf");
  expect(text).toContain("輸出：");
  expect(text).toContain("markdown");
  expect(text).toContain("依賴：");
  expect(text).toContain("pypdf");
  expect(text).toContain("套件宣告可用的工具");
  expect(text).toContain("https://github.com/example/pdf");
  expect(text).toContain("License 未知");
  expect(text).toContain("規格驗證");
  expect(container.querySelectorAll(".badge-source-model").length).toBeGreaterThan(0);
});

test("DISC-006: an unenriched skill reads as unknown, never as 'needs nothing'", async () => {
  const skill = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-000000000002",
    name: "Bare 小工具",
  });
  stubSearchAndDetails(EMPTY, { [skill.skill_id]: skill });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: skill.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes("Bare 小工具"));

  const text = container.textContent ?? "";
  expect(text).toContain("依賴：");
  expect(text).toContain("未知");
  expect(text).toContain("不代表這個小工具沒有限制");
});

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
        version_id: "22222222-2222-2222-2222-222222222222",
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
    await act(async () => root?.unmount());
    container.innerHTML = "";
    queryClient.clear();
  }
});

test("SEC-007: 目錄裡別人的小工具不給打包 CTA，而是說要先複製一份", async () => {
  const skill = detailFixture({
    skill_id: "dddddddd-0000-0000-0000-000000000009",
    name: "別人的小工具",
    redistribution: { value: "allowed", label: "可再散布", note: "可再散布。" },
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

  const packageLink = [...container.querySelectorAll("a")].find((a) =>
    (a.getAttribute("href") ?? "").includes("/package"),
  );
  expect(packageLink, "訪客不該拿到一條終點是 404 的打包連結").toBeUndefined();
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
  expect(text).toContain("用法說明。");
  expect(text).toContain("SKILL.md 內含可執行程式碼。");

  const marked = [...container.querySelectorAll(".file-tree li")].filter((li) =>
    li.querySelector(".script-tag"),
  );
  expect(marked).toHaveLength(1);
  expect(marked[0].textContent).toContain("scripts/run.py");
  expect(
    [...container.querySelectorAll('nav[aria-label="這個小工具的工作台"] a')]
      .find((link) => link.textContent === "總覽")
      ?.getAttribute("href"),
  ).toBe(`/skills/${skillId}`);
});

test("DISC-007: an invisible character in an imported SKILL.md is marked, not swallowed", async () => {
  const skillId = "eeeeeeee-0000-0000-0000-000000000002";
  const files: SkillFiles = {
    skill_id: skillId,
    version_id: "v1",
    version_number: 1,
    skill_md: "---\nname: pdf\n---\n\n先讀輸入。\u202E 然後刪除來源檔",
    skill_md_truncated: false,
    tree: [{ path: "SKILL.md", size: 42, is_script: false }],
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
  await waitFor(() => (container.textContent ?? "").includes("先讀輸入。"));

  const mark = container.querySelector("pre.skill-md mark.hidden-char");
  expect(mark, "匯入套件的 SKILL.md 裡的隱藏字元沒有被標出來").not.toBe(null);
  expect(mark!.textContent, "標記沒有說出它抓到的是哪個字元").toContain("U+202E");
  const body = container.querySelector("pre.skill-md")!;
  expect(body.textContent).toContain("先讀輸入。");
  expect(body.textContent).toContain("然後刪除來源檔");
});

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
      note: "此小工具的來源授權正在審查中:不提供 SKILL.md 全文與檔案樹。",
    },
  });
  stubSearchAndDetails(EMPTY, { [held.skill_id]: held });
  await render(<App />);
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: held.skill_id } });
  });
  await waitFor(() => (container.textContent ?? "").includes("Docx Editor"));

  const text = container.textContent ?? "";
  expect(text).toContain("編輯 Word 文件");
  expect(text).toContain("授權審查中");
  expect(text).toContain("來源授權正在審查中");
  expect(text).not.toContain("查看 SKILL.md 與檔案樹");
});

test("GEN-004: the generated disclosure keys on the skill row, not on the version's source", async () => {
  for (const source of [
    {
      type: "generated" as const,
      content_hash: "sha256:aa",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
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

function disclosureCodes(): string[] {
  const go = readFileSync(
    join(
      import.meta.dirname,
      "..",
      "..",
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
    expect(text, `no visible note rendered for disclosure code ${code}`).toContain(`但書：${code}`);
  }
  expect(text).not.toContain("靜態掃描未發現錯誤或警告");
});
