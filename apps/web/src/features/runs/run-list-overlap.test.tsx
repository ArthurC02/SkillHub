import { act } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, type InfiniteData } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { queryKeys } from "../../core/api/queryKeys";
import { RUNS } from "../../testing/fixtures/platform";
import { useRuns, type RunListItem } from "./runs.service";

type Page = { runs: RunListItem[]; nextOffset: number | undefined };
type History = InfiniteData<Page, number>;
type Condition = {
  name: string;
  arrange: (rows: RunListItem[]) => Page[];
  params: number[];
  counts: number[];
  unique: number;
  next: boolean;
  offsets: Array<number | undefined>;
  unchanged?: boolean;
  changed?: { row: number; id: string; name: string };
};

const FIRST = "444444ab-4444-4444-8444-000000000001";
const BOUNDARY = "444444ab-4444-4444-8444-000000000050";
const UPDATED = "重新讀取的小工具";
const FINAL = "第三次讀取的小工具";
const page = (runs: RunListItem[], nextOffset?: number): Page => ({ runs, nextOffset });

function latest(row: RunListItem, name = UPDATED, uppercase = false): RunListItem {
  const result = {
    ...row,
    skill_name: name,
    run_id: uppercase ? row.run_id.toUpperCase() : row.run_id,
  };
  delete result.status_reason;
  return result;
}

const conditions: Condition[] = [
  {
    name: "a confirmed empty page",
    arrange: () => [page([])],
    params: [0],
    counts: [0],
    unique: 0,
    next: false,
    offsets: [undefined],
    unchanged: true,
  },
  {
    name: "disjoint pages",
    arrange: (rows) => [page(rows.slice(0, 50), 50), page([rows[50]])],
    params: [0, 50],
    counts: [50, 1],
    unique: 51,
    next: false,
    offsets: [50, undefined],
    unchanged: true,
  },
  {
    name: "a repetition inside one page",
    arrange: (rows) => [page([...rows.slice(0, 49), latest(rows[0])])],
    params: [0],
    counts: [49],
    unique: 49,
    next: false,
    offsets: [undefined],
    changed: { row: 0, id: FIRST, name: UPDATED },
  },
  {
    name: "overlapping pages",
    arrange: (rows) => [page(rows.slice(0, 50), 50), page([latest(rows[49]), rows[50]])],
    params: [0, 50],
    counts: [50, 1],
    unique: 51,
    next: false,
    offsets: [50, undefined],
    changed: { row: 49, id: BOUNDARY, name: UPDATED },
  },
  {
    name: "case-equivalent overlapping UUIDs",
    arrange: (rows) => [
      page(rows.slice(0, 50), 50),
      page([latest(rows[49], UPDATED, true), rows[50]]),
    ],
    params: [0, 50],
    counts: [50, 1],
    unique: 51,
    next: false,
    offsets: [50, undefined],
    changed: { row: 49, id: BOUNDARY, name: UPDATED },
  },
  {
    name: "an overlap-only terminal page",
    arrange: (rows) => [page(rows.slice(0, 50), 50), page([latest(rows[49])])],
    params: [0, 50],
    counts: [50, 0],
    unique: 50,
    next: false,
    offsets: [50, undefined],
    changed: { row: 49, id: BOUNDARY, name: UPDATED },
  },
  {
    name: "three snapshots of the same Run",
    arrange: (rows) => [
      page(rows.slice(0, 50), 50),
      page([latest(rows[49]), ...rows.slice(50, 99)], 100),
      page([latest(rows[49], FINAL, true), rows[99]]),
    ],
    params: [0, 50, 100],
    counts: [50, 49, 1],
    unique: 100,
    next: false,
    offsets: [50, 100, undefined],
    changed: { row: 49, id: BOUNDARY, name: FINAL },
  },
  {
    name: "a fully overlapping page with a raw continuation",
    arrange: (rows) => [
      page(rows.slice(0, 50), 50),
      page([...rows.slice(0, 49), latest(rows[49])], 100),
    ],
    params: [0, 50],
    counts: [50, 0],
    unique: 50,
    next: true,
    offsets: [50, 100],
    changed: { row: 49, id: BOUNDARY, name: UPDATED },
  },
];

function Reader() {
  const runs = useRuns();
  return runs.error ? (
    <span>unreadable</span>
  ) : (
    <output>{JSON.stringify({ data: runs.data, next: runs.hasNextPage })}</output>
  );
}

test.each(conditions)(
  "$name retains unique current rows, stable positions and raw pagination",
  async (condition) => {
    const rows = Array.from({ length: 100 }, (_, index) => ({
      ...structuredClone(RUNS.runs[1]),
      run_id: `444444ab-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
    }));
    const input: History = { pages: condition.arrange(rows), pageParams: condition.params };
    const before = structuredClone(input);
    for (const item of input.pages) {
      for (const run of item.runs) Object.freeze(run);
      Object.freeze(item.runs);
      Object.freeze(item);
    }
    Object.freeze(input.pages);
    Object.freeze(input.pageParams);
    Object.freeze(input);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity, gcTime: Infinity } },
    });
    const key = queryKeys.runs.list(undefined, undefined);
    client.setQueryData(key, input);
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    try {
      await act(async () =>
        root.render(
          <QueryClientProvider client={client}>
            <Reader />
          </QueryClientProvider>,
        ),
      );
      expect(container.textContent).not.toBe("unreadable");
      const shown = JSON.parse(container.textContent!) as { data: History; next: boolean };
      expect(shown.data.pages.map((item) => item.runs.length)).toEqual(condition.counts);
      expect(
        new Set(
          shown.data.pages.flatMap((item) => item.runs.map((run) => run.run_id.toLowerCase())),
        ).size,
      ).toBe(condition.unique);
      expect(shown.data.pageParams).toEqual(condition.params);
      expect(shown.data.pages.map((item) => item.nextOffset)).toEqual(condition.offsets);
      expect(shown.next).toBe(condition.next);
      if (condition.unchanged) expect(shown.data).toEqual(JSON.parse(JSON.stringify(before)));
      if (condition.changed) {
        const current = shown.data.pages[0].runs[condition.changed.row];
        expect(current.run_id).toBe(condition.changed.id);
        expect(current.skill_name).toBe(condition.changed.name);
        expect(current).not.toHaveProperty("status_reason");
      }
      expect(client.getQueryData(key)).toEqual(before);
      expect(fetch).not.toHaveBeenCalled();
    } finally {
      await act(async () => root.unmount());
      client.clear();
      container.remove();
      vi.unstubAllGlobals();
    }
  },
);
