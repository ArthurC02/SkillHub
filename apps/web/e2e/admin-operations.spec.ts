import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { ADMIN_DISPATCH, ADMIN_MODEL_BUDGETS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("a release in progress blocks another dispatch decision on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let releaseRequest = 0;
  let finishRelease!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRelease = resolve;
  });
  await page.route("**/admin/dispatch/halt", async (route) => {
    if (route.request().method() !== "DELETE") return route.fallback();
    releaseRequest += 1;
    await held;
    await route.fulfill({ status: 204, body: "" });
  });
  await page.goto("/admin/dispatch");
  await page.locator("#admin-halt-lift-target").selectOption("pool");
  await page.locator("#admin-halt-lift-note").fill("incident resolved");
  await page.locator("#admin-halt-declare-note").fill("new incident");
  await expect(page.locator("#admin-halt-declare-scope")).toContainText("本次停止範圍：整個叢集");
  await page.getByRole("button", { name: "恢復派送" }).click();
  await page.getByRole("button", { name: "確認恢復派送" }).click();

  try {
    await expect.poll(() => releaseRequest).toBe(1);
    await expect(page.getByRole("button", { name: "停止派送" })).toBeDisabled();
    await expect(page.getByText("正在解除煞車，完成後才能停止派送。")).toBeVisible();
    await expect(page.getByRole("button", { name: "停止派送" })).toHaveAttribute(
      "aria-describedby",
      "admin-halt-declare-why",
    );
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: testInfo.outputPath("dispatch-release-phone.png"),
    });
  } finally {
    finishRelease();
  }
  await expect(page.getByRole("button", { name: "停止派送" })).toBeEnabled();
});

test("a release result stays reachable when dispatch status cannot be reread on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/dispatch", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    reads += 1;
    return route.fulfill(
      reads === 1
        ? { json: ADMIN_DISPATCH }
        : reads === 2
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: { dispatching: true, halts: [] } },
    );
  });
  await page.route("**/admin/dispatch/halt", (route) => route.fulfill({ status: 204, body: "" }));
  await page.goto("/admin/dispatch");
  await page.locator("#admin-halt-lift-target").selectOption("pool");
  await page.locator("#admin-halt-lift-note").fill("incident resolved");
  await page.getByRole("button", { name: "恢復派送" }).click();
  await page.getByRole("button", { name: "確認恢復派送" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取派送狀態");
  const result = page.locator("#admin-dispatch-lift-result");
  await expect(result).toContainText("暫勿假定已恢復派送");
  await expect(result).toHaveClass(/notice-warning/);
  await expect(result).toBeFocused();
  await expect(page.getByText("sandbox escape suspected on node-2")).toHaveCount(0);
  await expect(page.locator("#admin-halt-lift-target")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("dispatch-release-reread-failed-phone.png") });

  await page.getByRole("button", { name: "重新整理派送狀態" }).click();
  await expect(page.getByText("沒有生效中的煞車。")).toBeVisible();
  await expect(result).toHaveClass(/notice-success/);
  await expect(result).toContainText("已不在生效中的煞車清單");
});

test("restoring a model timeout also resets its editable draft on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let restored = false;
  await page.route("**/admin/model-budgets", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
          budget.kind === "judge-run" && restored
            ? { ...budget, seconds: null, reason: null, set_at: null }
            : budget,
        ),
      },
    });
  });
  await page.route("**/admin/model-budgets/judge-run", (route) => {
    if (route.request().method() !== "DELETE") return route.fallback();
    restored = true;
    return route.fulfill({ status: 200, json: {} });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("90");
  await page.locator("#admin-budget-judge-run-clear summary").click();
  await page.locator("#admin-budget-judge-run-clear-note").fill("restore the default");
  await page.getByRole("button", { name: "把 評估判定 改回預設" }).click();

  await expect(page.getByText("目前：預設 130 秒")).toBeVisible();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("130");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("model-timeout-default-phone.png"),
  });
});

test("a refreshed model timeout replaces a stale editable draft on a phone", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let currentSeconds = 90;
  await page.route("**/admin/model-budgets", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
          budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
        ),
      },
    });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await page.locator("#admin-budget-judge-run-seconds").fill("100");

  currentSeconds = 120;
  await page.getByRole("button", { name: "重新整理" }).click();
  await expect(page.getByText("目前：120 秒（已調整）")).toBeVisible();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("120");
  await expect(page.getByText("設定已變更；草稿改為最新的 120 秒，請確認後再送出。")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("a credit correction describes its deduction on a phone", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/admin/credits/ws-2/grants", (route) =>
    route.fulfill({
      json: { workspace_id: "ws-2", balance_credits: 90, amount_credits: -30 },
    }),
  );
  await page.goto("/admin/accounts");
  await page.getByLabel("Email").fill("member@example.com");
  await page.getByRole("button", { name: "查詢" }).click();
  await page.locator("#admin-grant-amount").fill("-30");
  await page.locator("#admin-grant-note").fill("corrects an over-grant");

  await expect(page.getByRole("heading", { name: "更正點數" })).toBeVisible();
  await page.getByRole("button", { name: "扣減點數" }).click();
  await expect(page.getByText("已扣減 30 點，餘額現在是 90 點。")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
