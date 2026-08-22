import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { RunTrace } from "./pages/RunTrace";
import type { TraceAdvanced, TraceSummary } from "./api/trace";

// TRACE-006/007. Three assertions, one per rule the view exists to keep:
// incompleteness is stated, an unreported cost is not zero, and a sandbox
// payload is rendered as inert text.
//
// Same hand-rolled DOM plumbing as disc.test.tsx: @testing-library is not a
// dependency and these assertions do not justify adding one.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

// The page reads its run id from the router, which is not mounted here, so the
// router surface it touches is stubbed rather than driving a full navigation for
// one id. `Link` renders its children so the page's cross-links stay inspectable.
vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20" }),
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
  Link: ({ children }: { children?: unknown }) => children,
}));

function stubTrace(
  general: TraceSummary,
  advanced: TraceAdvanced | ((url: string) => TraceAdvanced),
) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    // This run was never evaluated: the server answers 404, which the page
    // renders as 未評估 rather than as a pass (ADR-025).
    // This run produced no files. The artifacts section is not what these two
    // tests are about, but it is on the page, so the stub answers it.
    if (url.includes("/artifacts")) {
      return Promise.resolve(
        new Response(JSON.stringify({ artifacts: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    if (url.includes("/evaluation") || url.includes("/suggestions")) {
      return Promise.resolve(
        new Response(JSON.stringify({ error: "not found" }), {
          status: 404,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    const body = url.includes("mode=advanced")
      ? typeof advanced === "function"
        ? advanced(url)
        : advanced
      : general;
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

async function render() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <RunTrace />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => container.querySelector("[data-loading]") === null);
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const summary: TraceSummary = {
  run_id: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20",
  status: "failed",
  status_reason: "the provider could not carry the attempt",
  complete: false,
  skills: [{ name: "excel-deduplicate", decision: "activated" }],
  skills_total: 1,
  resources_read: 1,
  tool_calls: {
    total: 2,
    succeeded: 1,
    failed: 1,
    total_duration_ms: 3532,
    slowest_duration_ms: 3412,
    slowest_tool: "bash",
  },
  errors: [{ category: "provision", code: "provider_error", message: "no slot" }],
  errors_total: 1,
  summary_truncated: false,
  final_output: "Removed 17 duplicate rows.",
  usage: { model: "gpt-5-mini", input_tokens: 27042, output_tokens: 1180, cost_usd: null },
  steps: ["queued: run requested", "failed: the provider could not carry the attempt"],
};

const advanced: TraceAdvanced = {
  run_id: summary.run_id,
  complete: false,
  next_after: 1,
  has_more: false,
  streams: [
    {
      attempt: 1,
      emitted_by: "sandbox",
      received: 2,
      highest_seq: 3,
      missing_count: 1,
      missing_seq: [2],
      late_events: 1,
    },
  ],
  events: [
    {
      event_id: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
      attempt: 1,
      seq: 1,
      occurred_at: "2026-08-16T09:12:04.002Z",
      emitted_by: "sandbox",
      type: "script_log",
      status: "error",
      late: true,
      masked_fields: ["/message"],
      // A workload that tries to inject markup into the timeline.
      payload: { stream: "stderr", message: "<img src=x onerror=alert(1)>", truncated: false },
    },
  ],
};

test("the general mode says the trace is incomplete and never shows an unreported cost as zero", async () => {
  stubTrace(summary, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("部分事件未送達");
  // The gateway reported no cost. Rendering 0 would tell the user it was free.
  expect(text).toContain("未回報");
  expect(text).not.toContain("US$0.0000");
  // Status is whatever the runs table said, reason included (iron rule 5).
  expect(text).toContain("failed");
});

test("the advanced mode names the missing sequence numbers and renders payloads inert", async () => {
  stubTrace(summary, advanced);
  await render();

  const advancedButton = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "進階模式",
  );
  expect(advancedButton).toBeDefined();
  await act(async () => {
    advancedButton?.click();
  });
  await waitFor(() => container.querySelector("table") !== null);

  const text = container.textContent ?? "";
  // A hole in a producer's gapless sequence is a lost event, and the view has to
  // name it rather than present a shorter timeline as the whole one.
  expect(text).toContain("遲到");
  expect(container.querySelector("table")?.textContent).toContain("2");

  // The injected tag must exist as text and not as an element: everything under
  // emitted_by=sandbox crossed the trust boundary (ADR-001, ADR-009).
  expect(container.querySelector("img")).toBeNull();
  expect(container.querySelector("pre")?.textContent).toContain("<img src=x onerror=alert(1)>");
});

test("the advanced mode pages through the complete trace without retaining every payload", async () => {
  const requested: string[] = [];
  stubTrace(summary, (url) => {
    requested.push(url);
    if (url.includes("after=1")) {
      return {
        ...advanced,
        next_after: 2,
        has_more: false,
        events: [{ ...advanced.events[0], event_id: "page-two", seq: 2 }],
      };
    }
    return { ...advanced, has_more: true };
  });
  await render();

  const advancedButton = Array.from(container.querySelectorAll("button")).find(
    (button) => button.getAttribute("aria-pressed") === "false",
  );
  await act(async () => advancedButton?.click());
  await waitFor(() => container.querySelector('nav[aria-label="Trace event pages"]') !== null);

  const next = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "下一頁",
  );
  expect(next?.disabled).toBe(false);
  await act(async () => next?.click());
  await waitFor(
    () =>
      requested.some((url) => url.includes("after=1")) &&
      (container.querySelector("ol")?.textContent?.includes("#2") ?? false),
  );
  expect(container.querySelector("ol")?.textContent ?? "").toContain("#2");

  const beforeRefresh = requested.filter((url) => url.includes("after=1")).length;
  const refresh = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "重新整理 Trace",
  );
  await act(async () => refresh?.click());
  await waitFor(() => requested.filter((url) => url.includes("after=1")).length > beforeRefresh);
});

// 設計 §2.12 / ADR-042 決策 2. The third axis: what is happening, as opposed to
// what happened. A run in flight had no rendering of its own — the page showed
// a trace that simply had less in it, which reads the same as a run that did
// very little and finished.
test("§2.12: a run in flight says which step, that it ends by itself, and that you may leave", async () => {
  stubTrace({ ...summary, status: "running", last_event_at: "2026-08-22T10:04:00Z" }, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("進行中：執行中");
  expect(text).toContain("會自己跑到結束");
  // Not a guess: run_execute is a River job consumed by cmd/worker and trace
  // events are posted by the sandbox, so the browser is in neither path.
  expect(text).toContain("可以關掉這一頁");
  // The two facts a spinner cannot carry: something that moves, and how long
  // since it moved.
  expect(text).toContain("目前已記錄");
  expect(text).toContain("2026-08-22T10:04:00Z");
});

test("§2.12: no events yet is a named state, never 0 秒前 and never a blank", async () => {
  // A run still being provisioned genuinely has produced nothing. §2.9 rates the
  // blank worst and a zero elapsed time worse still — it says "just moved" about
  // something that has not started.
  stubTrace({ ...summary, status: "provisioning", last_event_at: undefined }, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("還沒有任何事件送達");
  expect(text).not.toContain("0 秒前");
});

test("§2.12: the banner is gone once the run is terminal", async () => {
  // A finished run already answers with a verdict and an execution state; a
  // third answer about waiting would be one fact worded twice (§3 第 14 條).
  stubTrace(summary, advanced);
  await render();

  expect(container.textContent ?? "").not.toContain("會自己跑到結束");
});
