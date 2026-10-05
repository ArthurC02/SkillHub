import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { SkillDetail } from "./detail/SkillDetail.page";
import { CATEGORIES, SKILL_VERSIONS, VERSION, skillDetail } from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

const SKILL = "11111111-1111-1111-1111-111111111111";

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
  Link: ({
    to,
    params,
    search,
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    className?: string;
    children?: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (acc, [key, value]) => acc.replace(`$${key}`, value),
      to,
    );
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter(
        (entry): entry is [string, string] => entry[1] !== undefined,
      ),
    );
    return (
      <a className={className} href={`${path}${query.size > 0 ? `?${query}` : ""}`}>
        {children as never}
      </a>
    );
  },
  useParams: () => ({ skillId: SKILL }),
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function detailBody() {
  return skillDetail(SKILL, "PDF Summariser");
}

function stubOwner(detail: Record<string, unknown> = {}) {
  const calls: Array<{ url: string; method: string; body?: unknown }> = [];
  let category = CATEGORIES.documents;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push({
      url,
      method: init?.method ?? "GET",
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    });
    const path = url.split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
    if (path.endsWith("/category") && init?.method === "PUT") {
      const body = JSON.parse(String(init.body)) as { category: keyof typeof CATEGORIES };
      category = CATEGORIES[body.category] ?? category;
      return json({
        skill_id: SKILL,
        name: "PDF Summariser",
        summary: "",
        redistribution: "allowed",
      });
    }
    if (path.endsWith("/versions") && init?.method === "POST")
      return json(
        {
          skill_id: SKILL,
          version_id: "22222222-2222-2222-2222-333333333333",
          version_number: 3,
          content_hash: "sha256:cc",
          duplicate: false,
          findings: { errors: [], warnings: [], infos: [] },
        },
        201,
      );
    if (path.endsWith("/versions")) return json(SKILL_VERSIONS);
    if (path.startsWith("/api/skills/")) return json({ ...detailBody(), ...detail, category });
    return json({ error: "not found" }, 404);
  });
  return calls;
}

function stubVisitor() {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json({ error: "not authenticated" }, 401);
    if (path.endsWith("/versions")) return json({ error: "not authenticated" }, 401);
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 401);
  });
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";
const settledAsOwner = () => text().includes("此小工具的測試題");
const settledAsVisitor = () => text().includes("登入後即可把這個小工具複製");

function elementSaying(needle: string): Element {
  const holders = Array.from(container.querySelectorAll("h1,h2,h3,p,li,span,code,strong,a")).filter(
    (el) => (el.textContent ?? "").includes(needle) && el.children.length < 4,
  );
  const innermost = holders.filter(
    (el) => !holders.some((other) => other !== el && el.contains(other)),
  );
  expect(
    innermost,
    `「${needle}」要剛好一個元素在說——0 個是這一句消失了，不只是被折起來；多個是這句話太泛、可能量錯元素`,
  ).toHaveLength(1);
  return innermost[0]!;
}

test("r2: 重排之後，擁有者看到的填色動作仍然只有一個，而且是打包", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const actions = Array.from(container.querySelectorAll(".action"));
  expect(actions.map((a) => a.textContent?.trim())).toEqual(["打包並下載這個版本"]);
  expect(container.querySelector(".detail-rail .action")).toBeNull();
  expect(container.querySelector("article.skill-detail > .detail-answer")).not.toBeNull();
  expect(container.querySelector(".detail-layout")?.firstElementChild?.className).toBe(
    "detail-rail",
  );
});

test("r2: 未登入的訪客一個填色動作也沒有——零個是合法的", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);

  expect(container.querySelectorAll(".action")).toHaveLength(0);
});

test.each([
  { mode: "GitHub", offline: false },
  { mode: "離線", offline: true },
])("$mode 訪客只在複製區看到一個登入入口，試跑、打包與版本仍指向它", async ({ offline }) => {
  vi.stubGlobal("__SKILLHUB_DEV_LOGIN__", offline);
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  await waitFor(
    () => text().includes("版本歷史需要登入") || text().includes("版本歷史只顯示你工作區裡的版本"),
  );

  const loginActions = container.querySelectorAll(
    offline ? 'button[type="submit"]' : 'a[href$="/auth/github/login"]',
  );
  expect(loginActions).toHaveLength(1);
  expect(loginActions[0].closest(".detail-rail section")?.querySelector("h2")?.textContent).toBe(
    "複製一份到你的工作區",
  );
  expect(text()).toContain("試跑需要你工作區裡的版本");
  expect(container.querySelectorAll('a[href="#fork-entry"]')).toHaveLength(2);
  const versionHeading = Array.from(container.querySelectorAll("h2")).find(
    (heading) => heading.textContent === "版本",
  );
  expect(versionHeading?.parentElement?.querySelector('[role="status"]')?.textContent).toContain(
    "版本歷史只顯示你工作區裡的版本",
  );
});

test("版本歷史讀取非 401 失敗仍顯示可重試的錯誤", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me") return json({ error: "not authenticated" }, 401);
    if (path.endsWith("/versions")) return json({ error: "temporarily unavailable" }, 503);
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
  await render(
    <SkillDetail />,
    () =>
      text().includes("暫時無法讀取版本歷史") || text().includes("版本歷史只顯示你工作區裡的版本"),
  );

  expect(text()).toContain("暫時無法讀取版本歷史。請重新整理，或稍後再試。");
  expect(text()).not.toContain("版本歷史只顯示你工作區裡的版本");
});

test("§2.10: 十項判斷事實一項都不在 <details> 裡", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const NEVER_FOLDED = [
    "錯誤 0／警告 1／提示 321",
    "SKILL.md 內含可執行程式碼區塊。",
    "通過",
    "已啟用",
    "腳本未執行,由模型轉譯",
    "License 已宣告",
    "可再散布MIT，可再散布。",
    "套件宣告可用的工具",
    "收錄不等於精選。",
    "未測量（沒有擷取到，不代表沒有）",
  ];

  for (const fact of NEVER_FOLDED) {
    expect(
      elementSaying(fact).closest("details"),
      `「${fact}」被折進 <details> 了——§2.10 是封閉清單，§0 順位 2`,
    ).toBeNull();
    expect(
      elementSaying(fact).closest("[data-tip]"),
      `「${fact}」被折進 Tip 了——§2.10 的十項一項都不准進去（§2.13 第 1 條）`,
    ).toBeNull();
  }
});

test("§2.11(c): 標頭的徽章列——類別在前，而且每一顆都帶著伺服器那句但書", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const row = container.querySelector("header .badge-row")!;
  const badges = Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent);
  expect(badges).toEqual(["文件", "已收錄", "來源可追溯"]);
  expect(row.textContent).toContain("收錄不等於精選。");
  expect(row.querySelectorAll(".note")).toHaveLength(badges.length);
});

test("§3 第 9 條: 一個 h1、十個 h2，頁面區段不跳級", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  expect(Array.from(container.querySelectorAll("h1")).map((h) => h.textContent)).toEqual([
    "PDF Summariser",
  ]);
  expect(Array.from(container.querySelectorAll("h2")).map((h) => h.textContent)).toEqual([
    "試跑",
    "複製一份到你的工作區",
    "類別",
    "風險揭露",
    "可散布性與打包",
    "相容性",
    "套件宣告可用的工具",
    "它能做什麼",
    "它從哪裡來",
    "版本",
  ]);
  const levels = Array.from(container.querySelectorAll("h1,h2,h3,h4,h5,h6")).map((h) =>
    Number(h.tagName.slice(1)),
  );
  for (const [i, level] of levels.entries()) {
    if (i > 0) expect(level).toBeLessThanOrEqual(levels[i - 1] + 1);
  }
});

test("r2 A1: 套件宣告可用的工具排在模型寫的那一段之前", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const headings = Array.from(container.querySelectorAll("h2")).map((h) => h.textContent);
  expect(headings.indexOf("套件宣告可用的工具")).toBeLessThan(headings.indexOf("它能做什麼"));
});

test("§2.13: 「模型寫的、沒有人核對」在這一頁只講一次，而且不在 title 裡", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const count = (needle: string) => text().split(needle).length - 1;
  expect(
    count("「AI 產生」的項目由模型重述套件內容，未經人工核對。"),
    "同一句但書在一頁上要剛好印一次（§2.13 去重第 1／2 條）",
  ).toBe(1);
  expect(
    container.querySelector(".badge-source-model[title], .badge-source-template[title]"),
  ).toBeNull();
  expect(text()).toContain("AI 產生");
});

test("§2.6: 通過的來源可用性探測折進識別碼，降級自述不跟著進去", async () => {
  stubOwner({
    source: {
      ...detailBody().source,
      last_checked_at: "2026-09-20T10:00:00Z",
      availability: { value: "lost", label: "來源已失效", note: "連續七天以上抓不到。" },
    },
  });
  await render(<SkillDetail />, settledAsOwner);

  const folded = elementSaying("最近一次來源可用性檢查").closest("details");
  expect(folded, "這一句以前平鋪在「它從哪裡來」的第一層").not.toBeNull();
  expect(folded!.textContent).not.toContain("來源已失效");
  expect(
    elementSaying("來源已失效").closest("details"),
    "降級自述是判斷事實，不准折起來",
  ).toBeNull();
});

const PLUGIN_SOURCE = {
  type: "git",
  url: "https://github.com/example/desk-tools",
  fetched_at: "2026-08-01T10:00:00Z",
  trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
  path: "skills/tidy-notes",
  plugin: {
    name: "desk-tools",
    version: "1.4.0",
    repository: "https://github.com/example/desk-tools",
    note: "只有這個小工具自己的目錄會被安裝。",
  },
  siblings: [
    { skill_id: "s-2", name: "Split CSV", path: "skills/split-csv" },
    { skill_id: "s-3", name: "Tag Inbox", path: "skills/tag-inbox" },
  ],
};

test("一個來自 Plugin 的小工具說出 Plugin 是哪一個、自己在裡面的哪個目錄", async () => {
  stubOwner({ source: PLUGIN_SOURCE });
  await render(<SkillDetail />, settledAsOwner);

  expect(text()).toContain("desk-tools");
  expect(text()).toContain("1.4.0");
  expect(text(), "沒有路徑，讀者回不到上游的那個目錄").toContain("skills/tidy-notes");
  expect(text(), "只說「來自一個 Plugin」而不說那代表什麼，等於沒說").toContain(
    "只有這個小工具自己的目錄會被安裝。",
  );
});

test("同一份來源帶進來的其他小工具各自有連結，而且不含自己", async () => {
  stubOwner({ source: PLUGIN_SOURCE });
  await render(<SkillDetail />, settledAsOwner);

  const heading = elementSaying("同一個來源帶進來的其他小工具（2）");
  const list = heading.nextElementSibling?.nextElementSibling;
  const links = Array.from(list?.querySelectorAll("a") ?? []).map((a) => a.getAttribute("href"));
  expect(links, "一套進來的小工具之間走不過去，使用者就看不出它們是一套").toEqual(
    expect.arrayContaining(["/skills/s-2", "/skills/s-3"]),
  );
  expect(links, "自己不是自己的同伴").not.toContain(`/skills/${SKILL}`);
});

test("來源不是 Plugin 時，不編造一個 Plugin 也不編造同伴", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  expect(text(), "來源沒有 Plugin 事實").not.toContain("來自 Agent Plugin");
  expect(text(), "一個來源只帶進一個小工具時，空的同伴清單是噪音").not.toContain(
    "同一個來源帶進來的其他小工具",
  );
});

const GIT_SOURCE = {
  type: "git",
  url: "https://github.com/example/pdf",
  fetched_at: "2026-08-01T10:00:00Z",
  last_checked_at: "2026-09-20T10:00:00Z",
  unavailable_since: "2026-09-01T10:00:00Z",
  trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
};

test("來源失效的判定照伺服器說的畫：失效才用風險色，暫時抓不到不是失效", async () => {
  stubOwner({
    source: {
      ...GIT_SOURCE,
      availability: { value: "lost", label: "來源已失效", note: "連續七天以上抓不到。" },
    },
  });
  await render(<SkillDetail />, settledAsOwner);
  expect(elementSaying("來源已失效").closest(".badge-risk")).not.toBeNull();
  expect(text()).toContain("連續七天以上抓不到。");

  await act(async () => root.unmount());
  queryClient.clear();
  stubOwner({
    source: {
      ...GIT_SOURCE,
      availability: { value: "unreachable", label: "暫時無法取得", note: "還不到七天。" },
    },
  });
  await render(<SkillDetail />, settledAsOwner);
  expect(text(), "還不到七天就說失效，會讓一次上游故障看起來像來源消失").not.toContain(
    "來源已失效",
  );
  expect(elementSaying("暫時無法取得").closest(".badge-risk")).toBeNull();
});

test("§2.13: 「無權檢視」三處都在，但那段解釋只講一次", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
    if (path.endsWith("/versions")) return json({ versions: [] });
    if (path.startsWith("/api/skills/")) return json({ ...detailBody(), version: undefined });
    return json({ error: "not found" }, 404);
  });
  await render(<SkillDetail />, () => text().includes("這個小工具不在你的工作區"));

  const count = (needle: string) => text().split(needle).length - 1;
  expect(count("無權檢視"), "型別詞是封閉清單上的東西，三處一處都不能少").toBe(3);
  expect(count("這不代表它沒有版本"), "同一段解釋在一頁上講了不只一次").toBe(1);
  expect(text()).toContain("沒有東西可以打包");
});

test("r4 B2: 複製一份那顆按鈕說得出它產生什麼", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  expect(text()).toContain("登入後即可把這個小工具複製到你的工作區。");

  vi.unstubAllGlobals();
  queryClient.clear();
  await act(async () => root?.unmount());
  container.remove();
  container = document.createElement("div");
  document.body.appendChild(container);

  stubOwner();
  await render(<SkillDetail />, settledAsOwner);
  const fork = Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("以這個小工具為起點"),
  );
  expect(fork?.textContent).toBe("以這個小工具為起點建立我自己的");
});

test("the overview sends version work to the exact immutable version context", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  expect(container.querySelector("#skill-version-file")).toBeNull();

  vi.unstubAllGlobals();
  queryClient.clear();
  await act(async () => root?.unmount());
  container.remove();
  container = document.createElement("div");
  document.body.appendChild(container);

  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  expect(container.querySelector("#skill-version-file")).toBeNull();
  expect(
    container.querySelector(
      `a[href="/skills/${SKILL}/versions/22222222-2222-2222-2222-222222222222"]`,
    ),
  ).not.toBeNull();
});

test("the validation entry keeps the newest owned version in the handoff", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  expect(
    container.querySelector(`a[href="/lab/test-cases?skill=${SKILL}&version=${VERSION}"]`),
  ).not.toBeNull();
});

function selectValue(select: HTMLSelectElement, value: string) {
  select.value = value;
  select.dispatchEvent(new Event("change", { bubbles: true }));
}

test("05 R-19: 擁有者看得到類別選單，四個選項齊全，送出後畫面更新", async () => {
  const calls = stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const select = container.querySelector<HTMLSelectElement>("#skill-category");
  expect(select, "擁有者看不到類別選單").not.toBeNull();
  expect(
    Array.from(select!.querySelectorAll("option"))
      .map((o) => o.value)
      .sort(),
  ).toEqual(["data", "documents", "unassigned", "writing"].sort());
  expect(select!.value).toBe("documents");

  await act(async () => selectValue(select!, "writing"));
  await act(async () =>
    Array.from(container.querySelectorAll("button"))
      .find((b) => (b.textContent ?? "").includes("儲存"))!
      .click(),
  );
  await waitFor(() => text().includes("類別已更新。"));

  expect(calls).toContainEqual({
    url: `/skills/${SKILL}/category`,
    method: "PUT",
    body: { category: "writing" },
  });
  await waitFor(() => container.querySelector("header .badge-row")!.textContent!.includes("寫作"));
});

test("05 R-19: 非擁有者看不到類別選單", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  expect(container.querySelector("#skill-category")).toBeNull();
});

test("05 R-19: 類別儲存失敗時顯示可以再按一次，而不是類別已更新", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
    if (path.endsWith("/category") && init?.method === "PUT") return json({ error: "boom" }, 500);
    if (path.endsWith("/versions")) return json(SKILL_VERSIONS);
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
  await render(<SkillDetail />, settledAsOwner);

  const select = container.querySelector<HTMLSelectElement>("#skill-category")!;
  await act(async () => selectValue(select, "writing"));
  await act(async () =>
    Array.from(container.querySelectorAll("button"))
      .find((b) => (b.textContent ?? "").includes("儲存"))!
      .click(),
  );
  await waitFor(() => text().includes("類別沒有設定成功，可以再按一次。"));

  expect(container.querySelector('[role="alert"]')?.textContent).toBe(
    "類別沒有設定成功，可以再按一次。",
  );
  expect(text()).not.toContain("類別已更新。");
});
