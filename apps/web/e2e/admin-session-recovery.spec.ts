import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreditLedger } from "../src/features/admin/admin.service";
import { ADMIN_ACCOUNT, ADMIN_LEDGER, ME } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

type SessionReply = "ready" | "server" | "network" | "signed-out" | "forbidden" | "member";
type IntentState = "draft" | "uncertain" | "pending";
const reason = "保留這筆營運異動";

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(`${label}-viewport.png`) });
}

async function openIntent(page: Page, amount: number, state: IntentState) {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  let reply: SessionReply = "ready";
  await page.route("**/me", (route) => {
    if (reply === "network") return route.abort("failed");
    if (reply === "server" || reply === "signed-out" || reply === "forbidden") {
      return route.fulfill({
        status: { server: 503, "signed-out": 401, forbidden: 403 }[reply],
        json: { error: "controlled operator check" },
      });
    }
    return route.fulfill({ json: { ...ME, operator: reply !== "member" } });
  });
  let documents = 0;
  page.on("domcontentloaded", () => documents++);
  let balance = 120;
  const entries: CreditLedger["entries"] = [...ADMIN_LEDGER.entries];
  const writes: { amount_credits: number; reason: string; idempotency_key: string }[] = [];
  const keys = new Set<string>();
  let release!: () => void;
  let delivered = false;
  const grantGate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}`, (route) =>
    route.fulfill({ json: { ...ADMIN_LEDGER, balance_credits: balance, entries } }),
  );
  await page.route(`**/admin/credits/${ADMIN_ACCOUNT.workspace_id}/grants`, async (route) => {
    const intent = route.request().postDataJSON();
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
    if (state === "uncertain" && writes.length === 1) return route.fulfill({ json: {} });
    if (state === "pending") await grantGate;
    await route.fulfill({
      json: {
        workspace_id: ADMIN_ACCOUNT.workspace_id,
        balance_credits: balance,
        amount_credits: intent.amount_credits,
      },
    });
    delivered = true;
  });
  await page.goto("/admin/accounts");
  await page.getByLabel("Email", { exact: true }).fill("member@example.com");
  await page.getByLabel("Email", { exact: true }).press("Enter");
  await page.locator("#admin-grant-amount").fill(String(amount));
  await page.locator("#admin-grant-note").fill(reason);
  return {
    writes,
    release,
    setReply(next: SessionReply) {
      reply = next;
    },
    get documents() {
      return documents;
    },
    get delivered() {
      return delivered;
    },
  };
}

async function refreshSession(page: Page) {
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
  );
}

for (const [failure, amount, state] of [
  ["server", 25, "draft"],
  ["network", -25, "draft"],
  ["network", 25, "uncertain"],
  ["server", -25, "uncertain"],
] as const) {
  test(`a ${failure} session check preserves ${state} ${amount} credit intent`, async ({
    page,
  }) => {
    const control = await openIntent(page, amount, state);
    try {
      const main = page.getByRole("main");
      const input = page.locator("#admin-grant-amount");
      const note = page.locator("#admin-grant-note");
      const submit = main.getByRole("button", {
        name: amount > 0 ? "授予" : "扣減點數",
        exact: true,
      });
      if (state === "uncertain") {
        await submit.click();
        await expect(main.getByRole("alert")).toContainText("無法確認操作是否完成");
      }
      control.setReply(failure);
      await refreshSession(page);
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取後台");
      await expect(input).toBeHidden();
      await expect(main.getByRole("heading", { name: "封測者甲", exact: true })).toHaveCount(0);
      expect(control.writes).toHaveLength(state === "uncertain" ? 1 : 0);
      await inspect(page, "session-unavailable");
      control.setReply("ready");
      await main.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(main.getByRole("heading", { level: 1, name: "帳號與點數" })).toBeFocused();
      await expect(input).toHaveValue(String(amount));
      await expect(note).toHaveValue(reason);
      if (state === "uncertain") {
        await expect(input).toHaveAttribute("readonly", "");
        await expect(main.getByRole("alert")).toContainText("無法確認操作是否完成");
      } else {
        await expect(input).not.toHaveAttribute("readonly", "");
        await expect(main.getByRole("alert")).toHaveCount(0);
      }
      expect(control.documents).toBe(1);
      expect(control.writes).toHaveLength(state === "uncertain" ? 1 : 0);
      await inspect(page, "session-recovered");
      await submit.click();
      await expect(main.getByRole("status")).toContainText(`餘額現在是 ${120 + amount} 點`);
      expect(control.writes).toHaveLength(state === "uncertain" ? 2 : 1);
      if (state === "uncertain") expect(control.writes[1]).toEqual(control.writes[0]);
      expect(control.writes[0]).toMatchObject({ amount_credits: amount, reason });
    } finally {
      control.release();
    }
  });
}

test("a completed grant while the session is unavailable keeps its receipt", async ({ page }) => {
  const control = await openIntent(page, 25, "pending");
  try {
    const main = page.getByRole("main");
    await main.getByRole("button", { name: "授予", exact: true }).click();
    await expect.poll(() => control.writes.length).toBe(1);
    control.setReply("server");
    await refreshSession(page);
    await expect(main.getByRole("alert")).toContainText("暫時無法讀取後台");
    await expect(page.locator("#admin-grant-amount")).toBeHidden();
    control.release();
    await expect.poll(() => control.delivered).toBe(true);
    control.setReply("ready");
    await main.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(main.getByRole("heading", { level: 1, name: "帳號與點數" })).toBeFocused();
    await expect(main.getByRole("status")).toContainText("已授予 25 點，餘額現在是 145 點");
    await expect(page.locator("#admin-grant-amount")).toHaveValue("25");
    await expect(page.locator("#admin-grant-note")).toHaveValue(reason);
    await expect(main.getByRole("button", { name: "授予", exact: true })).toBeDisabled();
    expect(control.writes).toHaveLength(1);
    expect(control.documents).toBe(1);
    await inspect(page, "completed-while-unavailable");
  } finally {
    control.release();
  }
});

for (const failure of ["signed-out", "forbidden", "member"] as const) {
  test(`a ${failure} session denial clears private credit controls`, async ({ page }) => {
    const control = await openIntent(page, 25, "draft");
    try {
      control.setReply(failure);
      await refreshSession(page);
      await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
      await expect(page.locator("#admin-grant-note")).toHaveCount(0);
      if (failure === "signed-out") {
        await expect.poll(() => control.documents).toBe(2);
        await expect(page.getByRole("main").getByRole("status")).toContainText("後台需要登入");
      } else if (failure === "forbidden") {
        await expect(page.getByRole("main").getByRole("alert")).toContainText("暫時無法讀取後台");
      } else {
        await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText(
          "這一頁現在不存在",
        );
      }
      await expect(page.getByRole("main").getByText("封測者甲", { exact: true })).toHaveCount(0);
      await expect(
        page.getByRole("banner").getByRole("link", { name: "後台", exact: true }),
      ).toHaveCount(0);
      expect(control.writes).toEqual([]);
      await inspect(page, "session-denied");
    } finally {
      control.release();
    }
  });
}

test("an initial unavailable operator check never mounts private credit controls", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  let ready = false;
  const requests: string[] = [];
  page.on("request", (request) => {
    if (
      request.resourceType() !== "document" &&
      new URL(request.url()).pathname.startsWith("/admin/")
    ) {
      requests.push(request.url());
    }
  });
  await page.route("**/me", (route) =>
    route.fulfill(
      ready ? { json: { ...ME, operator: true } } : { status: 503, json: { error: "unavailable" } },
    ),
  );
  await page.goto("/admin/accounts");
  const main = page.getByRole("main");
  await expect(main.getByRole("alert")).toContainText("暫時無法讀取後台");
  await expect(page.locator("#admin-account-email")).toHaveCount(0);
  expect(requests).toEqual([]);
  await inspect(page, "initial-session-unavailable");
  ready = true;
  await main.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator("#admin-account-email")).toHaveValue("");
  await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
  expect(requests).toEqual([]);
});
