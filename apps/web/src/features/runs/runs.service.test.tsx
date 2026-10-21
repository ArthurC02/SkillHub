import { act } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { RUN, OTHER_RUN, SKILL, VERSION } from "../../testing/fixtures/platform";
import { queryKeys } from "../../core/api/queryKeys";
import { useRun, type Run } from "./runs.service";

const source = {
  run_id: RUN,
  status: "failed",
  skill_id: SKILL,
  skill_version_id: VERSION,
  test_case_snapshot_id: "44444444-4444-4444-4444-444444444444",
  provider: "unassigned",
  cleanup_status: { value: "cleaned", label: "已清理", note: "沙箱已拆除。" },
  created_at: "2026-08-16T00:00:00Z",
} satisfies Run;

function RunReader() {
  const run = useRun(RUN);
  return <span>{run.error ? "unreadable" : run.data ? "loaded" : "pending"}</span>;
}

for (const { name, body, readable } of [
  { name: "own Run without an optional draft", body: source, readable: true },
  {
    name: "case-equivalent Run identity",
    body: { ...source, run_id: RUN.toUpperCase() },
    readable: true,
  },
  { name: "null source", body: null, readable: false },
  { name: "array source", body: [], readable: false },
  { name: "numeric source", body: 42, readable: false },
  { name: "boolean source", body: true, readable: false },
  { name: "string source", body: RUN, readable: false },
  { name: "missing identity", body: { ...source, run_id: undefined }, readable: false },
  { name: "numeric identity", body: { ...source, run_id: 42 }, readable: false },
  { name: "null identity", body: { ...source, run_id: null }, readable: false },
  { name: "foreign Run identity", body: { ...source, run_id: OTHER_RUN }, readable: false },
]) {
  test(`Run reads classify ${name} without caching another source`, async () => {
    vi.useFakeTimers();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body))));
    vi.stubGlobal("fetch", fetch);
    try {
      await act(async () => {
        root.render(
          <QueryClientProvider client={client}>
            <RunReader />
          </QueryClientProvider>,
        );
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(fetch).toHaveBeenCalledTimes(1);
      expect(fetch).toHaveBeenCalledWith(
        `/runs/${RUN}`,
        expect.objectContaining({ credentials: "include" }),
      );
      expect(container.textContent).toBe(readable ? "loaded" : "unreadable");
      const state = client.getQueryState(queryKeys.runs.detail(RUN));
      expect(state?.status).toBe(readable ? "success" : "error");
      if (readable) {
        expect(state?.data).toEqual(body);
        expect(state?.error).toBeNull();
      } else {
        expect(state?.data).toBeUndefined();
        expect(state?.error).toEqual(new Error("Invalid Run response"));
      }
    } finally {
      await act(async () => root.unmount());
      client.clear();
      container.remove();
      vi.unstubAllGlobals();
      vi.useRealTimers();
    }
  });
}
