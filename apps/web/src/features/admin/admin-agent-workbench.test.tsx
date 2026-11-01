import { act } from "react";
import { expect, test } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import {
  ADMIN_AGENT_FINDINGS,
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_RUNS,
  ADMIN_AGENT_STEPS,
  ADMIN_AGENTS,
  AGENT_FAILED_RUN,
  AGENT_FINDING,
  AGENT_PROPOSAL,
  AGENT_REPORT_RUN,
  ADMIN_SKILLS,
  SKILL,
} from "../../testing/fixtures/platform";
import {
  container,
  calls,
  router,
  stub,
  mountAt,
  waitFor,
  has,
  field,
  type,
  button,
  click,
  submit,
} from "./testing/harness";

test("the agent workbench names pending decisions, live findings, running work and brake state", async () => {
  stub(true);
  await mountAt("/admin/agents");
  await waitFor(() =>
    Boolean(
      container
        .querySelector('nav[aria-label="平台 Agent 工作區"]')
        ?.textContent?.includes("待核准 1 件"),
    ),
  );
  const summary = container.querySelector('nav[aria-label="平台 Agent 工作區"]');
  expect(summary?.textContent).toContain("待辦 2 件");
  expect(summary?.textContent).toContain("執行中 0 次");
  expect(summary?.textContent).toContain("煞車 已放開");
  expect(summary?.querySelectorAll('a[href^="#"]')).toHaveLength(4);
});

test("OPS-011: the emergency brake is reachable before the decision lists", async () => {
  stub(true);
  await mountAt("/admin/agents");
  await waitFor(has("拉下 Agent 煞車"));
  const brake = field<HTMLElement>("#admin-agent-brake");
  const proposals = field<HTMLElement>("#admin-agent-proposals");
  expect(brake.compareDocumentPosition(proposals) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(
    field<HTMLElement>('nav[aria-label="平台 Agent 工作區"] a[href="#admin-agent-brake"]'),
  ).toBeDefined();
});

test("OPS-011: an Agent shows what it may propose and its model role before enabling", async () => {
  stub(true, (path) =>
    path === "/admin/agents"
      ? {
          body: {
            agents: [
              {
                ...ADMIN_AGENTS.agents[0],
                enabled: false,
                actions: ["run-purge-audit"],
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("啟用 daily-report"));
  const controls = field<HTMLElement>("#admin-agent-controls");
  expect(controls.textContent).toContain("可提案：立刻補跑「清除過了保存期的稽核紀錄」");
  expect(controls.textContent).toContain("模型角色：skillhub-ops-report");
  expect(controls.textContent).toContain("負責營運者：22222222-2222-2222-2222-222222222222");
});

test("an Agent without a recorded owner does not appear assigned", async () => {
  stub(true, (path) =>
    path === "/admin/agents"
      ? {
          body: {
            agents: [{ ...ADMIN_AGENTS.agents[0], owner_user_id: undefined }],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("負責營運者：未記錄"));
  expect(has("負責營運者：22222222-2222-2222-2222-222222222222")()).toBe(false);
});

test.each([
  { kind: "提案", path: "/admin/agents/proposals", section: "#admin-agent-proposals" },
  { kind: "待辦", path: "/admin/agents/findings", section: "#admin-agent-findings" },
])(
  "$kind list discloses its freshness and can refresh the workbench count",
  async ({ path, section }) => {
    let refreshed = false;
    stub(true, (requestPath) => {
      if (requestPath !== path || !refreshed) return undefined;
      return path.endsWith("proposals")
        ? { body: { proposals: [], total: 0 }, status: 200 }
        : {
            body: {
              findings: [],
              counts: { open: 0, acknowledged: 0, resolved: 0, dismissed: 0, recovered: 0 },
            },
            status: 200,
          };
    });
    await mountAt("/admin/agents");
    const sectionNode = field<HTMLElement>(section);
    await waitFor(() => Boolean(sectionNode.querySelector(".download-item")));
    expect(sectionNode.textContent).toContain(
      `${path.endsWith("proposals") ? "提案" : "待辦"}清單上次取得於`,
    );
    refreshed = true;
    await click(sectionNode.querySelector<HTMLButtonElement>("button")!);
    await waitFor(() => !sectionNode.querySelector(".download-item"));
    expect(sectionNode.textContent).toContain("0 件");
    expect(calls.filter((c) => c.url.startsWith(path)).length).toBeGreaterThan(1);
  },
);

test("the agent workbench does not report zero decisions when proposals cannot be read", async () => {
  stub(true, (path) =>
    path === "/admin/agents/proposals"
      ? { body: { error: "proposals unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(() =>
    Boolean(
      container
        .querySelector('nav[aria-label="平台 Agent 工作區"]')
        ?.textContent?.includes("待核准 無法取得"),
    ),
  );
  const summary = container.querySelector('nav[aria-label="平台 Agent 工作區"]');
  expect(summary?.textContent).not.toContain("待核准 0 件");
});

test.each([
  { path: "/admin/agents/proposals", key: queryKeys.admin.agentProposalList, row: "打開這個提案" },
  {
    path: "/admin/agents/findings",
    key: queryKeys.admin.agentFindingList("live"),
    row: "打開這件事",
  },
])("the $path list hides cached rows when refreshing fails", async ({ path, key, row }) => {
  let unreadable = false;
  stub(true, (requestPath) =>
    unreadable && requestPath === path
      ? { body: { error: "list unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has(row));
  unreadable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: key });
  });
  await waitFor(has(path.endsWith("proposals") ? "暫時無法讀取提案" : "暫時無法讀取待辦"));
  expect(has(row)()).toBe(false);
});

const focusedAgentCases: { search: Record<string, string>; heading: string }[] = [
  { search: { proposal: AGENT_PROPOSAL }, heading: "這個提案" },
  { search: { finding: AGENT_FINDING }, heading: "這件事" },
  { search: { run: AGENT_REPORT_RUN }, heading: "這次執行" },
];

const agentRetryCases: {
  name: string;
  path: string;
  search?: Record<string, string>;
  failed: string;
  recovered: string;
  focus: string;
}[] = [
  {
    name: "Agent control",
    path: "/admin/agents",
    failed: " Agent 控制",
    recovered: "拉下 Agent 煞車",
    focus: "admin-agent-brake-heading",
  },
  {
    name: "proposal list",
    path: "/admin/agents/proposals",
    failed: "提案",
    recovered: "打開這個提案",
    focus: "admin-agent-proposals-heading",
  },
  {
    name: "finding list",
    path: "/admin/agents/findings",
    failed: "待辦",
    recovered: "打開這件事",
    focus: "admin-agent-findings-heading",
  },
  {
    name: "run list",
    path: "/admin/agents/runs",
    failed: "執行紀錄",
    recovered: "看這次的步驟",
    focus: "admin-agent-runs-heading",
  },
  {
    name: "proposal detail",
    path: `/admin/agents/proposals/${AGENT_PROPOSAL}`,
    search: { proposal: AGENT_PROPOSAL },
    failed: "這個提案",
    recovered: "會發生什麼",
    focus: "admin-proposal-heading",
  },
  {
    name: "finding detail",
    path: `/admin/agents/findings/${AGENT_FINDING}`,
    search: { finding: AGENT_FINDING },
    failed: "這件事",
    recovered: "依據",
    focus: "admin-finding-heading",
  },
  {
    name: "run detail",
    path: `/admin/agents/runs/${AGENT_REPORT_RUN}`,
    search: { run: AGENT_REPORT_RUN },
    failed: "執行紀錄",
    recovered: "需要注意：1 項",
    focus: "admin-agent-run-heading",
  },
  {
    name: "run steps",
    path: `/admin/agents/runs/${AGENT_REPORT_RUN}/steps`,
    search: { run: AGENT_REPORT_RUN },
    failed: "執行步驟",
    recovered: "呼叫 maintenance_report",
    focus: "admin-agent-steps-heading",
  },
];

test.each(agentRetryCases)(
  "$name can retry a failed read in place",
  async ({ path, search, failed, recovered, focus }) => {
    let unavailable = true;
    stub(true, (requestPath) =>
      unavailable && requestPath === path
        ? { body: { error: "service unavailable" }, status: 503 }
        : undefined,
    );
    await mountAt("/admin/agents", search);
    await waitFor(has(`暫時無法讀取${failed}`));
    expect(has(recovered)()).toBe(false);
    unavailable = false;
    await click(button("再試一次"));
    expect(document.activeElement).toBe(field<HTMLElement>(`#${focus}`));
    expect(calls.filter((call) => call.url.split("?")[0] === path)).toHaveLength(2);
    await waitFor(has(recovered));
    expect(has(`暫時無法讀取${failed}`)()).toBe(false);
    expect(calls.filter((call) => call.url.split("?")[0] === path)).toHaveLength(2);
  },
);

test("an unauthenticated Agent detail offers sign-in instead of retrying", async () => {
  stub(true, (path) =>
    path === `/admin/agents/proposals/${AGENT_PROPOSAL}`
      ? { body: { error: "not authenticated" }, status: 401 }
      : undefined,
  );
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("這個提案需要登入。"));
  expect(has("暫時無法讀取這個提案")()).toBe(false);
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "再試一次",
    ),
  ).toBe(false);
});

test.each(focusedAgentCases)(
  "opening $heading focuses that object instead of the whole workbench",
  async ({ search, heading }) => {
    stub(true);
    await mountAt("/admin/agents", search);
    await waitFor(has(heading));
    expect(container.querySelector('nav[aria-label="平台 Agent 工作區"]')).toBeNull();
    expect(container.querySelector("#admin-agent-controls")).toBeNull();
    expect(container.querySelector("#admin-agent-proposals")).toBeNull();
    expect(container.querySelector("#admin-agent-findings")).toBeNull();
  },
);

test("OPS-012: a run in the address shows its report, then each step's tool, answer, tokens and cost", async () => {
  stub(true);
  await mountAt("/admin/agents", { run: AGENT_REPORT_RUN });
  await waitFor(has("需要注意：1 項"));
  expect(has("正常：1 項")()).toBe(true);
  await waitFor(has("呼叫 maintenance_report"));
  expect(has("交出結果")()).toBe(true);
  expect(has("輸入 812 tokens、輸出 14 tokens；花費 $0.0012")()).toBe(true);
  expect(has("回到執行紀錄")()).toBe(true);
  expect(has("看這次的步驟")()).toBe(false);
  const rawSteps = container.querySelectorAll(".agent-step-raw");
  expect(rawSteps).toHaveLength(2);
  expect([...rawSteps].every((step) => !(step as HTMLDetailsElement).open)).toBe(true);
});

test("OPS-012: a linked run still shows its report after it leaves the recent-50 list", async () => {
  stub(true, (path) =>
    path === "/admin/agents/runs" ? { body: { runs: [], total: 73 }, status: 200 } : undefined,
  );
  await mountAt("/admin/agents", { run: AGENT_REPORT_RUN });
  await waitFor(has("需要注意：1 項"));
  expect(has("分割表輪替從來沒有成功過")()).toBe(true);
  expect(has("回到執行紀錄")()).toBe(true);
});

test("OPS-012: a failed run's report is shown as not checked, and its list row names why and what it cost", async () => {
  stub(true);
  await mountAt("/admin/agents", { run: AGENT_FAILED_RUN });
  await waitFor(has("這份日報沒有通過核對"));
  const warning = field<HTMLElement>(".notice-warning[role='status']");
  expect(warning.textContent).toContain("不是平台確認的維運事實，也沒有進待辦");
  expect(
    Array.from(container.querySelectorAll("h3")).map((heading) => heading.textContent),
  ).toContain("未核對的原稿 · 需要注意：1 項");
  expect(has("which no tool returned")()).toBe(true);
  expect(has("2 步；花費 $0.0018（另有 1 步沒有回報花費）")()).toBe(true);
  expect(has("2 步；花費 $0.0031")()).toBe(false);
});

test.each([
  {
    param: "run",
    collection: "runs",
    id: AGENT_REPORT_RUN,
    what: "執行紀錄",
    status: 404,
    retryable: false,
  },
  {
    param: "run",
    collection: "runs",
    id: AGENT_REPORT_RUN,
    what: "執行紀錄",
    status: 400,
    retryable: false,
  },
  {
    param: "run",
    collection: "runs",
    id: AGENT_REPORT_RUN,
    what: "執行紀錄",
    status: 503,
    retryable: true,
  },
  {
    param: "finding",
    collection: "findings",
    id: AGENT_FINDING,
    what: "這件事",
    status: 404,
    retryable: false,
  },
  {
    param: "finding",
    collection: "findings",
    id: AGENT_FINDING,
    what: "這件事",
    status: 503,
    retryable: true,
  },
  {
    param: "proposal",
    collection: "proposals",
    id: AGENT_PROPOSAL,
    what: "這個提案",
    status: 404,
    retryable: false,
  },
  {
    param: "proposal",
    collection: "proposals",
    id: AGENT_PROPOSAL,
    what: "這個提案",
    status: 503,
    retryable: true,
  },
])(
  "OPS-012: a $param detail answering $status offers a retry only when rereading can help",
  async ({ param, collection, id, what, status, retryable }) => {
    stub(true, (path) =>
      path === `/admin/agents/${collection}/${id}`
        ? { body: { error: "not found" }, status }
        : undefined,
    );
    await mountAt("/admin/agents", { [param]: id });
    await waitFor(() => Boolean(container.querySelector('[role="alert"]')));
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      retryable
        ? `暫時無法讀取${what}。請再試一次，或稍後再試。`
        : `找不到${what}，重新讀取也不會出現。`,
    );
    expect(
      Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "再試一次"),
    ).toBe(retryable);
  },
);

test("OPS-012: a truncated Agent run list names its exact total and display limit", async () => {
  stub(true, (path) =>
    path === "/admin/agents/runs"
      ? { body: { runs: ADMIN_AGENT_RUNS.runs, total: 73 }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("看這次的步驟"));
  expect(field<HTMLElement>("#admin-agent-runs").textContent).toContain(
    "共 73 次；目前顯示最近 2 次，這份清單最多顯示 50 次",
  );
});

test.each([
  { status: "running", label: "執行中", tone: "badge" },
  { status: "completed", label: "完成", tone: "badge" },
  { status: "incomplete", label: "未完成", tone: "badge-warning" },
  { status: "stopped", label: "已停止", tone: "badge-warning" },
  { status: "failed", label: "失敗", tone: "badge-danger" },
])("agent run $status uses the $tone status tone", async ({ status, label, tone }) => {
  stub(true, (path) =>
    path === `/admin/agents/runs/${AGENT_REPORT_RUN}`
      ? { body: { ...ADMIN_AGENT_RUNS.runs[1], status }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/agents", { run: AGENT_REPORT_RUN });
  await waitFor(has(label));
  const badge = field<HTMLElement>('section[aria-labelledby="admin-agent-run-heading"] .badge');
  expect(badge.className).toBe(tone === "badge" ? "badge" : `badge ${tone}`);
});

test("a running agent says what progressed, when it last acted, and that leaving is safe", async () => {
  stub(true, (path) =>
    path === `/admin/agents/runs/${AGENT_REPORT_RUN}`
      ? {
          body: { ...ADMIN_AGENT_RUNS.runs[1], status: "running", steps: 1, result: undefined },
          status: 200,
        }
      : path === `/admin/agents/runs/${AGENT_REPORT_RUN}/steps`
        ? { body: { steps: ADMIN_AGENT_STEPS.steps.slice(0, 1) }, status: 200 }
        : undefined,
  );
  await mountAt("/admin/agents", { run: AGENT_REPORT_RUN });
  await waitFor(has("仍在執行；已記錄 1 步"));
  expect(has("可以離開這頁")()).toBe(true);
  expect(has("每 3 秒自動更新")()).toBe(true);
  await waitFor(has("最近一步"));
  expect(has("2026/10/06")()).toBe(true);
});

test.each([
  { last: "2026-10-07T01:02:00Z", expected: "最近一步記錄於", missing: "尚未記錄第一步" },
  { last: undefined, expected: "尚未記錄第一步", missing: "最近一步記錄於" },
])(
  "a running row uses recorded activity, not list freshness, when last step is $last",
  async ({ last, expected, missing }) => {
    stub(true, (path) =>
      path === "/admin/agents/runs"
        ? {
            body: {
              runs: [
                {
                  ...ADMIN_AGENT_RUNS.runs[1],
                  status: "running",
                  steps: last ? 2 : 0,
                  last_step_at: last,
                },
              ],
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/agents");
    await waitFor(has("看這次的步驟"));
    expect(field<HTMLElement>("#admin-agent-runs").textContent).toContain("每 3 秒自動更新");
    const row = field<HTMLElement>("#admin-agent-runs .download-item");
    expect(row.textContent).toContain(expected);
    expect(row.textContent).not.toContain(missing);
  },
);

test("a running agent refreshes its steps and stops reporting progress after completion", async () => {
  let progressed = false;
  let completed = false;
  stub(true, (path) => {
    if (path === `/admin/agents/runs/${AGENT_REPORT_RUN}`) {
      return {
        body: {
          ...ADMIN_AGENT_RUNS.runs[1],
          status: completed ? "completed" : "running",
          steps: progressed ? 2 : 1,
          result: completed ? ADMIN_AGENT_RUNS.runs[1].result : undefined,
        },
        status: 200,
      };
    }
    if (path === `/admin/agents/runs/${AGENT_REPORT_RUN}/steps`) {
      return {
        body: { steps: progressed ? ADMIN_AGENT_STEPS.steps : ADMIN_AGENT_STEPS.steps.slice(0, 1) },
        status: 200,
      };
    }
    return undefined;
  });
  await mountAt("/admin/agents", { run: AGENT_REPORT_RUN });
  await waitFor(has("仍在執行；已記錄 1 步"));
  expect(has("交出結果")()).toBe(false);
  progressed = true;
  await waitFor(has("交出結果"), 7000);
  completed = true;
  await waitFor(has("完成。每一項都附它根據的事實"), 7000);
  expect(has("仍在執行；已記錄")()).toBe(false);
  expect(has("每 3 秒自動更新")()).toBe(false);
});

test("OPS-012: ids in the address that are not UUIDs are dropped instead of fetched", async () => {
  stub(true);
  await mountAt("/admin/agents", { run: "not-a-run", finding: "nope" });
  await waitFor(has("日報與執行紀錄"));
  await waitFor(has("回報 3 次"));
  expect(has("這次執行")()).toBe(false);
  expect(
    calls.some((c) => c.url.includes("/runs/not-a-run") || c.url.includes("/findings/nope")),
  ).toBe(false);
});

test("OPS-011: disabling an agent and engaging the brake each send the operator's note", async () => {
  stub(true, (_path, method) => (method === "GET" ? undefined : { body: {}, status: 200 }));
  await mountAt("/admin/agents");
  await waitFor(has("停用 daily-report"));
  await type("#admin-agent-daily-report-note", "  rotating keys  ");
  await submit("#admin-agent-daily-report-note");
  await waitFor(() => calls.some((c) => c.method === "PUT" && c.url.endsWith("/enabled")));
  expect(calls.find((c) => c.method === "PUT" && c.url.endsWith("/enabled"))).toEqual({
    method: "PUT",
    url: "/admin/agents/daily-report/enabled",
    body: { enabled: false, note: "rotating keys" },
  });
  await type("#admin-agent-brake-engage-note", "incident");
  await submit("#admin-agent-brake-engage-note");
  await waitFor(() => calls.some((c) => c.url === "/admin/agents/brake"));
  expect(calls.find((c) => c.url === "/admin/agents/brake")).toEqual({
    method: "PUT",
    url: "/admin/agents/brake",
    body: { note: "incident" },
  });
});

test("OPS-011: an engaged brake shows its reason and offers only the release", async () => {
  stub(true, (path) =>
    path === "/admin/agents"
      ? {
          body: {
            ...ADMIN_AGENTS,
            brake: { reason: "gateway incident", engaged_at: "2026-10-07T01:00:00Z" },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("煞車拉下：所有 Agent 停止"));
  expect(has("理由：gateway incident")()).toBe(true);
  expect(button("放開 Agent 煞車")).toBeDefined();
  expect(has("拉下 Agent 煞車")()).toBe(false);
});

test("OPS-012: after a finding moves, the sentence stays up although the refetched finding shows the new status", async () => {
  let moved = false;
  stub(true, (path, method) => {
    if (method === "PUT") {
      moved = true;
      return { body: {}, status: 204 };
    }
    if (path === `/admin/agents/findings/${AGENT_FINDING}`)
      return {
        body: {
          finding: {
            ...ADMIN_AGENT_FINDINGS.findings[0],
            status: moved ? "acknowledged" : "open",
          },
          events: [],
        },
        status: 200,
      };
    return undefined;
  });
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("我來處理"));
  await type("#admin-finding-note", "checking");
  await submit("#admin-finding-note");
  await waitFor(has("已改成「"));
  await waitFor(() => !has("我來處理")());
  await type("#admin-finding-note", "next note");
  await waitFor(() => !has("已改成「")());
});

test.each([
  ["approve", "approved", "已核准。"],
  ["reject", "rejected", "已駁回。"],
])(
  "OPS-013: after %s the sentence stays up although the refetched proposal is no longer open",
  async (decision, status, sentence) => {
    let decided = false;
    stub(true, (path, method) => {
      if (method === "PUT") {
        decided = true;
        return { body: {}, status: 204 };
      }
      if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}`)
        return {
          body: { ...ADMIN_AGENT_PROPOSAL, status: decided ? status : "proposed" },
          status: 200,
        };
      return undefined;
    });
    await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
    await waitFor(has("核准並執行"));
    await type(`#admin-proposal-${decision}-note`, "reason");
    if (decision === "approve") {
      await click(button("核准並執行"));
      await click(button("確認核准這個提案"));
    } else {
      await submit(`#admin-proposal-${decision}-note`);
    }
    await waitFor(has(sentence));
    await waitFor(() => container.querySelector("#admin-proposal-approve-note") === null);
  },
);

const OTHER_ITEM = "8e2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5c";

async function openInPlace(search: Record<string, string>) {
  await act(async () => {
    await router.navigate({ to: "/admin/agents", search: search as never });
  });
}

test("OPS-013: a decision's sentence does not follow the operator straight to another proposal", async () => {
  let decided = false;
  stub(true, (path, method) => {
    if (method === "PUT") {
      decided = true;
      return { body: {}, status: 204 };
    }
    if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}`)
      return {
        body: { ...ADMIN_AGENT_PROPOSAL, status: decided ? "rejected" : "proposed" },
        status: 200,
      };
    if (path === `/admin/agents/proposals/${OTHER_ITEM}`)
      return {
        body: { ...ADMIN_AGENT_PROPOSAL, id: OTHER_ITEM, reason: "另一個提案" },
        status: 200,
      };
    return undefined;
  });
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("核准並執行"));
  await type("#admin-proposal-reject-note", "reason");
  await submit("#admin-proposal-reject-note");
  await waitFor(has("已駁回。"));
  await openInPlace({ proposal: OTHER_ITEM });
  await waitFor(has("另一個提案"));
  expect(has("已駁回。")()).toBe(false);
});

test("OPS-013: a refused decision refetches the proposal so the screen shows what the server now holds", async () => {
  let refused = false;
  stub(true, (path, method) => {
    if (method === "PUT") {
      refused = true;
      return { body: { error: "already decided" }, status: 409 };
    }
    if (path === `/admin/agents/proposals/${AGENT_PROPOSAL}`)
      return {
        body: { ...ADMIN_AGENT_PROPOSAL, status: refused ? "expired" : "proposed" },
        status: 200,
      };
    return undefined;
  });
  await mountAt("/admin/agents", { proposal: AGENT_PROPOSAL });
  await waitFor(has("核准並執行"));
  await type("#admin-proposal-approve-note", "reason");
  await click(button("核准並執行"));
  await click(button("確認核准這個提案"));
  await waitFor(() => container.querySelector("#admin-proposal-approve-note") === null);
  expect(
    calls.filter((c) => c.method === "GET" && c.url === `/admin/agents/proposals/${AGENT_PROPOSAL}`)
      .length,
  ).toBeGreaterThanOrEqual(2);
});

test("OPS-012: a refused move refetches the finding", async () => {
  stub(true, (_path, method) =>
    method === "PUT" ? { body: { error: "already moved" }, status: 409 } : undefined,
  );
  await mountAt("/admin/agents", { finding: AGENT_FINDING });
  await waitFor(has("標記已解決"));
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="resolved"]'));
  await type("#admin-finding-note", "fixed");
  await submit("#admin-finding-note");
  await waitFor(has("操作未被接受；目前狀態可能已變更，請重新整理後再判斷。"));
  expect(has("already moved")()).toBe(false);
  await waitFor(
    () =>
      calls.filter((c) => c.method === "GET" && c.url === `/admin/agents/findings/${AGENT_FINDING}`)
        .length >= 2,
  );
});

test.each([
  ["disable", true, "已停用，執行中的那一次會在下一步之前停下。"],
  ["enable", false, "已啟用，下一次排程會執行。"],
])(
  "OPS-011: after you %s an agent the sentence stays up although the refetched agent shows the flipped state",
  async (_name, enabledBefore, sentence) => {
    let flipped = false;
    stub(true, (path, method) => {
      if (method === "PUT") {
        flipped = true;
        return { body: {}, status: 200 };
      }
      if (path === "/admin/agents")
        return {
          body: {
            agents: [
              { ...ADMIN_AGENTS.agents[0], enabled: flipped ? !enabledBefore : enabledBefore },
            ],
          },
          status: 200,
        };
      return undefined;
    });
    await mountAt("/admin/agents");
    await waitFor(has(enabledBefore ? "停用 daily-report" : "啟用 daily-report"));
    await type("#admin-agent-daily-report-note", "because");
    await submit("#admin-agent-daily-report-note");
    await waitFor(has(sentence));
    await waitFor(has(enabledBefore ? "啟用 daily-report" : "停用 daily-report"));
  },
);

test.each([
  ["disable", "stale", true],
  ["disable", "unavailable", true],
  ["enable", "stale", false],
  ["enable", "unavailable", false],
] as const)(
  "OPS-011: a %s Agent command keeps its result but hides stale controls when status is %s",
  async (direction, refresh, initiallyEnabled) => {
    let sent = false;
    let verified = false;
    stub(true, (path, method) => {
      if (path === "/admin/agents/daily-report/enabled" && method === "PUT") {
        sent = true;
        return { body: {}, status: 200 };
      }
      if (path === "/admin/agents") {
        if (sent && !verified && refresh === "unavailable") {
          return { body: { error: "agent status unavailable" }, status: 503 };
        }
        return {
          body: {
            ...ADMIN_AGENTS,
            agents: [
              {
                ...ADMIN_AGENTS.agents[0],
                enabled: verified ? !initiallyEnabled : initiallyEnabled,
              },
            ],
          },
          status: 200,
        };
      }
      return undefined;
    });
    await mountAt("/admin/agents");
    await waitFor(has(direction === "disable" ? "停用 daily-report" : "啟用 daily-report"));
    await type("#admin-agent-daily-report-note", "because");
    await submit("#admin-agent-daily-report-note");
    await waitFor(
      has(
        direction === "disable"
          ? "已送出停用 daily-report；最新狀態尚未確認。"
          : "已送出啟用 daily-report；最新狀態尚未確認。",
      ),
    );
    expect(container.querySelector("#admin-agent-daily-report-note")).toBeNull();
    expect(document.activeElement).toBe(field<HTMLElement>("#admin-agent-daily-report-result"));
    if (refresh === "unavailable") {
      await waitFor(has("暫時無法讀取 Agent 控制。請重新整理，或稍後再試。"));
    }
    verified = true;
    await click(button("重新整理此 Agent"));
    await waitFor(
      has(
        direction === "disable"
          ? "已停用，執行中的那一次會在下一步之前停下。"
          : "已啟用，下一次排程會執行。",
      ),
    );
    expect(
      button(direction === "disable" ? "啟用 daily-report" : "停用 daily-report"),
    ).toBeDefined();
    expect(field<HTMLTextAreaElement>("#admin-agent-daily-report-note").value).toBe("");
  },
);

test("OPS-011: engaging the brake and releasing it each keep their sentence after the form swaps", async () => {
  let brake: object | undefined;
  stub(true, (path, method) => {
    if (path === "/admin/agents/brake") {
      brake =
        method === "PUT" ? { reason: "incident", engaged_at: "2026-10-07T01:00:00Z" } : undefined;
      return { body: {}, status: method === "PUT" ? 200 : 204 };
    }
    if (path === "/admin/agents") return { body: { ...ADMIN_AGENTS, brake }, status: 200 };
    return undefined;
  });
  await mountAt("/admin/agents");
  await waitFor(has("拉下 Agent 煞車"));
  await type("#admin-agent-brake-engage-note", "incident");
  await submit("#admin-agent-brake-engage-note");
  await waitFor(has("已拉下，所有 Agent 在下一步之前停下。"));
  await waitFor(has("放開 Agent 煞車"));

  await type("#admin-agent-brake-release-note", "over");
  await waitFor(() => !has("已拉下，")());
  await submit("#admin-agent-brake-release-note");
  await waitFor(has("已放開，啟用中的 Agent 下一次排程會執行。"));
  await waitFor(has("拉下 Agent 煞車"));
  expect(has("已拉下，")()).toBe(false);
});

test.each([
  ["engage", "stale"],
  ["engage", "unavailable"],
  ["release", "stale"],
  ["release", "unavailable"],
] as const)(
  "OPS-011: a %s brake command keeps its result but hides stale controls when status is %s",
  async (direction, refresh) => {
    const engaged = { reason: "incident", engaged_at: "2026-10-07T01:00:00Z" };
    let sent = false;
    let verified = false;
    stub(true, (path, method) => {
      if (path === "/admin/agents/brake" && method !== "GET") {
        sent = true;
        return { body: {}, status: method === "PUT" ? 200 : 204 };
      }
      if (path === "/admin/agents") {
        if (sent && !verified && refresh === "unavailable") {
          return { body: { error: "agent status unavailable" }, status: 503 };
        }
        return {
          body: {
            ...ADMIN_AGENTS,
            brake: verified
              ? direction === "engage"
                ? engaged
                : undefined
              : direction === "release"
                ? engaged
                : undefined,
          },
          status: 200,
        };
      }
      return undefined;
    });
    await mountAt("/admin/agents");
    await waitFor(has(direction === "engage" ? "拉下 Agent 煞車" : "放開 Agent 煞車"));
    await type(
      direction === "engage" ? "#admin-agent-brake-engage-note" : "#admin-agent-brake-release-note",
      "because",
    );
    await submit(
      direction === "engage" ? "#admin-agent-brake-engage-note" : "#admin-agent-brake-release-note",
    );
    await waitFor(
      has(
        direction === "engage"
          ? "已送出拉下煞車；最新狀態尚未確認。"
          : "已送出放開煞車；最新狀態尚未確認。",
      ),
    );
    expect(field<HTMLElement>('nav[aria-label="平台 Agent 工作區"]').textContent).toContain(
      "煞車 待確認",
    );
    expect(has("拉下 Agent 煞車")()).toBe(false);
    expect(has("放開 Agent 煞車")()).toBe(false);
    expect(document.activeElement).toBe(field<HTMLElement>("#admin-agent-brake-result"));
    if (refresh === "unavailable") {
      await waitFor(has("暫時無法讀取 Agent 控制。請重新整理，或稍後再試。"));
    }
    verified = true;
    await click(button("重新整理 Agent 狀態"));
    await waitFor(
      has(
        direction === "engage"
          ? "已拉下，所有 Agent 在下一步之前停下。"
          : "已放開，啟用中的 Agent 下一次排程會執行。",
      ),
    );
    expect(button(direction === "engage" ? "放開 Agent 煞車" : "拉下 Agent 煞車")).toBeDefined();
  },
);

test("OPS-004: setting a restriction keeps its sentence after the refetched skill shows the restriction", async () => {
  let restricted: string | null = null;
  stub(true, (path, method) => {
    if (path === "/admin/skills")
      return {
        body: { skills: [{ ...ADMIN_SKILLS.skills[0], access_restriction: restricted }] },
        status: 200,
      };
    if (path.endsWith("/restriction")) {
      restricted = method === "PUT" ? "license-review" : null;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("設定受限展示"));
  await type("#admin-restriction-note", "terms under review");
  await click(button("設定受限"));
  await waitFor(has("解除受限展示"));
  await waitFor(has("「PDF Summariser」已設定受限展示。"));
  expect(document.activeElement?.id).toBe("admin-governance-result");
});

test("OPS-012: a daily-report item without cites shows its text and the page still renders", async () => {
  stub(true, (path) =>
    path === `/admin/agents/runs/${AGENT_FAILED_RUN}`
      ? {
          body: {
            ...ADMIN_AGENT_RUNS.runs[0],
            result: {
              items: [
                { status: "attention", text: "same words" },
                { status: "attention", text: "same words", cites: ["/a/b"] },
              ],
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/agents", { run: AGENT_FAILED_RUN });
  await waitFor(has("需要注意：2 項"));
  expect(container.querySelectorAll(".daily-report-cite").length).toBe(1);
  expect(container.textContent?.match(/same words/g)?.length).toBe(2);
});

test("agent and brake controls disappear when their refreshed status cannot be read", async () => {
  let unreadable = false;
  stub(true, (path) =>
    unreadable && path === "/admin/agents"
      ? { body: { error: "agent status unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/agents");
  await waitFor(has("拉下 Agent 煞車"));
  unreadable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.agents, exact: true });
  });
  await waitFor(() => Boolean(container.querySelector('[role="alert"]')));
  expect(has("拉下 Agent 煞車")()).toBe(false);
  expect(has("停用 daily-report")()).toBe(false);
});
