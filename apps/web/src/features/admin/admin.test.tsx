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
  ADMIN_ACCOUNT,
  ADMIN_AUDIT_LOG,
  ADMIN_DISPATCH,
  ADMIN_EXPOSURE_CASE,
  ADMIN_MODEL_BUDGETS,
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

test("OPS-002: submitting the same email again reads the current account", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/accounts") return undefined;
    reads += 1;
    return {
      body: { ...ADMIN_ACCOUNT, display_name: reads === 1 ? "封測者甲" : "封測者乙" },
      status: 200,
    };
  });
  await lookUp("member@example.com");
  await waitFor(has("封測者甲"));
  const initialReads = reads;

  await submit("#admin-account-email");
  await waitFor(has("封測者乙"));
  expect(reads).toBe(initialReads + 1);
  expect(window.location.search).not.toContain("member");
});

test("OPS-003: a failed ledger refresh hides the cached balance and entries", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/credits/ws-2" && unavailable
      ? { body: { error: "ledger unavailable" }, status: 503 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("目前餘額"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "welcome credit");
  expect(button("授予").disabled).toBe(false);

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.ledger("ws-2") });
  });
  await waitFor(has("暫時無法讀取點數"));
  expect(has("目前餘額")()).toBe(false);
  expect(container.querySelectorAll("tbody tr")).toHaveLength(0);
  expect(button("授予").disabled).toBe(true);
  expect(has("先讀到目前點數狀態，才能授予。")()).toBe(true);
  await submit("#admin-grant-note");
  expect(calls.some((call) => call.method === "POST")).toBe(false);
});

test("a pending ledger refresh blocks a grant until the current balance is known", async () => {
  stub(true);
  const fixtureFetch = globalThis.fetch;
  let reads = 0;
  let finishRead: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/credits/ws-2") && (init?.method ?? "GET") === "GET") {
      reads += 1;
      if (reads > 1) {
        return new Promise<Response>((resolve) => {
          finishRead = resolve;
        });
      }
    }
    return fixtureFetch(input, init);
  });
  await lookUp("member@example.com");
  await waitFor(has("目前餘額"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "welcome credit");
  expect(button("授予").disabled).toBe(false);

  await act(async () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.admin.ledger("ws-2") });
  });
  await waitFor(() => finishRead !== undefined);
  await waitFor(() => button("授予").disabled);
  await submit("#admin-grant-note");
  expect(calls.some((call) => call.method === "POST")).toBe(false);

  await act(async () => {
    finishRead!(
      new Response(JSON.stringify(platformResponse("/admin/credits/ws-2").body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await waitFor(() => !button("授予").disabled);
});

test("OPS-002: an email nobody has is named as such, not reported as a broken read", async () => {
  stub(true, (path) =>
    path === "/admin/accounts" ? { body: { error: "not found" }, status: 404 } : undefined,
  );
  await lookUp("ghost@example.com");
  await waitFor(has("沒有 email 是「ghost@example.com」的帳號"));
  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("editing a new account query hides the previous account and its grant form", async () => {
  stub(true);
  await lookUp("member@example.com");
  await waitFor(has("授予點數"));
  await type("#admin-grant-amount", "50");
  await type("#admin-grant-note", "welcome credit");
  expect(button("授予").disabled).toBe(false);

  await type("#admin-account-email", "next@example.com");
  expect(container.querySelector("#admin-grant-amount")).toBeNull();
  expect(has("封測者甲")()).toBe(false);
  expect(has("查詢條件已變更")()).toBe(true);
  expect(calls.some((c) => c.method === "POST")).toBe(false);
});

test("OPS-002: a failed account refresh hides cached identity and its grant form", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/accounts" && unavailable
      ? { body: { error: "account unavailable" }, status: 503 }
      : undefined,
  );
  await lookUp("member@example.com");
  await waitFor(has("封測者甲"));
  await waitFor(has("授予點數"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.admin.account("member@example.com"),
    });
  });
  await waitFor(has("暫時無法讀取帳號"));
  expect(has("封測者甲")()).toBe(false);
  expect(container.querySelector("#admin-grant-amount")).toBeNull();
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
  await waitFor(has("已授予 50 點，授予時餘額為 170 點。"));
  expect(calls.find((c) => c.method === "POST")?.body).toEqual({
    amount_credits: 50,
    reason: "beta reward",
    idempotency_key: expect.stringMatching(/^[0-9a-f-]{36}$/),
  });
  await waitFor(() => ledgerReads() > before);
  await type("#admin-grant-amount", "75");
  expect(has("已授予 50 點，授予時餘額為 170 點。")()).toBe(false);
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
  await waitFor(has("這個動作沒有完成，請稍後再試"));
  expect(has("grant failed")()).toBe(false);
  await click(button("授予"));
  await waitFor(has("已授予 10 點，授予時餘額為 60 點。"));
  await click(button("授予"));
  await waitFor(() => calls.filter((c) => c.method === "POST").length === 3);
  const keys = calls
    .filter((c) => c.method === "POST")
    .map((c) => (c.body as { idempotency_key: string }).idempotency_key);
  expect(keys[1]).toBe(keys[0]);
  expect(keys[2]).not.toBe(keys[0]);
});

test("OPS-003: an English refusal stays internal and editing clears the fallback", async () => {
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
  await waitFor(has("這個動作沒有完成，請稍後再試"));
  expect(has("amount_credits must not be zero")()).toBe(false);
  await type("#admin-grant-note", "修改後的理由");
  expect(has("這個動作沒有完成，請稍後再試")()).toBe(false);
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
  expect(has(`PDF Summariser（${SKILL}，工作區 ws-2）`)()).toBe(true);
  expect(has("理由：DMCA notice")()).toBe(true);
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
  await click(button("確認下架"));
  await waitFor(() => calls.some((c) => c.method === "PUT"));
  expect(calls.find((c) => c.method === "PUT")).toEqual({
    method: "PUT",
    url: `/admin/skills/${SKILL}/takedown`,
    body: { reason: "DMCA notice" },
  });
});

test("OPS-004: editing the skill search hides the old result and actions until submitted", async () => {
  stub(true);
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  await type("#admin-skill-q", "another skill");
  expect(has("查詢條件已變更")()).toBe(true);
  expect(has("PDF Summariser")()).toBe(false);
  expect(has("的動作")()).toBe(false);
  expect(calls.some((call) => call.url === "/admin/skills?q=another%20skill")).toBe(false);

  await submit("#admin-skill-q");
  await waitFor(() => calls.some((call) => call.url === "/admin/skills?q=another%20skill"));
  expect(field<HTMLInputElement>("#admin-skill-q").value).toBe("another skill");
  await go("/admin/skills", { q: SKILL });
  expect(field<HTMLInputElement>("#admin-skill-q").value).toBe(SKILL);
});

test("OPS-004: submitting the same skill query again reads current governance", async () => {
  let reads = 0;
  stub(true, (path) => {
    if (path !== "/admin/skills") return undefined;
    reads += 1;
    return {
      body: {
        skills: ADMIN_SKILLS.skills.map((skill) => ({
          ...skill,
          access_restriction: reads === 1 ? null : "license-review",
        })),
      },
      status: 200,
    };
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("設定受限展示"));
  const initialReads = reads;

  await submit("#admin-skill-q");
  await waitFor(has("解除受限展示"));
  expect(reads).toBe(initialReads + 1);
});

test("OPS-004: governance actions disappear while a cached result is refreshing", async () => {
  stub(true);
  const fixtureFetch = globalThis.fetch;
  let reads = 0;
  let finishRead: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).startsWith("/admin/skills?")) {
      reads += 1;
      if (reads > 1) {
        return new Promise<Response>((resolve) => {
          finishRead = resolve;
        });
      }
    }
    return fixtureFetch(input, init);
  });
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  await act(async () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.admin.skillSearch(SKILL) });
  });
  await waitFor(() => finishRead !== undefined);
  await waitFor(() => !has("對「PDF Summariser」的動作")());
  expect(has("載入小工具中")()).toBe(true);

  await act(async () => {
    finishRead!(
      new Response(JSON.stringify(ADMIN_SKILLS), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await waitFor(has("對「PDF Summariser」的動作"));
});

test("OPS-004: a failed skill refresh cannot leave cached governance actions available", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/skills" && unavailable
      ? { body: { error: "search unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/skills", { q: SKILL });
  await waitFor(has("對「PDF Summariser」的動作"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.skillSearch(SKILL) });
  });
  await waitFor(has("暫時無法讀取小工具"));
  expect(has("PDF Summariser")()).toBe(false);
  expect(has("的動作")()).toBe(false);
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

test("OPS-009: a configured timeout keeps its compiled default visible without failure styling", async () => {
  stub(true);
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));

  const rows = Array.from(container.querySelectorAll("li.download-item"));
  const configured = rows.find((row) => row.textContent?.includes("評估判定"));
  expect(configured?.textContent).toContain("程式預設 130 秒");
  expect(configured?.textContent).toContain("管理員設定 90 秒");
  expect(configured?.querySelector(".badge-danger")).toBeNull();

  const unconfigured = rows.find((row) => row.textContent?.includes("搜尋結果的推薦理由"));
  expect(unconfigured?.textContent).toContain("程式預設 8 秒");
  expect(unconfigured?.textContent).not.toContain("管理員設定");
});

test("OPS-009: a refreshed model timeout replaces the stale edit value", async () => {
  let current = ADMIN_MODEL_BUDGETS;
  stub(true, (path) =>
    path === "/admin/model-budgets" ? { body: current, status: 200 } : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(
    () =>
      container.querySelector<HTMLInputElement>("#admin-budget-judge-run-seconds")?.value === "90",
  );
  await type("#admin-budget-judge-run-seconds", "100");

  current = {
    budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
      budget.kind === "judge-run"
        ? { ...budget, seconds: 60, reason: "new operator setting", set_at: "2026-09-20T08:00:00Z" }
        : budget,
    ),
  };
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.modelBudgets });
  });
  await waitFor(
    () =>
      container.querySelector<HTMLInputElement>("#admin-budget-judge-run-seconds")?.value === "60",
  );
  expect(has("60 秒")()).toBe(true);
});

test("OPS-009: a failed budget refresh hides cached settings and actions", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/model-budgets" && unavailable
      ? { body: { error: "budgets unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.modelBudgets });
  });
  await waitFor(has("暫時無法讀取模型呼叫逾時"));
  expect(has("評估判定")()).toBe(false);
  expect(container.querySelector("#admin-budget-judge-run-seconds")).toBeNull();
});

test("OPS-009: setting a budget locks clearing the same kind until the write finishes", async () => {
  stub(true);
  const fixtureFetch = globalThis.fetch;
  let finishWrite: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/model-budgets/judge-run") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishWrite = resolve;
      });
    }
    return fixtureFetch(input, init);
  });
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await type("#admin-budget-judge-run-note", "wait longer");
  await type("#admin-budget-judge-run-clear-note", "return to default");
  await click(button("改 評估判定 的秒數"));
  await waitFor(() => finishWrite !== undefined);
  const clearButton = field<HTMLTextAreaElement>("#admin-budget-judge-run-clear-note")
    .closest("form")!
    .querySelector<HTMLButtonElement>('button[type="submit"]')!;
  await waitFor(() => clearButton.disabled);
  expect(has("此呼叫正在設定秒數，完成後才能改回預設。")()).toBe(true);
  expect(field<HTMLInputElement>("#admin-budget-judge-run-seconds").readOnly).toBe(true);
  await click(clearButton);
  expect(calls.some((call) => call.method === "DELETE")).toBe(false);

  await act(async () => {
    finishWrite!(new Response("{}", { status: 200 }));
  });
  await waitFor(has("已套用，下一次呼叫就用這個秒數。"));
});

test("OPS-009: clearing a budget removes the preceding set-success message", async () => {
  stub(true, (path, method) =>
    path === "/admin/model-budgets/judge-run" && (method === "PUT" || method === "DELETE")
      ? { body: {}, status: 200 }
      : undefined,
  );
  await mountAt("/admin/model-budgets");
  await waitFor(has("評估判定"));
  await type("#admin-budget-judge-run-note", "wait longer");
  await click(button("改 評估判定 的秒數"));
  await waitFor(has("已套用，下一次呼叫就用這個秒數。"));

  await type("#admin-budget-judge-run-clear-note", "return to default");
  await click(button("把 評估判定 改回預設"));
  await waitFor(has("已改回預設。"));
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
  expect(
    Array.from(container.querySelectorAll("main h2"), (heading) => heading.textContent),
  ).toEqual(["停止派送", "恢復派送"]);
  expect(has("P1 事故：只有人能解除")()).toBe(true);
  expect(has("整個叢集")()).toBe(true);
  expect(button("停止派送").classList.contains("caution")).toBe(true);
  expect(button("恢復派送").classList.contains("caution")).toBe(false);
  await type("#admin-halt-declare-note", "escape drill");
  expect(has("本次停止範圍：整個叢集")()).toBe(true);
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  expect(calls.find((c) => c.method === "PUT")?.body).toEqual({ note: "escape drill" });

  await type("#admin-halt-recovery-target", "pool");
  await type("#admin-halt-provider", "node-2");
  expect(has("整個叢集停止派送。")()).toBe(false);
  expect(has("本次停止範圍：節點 node-2")()).toBe(true);
  await type("#admin-halt-lift-note", "cleared");
  await click(button("恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({ note: "cleared" });
});

test("a pending dispatch halt prevents recovery from starting", async () => {
  stub(true);
  const fixtureFetch = globalThis.fetch;
  let finishHalt: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "PUT") {
      return new Promise<Response>((resolve) => {
        finishHalt = resolve;
      });
    }
    return fixtureFetch(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-recovery-target", "pool");
  await type("#admin-halt-declare-note", "stop for investigation");
  await type("#admin-halt-lift-note", "resume after investigation");
  await click(button("停止派送"));
  await waitFor(() => finishHalt !== undefined);

  await waitFor(() => button("恢復派送").disabled);
  await waitFor(has("正在停止派送，完成後才能恢復。"));
  await submit("#admin-halt-lift-note");
  expect(calls.some((call) => call.method === "DELETE")).toBe(false);

  await act(async () => {
    finishHalt!(new Response(JSON.stringify({ note: "整個叢集停止派送。" }), { status: 200 }));
  });
  await waitFor(has("整個叢集停止派送。"));
});

test("recovering dispatch blocks another halt and clears the preceding halt notice", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch/halt" && method === "PUT"
      ? { body: { note: "整個叢集停止派送。" }, status: 200 }
      : undefined,
  );
  const fixtureFetch = globalThis.fetch;
  let finishRecovery: ((response: Response) => void) | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/admin/dispatch/halt") && init?.method === "DELETE") {
      return new Promise<Response>((resolve) => {
        finishRecovery = resolve;
      });
    }
    return fixtureFetch(input, init);
  });
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-recovery-target", "pool");
  await type("#admin-halt-declare-note", "stop for investigation");
  await type("#admin-halt-lift-note", "resume after investigation");
  await click(button("停止派送"));
  await waitFor(has("整個叢集停止派送。"));
  await click(button("恢復派送"));
  await waitFor(() => finishRecovery !== undefined);

  await waitFor(() => button("停止派送").disabled);
  await waitFor(has("正在恢復派送，完成後才能再次停止。"));
  expect(has("整個叢集停止派送。")()).toBe(false);
  await submit("#admin-halt-declare-note");
  expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);

  await act(async () => {
    finishRecovery!(new Response("{}", { status: 200 }));
  });
  await waitFor(has("已解除，上面的狀態已更新。"));
  expect(has("整個叢集停止派送。")()).toBe(false);
});

test("recovery requires selecting an active halt and sends that target", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch" && method === "GET"
      ? {
          body: {
            dispatching: false,
            halts: [
              ...ADMIN_DISPATCH.halts,
              { ...ADMIN_DISPATCH.halts[0], target: "node-2", reason: "node investigation" },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("node investigation"));
  await type("#admin-halt-lift-note", "verified node repair");
  expect(button("恢復派送").disabled).toBe(true);
  expect(has("先選擇目前清單中的煞車")()).toBe(true);
  expect(
    Array.from(
      field<HTMLSelectElement>("#admin-halt-recovery-target").options,
      (option) => option.value,
    ),
  ).toEqual(["", "pool", "node-2"]);

  await type("#admin-halt-recovery-target", "node-2");
  expect(button("恢復派送").disabled).toBe(false);
  await click(button("恢復派送"));
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.body).toEqual({
    note: "verified node repair",
    provider: "node-2",
  });
});

test("a halt removed by a status refresh cannot be recovered from an old selection", async () => {
  stub(true);
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-recovery-target", "pool");
  await type("#admin-halt-lift-note", "verified repair");
  expect(button("恢復派送").disabled).toBe(false);

  await act(async () => {
    queryClient.setQueryData(queryKeys.admin.dispatch, { dispatching: true, halts: [] });
  });
  await waitFor(has("煞車：0 個"));
  expect(button("恢復派送").disabled).toBe(true);
  await submit("#admin-halt-lift-note");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
});

test("recovery is unavailable when no halt exists", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch" && method === "GET"
      ? { body: { dispatching: true, halts: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("煞車：0 個"));
  await type("#admin-halt-lift-note", "nothing to lift");
  expect(button("恢復派送").disabled).toBe(true);
  expect(has("目前沒有煞車可解除")()).toBe(true);
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
});

test("dispatch status failure preserves emergency stop but blocks recovery", async () => {
  stub(true, (path, method) =>
    path === "/admin/dispatch" && method === "GET"
      ? { body: { error: "unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("暫時無法讀取派送狀態"));
  await type("#admin-halt-declare-note", "emergency");
  await type("#admin-halt-lift-note", "cleared");

  expect(button("停止派送").disabled).toBe(false);
  expect(button("恢復派送").disabled).toBe(true);
  expect(has("必須先讀到目前的派送狀態")()).toBe(true);
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
});

test("a failed dispatch refresh does not present cached halts as current", async () => {
  let fails = false;
  stub(true, (path, method) =>
    path === "/admin/dispatch" && method === "GET" && fails
      ? { body: { error: "unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/dispatch");
  await waitFor(has("sandbox escape suspected on node-2"));
  await type("#admin-halt-recovery-target", "pool");
  await type("#admin-halt-lift-note", "verified repair");

  fails = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.dispatch });
  });
  await waitFor(has("暫時無法讀取派送狀態"));
  expect(has("sandbox escape suspected on node-2")()).toBe(false);
  expect(button("恢復派送").disabled).toBe(true);
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
});

test("OPS-005: the rosters page is read-only", async () => {
  stub(true);
  await mountAt("/admin/rosters");
  await waitFor(has("每一個登入的帳號都算受邀"));
  expect(field("main code").textContent).toBe("u-1");
  expect(container.querySelectorAll("main :is(input, textarea, select, button)")).toHaveLength(0);
});

test("OPS-005: an empty operator roster is a named zero, not an empty section", async () => {
  stub(true, (path) =>
    path === "/admin/rosters"
      ? { body: { operator_user_ids: [], beta_allowlist: [] }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/rosters");
  await waitFor(has("封測名單"));
  expect(has("目前生效的 operator：0 人。")()).toBe(true);
  expect(container.querySelector("main ul")).toBeNull();
});

test("OPS-005: a failed roster refresh hides cached membership", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/rosters" && unavailable
      ? { body: { error: "roster unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/rosters");
  await waitFor(has("u-1"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.rosters });
  });
  await waitFor(has("暫時無法讀取名冊"));
  expect(container.querySelectorAll("main code")).toHaveLength(0);
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
  const labels = ["時間", "動作", "operator", "對象", "Workspace", "內容"];
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
  expect(
    Array.from(table.querySelectorAll('tbody td[data-label="Workspace"]')).map((cell) =>
      cell.textContent?.trim(),
    ),
  ).toEqual(["ws-2", "ws-2"]);
  expect(field("td details summary").textContent).toBe("3 項");
  expect(field("td details").textContent).toContain("beta reward");
  expect(
    Array.from(container.querySelectorAll("button")).some((b) => b.textContent === "載入更多"),
  ).toBe(false);
});

test("the audit log names exposure reviews and model budget changes in plain language", async () => {
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? {
          body: {
            events: [
              {
                actor_user_id: "u-1",
                action: "publication.exposure.review",
                resource_type: "publication",
                resource_id: "publication-1",
                workspace_id: null,
                occurred_at: "2026-10-06T00:00:00Z",
                metadata: { decision: "approved", reason: "reviewed" },
              },
              {
                actor_user_id: "u-1",
                action: "model_budget.set",
                resource_type: "model_budget",
                resource_id: null,
                workspace_id: null,
                occurred_at: "2026-10-06T00:00:00Z",
                metadata: { reason: "shorten timeout" },
              },
            ],
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll('tbody th[scope="row"]').length === 2);
  expect(
    Array.from(container.querySelectorAll('tbody th[scope="row"]')).map((row) => row.textContent),
  ).toEqual(["曝光審核", "設定模型呼叫逾時"]);
  expect(
    Array.from(container.querySelectorAll('tbody td[data-label="對象"]')).map((cell) =>
      cell.textContent?.trim(),
    ),
  ).toEqual(["發佈物 publication-1", "模型呼叫逾時 不適用"]);
});

test("OPS-006: a halt the platform declared by itself names the platform as the actor", async () => {
  stub(true, (path) =>
    path.startsWith("/admin/audit-log")
      ? {
          body: {
            events: [
              {
                actor_user_id: null,
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
  expect(field<HTMLElement>('tbody td[data-label="Workspace"]').textContent).toBe("不適用");
  expect(has("未測量")()).toBe(false);
});

test("OPS-006: a failed next page marks the audit list partial and can retry", async () => {
  let nextFails = true;
  const event = ADMIN_AUDIT_LOG.events[0];
  const events = Array.from({ length: 51 }, (_, index) => ({
    ...event,
    resource_id: `entry-${index}`,
  }));
  stub(true, (path) => {
    if (path !== "/admin/audit-log") return undefined;
    const before = new URLSearchParams(calls.at(-1)?.url.split("?")[1]).get("before");
    return before === "audit-page-2" && nextFails
      ? { body: { error: "audit unavailable" }, status: 503 }
      : before === "audit-page-2"
        ? { body: { events: events.slice(50) }, status: 200 }
        : { body: { events: events.slice(0, 50), next_before: "audit-page-2" }, status: 200 };
  });
  await mountAt("/admin/audit-log");
  await waitFor(() => container.querySelectorAll("tbody tr").length === 50);
  await click(button("載入更多"));
  await waitFor(has("清單不完整"));
  expect(calls.at(-1)?.url).toContain("before=audit-page-2");
  expect(container.querySelectorAll("tbody tr")).toHaveLength(50);
  expect(button("重試載入更多")).toBeDefined();

  nextFails = false;
  await click(button("重試載入更多"));
  await waitFor(() => container.querySelectorAll("tbody tr").length === 51);
  expect(has("清單不完整")()).toBe(false);
});

test("OPS-006: a failed audit refresh does not show cached rows as current", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/audit-log" && unavailable
      ? { body: { error: "audit unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/audit-log");
  await waitFor(has("授予點數"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.auditLog });
  });
  await waitFor(has("暫時無法讀取動作紀錄"));
  expect(container.querySelectorAll("tbody tr")).toHaveLength(0);
});

test("OPS-006: a full page of 50 stops, the 51st event offers the next page", async () => {
  let total = 50;
  const event = ADMIN_AUDIT_LOG.events[0];
  stub(true, (path) =>
    path === "/admin/audit-log"
      ? {
          body: {
            events: Array.from({ length: 50 }, () => event),
            ...(total > 50 ? { next_before: "audit-page-2" } : {}),
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
  expect(
    calls
      .filter((c) => c.url.startsWith("/admin/audit-log"))
      .every((c) => c.url.includes("limit=50")),
  ).toBe(true);
});

test("OPS-007: cost statistics show window bounds and dollars", async () => {
  stub(true);
  await mountAt("/admin/cost-statistics");
  await waitFor(has("搜尋理由"));
  expect(field<HTMLElement>(".table-scroll").tabIndex).toBe(-1);
  const table = field<HTMLTableElement>("table.responsive-table");
  const labels = ["種類", "統計窗開始", "統計窗結束", "樣本數", "p50", "p90", "p95", "最大"];
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
  expect(
    Array.from(table.tBodies[0].rows).map((row) =>
      Array.from(row.querySelectorAll("td time"), (time) => time.getAttribute("datetime")),
    ),
  ).toEqual([
    ["2026-08-12T00:00:00Z", "2026-09-11T00:00:00Z"],
    ["2026-08-12T00:00:00Z", "2026-09-11T00:00:00Z"],
  ]);
  const rows = Array.from(container.querySelectorAll("tbody tr")).map((tr) => [
    tr.querySelector("th")?.textContent,
    ...Array.from(tr.querySelectorAll("td"))
      .slice(2)
      .map((td) => td.textContent),
  ]);
  expect(rows).toEqual([
    ["搜尋理由", "0", "未測量", "未測量", "未測量", "未測量"],
    ["評審", "40", "$0.0012", "$0.0034", "$0.0041", "$0.0090"],
  ]);
});

test("OPS-007: a failed statistics refresh does not present cached windows as current", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/cost-statistics" && unavailable
      ? { body: { error: "statistics unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/cost-statistics");
  await waitFor(has("搜尋理由"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.costStatistics });
  });
  await waitFor(has("暫時無法讀取成本統計"));
  expect(container.querySelectorAll("tbody tr")).toHaveLength(0);
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

test("OPS-008: a failed trend refresh hides that cached chart and balance", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/trends/credits" && unavailable
      ? { body: { error: "trend unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/trends");
  await waitFor(has("全平台目前餘額總和：1268 點。"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.trend("credits", 30) });
  });
  await waitFor(has("暫時無法讀取每日點數異動（淨額）"));
  const credits = Array.from(container.querySelectorAll("h2"))
    .find((heading) => heading.textContent === "每日點數異動（淨額）")
    ?.closest("section");
  expect(credits?.querySelectorAll("figure")).toHaveLength(0);
  expect(has("全平台目前餘額總和：1268 點。")()).toBe(false);
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
      "這段期間沒有事件：創作步驟、創作會話、搜尋向量、搜尋意圖分析、索引增強、改善建議、試跑、搜尋理由。",
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

test("DISC-007: a failed exposure queue refresh hides cached waiting releases", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === "/admin/exposure-reviews" && unavailable
      ? { body: { error: "queue unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/exposure");
  await waitFor(has("審這一筆"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.admin.exposureQueue });
  });
  await waitFor(has("暫時無法讀取待審清單"));
  expect(has("審這一筆")()).toBe(false);
});

test("DISC-007: a failed exposure case refresh hides cached review actions", async () => {
  let unavailable = false;
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure` && unavailable
      ? { body: { error: "case unavailable" }, status: 503 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));

  unavailable = true;
  await act(async () => {
    await queryClient.invalidateQueries({
      queryKey: queryKeys.admin.exposureCase(EXPOSURE_PUBLICATION),
    });
  });
  await waitFor(has("暫時無法讀取這一筆的曝光審核資料"));
  expect(has("審核這一版")()).toBe(false);
  expect(container.querySelector("#admin-exposure-review-note")).toBeNull();
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
  await waitFor(has(staleMessage));
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

test.each([
  ["missing", undefined],
  ["outdated", { ...ADMIN_EXPOSURE_CASE.snapshot, current: false }],
  ["incomplete", { ...ADMIN_EXPOSURE_CASE.snapshot, enriched: false }],
])("an %s search snapshot cannot be approved", async (_state, snapshot) => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? { body: { ...ADMIN_EXPOSURE_CASE, snapshot }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("審核這一版"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="approved"]'));
  await type("#admin-exposure-review-note", "符合規範");

  expect(button("送出核准").disabled).toBe(true);
  expect(has("必須先看到與這個 Release 相符的完整搜尋內容")()).toBe(true);
  expect(calls.some((c) => c.method === "POST")).toBe(false);
});

test("an absent search snapshot does not prevent revoking exposure", async () => {
  stub(true, (path) =>
    path === `/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`
      ? { body: { ...ADMIN_EXPOSURE_CASE, snapshot: undefined }, status: 200 }
      : undefined,
  );
  await mountAt("/admin/exposure", { publication: EXPOSURE_PUBLICATION });
  await waitFor(has("尚未進索引"));
  await click(field<HTMLInputElement>('input[name="admin-exposure-decision"][value="revoked"]'));
  await type("#admin-exposure-review-note", "下架待查");

  expect(button("送出撤銷").disabled).toBe(false);
});
