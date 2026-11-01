import { act } from "react";
import { expect, test } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import {
  ADMIN_AGENT_FINDING,
  ADMIN_AGENT_FINDINGS,
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_PROPOSALS,
  AGENT_FINDING,
  AGENT_PROPOSAL,
} from "../../testing/fixtures/platform";
import {
  container,
  calls,
  router,
  stub,
  mountAt,
  go,
  waitFor,
  has,
  field,
  type,
  button,
  click,
  submit,
} from "./testing/harness";

test("OPS-012: the inbox lists live findings with how often they were reported, and counts every status", async () => {
  stub(true);
  await mountAt("/admin/agents");
  await waitFor(has("分割表輪替從來沒有成功過，已經超過兩個週期。"));
  expect(has("回報 3 次")()).toBe(true);
  expect(has("待處理")()).toBe(true);
  expect(has("待辦：2")()).toBe(true);
  expect(has("已解決：4")()).toBe(true);
  expect(has("已自行恢復：2")()).toBe(true);
  expect(calls.some((c) => c.url === "/admin/agents/findings")).toBe(true);
});

test("OPS-012: the inbox can load a later finding and keep it after opening its detail", async () => {
  const later = {
    ...ADMIN_AGENT_FINDINGS.findings[0],
    id: "5c1d2e3f-4a5b-4c6d-8e7f-90a1b2c3d4e6",
    title: "較早的待辦",
  };
  stub(true, (path, _method, url) => {
    if (url === "/admin/agents/findings?cursor=later") {
      return { body: { findings: [later], counts: ADMIN_AGENT_FINDINGS.counts }, status: 200 };
    }
    if (url === "/admin/agents/findings") {
      return { body: { ...ADMIN_AGENT_FINDINGS, next_cursor: "later" }, status: 200 };
    }
    if (path === `/admin/agents/findings/${later.id}`) {
      return { body: { finding: later, events: [] }, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("分割表輪替從來沒有成功過，已經超過兩個週期。"));
  await click(button("載入更多待辦"));
  await waitFor(has("較早的待辦"));
  expect(calls.some((c) => c.url === "/admin/agents/findings?cursor=later")).toBe(true);
  const laterRow = Array.from(container.querySelectorAll(".download-item")).find((row) =>
    row.textContent?.includes("較早的待辦"),
  );
  await click(laterRow!.querySelector("a")!);
  await waitFor(has("回到待辦"));
  await click(
    Array.from(container.querySelectorAll("a")).find((link) => link.textContent === "回到待辦")!,
  );
  await waitFor(has("較早的待辦"));
});

test("OPS-012: a failed later inbox page keeps loaded findings and can retry", async () => {
  let nextPageReads = 0;
  const later = {
    ...ADMIN_AGENT_FINDINGS.findings[0],
    id: "5c1d2e3f-4a5b-4c6d-8e7f-90a1b2c3d4e6",
    title: "較早的待辦",
  };
  stub(true, (_path, _method, url) => {
    if (url === "/admin/agents/findings") {
      return { body: { ...ADMIN_AGENT_FINDINGS, next_cursor: "later" }, status: 200 };
    }
    if (url === "/admin/agents/findings?cursor=later") {
      nextPageReads += 1;
      return nextPageReads === 1
        ? { body: { error: "service unavailable" }, status: 503 }
        : { body: { findings: [later], counts: ADMIN_AGENT_FINDINGS.counts }, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("載入更多待辦"));
  await click(button("載入更多待辦"));
  await waitFor(has("後續待辦暫時無法讀取"));
  expect(has("分割表輪替從來沒有成功過，已經超過兩個週期。")()).toBe(true);
  expect(container.querySelector('nav[aria-label="平台 Agent 工作區"]')?.textContent).toContain(
    "待辦 2 件",
  );
  await click(button("重試載入更多待辦"));
  await waitFor(has("較早的待辦"));
  expect(nextPageReads).toBe(2);
  expect(has("後續待辦暫時無法讀取")()).toBe(false);
});

test("the admin home keeps the known finding count after a later page fails", async () => {
  stub(true, (_path, _method, url) => {
    if (url === "/admin/agents/findings") {
      return { body: { ...ADMIN_AGENT_FINDINGS, next_cursor: "later" }, status: 200 };
    }
    if (url === "/admin/agents/findings?cursor=later") {
      return { body: { error: "service unavailable" }, status: 503 };
    }
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("載入更多待辦"));
  await click(button("載入更多待辦"));
  await waitFor(has("後續待辦暫時無法讀取"));
  await go("/admin");
  expect(
    container.querySelector('a[href="/admin/agents#admin-agent-findings"]')?.textContent,
  ).toContain("2 件待辦");
});

test("OPS-012: losing the operator session on a later page hides cached findings", async () => {
  stub(true, (_path, _method, url) => {
    if (url === "/admin/agents/findings") {
      return { body: { ...ADMIN_AGENT_FINDINGS, next_cursor: "later" }, status: 200 };
    }
    if (url === "/admin/agents/findings?cursor=later") {
      return { body: { error: "not authenticated" }, status: 401 };
    }
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("載入更多待辦"));
  await click(button("載入更多待辦"));
  await waitFor(has("待辦需要登入"));
  expect(has("打開這件事")()).toBe(false);
  expect(has("重試載入更多待辦")()).toBe(false);
});

test("OPS-012: a closed status in the address lists that status; any other value falls back to the live list", async () => {
  stub(true);
  await mountAt("/admin/agents", { status: "resolved" });
  await waitFor(() => calls.some((c) => c.url === "/admin/agents/findings?status=resolved"));
  await go("/admin/agents", { status: "open" });
  await waitFor(() => calls.some((c) => c.url === "/admin/agents/findings"));
  expect(calls.some((c) => c.url.includes("status=open"))).toBe(false);
});

test("OPS-012: an empty view says how many it holds", async () => {
  stub(true, (path) =>
    path === "/admin/agents/findings"
      ? { body: { ...ADMIN_AGENT_FINDINGS, findings: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("待辦：0 件。"));
});

test("OPS-012: a finding opens with the values it rests on, its history and only the moves its status allows", async () => {
  stub(true);
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("這件事"));
  await waitFor(has("日報首次回報"));
  expect(has("/maintenance_jobs/rotate-partitions/overdue_ratio ＝ 3.4")()).toBe(true);
  expect(has("日報再次回報")()).toBe(true);
  expect(
    Array.from(
      container.querySelectorAll<HTMLInputElement>('input[name="admin-finding-move"]'),
    ).map((input) => input.value),
  ).toEqual(["acknowledged", "resolved", "dismissed"]);
  expect(container.querySelectorAll('textarea[id="admin-finding-note"]')).toHaveLength(1);
  expect(button("我來處理")).toBeDefined();
  expect(has("忽略這件事")()).toBe(true);
  expect(has("重新打開")()).toBe(false);
  expect(has("打開這件事")()).toBe(false);
});

test("a finding's cited report opens the agent run that produced its evidence", async () => {
  stub(true);
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("查看產生這份依據的執行"));
  const source = Array.from(container.querySelectorAll("a")).find(
    (link) => link.textContent === "查看產生這份依據的執行",
  )!;
  await click(source);
  await waitFor(has("這次執行"));
  expect(router.state.location.search).toMatchObject({
    run: "9a1f3c2e-0b4d-4e5f-8a6b-7c8d9e0f1a2b",
  });
  expect(router.state.location.search.finding).toBeUndefined();
});

test("a finding event without a reporting run does not invent a source link", async () => {
  stub(true, (path) =>
    path === `/admin/agents/findings/${AGENT_FINDING}`
      ? {
          body: {
            finding: ADMIN_AGENT_FINDINGS.findings[0],
            events: [{ ...ADMIN_AGENT_FINDING.events[0], run_id: undefined }],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("/maintenance_jobs/rotate-partitions/overdue_ratio ＝ 2.4"));
  expect(has("查看產生這份依據的執行")()).toBe(false);
});

test.each([
  { name: "missing", evidence: undefined },
  { name: "empty", evidence: {} },
])(
  "a finding's latest report with $name evidence names the gap instead of showing older values",
  async ({ evidence }) => {
    const latestRun = "11111111-1111-4111-8111-111111111111";
    stub(true, (path) =>
      path === `/admin/agents/findings/${AGENT_FINDING}`
        ? {
            body: {
              finding: ADMIN_AGENT_FINDING.finding,
              events: [
                ADMIN_AGENT_FINDING.events[0],
                { ...ADMIN_AGENT_FINDING.events[1], run_id: latestRun, evidence },
              ],
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/agents", { finding: AGENT_FINDING });
    await waitFor(has("未測量：目前沒有可核對的引用值。"));
    expect(has("/maintenance_jobs/rotate-partitions/overdue_ratio ＝ 2.4")()).toBe(false);
    const source = Array.from(container.querySelectorAll("a")).find(
      (link) => link.textContent === "查看最近一次回報的執行",
    );
    expect(source?.getAttribute("href")).toBe(`/admin/agents?run=${latestRun}`);
  },
);

test("a finding without report events names the missing evidence without inventing a source", async () => {
  stub(true, (path) =>
    path === `/admin/agents/findings/${AGENT_FINDING}`
      ? { body: { finding: ADMIN_AGENT_FINDING.finding, events: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("未測量：目前沒有可核對的引用值。"));
  expect(has("查看最近一次回報的執行")()).toBe(false);
});

test("OPS-012: a resolved finding offers only to reopen it", async () => {
  const resolved = { ...ADMIN_AGENT_FINDINGS.findings[0], status: "resolved" };
  stub(true, (path) =>
    path === `/admin/agents/findings/${AGENT_FINDING}`
      ? { body: { finding: resolved, events: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("重新打開"));
  expect(has("我來處理")()).toBe(false);
  expect(has("標記已解決")()).toBe(false);
  expect(has("忽略這件事")()).toBe(false);
});

test("a finding whose refreshed details cannot be read cannot be moved from cached facts", async () => {
  let unreadable = false;
  stub(true, (path) =>
    unreadable && path === `/admin/agents/findings/${AGENT_FINDING}`
      ? { body: { error: "finding unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("標記已解決"));
  unreadable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.agentFinding(AGENT_FINDING) });
  });
  await waitFor(() => Boolean(container.querySelector('[role="alert"]')));
  expect(has("標記已解決")()).toBe(false);
  expect(has("我來處理")()).toBe(false);
});

test("OPS-012: taking on a finding sends the move with the operator's note", async () => {
  stub(true, (_path, method) => (method === "PUT" ? { body: {}, status: 204 } : undefined));
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", " checking the job ");
  await submit("#admin-finding-note");
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/agents/findings/${AGENT_FINDING}/status`,
    body: { status: "acknowledged", note: "checking the job" },
  });
});

test("a completed finding move focuses its result after the controls change", async () => {
  let acknowledged = false;
  stub(true, (path, method) => {
    if (path === `/admin/agents/findings/${AGENT_FINDING}/status` && method === "PUT") {
      acknowledged = true;
      return { body: {}, status: 204 };
    }
    if (path === `/admin/agents/findings/${AGENT_FINDING}`) {
      return {
        body: {
          ...ADMIN_AGENT_FINDING,
          finding: {
            ...ADMIN_AGENT_FINDING.finding,
            status: acknowledged ? "acknowledged" : "open",
          },
        },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", "checking the job");
  await submit("#admin-finding-note");
  await waitFor(has("已改成「處理中」。"));
  expect(has("我來處理")()).toBe(false);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-finding-result"));
});

test.each(["stale", "unavailable"])(
  "a completed finding move keeps its result and hides unverified controls when details are %s",
  async (refresh) => {
    let moved = false;
    let verified = false;
    stub(true, (path, method) => {
      if (path === `/admin/agents/findings/${AGENT_FINDING}/status` && method === "PUT") {
        moved = true;
        return { body: {}, status: 204 };
      }
      if (path === `/admin/agents/findings/${AGENT_FINDING}` && verified) {
        return {
          body: {
            ...ADMIN_AGENT_FINDING,
            finding: { ...ADMIN_AGENT_FINDING.finding, status: "acknowledged" },
          },
          status: 200,
        };
      }
      if (
        path === `/admin/agents/findings/${AGENT_FINDING}` &&
        moved &&
        refresh === "unavailable"
      ) {
        return { body: { error: "finding unavailable" }, status: 503 };
      }
      return undefined;
    });
    await mountAt("/admin/agents", { finding: AGENT_FINDING });
    await waitFor(has("我來處理"));
    await type("#admin-finding-note", "checking the job");
    await submit("#admin-finding-note");
    await waitFor(has("已送出改為「處理中」；最新狀態尚未確認。"));
    expect(field<HTMLElement>("#admin-finding-result").classList.contains("notice-warning")).toBe(
      true,
    );
    expect(has("我來處理")()).toBe(false);
    expect(has("標記已解決")()).toBe(false);
    expect(container.querySelector("#admin-finding-note")).toBeNull();
    expect(document.activeElement).toBe(field<HTMLElement>("#admin-finding-result"));
    if (refresh === "unavailable") {
      await waitFor(has("暫時無法讀取這件事。請再試一次，或稍後再試。"));
      expect(button("再試一次")).toBeDefined();
      expect(field<HTMLElement>("#admin-finding-result").textContent).toContain(
        ADMIN_AGENT_FINDING.finding.title,
      );
      verified = true;
      await click(button("再試一次"));
      await waitFor(has("已改成「處理中」。"));
      expect(button("標記已解決")).toBeDefined();
    } else {
      expect(button("重新整理這件事")).toBeDefined();
      verified = true;
      await click(button("重新整理這件事"));
      await waitFor(has("已改成「處理中」。"));
      expect(field<HTMLElement>("#admin-finding-result").classList.contains("notice-success")).toBe(
        true,
      );
      expect(button("標記已解決")).toBeDefined();
    }
  },
);

test("a cached next finding does not inherit the previous finding's success message", async () => {
  const nextId = "8e2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5b";
  stub(true, (path, method) =>
    method === "PUT"
      ? { body: {}, status: 204 }
      : path === `/admin/agents/findings/${nextId}`
        ? {
            body: {
              finding: { ...ADMIN_AGENT_FINDING.finding, id: nextId, title: "下一件待辦" },
              events: ADMIN_AGENT_FINDING.events,
            },
            status: 200,
          }
        : undefined,
  );
  queryClient.setQueryData(queryKeys.admin.agentFinding(nextId), {
    finding: { ...ADMIN_AGENT_FINDING.finding, id: nextId, title: "下一件待辦" },
    events: ADMIN_AGENT_FINDING.events,
  });
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", "checking the job");
  await submit("#admin-finding-note");
  await waitFor(has("已送出改為「處理中」；最新狀態尚未確認。"));
  await go("/admin/agents", { finding: nextId });
  await waitFor(has("下一件待辦"));
  expect(has("已送出改為「處理中」")()).toBe(false);
});

test("OPS-012: changing the finding move clears a reason written for another move", async () => {
  stub(true);
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", "I will investigate");
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="dismissed"]'));
  expect(field<HTMLTextAreaElement>("#admin-finding-note").value).toBe("");
  expect(button("忽略這件事").disabled).toBe(true);
});

test("OPS-012: a changed finding status clears a reason written for the earlier status", async () => {
  let acknowledged = false;
  stub(true, (path) =>
    path === `/admin/agents/findings/${AGENT_FINDING}`
      ? {
          body: {
            finding: {
              ...ADMIN_AGENT_FINDINGS.findings[0],
              status: acknowledged ? "acknowledged" : "open",
            },
            events: [],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="resolved"]'));
  await type("#admin-finding-note", "fixed in the earlier status");

  acknowledged = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.agentFinding(AGENT_FINDING) });
  });
  await waitFor(has("處理中"));
  expect(field<HTMLTextAreaElement>("#admin-finding-note").value).toBe("");
  expect(button("標記已解決").disabled).toBe(true);
});

test("OPS-012: a refused move explains the changed state without raw server text", async () => {
  stub(true, (_path, method) =>
    method === "PUT"
      ? {
          body: { error: "the finding cannot move to that status from where it is now" },
          status: 409,
        }
      : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("標記已解決"));
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="resolved"]'));
  await type("#admin-finding-note", "fixed");
  await submit("#admin-finding-note");
  await waitFor(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。"));
  expect(has("the finding cannot move to that status from where it is now")()).toBe(false);
  expect(calls.find((call) => call.method === "PUT")?.body).toEqual({
    status: "resolved",
    note: "fixed",
  });
});

test("OPS-013: the waiting proposals show what each would run, its tier and why", async () => {
  stub(true);
  await mountAt("/admin/agents");
  await waitFor(has("立刻補跑「輪替分割表」"));
  expect(has("待核准")()).toBe(true);
  expect(has("破壞性")()).toBe(true);
  expect(has("分割表輪替從來沒有成功過，建議現在補跑一次。")()).toBe(true);
  expect(calls.some((c) => c.url === "/admin/agents/proposals?view=proposed&offset=0")).toBe(true);
});

test("the proposal queue reports all waiting decisions and lets an operator reach the next page", async () => {
  const firstPage = Array.from({ length: 20 }, (_, index) => ({
    ...ADMIN_AGENT_PROPOSALS.proposals[0],
    id: `00000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
  }));
  const remaining = {
    ...ADMIN_AGENT_PROPOSALS.proposals[0],
    id: "8e2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5b",
    reason: "第二頁仍有待核准提案。",
  };
  const secondPage = [
    remaining,
    ...Array.from({ length: 19 }, (_, index) => ({
      ...ADMIN_AGENT_PROPOSALS.proposals[0],
      id: `00000000-0000-4000-8001-${String(index).padStart(12, "0")}`,
    })),
  ];
  stub(true, (path, _method, url) => {
    if (path !== "/admin/agents/proposals") return undefined;
    if (url.includes("view=proposed&offset=20")) {
      return { body: { proposals: secondPage, total: 101 }, status: 200 };
    }
    if (url.includes("view=proposed")) {
      return { body: { proposals: firstPage, total: 101 }, status: 200 };
    }
    if (url.includes("view=closed")) {
      return {
        body: { proposals: [{ ...remaining, status: "expired" }], total: 1 },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("待核准 101 件"));
  expect(has("共 101 件")()).toBe(true);
  await click(button("下一頁"));
  await waitFor(has("第二頁仍有待核准提案。"));
  expect(calls.some((c) => c.url === "/admin/agents/proposals?view=proposed&offset=20")).toBe(true);
  await type("#admin-proposal-view", "closed");
  await waitFor(has("最近七天結案：共 1 件"));
  expect(has("待核准 101 件")()).toBe(true);
});

test.each([
  {
    name: "valid queue position",
    search: { proposal_view: "closed", proposal_offset: "20" },
    url: "/admin/agents/proposals?view=closed&offset=20",
  },
  {
    name: "invalid queue position",
    search: { proposal_view: "unknown", proposal_offset: "-1" },
    url: "/admin/agents/proposals?view=proposed&offset=0",
  },
])("a $name in the address selects the correct proposal page", async ({ search, url }) => {
  stub(true);
  await mountAt("/admin/agents", search);
  await waitFor(() => calls.some((call) => call.url === url));
});

test("returning from a proposal preserves its queue view and page", async () => {
  stub(true);
  await mountAt("/admin/agents", {
    proposal: AGENT_PROPOSAL,
    proposal_view: "closed",
    proposal_offset: "20",
  });
  await waitFor(has("要刪除的 Trace 分割表：1 筆"));
  const back = Array.from(container.querySelectorAll<HTMLAnchorElement>("a")).find(
    (link) => link.textContent?.trim() === "回到提案",
  );
  expect(back).toBeDefined();
  await click(back!);
  await waitFor(() =>
    calls.some((call) => call.url === "/admin/agents/proposals?view=closed&offset=20"),
  );
  expect(router.state.location.search).toMatchObject({
    proposal_view: "closed",
    proposal_offset: 20,
  });
  expect(router.state.location.search.proposal).toBeUndefined();
});

test.each([
  { name: "no total", body: { proposals: ADMIN_AGENT_PROPOSALS.proposals } },
  { name: "a negative total", body: { proposals: ADMIN_AGENT_PROPOSALS.proposals, total: -1 } },
  {
    name: "fewer total items than shown",
    body: { proposals: ADMIN_AGENT_PROPOSALS.proposals, total: 0 },
  },
])("a proposal response with $name cannot become a decision claim", async ({ body }) => {
  stub(true, (path) => (path === "/admin/agents/proposals" ? { body, status: 200 } : undefined));
  await mountAt("/admin/agents");
  await waitFor(has("待核准 無法取得"));
  expect(has("打開這個提案")()).toBe(false);
});

test("OPS-013: a proposal opens with what would happen and the facts it rests on", async () => {
  stub(true);
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("要刪除的 Trace 分割表：1 筆"));
  expect(has("會刪除的 Trace 事件：1834 筆")()).toBe(true);
  expect(has("/maintenance_jobs/rotate-partitions/overdue_ratio")()).toBe(true);
  expect(button("核准並執行")).toBeDefined();
  expect(button("駁回")).toBeDefined();
});

test("a proposal opens the run that supplied its evidence before a decision", async () => {
  stub(true);
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("查看提出這個提案的執行"));
  const source = Array.from(container.querySelectorAll("a")).find(
    (link) => link.textContent === "查看提出這個提案的執行",
  )!;
  await click(source);
  await waitFor(has("這次執行"));
  expect(router.state.location.search).toMatchObject({
    run: "3a4b5c6d-7e8f-4a9b-8c0d-1e2f3a4b5c6d",
  });
  expect(router.state.location.search.proposal).toBeUndefined();
});

test("a proposal whose refreshed preview cannot be read cannot be decided from cached facts", async () => {
  let unreadable = false;
  stub(true, (path) =>
    unreadable && path === `/admin/agents/proposals/${AGENT_PROPOSAL}`
      ? { body: { error: "proposal unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("要刪除的 Trace 分割表：1 筆"));
  unreadable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.agentProposal(AGENT_PROPOSAL) });
  });
  await waitFor(() => Boolean(container.querySelector('[role="alert"]')));
  expect(has("要刪除的 Trace 分割表：1 筆")()).toBe(false);
  expect(has("核准並執行")()).toBe(false);
  expect(has("駁回")()).toBe(false);
});

test.each(["approve", "reject"])(
  "OPS-013: deciding %s sends that decision with the operator's note",
  async (decision) => {
    stub(true, (_path, method) => (method === "PUT" ? { body: {}, status: 204 } : undefined));
    await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
    await waitFor(has("核准並執行"));
    await type(`#admin-proposal-${decision}-note`, " the job never ran ");
    if (decision === "approve") {
      await submit(`#admin-proposal-${decision}-note`);
      expect(calls.some((c) => c.method === "PUT")).toBe(false);
      await click(button("核准並執行"));
      expect(has("會刪除的 Trace 事件：1834 筆")()).toBe(true);
      expect(has("其他待審提案不受影響")()).toBe(true);
      expect(calls.some((c) => c.method === "PUT")).toBe(false);
      await click(button("確認核准這個提案"));
    } else {
      await submit(`#admin-proposal-${decision}-note`);
    }
    await waitFor(() => calls.some((c) => c.method === "PUT"));
    expect(calls.find((c) => c.method === "PUT")).toEqual({
      method: "PUT",
      url: `/admin/agents/proposals/${AGENT_PROPOSAL}/decision`,
      body: { decision, note: "the job never ran" },
    });
  },
);

test("a completed proposal decision focuses its result after the decision forms disappear", async () => {
  let approved = false;
  stub(true, (path, method) => {
    if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}/decision` && method === "PUT") {
      approved = true;
      return { body: {}, status: 204 };
    }
    if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}`) {
      return {
        body: {
          ...ADMIN_AGENT_PROPOSAL,
          status: approved ? "approved" : "proposed",
        },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("核准並執行"));
  await type("#admin-proposal-approve-note", "job needs attention");
  await click(button("核准並執行"));
  await click(button("確認核准這個提案"));
  await waitFor(has("已核准。"));
  expect(field<HTMLElement>("#admin-proposal-result").textContent).toBe(
    "立刻補跑「輪替分割表」：已核准。",
  );
  expect(has("核准並執行")()).toBe(false);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-proposal-result"));
});

test.each(["stale", "failed"] as const)(
  "a completed proposal decision stays confirmed and cannot be repeated after a %s detail refresh",
  async (refresh) => {
    let decided = false;
    stub(true, (path, method) => {
      if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}/decision` && method === "PUT") {
        decided = true;
        return { body: {}, status: 204 };
      }
      if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}`)
        return decided && refresh === "failed"
          ? { body: { error: "read unavailable" }, status: 503 }
          : { body: ADMIN_AGENT_PROPOSAL, status: 200 };
      return undefined;
    });
    await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
    await waitFor(has("駁回"));
    await type("#admin-proposal-reject-note", "not needed");
    await submit("#admin-proposal-reject-note");

    await waitFor(has("已駁回。"));
    expect(document.activeElement).toBe(field<HTMLElement>("#admin-proposal-result"));
    expect(container.querySelector("#admin-proposal-reject-note")).toBeNull();
    expect(container.querySelector("#admin-proposal-approve-note")).toBeNull();
    if (refresh === "failed") expect(has("暫時無法讀取這個提案")()).toBe(true);
  },
);

test("a destructive proposal can be cancelled after reviewing its scope", async () => {
  stub(true);
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("核准並執行"));
  expect(button("核准並執行").disabled).toBe(true);
  await type("#admin-proposal-approve-note", "need to run maintenance");
  await click(button("核准並執行"));
  expect(has("此頁沒有復原功能")()).toBe(true);
  await click(button("取消"));
  expect(button("核准並執行")).toBeDefined();
  expect(has("確認核准這個提案")()).toBe(false);
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
});

test("a successful rejection does not label an earlier failed approval as successful", async () => {
  let decisions = 0;
  stub(true, (_path, method) => {
    if (method !== "PUT") return undefined;
    decisions += 1;
    return decisions === 1
      ? { body: { error: "the proposal is no longer waiting for a decision" }, status: 409 }
      : { body: {}, status: 200 };
  });
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("核准並執行"));
  await type("#admin-proposal-approve-note", "first decision");
  await click(button("核准並執行"));
  await click(button("確認核准這個提案"));
  await waitFor(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。"));
  expect(has("the proposal is no longer waiting for a decision")()).toBe(false);
  await type("#admin-proposal-reject-note", "second decision");
  await submit("#admin-proposal-reject-note");
  await waitFor(has("已駁回。"));
  expect(has("已核准。")()).toBe(false);
});

test("a successful finding move does not label an earlier failed move as successful", async () => {
  let moves = 0;
  stub(true, (_path, method) => {
    if (method !== "PUT") return undefined;
    moves += 1;
    return moves === 1
      ? { body: { error: "the finding cannot move to that status" }, status: 409 }
      : { body: {}, status: 200 };
  });
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", "first move");
  await submit("#admin-finding-note");
  await waitFor(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。"));
  expect(has("the finding cannot move to that status")()).toBe(false);
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="resolved"]'));
  await type("#admin-finding-note", "second move");
  await submit("#admin-finding-note");
  await waitFor(has("已送出改為「已解決」；最新狀態尚未確認。"));
  expect(has("已送出改為「處理中」")()).toBe(false);
});

test("OPS-013: a decided proposal shows its decision and outcome and offers no decision", async () => {
  const done = {
    ...ADMIN_AGENT_PROPOSAL,
    status: "failed",
    decided_by_user_id: "0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e",
    decided_at: "2026-10-08T03:00:00Z",
    decision_note: "go ahead",
    started_at: "2026-10-08T03:05:00Z",
    finished_at: "2026-10-08T03:06:00Z",
    outcome: "TRACE_RETENTION must be a positive Go duration",
  };
  stub(true, (path) =>
    path === `/admin/agents/proposals/${AGENT_PROPOSAL}` ? { body: done, status: 200 } : undefined,
  );
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("營運者核准：go ahead"));
  expect(has("執行失敗：TRACE_RETENTION must be a positive Go duration")()).toBe(true);
  expect(has("核准並執行")()).toBe(false);
  expect(has("駁回")()).toBe(false);
});

test("OPS-013: a proposal in the address that is not a UUID falls back to the list", async () => {
  stub(true);
  await mountAt("/admin/agents", { proposal: "rotate" });
  await waitFor(has("打開這個提案"));
  expect(calls.some((c) => c.url.startsWith("/admin/agents/proposals/"))).toBe(false);
});
