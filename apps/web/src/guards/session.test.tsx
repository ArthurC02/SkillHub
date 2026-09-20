import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../core/api/queryClient";
import { ApiError } from "../core/api/client";
import { LoginRequired, ReadFailure, unauthenticated } from "../shared/ui/LoginRequired";
import { FeedbackEntry } from "../app/shell/FeedbackEntry";
import { ImportSkill } from "../features/creation/import/ImportSkill.page";
import { RunCompare } from "../features/runs/compare/RunCompare.page";
import { RunPreflight } from "../features/lab/preflight/RunPreflight.page";
import { SkillDetail } from "../features/skill/detail/SkillDetail.page";
import { TestCaseList } from "../features/lab/test-cases/TestCaseList.page";
import { WorkspaceAccount } from "../features/workspace/account/WorkspaceAccount.page";
import { WorkspaceSkills } from "../features/workspace/skills/WorkspaceSkills.page";
import { useSkillVersions } from "../features/skill/skills.service";
import { useTrace } from "../features/runs/trace.service";
import { RUN, SKILL, TEST_CASE, platformResponse } from "../testing/fixtures/platform";

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
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
  useParams: () => ({ skillId: SKILL, runId: RUN, testCaseId: TEST_CASE }),
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function loggedOutPlatform() {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path.startsWith("/api/")) {
      const { body, status } = platformResponse(String(input));
      return json(body, status);
    }
    return json({ error: "not authenticated" }, 401);
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

// Pumps an act() cycle before each check: without IS_REACT_ACT_ENVIRONMENT,
// a state update delivered during the initial act is not flushed to the DOM
// until another act runs.
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

const text = () => container.textContent ?? "";

const settled = (also: () => boolean = () => true) =>
  queryClient.isFetching() === 0 &&
  !container.querySelector("[data-loading]") &&
  text().length > 0 &&
  also();

function assertHonestArrival() {
  expect(text(), "the server's English 401 body reached a 繁體中文 screen").not.toContain(
    "not authenticated",
  );
  const login = Array.from(container.querySelectorAll("a")).filter((a) =>
    (a.getAttribute("href") ?? "").endsWith("/auth/github/login"),
  );
  expect(login.length, "nothing on screen says how to log in").toBeGreaterThan(0);
}

test("IA-6 `unauthenticated` is true for a 401 and for nothing else", () => {
  expect(unauthenticated(new ApiError(401, "not authenticated"))).toBe(true);
  expect(unauthenticated(new ApiError(403, "forbidden"))).toBe(false);
  expect(unauthenticated(new ApiError(500, "boom"))).toBe(false);
  expect(unauthenticated(new Error("network"))).toBe(false);
  expect(unauthenticated(null)).toBe(false);
});

test("IA-6 the 401 state says it in 繁體中文 and carries the login action", async () => {
  await render(<LoginRequired what="你的 Skill 清單" />, () => text().includes("需要登入"));
  expect(text()).toContain("你的 Skill 清單需要登入。");
  assertHonestArrival();
  expect(container.querySelector("[role=status]")).not.toBeNull();
  expect(container.querySelector("[role=alert]")).toBeNull();
});

test("IA-6 ReadFailure does NOT swallow a non-401 — a 500 still says what failed", async () => {
  await render(
    <ReadFailure error={new ApiError(500, "資料庫連線中斷")} what="下載紀錄" />,
    () => text().length > 0,
  );
  expect(text()).toContain("無法讀取下載紀錄：資料庫連線中斷");
  expect(container.querySelector("[role=alert]")).not.toBeNull();
  expect(text()).not.toContain("需要登入");
});

test("IA-6 ReadFailure renders nothing when there is no error", async () => {
  await render(<ReadFailure error={null} what="下載紀錄" />, () => true);
  expect(text()).toBe("");
});

test("IA-6 a page's own non-401 wording survives; the 401 branch replaces it", async () => {
  const own = <p role="alert">無法比較：伺服器忙碌</p>;
  await render(
    <ReadFailure error={new ApiError(503, "伺服器忙碌")} what="比較結果">
      {own}
    </ReadFailure>,
    () => text().length > 0,
  );
  expect(text()).toBe("無法比較：伺服器忙碌");

  await act(async () => root.unmount());
  container.innerHTML = "";
  await render(
    <ReadFailure error={new ApiError(401, "not authenticated")} what="比較結果">
      {own}
    </ReadFailure>,
    () => text().includes("需要登入"),
  );
  expect(text()).not.toContain("無法比較");
  assertHonestArrival();
});

test("IA-6 /workspace/skills — a nav destination stops printing the server's English", async () => {
  loggedOutPlatform();
  await render(<WorkspaceSkills />, () => settled());
  expect(text()).toContain("你的 Skill 清單需要登入。");
  assertHonestArrival();
});

test("IA-6 /lab/test-cases — both reads on the page answer, not just the first", async () => {
  loggedOutPlatform();
  await render(<TestCaseList />, () => settled());
  expect(text()).toContain(" Test Case需要登入。");
  expect(text()).toContain("你的 Skill 清單需要登入。");
  assertHonestArrival();
});

test("IA-6 /workspace/import says it BEFORE the file picker, not after (設計 §2.2/§2.4)", async () => {
  loggedOutPlatform();
  await render(<ImportSkill />, () =>
    settled(() => container.querySelector("form") !== null || text().includes("需要登入")),
  );
  expect(text()).toContain("匯入 Skill需要登入。");
  expect(container.querySelector("form")).toBeNull();
  expect(container.querySelector("input[type=file]")).toBeNull();
  assertHonestArrival();
});

test("IA-6 /runs/$runId/compare says it before an id is typed in", async () => {
  loggedOutPlatform();
  await render(<RunCompare />, () =>
    settled(() => container.querySelector("#against") !== null || text().includes("需要登入")),
  );
  expect(text()).toContain("Run 比較需要登入。");
  expect(container.querySelector("#against")).toBeNull();
  expect(text()).not.toContain("輸入另一個 Run 的 ID 後開始比較");
  assertHonestArrival();
});

test("IA-6 /lab/run stops sending a logged-out visitor to hunt for query parameters", async () => {
  loggedOutPlatform();
  await render(<RunPreflight />, () =>
    settled(() => text().includes("兩個 ID") || text().includes("需要登入")),
  );
  expect(text()).toContain("試跑與執行前權限確認需要登入。");
  expect(text()).not.toContain("?skill=");
  expect(text()).not.toContain("兩個 ID");
  assertHonestArrival();
});

test("IA-6 the site-wide feedback form says it before a paragraph is written", async () => {
  loggedOutPlatform();
  await render(<FeedbackEntry pathname="/policy" />, () =>
    settled(() => container.querySelector("form") !== null || text().includes("需要登入")),
  );
  expect(text()).toContain("回報問題需要登入。");
  expect(container.querySelector("form")).toBeNull();
  expect(container.querySelector("#feedback-message")).toBeNull();
  assertHonestArrival();
});

test("SEC/§2.2 the import screen states the rules it is enforced by, before the form", async () => {
  // Values no constant in this app holds, none a substring of another.
  vi.stubGlobal("fetch", (input: string) =>
    String(input).includes("/skills/import/limits")
      ? json({
          max_zip_bytes: 6 << 20,
          max_unpacked_bytes: 129 << 20,
          max_files: 813,
          max_file_bytes: 4 << 20,
          max_path_depth: 5,
          allowed_hosts: ["example.test", "sources.example.test"],
          note: "",
        })
      : json({ user_id: "u-1", workspace_id: "ws-1" }),
  );
  await render(<ImportSkill />, () => text().includes("zip 最大"));

  expect(text()).toContain("必須是 https");
  expect(text()).toContain("不得帶帳號密碼");

  for (const [what, sentence] of [
    ["the archive ceiling", "zip 最大 6 MB"],
    ["the unpacked ceiling a small archive can breach", "解壓後總量最大 129 MB"],
    ["the file count", "最多 813 個檔案"],
    ["the per-file ceiling", "單一檔案最大 4 MB"],
    ["the path depth", "路徑最深 5 層"],
  ]) {
    expect(text(), `${what} is not on screen, so it is learned by being refused`).toContain(
      sentence,
    );
  }

  expect(
    text(),
    "the hosts come from this deployment's fetcher, not from a list in the page",
  ).toContain("來源限 example.test、sources.example.test");

  expect(text(), "the page must not still say the numbers are unreadable").not.toContain(
    "大小上限見拒絕訊息",
  );
});

test("IA-6 precedent: ForkAction tells a visitor what logging in buys (SkillDetail)", async () => {
  loggedOutPlatform();
  await render(<SkillDetail />, () => text().includes("Fork"));
  expect(text()).toContain("登入後即可 Fork 這個 Skill 到你的工作區。");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) =>
      (b.textContent ?? "").includes("以這個 Skill 為起點"),
    ),
  ).toBe(false);
  expect(text()).not.toContain("not authenticated");
});

test("IA-6 precedent: /workspace/account says a login is needed, and how", async () => {
  loggedOutPlatform();
  await render(<WorkspaceAccount />, () => settled());
  expect(text()).toContain("帳號資料需要登入。");
  assertHonestArrival();
});

function RetryProbe() {
  const versions = useSkillVersions(SKILL);
  const trace = useTrace(RUN, "general");
  const state = (q: { isPending: boolean; isError: boolean }) =>
    q.isPending ? "pending" : q.isError ? "error" : "data";
  return (
    <p>
      versions:{state(versions)} trace:{state(trace)}
    </p>
  );
}

test("IA-6 a 401 is the answer at once — no 「載入中」 sat on through three retries", async () => {
  let calls = 0;
  vi.stubGlobal("fetch", (input: string) => {
    if (/\/versions$|\/trace/.test(String(input))) calls++;
    return json({ error: "not authenticated" }, 401);
  });

  await render(<RetryProbe />, () => calls >= 2);
  for (let i = 0; i < 20; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }

  expect(text()).toBe("versions:error trace:error");
  expect(calls).toBe(2);
});
