import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_LEDGER, ME } from "../../../testing/fixtures/platform";
import { getCreditLedger } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["standard", ADMIN_LEDGER],
  ["zero and empty history", { ...ADMIN_LEDGER, balance_credits: 0, entries: [] }],
  ["negative balance", { ...ADMIN_LEDGER, balance_credits: -1 }],
  ["maximum safe balance", { ...ADMIN_LEDGER, balance_credits: 9007199254740991 }],
  ["minimum safe balance", { ...ADMIN_LEDGER, balance_credits: -9007199254740991 }],
  [
    "equivalent UUID spelling",
    { ...ADMIN_LEDGER, workspace_id: ADMIN_LEDGER.workspace_id.toUpperCase() },
  ],
  [
    "safe delta boundaries",
    {
      ...ADMIN_LEDGER,
      entries: [
        { ...ADMIN_LEDGER.entries[0], delta_credits: 9007199254740991 },
        { ...ADMIN_LEDGER.entries[1], delta_credits: -9007199254740991 },
      ],
    },
  ],
  [
    "all entry kinds",
    {
      ...ADMIN_LEDGER,
      entries: ["debit", "grant", "topup", "adjustment"].map((kind) => ({
        kind,
        delta_credits: 0,
        ref_type: null,
        estimated: false,
        created_at: "2026-10-15T20:00:00Z",
      })),
    },
  ],
  [
    "empty source and extra metadata",
    {
      ...ADMIN_LEDGER,
      extra: true,
      entries: [{ ...ADMIN_LEDGER.entries[1], ref_type: "", extra: { value: true } }],
    },
  ],
])("a complete %s ledger preserves the exact response", async (_name, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getCreditLedger(ADMIN_LEDGER.workspace_id)).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(`/admin/credits/${ADMIN_LEDGER.workspace_id}`);
});

const ENTRY = ADMIN_LEDGER.entries[0];
const BAD_ENTRIES: [string, unknown][] = [
  ["null entry", null],
  ["primitive entry", 1],
  ["missing kind", { ...ENTRY, kind: undefined }],
  ["unknown kind", { ...ENTRY, kind: "unreported" }],
  ["missing delta", { ...ENTRY, delta_credits: undefined }],
  ["string delta", { ...ENTRY, delta_credits: "-30" }],
  ["fractional delta", { ...ENTRY, delta_credits: 1.5 }],
  ["too large delta", { ...ENTRY, delta_credits: 9007199254740992 }],
  ["too small delta", { ...ENTRY, delta_credits: -9007199254740992 }],
  ["missing source", { ...ENTRY, ref_type: undefined }],
  ["object source", { ...ENTRY, ref_type: {} }],
  ["missing estimate verdict", { ...ENTRY, estimated: undefined }],
  ["string estimate verdict", { ...ENTRY, estimated: "false" }],
  ["numeric estimate verdict", { ...ENTRY, estimated: 0 }],
  ["missing date", { ...ENTRY, created_at: undefined }],
  ["non-string date", { ...ENTRY, created_at: 0 }],
  ["unreadable date", { ...ENTRY, created_at: "not-a-date" }],
];

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["empty body", {}],
  ["missing workspace", { ...ADMIN_LEDGER, workspace_id: undefined }],
  ["non-string workspace", { ...ADMIN_LEDGER, workspace_id: {} }],
  ["foreign workspace", { ...ADMIN_LEDGER, workspace_id: ME.workspace_id }],
  ["missing balance", { ...ADMIN_LEDGER, balance_credits: undefined }],
  ["null balance", { ...ADMIN_LEDGER, balance_credits: null }],
  ["string balance", { ...ADMIN_LEDGER, balance_credits: "120" }],
  ["fractional balance", { ...ADMIN_LEDGER, balance_credits: 1.5 }],
  ["too large balance", { ...ADMIN_LEDGER, balance_credits: 9007199254740992 }],
  ["too small balance", { ...ADMIN_LEDGER, balance_credits: -9007199254740992 }],
  ["missing entries", { ...ADMIN_LEDGER, entries: undefined }],
  ["null entries", { ...ADMIN_LEDGER, entries: null }],
  ["object entries", { ...ADMIN_LEDGER, entries: {} }],
  ...BAD_ENTRIES.map(([name, entry]): [string, unknown] => [
    name,
    { ...ADMIN_LEDGER, entries: [entry] },
  ]),
  [
    "partly unreadable history",
    { ...ADMIN_LEDGER, entries: [ENTRY, { ...ENTRY, kind: "unreported" }] },
  ],
])(
  "a ledger with %s stays unreadable instead of enabling a financial decision",
  async (_name, body) => {
    vi.stubGlobal("fetch", () =>
      Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    );
    await expect(getCreditLedger(ADMIN_LEDGER.workspace_id)).rejects.toThrow(
      "Invalid credit ledger response",
    );
  },
);
