import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, TRACE_ADVANCED, TRACE_GENERAL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function capture(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`${state}-trace-a11y.json`), JSON.stringify(report, null, 2));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}-trace.png`) });
}

for (const { name, mode, body } of [
  { name: "null summary", mode: "general", body: null },
  { name: "missing steps", mode: "general", body: { ...TRACE_GENERAL, steps: undefined } },
  {
    name: "foreign summary",
    mode: "general",
    body: { ...TRACE_GENERAL, run_id: "11111111-1111-4111-8111-111111111111" },
  },
  {
    name: "unreadable choices",
    mode: "general",
    body: {
      ...TRACE_GENERAL,
      status: "succeeded",
      questions: [{ question: "Which?", why: "Pick", options: [null] }],
    },
  },
  {
    name: "unreadable cost",
    mode: "general",
    body: { ...TRACE_GENERAL, usage: { input_tokens: 0, output_tokens: 0, cost_credits: "0" } },
  },
  { name: "null events", mode: "advanced", body: null },
  { name: "missing events", mode: "advanced", body: { ...TRACE_ADVANCED, events: undefined } },
  { name: "unreadable streams", mode: "advanced", body: { ...TRACE_ADVANCED, streams: [null] } },
]) {
  test(`a ${name} stays local and recovers without losing the decision`, async ({ page }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let healthy = false;
    const writes: string[] = [];
    const reads: string[] = [];
    page.on("request", (request) => {
      if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
    });
    await page.route(`**/runs/${RUN}/trace*`, (route) => {
      const advanced = new URL(route.request().url()).searchParams.get("mode") === "advanced";
      const targeted = advanced === (mode === "advanced");
      const valid = advanced ? TRACE_ADVANCED : TRACE_GENERAL;
      if (targeted) reads.push(healthy ? "recovered" : "unreadable");
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(targeted && !healthy ? body : valid),
      });
    });
    await page.goto(`/runs/${RUN}${mode === "advanced" ? "?events=0" : ""}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    const verdict = page.locator("#run-decision .verdict");
    await expect(verdict).toHaveText("任務判定：未符合");
    await expect(
      page.getByRole("navigation", { name: "這次試跑的來源", exact: true }),
    ).toBeVisible();
    const trace = page.locator("#run-trace");
    await expect(trace.getByRole("alert")).toHaveText("無法讀取執行紀錄。");
    await expect(
      page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }),
    ).toHaveCount(0);
    await expect(trace.getByText("尚無事件。", { exact: true })).toHaveCount(0);
    await expect(
      trace.getByRole("region", { name: "這次試跑需要你回答", exact: true }),
    ).toHaveCount(0);
    await capture(page, info, "unreadable");
    const retry = trace.getByRole("button", { name: "再試一次", exact: true });
    await retry.focus();
    await expect(retry).toBeFocused();
    healthy = true;
    await page.keyboard.press("Enter");
    await expect(trace.getByRole("alert")).toHaveCount(0);
    await expect(page.locator("#run-trace-heading")).toBeFocused();
    if (mode === "advanced") {
      await expect(
        trace.getByRole("heading", { name: "事件（已遮罩，依序重建）", exact: true }),
      ).toBeVisible();
      await expect(trace.locator(".trace-events")).toContainText("boom");
    } else {
      await expect(trace.getByText("Removed 17 duplicate rows.", { exact: true })).toBeVisible();
      await expect(trace).toContainText("未測量");
    }
    await expect(verdict).toHaveText("任務判定：未符合");
    await capture(page, info, "recovered");
    expect(reads).toContain("unreadable");
    expect(reads).toContain("recovered");
    expect(writes).toEqual([]);
    expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
  });
}
