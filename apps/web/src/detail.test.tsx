import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { SkillDetail } from "./pages/SkillDetail";
import { CATEGORIES, SKILL_VERSIONS, skillDetail } from "./fixtures/platform";

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
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    className?: string;
    children?: unknown;
  }) => (
    <a
      className={className}
      href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}
    >
      {children as never}
    </a>
  ),
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

function stubOwner() {
  const calls: Array<{ url: string; method: string }> = [];
  let category = CATEGORIES.documents;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push({ url, method: init?.method ?? "GET" });
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
    if (path.startsWith("/api/skills/")) return json({ ...detailBody(), category });
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

const text = () => container.textContent ?? "";
const settledAsOwner = () => text().includes("此 Skill 的 Test Case");
const settledAsVisitor = () => text().includes("登入後即可 Fork");

function elementSaying(needle: string): Element {
  const found = Array.from(container.querySelectorAll("h1,h2,h3,p,li,span,code,strong,a")).find(
    (el) => (el.textContent ?? "").includes(needle) && el.children.length < 4,
  );
  expect(found, `找不到「${needle}」——這一句在頁面上消失了，不只是被折起來`).toBeDefined();
  return found!;
}

test("r2: 重排之後，擁有者看到的填色動作仍然只有一個，而且是打包", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const actions = Array.from(container.querySelectorAll(".action"));
  expect(actions.map((a) => a.textContent?.trim())).toEqual(["打包並下載這個版本"]);
  expect(container.querySelector(".detail-rail .action")).toBeNull();
});

test("r2: 未登入的訪客一個填色動作也沒有——零個是合法的", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);

  expect(container.querySelectorAll(".action")).toHaveLength(0);
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
    "可再散布",
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

test("§3 第 9 條: 一個 h1、七個 h2，從屬段落降成 h3", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  expect(Array.from(container.querySelectorAll("h1")).map((h) => h.textContent)).toEqual([
    "PDF Summariser",
  ]);
  expect(Array.from(container.querySelectorAll("h2")).map((h) => h.textContent)).toEqual([
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

  expect(text(), "同一句但書在一頁上印了不只一次（§2.13 去重第 1／2 條）").not.toContain(
    "由模型產生，未經人工核對",
  );
  expect(
    container.querySelector(".badge-source-model[title], .badge-source-template[title]"),
  ).toBeNull();
  expect(text()).toContain("AI 產生");
  expect(text()).toContain("「AI 產生」的項目由模型重述套件內容，未經人工核對。");
});

test("§2.6: 通過的來源可用性探測折進識別碼，降級自述不跟著進去", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const probe = elementSaying("最近一次來源可用性檢查");
  expect(probe.closest("details"), "這一句以前平鋪在「它從哪裡來」的第一層").not.toBeNull();
  expect(container.querySelector("details")?.textContent).not.toContain("來源已失效");
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
  await render(<SkillDetail />, () => text().includes("這個 Skill 不在你的工作區"));

  const count = (needle: string) => text().split(needle).length - 1;
  expect(count("無權檢視"), "型別詞是封閉清單上的東西，三處一處都不能少").toBe(3);
  expect(count("這不代表它沒有版本"), "同一段解釋在一頁上講了不只一次").toBe(1);
  expect(text()).toContain("沒有東西可以打包");
});

test("r4 B2: Fork 那顆按鈕說得出它產生什麼", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  expect(text()).toContain("登入後即可 Fork 這個 Skill 到你的工作區。");

  vi.unstubAllGlobals();
  queryClient.clear();
  await act(async () => root?.unmount());
  container.remove();
  container = document.createElement("div");
  document.body.appendChild(container);

  stubOwner();
  await render(<SkillDetail />, settledAsOwner);
  const fork = Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("以這個 Skill 為起點"),
  );
  expect(fork?.textContent).toBe("以這個 Skill 為起點建立我自己的");
});

test("r4 B1: 上傳新版本的表單只給擁有者，而且打在契約寫的那條路徑上", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  expect(container.querySelector("#skill-version-file")).toBeNull();
  expect(text()).not.toContain("上傳成新版本");

  vi.unstubAllGlobals();
  queryClient.clear();
  await act(async () => root?.unmount());
  container.remove();
  container = document.createElement("div");
  document.body.appendChild(container);

  const calls = stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const input = container.querySelector<HTMLInputElement>("#skill-version-file");
  expect(input, "擁有者看不到上傳表單").not.toBeNull();
  expect(text()).toContain("把你改過的套件上傳成這個 Skill 的新版本；舊版本原封不動留著");

  const file = new File(["zip"], "skill.zip", { type: "application/zip" });
  Object.defineProperty(input!, "files", { value: [file] });
  await act(async () => input!.dispatchEvent(new Event("change", { bubbles: true })));
  await act(async () =>
    container
      .querySelector("form.version-upload")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
  );
  await waitFor(() => text().includes("已存成 v3。"));

  expect(calls).toContainEqual({ url: `/skills/${SKILL}/versions`, method: "POST" });
  expect(container.querySelector('[role="status"]')?.textContent).toBe("已存成 v3。");
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

  expect(calls).toContainEqual({ url: `/skills/${SKILL}/category`, method: "PUT" });
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
