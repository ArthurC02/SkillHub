import { act } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { OTHER_RUN, RUNS, SKILL, TEST_CASE, VERSION } from "../../testing/fixtures/platform";
import { queryKeys } from "../../core/api/queryKeys";
import { useRuns } from "./runs.service";

const row = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;

function Reader() {
  const runs = useRuns({ testCaseId: TEST_CASE, skillVersionId: VERSION });
  return <span>{runs.error ? "unreadable" : runs.data ? "loaded" : "pending"}</span>;
}

async function read(body: unknown) {
  vi.useFakeTimers();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const container = document.createElement("div");
  const root = createRoot(container);
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body))));
  vi.stubGlobal("fetch", fetch);
  try {
    await act(async () => {
      root.render(
        <QueryClientProvider client={client}>
          <Reader />
        </QueryClientProvider>,
      );
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetch).toHaveBeenCalledExactlyOnceWith(
      `/runs?limit=51&offset=0&test_case_id=${TEST_CASE}&skill_version_id=${VERSION}`,
      expect.objectContaining({ credentials: "include" }),
    );
    return {
      text: container.textContent,
      state: client.getQueryState(queryKeys.runs.list(TEST_CASE, VERSION)),
    };
  } finally {
    await act(async () => root.unmount());
    client.clear();
    container.remove();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  }
}

test.each([
  ["empty", []],
  ["known", [row]],
  [
    "forward-compatible labels and optional facts",
    [
      {
        ...row,
        status: "future_status",
        evaluation: { value: "future_verdict", label: "尚無新判定", note: "等待更新" },
        cleanup_status: { value: "future_cleanup", label: "清理狀態未知" },
        failure_class: { value: "future_failure", label: "其他原因" },
        status_reason: "保留伺服器說明",
        skill_id: SKILL,
        test_case_id: TEST_CASE,
        started_at: "not-a-time",
        finished_at: "2026-08-16T00:01:00Z",
      },
    ],
  ],
] as const)("a %s Run list retains its original facts", async (_name, runs) => {
  const result = await read({ runs });
  expect(result.text).toBe("loaded");
  expect(result.state?.status).toBe("success");
  expect(result.state?.error).toBeNull();
  expect(result.state?.data).toEqual({ pages: [{ runs, nextOffset: undefined }], pageParams: [0] });
});

test.each([
  [50, undefined],
  [51, 50],
])("a %i-row response preserves the page boundary", async (count, nextOffset) => {
  const runs = Array.from({ length: count! }, (_, index) => ({
    ...row,
    skill_name: `Run ${index}`,
  }));
  const result = await read({ runs });
  expect(result.text).toBe("loaded");
  expect(result.state?.data).toMatchObject({ pages: [{ nextOffset }], pageParams: [0] });
  const data = result.state?.data as { pages: Array<{ runs: Array<{ skill_name: string }> }> };
  expect(data.pages[0].runs).toHaveLength(50);
  expect(data.pages[0].runs[0].skill_name).toBe("Run 0");
  expect(data.pages[0].runs[49].skill_name).toBe("Run 49");
  expect(data.pages[0].runs).not.toContainEqual(expect.objectContaining({ skill_name: "Run 50" }));
});

const invalid: Array<[string, unknown]> = [
  ["null envelope", null],
  ["primitive envelope", "runs"],
  ["array envelope", []],
  ["missing list", {}],
  ["null list", { runs: null }],
  ["string list", { runs: "not-a-list" }],
  ["object list", { runs: { 0: row } }],
  ["null row", { runs: [null] }],
  ["primitive row", { runs: [42] }],
  ["array row", { runs: [[]] }],
  ["missing identity", { runs: [{ ...row, run_id: undefined }] }],
  ["null verdict", { runs: [{ ...row, evaluation: null }] }],
  ["missing cleanup", { runs: [{ ...row, cleanup_status: undefined }] }],
  ["invalid sentinel", { runs: [...Array.from({ length: 50 }, () => row), null] }],
];
for (const field of [
  "run_id",
  "status",
  "skill_id",
  "skill_name",
  "skill_version_id",
  "provider",
  "created_at",
  "status_reason",
  "test_case_id",
  "started_at",
  "finished_at",
]) {
  invalid.push([`non-text ${field}`, { runs: [{ ...row, [field]: 42 }] }]);
}
for (const field of ["evaluation", "cleanup_status", "failure_class"]) {
  for (const [name, label] of [
    ["primitive", "unknown"],
    ["array", []],
    ["missing value", { label: "未知" }],
    ["non-text value", { value: 42, label: "未知" }],
    ["missing label", { value: "unknown" }],
    ["non-text label", { value: "unknown", label: {} }],
    ["non-text note", { value: "unknown", label: "未知", note: {} }],
  ]) {
    invalid.push([`${name} ${field}`, { runs: [{ ...row, [field]: label }] }]);
  }
}

test.each(invalid)("a %s response is rejected before caching rows", async (_name, body) => {
  const result = await read(body);
  expect(result.text).toBe("unreadable");
  expect(result.state?.status).toBe("error");
  expect(result.state?.data).toBeUndefined();
  expect(result.state?.error).toEqual(new Error("Invalid Run list response"));
});
