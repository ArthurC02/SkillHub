import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { RunTrace } from "./pages/RunTrace";
import type { TraceAdvanced, TraceSummary } from "./api/trace";

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
  usage: { model: "gpt-5-mini", input_tokens: 27042, output_tokens: 1180, cost_credits: null },
  steps: [
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
      payload: { stream: "stderr", message: "<img src=x onerror=alert(1)>", truncated: false },
    },
  ],
};

test("the general mode says the trace is incomplete and never shows an unreported cost as zero", async () => {
  stubTrace(summary, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("部分事件未送達");
  expect(text).toContain("未測量");
  expect(text).not.toContain("0 點");
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
  expect(text).toContain("遲到");
  expect(container.querySelector("table")?.textContent).toContain("2");

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

test("§2.12: a run in flight says which step, that it ends by itself, and that you may leave", async () => {
  stubTrace({ ...summary, status: "running", last_event_at: "2026-08-22T10:04:00Z" }, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("進行中：執行中");
  expect(text).toContain("會自己跑到結束");
  expect(text).toContain("可以關掉這一頁");
  expect(text).toContain("目前已記錄");
  expect(container.querySelector('time[datetime="2026-08-22T10:04:00Z"]')).not.toBeNull();
  expect(text).toMatch(/（\d+ (秒|分鐘|小時|天|週|個月|年)前）|（剛剛）/);
});

test("§2.12: no events yet is a named state, never 0 秒前 and never a blank", async () => {
  stubTrace({ ...summary, status: "provisioning", last_event_at: undefined }, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("還沒有任何事件送達");
  expect(text).not.toContain("0 秒前");
});

test("設計 §2.13：產出清單上每一列都一樣的那兩句，整份清單只講一次", async () => {
  const artifact = (id: string, name: string) => ({
    artifact_id: id,
    file_name: name,
    content_type: "text/csv",
    size_bytes: 2048,
    content_hash: `sha256:${id}`,
    created_at: "2026-08-17T00:03:00Z",
    purged: false,
  });
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/artifacts"))
      return json({
        artifacts: [
          artifact("a1", "one.csv"),
          artifact("a2", "two.csv"),
          artifact("a3", "three.csv"),
        ],
      });
    if (url.includes("/evaluation") || url.includes("/suggestions"))
      return json({ error: "not found" }, 404);
    return json(summary);
  });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("three.csv"));

  const text = container.textContent ?? "";
  const times = (needle: string) => text.split(needle).length - 1;
  expect(times("控制平面不打開它")).toBe(1);
  expect(times("這不表示它會永久保留")).toBe(1);
  expect(times("保存期限：尚未定值")).toBe(3);
});

test("設計 §2.13：進行中留下事實與強制者，推導與指路句不再平鋪", async () => {
  stubTrace({ ...summary, status: "running", last_event_at: "2026-08-22T10:04:00Z" }, advanced);
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("可以關掉這一頁（平台在跑，不是你的瀏覽器）");
  expect(text).not.toContain("不經過瀏覽器");
  expect(text).not.toContain("要提前停止");
  expect(text).toContain("進行中：執行中");
  expect(text).toContain("會自己跑到結束");
  expect(text).toContain("目前已記錄");
  expect(container.querySelector('time[datetime="2026-08-22T10:04:00Z"]')).not.toBeNull();
});

test("§2.13：進行中的 Tip 只有一個，預設收合，推導不與那句事實同一個節點", async () => {
  stubTrace({ ...summary, status: "running", last_event_at: "2026-08-22T10:04:00Z" }, advanced);
  await render();

  const tips = container.querySelectorAll("[data-tip]");
  expect(tips.length).toBe(1);

  const trigger = tips[0].querySelector("button.tip-trigger");
  expect(trigger?.textContent).toBe("為什麼可以關掉這一頁");

  const content = tips[0].querySelector("p.tip-content");
  expect(content?.hasAttribute("hidden")).toBe(true);
  expect(content?.getAttribute("data-role")).toBe("teaching");

  const flatSentence = Array.from(container.querySelectorAll("p")).find((p) =>
    (p.textContent ?? "").includes("可以關掉這一頁"),
  );
  expect(flatSentence?.closest("[data-tip]")).toBeNull();
});

test("§2.12: the banner is gone once the run is terminal", async () => {
  stubTrace(summary, advanced);
  await render();

  expect(container.textContent ?? "").not.toContain("會自己跑到結束");
});

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
  setSearch({ events: "1" });
  stubTrace(summary, (url) => {
    requested.push(url);
    return { ...advanced, has_more: true, next_after: 2 };
  });
  await render();

  await waitFor(() => container.querySelector('nav[aria-label="Trace event pages"]') !== null);
  expect(
    Array.from(container.querySelectorAll("button"))
      .find((button) => button.textContent === "進階模式")
      ?.getAttribute("aria-pressed"),
  ).toBe("true");

  const pager = () =>
    container.querySelector('nav[aria-label="Trace event pages"]')?.textContent ?? "";
  expect(requested.some((url) => url.includes("after=1"))).toBe(true);
  expect(pager()).toContain("第 2 頁");

  const next = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "下一頁",
  );
  await act(async () => next?.click());
  expect(search.events).toBe("1,2");
  await waitFor(() => container.querySelector('nav[aria-label="Trace event pages"]') !== null);

  const previous = Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === "上一頁",
  );
  await act(async () => previous?.click());
  expect(search.events).toBe("1");
});

test("04 丙-145: a run that fails to load says so on its own page, and 失敗類別 does not claim 未記錄", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (/\/runs\/[^/?]+$/.test(url.split("?")[0])) {
      return Promise.resolve(
        new Response(JSON.stringify({ error: "boom" }), {
          status: 500,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
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
    return Promise.resolve(
      new Response(JSON.stringify(summary), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await render();

  const text = container.textContent ?? "";
  expect(text).toContain("無法讀取這個 Run");
  expect(text).not.toContain("未記錄");
});

test("04 丙-145: a run read that needs login says so, on its own page", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (/\/runs\/[^/?]+$/.test(url.split("?")[0])) {
      return Promise.resolve(
        new Response(JSON.stringify({ error: "not authenticated" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
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
    return Promise.resolve(
      new Response(JSON.stringify(summary), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await render();

  expect(container.textContent ?? "").toContain("需要登入");
});

test("04 丙-145: cleanup_status renders on the run's own page", async () => {
  stubTrace(summary, advanced, {
    run_id: summary.run_id,
    skill_id: "s-1",
    skill_version_id: "v-1",
    test_case_snapshot_id: "snap-1",
    cleanup_status: {
      value: "failed",
      label: "清理失敗",
      note: "沙箱清理失敗，可能仍佔用資源。",
    },
  });
  await render();

  expect(container.textContent ?? "").toContain("清理失敗");
});

test("丙-115 進度 writes the status in this app's own words and relays the reason untouched", async () => {
  stubTrace(summary, advanced);
  await render();
  const steps = Array.from(container.querySelectorAll("ol li")).map((li) => li.textContent ?? "");
  const progress = steps.filter((t) => t.includes("已收到") || t.includes("could not carry"));

  expect(progress).toEqual([
    "排隊中：已收到這次 Run 的請求",
    "執行失敗：the provider could not carry the attempt",
  ]);
  expect(progress.join("")).not.toContain("queued");
  expect(progress.join("")).not.toContain("failed:");
});
