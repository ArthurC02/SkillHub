import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { Downloads } from "../packaging/downloads/Downloads.page";
import { RunTrace } from "../runs/trace/RunTrace.page";
import { WorkspaceAccount } from "./account/WorkspaceAccount.page";
import { WorkspaceRuns } from "../runs/list/WorkspaceRuns.page";
import { WorkspaceSkills } from "./skills/WorkspaceSkills.page";
import { SkillDetail } from "../skill/detail/SkillDetail.page";
import { ImportSkill } from "../creation/import/ImportSkill.page";
import { CancelRunControl } from "../runs/trace/components/CancelRunControl";
import {
  SKILL_VERSIONS,
  VERSION,
  VERSION_DIFF,
  skillDetail,
} from "../../testing/fixtures/platform";
import { useForkSkill } from "../skill/skills.service";
import { BundleSection } from "../publishing";

const SKILL = "11111111-1111-1111-1111-111111111111";
const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const CANCEL_NOTE = "已送出取消要求；在工作負載真的停下來之前，這個 Run 會維持目前的狀態。";
const ARTIFACT = "33333333-3333-3333-3333-333333333333";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  await act(async () => queryClient.cancelQueries());
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
  useParams: () => ({ runId: RUN, skillId: SKILL }),
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
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

function button(text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes(text),
  );
}

const text = () => container.textContent ?? "";

async function open(label: string) {
  const details = Array.from(container.querySelectorAll("details")).find((d) =>
    (d.querySelector("summary")?.textContent ?? "").includes(label),
  )!;
  await act(async () => {
    details.open = true;
    details.dispatchEvent(new Event("toggle"));
  });
}

const RUN_ROW = {
  run_id: RUN,
  status: "succeeded",
  skill_id: SKILL,
  skill_name: "CSV 清理",
  skill_version_id: "22222222-2222-2222-2222-222222222222",
  provider: "self-hosted",
  cleanup_status: { value: "cleaned", label: "已清理", note: "沙箱與其資源已回收。" },
  evaluation: {
    value: "met",
    label: "符合",
    note: "依這個 Run 當時的驗收條件判定為符合。",
  },
  created_at: "2026-08-17T00:00:00Z",
  finished_at: "2026-08-17T00:04:00Z",
};

test("WS-004 a run history row words `succeeded` as execution, never as a pass", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [RUN_ROW] }));
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  expect(text()).toContain("執行狀態：執行完成");
  expect(text()).toContain("任務判定：符合");
  expect(text().indexOf("任務判定")).toBeLessThan(text().indexOf("執行狀態"));
  expect(text()).not.toContain("成功");
});

test("WS-004 an unevaluated run says 未評估, which is not a blank and not a pass", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        {
          ...RUN_ROW,
          evaluation: {
            value: "not_evaluated",
            label: "未評估",
            note: "這個 Run 還沒有任務判定。執行狀態說的是工作負載跑完了沒有,不是任務有沒有做到。",
          },
        },
        {
          ...RUN_ROW,
          run_id: "run-9",
          evaluation: {
            value: "evaluation_failed",
            label: "評估失敗",
            note: "判定沒有產生出來。這不代表任務失敗——沒有人判過,不是判過不合格。",
          },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  expect(text()).toContain("任務判定：未評估");
  expect(text()).toContain("不是任務有沒有做到");
  expect(text()).toContain("任務判定：評估失敗");
  expect(text()).toContain("這不代表任務失敗");
  expect(container.querySelectorAll(".badge-danger")).toHaveLength(0);
});

test("WS-004 a run whose sandbox was not cleaned up says so on the row", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        { ...RUN_ROW, status: "failed", status_reason: "the provider could not carry the attempt" },
        {
          ...RUN_ROW,
          run_id: "run-2",
          cleanup_status: { value: "failed", label: "清理失敗", note: "平台會重試。" },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("執行失敗"));

  expect(text()).toContain("the provider could not carry the attempt");
  expect(text()).toContain("清理失敗");
  expect(text()).toContain("平台會重試。");
  expect(
    Array.from(container.querySelectorAll("[title]")).map((e) => e.getAttribute("title")),
    "a `title=` is back on this page, duplicating text that is already visible",
  ).toEqual([]);
  expect(text()).toContain("已清理");
  expect(container.querySelectorAll(".badge-unverified")).toHaveLength(0);
  expect(container.querySelectorAll(".badge-danger")).toHaveLength(1);
});

test("WS-004 a cleanup state the client does not recognise is still rendered as words", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        {
          ...RUN_ROW,
          cleanup_status: { value: "cleaning_up", label: "清理中", note: "會自己結束。" },
        },
        {
          ...RUN_ROW,
          run_id: "run-3",
          cleanup_status: {
            value: "a-state-from-a-newer-server",
            label: "a-state-from-a-newer-server",
            note: "這個平台版本沒有這個清理狀態的說明,值照原樣顯示,不猜測它的意思。",
          },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("清理中"));

  expect(text()).toContain("清理狀態：清理中");
  expect(text()).toContain("a-state-from-a-newer-server");
  for (const badge of container.querySelectorAll(".badge")) {
    expect(badge.textContent?.trim(), "a badge with no word (§2.3)").not.toBe("");
  }
});

test("WS-002 an empty run history says nothing ran, not that records were cleared", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [] }));
  await render(<WorkspaceRuns />, () => text().includes("還沒有跑過任何 Run"));

  expect(text()).toContain("不是紀錄被清掉了");
});

const SCANNED = {
  risk: {
    scan_status: "scanned",
    level: "disclosed",
    warnings: 0,
    disclosures: [
      { code: "script-file", label: "含可執行 Script 檔案", note: "平台不曾執行它們。" },
    ],
    note: "來自匯入時的靜態掃描,不執行套件內任何程式碼;開啟 Skill 可看逐項結果。",
  },
  verification: {
    value: "scanned",
    label: "已掃描",
    note: "匯入這個版本時做過靜態掃描,不執行套件內任何程式碼;逐項結果在 Skill 頁面。",
    scanned_at: "2026-08-01T10:00:00Z",
  },
} as const;

const FORKED = {
  risk: {
    scan_status: "unavailable",
    level: "unknown",
    warnings: 0,
    disclosures: [],
    note: "此結果尚無掃描紀錄,狀態未知——不代表已通過檢查。",
  },
  verification: {
    value: "not_measured",
    label: "未測量",
    note: "這個版本是 Fork 進來的複本,靜態掃描是在來源工作區做的,平台沒有在你的工作區重跑。",
    scanned_at: null,
  },
} as const;

test("WS-004 a gallery card flags only what stops the skill being taken away", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "自己匯入的",
          summary: "一份自己傳上來的套件。",
          redistribution: "self_supplied",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-3",
          name: "沒人判定過的",
          summary: "目錄裡還沒有人分類的內容。",
          redistribution: "unknown",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-4",
          name: "平台生成的",
          summary: "依任務描述生成出來的。",
          redistribution: "generated",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-2",
          name: "Fork 來的",
          summary: "從目錄 Fork 的。",
          redistribution: "allowed",
          access_restriction: null,
          forked_from_skill_id: "s-origin",
          ...FORKED,
        },
      ],
      limit: 100,
      truncated: true,
      total: 137,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("一份自己傳上來的套件"));

  const flags = (name: string) =>
    Array.from(container.querySelectorAll(".skill-card"))
      .find((card) => (card.textContent ?? "").includes(name))
      ?.querySelector(".skill-card-flags")?.textContent ?? "";
  expect(flags("沒人判定過的")).toContain("授權未知，不能打包");
  expect(flags("平台生成的")).toContain("平台生成，未經人工檢視");
  expect(flags("自己匯入的")).not.toContain("可下載");
  expect(text(), "a card that can be taken away still says so").not.toContain("可打包下載");
  expect(text()).toContain("只列出前 100 個");

  expect(text(), "清單有列的時候，那句『公開目錄的不在』才是它在做的事").toContain(
    "公開目錄的不在",
  );
});

test("WS-004 a forked card reaches its source from the manage menu, and claims no scan", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "Fork 來的",
          summary: "從目錄 Fork 的。",
          redistribution: "allowed",
          access_restriction: null,
          forked_from_skill_id: "s-origin",
          ...FORKED,
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("從目錄 Fork 的"));

  const menu = container.querySelector(".skill-menu")!;
  expect(Array.from(menu.querySelectorAll("a"), (a) => a.getAttribute("href"))).toContain(
    "/skills/s-origin",
  );
  expect(container.querySelector(".skill-card-flags")).toBeNull();
  expect(text()).not.toContain("未發現警告");
});

test("WS-004 a gallery card keeps the scan's warning count and drops the all-clear sentence", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "有警告的",
          summary: "掃出兩個警告。",
          redistribution: "self_supplied",
          access_restriction: null,
          risk: { ...SCANNED.risk, warnings: 2, disclosures: [] },
          verification: SCANNED.verification,
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("掃出兩個警告"));

  expect(container.querySelector(".skill-card-flags")?.textContent).toBe("警告 2");
  expect(text()).not.toContain("靜態掃描未發現警告");
});

test("WS-004 the own-skills list links each row on to its files and packaging", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "CSV 清理",
          summary: "整理 CSV。",
          redistribution: "allowed",
          access_restriction: null,
          ...SCANNED,
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain(`/skills/${SKILL}`);
  expect(hrefs).toContain(`/skills/${SKILL}/files`);
  expect(hrefs).toContain(`/skills/${SKILL}/package`);
  expect(hrefs).toContain("/workspace/runs");
  expect(hrefs).toContain("/workspace/downloads");
});

test("IA-9 the empty own-skills list offers importing as a link, not as prose", async () => {
  vi.stubGlobal("fetch", () => json({ skills: [], limit: 100, truncated: false }));
  await render(<WorkspaceSkills />, () => text().includes("還沒有任何 Skill"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain("/workspace/import");
  expect(text()).toContain("空清單");
  expect(text()).toContain("不是讀取失敗");

  expect(text(), "空清單上仍然印著那句只對有列的清單成立的簡介").not.toContain("公開目錄的不在");
});

test("空清單先說它是哪一種空，三張建立卡才是它的動作（§3 checklist 第 1 條）", async () => {
  vi.stubGlobal("fetch", () => json({ skills: [], limit: 100, truncated: false }));
  await render(<WorkspaceSkills />, () => text().includes("還沒有任何 Skill"));

  const absence = Array.from(container.querySelectorAll("p")).find((p) =>
    p.textContent?.includes("不是讀取失敗"),
  );
  const hub = container.querySelector("#create");
  expect(absence, "空狀態那一句不見了").toBeTruthy();
  expect(hub, "建立中心不見了").toBeTruthy();
  expect(
    Boolean(absence!.compareDocumentPosition(hub!) & Node.DOCUMENT_POSITION_FOLLOWING),
    "「還沒有任何 Skill」必須排在「建立一個 Skill」之前：答案先出來，動作在後面",
  ).toBe(true);
});

function stubSkillDetailPage(versions: unknown = SKILL_VERSIONS, versionsStatus = 200) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push(url);
    const path = url.split("?")[0];
    if (path.endsWith("/versions")) return json(versions, versionsStatus);
    if (path.endsWith("/diff")) return json(VERSION_DIFF);
    if (path.startsWith("/api/skills/")) return json(skillDetail(SKILL, "PDF Summariser"));
    return json({ error: "not found" }, 404);
  });
  return calls;
}

test("WS-001 the detail page lists the versions, newest first, with the oldest saying why it has no comparison", async () => {
  stubSkillDetailPage();
  await render(<SkillDetail />, () => text().includes("v1"));
  await waitFor(() => text().includes("v1"));

  expect(text()).toContain("v2");
  expect(text()).toContain("v1");
  expect(text()).toContain("版本不可變");
  expect(text()).toContain("這是最早的版本，沒有上一版可以比較");
  expect(container.querySelector('time[datetime="2026-08-17T00:00:00Z"]')).not.toBeNull();
});

test("WS-001 第 4 條 比較 asks the contract's endpoint for the right two versions", async () => {
  const calls = stubSkillDetailPage();
  await render(<SkillDetail />, () => button("與上一版比較") !== undefined);
  await waitFor(() => button("與上一版比較") !== undefined);

  expect(calls.some((u) => u.includes("/diff"))).toBe(false);
  await act(async () => button("與上一版比較")?.click());
  await waitFor(() => text().includes("assets/logo.png"));

  const diff = calls.find((u) => u.includes("/diff"));
  expect(diff).toBe(
    `/skills/${SKILL}/diff?from=22222222-2222-2222-2222-111111111111&to=22222222-2222-2222-2222-222222222222`,
  );
  expect(text()).toContain("assets/logo.png");
  expect(text()).toContain("（二進位或過大，不顯示差異）");
});

test("WS-001 a version list that fails to read says so, and 401 says to log in", async () => {
  stubSkillDetailPage({ error: "not authenticated" }, 401);
  await render(<SkillDetail />, () => text().includes("版本歷史需要登入"));
  await waitFor(() => text().includes("版本歷史需要登入"));

  expect(text()).toContain("版本歷史需要登入");
  expect(text()).not.toContain("not authenticated");
  expect(text()).not.toContain("這是最早的版本");
});

function stubOwnSkillsWithFeatures(features?: Record<string, boolean>) {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me")
      return json({
        user_id: "u-1",
        email: "t@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        deletion_requested_at: null,
        purge_after: null,
        deletion_scope: null,
        ...(features ? { features } : {}),
      });
    return json({
      skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。", ...SCANNED }],
      limit: 100,
      truncated: false,
      total: 1,
    });
  });
}

test("GEN-008 ⛔ with the flag off, /workspace/skills has no generation entry point", async () => {
  stubOwnSkillsWithFeatures();
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  expect(text()).toContain("CSV 清理");
  expect(container.querySelector("#generate-task")).toBeNull();
  expect(text()).not.toContain("讓平台依你的描述做一個");
});

test("GEN-008 with the flag on, /workspace/skills shows the door and not the workbench", async () => {
  stubOwnSkillsWithFeatures({ generate_skill: true });
  await render(<WorkspaceSkills />, () => text().includes("開始描述"));

  expect(container.querySelector("#generate-task"), "工作台又長回這一頁上了").toBeNull();

  const door = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.textContent ?? "").includes("開始描述"),
  );
  expect(door, "第三張卡不是一扇門").toBeTruthy();
  expect(door!.getAttribute("href")).toBe("/workspace/creations");
});

const hub = () => container.querySelector<HTMLElement>(".create-hub");
const hubText = () => (hub()?.textContent ?? "").replace(/\s+/g, "");

test("建立中心 的三扇門同框，而且這一頁一個填色動作都沒有", async () => {
  stubOwnSkillsWithFeatures();
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  expect(hub()).not.toBeNull();
  expect(hub()!.id).toBe("create");

  const actions = container.querySelectorAll("a.action, button.action");
  expect(Array.from(actions).map((a) => a.getAttribute("href") ?? a.textContent)).toEqual([]);

  const doors = hub()!.querySelectorAll("a.action-secondary, button");
  expect(doors.length, "三扇門沒有全部拿到次要按鈕語彙").toBeGreaterThanOrEqual(2);

  expect(hub()!.querySelectorAll("ul.create-cards > li.download-item").length).toBeGreaterThan(0);
});

test("建立中心 states the invite requirement on the from-catalogue card, in visible text", async () => {
  stubOwnSkillsWithFeatures();
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  const card = Array.from(hub()!.querySelectorAll("li")).find((li) =>
    (li.textContent ?? "").includes("從目錄挑一個來改"),
  );
  expect(card, "the from-catalogue card is missing").toBeTruthy();
  const copy = (card!.textContent ?? "").replace(/\s+/g, "");
  expect(copy).toContain("平台目前只讓有封測邀請的帳號Fork。");
  expect(copy.indexOf("平台"), "這一句沒有以強制者開頭").toBe(
    copy.indexOf("平台目前只讓有封測邀請的帳號Fork。"),
  );
  expect(Array.from(card!.querySelectorAll("a")).map((a) => a.getAttribute("href"))).toContain("/");
});

test("建立中心 ⛔ with the flag off, the hub has no generation card and does not mention 生成", async () => {
  stubOwnSkillsWithFeatures();
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  expect(hubText()).toContain("匯入現成的套件");
  expect(hubText()).toContain("從目錄挑一個來改");

  expect(hub()!.querySelector("#generate-task")).toBeNull();
  expect(hubText()).not.toContain("生成");
  expect(hubText()).not.toContain("即將推出");
});

test("建立中心 on an empty list teaches each door", async () => {
  vi.stubGlobal("fetch", () => json({ skills: [], limit: 100, truncated: false }));
  await render(<WorkspaceSkills />, () => text().includes("還沒有任何 Skill"));

  expect(hub()!.querySelectorAll('[data-role="teaching"]').length).toBe(2);
});

test("建立中心 below a list that has a Skill keeps every door and the invite rule, without the teaching", async () => {
  stubOwnSkillsWithFeatures({ generate_skill: true });
  await render(<WorkspaceSkills />, () => text().includes("開始描述"));

  expect(hub()!.querySelectorAll('[data-role="teaching"]').length).toBe(0);
  expect(hub()!.querySelectorAll("a").length).toBe(3);
  expect(hubText()).toContain("平台目前只讓有封測邀請的帳號Fork。");
});

test("建立中心 with the flag on, the generation entry appears exactly once on the page", async () => {
  stubOwnSkillsWithFeatures({ generate_skill: true });
  await render(<WorkspaceSkills />, () => text().includes("開始描述"));

  expect(container.querySelectorAll("#generate-task").length).toBe(0);

  const doors = Array.from(container.querySelectorAll("a")).filter(
    (a) => a.getAttribute("href") === "/workspace/creations",
  );
  expect(doors.length).toBe(1);
  expect(hub()!.querySelectorAll('a[href="/workspace/creations"]').length).toBe(1);
});

test("WS-005 deleting a skill says what survives it before anything is destroyed", async () => {
  const calls: [string, string | undefined][] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deleted: true,
        versions_retained: 2,
        note: "已從你的工作區、清單與搜尋移除；版本快照維持凍結，這次刪除不會移除它們；Fork 引用的共用套件物件不受影響",
      });
    }
    return json({
      skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。", ...SCANNED }],
    });
  });
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  await act(async () => button("刪除")?.click());
  expect(text()).toContain("版本快照會凍結保留");
  expect(text()).not.toContain("再清除");
  expect(text()).toContain("別人 Fork 過的版本");
  expect(calls.some(([, method]) => method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => calls.some(([, method]) => method === "DELETE"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain(`/skills/${SKILL}`);

  await waitFor(() => text().includes("維持凍結"));
});

const ME = {
  user_id: "u-1",
  email: "tester@example.com",
  display_name: "tester",
  workspace_id: "ws-1",
  deletion_requested_at: null,
  purge_after: null,
  deletion_scope: null,
};

const DELETION_SCOPE =
  "寬限期結束前，你的帳號照常可用。到期後，你上傳的資料集、Run 產出，以及沒有任何人 Fork 或執行過的 Skill 會連同檔案永久刪除。被其他使用者 Fork 過、或歷史 Run 使用過的 Skill 版本會保留（它們的內容是別人的來源鏈），但你的身分會從上面移除，顯示為已刪除的使用者所有。";

const BALANCE = {
  balance_credits: 120,
  debt_floor_credits: -50,
  estimated_session: { low_credits: 1, high_credits: 3, sample_size: 0, estimated: true },
  can_start: true,
};

const STATEMENT_NOTE = "花了多少，以這裡為準。";

function stubAccountFetch(
  handler: (input: string, init?: RequestInit) => Promise<Response>,
  statement: (before: string | null) => unknown = () => ({ entries: [], note: STATEMENT_NOTE }),
) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname.endsWith("/me/credits/entries")) {
      return json(statement(url.searchParams.get("before")));
    }
    if (url.pathname.endsWith("/me/credits")) return json(BALANCE);
    if (url.pathname === "/skills") {
      return json({ skills: [], limit: 100, truncated: false, total: 0 });
    }
    if (url.pathname === "/me/bundles") return json({ bundles: [] });
    return handler(input, init);
  });
}

test("CRED-009 the account page lists what each credit went to and links only the runs it names", async () => {
  stubAccountFetch(
    () => json(ME),
    () => ({
      entries: [
        {
          id: "e3",
          kind: "debit",
          label: "試跑",
          delta_credits: -7,
          estimated: true,
          created_at: "2026-09-03T00:00:00Z",
          run_id: RUN,
        },
        {
          id: "e2",
          kind: "debit",
          label: "互動創作",
          delta_credits: -4,
          estimated: false,
          created_at: "2026-09-02T00:00:00Z",
        },
        {
          id: "e1",
          kind: "grant",
          label: "營運者授予",
          delta_credits: 100,
          estimated: false,
          created_at: "2026-09-01T00:00:00Z",
        },
      ],
      note: STATEMENT_NOTE,
    }),
  );
  await render(<WorkspaceAccount />, () => text().includes("營運者授予"));

  expect(text()).toContain("目前餘額 120 點");
  const rows = Array.from(container.querySelectorAll("li")).map((li) => li.textContent ?? "");
  expect(rows[0]).toContain("-7 點");
  expect(rows[0]).toContain("估計");
  expect(rows[1]).not.toContain("估計");
  expect(rows[2]).toContain("+100 點");
  const links = Array.from(container.querySelectorAll("li a")).map((a) => a.getAttribute("href"));
  expect(links, "only the entry that names a run may link to one").toEqual([`/runs/${RUN}`]);
  expect(text()).toContain(STATEMENT_NOTE);
  expect(button("載入更早的紀錄")).toBeUndefined();
});

test("CRED-009 older entries load with the cursor the server returned, and the last page offers no more", async () => {
  const befores: (string | null)[] = [];
  stubAccountFetch(
    () => json(ME),
    (before) => {
      befores.push(before);
      return before === "cursor-1"
        ? {
            entries: [
              {
                id: "old",
                kind: "topup",
                label: "儲值",
                delta_credits: 50,
                estimated: false,
                created_at: "2026-08-01T00:00:00Z",
              },
            ],
            note: STATEMENT_NOTE,
          }
        : {
            entries: [
              {
                id: "new",
                kind: "grant",
                label: "營運者授予",
                delta_credits: 10,
                estimated: false,
                created_at: "2026-09-01T00:00:00Z",
              },
            ],
            next_before: "cursor-1",
            note: STATEMENT_NOTE,
          };
    },
  );
  await render(<WorkspaceAccount />, () => text().includes("營運者授予"));
  expect(text()).not.toContain("儲值");

  await act(async () => button("載入更早的紀錄")?.click());
  await waitFor(() => text().includes("儲值"));
  expect(befores).toContain("cursor-1");
  expect(text()).toContain("營運者授予");
  expect(button("載入更早的紀錄")).toBeUndefined();
});

test("CORE-007 requesting account deletion starts a grace period and shows the server's scope", async () => {
  const calls: [string, string | undefined][] = [];
  stubAccountFetch((input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deletion_requested_at: "2026-08-18T00:00:00Z",
        purge_after: "2026-09-17T00:00:00Z",
        cancellable: true,
        scope: DELETION_SCOPE,
      });
    }
    return json(ME);
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除我的帳號"));

  await act(async () => button("刪除我的帳號")?.click());
  expect(text()).toContain("不會立刻刪掉任何東西");
  expect(calls.some(([, m]) => m === "DELETE")).toBe(false);

  await act(async () => button("確認開始刪除")?.click());
  await waitFor(() => text().includes("寬限期結束前"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain("/me");
});

test("丙-150 a failed deletion request says the fixed sentence, not the server's raw body, in role=alert", async () => {
  stubAccountFetch((_input: string, init?: RequestInit) => {
    if (init?.method === "DELETE") {
      return json({ error: "internal error exploding pants" }, 500);
    }
    return json(ME);
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除我的帳號"));

  await act(async () => button("刪除我的帳號")?.click());
  await act(async () => button("確認開始刪除")?.click());
  await waitFor(() => text().includes("這個要求沒有記錄成功"));

  const alert = Array.from(container.querySelectorAll('[role="alert"]')).find((n) =>
    (n.textContent ?? "").includes("這個要求沒有記錄成功"),
  );
  expect(alert, "the failure sentence is not in role=alert").toBeTruthy();
  expect(text()).not.toContain("exploding pants");
  const status = container.querySelector('[role="status"]');
  expect(status?.textContent ?? "").not.toContain("這個要求沒有記錄成功");
});

test("丙-150 a 409 on cancel (deletion already irreversible) says so, not the server's raw body", async () => {
  stubAccountFetch((_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return json({ error: "刪除已經不可逆，無法再變更" }, 409);
    }
    return json({
      ...ME,
      deletion_requested_at: "2026-08-18T00:00:00Z",
      purge_after: "2026-09-17T00:00:00Z",
      deletion_scope: DELETION_SCOPE,
    });
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除申請中"));

  await act(async () => button("取消刪除申請")?.click());
  await waitFor(() => text().includes("刪除已經不可逆，無法再變更。"));
  expect(text()).not.toContain("already irreversible");
});

test("設計 §2.6 the workspace UUID is behind a disclosure, not flat beside the account name", async () => {
  stubAccountFetch(() => json(ME));
  await render(<WorkspaceAccount />, () => text().includes("刪除我的帳號"));

  const fold = Array.from(container.querySelectorAll("details")).find((d) =>
    (d.querySelector("summary")?.textContent ?? "").includes("工作區識別碼"),
  );
  expect(fold, "the workspace id is not behind a disclosure").toBeTruthy();
  expect(fold!.textContent).toContain("ws-1");
  const flat = Array.from(container.querySelectorAll("p"))
    .map((p) => p.textContent ?? "")
    .join("");
  expect(flat, "the workspace id is flat on the page again").not.toContain("ws-1");
  expect(text()).toContain("tester@example.com");
});

test("CORE-007 a pending deletion is a state with a date and a way out, not a receipt", async () => {
  const posts: string[] = [];
  stubAccountFetch((input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      posts.push(String(input));
      return json({ deletion_requested_at: null });
    }
    return json({
      ...ME,
      deletion_requested_at: "2026-08-18T00:00:00Z",
      purge_after: "2026-09-17T00:00:00Z",
      deletion_scope: DELETION_SCOPE,
    });
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除申請中"));

  expect(container.querySelector('time[datetime="2026-09-17T00:00:00Z"]')).not.toBeNull();
  expect(text()).toContain("再按一次刪除不會提早");
  expect(text()).toContain(DELETION_SCOPE);

  await act(async () => button("取消刪除申請")?.click());
  await waitFor(() => posts.length > 0);
  expect(posts[0]).toContain("/me/deletion/cancel");
});

const ARTIFACT_ROW = {
  artifact_id: ARTIFACT,
  skill_id: SKILL,
  skill_version_id: "22222222-2222-2222-2222-222222222222",
  target: "standard" as const,
  file_name: "csv-cleanup-v2.zip",
  size_bytes: 4096,
  content_hash: "sha256:bbbb",
  manifest_hash: "sha256:cccc",
  status: "available" as const,
  servable: true,
  serve_state: { value: "available", label: "可下載", note: "" },
  version_number: 2,
  latest_version_number: 5,
  version_state: {
    value: "superseded",
    label: "v2（這個 Skill 已經到 v5）",
    note: "這一份是 v2 的內容,而且不會改變——版本是不可變的。要拿 v5 的內容,回到該 Skill 對 v5 重新打包一次。",
  },
  expires_at: "2099-01-01T00:00:00Z",
  created_at: "2026-08-17T00:00:00Z",
  download_count: 2,
  includes_test_cases: false,
};

test("WS-004 a download row says which version it is and whether a newer one exists", async () => {
  vi.stubGlobal("fetch", () => json({ downloads: [ARTIFACT_ROW] }));
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("v2（這個 Skill 已經到 v5）");
  expect(text()).toContain("重新打包");
  expect(text()).toContain("可下載");
  expect(container.querySelector('a[href*="/content"]')).not.toBeNull();
});

test("WS-004 the download history answers 誰 and 何時 per download, not just a count", async () => {
  const urls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    urls.push(String(input));
    if (String(input).includes("/records")) {
      return json({
        records: [
          { downloaded_at: "2026-08-17T09:00:00Z", actor: "tester" },
          { downloaded_at: "2026-08-17T10:00:00Z", actor: "deleted user" },
        ],
      });
    }
    return json({ downloads: [ARTIFACT_ROW] });
  });
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(urls.some((u) => u.includes("/records"))).toBe(false);

  await open("誰下載過");
  await waitFor(() => text().includes("tester"));

  expect(urls.some((u) => u.includes(`/downloads/${ARTIFACT}/records`))).toBe(true);
  expect(container.querySelector('time[datetime="2026-08-17T09:00:00Z"]')).not.toBeNull();
  expect(text()).toContain("deleted user");
  expect(text()).toContain("與稽核事件是兩份不同的紀錄");
});

const RUN_ARTIFACT = {
  artifact_id: "aaaa1111-2222-3333-4444-555566667777",
  file_name: "cleaned.csv",
  content_type: "text/csv",
  size_bytes: 2048,
  content_hash: "sha256:dddd",
  created_at: "2026-08-17T00:03:00Z",
  purged: false,
};

function stubRunPage(artifacts: unknown[], onDelete?: (url: string) => void, truncated = false) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === "DELETE") {
      onDelete?.(url);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/artifacts")) return json({ artifacts, truncated });
    if (url.includes("/trace"))
      return json({
        run_id: RUN,
        status: "succeeded",
        complete: true,
        skills: [],
        resources_read: 0,
        tool_calls: {
          total: 0,
          succeeded: 0,
          failed: 0,
          total_duration_ms: 0,
          slowest_duration_ms: 0,
        },
        errors: [],
        steps: [],
      });
    return json({ error: "not found" }, 404);
  });
}

test("SEC-006 a run output can be deleted, and the scope says what survives it", async () => {
  const deletes: string[] = [];
  stubRunPage([RUN_ARTIFACT], (url) => deletes.push(url));
  await render(<RunTrace />, () => text().includes("cleaned.csv"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs.some((h) => h.includes("/artifacts/"))).toBe(false);
  expect(text()).toContain("控制平面不打開它");

  await act(async () => button("刪除")?.click());
  expect(text()).toContain("引用過這個檔案的評估不會被改寫");
  expect(deletes).toHaveLength(0);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => deletes.length > 0);
  expect(deletes[0]).toContain(`/runs/${RUN}/artifacts/${RUN_ARTIFACT.artifact_id}`);
});

test("SEC-006 a purged output keeps its row and says the bytes are gone", async () => {
  stubRunPage([{ ...RUN_ARTIFACT, purged: true, expires_at: "2026-08-16T00:00:00Z" }]);
  await render(<RunTrace />, () => text().includes("cleaned.csv"));

  expect(text()).toContain("檔案已不存在");
  expect(text()).toContain("曾經產生過這個檔案」仍然是事實");
});

test("RUN-002 says when the sandbox had to drop some output files", async () => {
  stubRunPage([], undefined, true);
  await render(<RunTrace />, () => text().includes("有些產出未被收集"));

  expect(text()).toContain("清單只保留成功收集的檔案");
  expect(text()).toContain("無法據此判定這次 Run 沒有產生檔案");
  expect(text()).not.toContain("這次 Run 沒有留下任何檔案產出");
});

test("WS-004 a package nobody downloaded says so instead of loading an empty list", async () => {
  const urls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    urls.push(String(input));
    return json({ downloads: [{ ...ARTIFACT_ROW, download_count: 0 }] });
  });
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  await open("誰下載過");

  expect(text()).toContain("還沒有人下載過");
  expect(text()).toContain("建立一個套件不等於取走它");
  expect(urls.some((u) => u.includes("/records"))).toBe(false);
});

test("WS-004 a fork invalidates the list it writes to, and does not touch the search key", async () => {
  vi.stubGlobal("fetch", () => json({ skill_id: "forked-1", version_id: "v1" }, 201));

  queryClient.setQueryData(["own-skills"], { skills: [] });
  queryClient.setQueryData(["skills", "search", "pdf"], { results: [] });

  let fork: ReturnType<typeof useForkSkill> | undefined;
  function ForkHarness() {
    fork = useForkSkill();
    return null;
  }
  await render(<ForkHarness />, () => fork !== undefined);
  await act(async () => {
    await fork!.mutateAsync(SKILL);
  });

  expect(queryClient.getQueryState(["own-skills"])?.isInvalidated).toBe(true);
  expect(queryClient.getQueryState(["skills", "search", "pdf"])?.isInvalidated).toBe(false);
});

test("SKILL-002 an import invalidates 我的 Skill, and does not re-run the search", async () => {
  vi.stubGlobal("fetch", () =>
    json(
      {
        shape: "skill",
        skills: [
          {
            path: "",
            skill_id: SKILL,
            version_id: "v1",
            version_number: 1,
            content_hash: "sha256:aaaa",
            duplicate: false,
            findings: { errors: [], warnings: [], infos: [] },
          },
        ],
        refused: [],
        excluded_components: [],
      },
      201,
    ),
  );
  queryClient.setQueryData(["own-skills"], { skills: [] });
  queryClient.setQueryData(["skills", "search", "pdf"], { results: [] });

  await render(<ImportSkill />, () => text().includes("匯入 Skill"));
  const input = container.querySelector<HTMLInputElement>('input[type="url"]')!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.bind(
      input,
    );
    setter("https://github.com/example/skill");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await waitFor(() => text().includes("匯入完成"));

  expect(
    queryClient.getQueryState(["own-skills"])?.isInvalidated,
    "an import that does not invalidate 我的 Skill leaves the list without the skill just added",
  ).toBe(true);
  expect(queryClient.getQueryState(["skills", "search", "pdf"])?.isInvalidated).toBe(false);
});

test("RUN-005 cancelling a run invalidates its trace and its row, not every trace", async () => {
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") return json({ note: CANCEL_NOTE }, 202);
    return json({ error: "not found" }, 404);
  });
  queryClient.setQueryData(["trace", RUN, "general", 0], { status: "running" });
  queryClient.setQueryData(["run", RUN], { run_id: RUN });
  queryClient.setQueryData(["trace", "other-run", "general", 0], { status: "running" });

  await render(
    <CancelRunControl runId={RUN} status="running" />,
    () => button("取消這個 Run") !== undefined,
  );
  await act(async () => button("取消這個 Run")?.click());
  await act(async () => button("確認取消")?.click());
  await waitFor(() => text().includes(CANCEL_NOTE));

  expect(queryClient.getQueryState(["trace", RUN, "general", 0])?.isInvalidated).toBe(true);
  expect(queryClient.getQueryState(["run", RUN])?.isInvalidated).toBe(true);
  expect(queryClient.getQueryState(["trace", "other-run", "general", 0])?.isInvalidated).toBe(
    false,
  );
});

test("04 丙-143(c): cancelling a run that needs login says so, not the raw server string", async () => {
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") return json({ error: "not authenticated" }, 401);
    return json({ error: "not found" }, 404);
  });
  queryClient.setQueryData(["trace", RUN, "general", 0], { status: "running" });
  queryClient.setQueryData(["run", RUN], { run_id: RUN });

  await render(
    <CancelRunControl runId={RUN} status="running" />,
    () => button("取消這個 Run") !== undefined,
  );
  await act(async () => button("取消這個 Run")?.click());
  await act(async () => button("確認取消")?.click());
  await waitFor(() => text().includes("需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("04 丙-143(c): cancelling a run that already ended says so (409)", async () => {
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") return json({ error: "already terminal" }, 409);
    return json({ error: "not found" }, 404);
  });
  queryClient.setQueryData(["trace", RUN, "general", 0], { status: "running" });
  queryClient.setQueryData(["run", RUN], { run_id: RUN });

  await render(
    <CancelRunControl runId={RUN} status="running" />,
    () => button("取消這個 Run") !== undefined,
  );
  await act(async () => button("取消這個 Run")?.click());
  await act(async () => button("確認取消")?.click());
  await waitFor(() => text().includes("已經結束"));
});

test("a run in a terminal status shows no cancel button and no confirm dialog", async () => {
  await render(<CancelRunControl runId={RUN} status="succeeded" />, () => true);

  expect(button("取消這個 Run")).toBeUndefined();
  expect(text()).not.toContain("確定要取消？");
});

test("CORE-007 cancelling a deletion request invalidates /me, so the badge goes away", async () => {
  const invalidated = vi.spyOn(queryClient, "invalidateQueries");
  stubAccountFetch((_input: string, init?: RequestInit) => {
    if (init?.method === "DELETE") return json({ cancelled: true });
    return json({
      user_id: "u-1",
      email: "t@example.com",
      display_name: "tester",
      workspace_id: "ws-1",
      deletion_requested_at: "2026-08-18T00:00:00Z",
      purge_after: "2026-09-17T00:00:00Z",
      deletion_scope: "purging destroys skills, versions, runs and downloads",
    });
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除申請中"));
  invalidated.mockClear();

  await act(async () => button("取消刪除申請")?.click());
  await waitFor(() => text().includes("已取消"));

  expect(
    invalidated.mock.calls.map(([arg]) => arg?.queryKey),
    "cancelling a deletion did not invalidate /me — the page goes on showing 刪除申請中 " +
      "with a purge date for an account that is no longer being deleted",
  ).toContainEqual(["me"]);
  invalidated.mockRestore();
});

function stubDetailAsSignedIn(versions: unknown) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
    if (path.endsWith("/versions")) return json(versions);
    if (path.startsWith("/api/skills/")) return json(skillDetail(SKILL, "PDF Summariser"));
    return json({ error: "not found" }, 404);
  });
}

const trialSectionAnswered = () =>
  text().includes("此 Skill 的 Test Case") || text().includes("這個 Skill 不在你的工作區");

test("丙-116 試跑 on a skill in your own workspace still links to its Test Cases", async () => {
  stubDetailAsSignedIn(SKILL_VERSIONS);
  await render(<SkillDetail />, trialSectionAnswered);

  expect(text()).toContain("此 Skill 的 Test Case");
  expect(text()).not.toContain("這個 Skill 不在你的工作區");
});

test("丙-116 試跑 on somebody else's skill says so BEFORE the corridor, not after it", async () => {
  stubDetailAsSignedIn({ versions: [] });
  await render(<SkillDetail />, trialSectionAnswered);

  // Pinned whole: Prettier wraps this JSX text and joins the wrapped lines
  // with one space, so where the break lands decides whether a full-width
  // comma grows an extra space after it.
  expect(text()).toContain(
    "這個 Skill 不在你的工作區。Test Case 屬於工作區，所以 Test Case 清單裡看不到它、建立表單的 Skill 選單也選不到它——要先 Fork 一份，才會有屬於你的版本可以試跑。下方的「Fork 到你的工作區」就是那一步。",
  );
  expect(text()).toContain("選單也選不到它");
  expect(text()).not.toContain("此 Skill 的 Test Case");
});

test("丙-116 the one action that does work is a named section, not a bare button", async () => {
  stubDetailAsSignedIn({ versions: [] });
  await render(<SkillDetail />, trialSectionAnswered);

  const headings = Array.from(container.querySelectorAll("h2,h3")).map((h) => h.textContent);
  expect(headings).toContain("Fork 到你的工作區");
  expect(button("以這個 Skill 為起點建立我自己的")).not.toBeUndefined();
});

test("§2.12 第 6 條 a run history with a run still going says how old it is and can be refreshed", async () => {
  const fetchSpy = vi.fn(() =>
    json({
      runs: [
        RUN_ROW,
        {
          ...RUN_ROW,
          run_id: "run-live",
          status: "running",
          finished_at: undefined,
          evaluation: { value: "not_evaluated", label: "未評估", note: "還在跑。" },
        },
      ],
    }),
  );
  vi.stubGlobal("fetch", fetchSpy);
  await render(<WorkspaceRuns />, () => text().includes("上次取得於"));

  const refresh = button("重新整理");
  expect(refresh, "no visible refresh control on a list with a running row").toBeTruthy();
  const freshness = refresh!.closest("p")!;
  expect(freshness.querySelector("time")).not.toBeNull();

  const before = fetchSpy.mock.calls.length;
  await act(async () => refresh!.click());
  await waitFor(() => fetchSpy.mock.calls.length > before);
});

test("§2.12 第 6 條 a run history with nothing running carries no refresh control", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [RUN_ROW] }));
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  expect(button("重新整理")).toBeUndefined();
  expect(text()).not.toContain("上次取得於");
});

function stubBundleRoutes(opts: {
  ownSkills?: Array<{ skill_id: string; embedded: unknown }>;
  bundles?: unknown[];
  onCreate?: (body: Record<string, unknown>) => { body: unknown; status?: number };
  publication?: (name: string) => { body: unknown; status?: number };
  onExport?: (name: string) => { body: unknown; status?: number };
  onPublish?: (name: string, body: Record<string, unknown>) => { body: unknown; status?: number };
  onDelist?: (name: string) => { body: unknown; status?: number };
}) {
  const calls: Array<{ path: string; method: string; body?: Record<string, unknown> }> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";
    const body = init?.body
      ? (JSON.parse(String(init.body)) as Record<string, unknown>)
      : undefined;
    calls.push({ path, method, body });

    if (path === "/skills" && method === "GET") {
      return json({
        skills: (opts.ownSkills ?? []).map((s) => ({
          skill_id: s.skill_id,
          name: "",
          summary: "",
          redistribution: "allowed",
          access_restriction: null,
        })),
        limit: 100,
        truncated: false,
        total: (opts.ownSkills ?? []).length,
      });
    }
    const embeddedMatch = path.match(/^\/api\/skills\/([^/]+)$/);
    if (embeddedMatch && method === "GET") {
      const found = (opts.ownSkills ?? []).find((s) => s.skill_id === embeddedMatch[1]);
      return found ? json(found.embedded) : json({ error: "not found" }, 404);
    }
    if (path === "/me/bundles" && method === "GET") return json({ bundles: opts.bundles ?? [] });
    if (path === "/me/bundles" && method === "POST") {
      const result = opts.onCreate?.(body ?? {}) ?? {
        body: { error: "not configured" },
        status: 500,
      };
      return json(result.body, result.status ?? 201);
    }
    const exportMatch = path.match(/^\/me\/bundles\/([^/]+)\/export$/);
    if (exportMatch && method === "POST") {
      const result = opts.onExport?.(exportMatch[1]) ?? {
        body: { error: "not configured" },
        status: 500,
      };
      return json(result.body, result.status ?? 201);
    }
    const pubMatch = path.match(/^\/me\/bundles\/([^/]+)\/publication$/);
    if (pubMatch && method === "GET") {
      const result = opts.publication?.(pubMatch[1]) ?? {
        body: { error: "not published" },
        status: 404,
      };
      return json(result.body, result.status ?? 200);
    }
    if (pubMatch && method === "POST") {
      const result = opts.onPublish?.(pubMatch[1], body ?? {}) ?? {
        body: { error: "not configured" },
        status: 500,
      };
      return json(result.body, result.status ?? 200);
    }
    if (pubMatch && method === "DELETE") {
      const result = opts.onDelist?.(pubMatch[1]) ?? {
        body: { error: "not configured" },
        status: 500,
      };
      return json(result.body, result.status ?? 200);
    }
    return json({ error: "not found" }, 404);
  });
  return calls;
}

function bundleRow(bundle: string) {
  return {
    bundle,
    version: "1.0.0",
    description: "一組 PDF 工具",
    content_hash: "sha256:bundle-1",
    created_at: "2026-09-01T00:00:00Z",
    members: [
      {
        skill_id: SKILL,
        version_id: VERSION,
        name: "summariser",
        version_number: 2,
        content_hash: "sha256:aa",
      },
    ],
  };
}

test("PACK-018 Bundle 區塊列出既有 Bundle 的名稱、版本、說明與成員", async () => {
  stubBundleRoutes({
    bundles: [bundleRow("pdf-toolkit-list")],
    publication: () => ({ body: { error: "not published" }, status: 404 }),
  });
  await render(<BundleSection />, () => text().includes("pdf-toolkit-list"));

  expect(text()).toContain("v1.0.0");
  expect(text()).toContain("一組 PDF 工具");
  expect(text()).toContain("summariser v2");
});

test("PACK-018 匯出為 Plugin 成功後顯示下載連結，且揭露句在按鈕旁", async () => {
  stubBundleRoutes({
    bundles: [bundleRow("pdf-toolkit-export")],
    publication: () => ({ body: { error: "not published" }, status: 404 }),
    onExport: () => ({
      body: {
        artifact_id: "art-2",
        file_name: "pdf-toolkit-1.0.0-plugin.zip",
        size_bytes: 10,
        content_hash: "sha256:x",
        expires_at: "2099-01-01T00:00:00Z",
        duplicate: false,
        content_url: "/downloads/art-2/content",
      },
    }),
  });
  await render(<BundleSection />, () => text().includes("pdf-toolkit-export"));

  expect(text()).toContain("Plugin 只含 Agent Skill，不含 MCP 設定或宿主專屬元件。");

  await act(async () => button("匯出為 Plugin")?.click());
  await waitFor(() => text().includes("pdf-toolkit-1.0.0-plugin.zip"));

  const link = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.textContent ?? "").includes("pdf-toolkit-1.0.0-plugin.zip"),
  );
  expect(link?.getAttribute("href")).toBe("/downloads/art-2/content");
});

test("PACK-018 尚未發佈時可以送出發佈，422 顯示伺服器的字串", async () => {
  stubBundleRoutes({
    bundles: [bundleRow("pdf-toolkit-422")],
    publication: () => ({ body: { error: "not published" }, status: 404 }),
    onPublish: () => ({
      body: { error: "Bundle 需要一段說明", reason: "description_missing" },
      status: 422,
    }),
  });
  await render(<BundleSection />, () => text().includes("pdf-toolkit-422"));
  await waitFor(() => Boolean(button("發佈")));

  await act(async () => button("發佈")?.click());
  await waitFor(() => text().includes("Bundle 需要一段說明"));
});

test("PACK-018 已發佈時顯示公開位址、狀態與撤回按鈕", async () => {
  stubBundleRoutes({
    bundles: [bundleRow("pdf-toolkit-published")],
    publication: () => ({
      body: {
        kind: "bundle",
        publisher: "acme-tools",
        name: "pdf-toolkit-published",
        address: "/p/acme-tools/pdf-toolkit-published",
        status: "published",
        status_changed_at: "2026-09-01T00:00:00Z",
        releases: [
          {
            bundle_version: "1.0.0",
            content_hash: "sha256:bundle-1",
            released_at: "2026-09-01T00:00:00Z",
            rights_attested: false,
            findings: { errors: [], warnings: [], infos: [] },
          },
        ],
      },
    }),
  });
  await render(<BundleSection />, () => text().includes("pdf-toolkit-published"));

  await waitFor(() => text().includes("/p/acme-tools/pdf-toolkit-published"));
  expect(text()).toContain("已發佈");
  expect(button("撤回")).toBeDefined();
});

test("PACK-018 建立 Bundle Version 時，成員送出的是各自最新版本的 version_id", async () => {
  const calls = stubBundleRoutes({
    bundles: [],
    ownSkills: [{ skill_id: SKILL, embedded: skillDetail(SKILL, "Summariser") }],
    onCreate: (body) => ({
      body: {
        bundle: body.name,
        version: body.version,
        description: body.description,
        content_hash: "sha256:new",
        created_at: "2026-09-11T00:00:00Z",
        members: [],
      },
    }),
  });
  await render(<BundleSection />, () => text().includes("Summariser"));

  const form = Array.from(container.querySelectorAll("form")).find((f) =>
    (f.textContent ?? "").includes("建立 Bundle Version"),
  )!;
  const [nameInput, versionInput] = Array.from(
    form.querySelectorAll('input:not([type="checkbox"])'),
  ) as HTMLInputElement[];
  const textarea = form.querySelector("textarea")!;
  const setValue = (el: HTMLInputElement | HTMLTextAreaElement, value: string) => {
    const proto =
      el instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };
  await act(async () => {
    setValue(nameInput, "pdf-toolkit");
    setValue(versionInput, "1.0.0");
    setValue(textarea, "一組 PDF 工具");
  });
  const checkbox = form.querySelector('input[type="checkbox"]') as HTMLInputElement;
  await act(async () => checkbox.click());
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await waitFor(() => calls.some((c) => c.path === "/me/bundles" && c.method === "POST"));

  const created = calls.find((c) => c.path === "/me/bundles" && c.method === "POST")!;
  expect(created.body?.member_version_ids).toEqual([VERSION]);
  expect(created.body?.name).toBe("pdf-toolkit");
});
