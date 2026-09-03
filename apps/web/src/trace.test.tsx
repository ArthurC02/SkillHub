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

/*
 * The page reads its run id from the router, which is not mounted here, so the
 * router surface it touches is stubbed rather than driving a full navigation for
 * one id. `Link` renders its children so the page's cross-links stay inspectable.
 *
 * The search half is a real (tiny) store rather than a constant `{}`: 資訊架構
 * §0.1 R4 puts the advanced Trace's page position in the URL, so a `useNavigate`
 * that swallowed the write would let a page that never leaves component state
 * pass. Components re-read it through `useSyncExternalStore`, which is what
 * makes a navigation land on screen the way a real one does.
 */
const searchListeners = new Set<() => void>();
let search: Record<string, string | undefined> = {};

function setSearch(next: Record<string, string | undefined>) {
  search = next;
  for (const listener of searchListeners) listener();
}

vi.mock("@tanstack/react-router", async () => {
  const { useSyncExternalStore } = await import("react");
  return {
    useParams: () => ({ runId: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20" }),
    useSearch: () =>
      useSyncExternalStore(
        (listener: () => void) => {
          searchListeners.add(listener);
          return () => searchListeners.delete(listener);
        },
        () => search,
      ),
    useNavigate: () => (options: { search?: unknown }) => {
      const next =
        typeof options.search === "function"
          ? (options.search as (prev: typeof search) => typeof search)(search)
          : (options.search as typeof search);
      setSearch({ ...next });
      return Promise.resolve();
    },
    Link: ({ children }: { children?: unknown }) => children,
  };
});

beforeEach(() => setSearch({}));

function stubTrace(
  general: TraceSummary,
  advanced: TraceAdvanced | ((url: string) => TraceAdvanced),
  /**
   * `GET /runs/{id}`. Answered with the trace body until now — every field the
   * page read off it happened to exist there too, so nothing noticed that this
   * endpoint was never really stubbed, and `failure_class` (the one field that
   * says WHY a run failed) could go unrendered without a single test caring.
   */
  run?: Record<string, unknown>,
) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (run && /\/runs\/[^/?]+$/.test(url.split("?")[0])) {
      return Promise.resolve(
        new Response(JSON.stringify(run), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
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
  steps: [
    // One of each kind the field carries, which is what the server now
    // produces: the platform's own sentence, in the interface language, and
    // one relayed verbatim from the provider, which nothing translates.
    { status: "queued", reason: "已收到這次 Run 的請求" },
    { status: "failed", reason: "the provider could not carry the attempt" },
  ],
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
  // The gateway reported no cost. Rendering 0 would tell the user it was free,
  // and 設計 §2.9's table has one word for it (「未回報」 was not on the table).
  expect(text).toContain("未測量");
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
  expect(container.querySelector('time[datetime="2026-08-22T10:04:00Z"]')).not.toBeNull();
  // 設計 §2.12 第 3 條's 「多久沒動了」 is now a quantity a reader can act on
  // rather than a UTC string they have to subtract in their head.
  expect(text).toMatch(/（\d+ (秒|分鐘|小時|天|週|個月|年)前）|（剛剛）/);
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

/*
 * 資訊架構 §0.1 R4 — 「你在看哪一份東西」進網址.
 *
 * The advanced Trace's page position was `useState([0])`, so page 7 of the
 * event stream could not be linked and did not survive a reload — on the screen
 * system.md §1.3 itself calls 「最需要把『你看，這裡少了三筆』寄給別人的畫面」.
 *
 * Both halves are asserted, because either one alone is passable by a wrong
 * implementation: reading the address (an address somebody was sent opens on
 * that page) and writing it (the page you paged to is the address you can send).
 */
/**
 * 設計 §3 checklist 第 4 條逐字寫的失效形狀——「型別裡有、伺服器送了、頁面丟掉」。
 *
 * `/workspace/runs` 的每一列都印著「失敗類別 能力不符」外加一整句解釋，而使用者點進
 * 這一頁想知道細節，得到的是**更少**：只有「執行狀態：執行失敗（failed）」。
 * `failure_class` 從頭到尾沒有在這一頁出現過，因為 `api/runs.ts` 的手寫 `Run` 型別
 * 漏了它——contract 在 2026-09-01 才補上宣告，而伺服器**早就在送**（那段 description
 * 逐字寫著這件事，以及為什麼沒有機器發現：Go 側是 models-only、handler 手寫，所以
 * handler 送得出 contract 沒宣告的東西）。
 *
 * 這個差別就是下一步本身：`workload_error` 是 Skill 自己做不到，`capability_mismatch`
 * 是平台在跑之前就拒絕。
 */
test("設計 §3 第 4 條：失敗的 Run 在自己的頁面上要說出失敗類別，不能比清單頁說得少", async () => {
  stubTrace(summary, advanced, {
    run_id: "r-1",
    skill_id: "s-1",
    skill_version_id: "v-1",
    test_case_snapshot_id: "snap-1",
    failure_class: {
      value: "capability_mismatch",
      label: "能力不符",
      note: "平台在跑之前就拒絕了這次 Run，不是 Skill 執行到一半失敗。",
    },
  });
  await render();

  expect(container.textContent).toContain("能力不符");
  expect(container.textContent).toContain("平台在跑之前就拒絕了這次 Run");
  expect(container.textContent).not.toContain("失敗類別：未記錄");
});

test("R4: the advanced Trace opens on the page its address names, and paging writes it back", async () => {
  const requested: string[] = [];
  // The address of page 2: one cursor pushed onto the implicit page-1 zero.
  setSearch({ events: "1" });
  stubTrace(summary, (url) => {
    requested.push(url);
    return { ...advanced, has_more: true, next_after: 2 };
  });
  await render();

  // 沒有「先按一下進階模式」這一步了，而它的消失就是這一批修掉的東西：`?events=`
  // 早就是網址狀態，但初始 mode 硬編成 general，所以收到連結的人打開之後看到的是
  // 一般模式的摘要——位置參數不作用、畫面上也沒有一個字提到它存在，他得先自己猜到
  // 要按「進階模式」。這支測試以前**自己按了那顆按鈕**，也就是它驗的是「按了之後
  // 讀不讀網址」，而不是它自己註解裡寫的「an address somebody was sent opens on
  // that page」。
  await waitFor(() => container.querySelector('nav[aria-label="Trace event pages"]') !== null);
  expect(
    Array.from(container.querySelectorAll("button"))
      .find((button) => button.textContent === "進階模式")
      ?.getAttribute("aria-pressed"),
  ).toBe("true");

  const pager = () =>
    container.querySelector('nav[aria-label="Trace event pages"]')?.textContent ?? "";
  // Read: the cursor came from the address, not from a stack that always starts
  // empty. With the position in component state this asks the server for page 1.
  expect(requested.some((url) => url.includes("after=1"))).toBe(true);
  expect(pager()).toContain("第 2 頁");

  // Write: paging on is a change of address, so the reader can send what they
  // are looking at.
  const next = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "下一頁",
  );
  await act(async () => next?.click());
  expect(search.events).toBe("1,2");
  // The new page is a fresh (heavy) request, so the pager leaves the DOM while
  // it loads; wait for it back before driving it again.
  await waitFor(() => container.querySelector('nav[aria-label="Trace event pages"]') !== null);

  // And back again — the stack travels, because a cursor is a receive-order
  // offset and page 2's position cannot be recomputed from page 3's.
  const previous = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "上一頁",
  );
  await act(async () => previous?.click());
  expect(search.events).toBe("1");
});

/**
 * 04 丙-115 ①. The progress list printed whatever the server had pre-joined,
 * which was `"<status>: <reason>"` — so this page wrote the same status two ways
 * within four lines: 「執行完成」 in the status paragraph, `succeeded:` in the list
 * underneath it. The server now sends the two fields apart and the mapping this
 * app already owns does the writing.
 *
 * The reason is asserted to arrive UNCHANGED, both kinds. That half is not
 * decoration: this field also carries sentences relayed verbatim from the
 * provider and the text of Go errors, and a surface that translated what it
 * recognised while silently rewriting the rest would be inventing words for a
 * system whose words it does not have.
 */
test("丙-115 進度 writes the status in this app's own words and relays the reason untouched", async () => {
  stubTrace(summary, advanced);
  await render();
  const steps = Array.from(container.querySelectorAll("ol li")).map((li) => li.textContent ?? "");
  const progress = steps.filter((t) => t.includes("已收到") || t.includes("could not carry"));

  expect(progress).toEqual([
    "排隊中：已收到這次 Run 的請求",
    "執行失敗：the provider could not carry the attempt",
  ]);
  // The raw enum is gone from the list. `queued` as a bare token was the defect.
  expect(progress.join("")).not.toContain("queued");
  expect(progress.join("")).not.toContain("failed:");
});
