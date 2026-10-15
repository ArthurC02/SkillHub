import { afterEach, expect, test, vi } from "vitest";
import { ME } from "../../testing/fixtures/platform";
import { getMe } from "./me.service";

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["pending deletion", ME],
  ["no deletion", { ...ME, deletion_requested_at: null, purge_after: null, deletion_scope: null }],
  ["operator", { ...ME, operator: true }],
  ["entry flags", { ...ME, features: { generate_skill: true, clean_mode: false, future: true } }],
  ["empty entry flags", { ...ME, features: {} }],
  [
    "uppercase UUIDs",
    { ...ME, user_id: ME.user_id.toUpperCase(), workspace_id: ME.workspace_id.toUpperCase() },
  ],
  ["timestamp offsets", { ...ME, deletion_requested_at: "2026-08-17T08:00:00.123+08:00" }],
])("the session read accepts a complete profile with %s", async (_name, profile) => {
  const request = vi.fn(async () => Response.json(profile));
  vi.stubGlobal("fetch", request);
  await expect(getMe()).resolves.toEqual(profile);
  expect(request).toHaveBeenCalledExactlyOnceWith("/me", {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
});

test.each<[string, unknown]>([
  ["null", null],
  ["array", []],
  ["primitive", 42],
  ["empty object", {}],
  ...[
    "user_id",
    "workspace_id",
    "email",
    "display_name",
    "operator",
    "deletion_scope",
    "deletion_requested_at",
    "purge_after",
  ].map<[string, unknown]>((field) => ["missing " + field, { ...ME, [field]: undefined }]),
  ["invalid user UUID", { ...ME, user_id: "u-1" }],
  ["invalid Workspace UUID", { ...ME, workspace_id: "ws-1" }],
  ["non-string identity", { ...ME, user_id: 3 }],
  ["non-string email", { ...ME, email: null }],
  ["non-string display name", { ...ME, display_name: {} }],
  ["string operator flag", { ...ME, operator: "false" }],
  ["numeric operator flag", { ...ME, operator: 1 }],
  ["non-string deletion scope", { ...ME, deletion_scope: [] }],
  ["unreadable deletion timestamp", { ...ME, deletion_requested_at: "not-a-date" }],
  ["unreadable purge timestamp", { ...ME, purge_after: "not-a-date" }],
  ["non-string timestamp", { ...ME, purge_after: 42 }],
  ["null entry flags", { ...ME, features: null }],
  ["array entry flags", { ...ME, features: [] }],
  ["scalar entry flags", { ...ME, features: true }],
  ["non-boolean entry flag", { ...ME, features: { generate_skill: "false" } }],
])("the session read rejects a profile with %s", async (_name, profile) => {
  const request = vi.fn(async () => Response.json(profile));
  vi.stubGlobal("fetch", request);
  await expect(getMe()).rejects.toThrow("Invalid session response");
  expect(request).toHaveBeenCalledOnce();
});

test("a session reply without a body cannot confirm identity", async () => {
  const request = vi.fn(async () => new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", request);
  await expect(getMe()).rejects.toThrow("Invalid session response");
  expect(request).toHaveBeenCalledOnce();
});
