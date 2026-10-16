import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_COST_STATISTICS } from "../../../testing/fixtures/platform";
import { getCostStatistics } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const WINDOW = ADMIN_COST_STATISTICS.statistics[0];
const AMOUNTS = ["p50_usd_micros", "p90_usd_micros", "p95_usd_micros", "max_usd_micros"] as const;

test.each([
  ["standard", ADMIN_COST_STATISTICS],
  ["empty", { statistics: [] }],
  ["unknown kind", { statistics: [{ ...WINDOW, kind: "future_kind" }] }],
  ["empty kind", { statistics: [{ ...WINDOW, kind: "" }] }],
  [
    "zero measurements",
    {
      statistics: [
        { ...WINDOW, p50_usd_micros: 0, p90_usd_micros: 0, p95_usd_micros: 0, max_usd_micros: 0 },
      ],
    },
  ],
  [
    "maximum safe integers",
    {
      statistics: [
        {
          ...WINDOW,
          sample_count: 9007199254740991,
          p50_usd_micros: 9007199254740991,
          p90_usd_micros: 9007199254740991,
          p95_usd_micros: 9007199254740991,
          max_usd_micros: 9007199254740991,
        },
      ],
    },
  ],
  [
    "minimum safe integers",
    {
      statistics: [
        {
          ...WINDOW,
          sample_count: -9007199254740991,
          p50_usd_micros: -9007199254740991,
          p90_usd_micros: -9007199254740991,
          p95_usd_micros: -9007199254740991,
          max_usd_micros: -9007199254740991,
        },
      ],
    },
  ],
  [
    "time-zone offsets",
    {
      statistics: [
        {
          ...WINDOW,
          window_start: "2026-08-12T08:00:00+08:00",
          window_end: "2026-09-11T08:00:00+08:00",
        },
      ],
    },
  ],
  ["extra metadata", { statistics: [{ ...WINDOW, extra: true }], extra: { source: "operator" } }],
])("complete %s cost statistics preserve the exact response", async (_name, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getCostStatistics()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/admin/cost-statistics");
});

const BAD_WINDOWS: [string, unknown][] = [
  ["null window", null],
  ["primitive window", 1],
  ["array window", []],
  ["missing kind", { ...WINDOW, kind: undefined }],
  ["non-string kind", { ...WINDOW, kind: {} }],
  ["missing start", { ...WINDOW, window_start: undefined }],
  ["non-string start", { ...WINDOW, window_start: 0 }],
  ["unreadable start", { ...WINDOW, window_start: "not-a-time" }],
  ["missing end", { ...WINDOW, window_end: undefined }],
  ["non-string end", { ...WINDOW, window_end: false }],
  ["unreadable end", { ...WINDOW, window_end: "not-a-time" }],
  ["missing sample count", { ...WINDOW, sample_count: undefined }],
  ["null sample count", { ...WINDOW, sample_count: null }],
  ["string sample count", { ...WINDOW, sample_count: "0" }],
  ["fractional sample count", { ...WINDOW, sample_count: 0.5 }],
  ["too large sample count", { ...WINDOW, sample_count: 9007199254740992 }],
  ["too small sample count", { ...WINDOW, sample_count: -9007199254740992 }],
  ...AMOUNTS.flatMap((field): [string, unknown][] => [
    [`missing ${field}`, { ...WINDOW, [field]: undefined }],
    [`string ${field}`, { ...WINDOW, [field]: "1" }],
    [`fractional ${field}`, { ...WINDOW, [field]: 0.5 }],
    [`too large ${field}`, { ...WINDOW, [field]: 9007199254740992 }],
    [`too small ${field}`, { ...WINDOW, [field]: -9007199254740992 }],
  ]),
];

test.each<[string, unknown]>([
  ["null body", null],
  ["primitive body", true],
  ["array body", []],
  ["missing list", {}],
  ["null list", { statistics: null }],
  ["object list", { statistics: {} }],
  ...BAD_WINDOWS.map(([name, window]): [string, unknown] => [name, { statistics: [window] }]),
  ["partly unreadable list", { statistics: [WINDOW, { ...WINDOW, sample_count: "1" }] }],
])("cost statistics with %s remain unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getCostStatistics()).rejects.toThrow();
});
