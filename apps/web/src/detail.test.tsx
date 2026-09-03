import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { SkillDetail } from "./pages/SkillDetail";
import { SKILL_VERSIONS, skillDetail } from "./fixtures/platform";

/**
 * r2「產品資訊展示太多細節」在 `/skills/$skillId` 上的那一半，量測日期 2026-09-03：
 * 1231 個字元攤在 2930px、15 個等寬全寬區塊、12 個互為兄弟的 h2。
 *
 * 這一支測的是**重排沒有偷走任何東西**，而不是重排好不好看：
 *  - §2.10 的十項一項都沒有進 `<details>`（那是 §0 順位 2，封閉清單）；
 *  - §4.6.3 的「一頁至多一個主要動作」在重排之後仍然成立；
 *  - 標題大綱是 1 個 h1 ＋ 7 個 h2，而不是 12 個兄弟（§3 第 9 條）；
 *  - 工具清單在模型寫的那一段**之前**（§2.10 第 6 項：執行前要看得到）。
 *
 * 為什麼用 DOM 位置而不是文字比對：`textContent` 讀得到關閉的 `<details>` 裡面的字，
 * 所以「有這句話」證明不了「不用互動就看得到」。`closest("details")` 是這條規則唯一
 * 說得出真話的問法。
 */

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
  // `className` forwarded, for the same reason workspace.test.tsx forwards it:
  // 「一頁至多一個 .action」 is a rule about a class, and a mock that dropped the
  // class would let this file claim a page has none while it has three.
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

/** 共用 fixture 直接用：`category` 是契約的必填欄位，那一份已經帶著它。 */
function detailBody() {
  return skillDetail(SKILL, "PDF Summariser");
}

/** 擁有者：`/skills/{id}/versions` 是 workspace-scoped，非空＝這一份是你的（ADR-011）。 */
function stubOwner() {
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push({ url, method: init?.method ?? "GET" });
    const path = url.split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
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
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
  return calls;
}

/** 訪客：沒有 session，所以 `/me` 與版本清單都是 401（`RequireSession`）。 */
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

/** 每一個文字節點的容器，供「這句話在不在 `<details>` 裡」的提問使用。 */
function elementSaying(needle: string): Element {
  const found = Array.from(container.querySelectorAll("h1,h2,h3,p,li,span,code,strong,a")).find(
    (el) => (el.textContent ?? "").includes(needle) && el.children.length < 4,
  );
  expect(found, `找不到「${needle}」——這一句在頁面上消失了，不只是被折起來`).toBeDefined();
  return found!;
}

// --- §4.6.3 一頁至多一個主要動作 --------------------------------------------

test("r2: 重排之後，擁有者看到的填色動作仍然只有一個，而且是打包", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const actions = Array.from(container.querySelectorAll(".action"));
  expect(actions.map((a) => a.textContent?.trim())).toEqual(["打包並下載這個版本"]);
  // 右欄不得有填色動作：它的理由（可散布性判定）在主欄，把按鈕拉開會弄壞 §2.4。
  expect(container.querySelector(".detail-rail .action")).toBeNull();
});

test("r2: 未登入的訪客一個填色動作也沒有——零個是合法的", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);

  expect(container.querySelectorAll(".action")).toHaveLength(0);
});

// --- §2.10 十項：不得靠互動才看得到 ------------------------------------------

test("§2.10: 十項判斷事實一項都不在 <details> 裡", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const NEVER_FOLDED = [
    "錯誤 0／警告 1／提示 321", // 1 風險的計數與最高嚴重度
    "SKILL.md 內含可執行程式碼區塊。", // 1 每一條 warning 揭露
    "通過", // 2 相容性：規格驗證
    "已啟用", // 2 相容性：能力相容
    "腳本未執行,由模型轉譯", // 2 相容性：執行環境相容
    "License 已宣告", // 3 License status
    "可再散布", // 3 可散布性判定
    "套件宣告可用的工具", // 6 執行前的工具清單
    "收錄不等於精選。", // §2.11(c) 徽章的但書
    "未測量（沒有擷取到，不代表沒有）", // 10 每一個未測量
  ];

  for (const fact of NEVER_FOLDED) {
    expect(
      elementSaying(fact).closest("details"),
      `「${fact}」被折進 <details> 了——§2.10 是封閉清單，§0 順位 2`,
    ).toBeNull();
  }
});

test("§2.11(c): 標頭的徽章列——類別在前，而且每一顆都帶著伺服器那句但書", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const row = container.querySelector("header .badge-row")!;
  const badges = Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent);
  // 「這是什麼東西」在「可不可信」之前。
  expect(badges).toEqual(["文件", "已收錄", "來源可追溯"]);
  // 每一顆旁邊的那句話是可見文字，不是 `title`（§2.11(c)，`LabelledBadge` 檔頭）。
  expect(row.textContent).toContain("收錄不等於精選。");
  expect(row.querySelectorAll(".note")).toHaveLength(badges.length);
});

// --- §3 第 9 條：十二個兄弟 h2 ----------------------------------------------

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
  // 標題不得跳級：h1 之後只出現 h2，h2 之後才出現 h3。
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
  // 實測 2026-09-03 它在 y2244（整頁 77%），而 §2.10 第 6 項要的是「執行前」。
  expect(headings.indexOf("套件宣告可用的工具")).toBeLessThan(headings.indexOf("它能做什麼"));
});

// --- 04 丙-142 第一批：同一句話一頁講一次（§2.13） ---------------------------

/**
 * 「這段是模型寫的、沒有人核對」在這一頁曾經有四份可見複本（合計 47 字）加兩個
 * `title`。留下的是說得最完整的那一份（〈限制〉那一句同時解釋徽章語意與涵蓋範圍），
 * 加上伺服器自己那句 `enrichment.note`——它不只說「模型寫的」，還說了你的 Agent 讀的
 * 不是這一段，所以它不是同一句話。
 *
 * 押的是**次數**：`toContain` 對一句印四遍的話永遠是綠的。
 */
test("§2.13: 「模型寫的、沒有人核對」在這一頁只講一次，而且不在 title 裡", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  // 這一列自己那份 12 字的複述。留下的兩份各自多說了一件這一份沒說的事，所以它們
  // 不是同一句話；這一份是純粹的第三次。
  expect(text(), "同一句但書在一頁上印了不只一次（§2.13 去重第 1／2 條）").not.toContain(
    "由模型產生，未經人工核對",
  );
  // §2.4 的補句：原因搬出 `title` 之後，`title` 要拿掉——一句只活在 tooltip 裡的話
  // 在觸控裝置上不存在，留著就是同一句話的第二份。
  // （只問這一頁自己畫的那兩顆。`LabelledBadge` 與 `RiskIndicator` 今天仍然把伺服器
  // 的 `note` 同時放進 `title` 與可見文字，那是同一條規則的另外兩個呼叫點，記在丙-142
  // 的回報裡，不由這一支押。）
  expect(
    container.querySelector(".badge-source-model[title], .badge-source-template[title]"),
  ).toBeNull();
  // 第一訊號（可見文字）沒有跟著不見：徽章與它的但書都還在。
  expect(text()).toContain("AI 產生");
  expect(text()).toContain("「AI 產生」的項目由模型重述套件內容，未經人工核對。");
});

/**
 * §2.6／§2.13 的 F 類：一個**通過**的可用性探測是「這份東西有多舊」那條軸上的推導
 * 細節，不是判斷依據——它成立時畫面上什麼都不必改變。折進〈識別碼〉。
 *
 * 邊界在另一半：`unavailable_since`（「來源已失效，自 … 起」）是 §2.10 第 9 項的平台
 * 降級自述，永不折疊。兩者互斥，所以折疊區裡不會出現一句與外面矛盾的「當時可取得」。
 */
test("§2.6: 通過的來源可用性探測折進識別碼，降級自述不跟著進去", async () => {
  stubOwner();
  await render(<SkillDetail />, settledAsOwner);

  const probe = elementSaying("最近一次來源可用性檢查");
  expect(probe.closest("details"), "這一句以前平鋪在「它從哪裡來」的第一層").not.toBeNull();
  // 這個 fixture 的來源還活著，所以降級自述本來就不渲染——押的是它沒有被搬進去。
  expect(container.querySelector("details")?.textContent).not.toContain("來源已失效");
});

/**
 * §2.9／§2.10 第 10 項：缺席的**型別詞**三處都要留；搬走的是那段對三處都一樣的解釋。
 *
 * 這個 stub 是一個**登入了、但這不是他的 Skill、而且平台連版本內容都沒有回**的讀者：
 * 三個「無權檢視」同時渲染，那正是它們互為複本的那一格。
 */
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
  // 每一格自己的後果留在自己那一格（§2.4：控制項不在的原因）。
  expect(text()).toContain("沒有東西可以打包");
});

// --- r4 B1／B2：右欄的兩個建立動作 -------------------------------------------

test("r4 B2: Fork 那顆按鈕說得出它產生什麼", async () => {
  stubVisitor();
  await render(<SkillDetail />, settledAsVisitor);
  // 未登入時是那一句話加登入入口，按鈕不畫（§2.2 第三向）。
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

  // 契約：`POST /skills/{id}/versions`，body 是 application/zip 本身。
  expect(calls).toContainEqual({ url: `/skills/${SKILL}/versions`, method: "POST" });
  // 成功那一句要說出是哪一版，而不是「已上傳」。
  expect(container.querySelector('[role="status"]')?.textContent).toBe("已存成 v3。");
});
