import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  correctedSearchText,
  emptySearchIntent,
  readSearchCorrection,
} from "../../core/api/searchIntent";
import type { SearchInterpretation } from "../../core/api/types";
import { IntentInterpretation } from "./home/components/IntentInterpretation";
import App from "../../app/App";
import { router } from "../../app/router";
import { queryClient } from "../../core/api/queryClient";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

function interpretation(status: SearchInterpretation["status"] = "analyzed"): SearchInterpretation {
  return {
    status,
    intent: { ...emptySearchIntent(), input: "invoice" },
    keywords: ["invoice"],
    filters: {},
  };
}

async function click(text: string) {
  const button = [...container.querySelectorAll("button")].find(
    (node) => node.textContent === text,
  );
  expect(button).toBeDefined();
  await act(async () => button!.click());
}

test("analysis displays all five fields without inferring missing facts", async () => {
  await act(async () =>
    root.render(<IntentInterpretation interpretation={interpretation()} onCorrect={vi.fn()} />),
  );
  expect([...container.querySelectorAll("dt")].map((node) => node.textContent)).toEqual([
    "輸入",
    "輸出",
    "工具",
    "資料",
    "環境",
  ]);
  expect([...container.querySelectorAll("dd")].map((node) => node.textContent)).toEqual([
    "invoice",
    "未提及",
    "未提及",
    "未提及",
    "未提及",
  ]);
});

test("unavailable analysis reports unknown instead of claiming unmentioned facts", async () => {
  await act(async () =>
    root.render(
      <IntentInterpretation interpretation={interpretation("fallback")} onCorrect={vi.fn()} />,
    ),
  );
  expect(container.querySelector('[role="status"]')?.textContent).toContain("意圖分析暫時無法使用");
  expect([...container.querySelectorAll("dd")].map((node) => node.textContent)).toEqual(
    Array(5).fill("未測量"),
  );
});

test("editing an output preserves keywords and submits the changed field", async () => {
  const onCorrect = vi.fn();
  await act(async () =>
    root.render(<IntentInterpretation interpretation={interpretation()} onCorrect={onCorrect} />),
  );
  await click("修正搜尋理解");
  const output = container.querySelectorAll("input")[1];
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(output, "CSV");
    output.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await click("套用修正並搜尋");
  expect(onCorrect).toHaveBeenCalledExactlyOnceWith({
    intent: { ...emptySearchIntent(), input: "invoice", output: "CSV" },
    keywords: ["invoice"],
  });
  expect(container.querySelector("form")).toBeNull();
});

test("overturning interpretation clears every field keyword and filter", async () => {
  const onCorrect = vi.fn();
  await act(async () =>
    root.render(<IntentInterpretation interpretation={interpretation()} onCorrect={onCorrect} />),
  );
  await click("捨棄理解與篩選，以原句搜尋");
  expect(onCorrect).toHaveBeenCalledExactlyOnceWith(
    { intent: emptySearchIntent(), keywords: [] },
    true,
  );
});

test("corrected retrieval text includes each field in order and removes duplicates", () => {
  expect(
    correctedSearchText({
      intent: {
        input: "invoice",
        output: "CSV",
        tools: "Python",
        data: "ledger",
        environment: "offline",
      },
      keywords: ["invoice", "report", "report"],
    }),
  ).toBe("invoice CSV Python ledger offline report");
  expect(correctedSearchText({ intent: emptySearchIntent(), keywords: [] })).toBe("");
});

test.each([2000, 2001])("combined correction validates the %i character boundary", (length) => {
  const raw = JSON.stringify({
    intent: { ...emptySearchIntent(), input: "文".repeat(length - 2) },
    keywords: ["字"],
  });
  if (length === 2000) {
    expect(correctedSearchText(readSearchCorrection(raw))).toBe("文".repeat(1998) + " 字");
  } else {
    expect(() => readSearchCorrection(raw)).toThrow("五欄理解與關鍵詞合計最多 2000 字");
  }
});

test.each(["{", "undefined", ""])(
  "malformed correction JSON %j reports a readable error",
  (raw) => {
    expect(() => readSearchCorrection(raw)).toThrow("搜尋修正格式無效");
  },
);

test.each([null, 7, "text", [], {}, { intent: emptySearchIntent() }, { keywords: [] }])(
  "correction %j requires both intent and keywords",
  (value) => {
    expect(() => readSearchCorrection(JSON.stringify(value))).toThrow("搜尋修正缺少意圖或關鍵詞");
  },
);

test.each([
  null,
  [],
  { input: null, output: null, tools: null, data: null },
  { input: null, output: null, tools: null, data: null, unknown: null },
  { ...emptySearchIntent(), extra: null },
  { ...emptySearchIntent(), input: 42 },
  { ...emptySearchIntent(), tools: "" },
  { ...emptySearchIntent(), environment: " \n\t" },
])("correction rejects malformed five-field intent %j", (intent) => {
  expect(() => readSearchCorrection(JSON.stringify({ intent, keywords: [] }))).toThrow(
    "意圖必須包含五欄",
  );
});

test.each([null, "word", [1], [null], [""], [" \n\t"]])(
  "correction rejects malformed keyword collection %j",
  (keywords) => {
    expect(() =>
      readSearchCorrection(JSON.stringify({ intent: emptySearchIntent(), keywords })),
    ).toThrow("關鍵詞最多 8 組，每組最多 128 字");
  },
);

test.each([0, 8, 9])("correction validates the %i keyword-count boundary", (count) => {
  const correction = { intent: emptySearchIntent(), keywords: Array<string>(count).fill("word") };
  const read = () => readSearchCorrection(JSON.stringify(correction));
  if (count <= 8) expect(read()).toEqual(correction);
  else expect(read).toThrow("關鍵詞最多 8 組，每組最多 128 字");
});

test.each([128, 129])(
  "correction counts %i astral keyword characters as Unicode code points",
  (length) => {
    const correction = { intent: emptySearchIntent(), keywords: ["😀".repeat(length)] };
    const read = () => readSearchCorrection(JSON.stringify(correction));
    if (length === 128) expect(read()).toEqual(correction);
    else expect(read).toThrow("關鍵詞最多 8 組，每組最多 128 字");
  },
);

test.each([2000, 2001])(
  "correction counts %i astral field characters as Unicode code points",
  (length) => {
    const correction = {
      intent: { ...emptySearchIntent(), output: "😀".repeat(length) },
      keywords: [],
    };
    const read = () => readSearchCorrection(JSON.stringify(correction));
    if (length === 2000) expect(read()).toEqual(correction);
    else expect(read).toThrow("意圖必須包含五欄，每欄最多 2000 字");
  },
);

test("home posts corrections and resets but starts a new query without the old correction", async () => {
  const requests: { method: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    const url = new URL(String(input), window.location.origin);
    if (url.pathname === "/api/skills/search") {
      const method = init?.method ?? "GET";
      const body =
        method === "POST" ? JSON.parse(String(init?.body)) : { query: url.searchParams.get("q") };
      requests.push({ method, body });
      return new Response(
        JSON.stringify({
          query: body.query,
          results: [],
          degraded: false,
          partial_index: false,
          no_results: true,
          filtered_out: false,
          limit: 20,
          truncated: false,
          total: 0,
          interpretation:
            method === "POST"
              ? {
                  status: "corrected",
                  intent: body.intent,
                  keywords: body.keywords,
                  filters: body.filters,
                }
              : { ...interpretation(), filters: { category: "documents" } },
        }),
      );
    }
    if (url.pathname === "/api/skills/catalog") {
      return new Response(JSON.stringify({ results: [], total: 0, limit: 100, truncated: false }));
    }
    return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
  });
  await act(async () => root.render(<App />));
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "原本的請求" } });
  });
  await vi.waitFor(async () => {
    await act(async () => Promise.resolve());
    expect(container.querySelector(".intent-interpretation")).not.toBeNull();
  });
  await click("修正搜尋理解");
  const output = container.querySelectorAll(".intent-interpretation input")[1];
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(output, "CSV");
    output.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await click("套用修正並搜尋");
  await vi.waitFor(async () => {
    await act(async () => Promise.resolve());
    expect(container.textContent).toContain("你修正的搜尋理解");
  });
  expect(requests.at(-1)).toEqual({
    method: "POST",
    body: {
      query: "原本的請求",
      intent: { ...emptySearchIntent(), input: "invoice", output: "CSV" },
      keywords: ["invoice"],
      filters: { category: "documents" },
      limit: 20,
    },
  });
  expect(container.querySelector(".intent-interpretation")?.textContent).toContain("invoice CSV");
  await click("捨棄理解與篩選，以原句搜尋");
  await vi.waitFor(async () => {
    await act(async () => Promise.resolve());
    expect(requests.at(-1)?.body.filters).toEqual({});
  });
  expect(requests.at(-1)).toEqual({
    method: "POST",
    body: {
      query: "原本的請求",
      intent: emptySearchIntent(),
      keywords: [],
      filters: {},
      limit: 20,
    },
  });
  expect(requests.filter((request) => request.method === "GET")).toHaveLength(1);
  expect(router.state.location.search.q).toBe("原本的請求");
  expect(router.state.location.search.category).toBeUndefined();

  const taskInput = container.querySelector<HTMLInputElement>('input[aria-label="任務描述"]')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(
      taskInput,
      "新的任務",
    );
    taskInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await click("搜尋");
  await vi.waitFor(async () => {
    await act(async () => Promise.resolve());
    expect(requests.at(-1)).toEqual({ method: "GET", body: { query: "新的任務" } });
  });
  expect(router.state.location.search.q).toBe("新的任務");
  expect(router.state.location.search.correction).toBeUndefined();
}, 15000);
