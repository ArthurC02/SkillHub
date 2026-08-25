import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { ApiError } from "./api/client";
import { LoginRequired, ReadFailure, unauthenticated } from "./components/LoginRequired";
import { FeedbackEntry } from "./components/FeedbackEntry";
import { ImportSkill } from "./pages/ImportSkill";
import { RunCompare } from "./pages/RunCompare";
import { RunPreflight } from "./pages/RunPreflight";
import { SkillDetail } from "./pages/SkillDetail";
import { TestCaseList } from "./pages/TestCases";
import { WorkspaceAccount } from "./pages/WorkspaceAccount";
import { WorkspaceSkills } from "./pages/WorkspaceSkills";
import { useSkillVersions } from "./api/skills";
import { useTrace } from "./api/trace";
import { RUN, SKILL, TEST_CASE, platformResponse } from "./fixtures/platform";

/**
 * 資訊架構 §5 IA-6（2026-08-25 裁定）：**登出狀態不由 router 守衛，由 401 這個具名
 * 狀態自己說。**
 *
 * 這個檔守的是那條裁定，而它存在的直接理由寫在裁定的「刻意留在外面的」那一段：
 * **裁定所依據的兩句既有前例（`SkillDetail` 的 `ForkAction`、`WorkspaceAccount`
 * 的登出說明）此前一支測試都沒有**。一條沒有測試擋著的慣例，就是下一次被無聲刪掉
 * 的那一條。
 *
 * 三層，由小到大：
 *
 *  1. 共用元件本身——包含「非 401 不得被吞掉」，那是這個抽象最容易壞掉的地方。
 *  2. 逐路由的登出抵達，其中兩頁是「事前說」的那兩頁（`/workspace/import` 與
 *     `/runs/$runId/compare`），一頁是被修掉的誤導（`/lab/run`）。
 *  3. `not authenticated` 這個英文字串**不得抵達畫面**——`assertHonestArrival` 在
 *     每一個路由案例的結尾跑。這一條才是「有人把某個呼叫點改回去」時會變紅的那條。
 */

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
  // Empty on purpose: `/lab/run` with no `?skill=&test_case=` and
  // `/runs/$runId/compare` with an empty `against` are exactly the two states
  // the ruling calls out, and an empty search object is how a visitor arrives.
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

/**
 * The platform as a logged-out visitor meets it: `RequireSession` answers
 * `401 {"error":"not authenticated"}` — the literal body from
 * `creator/workspace/http.go` — and the three public `/api/*` reads still work,
 * because DISC-001/010 serve the catalogue to anyone.
 */
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

/**
 * Pumps FIRST, then checks — the opposite order to the other suites here, and
 * deliberately.
 *
 * This app's tests never set `IS_REACT_ACT_ENVIRONMENT`, so a state update that
 * React Query delivered during the initial `act` is not flushed to the DOM until
 * another `act` runs. The other suites hide that because they wait on the very
 * text they are about to assert, so the loop keeps pumping until it appears.
 * `settled` below waits on something true of the fixed AND the broken page, so
 * it can be satisfied on the first check with a stale DOM — checking after the
 * pump is what keeps that from reading a page one render behind.
 */
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

/**
 * Settled, WITHOUT asking whether the fix is in place.
 *
 * The first version of this file waited for 「需要登入」 — so reverting a call
 * site produced a `waitFor` timeout, which is a red test but not the red test:
 * it says 「the page never said the thing」 rather than 「the page said the wrong
 * thing」, and it would have gone on timing out for any reason at all. Waiting on
 * a condition true in BOTH the fixed and the broken page is what puts the
 * failure on the `expect` below (AGENTS.md 規則 9).
 */
const settled = (also: () => boolean = () => true) =>
  queryClient.isFetching() === 0 &&
  !container.querySelector("[data-loading]") &&
  text().length > 0 &&
  also();

/**
 * The assertion that goes red when somebody reverts a call site.
 *
 * Both halves matter: the English must be gone, AND a way in must be on screen.
 * Deleting the interpolation alone would satisfy the first and leave a visitor
 * with a page that refuses to say why it is empty.
 */
function assertHonestArrival() {
  expect(text(), "the server's English 401 body reached a 繁體中文 screen").not.toContain(
    "not authenticated",
  );
  const login = Array.from(container.querySelectorAll("a")).filter((a) =>
    (a.getAttribute("href") ?? "").endsWith("/auth/github/login"),
  );
  expect(login.length, "nothing on screen says how to log in").toBeGreaterThan(0);
}

// --- 1. the shared component ------------------------------------------------

test("IA-6 `unauthenticated` is true for a 401 and for nothing else", () => {
  expect(unauthenticated(new ApiError(401, "not authenticated"))).toBe(true);
  // 403 is a different fact — you are logged in and this is not yours (ADR-011),
  // and SkillFiles already words it separately. Logging in again fixes nothing.
  expect(unauthenticated(new ApiError(403, "forbidden"))).toBe(false);
  expect(unauthenticated(new ApiError(500, "boom"))).toBe(false);
  expect(unauthenticated(new Error("network"))).toBe(false);
  expect(unauthenticated(null)).toBe(false);
});

test("IA-6 the 401 state says it in 繁體中文 and carries the login action", async () => {
  await render(<LoginRequired what="你的 Skill 清單" />, () => text().includes("需要登入"));
  expect(text()).toContain("你的 Skill 清單需要登入。");
  assertHonestArrival();
  // Expected state, not an error: `role="alert"` interrupts, and being logged
  // out is not an interruption (the ruling's word is 具名狀態).
  expect(container.querySelector("[role=status]")).not.toBeNull();
  expect(container.querySelector("[role=alert]")).toBeNull();
});

test("IA-6 ReadFailure does NOT swallow a non-401 — a 500 still says what failed", async () => {
  // Neutral settle again: waiting for 「無法讀取」 would turn a component that
  // swallowed the 500 into a timeout rather than into this assertion.
  await render(
    <ReadFailure error={new ApiError(500, "資料庫連線中斷")} what="下載紀錄" />,
    () => text().length > 0,
  );
  expect(text()).toContain("無法讀取下載紀錄：資料庫連線中斷");
  expect(container.querySelector("[role=alert]")).not.toBeNull();
  // And it must not offer a login for a failure a login cannot fix.
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

// --- 2. arriving logged out, route by route ---------------------------------

test("IA-6 /workspace/skills — a nav destination stops printing the server's English", async () => {
  loggedOutPlatform();
  await render(<WorkspaceSkills />, () => settled());
  expect(text()).toContain("你的 Skill 清單需要登入。");
  assertHonestArrival();
});

test("IA-6 /lab/test-cases — both reads on the page answer, not just the first", async () => {
  loggedOutPlatform();
  await render(<TestCaseList />, () => settled());
  // 兩處呼叫點：清單本身，以及「建立新的 Test Case」底下的 Skill 選單。
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
  // The whole point of this one: the form a visitor would have filled in is not
  // there to be filled in. Refusing after the work is the shape the ruling
  // called the worst in the audit.
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
  // 「輸入另一個 Run 的 ID 後開始比較。」 is advice for somebody who could.
  expect(text()).not.toContain("輸入另一個 Run 的 ID 後開始比較");
  assertHonestArrival();
});

test("IA-6 /lab/run stops sending a logged-out visitor to hunt for query parameters", async () => {
  loggedOutPlatform();
  await render(<RunPreflight />, () =>
    settled(() => text().includes("兩個 ID") || text().includes("需要登入")),
  );
  expect(text()).toContain("試跑與執行前權限確認需要登入。");
  // The misdirection, verbatim from the audit: with no `?skill=&test_case=`
  // this page used to answer the parameters question first, to a visitor for
  // whom the parameters were never the problem.
  expect(text()).not.toContain("?skill=");
  expect(text()).not.toContain("兩個 ID");
  assertHonestArrival();
});

test("IA-6 the site-wide feedback form says it before a paragraph is written", async () => {
  loggedOutPlatform();
  // On `/policy` deliberately: that page is the one the product decided to keep
  // outside the session, and this form is in the layout — so the deferred
  // rejection was on all 17 routes, not on the two the ruling enumerated.
  await render(<FeedbackEntry pathname="/policy" />, () =>
    settled(() => container.querySelector("form") !== null || text().includes("需要登入")),
  );
  expect(text()).toContain("回報問題需要登入。");
  expect(container.querySelector("form")).toBeNull();
  expect(container.querySelector("#feedback-message")).toBeNull();
  assertHonestArrival();
});

// --- 3. the two precedents the ruling rests on, which had no test at all -----

test("IA-6 precedent: ForkAction tells a visitor what logging in buys (SkillDetail)", async () => {
  loggedOutPlatform();
  await render(<SkillDetail />, () => text().includes("Fork"));
  // The sentence itself. It is one of the three the ruling cites as this
  // product having already answered 「由頁面自己說」, and nothing held it down.
  expect(text()).toContain("登入後即可 Fork 這個 Skill 到你的工作區。");
  // And the control it replaces is genuinely absent, not merely disabled.
  expect(
    Array.from(container.querySelectorAll("button")).some((b) =>
      (b.textContent ?? "").includes("Fork 這個 Skill"),
    ),
  ).toBe(false);
  expect(text()).not.toContain("not authenticated");
});

test("IA-6 precedent: /workspace/account says a login is needed, and how", async () => {
  loggedOutPlatform();
  await render(<WorkspaceAccount />, () => settled());
  expect(text()).toContain("帳號資料需要登入。");
  // This page was the twelfth site, not one of the eleven: it already carried
  // 「沒有登入的話，這一頁不會有東西可以看」 — the right state, printed after
  // 「無法讀取帳號資料：not authenticated」 and with no way to act on it.
  assertHonestArrival();
});

// --- 4. the seven seconds of nothing ----------------------------------------

/**
 * Both hooks that were missing `retry: false`, in one probe.
 *
 * Nothing but the query state: what is being measured is whether a refused read
 * is ALLOWED TO BE REFUSED, and any markup around it would only add ways for
 * this to pass for the wrong reason.
 */
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

  // Settled on the request having HAPPENED, not on the state it produced: this
  // has to be able to observe a query still sitting in pending.
  await render(<RetryProbe />, () => calls >= 2);
  for (let i = 0; i < 20; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }

  // With React Query's default three retries the first 401 starts a 1s backoff
  // and BOTH stay `isPending` — which is what put 「載入版本清單中…」 and
  // 「載入執行紀錄中…」 on screen for about seven seconds with nothing behind
  // them (設計 §2.1，資訊架構 §5 IA-6 的最後一段).
  expect(text()).toBe("versions:error trace:error");
  // And exactly one request each: a refusal that is the same however often it
  // is asked is not worth asking twice.
  expect(calls).toBe(2);
});
