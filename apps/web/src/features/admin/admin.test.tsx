import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, expect, test, vi } from "vitest";
import { focusManager } from "@tanstack/react-query";
import App from "../../app/App";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { createAppRouter } from "../../app/router";
import { daysOf, seriesOf, usd } from "./admin.service";
import {
  ADMIN_AGENT_FINDINGS,
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_PROPOSALS,
  ADMIN_AGENT_RUNS,
  ADMIN_AGENT_STEPS,
  ADMIN_AGENTS,
  ADMIN_AUDIT_LOG,
  AGENT_FAILED_RUN,
  AGENT_FINDING,
  AGENT_PROPOSAL,
  AGENT_REPORT_RUN,
  ADMIN_EXPOSURE_CASE,
  ADMIN_SKILLS,
  PUBLICATION,
  PUBLISHER,
  SKILL,
  platformResponse,
} from "../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { preloadEveryPage } from "../../testing/pages";

beforeAll(preloadEveryPage);

type Call = { method: string; url: string; body?: Record<string, unknown> };
type Reply = { body: unknown; status: number } | undefined;

let container: HTMLDivElement;
let root: Root;
let calls: Call[];
let router: ReturnType<typeof createAppRouter>;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  router = createAppRouter();
  container = document.createElement("div");
  document.body.appendChild(container);
  calls = [];
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  delete window.__SKILLHUB_CLEAN_MODE__;
});

function stub(
  operator: boolean,
  override: (path: string, method: string, url: string) => Reply = () => undefined,
) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const { body, status } = override(path, method, url) ?? platformResponse(url);
    const payload = path === "/me" ? { ...(body as object), operator } : body;
    return Promise.resolve(
      new Response(status === 204 ? null : JSON.stringify(payload), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

async function mountAt(to: string, search?: Record<string, string>) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App router={router} />
      </StrictMode>,
    );
  });
  await go(to, search);
}

async function go(to: string, search?: Record<string, string>) {
  await act(async () => {
    await router.navigate({ to: to as "/", search: (search ?? {}) as never });
  });
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const has = (needle: string) => () => (container.textContent ?? "").includes(needle);

function field<T extends HTMLElement>(selector: string): T {
  const el = container.querySelector<T>(selector);
  expect(el, `no element matches ${selector}`).not.toBeNull();
  return el!;
}

async function type(selector: string, value: string) {
  const el = field<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(selector);
  const proto = Object.getPrototypeOf(el) as object;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(
      new Event(el instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }),
    );
  });
}

function button(text: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => (b.textContent ?? "").trim() === text,
  );
  expect(found, `no button reads 「${text}」`).toBeDefined();
  return found!;
}

async function click(el: HTMLElement) {
  await act(async () => el.click());
}

async function submit(selector: string) {
  const form = field<HTMLElement>(selector).closest("form")!;
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

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

test("the admin home keeps decisions and operations inside Governing", async () => {
  stub(true);
  await mountAt("/admin");
  await waitFor(has("營運後台"));

  expect(
    Array.from(container.querySelectorAll(".admin-home-eyebrow"), (item) => item.textContent),
  ).toEqual(["Governing · Decisions", "Governing · Operations"]);
  expect(has("Conducting")()).toBe(false);
});

test("the admin home leads with live operational priorities", async () => {
  stub(true);
  await mountAt("/admin");
  await waitFor(has("停止派送"));

  const priorities = field<HTMLElement>('[aria-label="目前需留意"]');
  expect(priorities.querySelector('a[href="/admin/dispatch"]')?.textContent).toContain("停止派送");
  const agentPriorities = Array.from(
    priorities.querySelectorAll('a[href="/admin/agents"]'),
    (item) => item.textContent,
  );
  expect(agentPriorities).toEqual(
    expect.arrayContaining([
      expect.stringContaining("1 件待核准"),
      expect.stringContaining("2 件待辦"),
    ]),
  );
  expect(priorities.querySelector('a[href="/admin/exposure"]')?.textContent).toContain("1 件待審");
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
  expect(priorities.querySelector('a[href="/admin/agents"]')?.textContent).toContain("無法取得");
  expect(priorities.querySelector('a[href="/admin/agents"]')?.textContent).not.toContain(
    "0 件待核准",
  );
});

test("the admin home can refresh operational state without leaving the page", async () => {
  let dispatchReads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    dispatchReads += 1;
    return { body: { dispatching: dispatchReads > 1, halts: [] }, status: 200 };
  });
  await mountAt("/admin");
  await waitFor(has("停止派送"));

  await click(button("重新整理狀態"));
  await waitFor(has("正在派送"));
  expect(dispatchReads).toBe(2);
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
  await waitFor(has("暫時無法讀取後台。請重新整理，或稍後再試。"));
  expect(field<HTMLElement>('main [role="alert"]').textContent).toBe(
    "暫時無法讀取後台。請重新整理，或稍後再試。",
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

async function lookUp(email: string) {
  await mountAt("/admin/accounts");
  await waitFor(has("誰在何時查了誰"));
  await type("#admin-account-email", email);
  await submit("#admin-account-email");
}

test("OPS-002: a lookup keeps the email out of the address and shows the account with its ledger", async () => {
  stub(true);
  await lookUp("Member@Example.com");
  await waitFor(has("封測者甲"));
  await waitFor(has("目前餘額"));
  expect(calls.some((c) => c.url === "/admin/accounts?email=Member%40Example.com")).toBe(true);
  expect(window.location.href).not.toContain("xample");
  const rows = Array.from(container.querySelectorAll("tbody tr")).map((tr) =>
    Array.from(tr.querySelectorAll("td"))
      .slice(1)
      .map((td) => td.textContent),
  );
  expect(rows).toEqual([
    ["扣點（估計值）", "-30", "run"],
    ["授予", "+150", "不適用"],
  ]);
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(0);
  expect(field("strong").textContent).toBe("120");
});

test("OPS-002: an email nobody has is named as such, not reported as a broken read", async () => {
  stub(true, (path) =>
    path === "/admin/accounts" ? { body: { error: "not found" }, status: 404 } : undefined,
  );
  await lookUp("ghost@example.com");
  await waitFor(has("沒有 email 是「ghost@example.com」的帳號"));
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("OPS-003: a grant waits for a non-zero whole amount and a reason, then posts both and reloads the ledger", async () => {
  stub(true, (path, method) =>
    method === "POST" && path === "/admin/credits/ws-2/grants"
      ? { body: { workspace_id: "ws-2", balance_credits: 170, amount_credits: 50 }, status: 200 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  const grant = () => button("授予");
  expect(grant().disabled).toBe(true);

  await type("#admin-grant-note", "  beta reward ");
  for (const amount of ["0", "1.5", ""]) {
    await type("#admin-grant-amount", amount);
    expect(grant().disabled, `amount 「${amount}」`).toBe(true);
  }
  await type("#admin-grant-amount", "50");
  expect(grant().disabled).toBe(false);

  const ledgerReads = () => calls.filter((c) => c.url === "/admin/credits/ws-2").length;
  const before = ledgerReads();
  await click(grant());
  await waitFor(has("已授予 50 點，餘額現在是 170 點。"));
  expect(calls.find((c) => c.method === "POST")?.body).toEqual({
    amount_credits: 50,
    reason: "beta reward",
    idempotency_key: expect.stringMatching(/^[0-9a-f-]{36}$/),
  });
  await waitFor(() => ledgerReads() > before);
  await type("#admin-grant-amount", "75");
  expect(has("已授予 50 點，餘額現在是 170 點。")()).toBe(false);
});

test("OPS-003: a failed grant is retried under the same key, and the next grant gets a fresh one", async () => {
  let attempt = 0;
  stub(true, (path, method) => {
    if (method !== "POST" || !path.endsWith("/grants")) return undefined;
    attempt += 1;
    return attempt === 1
      ? { body: { error: "grant failed" }, status: 500 }
      : { body: { workspace_id: "ws-2", balance_credits: 60, amount_credits: 10 }, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "10");
  await type("#admin-grant-note", "r");
  await click(button("授予"));
  await waitFor(has("沒有完成，伺服器說：grant failed"));
  await click(button("授予"));
  await waitFor(has("已授予 10 點，餘額現在是 60 點。"));
  await click(button("授予"));
  await waitFor(() => calls.filter((c) => c.method === "POST").length === 3);
  const keys = calls
    .filter((c) => c.method === "POST")
    .map((c) => (c.body as { idempotency_key: string }).idempotency_key);
  expect(keys[1]).toBe(keys[0]);
  expect(keys[2]).not.toBe(keys[0]);
});

test("OPS-003: a refused grant says so with the server's words", async () => {
  stub(true, (path, method) =>
    method === "POST" && path.endsWith("/grants")
      ? { body: { error: "amount_credits must not be zero" }, status: 400 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "5");
  await type("#admin-grant-note", "r");
  await click(button("授予"));
  await waitFor(has("沒有完成，伺服器說：amount_credits must not be zero"));
  await type("#admin-grant-note", "修改後的理由");
  expect(has("沒有完成，伺服器說：amount_credits must not be zero")()).toBe(false);
});

test("OPS-004: takedown of the one skill found takes a reason and a second click", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  expect(calls.some((c) => c.url === `/admin/skills?q=${SKILL}`)).toBe(true);
  expect(has("填了理由才能下架。")()).toBe(true);
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "下架"),
  ).toBe(false);

  await type("#admin-takedown-reason", " DMCA notice ");
  await click(button("下架"));
  expect(has("下架沒有恢復的路")()).toBe(true);
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
  await click(button("確認下架"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/takedown`,
    body: { reason: "DMCA notice" },
  });
});

test("OPS-004: releasing a skill needs licence evidence; blocking it does not", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/redistribution` && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布判定"));
  await type("#admin-redistribution-note", "legal cleared");
  expect(button("送出判定").disabled).toBe(false);

  await type("#admin-redistribution-value", "allowed");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-license-expression", "MIT");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-license-source", "package-license-file");
  expect(button("送出判定").disabled).toBe(false);
  await click(button("送出判定"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/redistribution`,
    body: {
      value: "allowed",
      note: "legal cleared",
      license_expression: "MIT",
      license_source: "package-license-file",
    },
  });
  await waitFor(has("已送出，上面的狀態已更新。"));
  await type("#admin-license-expression", "Apache-2.0");
  expect(has("已送出，上面的狀態已更新。")()).toBe(false);
});

test("a revised model timeout does not inherit the previous success notice", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));

  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("已套用，下一次呼叫就用這個秒數。"));

  await type("#admin-budget-judge-run-seconds", "101");
  expect(has("已套用，下一次呼叫就用這個秒數。")()).toBe(false);
});

test("OPS-004: a restriction is set with the known reason code and lifted by the same form", async () => {
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
  await waitFor(has("受限展示：license-review"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
    reason: "license-review",
    note: "terms under review",
  });

  await waitFor(has("解除受限展示"));
  await type("#admin-restriction-note", "cleared");
  await click(button("解除受限"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
  await waitFor(has("沒有受限"));
  expect(has("受限展示：")()).toBe(false);
  expect(has("設定受限展示")()).toBe(true);
});

test("OPS-004: a name matching several skills lists a way to pick each and offers no action yet", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              ADMIN_SKILLS.skills[0],
              {
                ...ADMIN_SKILLS.skills[0],
                skill_id: "cccccccc-4444-4444-4444-444444444444",
                workspace_id: "ws-3",
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: "pdf" });
  await waitFor(has("處理這一個"));
  expect(container.querySelectorAll('a[href*="/admin/skills?q="]')).toHaveLength(2);
  expect(has("的動作")()).toBe(false);
});

test("OPS-004: a taken-down skill shows when and why, and offers no action", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              {
                ...ADMIN_SKILLS.skills[0],
                takedown_at: "2026-09-10T00:00:00Z",
                takedown_reason: "DMCA",
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("理由：DMCA"));
  expect(has("的動作")()).toBe(false);
});

test("OPS-004: a takedown with no reason on record says it was not recorded", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [
              {
                ...ADMIN_SKILLS.skills[0],
                takedown_at: "2026-09-10T00:00:00Z",
                takedown_reason: null,
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("理由：未記錄"));
});

test("OPS-005: the dispatch page names the halt, and a declaration without a node halts the fleet", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch/halt" && method === "PUT"
      ? { body: { note: "整個叢集停止派送。" }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  expect(has("P1 事故：只有人能解除")()).toBe(true);
  expect(has("整個叢集")()).toBe(true);
  expect(button("停止派送").classList.contains("caution")).toBe(true);
  expect(button("恢復派送").classList.contains("caution")).toBe(false);
  expect(button("恢復派送").disabled).toBe(true);
  await type("#admin-halt-declare-note", "escape drill");
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ note: "escape drill" });

  await type("#admin-halt-provider", "node-2");
  expect(has("整個叢集停止派送。")()).toBe(false);
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "cleared");
  await click(button("恢復派送"));
  expect(has("觸發條件已消失")()).toBe(true);
  await click(button("確認恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
});

test("OPS-005: a node halt remains visible while other nodes can still dispatch", async () => {
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      return {
        body: {
          dispatching: true,
          halts: [
            {
              target: "node-2",
              source: "orphan_threshold",
              reason: "orphan capacity reached",
              declared_at: "2026-09-11T09:00:00Z",
              automatic_recovery: true,
            },
          ],
        },
        status: 200,
      };
    }
    return path === "/admin/dispatch/halt" && method === "DELETE"
      ? { body: undefined, status: 204 }
      : undefined;
  });
  await mountAt("/admin");
  await waitFor(has("仍在派送；1 個煞車"));
  expect(
    field<HTMLElement>('[aria-label="目前需留意"] a[href="/admin/dispatch"]').getAttribute(
      "data-state",
    ),
  ).toBe("pending");

  await go("/admin/dispatch");
  await waitFor(has("部分節點停止派送"));
  expect(has("其他節點仍可派送")()).toBe(true);
  await type("#admin-halt-lift-target", "node-2");
  await type("#admin-halt-lift-note", "capacity cleared and verified");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));
  await waitFor(() => calls.some((call) => call.method === "DELETE"));
  expect(calls.find((call) => call.method === "DELETE")?.body).toEqual({
    note: "capacity cleared and verified",
    provider: "node-2",
  });
});

test("OPS-005: no active halt means there is nothing to release", async () => {
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { body: { dispatching: true, halts: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("沒有生效中的煞車"));
  expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
  expect(button("停止派送")).toBeDefined();
});

test("OPS-005: stopped dispatch with no listed halt is not described as healthy", async () => {
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { body: { dispatching: false, halts: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("未列出煞車，請確認節點設定與平台狀態"));
  expect(has("停止派送")()).toBe(true);
  expect(has("未列出煞車，請確認節點設定與平台狀態")()).toBe(true);
});

test("OPS-005: an unreadable dispatch state keeps emergency halt available but hides release", async () => {
  stub(true, (path) =>
    path === "/admin/dispatch"
      ? { body: { error: "service unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("暫時無法讀取派送狀態"));
  expect(button("停止派送")).toBeDefined();
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
});

test("OPS-005: refreshing dispatch status removes a halt that is no longer active", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return reads === 1
      ? {
          body: {
            dispatching: false,
            halts: [
              {
                target: "pool",
                source: "p1_incident",
                reason: "incident",
                declared_at: "2026-09-11T09:00:00Z",
                automatic_recovery: false,
              },
            ],
          },
          status: 200,
        }
      : { body: { dispatching: true, halts: [] }, status: 200 };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("incident"));
  expect(button("恢復派送")).toBeDefined();

  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(reads).toBe(2);
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
});

test("OPS-005: a changed halt requires a new release reason and confirmation", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return {
      body: {
        dispatching: true,
        halts: [
          {
            target: "node-2",
            source: reads === 1 ? "orphan_threshold" : "p1_incident",
            reason: reads === 1 ? "capacity threshold" : "new incident",
            declared_at: reads === 1 ? "2026-09-11T09:00:00Z" : "2026-09-11T10:00:00Z",
            automatic_recovery: reads === 1,
          },
        ],
      },
      status: 200,
    };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("capacity threshold"));
  await type("#admin-halt-lift-target", "node-2");
  await type("#admin-halt-lift-note", "capacity cleared");
  await click(button("恢復派送"));
  expect(button("確認恢復派送")).toBeDefined();

  await click(button("重新整理派送狀態"));
  await waitFor(has("new incident"));
  expect(field<HTMLTextAreaElement>("#admin-halt-lift-note").value).toBe("");
  expect(button("恢復派送").disabled).toBe(true);
});

test("OPS-005: the rosters page is read-only", async () => {
  stub(true);
  await mountAt("/admin/rosters");
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(field("main code").textContent).toBe("u-1");
  expect(container.querySelectorAll("main :is(input, textarea, select, button)")).toHaveLength(0);
});

test("OPS-006: the audit log names actions in words and folds the metadata", async () => {
  stub(true);
  await mountAt("/admin/audit-log");
  await waitFor(has("授予點數"));
  expect(has("查詢帳號")()).toBe(true);
  expect(has("點數分錄")()).toBe(true);
  expect(has("credit_entry")()).toBe(false);
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(-1);
  const table = field<HTMLTableElement>("table.responsive-table");
  const labels = ["時間", "動作", "行為者", "對象", "內容"];
  expect(table.caption?.textContent).toBe("全平台動作紀錄，新的在上面");
  expect(has("已載入 2 筆動作紀錄")()).toBe(true);
  expect(Array.from(table.querySelectorAll("thead th")).map((th) => th.textContent)).toEqual(
    labels,
  );
  expect(
    Array.from(table.tBodies[0].rows).map((row) =>
      Array.from(row.children).map((cell) => cell.getAttribute("data-label")),
    ),
  ).toEqual([labels, labels]);
  expect(
    Array.from(table.querySelectorAll('tbody th[scope="row"]')).map((th) => th.textContent),
  ).toEqual(["授予點數", "查詢帳號"]);
  expect(field("td details summary").textContent).toBe("3 項");
  expect(field("td details").textContent).toContain("beta reward");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);
});

test("OPS-006: a halt the platform declared by itself names the platform as the actor", async () => {
  stub(true, (path) =>
    path.startsWith("/admin/audit-log")
      ? {
          body: {
            events: [
              {
                actor_kind: "system",
                actor_user_id: null,
                actor_agent_id: null,
                action: "dispatch.halted",
                resource_type: "dispatch",
                resource_id: "h-1",
                workspace_id: null,
                occurred_at: "2026-09-10T08:00:00Z",
                metadata: { target: "all", source: "orphan_threshold", reason: "orphans" },
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("停止派送"));
  expect(has("平台自動")()).toBe(true);
  expect(has("未測量")()).toBe(false);
});

test("OPS-011: an action one of the platform's agents took names the agent, not the platform", async () => {
  stub(true, (path) =>
    path.startsWith("/admin/audit-log")
      ? {
          body: {
            events: [
              {
                actor_kind: "agent",
                actor_user_id: null,
                actor_agent_id: "agent-7",
                action: "dispatch.halted",
                resource_type: "dispatch",
                resource_id: "h-2",
                workspace_id: null,
                occurred_at: "2026-09-10T08:00:00Z",
                metadata: {},
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("停止派送"));
  expect(has("平台 Agent agent-7")()).toBe(true);
  expect(has("平台自動")()).toBe(false);
});

test("OPS-006: a full page of 50 stops, the 51st event offers the next page", async () => {
  let total = 50;
  const event = ADMIN_AUDIT_LOG.events[0];
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? { body: { events: Array.from({ length: total }, () => event) }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);

  total = 51;
  queryClient.clear();
  await act(async () => root.unmount());
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  expect(button("載入更多")).toBeDefined();
  expect(has("已載入 50 筆動作紀錄；還有更多")()).toBe(true);
  expect(
    calls
      .filter((c) => c.url.startsWith("/admin/audit-log"))
      .every((c) => c.url.includes("limit=51")),
  ).toBe(true);
});

test("OPS-007: cost statistics show dollars and name a window with no samples", async () => {
  stub(true);
  await mountAt("/admin/cost-statistics");
  await waitFor(has("搜尋理由"));
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(-1);
  const table = field<HTMLTableElement>("table.responsive-table");
  const labels = ["種類", "統計窗結束", "樣本數", "p50", "p90", "p95", "最大"];
  expect(Array.from(table.querySelectorAll("thead th")).map((th) => th.textContent)).toEqual(
    labels,
  );
  expect(
    Array.from(table.tBodies[0].rows).map((row) =>
      Array.from(row.children).map((cell) => cell.getAttribute("data-label")),
    ),
  ).toEqual([labels, labels]);
  expect(
    Array.from(table.querySelectorAll('tbody th[scope="row"]')).map((th) => th.textContent),
  ).toEqual(["搜尋理由", "評審"]);
  const rows = Array.from(container.querySelectorAll("tbody tr")).map((tr) => [
    tr.querySelector("th")?.textContent,
    ...Array.from(tr.querySelectorAll("td"))
      .slice(1)
      .map((td) => td.textContent),
  ]);
  expect(rows).toEqual([
    ["搜尋理由", "0", "未測量", "未測量", "未測量", "未測量"],
    ["評審", "40", "$0.0012", "$0.0034", "$0.0041", "$0.0090"],
  ]);
});

test("OPS-007: micro-dollars format at four places, and a missing percentile is named", () => {
  expect(usd(null)).toBe("未測量");
  expect(usd(0)).toBe("$0.0000");
  expect(usd(49)).toBe("$0.0000");
  expect(usd(50)).toBe("$0.0001");
  expect(usd(1_234_567)).toBe("$1.2346");
});

test("OPS-008: the day list runs from the first to the last day inclusive, across a month end", () => {
  expect(daysOf("2026-08-30", "2026-09-02")).toEqual([
    "2026-08-30",
    "2026-08-31",
    "2026-09-01",
    "2026-09-02",
  ]);
  expect(daysOf("2026-09-12", "2026-09-12")).toEqual(["2026-09-12"]);
  expect(daysOf("2026-09-13", "2026-09-12")).toEqual([]);
});

test("OPS-008: a series fills a day with no bucket with zero and drops a bucket outside the range", () => {
  const { days, series } = seriesOf(
    {
      from: "2026-09-10",
      to: "2026-09-12",
      buckets: [
        { day: "2026-09-12", key: "review", count: 2, total: 30 },
        { day: "2026-09-10", key: "review", count: 1, total: 5 },
        { day: "2026-09-09", key: "review", count: 9, total: 900 },
        { day: "2026-09-11", key: "generate", count: 1, total: 7 },
      ],
    },
    (bucket) => bucket.total,
  );
  expect(days).toEqual(["2026-09-10", "2026-09-11", "2026-09-12"]);
  expect(series).toEqual([
    { key: "generate", counts: [0, 1, 0], values: [0, 7, 0] },
    { key: "review", counts: [1, 0, 2], values: [5, 0, 30] },
  ]);
});

const TREND_PATHS = ["cost", "credits", "funnel", "operator-actions", "runs"];
const trendCalls = () => calls.filter((c) => c.url.startsWith("/admin/trends/")).map((c) => c.url);
const trendsFor = (days: number) =>
  new Set(TREND_PATHS.map((path) => `/admin/trends/${path}?days=${days}`));

test("OPS-008: the trends page asks each owner for 30 days by default and draws one chart per kind with events", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(new Set(trendCalls())).toEqual(trendsFor(30));
  expect(
    Array.from(container.querySelectorAll<HTMLElement>("figure .table-scroll")).map(
      (scroll) => scroll.tabIndex,
    ),
  ).toEqual(Array(11).fill(0));
  expect(has("2026-09-06 到 2026-09-12（UTC），共 7 天。")()).toBe(true);
  expect(
    Array.from(container.querySelectorAll('canvas[role="img"]')).map((c) =>
      c.getAttribute("aria-label"),
    ),
  ).toEqual(
    [
      "單次生成",
      "評審",
      "扣點",
      "授予",
      "執行失敗",
      "執行完成",
      "查詢帳號",
      "授予點數",
      "開始試跑",
      "搜尋",
      "看小工具詳情",
    ].map((name) => `${name}：每日長條圖，逐日數字在下方的表`),
  );
});

test("OPS-008: a kind's figure totals its range and its table shows zero on the days it had nothing", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("評審：3 筆，合計 $0.0036"));
  expect(has("扣點：4 筆，合計 -32 點")()).toBe(true);
  const review = Array.from(container.querySelectorAll("figure")).find((figure) =>
    figure.textContent?.startsWith("評審"),
  );
  expect(Array.from(review!.querySelectorAll("tbody tr")).map((tr) => tr.textContent)).toEqual([
    "2026-09-062$0.0024",
    "2026-09-070$0.0000",
    "2026-09-081$0.0012",
    "2026-09-090$0.0000",
    "2026-09-100$0.0000",
    "2026-09-110$0.0000",
    "2026-09-120$0.0000",
  ]);
});

test("OPS-008: the funnel says what one count means at every stage, from the server's own sentences", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("每個瀏覽工作階段一天算一次，這一段系統性偏高。"));
  const grains = Array.from(container.querySelectorAll("dl > div")).map((row) => [
    row.querySelector("dt")?.textContent,
    row.querySelector("dd")?.textContent,
  ]);
  expect(grains).toEqual([
    ["搜尋", "每個瀏覽工作階段一天算一次，這一段系統性偏高。"],
    ["看小工具詳情", "粒度同搜尋。"],
    ["開始試跑", "每個工作區一天算一次，不能相除成轉換率。"],
    ["按下下載", "每個工作區一天算一次；打包仍可能被拒。"],
  ]);
  expect(has("這段期間沒有事件：按下下載。")()).toBe(true);
  expect(has("搜尋：12 筆")()).toBe(true);
});

test("OPS-008: a kind with no events in the range is named instead of drawn", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和"));
  expect(has("這段期間沒有事件：儲值、更正。")()).toBe(true);
  expect(
    has(
      "這段期間沒有事件：創作步驟、創作會話、搜尋向量、搜尋意圖分析、平台 Agent、索引增強、改善建議、試跑、搜尋理由。",
    )(),
  ).toBe(true);
});

test("OPS-008: a range in the address is asked for, and a range the page does not offer falls back to 30", async () => {
  stub(true);
  await mountAt("/admin/trends", { days: "7" });
  await waitFor(has("全平台目前餘額總和"));
  expect(new Set(trendCalls())).toEqual(trendsFor(7));

  calls = [];
  await go("/admin/trends", { days: "8" });
  await waitFor(() => trendCalls().length >= 5);
  expect(new Set(trendCalls())).toEqual(trendsFor(30));
});

test("OPS-008: the trends are asked for once, not again when the window regains focus", async () => {
  stub(true);
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和"));
  const asked = trendCalls().length;
  await act(async () => {
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  focusManager.setFocused(undefined);
  expect(trendCalls().length).toBe(asked);
});

test("OPS-008: useTrend itself refuses a focus refetch even when the app default would allow it", async () => {
  const defaults = queryClient.getDefaultOptions();
  queryClient.setDefaultOptions({
    queries: { ...defaults.queries, refetchOnWindowFocus: true, staleTime: 0 },
  });
  try {
    stub(true);
    await mountAt("/admin/trends");
    await waitFor(has("全平台目前餘額總和"));
    const asked = trendCalls().length;
    expect(asked).toBeGreaterThan(0);
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(trendCalls().length).toBe(asked);
  } finally {
    focusManager.setFocused(undefined);
    queryClient.setDefaultOptions(defaults);
  }
});

test("OPS-008: a member who types the trends address gets the missing page and no trend request", async () => {
  stub(false);
  await mountAt("/admin/trends");
  await waitFor(has("這一頁現在不存在"));
  expect(trendCalls()).toEqual([]);
});

const EXPOSURE_PUBLICATION = `${PUBLISHER}/${PUBLICATION}`;

test("DISC-007: the queue lists a waiting release, and reviewing it shows the exact snapshot search holds", async () => {
  stub(true);
  await mountAt("/admin/exposure");
  await waitFor(has(EXPOSURE_PUBLICATION));
  expect(has("曾核准，之後內容有變，需要重新審核。")()).toBe(true);

  await click(
    Array.from(container.querySelectorAll("a")).find((a) => a.textContent === "審這一筆")!,
  );
  await waitFor(has("目前未曝光"));
  expect(new URLSearchParams(window.location.search).get("publication")).toBe(EXPOSURE_PUBLICATION);
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.enriched_summary)()).toBe(true);
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.task_examples)()).toBe(true);
  expect(has("pdf、summary")()).toBe(true);
  expect(has("內容符合規範")()).toBe(true);
  expect(
    field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]').checked,
  ).toBe(false);
  expect(
    field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]').checked,
  ).toBe(false);
  expect(button("送出審核結論").disabled).toBe(true);
});

test("DISC-007: submitting a review sends this screen's release_id, sequence and snapshot digest", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(() => calls.some((c) => c.method === "POST"));
  expect(calls.find((c) => c.method === "POST")).toEqual({
    method: "POST",
    url: `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`,
    body: {
      release_id: ADMIN_EXPOSURE_CASE.release.release_id,
      expected_sequence: ADMIN_EXPOSURE_CASE.sequence,
      expected_snapshot_digest: ADMIN_EXPOSURE_CASE.snapshot.digest,
      decision: "approved",
      reason: "看過了，符合規範",
    },
  });
});

test("a completed exposure decision does not describe the next decision", async () => {
  stub(true, (path, method) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && method === "POST"
      ? { body: ADMIN_EXPOSURE_CASE, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("已送出，上面的狀態已更新。"));

  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]'));
  expect(has("已送出，上面的狀態已更新。")()).toBe(false);
});

test("DISC-007: a decision and reason are both required before submission", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  expect(button("送出審核結論").disabled).toBe(true);
  await type("#admin-exposure-review-note", "看過了");
  expect(button("送出審核結論").disabled).toBe(true);
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  expect(button("送出核准").disabled).toBe(false);
});

test("DISC-007: a decision chosen without a reason keeps submission disabled", async () => {
  stub(true);
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  expect(button("送出核准").disabled).toBe(true);
});

test("DISC-007: an empty queue is named as a genuine zero, not a blank list", async () => {
  stub(true, (path) =>
    path === "/admin/exposure-reviews" ? { body: { publications: [] }, status: 200 } : undefined,
  );
  await mountAt("/admin/exposure");
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(container.querySelectorAll(".download-item")).toHaveLength(0);
});

test("DISC-007: a stale review (409) shows the server's own words, not a generic failure", async () => {
  const staleMessage =
    "這份審核的前提已經過期：有新的 Release，或別人已經審過。重新打開這一筆，看過現在的內容再送出";
  stub(true, (path, method) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && method === "POST"
      ? { body: { error: staleMessage, reason: "review_stale" }, status: 409 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了");
  await click(button("送出核准"));
  await waitFor(has(`沒有完成，伺服器說：${staleMessage}`));
});

test("DISC-007: a release search has not indexed yet says so instead of showing stale text", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? { body: { ...ADMIN_EXPOSURE_CASE, snapshot: undefined }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("尚未進索引"));
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.enriched_summary)()).toBe(false);
});

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
  expect(button("我來處理")).toBeDefined();
  expect(button("忽略這件事")).toBeDefined();
  expect(has("重新打開")()).toBe(false);
  expect(has("打開這件事")()).toBe(false);
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
  await type("#admin-finding-acknowledged-note", " checking the job ");
  await submit("#admin-finding-acknowledged-note");
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/agents/findings/${AGENT_FINDING}/status`,
    body: { status: "acknowledged", note: "checking the job" },
  });
});

test("OPS-012: a refused move says so with the server's words", async () => {
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
  await type("#admin-finding-resolved-note", "fixed");
  await submit("#admin-finding-resolved-note");
  await waitFor(has("the finding cannot move to that status from where it is now"));
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
  await waitFor(has("the proposal is no longer waiting for a decision"));
  await type("#admin-proposal-reject-note", "second decision");
  await submit("#admin-proposal-reject-note");
  await waitFor(has("已駁回。"));
  expect(has("已核准，維運程序會在幾分鐘內執行。")()).toBe(false);
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
  await type("#admin-finding-acknowledged-note", "first move");
  await submit("#admin-finding-acknowledged-note");
  await waitFor(has("the finding cannot move to that status"));
  await type("#admin-finding-resolved-note", "second move");
  await submit("#admin-finding-resolved-note");
  await waitFor(has("已改成「已解決」。"));
  expect(has("已改成「處理中」。")()).toBe(false);
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
  expect(has("which no tool returned")()).toBe(true);
  expect(has("2 步；花費 $0.0018（另有 1 步沒有回報花費）")()).toBe(true);
  expect(has("2 步；花費 $0.0031")()).toBe(false);
});

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
  await type("#admin-finding-acknowledged-note", "checking");
  await submit("#admin-finding-acknowledged-note");
  await waitFor(has("已改成「"));
  await waitFor(() => !has("我來處理")());
  await type("#admin-finding-resolved-note", "next note");
  await waitFor(() => !has("已改成「")());
});

test.each([
  ["approve", "approved", "已核准，維運程序會在幾分鐘內執行。"],
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
  await type("#admin-finding-resolved-note", "fixed");
  await submit("#admin-finding-resolved-note");
  await waitFor(has("already moved"));
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
  await waitFor(has("已送出，上面的狀態已更新。"));
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
