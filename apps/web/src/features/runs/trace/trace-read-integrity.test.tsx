import { expect, test } from "vitest";
import { RUN, TRACE_ADVANCED, TRACE_GENERAL } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

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
  test(`a ${name} stays local and recovers without losing the decision`, async () => {
    const page = app();
    let healthy = false;
    const reads: string[] = [];
    page.route(`**/runs/${RUN}/trace*`, ({ url }) => {
      const advanced = url.searchParams.get("mode") === "advanced";
      const targeted = advanced === (mode === "advanced");
      const valid = advanced ? TRACE_ADVANCED : TRACE_GENERAL;
      if (targeted) reads.push(healthy ? "recovered" : "unreadable");
      return { status: 200, json: targeted && !healthy ? body : valid };
    });
    await page.goto(`/runs/${RUN}${mode === "advanced" ? "?events=0" : ""}`);
    const verdict = page.locator("#run-decision .verdict");
    const trace = page.locator("#run-trace");
    await page.eventually(() => {
      expect(page.locator(".auth-name").text()).toBe("tester");
      expect(verdict.text()).toBe("任務判定：未符合");
      expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true }).count()).toBe(1);
      expect(trace.getByRole("alert").text()).toBe("無法讀取執行紀錄。");
      expect(page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }).count()).toBe(
        0,
      );
      expect(trace.getByText("尚無事件。", { exact: true }).count()).toBe(0);
      expect(trace.getByRole("region", { name: "這次試跑需要你回答", exact: true }).count()).toBe(
        0,
      );
    });
    expect(await page.axeViolations("main")).toEqual([]);
    const retry = trace.getByRole("button", { name: "再試一次", exact: true });
    await retry.focus();
    await expect(retry).toBeFocused();
    healthy = true;
    await page.press("Enter");
    await page.eventually(() => {
      expect(trace.getByRole("alert").count()).toBe(0);
      expect(page.locator("#run-trace-heading").isFocused()).toBe(true);
      if (mode === "advanced") {
        expect(
          trace.getByRole("heading", { name: "事件（已遮罩，依序重建）", exact: true }).count(),
        ).toBe(1);
        expect(trace.locator(".trace-events").text()).toContain("boom");
      } else {
        expect(trace.getByText("Removed 17 duplicate rows.", { exact: true }).count()).toBe(1);
        expect(trace.text()).toContain("未測量");
      }
      expect(verdict.text()).toBe("任務判定：未符合");
    });
    expect(await page.axeViolations("main")).toEqual([]);
    expect(reads).toContain("unreadable");
    expect(reads).toContain("recovered");
    expect(
      page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method)),
    ).toEqual([]);
    expect(page.pathname()).toBe(`/runs/${RUN}`);
  });
}
