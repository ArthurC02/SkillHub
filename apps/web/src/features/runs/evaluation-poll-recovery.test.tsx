import { act } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { EVALUATION, REVISIONS, RUN } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
});

afterEach(() => {
  vi.useRealTimers();
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const PENDING = { ...EVALUATION, status: "pending", overall: "undetermined" };

async function advance(ms: number) {
  await act(async () => {
    vi.advanceTimersByTime(ms);
  });
}

async function settle() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 30));
  });
}

async function pollEvaluation(page: App, reads: () => number, expected: number) {
  await advance(3000);
  await page.eventually(() => expect(reads()).toBe(expected));
}

for (const [scenario, limit, firstLabel, announcement] of [
  ["pending", 100, "評估進行中", "尚未確認完成判定"],
  ["read failures", 100, "評估進行中", "已停止自動確認評估結果"],
  ["no evaluation", 20, "未評估", "已停止自動確認評估結果"],
] as const) {
  test(`evaluation polling bounds ${scenario} and recovers through a queued manual read`, async () => {
    const page = app();
    let reads = 0;
    let recovered = false;
    page.route(`**/runs/${RUN}/evaluation`, (request) => {
      expect(request.method).toBe("GET");
      reads += 1;
      if (recovered) return { json: EVALUATION };
      if (scenario === "no evaluation") return { status: 404, json: { error: "not found" } };
      if (scenario === "read failures" && reads > 1)
        return { status: 503, json: { error: "service unavailable" } };
      return { json: PENDING };
    });
    try {
      await page.goto(`/runs/${RUN}`);
      const panel = page.locator("#run-decision");
      await expect(panel.getByText(firstLabel, { exact: true })).toBeVisible();
      for (let count = 2; count < limit; count += 1) await pollEvaluation(page, () => reads, count);
      expect(reads).toBe(limit - 1);
      const retry = panel.getByRole("button", { name: "重新確認評估結果", exact: true });
      await expect(retry).toHaveCount(0);
      await pollEvaluation(page, () => reads, limit);
      await expect(retry).toBeEnabled();
      expect(reads).toBe(limit);
      await expect(panel).toContainText("已停止自動確認評估結果");
      await expect(panel.getByRole("status").filter({ hasText: announcement })).toBeVisible();
      await expect(panel).not.toContainText("工作沒有在推進");
      await expect(panel).not.toContainText("每 3 秒");
      await advance(15000);
      await settle();
      expect(reads).toBe(limit);
      const timestamp = panel.locator(".notice time");
      const observedAt = scenario === "pending" ? timestamp.attribute("datetime") : null;
      if (scenario === "pending") {
        expect(observedAt).toBeTruthy();
        await expect(timestamp).toContainText("秒前");
        expect(timestamp.element().closest('[role="status"]')).toBeNull();
      }
      expect(await page.axeViolations("main")).toEqual([]);
      await page.setOffline(true);
      await retry.focus();
      await page.press("Enter");
      await expect(panel.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
      await expect(
        panel.getByRole("status").filter({ hasText: "等待連線以確認評估結果" }),
      ).toBeVisible();
      await expect(page.locator("#evaluation-heading")).toBeFocused();
      await advance(15000);
      await settle();
      expect(reads).toBe(limit);
      if (scenario === "pending") await expect(timestamp).toHaveAttribute("datetime", observedAt!);
      recovered = true;
      await page.setOffline(false);
      await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
      await expect(retry).toHaveCount(0);
      await expect(panel).not.toContainText("已停止自動確認評估結果");
      await expect(page.locator("#evaluation-heading")).toBeFocused();
      await advance(15000);
      await settle();
      expect(reads).toBe(limit + 1);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      await page.setOffline(false);
    }
  });
}

test("an offline evaluation history reread preserves the current verdict and feedback draft", async () => {
  const page = app();
  let reads = 0;
  let evaluationReads = 0;
  let recovered = false;
  page.route(`**/runs/${RUN}/evaluation`, () => {
    evaluationReads += 1;
    return { json: evaluationReads === 1 ? PENDING : EVALUATION };
  });
  page.route(`**/runs/${RUN}/evaluation/revisions`, (request) => {
    expect(request.method).toBe("GET");
    reads += 1;
    return recovered || reads === 1
      ? { json: REVISIONS }
      : { status: 503, json: { error: "service unavailable" } };
  });
  try {
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator("#run-decision");
    await expect(panel.getByText("評估進行中", { exact: true })).toBeVisible();
    await expect(page.getByLabel("評估版本", { exact: true })).toBeVisible();
    await pollEvaluation(page, () => evaluationReads, 2);
    await expect(panel.getByRole("alert")).toContainText("歷史評估版本");
    const comment = page.locator("#feedback-comment");
    await comment.fill("unsent review");
    await page.setOffline(true);
    await panel.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.press("Enter");
    await expect(panel.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
    await expect(
      panel.getByRole("status").filter({ hasText: "等待連線以確認歷史評估版本" }),
    ).toBeVisible();
    await expect(page.locator("#evaluation-heading")).toBeFocused();
    await expect(panel.locator(".verdict")).toHaveText("任務判定：未符合");
    await expect(comment).toHaveValue("unsent review");
    expect(reads).toBe(2);
    recovered = true;
    await page.setOffline(false);
    await expect(page.getByLabel("評估版本", { exact: true })).toBeVisible();
    await expect(panel.getByRole("alert")).toHaveCount(0);
    await expect(comment).toHaveValue("unsent review");
    expect(reads).toBe(3);
    expect(evaluationReads).toBe(2);
    expect(await page.axeViolations("main")).toEqual([]);
  } finally {
    await page.setOffline(false);
  }
});

for (const [status, error, message, retryCount] of [
  [401, "not authenticated", "評估結果需要登入。", 0],
  [403, "forbidden", "暫時無法讀取評估結果", 1],
  [404, "not found", "暫時無法讀取評估結果", 1],
] as const) {
  test(`a pending evaluation stops after HTTP ${status} without promising another automatic read`, async () => {
    const page = app();
    let reads = 0;
    page.route(`**/runs/${RUN}/evaluation`, () => {
      reads += 1;
      return reads === 1 ? { json: PENDING } : { status, json: { error } };
    });
    await page.goto(`/runs/${RUN}`);
    const panel = page.locator("#run-decision");
    await expect(panel.getByText("評估進行中", { exact: true })).toBeVisible();
    expect(reads).toBe(1);
    await pollEvaluation(page, () => reads, 2);
    await expect(panel).toContainText(message);
    await advance(15000);
    await settle();
    expect(reads).toBe(2);
    await expect(panel.getByText("未評估", { exact: true })).toHaveCount(0);
    await expect(panel).not.toContainText("每 3 秒");
    await expect(panel.locator(".verdict")).toHaveCount(0);
    await expect(panel.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(
      retryCount,
    );
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
