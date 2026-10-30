import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_SETTINGS, ADMIN_RUN_SETTINGS } from "../../../testing/fixtures/platform";
import { getPlatformSettings, getRunSettings } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const fields = {
  continuation_rounds: ["rounds", "min", "max"],
  model_budget: ["credits", "default_credits", "min_credits", "max_credits"],
  tokens_per_minute: ["limit", "default_limit", "min", "max"],
};
type Group = keyof typeof fields;
type Case = [string, "platform" | "runs", unknown];
const valid: Case[] = [
  ["standard platform", "platform", ADMIN_SETTINGS],
  ["enabled panel", "platform", { judge_panel: { enabled: true } }],
  ["standard runs", "runs", ADMIN_RUN_SETTINGS],
];
const metadata = {
  reason: "operator choice",
  set_by_user_id: "11111111-2222-3333-4444-555555555555",
  set_at: "2026-09-06T08:00:00+08:00",
};
valid.push(["panel metadata", "platform", { judge_panel: { enabled: false, ...metadata } }]);
for (const value of [-9007199254740991, 0, 9007199254740991]) {
  const body = structuredClone(ADMIN_RUN_SETTINGS);
  for (const group of Object.keys(fields) as Group[]) {
    Object.assign(
      body[group],
      metadata,
      Object.fromEntries(fields[group].map((key) => [key, value])),
    );
  }
  valid.push([`integer boundary ${value}`, "runs", body]);
}

test.each(valid)("complete %s settings preserve their response", async (_name, kind, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(kind === "platform" ? getPlatformSettings() : getRunSettings()).resolves.toEqual(
    body,
  );
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(
    kind === "platform" ? "/admin/settings" : "/admin/settings/runs",
  );
});

const invalid: Case[] = [];
for (const kind of ["platform", "runs"] as const) {
  for (const body of [null, [], true, {}])
    invalid.push([`${kind} body ${JSON.stringify(body)}`, kind, body]);
}
for (const panel of [null, [], {}, { enabled: null }, { enabled: "false" }, { enabled: 0 }]) {
  invalid.push([`panel ${JSON.stringify(panel)}`, "platform", { judge_panel: panel }]);
}
for (const group of Object.keys(fields) as Group[]) {
  for (const value of [undefined, null, []]) {
    invalid.push([
      `${group} ${JSON.stringify(value)}`,
      "runs",
      { ...ADMIN_RUN_SETTINGS, [group]: value },
    ]);
  }
  for (const field of fields[group]) {
    for (const value of [undefined, null, "1", 0.5, -9007199254740992, 9007199254740992]) {
      invalid.push([
        `${group}.${field} ${value}`,
        "runs",
        { ...ADMIN_RUN_SETTINGS, [group]: { ...ADMIN_RUN_SETTINGS[group], [field]: value } },
      ]);
    }
  }
}
for (const [field, value] of [
  ["reason", {}],
  ["reason", null],
  ["set_at", 0],
  ["set_at", "not-a-time"],
  ["set_by_user_id", true],
  ["set_by_user_id", "not-an-id"],
] as [string, unknown][]) {
  invalid.push([
    `panel ${field} ${JSON.stringify(value)}`,
    "platform",
    { judge_panel: { enabled: false, [field]: value } },
  ]);
  for (const group of Object.keys(fields) as Group[]) {
    invalid.push([
      `${group} ${field} ${JSON.stringify(value)}`,
      "runs",
      { ...ADMIN_RUN_SETTINGS, [group]: { ...ADMIN_RUN_SETTINGS[group], [field]: value } },
    ]);
  }
}

test.each(invalid)("settings with %s stay unreadable", async (_name, kind, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(kind === "platform" ? getPlatformSettings() : getRunSettings()).rejects.toThrow();
});
