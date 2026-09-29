import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../core/api/queryClient";
import { queryKeys } from "../../../core/api/queryKeys";
import { WorkspaceHome } from "./WorkspaceHome.page";

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
    children: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce((value, [key, param]) => {
      return value.replace(`$${key}`, param);
    }, to);
    const query = new URLSearchParams(search).toString();
    return (
      <a className={className} href={query ? `${path}?${query}` : path}>
        {children as never}
      </a>
    );
  },
}));

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  await act(async () => queryClient.cancelQueries());
  container.remove();
  vi.unstubAllGlobals();
});

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

function section(title: string) {
  return Array.from(container.querySelectorAll("section")).find(
    (candidate) => candidate.firstElementChild?.querySelector("h2")?.textContent === title,
  )!;
}

const run = (id: string, name: string, status: string, verdict: string) => ({
  run_id: id,
  status,
  skill_id: "skill-1",
  skill_name: name,
  skill_version_id: "version-1",
  provider: "self-hosted",
  cleanup_status: { value: "pending", label: "待清理", note: "完成後由平台清理。" },
  evaluation: { value: verdict, label: verdict, note: "依驗收條件判定。" },
  created_at: "2026-09-28T12:00:00Z",
});

test("workspace home separates decisions, active work, and owned assets from server facts", async () => {
  const fetchSpy = vi.fn((input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
      });
    }
    if (url.includes("/runs?")) {
      return json({
        runs: [
          run("run-review", "Needs review", "succeeded", "not_met"),
          run("run-failed", "Needs failure review", "failed", "not_evaluated"),
          run("run-active", "Still running", "running", "undetermined"),
          run("run-cancelled", "Cancelled", "cancelled", "not_evaluated"),
          run("run-done", "Already good", "succeeded", "met"),
        ],
      });
    }
    if (url.endsWith("/skills")) {
      return json({
        skills: [
          {
            skill_id: "skill-1",
            name: "PDF Summariser",
            summary: "整理長篇 PDF",
            redistribution: "unknown",
            risk: { scan_status: "scanned", level: "none", warnings: 0, disclosures: [] },
            verification: { value: "scanned", label: "已掃描", note: "靜態掃描完成。" },
          },
        ],
        total: 1,
        limit: 100,
        truncated: false,
      });
    }
    return json({ error: "not found" }, 404);
  });
  vi.stubGlobal("fetch", fetchSpy);

  await render(<WorkspaceHome />, () => (container.textContent ?? "").includes("PDF Summariser"));

  expect(section("需要留意").textContent).toContain("Needs review");
  expect(section("需要留意").textContent).toContain("Needs failure review");
  expect(section("需要留意").textContent).toContain("檢視證據");
  expect(section("需要留意").textContent).toContain("查看原因");
  expect(section("需要留意").textContent).not.toContain("Still running");
  expect(section("需要留意").textContent).not.toContain("Cancelled");
  expect(section("執行中").textContent).toContain("Still running");
  expect(section("執行中").textContent).not.toContain("Needs review");
  const allTrials = Array.from(section("執行中").querySelectorAll("a")).find(
    (link) => link.textContent === "查看全部試跑",
  );
  expect(allTrials?.getAttribute("href")).toBe("/workspace/runs");
  expect(container.textContent).not.toContain("Already good");
  expect(section("你的資產").textContent).toContain("PDF Summariser");
  expect(section("繼續進行")).toBeUndefined();
  expect(fetchSpy.mock.calls.some(([input]) => String(input).endsWith("/creation-sessions"))).toBe(
    false,
  );

  const refresh = Array.from(container.querySelectorAll("button")).find((candidate) =>
    candidate.textContent?.includes("重新整理"),
  );
  expect(refresh).toBeDefined();
  expect(refresh?.closest("p")?.querySelector("time")).not.toBeNull();

  const before = fetchSpy.mock.calls.filter(([input]) => String(input).includes("/runs?")).length;
  await act(async () => refresh?.click());
  await waitFor(
    () => fetchSpy.mock.calls.filter(([input]) => String(input).includes("/runs?")).length > before,
  );
});

test.each([
  { generate_skill: true, creation_skill: false },
  { generate_skill: false, creation_skill: true },
])(
  "workspace home does not request creation sessions when one creation flag is off",
  async (features) => {
    const fetchSpy = vi.fn((input: string) => {
      const url = String(input);
      if (url.endsWith("/me")) {
        return json({
          user_id: "u-1",
          email: "tester@example.com",
          display_name: "tester",
          workspace_id: "ws-1",
          operator: false,
          features,
        });
      }
      if (url.includes("/runs?")) return json({ runs: [] });
      if (url.endsWith("/skills")) {
        return json({ skills: [], total: 0, limit: 100, truncated: false });
      }
      return json({ error: "not found" }, 404);
    });
    vi.stubGlobal("fetch", fetchSpy);

    await render(<WorkspaceHome />, () =>
      (container.textContent ?? "").includes("目前沒有正在執行的試跑"),
    );

    expect(section("繼續進行")).toBeUndefined();
    expect(
      fetchSpy.mock.calls.some(([input]) => String(input).endsWith("/creation-sessions")),
    ).toBe(false);
  },
);

test("workspace home resumes the three most recent actionable creation sessions", async () => {
  const future = "2099-09-28T14:00:00Z";
  const session = (id: string, brief: string, state: string, updated_at: string) => ({
    id,
    revision: 1,
    state,
    snapshot: { brief },
    created_at: "2026-09-28T10:00:00Z",
    updated_at,
    expires_at: "2099-09-29T14:00:00Z",
    deadline: future,
  });
  const fetchSpy = vi.fn((input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
        features: { generate_skill: true, creation_skill: true },
      });
    }
    if (url.endsWith("/creation-sessions")) {
      return json([
        session("55555555-5555-4555-8555-555555555555", "已保存", "saved", "2026-09-28T14:00:00Z"),
        {
          ...session(
            "77777777-7777-4777-8777-777777777777",
            "已過操作期限",
            "waiting_confirmation",
            "2026-09-28T13:30:00Z",
          ),
          deadline: "2020-09-28T14:00:00Z",
        },
        session(
          "11111111-1111-4111-8111-111111111111",
          "整理採購文件",
          "waiting_input",
          "2026-09-28T13:00:00Z",
        ),
        session(
          "22222222-2222-4222-8222-222222222222",
          "檢查摘要品質",
          "working",
          "2026-09-28T12:00:00Z",
        ),
        session(
          "33333333-3333-4333-8333-333333333333",
          "修正流程圖",
          "needs_reupload",
          "2026-09-28T11:00:00Z",
        ),
        session(
          "44444444-4444-4444-8444-444444444444",
          "第四筆可續作",
          "failed",
          "2026-09-28T10:00:00Z",
        ),
        session(
          "66666666-6666-4666-8666-666666666666",
          "已取消",
          "cancelled",
          "2026-09-28T09:00:00Z",
        ),
      ]);
    }
    if (url.includes("/runs?")) return json({ runs: [] });
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });
  vi.stubGlobal("fetch", fetchSpy);

  await render(<WorkspaceHome />, () => (container.textContent ?? "").includes("最後更新："));

  const continuation = section("繼續進行");
  expect(continuation.textContent).toContain("整理採購文件");
  expect(continuation.textContent).toContain("等待你的補充");
  expect(continuation.textContent).toContain("檢查摘要品質");
  expect(continuation.textContent).toContain("正在創作");
  expect(continuation.textContent).toContain("修正流程圖");
  expect(continuation.textContent).toContain("請重新上傳流程圖");
  expect(
    Array.from(continuation.querySelectorAll("li strong"), (title) => title.textContent),
  ).toEqual(["整理採購文件", "檢查摘要品質", "修正流程圖"]);
  expect(continuation.querySelectorAll("li time")).toHaveLength(3);
  expect(
    continuation.querySelector(
      'a[href="/workspace/creations?session=11111111-1111-4111-8111-111111111111"]',
    )?.textContent,
  ).toBe("開啟會話");
  expect(
    continuation.querySelector(
      'a[href="/workspace/creations?session=22222222-2222-4222-8222-222222222222"]',
    )?.textContent,
  ).toBe("查看進度");
  expect(
    continuation.querySelector(
      'a[href="/workspace/creations?session=33333333-3333-4333-8333-333333333333"]',
    )?.textContent,
  ).toBe("查看這一步");
  expect(Array.from(container.querySelectorAll("h2"), (heading) => heading.textContent)).toEqual([
    "需要留意",
    "繼續進行",
    "執行中",
    "你的資產",
  ]);
});

test("workspace home keeps a creation read failure distinct from an empty continuation list", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
        features: { generate_skill: true, creation_skill: true },
      });
    }
    if (url.endsWith("/creation-sessions")) return json({ error: "service unavailable" }, 503);
    if (url.includes("/runs?")) return json({ runs: [] });
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () =>
    (container.textContent ?? "").includes("暫時無法讀取最近的創作"),
  );

  expect(section("繼續進行").textContent).not.toContain("沒有可繼續的創作");
  expect(section("繼續進行").textContent).not.toContain("service unavailable");
});

test("workspace home omits the continuation section when the recent session slice has no actionable item", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
        features: { generate_skill: true, creation_skill: true },
      });
    }
    if (url.endsWith("/creation-sessions")) return json([]);
    if (url.includes("/runs?")) return json({ runs: [] });
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () =>
    (container.textContent ?? "").includes("目前沒有正在執行的試跑"),
  );

  expect(section("繼續進行")).toBeUndefined();
});

test("workspace home surfaces a refresh failure after an earlier empty creation response", async () => {
  let creationReads = 0;
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
        features: { generate_skill: true, creation_skill: true },
      });
    }
    if (url.endsWith("/creation-sessions")) {
      creationReads += 1;
      return creationReads === 1 ? json([]) : json({ error: "service unavailable" }, 503);
    }
    if (url.includes("/runs?")) return json({ runs: [] });
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () =>
    (container.textContent ?? "").includes("目前沒有正在執行的試跑"),
  );
  expect(section("繼續進行")).toBeUndefined();

  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.creation.sessions });
  });
  await waitFor(() => (container.textContent ?? "").includes("暫時無法讀取最近的創作"));

  expect(section("繼續進行").textContent).not.toContain("沒有可繼續的創作");
  expect(section("繼續進行").textContent).not.toContain("service unavailable");
});

test("workspace home removes a creation continuation when its action deadline passes", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
        features: { generate_skill: true, creation_skill: true },
      });
    }
    if (url.endsWith("/creation-sessions")) {
      return json([
        {
          id: "88888888-8888-4888-8888-888888888888",
          revision: 1,
          state: "waiting_input",
          snapshot: { brief: "即將到期的創作" },
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          expires_at: new Date(Date.now() + 60_000).toISOString(),
          deadline: new Date(Date.now() + 100).toISOString(),
        },
      ]);
    }
    if (url.includes("/runs?")) return json({ runs: [] });
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () => (container.textContent ?? "").includes("即將到期的創作"));
  await waitFor(() => section("繼續進行") === undefined);

  expect(container.textContent).not.toContain("即將到期的創作");
});

test("workspace home without active work does not show a stale refresh control", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
      });
    }
    if (url.includes("/runs?")) {
      return json({ runs: [run("run-done", "Already good", "succeeded", "met")] });
    }
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () =>
    (container.textContent ?? "").includes("目前沒有正在執行的試跑"),
  );

  expect(container.textContent).not.toContain("上次取得於");
  expect(
    Array.from(container.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.includes("重新整理"),
    ),
  ).toBeUndefined();
});

async function waitFor(done: () => boolean) {
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`wait timed out: ${container.textContent}`);
}
