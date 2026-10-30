import { afterEach, expect, test, vi } from "vitest";
import { ADMIN_AGENTS } from "../../../testing/fixtures/platform";
import { getPlatformAgents } from "../admin.service";

afterEach(() => vi.unstubAllGlobals());

const agent = ADMIN_AGENTS.agents[0];
const brake = { reason: "review", engaged_at: "2026-10-07T00:00:00Z" };
const actor = "11111111-2222-3333-4444-555555555555";
type Case = [string, unknown];
const valid: Case[] = [
  ["standard", ADMIN_AGENTS],
  ["empty list", { agents: [] }],
  ["disabled Agent", { agents: [{ ...agent, enabled: false }] }],
  ["brake", { agents: [agent], brake }],
  [
    "recorded actors",
    {
      agents: [{ ...agent, owner_user_id: actor }],
      brake: { ...brake, engaged_by_user_id: actor },
    },
  ],
];
for (const value of [-9007199254740991, 0, 9007199254740991]) {
  valid.push([
    `integer boundary ${value}`,
    {
      agents: [
        { ...agent, daily_spend_cap_usd_micros: value, default_daily_spend_cap_usd_micros: value },
      ],
    },
  ]);
}

test.each(valid)("complete %s Agent controls preserve their response", async (_name, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getPlatformAgents()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/admin/agents");
});

const invalid: Case[] = [
  ...[null, [], true, {}].map((body): Case => [`body ${JSON.stringify(body)}`, body]),
  ...[null, {}, true, "agents"].map((agents): Case => [
    `list ${JSON.stringify(agents)}`,
    { agents },
  ]),
  ...[null, [], {}].map((entry): Case => [`entry ${JSON.stringify(entry)}`, { agents: [entry] }]),
];
for (const field of ["name", "purpose", "model_role"]) {
  for (const value of [undefined, null, 1])
    invalid.push([`${field} ${value}`, { agents: [{ ...agent, [field]: value }] }]);
}
for (const field of ["enabled", "daily_spend_cap_overridden"]) {
  for (const value of [undefined, null, "false", 0])
    invalid.push([`${field} ${value}`, { agents: [{ ...agent, [field]: value }] }]);
}
for (const field of ["daily_spend_cap_usd_micros", "default_daily_spend_cap_usd_micros"]) {
  for (const value of [undefined, null, "1", 0.5, -9007199254740992, 9007199254740992])
    invalid.push([`${field} ${value}`, { agents: [{ ...agent, [field]: value }] }]);
}
for (const field of ["tools", "actions"]) {
  for (const value of [undefined, null, {}, [null], [1]])
    invalid.push([`${field} ${JSON.stringify(value)}`, { agents: [{ ...agent, [field]: value }] }]);
}
for (const value of [null, 0, "invalid-id"]) {
  invalid.push([`owner ${value}`, { agents: [{ ...agent, owner_user_id: value }] }]);
  invalid.push([
    `brake actor ${value}`,
    { agents: [agent], brake: { ...brake, engaged_by_user_id: value } },
  ]);
}
for (const value of [
  null,
  [],
  {},
  { reason: "review" },
  { ...brake, reason: null },
  { ...brake, engaged_at: 0 },
  { ...brake, engaged_at: "not-a-time" },
]) {
  invalid.push([`brake ${JSON.stringify(value)}`, { agents: [agent], brake: value }]);
}

test.each(invalid)("Agent controls with %s stay unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () => Promise.resolve(new Response(JSON.stringify(body))));
  await expect(getPlatformAgents()).rejects.toThrow("Invalid Agent controls response");
});
