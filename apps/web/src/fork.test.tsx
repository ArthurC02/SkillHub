import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { SkillDetail } from "./pages/SkillDetail";
import { SkillFiles } from "./pages/SkillFiles";
import { SKILL_VERSIONS, skillDetail } from "./fixtures/platform";

// 04 丙-150/151/153: SkillDetail's ForkAction stopped printing `err.message`
// verbatim, and SkillFiles gained its own 404/503 sentences. Same hand-rolled
// DOM plumbing as detail.test.tsx / workspace.test.tsx; @testing-library is
// not a dependency of this app.

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

/** 擁有者：`/skills/{id}/versions` 是 workspace-scoped，非空＝這一份是你的（ADR-011）。 */
function stubOwner(forkResponse: () => Promise<Response>) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json({ user_id: "u-1", workspace_id: "ws-1" });
    if (path.endsWith("/fork") && init?.method === "POST") return forkResponse();
    if (path.endsWith("/versions")) return json(SKILL_VERSIONS);
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
}

const settledAsOwner = () => text().includes("此 Skill 的 Test Case");

function forkButton(): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("以這個 Skill 為起點建立我自己的"),
  );
}

async function clickFork() {
  const button = forkButton();
  expect(button, "找不到 Fork 按鈕").toBeDefined();
  await act(async () => {
    button!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

// --- 04 丙-153: CreateHub.tsx:76-82 的邀請句在 Fork 按鈕之前 --------------------

test("丙-153: Fork 按鈕之前有封測邀請的說明句（照 CreateHub 的句型）", async () => {
  stubOwner(() => json({ error: "not authenticated" }, 401));
  await render(<SkillDetail />, settledAsOwner);

  expect(text()).toContain(
    "Fork 需要封測邀請，這道限制由平台強制；還沒有邀請的話，那一步會被擋下來。",
  );
});

// --- 04 丙-150/153: 按 status 選中文句，從不印 err.message ----------------------

test("丙-153: Fork 403（沒有封測邀請）印中文句，不印 betaNotInvited 的英文段落", async () => {
  stubOwner(() =>
    json(
      {
        error:
          "Skill Hub is in closed beta: browsing and skill details are open to everyone, but forking, running and downloading are limited to invited participants.",
      },
      403,
    ),
  );
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("Fork 沒有成功"));

  expect(text()).toContain(
    "這個帳號還沒有封測邀請，所以 Fork 沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。",
  );
  expect(text()).not.toContain("closed beta");
});

test("丙-150: Fork 409（同名）印「你的工作區已經有同名的 Skill。」，不印 ErrNameTaken 的英文句", async () => {
  stubOwner(() => json({ error: "a skill with this name already exists in your workspace" }, 409));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("你的工作區已經有同名的 Skill"));

  expect(text()).toContain("你的工作區已經有同名的 Skill。");
  expect(text()).not.toContain("already exists in your workspace");
});

test("丙-150: Fork 500 等其他狀態印通用的重試句", async () => {
  stubOwner(() => json({ error: "internal error" }, 500));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("Fork 沒有成功"));

  expect(text()).toContain("Fork 沒有成功，可以再按一次。");
});

// --- 04 丙-149: SkillFiles 的 503 走這一頁自己的句子，不落進通用分支 -------------

test("丙-149/150: SkillFiles 讀取 503 印「儲存的套件目前讀不到，稍後再試一次。」", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.startsWith("/api/skills/") && url.endsWith("/files"))
      return json({ error: "stored package is not readable" }, 503);
    return json({ error: "not found" }, 404);
  });
  await render(<SkillFiles />, () => text().includes("儲存的套件目前讀不到"));

  expect(text()).toContain("儲存的套件目前讀不到，稍後再試一次。");
  expect(text()).not.toContain("stored package is not readable");
});
