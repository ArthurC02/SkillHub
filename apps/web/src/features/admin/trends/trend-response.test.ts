import { afterEach, expect, test, vi } from "vitest";
import {
  ADMIN_TREND_ACTIONS,
  ADMIN_TREND_COST,
  ADMIN_TREND_CREDITS,
  ADMIN_TREND_FUNNEL,
  ADMIN_TREND_JOURNEYS,
  ADMIN_TREND_RUNS,
} from "../../../testing/fixtures/platform";
import { getTrend } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const TRENDS = {
  cost: ADMIN_TREND_COST,
  credits: ADMIN_TREND_CREDITS,
  runs: ADMIN_TREND_RUNS,
  "operator-actions": ADMIN_TREND_ACTIONS,
  funnel: ADMIN_TREND_FUNNEL,
  journeys: ADMIN_TREND_JOURNEYS,
};
type Path = keyof typeof TRENDS;
type Case = [string, Path, unknown];
const paths = Object.keys(TRENDS) as Path[];
const valid: Case[] = paths.flatMap((path) => [
  [`standard ${path}`, path, TRENDS[path]],
  [`empty ${path}`, path, { ...TRENDS[path], buckets: [] }],
]);
for (const amount of [-9007199254740991, 0, 9007199254740991]) {
  valid.push([
    `signed amount ${amount}`,
    "credits",
    {
      ...ADMIN_TREND_CREDITS,
      balance_total: amount,
      buckets: [{ day: "2026-09-06", key: "future_kind", count: amount, total: amount }],
      extra: true,
    },
  ]);
}
for (const median of [undefined, 0, 1.5]) {
  valid.push([
    `optional median ${median}`,
    "journeys",
    { ...ADMIN_TREND_JOURNEYS, period: { ...ADMIN_TREND_JOURNEYS.period, median_minutes: median } },
  ]);
}
valid.push([
  "leap day",
  "runs",
  { from: "2024-02-29", to: "2024-02-29", buckets: [{ day: "2024-02-29", key: "", count: 0 }] },
]);
valid.push(["empty stages", "funnel", { ...ADMIN_TREND_FUNNEL, stages: [] }]);

test.each(valid)("complete %s trend preserves its response", async (_name, path, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getTrend(path, 7)).resolves.toEqual(JSON.parse(JSON.stringify(body)));
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(`/admin/trends/${path}?days=7`);
});

const invalid: Case[] = [];
for (const path of paths) {
  for (const [name, body] of [
    ["null body", null],
    ["array body", []],
    ["missing buckets", { ...TRENDS[path], buckets: undefined }],
    ["object buckets", { ...TRENDS[path], buckets: {} }],
  ] as [string, unknown][])
    invalid.push([`${path} ${name}`, path, body]);
}
for (const field of ["from", "to"] as const) {
  for (const date of [undefined, null, 0, "yesterday", "2026-02-29", "2026-09-06T00:00:00Z"]) {
    invalid.push([`${field} ${date}`, "runs", { ...ADMIN_TREND_RUNS, [field]: date }]);
  }
}
const bucket = ADMIN_TREND_RUNS.buckets[0];
for (const [name, row] of [
  ["null bucket", null],
  ["array bucket", []],
  ["missing day", { ...bucket, day: undefined }],
  ["impossible day", { ...bucket, day: "2026-04-31" }],
  ["missing key", { ...bucket, key: undefined }],
  ["non-string key", { ...bucket, key: 1 }],
] as [string, unknown][]) {
  invalid.push([name, "runs", { ...ADMIN_TREND_RUNS, buckets: [bucket, row] }]);
}
const badIntegers = [undefined, null, "1", 0.5, -9007199254740992, 9007199254740992];
for (const number of badIntegers) {
  invalid.push([
    `count ${number}`,
    "runs",
    { ...ADMIN_TREND_RUNS, buckets: [{ ...bucket, count: number }] },
  ]);
  for (const path of ["cost", "credits"] as const) {
    invalid.push([
      `${path} total ${number}`,
      path,
      { ...TRENDS[path], buckets: [{ ...TRENDS[path].buckets[0], total: number }] },
    ]);
  }
  invalid.push([`balance ${number}`, "credits", { ...ADMIN_TREND_CREDITS, balance_total: number }]);
  for (const field of ["promise_minutes", "journey_window_hours"] as const) {
    invalid.push([`${field} ${number}`, "journeys", { ...ADMIN_TREND_JOURNEYS, [field]: number }]);
  }
  for (const field of ["searched", "downloaded", "within_promise"] as const) {
    invalid.push([
      `${field} ${number}`,
      "journeys",
      { ...ADMIN_TREND_JOURNEYS, period: { ...ADMIN_TREND_JOURNEYS.period, [field]: number } },
    ]);
  }
}
for (const stages of [undefined, null, {}, [null], [[]]]) {
  invalid.push([
    `unreadable stages ${JSON.stringify(stages)}`,
    "funnel",
    { ...ADMIN_TREND_FUNNEL, stages },
  ]);
}
for (const field of ["key", "label", "grain"] as const) {
  for (const value of [undefined, null, 1]) {
    invalid.push([
      `stage ${field} ${value}`,
      "funnel",
      { ...ADMIN_TREND_FUNNEL, stages: [{ ...ADMIN_TREND_FUNNEL.stages[0], [field]: value }] },
    ]);
  }
}
for (const period of [undefined, null, []]) {
  invalid.push([
    `unreadable period ${JSON.stringify(period)}`,
    "journeys",
    { ...ADMIN_TREND_JOURNEYS, period },
  ]);
}
for (const median of [null, "1", false]) {
  invalid.push([
    `unreadable median ${median}`,
    "journeys",
    { ...ADMIN_TREND_JOURNEYS, period: { ...ADMIN_TREND_JOURNEYS.period, median_minutes: median } },
  ]);
}

test.each(invalid)("trend with %s stays unreadable", async (_name, path, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getTrend(path, 7)).rejects.toThrow("Invalid trend response");
});
