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
  ADMIN_AGENT_FINDING,
  ADMIN_AGENT_FINDINGS,
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_PROPOSALS,
  ADMIN_AGENT_RUNS,
  ADMIN_AGENT_STEPS,
  ADMIN_AGENTS,
  ADMIN_AUDIT_LOG,
  ADMIN_ACCOUNT,
  ADMIN_COST_STATISTICS,
  ADMIN_DISPATCH,
  ADMIN_LEDGER,
  ADMIN_MODEL_BUDGETS,
  ADMIN_ROSTERS,
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
  expect(has("封測准入目前可通過；可能是已受邀，或此部署未限制。")()).toBe(true);
});

test("OPS-002: submitting the same email again performs a fresh audited lookup", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return {
      body: { ...ADMIN_ACCOUNT, display_name: `第 ${reads} 次查詢` },
      status: 200,
    };
  });
  await lookUp("member@example.com");
  await waitFor(has("第 1 次查詢"));
  expect(reads).toBe(1);

  await submit("#admin-account-email");
  await waitFor(has("第 2 次查詢"));
  expect(reads).toBe(2);
});

test("an account denied by beta admission is not described as merely absent from a roster", async () => {
  stub(true, (path) =>
    path === "/admin/accounts"
      ? { body: { ...ADMIN_ACCOUNT, in_beta_allowlist: false }, status: 200 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("目前不可通過；請確認封測名單或部署設定。"));
  expect(has("不在名單上")()).toBe(false);
});

test("OPS-002: an email nobody has is named as such, not reported as a broken read", async () => {
  stub(true, (path) =>
    path === "/admin/accounts" ? { body: { error: "not found" }, status: 404 } : undefined,
  );
  await lookUp("ghost@example.com");
  await waitFor(has("沒有 email 是「ghost@example.com」的帳號"));
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("OPS-002: editing the email hides the prior account until the new lookup completes", async () => {
  stub(true, (path, _method, url) =>
    path === "/admin/credits/ws-3"
      ? { body: { ...ADMIN_LEDGER, workspace_id: "ws-3" }, status: 200 }
      : path === "/admin/accounts" && url.includes("other%40example.com")
        ? {
            body: {
              ...ADMIN_ACCOUNT,
              email: "other@example.com",
              display_name: "封測者乙",
              workspace_id: "ws-3",
            },
            status: 200,
          }
        : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("封測者甲"));
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "first account");

  await type("#admin-account-email", "MEMBER@example.com");
  expect(has("封測者甲")()).toBe(true);
  await type("#admin-account-email", "other@example.com");
  expect(has("Email 已變更；按「查詢」載入新帳號。")()).toBe(true);
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();
  expect(calls.some((call) => call.url.includes("other%40example.com"))).toBe(false);

  await submit("#admin-account-email");
  await waitFor(has("封測者乙"));
  await waitFor(has("授予點數"));
  expect(field<HTMLInputElement>("#admin-grant-amount").value).toBe("");
  expect(field<HTMLTextAreaElement>("#admin-grant-note").value).toBe("");
  expect(button("授予").disabled).toBe(true);
});

test("OPS-002: a failed account reread hides the old target and offers retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ACCOUNT, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.account("member@example.com") });
  });
  await waitFor(has("暫時無法讀取帳號"));
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("封測者甲"));
  expect(reads).toBe(3);
});

test("OPS-002: a first lookup failure offers retry without inventing an account", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ACCOUNT, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("暫時無法讀取帳號"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("封測者甲"));
  expect(reads).toBe(2);
});

test("OPS-003: a failed ledger reread hides stale balance and blocks a grant until retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/credits/ws-2") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_LEDGER, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("目前餘額"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "correction");
  expect(button("授予").disabled).toBe(false);

  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.ledger("ws-2") });
  });
  await waitFor(has("暫時無法讀取點數"));
  expect(has("目前餘額")()).toBe(false);
  expect(button("授予").disabled).toBe(true);
  expect(has("要等最新餘額讀取完成，才可授予點數。")()).toBe(true);
  expect(has("「授予」要等上面的欄位都填好。")()).toBe(false);
  expect(button("授予").getAttribute("aria-describedby")).toBe("admin-grant-why");
  expect(field<HTMLElement>("#admin-grant-why").textContent).toBe(
    "要等最新餘額讀取完成，才可授予點數。",
  );

  await click(button("再試一次"));
  await waitFor(has("目前餘額"));
  expect(button("授予").disabled).toBe(false);
  expect(reads).toBe(3);
});

test("OPS-003: a first ledger read failure offers retry without a grant form", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/credits/ws-2") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_LEDGER, status: 200 };
  });
  await lookUp("member@example.com");
  await waitFor(has("暫時無法讀取點數"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("目前餘額")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("目前餘額"));
  expect(reads).toBe(2);
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

test("OPS-003: a negative correction says it deducts credits before and after submission", async () => {
  stub(true, (path, method) =>
    method === "POST" && path === "/admin/credits/ws-2/grants"
      ? { body: { workspace_id: "ws-2", balance_credits: 90, amount_credits: -30 }, status: 200 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "-30");
  await type("#admin-grant-note", "corrects an over-grant");

  expect(has("更正點數")()).toBe(true);
  expect(button("扣減點數").disabled).toBe(false);
  await click(button("扣減點數"));
  await waitFor(has("已扣減 30 點，餘額現在是 90 點。"));
  expect(calls.find((call) => call.method === "POST")?.body).toMatchObject({
    amount_credits: -30,
    reason: "corrects an over-grant",
  });
});

test("OPS-003: a failed grant retries with one key, but a completed draft cannot be sent again", async () => {
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
  expect(button("授予").disabled).toBe(true);
  expect(button("授予").getAttribute("aria-describedby")).toBe("admin-grant-why");
  expect(field<HTMLElement>("#admin-grant-why").textContent).toBe(
    "已完成這筆操作；修改上方欄位或理由後再送出，會建立另一筆操作。",
  );
  expect(calls.filter((c) => c.method === "POST")).toHaveLength(2);
  await type("#admin-grant-note", "another grant");
  expect(button("授予").disabled).toBe(false);
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

  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", " DMCA notice ");
  await click(button("下架"));
  expect(container.querySelector("#admin-takedown-scope")?.textContent).toContain(
    "後台目前不提供恢復",
  );
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
  await click(button("確認下架"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/takedown`,
    body: { reason: "DMCA notice" },
  });
});

test("a completed takedown stays confirmed after its controls disappear and a later read fails", async () => {
  const takenDown = {
    ...ADMIN_SKILLS.skills[0],
    takedown_at: "2026-09-11T10:00:00Z",
    takedown_reason: "DMCA notice",
  };
  let reads = 0;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { skills: [reads === 1 ? ADMIN_SKILLS.skills[0] : takenDown] }, status: 200 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("已下架"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-takedown-result"));
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();

  await click(button("重新整理治理狀態"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(has("已下架")()).toBe(true);
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
});

test("a successful takedown remains identified when the first refresh fails", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_SKILLS, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(has("「PDF Summariser」已下架。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-takedown-result"));
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
  expect(reads).toBe(2);
});

test("a new governance query does not inherit the previous takedown result", async () => {
  let takenDown = false;
  stub(true, (path, method) => {
    if (path === `/admin/skills/${SKILL}/takedown` && method === "PUT") {
      takenDown = true;
      return { body: { skill_id: SKILL, taken_down: true }, status: 200 };
    }
    return path === "/admin/skills"
      ? {
          body: {
            skills: [
              takenDown
                ? {
                    ...ADMIN_SKILLS.skills[0],
                    takedown_at: "2026-09-11T10:00:00Z",
                    takedown_reason: "DMCA notice",
                  }
                : ADMIN_SKILLS.skills[0],
            ],
          },
          status: 200,
        }
      : undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "DMCA notice");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("「PDF Summariser」已下架。"));

  await go("/admin/skills", { q: "PDF" });
  await waitFor(() => field<HTMLInputElement>("#admin-skill-q").value === "PDF");
  await waitFor(has("下架於"));
  expect(has("「PDF Summariser」已下架。")()).toBe(false);
});

test("OPS-004: governance search states how many skills were found", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("查到 1 筆小工具。"));
});

test("OPS-004: governance choices stay scannable without hiding takedown scope", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  const choices = Array.from(
    container.querySelectorAll<HTMLDetailsElement>("details[id^='admin-skill-']"),
  );
  expect(choices.map((choice) => choice.id)).toEqual([
    "admin-skill-restriction",
    "admin-skill-redistribution",
    "admin-skill-takedown",
  ]);
  expect(choices.every((choice) => !choice.open)).toBe(true);
  const scope = Array.from(container.querySelectorAll("p")).find((item) =>
    item.textContent?.startsWith("下架後這個小工具從目錄與搜尋消失"),
  );
  expect(scope?.closest("details")).toBeNull();
  expect(scope?.textContent).toContain("後台目前不提供恢復");

  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  expect(choices[1].open).toBe(true);
  expect(choices[0].open).toBe(false);
  expect(choices[2].open).toBe(false);
});

test("OPS-004: governance status precedes machine identifiers", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  const row = field<HTMLLIElement>("li.download-item");
  expect(row.querySelector(".badge-row")?.nextElementSibling?.textContent ?? "").toContain(
    "工作區",
  );
});

test("OPS-004: changing a takedown reason requires a new confirmation", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "old reason");
  await click(button("下架"));
  expect(button("確認下架")).toBeDefined();

  await type("#admin-takedown-reason", "revised reason");
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(() => calls.some((call) => call.url === `/admin/skills/${SKILL}/takedown`));
  expect(calls.find((call) => call.url === `/admin/skills/${SKILL}/takedown`)?.body).toEqual({
    reason: "revised reason",
  });
});

test("OPS-004: a new takedown draft clears the previous governance success", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/redistribution` && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "distribution blocked");
  await type("#admin-redistribution-value", "blocked");
  await click(button("送出判定"));
  await waitFor(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。"));

  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "New evidence");
  expect(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-takedown-reason").value).toBe("New evidence");
  expect(calls.some((call) => call.url.endsWith("/takedown"))).toBe(false);
});

test("OPS-004: editing a rejected takedown clears the old failure", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/takedown` && method === "PUT"
      ? { body: { error: "review required" }, status: 409 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "Old evidence");
  await click(button("下架"));
  await click(button("確認下架"));
  await waitFor(has("沒有完成，伺服器說：review required"));

  await type("#admin-takedown-reason", "New evidence");
  expect(has("沒有完成，伺服器說：review required")()).toBe(false);
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
});

test("OPS-004: a failed governance refresh hides stale state and actions", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_SKILLS, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(button("重新整理治理狀態"));
  await waitFor(has("暫時無法讀取小工具"));
  expect(reads).toBe(2);
  expect(has("對「PDF Summariser」的動作")()).toBe(false);
  expect(has("沒有符合")()).toBe(false);
});

test("OPS-004: a different skill returned by refresh starts with empty drafts", async () => {
  const other = {
    ...ADMIN_SKILLS.skills[0],
    skill_id: "cccccccc-4444-4444-4444-444444444444",
    name: "Other Tool",
  };
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return { body: { skills: [reads === 1 ? ADMIN_SKILLS.skills[0] : other] }, status: 200 };
  });
  await mountAt("/admin/skills", { q: "tool" });
  await waitFor(has("對「PDF Summariser」的動作"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "first skill evidence");
  await type("#admin-redistribution-value", "allowed");
  await type("#admin-license-expression", "MIT");
  await type("#admin-license-source", "manifest");
  await click(field<HTMLElement>("#admin-skill-takedown summary"));
  await type("#admin-takedown-reason", "first skill takedown");
  await click(button("下架"));

  await click(button("重新整理治理狀態"));
  await waitFor(has("對「Other Tool」的動作"));
  expect(field<HTMLSelectElement>("#admin-redistribution-value").value).toBe("");
  expect(field<HTMLTextAreaElement>("#admin-redistribution-note").value).toBe("");
  expect(container.querySelector("#admin-license-expression")).toBeNull();
  expect(field<HTMLInputElement>("#admin-takedown-reason").value).toBe("");
  expect(container.querySelector("#admin-takedown-scope")).toBeNull();
});

test("OPS-004: the search field follows the skill selected in the address", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: "pdf" });
  await waitFor(has("對「PDF Summariser」的動作"));
  await go("/admin/skills", { q: SKILL });
  await waitFor(() => field<HTMLInputElement>("#admin-skill-q").value === SKILL);
  expect(new URLSearchParams(window.location.search).get("q")).toBe(SKILL);
});

test("OPS-004: editing a governance query hides the prior skill and actions until search", async () => {
  const other = {
    ...ADMIN_SKILLS.skills[0],
    skill_id: "cccccccc-4444-4444-4444-444444444444",
    name: "Other Tool",
    workspace_id: "ws-3",
  };
  stub(true, (path, _method, url) =>
    path === "/admin/skills" && url.includes("q=other")
      ? { body: { skills: [other] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  await type("#admin-skill-q", "other");
  expect(has("PDF Summariser")()).toBe(false);
  expect(container.querySelector("#admin-skill-takedown")).toBeNull();
  expect(has("查詢條件已變更；按「查詢」載入新小工具。")()).toBe(true);
  expect(calls.some((call) => call.url === "/admin/skills?q=other")).toBe(false);

  await type("#admin-skill-q", SKILL);
  expect(has("對「PDF Summariser」的動作")()).toBe(true);
  await type("#admin-skill-q", "other");
  await submit("#admin-skill-q");
  await waitFor(has("對「Other Tool」的動作"));
  expect(has("PDF Summariser")()).toBe(false);
  expect(new URLSearchParams(window.location.search).get("q")).toBe("other");
});

test.each(["unknown", "blocked", "allowed"] as const)(
  "OPS-004: a %s redistribution state does not preselect the operator's next decision",
  async (redistribution) => {
    stub(true, (path) =>
      path === "/admin/skills"
        ? {
            body: {
              skills: [{ ...ADMIN_SKILLS.skills[0], redistribution }],
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/skills", { q: SKILL });
    await waitFor(has("再散布判定"));
    await click(field<HTMLElement>("#admin-skill-redistribution summary"));
    expect(field<HTMLSelectElement>("#admin-redistribution-value").value).toBe("");
    await type("#admin-redistribution-note", "legal review");
    expect(button("送出判定").disabled).toBe(true);
    expect(calls.some((call) => call.url.endsWith("/redistribution"))).toBe(false);
    await type("#admin-redistribution-value", "blocked");
    expect(button("送出判定").disabled).toBe(false);
  },
);

test("OPS-004: releasing a skill needs licence evidence; blocking it does not", async () => {
  stub(true, (path, method) =>
    path === `/admin/skills/${SKILL}/redistribution` && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布判定"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "legal cleared");
  expect(button("送出判定").disabled).toBe(true);
  await type("#admin-redistribution-value", "blocked");
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
  await waitFor(has("「PDF Summariser」的再散布判定已改為「可以再散布」。"));
  expect(document.activeElement?.id).toBe("admin-governance-result");
  await type("#admin-license-expression", "Apache-2.0");
  expect(has("「PDF Summariser」的再散布判定已改為「可以再散布」。")()).toBe(false);
});

test("a revised model timeout replaces the previous result even when reread stays stale", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && method === "PUT"
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));

  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("重新讀取仍顯示 90 秒；請重新整理確認。"));
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-warning")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));

  await type("#admin-budget-judge-run-seconds", "101");
  expect(container.querySelector("#admin-budget-result")).toBeNull();
});

test("OPS-009: changing a timeout confirms the reread value before promising the next call", async () => {
  let currentSeconds = 90;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      return {
        body: {
          budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
            budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
          ),
        },
        status: 200,
      };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "PUT") {
      currentSeconds = 100;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));

  await waitFor(has("目前：100 秒（已調整）"));
  await waitFor(has("目前顯示設定 100 秒；下次呼叫將使用此設定。"));
  expect(has("設定已變更；草稿改為最新的 100 秒")()).toBe(false);
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("100");
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-success")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
});

test("OPS-009: a completed timeout change stays visible when rereading fails", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_MODEL_BUDGETS, status: 200 }
        : { body: { error: "service unavailable" }, status: 503 };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "PUT") {
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));

  expect(has("目前：90 秒（已調整）")()).toBe(false);
  expect(field<HTMLElement>("#admin-budget-result").textContent).toContain(
    "目前設定暫時無法重新讀取，請稍後核對。",
  );
  expect(field<HTMLElement>("#admin-budget-result").classList.contains("notice-warning")).toBe(
    true,
  );
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
});

test("OPS-009: an override shows its effective value, default, range, and closed choices", async () => {
  stub(true);
  await mountAt("/admin/model-budgets");
  await waitFor(() => !!container.querySelector("li.download-item"));

  const judge = field<HTMLLIElement>("li.download-item");
  expect(judge.querySelector(".badge-row")?.textContent).toContain("目前：90 秒（已調整）");
  expect(judge.textContent).toContain("程式預設：130 秒；可設定範圍：1～130 秒");
  expect(judge.querySelector(".badge-danger")).toBeNull();
  const change = field<HTMLDetailsElement>("#admin-budget-judge-run-set");
  const restore = field<HTMLDetailsElement>("#admin-budget-judge-run-clear");
  expect(change.open).toBe(false);
  expect(restore.open).toBe(false);
  expect(restore.querySelector("summary")?.textContent).toContain("130 秒");

  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  expect(change.open).toBe(true);
  expect(restore.open).toBe(false);
  expect(container.querySelector("#admin-budget-match-reasons-clear")).toBeNull();
});

test("OPS-009: restoring the default keeps a focused result after the restore form disappears", async () => {
  let overridden = true;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      return {
        body: {
          budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
            budget.kind === "judge-run" && !overridden
              ? { ...budget, seconds: null, reason: null, set_at: null }
              : budget,
          ),
        },
        status: 200,
      };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "DELETE") {
      overridden = false;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("90");
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-clear-note", "恢復平台預設");
  await click(button("把 評估判定 改回預設"));
  await waitFor(has("目前：預設 130 秒"));

  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("130");
  expect(has("設定已變更；草稿改為最新的 130 秒")()).toBe(false);
  expect(container.querySelector("#admin-budget-judge-run-clear")).toBeNull();
  await waitFor(has("目前顯示程式預設 130 秒；下次呼叫將使用此設定。"));
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-budget-result"));
  await type("#admin-budget-judge-run-seconds", "100");
  expect(container.querySelector("#admin-budget-result")).toBeNull();
});

test("OPS-009: a successful restore remains visible when the new budget cannot be read", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/model-budgets" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_MODEL_BUDGETS, status: 200 }
        : { body: { error: "service unavailable" }, status: 503 };
    }
    if (path === "/admin/model-budgets/judge-run" && method === "DELETE") {
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-clear-note", "恢復平台預設");
  await click(button("把 評估判定 改回預設"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));

  expect(has("目前：90 秒（已調整）")()).toBe(false);
  await waitFor(has("評估判定的恢復預設請求已完成。"));
  expect(field<HTMLElement>("#admin-budget-result").textContent).toContain(
    "目前設定暫時無法重新讀取，請稍後核對。",
  );
});

test("OPS-009: refreshing a changed model timeout replaces an obsolete editable draft", async () => {
  let currentSeconds = 90;
  stub(true, (path, method) =>
    path === "/admin/model-budgets" && method === "GET"
      ? {
          body: {
            budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
              budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
            ),
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前：90 秒（已調整）"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-seconds", "100");

  currentSeconds = 120;
  await click(button("重新整理"));
  await waitFor(has("目前：120 秒（已調整）"));
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").value).toBe("120");
  expect(has("設定已變更；草稿改為最新的 120 秒，請確認後再送出。")()).toBe(true);
});

test("OPS-009: range endpoints are accepted and adjacent values are blocked", async () => {
  stub(true);
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await type("#admin-budget-judge-run-note", "reviewing deadline");

  for (const seconds of ["1", "130"]) {
    await type("#admin-budget-judge-run-seconds", seconds);
    expect(button("改 評估判定 的秒數").disabled).toBe(false);
  }
  for (const seconds of ["0", "131"]) {
    await type("#admin-budget-judge-run-seconds", seconds);
    expect(button("改 評估判定 的秒數").disabled).toBe(true);
    expect(has("要填 1 到 130 之間的整數秒")()).toBe(true);
  }
});

test("OPS-009: failed refresh hides stale settings and retry restores the list", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/model-budgets") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_MODEL_BUDGETS, status: 200 };
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取模型呼叫逾時"));
  expect(reads).toBe(2);
  expect(container.querySelector("li.download-item")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 3 && !!container.querySelector("li.download-item"));
  expect(has("暫時無法讀取模型呼叫逾時")()).toBe(false);
});

test("OPS-009: an in-flight timeout change blocks restoring the same call kind", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && method === "DELETE"
      ? { body: {}, status: 200 }
      : undefined,
  );
  const read = globalThis.fetch;
  let finishSet: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/model-budgets/judge-run") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishSet = resolve;
      });
    }
    return read(input, init);
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await click(field<HTMLElement>("#admin-budget-judge-run-set summary"));
  await click(field<HTMLElement>("#admin-budget-judge-run-clear summary"));
  await type("#admin-budget-judge-run-seconds", "100");
  await type("#admin-budget-judge-run-note", "調整等待時間");
  await type("#admin-budget-judge-run-clear-note", "恢復預設");
  await click(button("改 評估判定 的秒數"));
  await waitFor(() => finishSet !== undefined);

  expect(button("把 評估判定 改回預設").disabled).toBe(true);
  expect(has("這一種呼叫正在調整秒數，完成後才能恢復預設。")()).toBe(true);
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").readOnly).toBe(true);
  await act(async () => finishSet!(new Response("{}", { status: 200 })));
  await waitFor(has("評估判定的調整秒數請求已完成。"));

  await click(button("把 評估判定 改回預設"));
  await waitFor(has("評估判定的恢復預設請求已完成。"));
  expect(has("評估判定的調整秒數請求已完成。")()).toBe(false);
});

test("OPS-009: an empty configured-call roster names the absence", async () => {
  stub(true, (path) =>
    path === "/admin/model-budgets" ? { body: { budgets: [] }, status: 200 } : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("目前沒有可設定的模型呼叫"));
  expect(container.querySelector("li.download-item")).toBeNull();
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
  await click(field<HTMLElement>("#admin-skill-restriction summary"));
  await type("#admin-restriction-note", "terms under review");
  await click(button("設定受限"));
  await waitFor(has("受限展示：授權審查中"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
    reason: "license-review",
    note: "terms under review",
  });

  await waitFor(has("解除受限展示"));
  await waitFor(has("「PDF Summariser」已設定受限展示。"));
  expect(document.activeElement?.id).toBe("admin-governance-result");
  expect(field<HTMLTextAreaElement>("#admin-restriction-note").value).toBe("");
  expect(button("解除受限").disabled).toBe(true);
  await type("#admin-restriction-note", "cleared");
  expect(has("「PDF Summariser」已設定受限展示。")()).toBe(false);
  await click(button("解除受限"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
  await waitFor(has("沒有受限"));
  expect(has("受限展示：")()).toBe(false);
  expect(has("設定受限展示")()).toBe(true);
  await waitFor(has("「PDF Summariser」已解除受限展示。"));
  expect(field<HTMLTextAreaElement>("#admin-restriction-note").value).toBe("");
});

test("OPS-004: a confirmed redistribution remains visible when refreshing governance fails", async () => {
  let changed = false;
  stub(true, (path, method) => {
    if (path === "/admin/skills" && method === "GET")
      return changed
        ? { body: { error: "read unavailable" }, status: 503 }
        : { body: ADMIN_SKILLS, status: 200 };
    if (path === `/admin/skills/${SKILL}/redistribution` && method === "PUT") {
      changed = true;
      return { body: {}, status: 200 };
    }
    return undefined;
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("再散布判定"));
  await click(field<HTMLElement>("#admin-skill-redistribution summary"));
  await type("#admin-redistribution-note", "distribution blocked");
  await type("#admin-redistribution-value", "blocked");
  await click(button("送出判定"));

  await waitFor(has("「PDF Summariser」的再散布判定已改為「禁止再散布」。"));
  await waitFor(() => field<HTMLElement>("#admin-governance-result") === document.activeElement);
  await waitFor(() => container.querySelector("#admin-skill-redistribution") === null);
  expect(has("暫時無法讀取小工具")()).toBe(true);
});

test("an unknown restriction reason stays visible without looking unrestricted", async () => {
  stub(true, (path) =>
    path === "/admin/skills"
      ? {
          body: {
            skills: [{ ...ADMIN_SKILLS.skills[0], access_restriction: "policy-review" }],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("受限展示：其他原因（policy-review）"));
  expect(has("沒有受限")()).toBe(false);
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
  expect(has("重新上架須先完成審查")()).toBe(true);
  expect(
    field<HTMLLIElement>("li.download-item").querySelector(".badge-row")?.nextElementSibling
      ?.textContent,
  ).toContain("理由：DMCA");
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
  expect(field<HTMLElement>("#admin-halt-declare-scope").textContent).toBe(
    "本次停止範圍：整個叢集。新的 Run 將無法建立或派送；清理會停下以保留現場。",
  );
  expect(button("停止派送").classList.contains("caution")).toBe(true);
  expect(field<HTMLElement>("#admin-halt-declare-why").textContent).toBe(
    "先填宣告理由；節點名稱可以留空，代表停止整個叢集。",
  );
  expect(button("恢復派送").classList.contains("caution")).toBe(false);
  expect(button("恢復派送").disabled).toBe(true);
  await type("#admin-halt-declare-note", "escape drill");
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ note: "escape drill" });

  await type("#admin-halt-provider", "node-2");
  expect(field<HTMLElement>("#admin-halt-declare-scope").textContent).toBe(
    "本次停止範圍：節點 node-2。該節點不再接收新 Run；清理會停下以保留現場。",
  );
  expect(has("整個叢集停止派送。")()).toBe(false);
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "cleared");
  await click(button("恢復派送"));
  expect(has("觸發條件已消失")()).toBe(true);
  await click(button("確認恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
});

test("OPS-005: a released halt leaves a focused, verified result after its form disappears", async () => {
  let released = false;
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      return { body: released ? { dispatching: true, halts: [] } : ADMIN_DISPATCH, status: 200 };
    }
    if (path === "/admin/dispatch/halt" && method === "DELETE") {
      released = true;
      return { body: undefined, status: 204 };
    }
    return undefined;
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("沒有生效中的煞車"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.textContent).toContain("整個叢集煞車的請求已完成");
  expect(result.textContent).toContain("已不在生效中的煞車清單");
  expect(result.classList.contains("notice-success")).toBe(true);
  expect(document.activeElement).toBe(result);
  expect(container.querySelector("#admin-halt-lift-target")).toBeNull();
});

test("OPS-005: a completed release remains visible when status reread fails, then verifies on retry", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/dispatch" && method === "GET") {
      reads += 1;
      return reads === 1
        ? { body: ADMIN_DISPATCH, status: 200 }
        : reads === 2
          ? { body: { error: "service unavailable" }, status: 503 }
          : { body: { dispatching: true, halts: [] }, status: 200 };
    }
    if (path === "/admin/dispatch/halt" && method === "DELETE") {
      return { body: undefined, status: 204 };
    }
    return undefined;
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("暫時無法讀取派送狀態"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.textContent).toContain("暫勿假定已恢復派送");
  expect(result.classList.contains("notice-warning")).toBe(true);
  expect(document.activeElement).toBe(result);
  expect(has("sandbox escape suspected on node-2")()).toBe(false);
  expect(container.querySelector("#admin-halt-lift-target")).toBeNull();

  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(result.classList.contains("notice-success")).toBe(true);
  expect(result.textContent).toContain("已不在生效中的煞車清單");
  expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(1);
});

test("OPS-005: an unverified release clears when a new halt is declared", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch/halt"
      ? method === "DELETE"
        ? { body: undefined, status: 204 }
        : { body: { note: "叢集已再次停止派送。" }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));

  await waitFor(has("目前仍列有這個對象的煞車"));
  const result = field<HTMLElement>("#admin-dispatch-lift-result");
  expect(result.classList.contains("notice-warning")).toBe(true);
  expect(result.textContent).not.toContain("已不在生效中的煞車清單");

  await type("#admin-halt-declare-note", "new incident");
  await click(button("停止派送"));
  expect(container.querySelector("#admin-dispatch-lift-result")).toBeNull();
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
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/dispatch") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { dispatching: true, halts: [] }, status: 200 };
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("暫時無法讀取派送狀態"));
  expect(button("停止派送")).toBeDefined();
  expect(
    Array.from(container.querySelectorAll("button")).some(
      (item) => item.textContent === "恢復派送",
    ),
  ).toBe(false);
  await click(button("重新整理派送狀態"));
  await waitFor(has("沒有生效中的煞車"));
  expect(reads).toBe(2);
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

test("OPS-005: a pending release prevents a conflicting halt declaration", async () => {
  stub(true);
  const fetchBefore = globalThis.fetch;
  let finishRelease: ((response: Response) => void) | undefined;
  let declarations = 0;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "DELETE") {
      return new Promise<Response>((resolve) => {
        finishRelease = resolve;
      });
    }
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "PUT") {
      declarations += 1;
    }
    return fetchBefore(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await type("#admin-halt-declare-note", "new incident");
  await click(button("恢復派送"));
  await click(button("確認恢復派送"));
  await waitFor(() => Boolean(finishRelease));

  expect(button("停止派送").disabled).toBe(true);
  expect(has("正在解除煞車")()).toBe(true);
  await submit("#admin-halt-declare-note");
  expect(declarations).toBe(0);
  await act(async () => finishRelease!(new Response(null, { status: 204 })));
});

test("OPS-005: a pending halt declaration prevents a prepared release confirmation", async () => {
  stub(true);
  const fetchBefore = globalThis.fetch;
  let finishDeclaration: ((response: Response) => void) | undefined;
  let releases = 0;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishDeclaration = resolve;
      });
    }
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "DELETE") {
      releases += 1;
    }
    return fetchBefore(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-lift-target", "pool");
  await type("#admin-halt-lift-note", "incident resolved");
  await click(button("恢復派送"));
  expect(button("確認恢復派送")).toBeDefined();
  await type("#admin-halt-declare-note", "new incident");
  await click(button("停止派送"));
  await waitFor(() => Boolean(finishDeclaration));

  expect(button("確認恢復派送").disabled).toBe(true);
  expect(field<HTMLSelectElement>("#admin-halt-lift-target").disabled).toBe(true);
  expect(has("正在停止派送")()).toBe(true);
  expect(releases).toBe(0);
  await act(async () =>
    finishDeclaration!(
      new Response(JSON.stringify({ note: "stop recorded" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
});

test("OPS-005: the rosters page is read-only", async () => {
  stub(true);
  await mountAt("/admin/rosters");
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(field("main code").textContent).toBe("u-1");
  expect(container.querySelectorAll("main :is(input, textarea, select)")).toHaveLength(0);
  expect(button("重新整理")).toBeDefined();
});

test("OPS-005: unreadable rosters hide cached membership and recover on retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/rosters") return undefined;
    reads += 1;
    return reads === 1 || reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_ROSTERS, status: 200 };
  });
  await mountAt("/admin/rosters");
  await waitFor(has("暫時無法讀取名冊"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("每一個登入的帳號都算受邀")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(has("名冊清單上次取得於")()).toBe(true);
  await click(button("重新整理"));
  await waitFor(has("先前載入的名冊已隱藏"));
  expect(has("每一個登入的帳號都算受邀")()).toBe(false);
  expect(has("目前 1 位")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(reads).toBe(4);
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
  expect(table.tBodies[0].rows[0].querySelector('[data-label="對象"]')?.textContent).toContain(
    "工作區：ws-2",
  );
  expect(field("td details summary").textContent).toBe("3 項");
  expect(field("td details").textContent).toContain("beta reward");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);
});

test("audit events without a workspace name its absence", async () => {
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? { body: { events: [{ ...ADMIN_AUDIT_LOG.events[0], workspace_id: null }] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("授予點數"));
  expect(field<HTMLElement>('[data-label="對象"]').textContent).toContain("工作區：不適用");
});

test("a failed audit refresh hides old rows until retry succeeds", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 2);
  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取動作紀錄"));
  expect(reads).toBe(2);
  expect(container.querySelector("tbody tr")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 3 && container.querySelectorAll("tbody tr").length === 2);
  expect(has("暫時無法讀取動作紀錄")()).toBe(false);
});

test("an initial audit read failure offers retry without claiming prior data", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(has("暫時無法讀取動作紀錄"));
  expect(has("請稍後再試")()).toBe(true);
  expect(has("先前載入的內容已隱藏")()).toBe(false);
  expect(container.querySelector("table")).toBeNull();

  await click(button("再試一次"));
  await waitFor(() => reads === 2 && container.querySelectorAll("tbody tr").length === 2);
});

test("a lost operator session hides previously loaded audit rows", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    reads += 1;
    return reads === 2
      ? { body: { error: "not authenticated" }, status: 401 }
      : { body: ADMIN_AUDIT_LOG, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 2);
  await click(button("重新整理"));
  await waitFor(has("動作紀錄需要登入"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("授予點數")()).toBe(false);
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
      ? {
          body: {
            events: Array.from({ length: Math.min(total, 50) }, () => event),
            ...(total > 50 ? { next_cursor: "next-50" } : {}),
          },
          status: 200,
        }
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
      .every((c) => c.url === "/admin/audit-log?limit=50"),
  ).toBe(true);
});

test("a failed next audit page keeps loaded rows and retries only that page", async () => {
  const event = ADMIN_AUDIT_LOG.events[0];
  const cursor = "2026-09-10T08:00:00Z_50";
  let nextPageReads = 0;
  stub(true, (_path, _method, url) => {
    if (!url.startsWith("/admin/audit-log?")) return undefined;
    if (!new URL(url, "http://localhost").searchParams.has("cursor")) {
      return {
        body: { events: Array.from({ length: 50 }, () => event), next_cursor: cursor },
        status: 200,
      };
    }
    nextPageReads += 1;
    expect(new URL(url, "http://localhost").searchParams.get("cursor")).toBe(cursor);
    return nextPageReads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { events: [ADMIN_AUDIT_LOG.events[1]] }, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  await click(button("載入更多"));
  await waitFor(has("後續紀錄暫時無法讀取"));
  expect(container.querySelectorAll("tbody tr")).toHaveLength(50);
  expect(has("已載入 50 筆動作紀錄；還有更多")()).toBe(true);
  await click(button("重試載入更多"));
  await waitFor(() => container.querySelectorAll("tbody tr").length === 51);
  expect(nextPageReads).toBe(2);
  expect(has("後續紀錄暫時無法讀取")()).toBe(false);
  expect(calls.filter((call) => call.url.includes("/admin/audit-log?"))).toHaveLength(3);
});

test("a lost operator session on the next audit page hides loaded rows", async () => {
  const event = ADMIN_AUDIT_LOG.events[0];
  stub(true, (_path, _method, url) => {
    if (!url.startsWith("/admin/audit-log?")) return undefined;
    return url.includes("cursor=next-50")
      ? { body: { error: "not authenticated" }, status: 401 }
      : {
          body: { events: Array.from({ length: 50 }, () => event), next_cursor: "next-50" },
          status: 200,
        };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  await click(button("載入更多"));
  await waitFor(has("動作紀錄需要登入"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("重試載入更多")()).toBe(false);
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

test("OPS-007: unreadable cost statistics hide cached figures and recover on retry", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/cost-statistics") return undefined;
    reads += 1;
    return reads === 1 || reads === 3
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: ADMIN_COST_STATISTICS, status: 200 };
  });
  await mountAt("/admin/cost-statistics");
  await waitFor(has("暫時無法讀取成本統計"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(container.querySelector("table")).toBeNull();

  await click(button("再試一次"));
  await waitFor(has("搜尋理由"));
  expect(has("成本統計清單上次取得於")()).toBe(true);
  await click(button("重新整理"));
  await waitFor(has("先前載入的數字已隱藏"));
  expect(container.querySelector("table")).toBeNull();
  expect(has("$0.0090")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("搜尋理由"));
  expect(reads).toBe(4);
});

test("OPS-007: nonzero micro-dollar costs remain distinct from zero and missing values", () => {
  expect(usd(null)).toBe("未測量");
  expect(usd(0)).toBe("$0.0000");
  expect(usd(1)).toBe("$0.000001");
  expect(usd(49)).toBe("$0.000049");
  expect(usd(50)).toBe("$0.00005");
  expect(usd(100)).toBe("$0.0001");
  expect(usd(1_234_567)).toBe("$1.234567");
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

test("OPS-008: a failed trend hides its cached chart while other trends remain usable and refresh recovers it", async () => {
  let creditReads = 0;
  stub(true, (path) => {
    if (path !== "/admin/trends/credits") return undefined;
    creditReads += 1;
    return creditReads === 2 ? { body: { error: "service unavailable" }, status: 503 } : undefined;
  });
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));

  const credits = Array.from(container.querySelectorAll("section")).find(
    (section) => section.querySelector("h2")?.textContent === "每日點數異動（淨額）",
  )!;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: queryKeys.admin.trend("credits", 30) });
  });
  await waitFor(() => credits.querySelector('[role="alert"]') !== null);
  expect(credits.textContent).toContain("先前載入的趨勢已隱藏");
  expect(credits.textContent).not.toContain("全平台目前餘額總和：1268 點。");
  expect(credits.querySelector("figure")).toBeNull();
  expect(has("評審：3 筆，合計 $0.0036")()).toBe(true);
  expect(has("已取得 4/5 組趨勢")()).toBe(true);

  await click(button("重新整理五組趨勢"));
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(creditReads).toBe(3);
  expect(has("已取得 5/5 組趨勢")()).toBe(true);
});

test("OPS-008: losing all trend reads hides the cached range until refresh succeeds", async () => {
  let unavailable = false;
  stub(true, (path) =>
    unavailable && path.startsWith("/admin/trends/")
      ? { body: { error: "service unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));

  unavailable = true;
  await act(async () => {
    await queryClient.refetchQueries({ queryKey: ["admin", "trends"] });
  });
  await waitFor(has("目前沒有可用趨勢。"));
  expect(has("2026-09-06 到 2026-09-12（UTC），共 7 天。")()).toBe(false);
  expect(container.querySelectorAll("figure")).toHaveLength(0);

  unavailable = false;
  await click(button("重新整理五組趨勢"));
  await waitFor(has("全平台目前餘額總和：1268 點。"));
  expect(has("2026-09-06 到 2026-09-12（UTC），共 7 天。")()).toBe(true);
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
  const funnelSection = Array.from(container.querySelectorAll("section")).find(
    (section) => section.querySelector("h2")?.textContent === "漏斗各段每天到達的數量",
  )!;
  expect(
    funnelSection
      .querySelector("dl")!
      .compareDocumentPosition(funnelSection.querySelector("figure")!) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
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
  const absent = Array.from(container.querySelectorAll("details")).find((details) =>
    details.textContent?.includes("這段期間沒有事件：創作步驟"),
  )!;
  expect(absent.open).toBe(false);
  expect(absent.querySelector("summary")?.textContent).toBe("這段期間有 9 類沒有事件");
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

test("OPS-008: exactly the queried range is marked as selected after navigation and fallback", async () => {
  stub(true);
  await mountAt("/admin/trends");
  const selectedRange = () =>
    Array.from(container.querySelectorAll('nav[aria-label="時間範圍"] a[aria-current="page"]')).map(
      (link) => link.textContent,
    );
  await waitFor(() => selectedRange().join() === "30 天");
  expect(selectedRange()).toEqual(["30 天"]);

  await go("/admin/trends", { days: "7" });
  await waitFor(() => selectedRange().join() === "7 天");
  expect(selectedRange()).toEqual(["7 天"]);

  await go("/admin/trends", { days: "8" });
  await waitFor(() => selectedRange().join() === "30 天");
  expect(selectedRange()).toEqual(["30 天"]);
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
  expect(has("待審：共 1 筆。")()).toBe(true);
  expect(has("這份待審清單上次取得於")()).toBe(true);
  expect(field<HTMLAnchorElement>(`a[aria-label="審核 ${EXPOSURE_PUBLICATION}"]`).textContent).toBe(
    "審這一筆",
  );

  await click(
    Array.from(container.querySelectorAll("a")).find((a) => a.textContent === "審這一筆")!,
  );
  await waitFor(has("目前未曝光"));
  expect(container.querySelector("h2")?.textContent).toBe(`審這一筆：${EXPOSURE_PUBLICATION}`);
  expect(has("待審清單")()).toBe(true);
  expect(has("曾核准，之後內容有變，需要重新審核。")()).toBe(false);
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

  await click(
    Array.from(container.querySelectorAll("a")).find((a) => a.textContent === "返回待審清單")!,
  );
  await waitFor(has("曾核准，之後內容有變，需要重新審核。"));
  expect(new URLSearchParams(window.location.search).has("publication")).toBe(false);
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
  await waitFor(has("這筆曝光審核已核准。"));

  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]'));
  expect(has("這筆曝光審核已核准。")()).toBe(false);
});

test("a completed exposure review keeps its result when the server advances the review sequence", async () => {
  const updated = {
    ...ADMIN_EXPOSURE_CASE,
    sequence: 3,
    exposed: true,
    history: [
      { ...ADMIN_EXPOSURE_CASE.history[0], sequence: 3, reason: "看過了，符合規範" },
      ...ADMIN_EXPOSURE_CASE.history,
    ],
  };
  let reviewed = false;
  stub(true, (path, method) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    if (method === "POST") {
      reviewed = true;
      return { body: updated, status: 200 };
    }
    return { body: reviewed ? updated : ADMIN_EXPOSURE_CASE, status: 200 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("審核序號：3"));
  expect(has("目前曝光中")()).toBe(true);
  expect(has("這筆曝光審核已核准。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-exposure-result"));
});

test("editing a new exposure reason clears the previous review result", async () => {
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
  await waitFor(has("這筆曝光審核已核准。"));
  await type("#admin-exposure-review-note", "需要重新確認");
  expect(has("這筆曝光審核已核准。")()).toBe(false);
});

test("a completed exposure review stays confirmed when rereading the current state fails", async () => {
  const updated = { ...ADMIN_EXPOSURE_CASE, sequence: 3, exposed: true };
  let reviewed = false;
  stub(true, (path, method) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    if (method === "POST") {
      reviewed = true;
      return { body: updated, status: 200 };
    }
    return reviewed
      ? { body: { error: "refresh unavailable" }, status: 503 }
      : { body: ADMIN_EXPOSURE_CASE, status: 200 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "看過了，符合規範");
  await click(button("送出核准"));
  await waitFor(has("暫時無法讀取這一筆的曝光審核資料"));
  expect(has("這筆曝光審核已核准。")()).toBe(true);
  expect(document.activeElement).toBe(field<HTMLElement>("#admin-exposure-result"));
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

test("DISC-007: a failed queue refresh hides stale work and retry restores the latest queue", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/exposure-reviews") return undefined;
    reads += 1;
    if (reads === 2) return { body: { error: "service unavailable" }, status: 503 };
    return reads === 1 ? platformResponse(path) : { body: { publications: [] }, status: 200 };
  });
  await mountAt("/admin/exposure");
  await waitFor(has("待審：共 1 筆。"));

  await click(button("重新整理"));
  await waitFor(has("暫時無法讀取待審清單"));
  expect(container.querySelectorAll(".download-item")).toHaveLength(0);
  expect(has("先前載入的內容已隱藏。")()).toBe(true);

  await click(button("再試一次"));
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(reads).toBe(3);
});

test("DISC-007: an initial queue failure offers retry without claiming the queue is empty", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/exposure-reviews") return undefined;
    reads += 1;
    return reads === 1
      ? { body: { error: "service unavailable" }, status: 503 }
      : { body: { publications: [] }, status: 200 };
  });
  await mountAt("/admin/exposure");
  await waitFor(has("暫時無法讀取待審清單"));
  expect(has("請稍後再試。")()).toBe(true);
  expect(has("沒有等待審核的發佈物：0 筆。")()).toBe(false);

  await click(button("再試一次"));
  await waitFor(has("沒有等待審核的發佈物：0 筆。"));
  expect(reads).toBe(2);
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
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            snapshot: undefined,
            approval: {
              allowed: false,
              refusal: {
                reason: "snapshot_not_current",
                error: "搜尋索引裡的內容不是這一筆 Release 的版本",
              },
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("尚未進索引"));
  expect(has(ADMIN_EXPOSURE_CASE.snapshot.enriched_summary)()).toBe(false);
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(has("搜尋索引裡的內容不是這一筆 Release 的版本")()).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test.each([
  [
    "搜尋索引仍指向另一版",
    { ...ADMIN_EXPOSURE_CASE.snapshot, current: false },
    "snapshot_not_current",
    "搜尋索引裡的內容不是這一筆 Release 的版本",
  ],
  [
    "搜尋內容還在補充",
    { ...ADMIN_EXPOSURE_CASE.snapshot, enriched: false },
    "snapshot_pending",
    "這一版的搜尋內容還在補充",
  ],
])("DISC-007: %s時不能核准曝光", async (_condition, snapshot, reason, error) => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            snapshot,
            approval: { allowed: false, refusal: { reason, error } },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(has(error)()).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test("DISC-007: a withdrawn publication cannot be approved again", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? {
          body: {
            ...ADMIN_EXPOSURE_CASE,
            status: "delisted",
            approval: {
              allowed: false,
              refusal: { reason: "not_published", error: "這個發佈物已經撤回，不能核准曝光" },
            },
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("這個發佈物已經撤回，不能核准曝光"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
});

test.each([
  [
    "redistribution_not_allowed",
    "可散布判定不是 allowed：要先以既有的可散布判定動詞附授權證據判成 allowed，才能核准曝光",
  ],
  ["not_available", "這個 Skill 目前被下架、被保留或已刪除，不能核准曝光"],
])(
  "DISC-007: %s blocks approval before submission and preserves revocation",
  async (reason, error) => {
    stub(true, (path) =>
      path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
        ? {
            body: {
              ...ADMIN_EXPOSURE_CASE,
              approval: { allowed: false, refusal: { reason, error } },
            },
            status: 200,
          }
        : undefined,
    );
    await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
    await waitFor(has("審核這一版"));
    expect(has(error)()).toBe(true);
    const approved = field<HTMLInputElement>('input[value="approved"]');
    expect(approved.disabled).toBe(true);
    expect(approved.getAttribute("aria-describedby")).toBe("admin-exposure-approval-why");
    expect(field<HTMLInputElement>('input[value="revoked"]').disabled).toBe(false);
  },
);

test("DISC-007: missing approval evidence does not make approval available", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? { body: { ...ADMIN_EXPOSURE_CASE, approval: undefined }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("無法確認核准資格；請重新整理審核資料。"));
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
});

test("DISC-007: changed approval evidence clears a draft based on the old eligibility", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return {
      body:
        reads === 1
          ? ADMIN_EXPOSURE_CASE
          : {
              ...ADMIN_EXPOSURE_CASE,
              approval: {
                allowed: false,
                refusal: { reason: "not_available", error: "這個 Skill 目前被保留，不能核准曝光" },
              },
            },
      status: 200,
    };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[value="approved"]'));
  await type("#admin-exposure-review-note", "原先看到的資料");

  await click(button("重新整理審核資料"));
  await waitFor(has("這個 Skill 目前被保留，不能核准曝光"));
  expect(field<HTMLInputElement>('input[value="approved"]').checked).toBe(false);
  expect(field<HTMLInputElement>('input[value="approved"]').disabled).toBe(true);
  expect(field<HTMLTextAreaElement>("#admin-exposure-review-note").value).toBe("");
});

test("DISC-007: a failed refresh hides a previously loaded case and its approval control", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return reads === 1
      ? { body: ADMIN_EXPOSURE_CASE, status: 200 }
      : { body: { error: "service unavailable" }, status: 503 };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(button("重新整理審核資料"));
  await waitFor(has("暫時無法讀取這一筆的曝光審核資料"));
  expect(reads).toBe(2);
  expect(container.querySelector('input[value="approved"]')).toBeNull();
});

test("DISC-007: a new review premise clears the previous decision and reason", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`) return undefined;
    reads += 1;
    return {
      body: {
        ...ADMIN_EXPOSURE_CASE,
        sequence: reads === 1 ? 2 : 3,
        release: {
          ...ADMIN_EXPOSURE_CASE.release,
          version_number: reads === 1 ? 1 : 2,
        },
      },
      status: 200,
    };
  });
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核序號：2"));
  await click(field<HTMLInputElement>('input[value="approved"]'));
  await type("#admin-exposure-review-note", "已看過舊版");
  expect(button("送出核准").disabled).toBe(false);

  await click(button("重新整理審核資料"));
  await waitFor(has("審核序號：3"));
  expect(field<HTMLInputElement>('input[value="approved"]').checked).toBe(false);
  expect(field<HTMLTextAreaElement>("#admin-exposure-review-note").value).toBe("");
  expect(button("送出審核結論").disabled).toBe(true);
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
      await waitFor(has("暫時無法讀取這件事。請重新整理，或稍後再試。"));
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
  await click(field<HTMLInputElement>('input[name="admin-finding-move"][value="resolved"]'));
  await type("#admin-finding-note", "fixed");
  await submit("#admin-finding-note");
  await waitFor(has("the finding cannot move to that status from where it is now"));
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
  await waitFor(has("the proposal is no longer waiting for a decision"));
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
  await waitFor(has("the finding cannot move to that status"));
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
