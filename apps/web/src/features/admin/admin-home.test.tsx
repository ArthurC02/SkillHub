import { act } from "react";
import { expect, test, vi } from "vitest";
import { platformResponse } from "../../testing/fixtures/platform";
import {
  container,
  calls,
  stub,
  mountAt,
  go,
  waitFor,
  has,
  field,
  button,
  click,
} from "./testing/harness";

const ADMIN_PATHS = [
  "/admin",
  "/admin/accounts",
  "/admin/skills",
  "/admin/dispatch",
  "/admin/rosters",
  "/admin/audit-log",
  "/admin/cost-statistics",
  "/admin/exposure",
  "/admin/agents",
  "/admin/settings",
];

test("OPS-001: the account menu offers 後台 to an operator", async () => {
  stub(true);
  await mountAt("/");
  await waitFor(has("tester"));
  expect(container.querySelector('header a[href="/admin"]')?.textContent).toBe("後台");
});

test("OPS-001: the account menu offers nothing to a member", async () => {
  stub(false);
  await mountAt("/");
  await waitFor(has("tester"));
  expect(container.querySelector('a[href="/admin"]')).toBeNull();
});

test("the admin home separates governance decisions from operations", async () => {
  stub(true);
  await mountAt("/admin");
  await waitFor(has("營運後台"));

  expect(
    Array.from(container.querySelectorAll(".admin-home-eyebrow"), (item) => item.textContent),
  ).toEqual(["Governing · Decisions", "Conducting · Operations"]);
});

test("the admin home leads with live operational priorities", async () => {
  stub(true);
  await mountAt("/admin");
  await waitFor(has("停止派送"));

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(priorities.querySelector('a[href="/admin/dispatch"]')?.textContent).toContain("停止派送");
  expect(
    priorities.querySelector('a[href="/admin/agents#admin-agent-proposals"]')?.textContent,
  ).toContain("1 件待核准");
  expect(
    priorities.querySelector('a[href="/admin/agents#admin-agent-findings"]')?.textContent,
  ).toContain("2 件待辦");
  expect(priorities.textContent).toContain("已取得 4/4 項狀態；其中最早取得於");
  expect(priorities.querySelector('a[href="/admin/exposure"]')?.textContent).toContain("1 件待審");
  expect(priorities.querySelector('[role="status"]')?.textContent).toBe("目前有 4 類狀態需留意。");
});

test("the admin home names an all-clear only after all four reads succeed", async () => {
  stub(true, (path) => {
    if (path === "/admin/dispatch") {
      return { body: { dispatching: true, halts: [] }, status: 200 };
    }
    if (path === "/admin/agents/proposals") {
      return { body: { proposals: [], total: 0 }, status: 200 };
    }
    if (path === "/admin/agents/findings") {
      return {
        body: {
          findings: [],
          counts: { open: 0, acknowledged: 0, resolved: 0, dismissed: 0, recovered: 0 },
        },
        status: 200,
      };
    }
    if (path === "/admin/exposure-reviews") {
      return { body: { publications: [] }, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin");
  await waitFor(has("已取得 4/4 項狀態"));

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(priorities.querySelector('[role="status"]')?.textContent).toBe(
    "這四項目前沒有需留意的狀態。",
  );
  expect(priorities.querySelectorAll('a[data-state="pending"], a[data-state="halt"]')).toHaveLength(
    0,
  );
});

test("the admin home never claims all-clear when one of four zero-count reads fails", async () => {
  stub(true, (path) => {
    if (path === "/admin/dispatch") {
      return { body: { dispatching: true, halts: [] }, status: 200 };
    }
    if (path === "/admin/agents/proposals") {
      return { body: { error: "service unavailable" }, status: 503 };
    }
    if (path === "/admin/agents/findings") {
      return {
        body: {
          findings: [],
          counts: { open: 0, acknowledged: 0, resolved: 0, dismissed: 0, recovered: 0 },
        },
        status: 200,
      };
    }
    if (path === "/admin/exposure-reviews") {
      return { body: { publications: [] }, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin");
  await waitFor(has("已取得 3/4 項狀態"));

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(priorities.querySelector('[role="status"]')).toBeNull();
  expect(priorities.querySelector('[role="alert"]')?.textContent).toContain("1 項狀態無法取得");
});

test("the admin home does not mistake a failed priority read for an empty queue", async () => {
  stub(true, (path) =>
    path === "/admin/agents/proposals"
      ? { body: { error: "service unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin");
  await waitFor(has("無法取得"));

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(
    priorities.querySelector('a[href="/admin/agents#admin-agent-proposals"]')?.textContent,
  ).toContain("無法取得");
  expect(priorities.textContent).not.toContain("已取得 4/4 項狀態");
  expect(
    priorities.querySelector('a[href="/admin/agents#admin-agent-proposals"]')?.textContent,
  ).not.toContain("0 件待核准");
  expect(priorities.querySelector('[role="alert"]')?.textContent).toBe(
    "1 項狀態無法取得；請重新整理後再判斷是否還有待處理事項。",
  );
  expect(priorities.textContent).toContain("已取得 3/4 項狀態；其中最早取得於");
});

test("the admin home shows no freshness claim when all four priority reads fail", async () => {
  stub(true, (path) =>
    [
      "/admin/dispatch",
      "/admin/agents/proposals",
      "/admin/agents/findings",
      "/admin/exposure-reviews",
    ].includes(path)
      ? { body: { error: "service unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin");
  await waitFor(() => {
    const values = Array.from(
      container.querySelectorAll('[aria-label="目前需留意"] a strong'),
      (strong) => strong.textContent,
    );
    return values.length === 4 && values.every((value) => value === "無法取得");
  });

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(priorities.querySelector('[role="alert"]')?.textContent).toBe(
    "4 項狀態無法取得；請重新整理後再判斷是否還有待處理事項。",
  );
  expect(priorities.textContent).not.toContain("最早取得於");
});

test("the admin home can refresh operational state without leaving the page", async () => {
  let dispatchReads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    dispatchReads += 1;
    return { body: { dispatching: dispatchReads > 1, halts: [] }, status: 200 };
  });
  await mountAt("/admin");
  await waitFor(has("目前有 4 類狀態需留意。"));

  await click(button("重新整理狀態"));
  await waitFor(has("目前有 3 類狀態需留意。"));
  expect(has("正在派送")()).toBe(true);
  expect(dispatchReads).toBe(2);
});

test.each([
  "/admin/dispatch",
  "/admin/agents/proposals",
  "/admin/agents/findings",
  "/admin/exposure-reviews",
])("the admin home waits for %s before renewing its priority conclusion", async (pendingPath) => {
  const clear: Record<string, unknown> = {
    "/admin/dispatch": { dispatching: true, halts: [] },
    "/admin/agents/proposals": { proposals: [], total: 0 },
    "/admin/agents/findings": {
      findings: [],
      counts: { open: 0, acknowledged: 0, resolved: 0, dismissed: 0, recovered: 0 },
    },
    "/admin/exposure-reviews": { publications: [] },
  };
  stub(true, (path) => (path in clear ? { body: clear[path], status: 200 } : undefined));
  await mountAt("/admin");
  await waitFor(has("已取得 4/4 項狀態"));
  const summary = field<HTMLElement>(".admin-home-summary");
  const refresh = button("重新整理狀態");
  expect(summary.textContent).toBe("這四項目前沒有需留意的狀態。");

  let finishRead!: (response: Response) => void;
  const held = new Promise<Response>((resolve) => {
    finishRead = resolve;
  });
  const originalFetch = fetch;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) =>
    String(input).split("?")[0] === pendingPath ? held : originalFetch(input, init),
  );
  try {
    await click(refresh);
    await waitFor(() => refresh.disabled);
    expect(summary.getAttribute("aria-busy")).toBe("true");
    expect(summary.textContent).toBe("正在更新四項狀態；完成後再判斷是否有待處理事項。");
    expect(button("更新中…").disabled).toBe(true);
    expect(container.querySelectorAll(".admin-home-priority")).toHaveLength(4);
  } finally {
    await act(async () => {
      finishRead(new Response(JSON.stringify(platformResponse(pendingPath).body)));
    });
  }
  await waitFor(() => !refresh.disabled);
  expect(summary.getAttribute("aria-busy")).toBe("false");
  expect(summary.textContent).toBe("目前有 1 類狀態需留意。");
  expect(button("重新整理狀態").disabled).toBe(false);
  stub(true, (path) =>
    path === pendingPath
      ? { body: { error: "service unavailable" }, status: 503 }
      : path in clear
        ? { body: clear[path], status: 200 }
        : undefined,
  );
  await click(button("重新整理狀態"));
  await waitFor(has("1 項狀態無法取得"));
  expect(container.querySelector(".admin-home-summary")).toBeNull();
  expect(has("這四項目前沒有需留意的狀態。")()).toBe(false);
});

test("OPS-001: the admin page stays loading while the operator check is pending", async () => {
  vi.stubGlobal("fetch", () => new Promise<Response>(() => {}));
  await mountAt("/admin");
  expect(field<HTMLElement>("main [data-loading]").textContent).toBe("載入中…");
  expect(container.querySelector("main h1")).toBeNull();
});

test("OPS-001: a failed operator check names the read failure instead of a missing page", async () => {
  stub(true, (path) =>
    path === "/me" ? { body: { error: "service unavailable" }, status: 503 } : undefined,
  );
  await mountAt("/admin");
  await waitFor(has("暫時無法讀取後台。請再試一次，或稍後再試。"));
  expect(field<HTMLElement>('main [role="alert"]').textContent).toBe(
    "暫時無法讀取後台。請再試一次，或稍後再試。",
  );
  expect(has("service unavailable")()).toBe(false);
  expect(has("這一頁現在不存在")()).toBe(false);
});

test("OPS-001: an unauthenticated operator check asks for sign-in", async () => {
  stub(true, (path) =>
    path === "/me" ? { body: { error: "not authenticated" }, status: 401 } : undefined,
  );
  await mountAt("/admin");
  await waitFor(has("後台需要登入。"));
  expect(container.querySelector('main [role="status"]')?.textContent).toContain("後台需要登入。");
  expect(has("not authenticated")()).toBe(false);
  expect(has("這一頁現在不存在")()).toBe(false);
});

test("OPS-001: a member who types an /admin address gets the missing page and no admin request", async () => {
  stub(false);
  await mountAt("/");
  for (const path of ADMIN_PATHS) {
    await go("/");
    await waitFor(() => !has("這一頁現在不存在")());
    await go(path);
    await waitFor(has("這一頁現在不存在"));
    expect(window.location.pathname).toBe(path);
    expect(container.querySelector('nav[aria-label="後台"]'), path).toBeNull();
  }
  expect(calls.filter((c) => c.url.startsWith("/admin"))).toEqual([]);
});

test("OPS-001: an address nobody mounted renders the same missing page", async () => {
  stub(true);
  await mountAt("/no-such-page");
  await waitFor(has("這一頁現在不存在"));
});

test("OPS-001: clean mode adds the operator sentence on /admin pages only", async () => {
  window.__SKILLHUB_CLEAN_MODE__ = true;
  stub(true);
  await mountAt("/workspace/account");
  await waitFor(has("淨測試模式"));
  expect(has("任何人都能以 operator 登入")()).toBe(false);
  await go("/admin");
  await waitFor(has("任何人都能以 operator 登入"));
});
