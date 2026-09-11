import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { usd } from "./api/admin";
import { ADMIN_AUDIT_LOG, ADMIN_SKILLS, SKILL, platformResponse } from "./fixtures/platform";

type Call = { method: string; url: string; body?: Record<string, unknown> };
type Reply = { body: unknown; status: number } | undefined;

let container: HTMLDivElement;
let root: Root;
let calls: Call[];

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
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
  override: (path: string, method: string) => Reply = () => undefined,
) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const { body, status } = override(path, method) ?? platformResponse(url);
    const payload = path === "/me" ? { ...(body as object), operator } : body;
    return Promise.resolve(
      new Response(JSON.stringify(payload), {
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
        <App />
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

async function waitFor(done: () => boolean, timeoutMs = 4000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
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

test("OPS-001: a member who types an /admin address gets the missing page and no admin request", async () => {
  stub(false);
  await mountAt("/");
  for (const path of ADMIN_PATHS) {
    await go(path);
    await waitFor(has("這一頁現在不存在"));
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
  await waitFor(has("輸入 email 找帳號"));
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
  });
  await waitFor(() => ledgerReads() > before);
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
  stub(true);
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
  await waitFor(has("已下架"));
  expect(has("理由：DMCA")()).toBe(true);
  expect(has("的動作")()).toBe(false);
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
  await type("#admin-halt-declare-note", "escape drill");
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ note: "escape drill" });

  await type("#admin-halt-provider", "node-2");
  await type("#admin-halt-lift-note", "cleared");
  await click(button("恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({
    note: "cleared",
    provider: "node-2",
  });
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
  expect(field("td details summary").textContent).toBe("3 項");
  expect(field("td details").textContent).toContain("beta reward");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);
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
