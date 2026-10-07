import { expect, test } from "@playwright/test";
import {
  ADMIN_ACCOUNT,
  ADMIN_DISPATCH,
  ADMIN_LEDGER,
  ADMIN_SKILLS,
  SKILL,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("operator can find an account and verify a credit grant in the refreshed ledger", async ({
  page,
}) => {
  await stubPlatform(page);
  const lookedUpEmails: string[] = [];
  const grants: { amount_credits: number; reason: string }[] = [];
  await page.route("**/admin/accounts?*", async (route) => {
    lookedUpEmails.push(new URL(route.request().url()).searchParams.get("email") ?? "");
    await route.fulfill({ json: ADMIN_ACCOUNT });
  });
  await page.route("**/admin/credits/ws-2**", async (route) => {
    if (route.request().method() === "POST") {
      const body = route.request().postDataJSON() as {
        amount_credits: number;
        reason: string;
      };
      grants.push({ amount_credits: body.amount_credits, reason: body.reason });
      await route.fulfill({
        json: { balance_credits: 145, amount_credits: body.amount_credits },
      });
      return;
    }
    await route.fulfill({
      json: {
        ...ADMIN_LEDGER,
        balance_credits: grants.length ? 145 : 120,
        entries: grants.length
          ? [
              {
                kind: "grant",
                delta_credits: 25,
                ref_type: null,
                estimated: false,
                created_at: "2026-09-12T08:00:00Z",
              },
              ...ADMIN_LEDGER.entries,
            ]
          : ADMIN_LEDGER.entries,
      },
    });
  });

  await page.goto("/admin");
  await page.locator(".admin-home-list").getByRole("link", { name: "帳號與點數" }).click();
  await page.getByLabel("Email").fill("member@example.com");
  await page.getByRole("button", { name: "查詢" }).click();
  await expect(page.getByText("目前餘額 120 點")).toBeVisible();
  expect(new URL(page.url()).searchParams.has("email")).toBe(false);
  expect(lookedUpEmails).toContain("member@example.com");

  await page.getByLabel("點數（整數，不能是 0；負數是更正）").fill("25");
  await page.getByLabel("理由（必填，會寫進動作紀錄）").fill("封測補點");
  await page.getByRole("button", { name: "授予", exact: true }).click();
  await expect(page.getByText("目前餘額 145 點")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "已授予 25 點" })).toBeVisible();
  expect(grants).toEqual([{ amount_credits: 25, reason: "封測補點" }]);
});

test("operator can halt and resume dispatch with the affected scope visible", async ({ page }) => {
  await stubPlatform(page);
  let halted = false;
  const writes: { method: string; body: Record<string, unknown> }[] = [];
  await page.route("**/admin/dispatch", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    return route.fulfill({
      json: halted
        ? { ...ADMIN_DISPATCH, halts: [{ ...ADMIN_DISPATCH.halts[0], reason: "保留事故現場" }] }
        : { dispatching: true, halts: [] },
    });
  });
  await page.route("**/admin/dispatch/halt", (route) => {
    const method = route.request().method();
    writes.push({ method, body: route.request().postDataJSON() as Record<string, unknown> });
    halted = method === "PUT";
    return route.fulfill({
      json:
        method === "PUT"
          ? {
              note: "new runs are refused and nothing is dispatched to this target; cleanup and orphan teardown stand down so the scene is preserved. This halt is never lifted automatically.",
            }
          : {},
    });
  });

  await page.goto("/admin/dispatch");
  await expect(page.getByText("正在派送", { exact: true })).toBeVisible();
  await page.locator("#admin-halt-declare-note").fill("保留事故現場");
  await page.getByRole("button", { name: "停止派送", exact: true }).click();
  await expect(page.locator("span.badge-danger").filter({ hasText: "停止派送" })).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "煞車不會自動解除" })).toBeVisible();
  await expect(page.getByText("理由：保留事故現場")).toBeVisible();

  await page.getByLabel("要解除的煞車").selectOption("pool");
  await page.locator("#admin-halt-lift-note").fill("已確認可恢復");
  await page.getByRole("button", { name: "恢復派送", exact: true }).click();
  await expect(page.getByText("正在派送", { exact: true })).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "解除請求已處理" })).toBeVisible();
  expect(writes).toEqual([
    { method: "PUT", body: { note: "保留事故現場" } },
    { method: "DELETE", body: { note: "已確認可恢復" } },
  ]);
});

test("operator must confirm a skill takedown before it is sent and sees the refreshed state", async ({
  page,
}) => {
  await stubPlatform(page);
  let takenDown = false;
  const reasons: string[] = [];
  await page.route("**/admin/skills?*", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    return route.fulfill({
      json: {
        ...ADMIN_SKILLS,
        skills: ADMIN_SKILLS.skills.map((skill) =>
          takenDown
            ? { ...skill, takedown_at: "2026-09-12T08:00:00Z", takedown_reason: "DMCA notice" }
            : skill,
        ),
      },
    });
  });
  await page.route(`**/admin/skills/${SKILL}/takedown`, (route) => {
    reasons.push((route.request().postDataJSON() as { reason: string }).reason);
    takenDown = true;
    return route.fulfill({ json: { skill_id: SKILL, taken_down: true } });
  });

  await page.goto(`/admin/skills?q=${SKILL}`);
  await expect(page.getByRole("heading", { name: "對「PDF Summariser」的動作" })).toBeVisible();
  await page.getByLabel("下架理由（必填，最多 1000 位元組，會寫進動作紀錄）").fill(" DMCA notice ");
  await page.getByRole("button", { name: "下架", exact: true }).click();
  const confirm = page.getByRole("button", { name: "確認下架" });
  await expect(confirm).toBeFocused();
  await expect(confirm).toHaveAttribute("aria-describedby", "admin-takedown-scope");
  await expect(page.locator("#admin-takedown-scope")).toContainText("下架沒有恢復的路");
  expect(reasons).toEqual([]);

  await page.getByRole("button", { name: "取消", exact: true }).click();
  expect(reasons).toEqual([]);
  await page.getByRole("button", { name: "下架", exact: true }).click();
  await page.getByRole("button", { name: "確認下架" }).click();
  await expect(page.getByText("已下架", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "對「PDF Summariser」的動作" })).toHaveCount(0);
  expect(reasons).toEqual(["DMCA notice"]);
});
