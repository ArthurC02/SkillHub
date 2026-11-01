import { expect, test } from "vitest";
import { ADMIN_MCP_TOOLS } from "../../testing/fixtures/platform";
import {
  container,
  calls,
  stub,
  mountAt,
  waitFor,
  has,
  type,
  button,
  click,
} from "./testing/harness";

test("SKILL-007: the MCP tool list names each tool by its reference and keeps retired ones as history", async () => {
  stub(true);
  await mountAt("/admin/settings");
  await waitFor(has("mcp__calendar__create_event"));
  expect(has("Creates an event on the team calendar.")()).toBe(true);
  expect(has("參數：title, start, end")()).toBe(true);
  expect(has("位址：https://mcp.example.com/calendar")()).toBe(true);
  expect(has("加入理由：sales asked for meeting follow-ups")()).toBe(true);
  expect(has("參數：無")()).toBe(true);
  expect(has("已退役")()).toBe(true);
  expect(has("退役理由：the pilot ended")()).toBe(true);
  const retireControls = Array.from(container.querySelectorAll("summary")).map(
    (s) => s.textContent,
  );
  expect(retireControls).toContain("退役 mcp__calendar__create_event");
  expect(retireControls).not.toContain("退役 mcp__mail__send");
});

test("SKILL-007: an empty MCP tool list says creation cannot declare any tool", async () => {
  stub(true, (path) =>
    path === "/admin/mcp-tools" ? { body: { tools: [] }, status: 200 } : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("清單是空的：創作出的 Skill 目前不能宣告任何 MCP 工具。"));
});

test.each([
  ["a 503", { body: { error: "unavailable" }, status: 503 }],
  ["a tool without a reference", { body: { tools: [{ id: "x" }] }, status: 200 }],
  [
    "a retired tool without its reason",
    {
      body: { tools: [{ ...ADMIN_MCP_TOOLS.tools[1], retire_reason: undefined }] },
      status: 200,
    },
  ],
  ["no list", { body: {}, status: 200 }],
])("SKILL-007: %s is an unreadable MCP tool list, not an empty one", async (_name, reply) => {
  stub(true, (path) => (path === "/admin/mcp-tools" ? reply : undefined));
  await mountAt("/admin/settings");
  await waitFor(has("暫時無法讀取 MCP 工具清單，請再試一次。"));
  expect(has("清單是空的")()).toBe(false);
  expect(has("目前：")()).toBe(true);
});

test("SKILL-007: adding an MCP tool waits for every field in its form and sends exactly what was typed", async () => {
  let reads = 0;
  stub(true, (path, method) => {
    if (path === "/admin/mcp-tools" && method === "GET") {
      reads += 1;
      return undefined;
    }
    return path === "/admin/mcp-tools" && method === "POST"
      ? {
          body: { ...ADMIN_MCP_TOOLS.tools[0], id: "7a1c0000-0000-4000-8000-000000000003" },
          status: 201,
        }
      : undefined;
  });
  await mountAt("/admin/settings");
  await waitFor(has("新增工具"));
  const fill = {
    server: "crm",
    tool: "find_contact",
    purpose: "Looks up a contact.",
    parameters: "",
    server_url: "https://mcp.example.com/crm",
  };
  for (const [key, value] of Object.entries(fill)) await type(`#admin-mcp-tools-${key}`, value);
  await type("#admin-mcp-tools-add-note", "sales follow-ups");
  expect(button("加入清單").disabled).toBe(false);

  for (const [key, bad] of [
    ["server", "CRM"],
    ["server", "my--crm"],
    ["tool", "find__contact"],
    ["purpose", "  "],
    ["server_url", "http://mcp.example.com/crm"],
    ["server", "a".repeat(65)],
  ] as const) {
    await type(`#admin-mcp-tools-${key}`, bad);
    expect(button("加入清單").disabled, `${key}=${bad}`).toBe(true);
    await type(`#admin-mcp-tools-${key}`, fill[key]);
  }
  await type("#admin-mcp-tools-server", "a".repeat(64));
  expect(button("加入清單").disabled).toBe(false);
  await type("#admin-mcp-tools-server", "crm");

  const before = reads;
  await click(button("加入清單"));
  await waitFor(has("已加入清單，下一次創作就能選用。"));
  expect(calls.find((c) => c.method === "POST" && c.url === "/admin/mcp-tools")?.body).toEqual({
    ...fill,
    note: "sales follow-ups",
  });
  await waitFor(() => reads > before);
});

test("SKILL-007: retiring an MCP tool posts the reason to that tool", async () => {
  stub(true, (path, method) =>
    path === "/admin/mcp-tools/7a1c0000-0000-4000-8000-000000000001/retire" && method === "POST"
      ? {
          body: {
            ...ADMIN_MCP_TOOLS.tools[0],
            retired_at: "2026-11-02T08:00:00Z",
            retire_reason: "replaced",
          },
          status: 200,
        }
      : undefined,
  );
  await mountAt("/admin/settings");
  await waitFor(has("退役 mcp__calendar__create_event"));
  const id = "admin-mcp-tools-retire-7a1c0000-0000-4000-8000-000000000001";
  await type(`#${id}-note`, "replaced");
  await click(button("退役這個工具"));
  await waitFor(() => calls.some((c) => c.method === "POST" && c.url.endsWith("/retire")));
  expect(calls.find((c) => c.url.endsWith("/retire"))).toEqual({
    method: "POST",
    url: "/admin/mcp-tools/7a1c0000-0000-4000-8000-000000000001/retire",
    body: { note: "replaced" },
  });
});
