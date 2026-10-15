import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { EVALUATION, REVISIONS, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const PENDING = { ...EVALUATION, status: "pending", overall: "undetermined" };

async function pollEvaluation(page: Page) {
  const next = page.waitForResponse(
    (response) => new URL(response.url()).pathname === `/runs/${RUN}/evaluation`,
  );
  await page.clock.runFor(3000);
  await (await next).finished();
  await page.clock.runFor(1);
}

async function verifyEvaluationLayout(page: Page, state: string) {
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await writeFile(
    test.info().outputPath(`${state}-accessibility.json`),
    JSON.stringify(accessibility),
  );
}

for (const [scenario, limit, firstLabel, announcement] of [
  ["pending", 100, "評估進行中", "尚未確認完成判定"],
  ["read failures", 100, "評估進行中", "已停止自動確認評估結果"],
  ["no evaluation", 20, "未評估", "已停止自動確認評估結果"],
] as const) {
  test(`evaluation polling bounds ${scenario} and recovers through a queued manual read`, async ({
    page,
    context,
  }) => {
    await page.clock.install();
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    let recovered = false;
    await page.route(`**/runs/${RUN}/evaluation`, (route) => {
      expect(route.request().method()).toBe("GET");
      reads += 1;
      if (recovered) return route.fulfill({ json: EVALUATION });
      if (scenario === "no evaluation")
        return route.fulfill({ status: 404, json: { error: "not found" } });
      if (scenario === "read failures" && reads > 1)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      return route.fulfill({ json: PENDING });
    });
    try {
      await page.goto(`/runs/${RUN}`);
      const panel = page.locator("#run-decision");
      await expect(panel.getByText(firstLabel, { exact: true })).toBeVisible();
      for (let count = 2; count < limit; count += 1) await pollEvaluation(page);
      expect(reads).toBe(limit - 1);
      const retry = panel.getByRole("button", { name: "重新確認評估結果", exact: true });
      await expect(retry).toHaveCount(0);
      await pollEvaluation(page);
      await expect(retry).toBeEnabled();
      expect(reads).toBe(limit);
      await expect(panel).toContainText("已停止自動確認評估結果");
      await expect(panel.getByRole("status").filter({ hasText: announcement })).toBeVisible();
      await expect(panel).not.toContainText("工作沒有在推進");
      await expect(panel).not.toContainText("每 3 秒");
      await page.clock.runFor(15000);
      expect(reads).toBe(limit);
      const timestamp = panel.locator(".notice time");
      const observedAt = scenario === "pending" ? await timestamp.getAttribute("datetime") : null;
      if (scenario === "pending") {
        expect(observedAt).toBeTruthy();
        await expect(timestamp).toContainText("秒前");
        expect(
          await timestamp.evaluate((element) => element.closest('[role="status"]') === null),
        ).toBe(true);
      }
      await verifyEvaluationLayout(page, "stopped");
      await page.screenshot({
        path: test.info().outputPath("evaluation-poll-stopped.png"),
        fullPage: true,
      });
      await context.setOffline(true);
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect(panel.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
      await expect(
        panel.getByRole("status").filter({ hasText: "等待連線以確認評估結果" }),
      ).toBeVisible();
      await expect(page.locator("#evaluation-heading")).toBeFocused();
      const position = await page.locator("#evaluation-heading").evaluate((element) => ({
        top: element.getBoundingClientRect().top,
        header: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      }));
      expect(position.top).toBeGreaterThanOrEqual(position.header);
      await page.clock.runFor(15000);
      expect(reads).toBe(limit);
      if (scenario === "pending") await expect(timestamp).toHaveAttribute("datetime", observedAt!);
      recovered = true;
      await context.setOffline(false);
      await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
      await expect(retry).toHaveCount(0);
      await expect(panel).not.toContainText("已停止自動確認評估結果");
      await expect(page.locator("#evaluation-heading")).toBeFocused();
      await page.clock.runFor(15000);
      expect(reads).toBe(limit + 1);
      await verifyEvaluationLayout(page, "recovered");
    } finally {
      await context.setOffline(false);
    }
  });
}

test("an offline evaluation history reread preserves the current verdict and feedback draft", async ({
  page,
  context,
}) => {
  await page.clock.install();
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let evaluationReads = 0;
  let recovered = false;
  await page.route(`**/runs/${RUN}/evaluation`, (route) => {
    evaluationReads += 1;
    return route.fulfill({ json: evaluationReads === 1 ? PENDING : EVALUATION });
  });
  await page.route(`**/runs/${RUN}/evaluation/revisions`, (route) => {
    expect(route.request().method()).toBe("GET");
    reads += 1;
    return route.fulfill(
      recovered || reads === 1
        ? { json: REVISIONS }
        : { status: 503, json: { error: "service unavailable" } },
    );
  });
  try {
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator("#run-decision");
    await expect(panel.getByText("評估進行中", { exact: true })).toBeVisible();
    await expect(page.getByLabel("評估版本", { exact: true })).toBeVisible();
    await pollEvaluation(page);
    await expect(panel.getByRole("alert")).toContainText("歷史評估版本");
    const comment = page.locator("#feedback-comment");
    await comment.fill("unsent review");
    await context.setOffline(true);
    await panel.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(panel.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
    await expect(
      panel.getByRole("status").filter({ hasText: "等待連線以確認歷史評估版本" }),
    ).toBeVisible();
    await expect(page.locator("#evaluation-heading")).toBeFocused();
    await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
    await expect(comment).toHaveValue("unsent review");
    expect(reads).toBe(2);
    recovered = true;
    await context.setOffline(false);
    await expect(page.getByLabel("評估版本", { exact: true })).toBeVisible();
    await expect(panel.getByRole("alert")).toHaveCount(0);
    await expect(comment).toHaveValue("unsent review");
    expect(reads).toBe(3);
    expect(evaluationReads).toBe(2);
    await verifyEvaluationLayout(page, "history-recovered");
  } finally {
    await context.setOffline(false);
  }
});

for (const [status, error, message, retryCount] of [
  [401, "not authenticated", "評估結果需要登入。", 0],
  [403, "forbidden", "暫時無法讀取評估結果", 1],
  [404, "not found", "暫時無法讀取評估結果", 1],
] as const) {
  test(`a pending evaluation stops after HTTP ${status} without promising another automatic read`, async ({
    page,
  }) => {
    await page.clock.install();
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    await page.route(`**/runs/${RUN}/evaluation`, (route) => {
      reads += 1;
      return route.fulfill(reads === 1 ? { json: PENDING } : { status, json: { error } });
    });
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator("#run-decision");
    await expect(panel.getByText("評估進行中", { exact: true })).toBeVisible();
    expect(reads).toBe(1);
    await pollEvaluation(page);
    await expect(panel).toContainText(message);
    await page.clock.runFor(15000);
    expect(reads).toBe(2);
    await expect(panel.getByText("未評估", { exact: true })).toHaveCount(0);
    await expect(panel).not.toContainText("每 3 秒");
    await expect(panel.locator(".verdict")).toHaveCount(0);
    await expect(panel.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(
      retryCount,
    );
    await verifyEvaluationLayout(page, "refused");
    await page.screenshot({
      path: test.info().outputPath("evaluation-refused.png"),
      fullPage: true,
    });
  });
}
