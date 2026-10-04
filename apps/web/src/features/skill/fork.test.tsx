import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { SkillDetail } from "./detail/SkillDetail.page";
import { SkillFiles } from "./files/SkillFiles.page";
import { SKILL_VERSIONS, skillDetail } from "../../testing/fixtures/platform";
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

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";

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

const settledAsOwner = () => text().includes("此小工具的測試題");

function forkButton(): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("以這個小工具為起點建立我自己的"),
  );
}

async function clickFork() {
  const button = forkButton();
  expect(button, "找不到複製一份按鈕").toBeDefined();
  await act(async () => {
    button!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

test("丙-153: 複製一份按鈕之前有封測邀請的說明句（照 CreateHub 的句型）", async () => {
  stubOwner(() => json({ error: "not authenticated" }, 401));
  await render(<SkillDetail />, settledAsOwner);

  expect(text()).toContain("平台目前只讓有封測邀請的帳號複製小工具。");
});

test("丙-153: 複製一份 403（沒有封測邀請）印中文句，不印 betaNotInvited 的英文段落", async () => {
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
  await waitFor(() => text().includes("複製沒有成功"));

  expect(text()).toContain(
    "這個帳號還沒有封測邀請，所以複製沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。",
  );
  expect(text()).not.toContain("closed beta");
});

test("丙-150: 複製一份 409（同名）印「你的工作區已經有同名的小工具。」，不印 ErrNameTaken 的英文句", async () => {
  stubOwner(() => json({ error: "a skill with this name already exists in your workspace" }, 409));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("你的工作區已經有同名的小工具"));

  expect(text()).toContain("你的工作區已經有同名的小工具。");
  expect(text()).not.toContain("already exists in your workspace");
  const recovery = Array.from(container.querySelectorAll("a")).find(
    (link) => link.textContent === "前往資產庫找出同名小工具",
  );
  expect(recovery?.getAttribute("href")).toBe("/library");
});

test("丙-150: 複製一份 500 等其他狀態印通用的重試句", async () => {
  stubOwner(() => json({ error: "internal error" }, 500));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("複製沒有成功"));

  expect(text()).toContain("複製沒有成功，可以再按一次。");
});

test("a successful 複製一份 hands off to the exact created version and cannot be submitted twice", async () => {
  stubOwner(() =>
    json(
      {
        ...detailBody(),
        skill_id: "fork-1",
        name: "PDF Summariser 副本",
        version_id: "fork-version-1",
        version_number: 1,
      },
      201,
    ),
  );
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("已複製到你的工作區"));

  const next = Array.from(container.querySelectorAll("a")).find((link) =>
    link.textContent?.includes("開啟 PDF Summariser 副本 v1"),
  );
  expect(next?.getAttribute("href")).toBe("/skills/fork-1/versions/fork-version-1");
  const completed = Array.from(container.querySelectorAll("button")).find(
    (item) => item.textContent === "已建立自己的版本",
  );
  expect(completed?.disabled).toBe(true);
});

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

test("小工具Files 403 說明尚未開放，不顯示伺服器原文", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.startsWith("/api/skills/") && url.endsWith("/files")) {
      return json({ error: "closed beta access denied" }, 403);
    }
    return json({ error: "not found" }, 404);
  });
  await render(<SkillFiles />, () => text().includes("尚未開放"));

  expect(text()).toContain("這份套件的檔案目前尚未開放查看");
  expect(text()).not.toContain("closed beta access denied");
});
