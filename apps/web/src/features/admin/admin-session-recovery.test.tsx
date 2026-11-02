import { expect, test, vi } from "vitest";
import type { CreditLedger } from "./admin.service";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../../testing/fixtures/platform";
import { queryClient } from "../../core/api/queryClient";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

type SessionReply = "ready" | "server" | "network" | "signed-out" | "forbidden" | "member";
type IntentState = "draft" | "uncertain" | "pending";
const reason = "保留這筆營運異動";

function countReloads() {
  const spy = vi.spyOn(queryClient, "clear");
  spy.mockClear();
  return () => spy.mock.calls.length;
}

async function openIntent(page: App, amount: number, state: IntentState) {
  let reply: SessionReply = "ready";
  page.route("**/me", () => {
    if (reply === "network") return "network-error";
    if (reply === "server" || reply === "signed-out" || reply === "forbidden") {
      return {
        status: { server: 503, "signed-out": 401, forbidden: 403 }[reply],
        json: { error: "controlled operator check" },
      };
    }
    return { json: { ...ME, operator: reply !== "member" } };
  });
  const reloads = countReloads();
  let balance = 120;
  const entries: CreditLedger["entries"] = [...ADMIN_LEDGER.entries];
  const writes: { amount_credits: number; reason: string; idempotency_key: string }[] = [];
  const keys = new Set<string>();
  let release!: () => void;
  let delivered = false;
  const grantGate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, () => ({
    json: { ...ADMIN_LEDGER, balance_credits: balance, entries },
  }));
  page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, async ({ body }) => {
    const intent = body as (typeof writes)[number];
    writes.push(intent);
    if (!keys.has(intent.idempotency_key)) {
      keys.add(intent.idempotency_key);
      balance += intent.amount_credits;
      entries.unshift({
        kind: intent.amount_credits > 0 ? "grant" : "adjustment",
        delta_credits: intent.amount_credits,
        ref_type: "operator_grant",
        estimated: false,
        created_at: "2026-10-16T00:00:00Z",
      });
    }
    if (state === "uncertain" && writes.length === 1) return { json: {} };
    if (state === "pending") await grantGate;
    delivered = true;
    return {
      json: {
        workspace_id: ADMIN_ACCOUNT.workspace_id,
        balance_credits: balance,
        amount_credits: intent.amount_credits,
      },
    };
  });
  await page.goto("/admin/accounts");
  await page.eventually(() => expect(page.getByLabel("Email", { exact: true }).count()).toBe(1));
  await page.getByLabel("Email", { exact: true }).fill("member@example.com");
  await page.getByLabel("Email", { exact: true }).press("Enter");
  await page.eventually(() => expect(page.locator("#admin-grant-amount").count()).toBe(1));
  await page.locator("#admin-grant-amount").fill(String(amount));
  await page.locator("#admin-grant-note").fill(reason);
  return {
    writes,
    release,
    reloads,
    setReply(next: SessionReply) {
      reply = next;
    },
    get delivered() {
      return delivered;
    },
  };
}

function refreshSession() {
  document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
}

for (const [failure, amount, state] of [
  ["server", 25, "draft"],
  ["network", -25, "draft"],
  ["network", 25, "uncertain"],
  ["server", -25, "uncertain"],
] as const) {
  test(`a ${failure} session check preserves ${state} ${amount} credit intent`, async () => {
    const page = app();
    const control = await openIntent(page, amount, state);
    try {
      const main = page.locator("main");
      const input = page.locator("#admin-grant-amount");
      const note = page.locator("#admin-grant-note");
      const submit = main.getByRole("button", {
        name: amount > 0 ? "授予" : "扣減點數",
        exact: true,
      });
      if (state === "uncertain") {
        await submit.click();
        await page.eventually(() =>
          expect(main.getByRole("alert").text()).toContain("無法確認操作是否完成"),
        );
      }
      control.setReply(failure);
      refreshSession();
      await page.eventually(() =>
        expect(main.getByRole("alert").text()).toContain("暫時無法讀取後台"),
      );
      expect(input.count()).toBe(0);
      expect(main.getByRole("heading", { name: "封測者甲", exact: true }).count()).toBe(0);
      expect(control.writes).toHaveLength(state === "uncertain" ? 1 : 0);
      expect(await page.axeViolations("body")).toEqual([]);
      control.setReply("ready");
      await main.getByRole("button", { name: "再試一次", exact: true }).click();
      const heading = main.locator("h1").filter({ hasText: "帳號與點數" });
      await page.eventually(() => expect(heading.isFocused()).toBe(true));
      expect(input.value()).toBe(String(amount));
      expect(note.value()).toBe(reason);
      if (state === "uncertain") {
        expect(input.attribute("readonly")).toBe("");
        expect(main.getByRole("alert").text()).toContain("無法確認操作是否完成");
      } else {
        expect(input.attribute("readonly")).toBeNull();
        expect(main.getByRole("alert").count()).toBe(0);
      }
      expect(control.reloads()).toBe(0);
      expect(control.writes).toHaveLength(state === "uncertain" ? 1 : 0);
      expect(await page.axeViolations("body")).toEqual([]);
      await submit.click();
      await page.eventually(() =>
        expect(main.getByRole("status").text()).toContain(`餘額現在是 ${120 + amount} 點`),
      );
      expect(control.writes).toHaveLength(state === "uncertain" ? 2 : 1);
      if (state === "uncertain") expect(control.writes[1]).toEqual(control.writes[0]);
      expect(control.writes[0]).toMatchObject({ amount_credits: amount, reason });
    } finally {
      control.release();
    }
  });
}

test("a completed grant while the session is unavailable keeps its receipt", async () => {
  const page = app();
  const control = await openIntent(page, 25, "pending");
  try {
    const main = page.locator("main");
    await main.getByRole("button", { name: "授予", exact: true }).click();
    await page.eventually(() => expect(control.writes.length).toBe(1));
    control.setReply("server");
    refreshSession();
    await page.eventually(() =>
      expect(main.getByRole("alert").text()).toContain("暫時無法讀取後台"),
    );
    expect(page.locator("#admin-grant-amount").count()).toBe(0);
    control.release();
    await page.eventually(() => expect(control.delivered).toBe(true));
    control.setReply("ready");
    await main.getByRole("button", { name: "再試一次", exact: true }).click();
    const heading = main.locator("h1").filter({ hasText: "帳號與點數" });
    await page.eventually(() => expect(heading.isFocused()).toBe(true));
    await page.eventually(() =>
      expect(main.getByRole("status").text()).toContain("已授予 25 點，餘額現在是 145 點"),
    );
    expect(page.locator("#admin-grant-amount").value()).toBe("25");
    expect(page.locator("#admin-grant-note").value()).toBe(reason);
    expect(main.getByRole("button", { name: "授予", exact: true }).isDisabled()).toBe(true);
    expect(control.writes).toHaveLength(1);
    expect(control.reloads()).toBe(0);
    expect(await page.axeViolations("body")).toEqual([]);
  } finally {
    control.release();
  }
});

for (const failure of ["signed-out", "forbidden", "member"] as const) {
  test(`a ${failure} session denial clears private credit controls`, async () => {
    const page = app();
    const control = await openIntent(page, 25, "draft");
    try {
      control.setReply(failure);
      refreshSession();
      await page.eventually(() => expect(page.locator("#admin-grant-amount").count()).toBe(0));
      expect(page.locator("#admin-grant-note").count()).toBe(0);
      const main = page.locator("main");
      if (failure === "signed-out") {
        await page.eventually(() => expect(control.reloads()).toBe(1));
        await page.eventually(() =>
          expect(main.getByRole("status").text()).toContain("後台需要登入"),
        );
      } else if (failure === "forbidden") {
        await page.eventually(() =>
          expect(main.getByRole("alert").text()).toContain("暫時無法讀取後台"),
        );
      } else {
        await page.eventually(() => expect(main.locator("h1").text()).toBe("這一頁現在不存在"));
      }
      expect(main.getByText("封測者甲", { exact: true }).count()).toBe(0);
      expect(page.locator("header").getByRole("link", { name: "後台", exact: true }).count()).toBe(
        0,
      );
      expect(control.writes).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      control.release();
    }
  });
}

test("an initial unavailable operator check never mounts private credit controls", async () => {
  const page = app();
  let ready = false;
  page.route("**/me", () =>
    ready ? { json: { ...ME, operator: true } } : { status: 503, json: { error: "unavailable" } },
  );
  await page.goto("/admin/accounts");
  const main = page.locator("main");
  await page.eventually(() => expect(main.getByRole("alert").text()).toContain("暫時無法讀取後台"));
  expect(page.locator("#admin-account-email").count()).toBe(0);
  const adminReads = () =>
    page.requests.filter((request) => request.url.pathname.startsWith("/admin/"));
  expect(adminReads()).toEqual([]);
  expect(await page.axeViolations("body")).toEqual([]);
  ready = true;
  await main.getByRole("button", { name: "再試一次", exact: true }).click();
  await page.eventually(() => expect(page.locator("#admin-account-email").count()).toBe(1));
  expect(page.locator("#admin-account-email").value()).toBe("");
  expect(page.locator("#admin-grant-amount").count()).toBe(0);
  expect(adminReads()).toEqual([]);
});
