import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_ACCOUNT } from "../../../testing/fixtures/platform";
import { lookupAccount } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["active account", ADMIN_ACCOUNT],
  ["denied admission", { ...ADMIN_ACCOUNT, in_beta_allowlist: false }],
  ["deletion requested", { ...ADMIN_ACCOUNT, deletion_requested_at: "2026-10-15T09:00:00+08:00" }],
  ["empty profile strings", { ...ADMIN_ACCOUNT, email: "", display_name: "" }],
  [
    "UUID spelling",
    {
      ...ADMIN_ACCOUNT,
      user_id: "00000000-0000-0000-0000-000000000000",
      workspace_id: "ABCDEFAB-CDEF-ABCD-EFAB-CDEFABCDEFAB",
    },
  ],
  ["additional fields", { ...ADMIN_ACCOUNT, future_property: { value: true } }],
])(
  "a complete %s lookup preserves the response and the encoded requested email",
  async (_name, body) => {
    const fetch = vi.fn((_input: RequestInfo | URL) =>
      Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    );
    vi.stubGlobal("fetch", fetch);
    await expect(lookupAccount("Member+Ops@Example.com")).resolves.toEqual(body);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe("/admin/accounts?email=Member%2BOps%40Example.com");
  },
);

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["empty body", {}],
  ["missing user id", { ...ADMIN_ACCOUNT, user_id: undefined }],
  ["non-string user id", { ...ADMIN_ACCOUNT, user_id: 5 }],
  ["invalid user id", { ...ADMIN_ACCOUNT, user_id: "not-a-uuid" }],
  ["missing workspace id", { ...ADMIN_ACCOUNT, workspace_id: undefined }],
  ["non-string workspace id", { ...ADMIN_ACCOUNT, workspace_id: {} }],
  ["invalid workspace id", { ...ADMIN_ACCOUNT, workspace_id: "../unverified-workspace" }],
  ["missing email", { ...ADMIN_ACCOUNT, email: undefined }],
  ["non-string email", { ...ADMIN_ACCOUNT, email: {} }],
  ["missing display name", { ...ADMIN_ACCOUNT, display_name: undefined }],
  ["non-string display name", { ...ADMIN_ACCOUNT, display_name: {} }],
  ["missing beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: undefined }],
  ["null beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: null }],
  ["string beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: "false" }],
  ["numeric beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: 0 }],
  ["missing creation date", { ...ADMIN_ACCOUNT, created_at: undefined }],
  ["null creation date", { ...ADMIN_ACCOUNT, created_at: null }],
  ["non-string creation date", { ...ADMIN_ACCOUNT, created_at: 0 }],
  ["unreadable creation date", { ...ADMIN_ACCOUNT, created_at: "not-a-date" }],
  ["missing deletion date", { ...ADMIN_ACCOUNT, deletion_requested_at: undefined }],
  ["non-string deletion date", { ...ADMIN_ACCOUNT, deletion_requested_at: false }],
  ["unreadable deletion date", { ...ADMIN_ACCOUNT, deletion_requested_at: "not-a-date" }],
])(
  "a lookup with %s remains an unreadable response instead of an account verdict",
  async (_name, body) => {
    vi.stubGlobal("fetch", () =>
      Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    );
    await expect(lookupAccount("member@example.com")).rejects.toThrow(
      "Invalid account lookup response",
    );
  },
);
