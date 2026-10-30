import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { SkillDetail } from "./detail/SkillDetail.page";
import { SkillFiles } from "./files/SkillFiles.page";
import { ForkAction } from "./detail/components/ForkAction";
import { ME, SKILL_VERSIONS, skillDetail } from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

const SKILL = "11111111-1111-1111-1111-111111111111";

let container: HTMLDivElement;
let root: Root;
const originalScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollIntoView");

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  if (originalScroll)
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", originalScroll);
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
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
    search?: Record<string, string>;
    className?: string;
    children?: unknown;
  }) => (
    <a
      className={className}
      href={
        Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) +
        (search ? `?${new URLSearchParams(search).toString()}` : "")
      }
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

function stubOwner(forkResponse: () => Promise<Response>, me: typeof ME = ME) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json(me);
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

const FORK_UNINVITED_REASON = "平台目前只讓有封測邀請的帳號複製小工具。";

test("丙-153: an invited account sees no invite rule beside the fork button", async () => {
  stubOwner(() => json({ error: "not authenticated" }, 401));
  await render(<SkillDetail />, settledAsOwner);

  expect(text()).not.toContain(FORK_UNINVITED_REASON);
  expect(forkButton()?.disabled).toBe(false);
});

test("丙-153: an uninvited account finds the fork button disabled, the enforcer naming the reason", async () => {
  let posted = false;
  stubOwner(
    () => {
      posted = true;
      return json({ error: "closed beta" }, 403);
    },
    { ...ME, invited: false },
  );
  await render(<SkillDetail />, () => settledAsOwner() && forkButton()?.disabled === true);

  const fork = forkButton()!;
  const reason = container.querySelector(`#${fork.getAttribute("aria-describedby")}`);
  expect(reason?.textContent).toBe(FORK_UNINVITED_REASON);
  await clickFork();
  expect(posted).toBe(false);
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

test("丙-150: 複製一份 409（同名）說出同名並指出資產庫裡的出路，不印 ErrNameTaken 的英文句", async () => {
  stubOwner(() => json({ error: "a skill with this name already exists in your workspace" }, 409));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("你的工作區已經有同名的小工具"));

  expect(text()).toContain(
    "你的工作區已經有同名的小工具，通常就是你先前複製的那一份，可以直接到資產庫使用它；要重新複製，先在資產庫刪掉那一份。",
  );
  expect(text()).not.toContain("already exists in your workspace");
  const recovery = Array.from(container.querySelectorAll("a")).find(
    (link) => link.textContent === "前往資產庫找出同名小工具",
  );
  expect(recovery?.getAttribute("href")).toBe("/library");
});

test("複製一份收到伺服器錯誤時保留結果的不確定性", async () => {
  stubOwner(() => json({ error: "internal error" }, 500));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => container.querySelector('[role="alert"]') !== null);

  expect(text()).toContain("尚未確認是否已複製到你的工作區。");
});

test("a successful 複製一份 hands off to the exact created version and cannot be submitted twice", async () => {
  stubOwner(() =>
    json(
      {
        ...detailBody(),
        summary: "",
        redistribution: "allowed",
        skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
        name: "PDF Summariser 副本",
        version_id: "abcdef02-2345-4789-abcd-0123456789ab",
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
  expect(next?.getAttribute("href")).toBe(
    "/skills/abcdef01-2345-4789-abcd-0123456789ab/versions/abcdef02-2345-4789-abcd-0123456789ab",
  );
  const completed = Array.from(container.querySelectorAll("button")).find(
    (item) => item.textContent === "已建立自己的版本",
  );
  expect(completed?.disabled).toBe(true);
  const packaging = Array.from(container.querySelectorAll("a")).find((link) =>
    link.textContent?.includes("直接打包並下載 v1"),
  );
  expect(packaging?.getAttribute("href")).toBe(
    "/skills/abcdef01-2345-4789-abcd-0123456789ab/package?version=abcdef02-2345-4789-abcd-0123456789ab",
  );
});

test("a catalog skill already copied into the workspace points at that copy instead of asking for another", async () => {
  const copy = {
    ...detailBody(),
    skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
    name: "PDF Summariser-fork",
    forked_from_skill_id: SKILL,
  };
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me") return json(ME);
    if (path === "/skills") return json({ skills: [copy], limit: 100, truncated: false, total: 1 });
    if (path.endsWith("/versions")) return json({ ...SKILL_VERSIONS, versions: [] });
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
  await render(<SkillDetail />, () => text().includes("已經有這個小工具的複本"));

  const links = Array.from(container.querySelectorAll("a")).filter(
    (link) => link.textContent === "PDF Summariser-fork",
  );
  expect(links.map((link) => link.getAttribute("href"))).toEqual([
    "/skills/abcdef01-2345-4789-abcd-0123456789ab",
    "/skills/abcdef01-2345-4789-abcd-0123456789ab",
    "/skills/abcdef01-2345-4789-abcd-0123456789ab",
  ]);
  expect(text()).toContain("到它的頁面打包並下載");
  expect(text()).toContain("到它的頁面寫測試題並試跑");
  expect(text()).not.toContain("要先複製一份");
});

test("the copy-first notes name the fork section by the heading it actually carries", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me") return json(ME);
    if (path.endsWith("/versions")) return json({ ...SKILL_VERSIONS, versions: [] });
    if (path.startsWith("/api/skills/")) return json(detailBody());
    return json({ error: "not found" }, 404);
  });
  await render(<SkillDetail />, () => text().includes("要先複製一份"));

  const named = [...text().matchAll(/「([^」]+)」就是那一步/g)].map((match) => match[1]);
  const headings = Array.from(container.querySelectorAll("h2,h3")).map((h) => h.textContent);
  expect(named.length).toBeGreaterThanOrEqual(2);
  for (const label of named) expect(headings).toContain(label);
});

for (const change of ["another skill", "signed-out return"]) {
  test(`a ${change} clears the previous private fork receipt in the same mounted view`, async () => {
    stubOwner(() =>
      json(
        {
          skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
          version_id: "abcdef02-2345-4789-abcd-0123456789ab",
          version_number: 1,
          name: "PDF Summariser-fork",
          summary: "",
          redistribution: "allowed",
        },
        201,
      ),
    );
    await render(
      <ForkAction skillId={SKILL} isLoggedIn available />,
      () => forkButton() !== undefined,
    );
    await clickFork();
    await waitFor(() => container.querySelector('[role="status"]') !== null);
    expect(text()).toContain("已複製到你的工作區");
    const replace = async (skillId: string, isLoggedIn: boolean) =>
      act(async () =>
        root.render(
          <StrictMode>
            <QueryClientProvider client={queryClient}>
              <ForkAction skillId={skillId} isLoggedIn={isLoggedIn} available />
            </QueryClientProvider>
          </StrictMode>,
        ),
      );
    if (change === "signed-out return") {
      await replace(SKILL, false);
      expect(text()).toContain("登入後即可把這個小工具複製到你的工作區");
      expect(text()).not.toContain("已複製到你的工作區");
    }
    await replace(
      change === "another skill" ? "33333333-3333-3333-3333-333333333333" : SKILL,
      true,
    );
    expect(forkButton()?.disabled).toBe(false);
    expect(text()).not.toContain("已複製到你的工作區");
    expect(container.querySelector('a[href*="abcdef01-2345-4789-abcd-0123456789ab"]')).toBeNull();
  });
}

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

test("a fork of a skill that is gone says so instead of asking to recheck", async () => {
  stubOwner(() => json({ error: "not found" }, 404));
  await render(<SkillDetail />, settledAsOwner);
  await clickFork();
  await waitFor(() => text().includes("這個小工具已經不在了"));

  expect(text()).toContain("這個小工具已經不在了（可能被下架或刪除），所以無法複製。");
  expect(text()).not.toContain("請重新確認小工具資訊後再試");
});
