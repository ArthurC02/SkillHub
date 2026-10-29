import { afterEach, expect, test, vi } from "vitest";
import { getRosters } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

test.each<[string, unknown]>([
  ["empty deployment lists", { operator_user_ids: [], beta_allowlist: [] }],
  [
    "operator UUIDs and external identities",
    {
      operator_user_ids: ["11111111-1111-1111-1111-111111111111"],
      beta_allowlist: ["github|invitee", "邀請者"],
    },
  ],
  [
    "uppercase and nil UUIDs",
    {
      operator_user_ids: [
        "ABCDEFAB-CDEF-ABCD-EFAB-CDEFABCDEFAB",
        "00000000-0000-0000-0000-000000000000",
      ],
      beta_allowlist: [],
    },
  ],
  [
    "empty identity string permitted by the schema",
    { operator_user_ids: [], beta_allowlist: [""] },
  ],
  ["extra metadata", { operator_user_ids: [], beta_allowlist: [], extra: true }],
])("rosters with %s preserve the complete response", async (_name, body) => {
  const fetch = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal("fetch", fetch);
  await expect(getRosters()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("/admin/rosters", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

test.each<[string, unknown]>([
  ["null body", null],
  ["primitive body", false],
  ["array body", []],
  ["missing operators", { beta_allowlist: [] }],
  ["null operators", { operator_user_ids: null, beta_allowlist: [] }],
  ["object operators", { operator_user_ids: {}, beta_allowlist: [] }],
  ["string operators", { operator_user_ids: "", beta_allowlist: [] }],
  ["numeric operator", { operator_user_ids: [1], beta_allowlist: [] }],
  ["null operator", { operator_user_ids: [null], beta_allowlist: [] }],
  ["invalid operator UUID", { operator_user_ids: ["u-1"], beta_allowlist: [] }],
  ["blank operator UUID", { operator_user_ids: [""], beta_allowlist: [] }],
  [
    "padded operator UUID",
    { operator_user_ids: [" 11111111-1111-1111-1111-111111111111"], beta_allowlist: [] },
  ],
  [
    "partly unreadable operators",
    { operator_user_ids: ["11111111-1111-1111-1111-111111111111", "u-1"], beta_allowlist: [] },
  ],
  ["missing beta list", { operator_user_ids: [] }],
  ["null beta list", { operator_user_ids: [], beta_allowlist: null }],
  ["object beta list", { operator_user_ids: [], beta_allowlist: {} }],
  ["string beta list", { operator_user_ids: [], beta_allowlist: "" }],
  ["numeric beta identity", { operator_user_ids: [], beta_allowlist: [1] }],
  ["null beta identity", { operator_user_ids: [], beta_allowlist: [null] }],
  ["object beta identity", { operator_user_ids: [], beta_allowlist: [{}] }],
  ["partly unreadable beta list", { operator_user_ids: [], beta_allowlist: ["github|invitee", 1] }],
])("rosters with %s remain unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getRosters()).rejects.toThrow("Invalid rosters response");
});
