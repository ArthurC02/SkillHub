import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { Downloads } from "./pages/Downloads";
import { RunTrace } from "./pages/RunTrace";
import { WorkspaceAccount } from "./pages/WorkspaceAccount";
import { WorkspaceRuns } from "./pages/WorkspaceRuns";
import { WorkspaceSkills } from "./pages/WorkspaceSkills";

// 02:WS-002 第 1 條 / WS-004 — the workspace's own lists, plus 第 3 條's delete
// on what a run produced (02:SEC-006). Same hand-rolled DOM plumbing as the
// other suites; @testing-library is not a dependency of this app.

const SKILL = "11111111-1111-1111-1111-111111111111";
const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
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
  useParams: () => ({ runId: RUN }),
  useSearch: () => ({}),
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

/** Opens the disclosure whose summary says `label`, the way a click would. */
async function open(label: string) {
  const details = Array.from(container.querySelectorAll("details")).find((d) =>
    (d.querySelector("summary")?.textContent ?? "").includes(label),
  )!;
  await act(async () => {
    details.open = true;
    details.dispatchEvent(new Event("toggle"));
  });
}

// --- the run history --------------------------------------------------------

const RUN_ROW = {
  run_id: RUN,
  status: "succeeded",
  skill_id: SKILL,
  skill_name: "CSV 清理",
  skill_version_id: "22222222-2222-2222-2222-222222222222",
  provider: "self-hosted",
  cleanup_status: "cleaned",
  created_at: "2026-08-17T00:00:00Z",
  finished_at: "2026-08-17T00:04:00Z",
};

test("WS-004 a run history row words `succeeded` as execution, never as a pass", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [RUN_ROW] }));
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  // ADR-025: a list is where "the workload finished" is cheapest to misread as
  // "the task was done", so the row says which one it means.
  expect(text()).toContain("執行狀態：執行完成");
  expect(text()).toContain("不是「任務有沒有做到」");
  expect(text()).not.toContain("成功");
});

test("WS-004 a run whose sandbox was not cleaned up says so on the row", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        { ...RUN_ROW, status: "failed", status_reason: "the provider could not carry the attempt" },
        { ...RUN_ROW, run_id: "run-2", cleanup_status: "failed" },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("執行失敗"));

  expect(text()).toContain("the provider could not carry the attempt");
  expect(text()).toContain("清理失敗");
  // 設計系統 §2.1: 已清理 is a fact the owner wants — the sandbox was torn down —
  // and rendering nothing for it made the majority case indistinguishable from a
  // field that was never rendered at all.
  expect(text()).toContain("已清理");
  // §4.4: --accent-border is 未知／未驗證 and --danger is 這件事不通過. 清理失敗 is
  // the second, so it must not wear the class 尚未清理 and 清理中 wear.
  expect(container.querySelectorAll(".badge-unverified")).toHaveLength(0);
  expect(container.querySelectorAll(".badge-danger")).toHaveLength(1);
});

test("WS-002 an empty run history says nothing ran, not that records were cleared", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [] }));
  await render(<WorkspaceRuns />, () => text().includes("還沒有跑過任何 Run"));

  expect(text()).toContain("不是紀錄被清掉了");
});

// --- the own-skills list ----------------------------------------------------

test("WS-004 the own-skills list links each row on to its files and packaging", async () => {
  vi.stubGlobal("fetch", () =>
    json({ skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。" }] }),
  );
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain(`/skills/${SKILL}`);
  expect(hrefs).toContain(`/skills/${SKILL}/files`);
  expect(hrefs).toContain(`/skills/${SKILL}/package`);
  // The other workspace lists are reachable from here rather than only from the
  // header, because this is the page a reader lands on looking for "my stuff".
  expect(hrefs).toContain("/workspace/runs");
  expect(hrefs).toContain("/workspace/downloads");
});

// --- deleting a skill (WS-005, 04 丙-22 ①) ----------------------------------

test("WS-005 deleting a skill says what survives it before anything is destroyed", async () => {
  const calls: [string, string | undefined][] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deleted: true,
        versions_retained: 2,
        note: "skill removed from your workspace, lists, and search; version snapshots are retained for the 30-day grace period",
      });
    }
    return json({ skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。" }] });
  });
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  await act(async () => button("刪除")?.click());
  // 02:WS-002 第 3 條: the scope is on screen before the request, and it names
  // the grace period and the forks it does not touch.
  expect(text()).toContain("版本快照會先凍結保留一段期間再清除");
  expect(text()).toContain("別人 Fork 過的版本");
  expect(calls.some(([, method]) => method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => calls.some(([, method]) => method === "DELETE"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain(`/skills/${SKILL}`);

  // After the fact the server's own note is what is shown — not a second copy of
  // the scope written on this side.
  await waitFor(() => text().includes("version snapshots are retained"));
});

// --- deleting the account (CORE-007, 04 丙-22 ②) -----------------------------

const ME = {
  user_id: "u-1",
  email: "tester@example.com",
  display_name: "tester",
  workspace_id: "ws-1",
  deletion_requested_at: null,
  purge_after: null,
};

test("CORE-007 requesting account deletion starts a grace period and shows the server's scope", async () => {
  const calls: [string, string | undefined][] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deletion_requested_at: "2026-08-18T00:00:00Z",
        purge_after: "2026-09-17T00:00:00Z",
        cancellable: true,
        scope: "Your account stays usable until the grace period ends.",
      });
    }
    return json(ME);
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除我的帳號"));

  await act(async () => button("刪除我的帳號")?.click());
  // The pre-scope is about this control, and its whole job is to say that
  // nothing is destroyed yet — the account delete is a countdown, not an act.
  expect(text()).toContain("不會立刻刪掉任何東西");
  expect(calls.some(([, m]) => m === "DELETE")).toBe(false);

  await act(async () => button("確認開始刪除")?.click());
  await waitFor(() => text().includes("Your account stays usable"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain("/me");
});

test("CORE-007 a pending deletion is a state with a date and a way out, not a receipt", async () => {
  const posts: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      posts.push(String(input));
      return json({ deletion_requested_at: null });
    }
    // GET /me after the cancel still answers the pending row; what this test is
    // about is the request going out, not the invalidation round trip.
    return json({
      ...ME,
      deletion_requested_at: "2026-08-18T00:00:00Z",
      purge_after: "2026-09-17T00:00:00Z",
    });
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除申請中"));

  // A user who closed the tab after asking has no other place to find either of
  // these, which is what 02:SEC-006「刪除工作具可追蹤狀態」 is asking for.
  expect(text()).toContain("2026-09-17T00:00:00Z");
  expect(text()).toContain("再按一次刪除不會提早");

  await act(async () => button("取消刪除申請")?.click());
  await waitFor(() => posts.length > 0);
  expect(posts[0]).toContain("/me/deletion/cancel");
});

// --- the per-download records ----------------------------------------------

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
  expires_at: "2099-01-01T00:00:00Z",
  created_at: "2026-08-17T00:00:00Z",
  download_count: 2,
  includes_test_cases: false,
};

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

  // Not fetched until asked for: a history page holds every package this
  // workspace ever built, and one request per row on load would make the
  // cheapest question on the page the most expensive read in the API.
  expect(urls.some((u) => u.includes("/records"))).toBe(false);

  await open("誰下載過");
  await waitFor(() => text().includes("tester"));

  expect(urls.some((u) => u.includes(`/downloads/${ARTIFACT}/records`))).toBe(true);
  expect(text()).toContain("2026-08-17T09:00:00Z");
  // A purged account leaves the row de-identified rather than removing it:
  // "somebody, at this time" is still true.
  expect(text()).toContain("deleted user");
  // CORE-008: this is the product feature, the audit event is the other record.
  expect(text()).toContain("與稽核事件是兩份不同的紀錄");
});

// --- what a run produced, and deleting one of them --------------------------

const RUN_ARTIFACT = {
  artifact_id: "aaaa1111-2222-3333-4444-555566667777",
  file_name: "cleaned.csv",
  content_type: "text/csv",
  size_bytes: 2048,
  content_hash: "sha256:dddd",
  created_at: "2026-08-17T00:03:00Z",
  purged: false,
};

/** Everything the run page reads; the parts this test is not about answer 404. */
function stubRunPage(artifacts: unknown[], onDelete?: (url: string) => void) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === "DELETE") {
      onDelete?.(url);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/artifacts")) return json({ artifacts });
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

  // The bytes are never offered: the archive is a sandbox's output and the
  // control plane does not open it, so there is no link to invent.
  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs.some((h) => h.includes("/artifacts/"))).toBe(false);
  expect(text()).toContain("控制平面不打開它");

  await act(async () => button("刪除")?.click());
  // 02:WS-002 第 3 條: the scope is on screen before anything is destroyed, and
  // what it promises is that the evaluation keeps its citation.
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
  // "It expired" and "it never existed" are different answers; the row is the
  // first one and the empty state is the second.
  expect(text()).toContain("曾經產生過這個檔案」仍然是事實");
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
