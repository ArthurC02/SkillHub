import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_DISPATCH } from "../../../testing/fixtures/platform";
import { getDispatchStatus } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const HALT = ADMIN_DISPATCH.halts[0];

test.each<[string, unknown]>([
  ["incident and capacity halts", ADMIN_DISPATCH],
  ["dispatching without halts", { dispatching: true, halts: [] }],
  ["stopped without halts", { dispatching: false, halts: [] }],
  ["partial dispatch", { ...ADMIN_DISPATCH, dispatching: true }],
  [
    "absent optional progress",
    { ...ADMIN_DISPATCH, halts: [{ ...HALT, clear_rounds: undefined }] },
  ],
  ["zero progress", { ...ADMIN_DISPATCH, halts: [{ ...HALT, clear_rounds: 0 }] }],
  ["negative progress", { ...ADMIN_DISPATCH, halts: [{ ...HALT, clear_rounds: -1 }] }],
  [
    "safe integer progress",
    { ...ADMIN_DISPATCH, halts: [{ ...HALT, clear_rounds: 9007199254740991 }] },
  ],
  [
    "minimum safe integer progress",
    { ...ADMIN_DISPATCH, halts: [{ ...HALT, clear_rounds: -9007199254740991 }] },
  ],
  [
    "offset timestamp",
    { ...ADMIN_DISPATCH, halts: [{ ...HALT, declared_at: "2026-08-12T08:00:00+08:00" }] },
  ],
  ["empty required text", { ...ADMIN_DISPATCH, halts: [{ ...HALT, target: "", reason: "" }] }],
  ["extra metadata", { ...ADMIN_DISPATCH, extra: true, halts: [{ ...HALT, extra: true }] }],
])("complete %s dispatch status preserves the response", async (_name, body) => {
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getDispatchStatus()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("/admin/dispatch", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

const BAD_HALTS: [string, unknown][] = [
  ["null halt", null],
  ["primitive halt", true],
  ["array halt", []],
  ["missing target", { ...HALT, target: undefined }],
  ["non-string target", { ...HALT, target: 1 }],
  ["missing source", { ...HALT, source: undefined }],
  ["unknown source", { ...HALT, source: "other" }],
  ["missing reason", { ...HALT, reason: undefined }],
  ["non-string reason", { ...HALT, reason: {} }],
  ["missing declaration time", { ...HALT, declared_at: undefined }],
  ["numeric declaration time", { ...HALT, declared_at: 0 }],
  ["unreadable declaration time", { ...HALT, declared_at: "not-a-time" }],
  ["missing recovery mode", { ...HALT, automatic_recovery: undefined }],
  ["string recovery mode", { ...HALT, automatic_recovery: "false" }],
  ["null progress", { ...HALT, clear_rounds: null }],
  ["string progress", { ...HALT, clear_rounds: "0" }],
  ["fractional progress", { ...HALT, clear_rounds: 0.5 }],
  ["unsafe progress", { ...HALT, clear_rounds: 9007199254740992 }],
  ["too small progress", { ...HALT, clear_rounds: -9007199254740992 }],
];

test.each<[string, unknown]>([
  ["null body", null],
  ["primitive body", false],
  ["array body", []],
  ["missing dispatch decision", { halts: [] }],
  ["null dispatch decision", { dispatching: null, halts: [] }],
  ["string dispatch decision", { dispatching: "false", halts: [] }],
  ["numeric dispatch decision", { dispatching: 0, halts: [] }],
  ["missing halts", { dispatching: true }],
  ["null halts", { dispatching: true, halts: null }],
  ["object halts", { dispatching: true, halts: {} }],
  ...BAD_HALTS.map(([name, halt]): [string, unknown] => [
    name,
    { dispatching: true, halts: [halt] },
  ]),
  ["partly unreadable halts", { dispatching: true, halts: [HALT, { ...HALT, reason: null }] }],
])("dispatch status with %s remains unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getDispatchStatus()).rejects.toThrow("Invalid dispatch status response");
});
