import { afterEach, expect, test, vi } from "vitest";
import type { ImportResult } from "../../core/api/types";
import { SKILL, VERSION } from "../../testing/fixtures/platform";
import { importSkillFromURL, isImportResult, uploadSkillPackage } from "./import.service";

const FINDING = {
  severity: "error" as const,
  code: "skill-md-missing",
  message: "套件根目錄找不到 SKILL.md",
};
const RECEIPT: ImportResult = {
  shape: "plugin",
  plugin: { name: "desk-tools", version: "1.0", repository: "https://github.com/example/skill" },
  skills: [
    {
      path: "skills/tidy",
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 1,
      content_hash: "sha256:skill",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};
const SKILL_RECEIPT = RECEIPT.skills[0];

afterEach(() => vi.unstubAllGlobals());

for (const [source, invoke] of [
  ["URL", () => importSkillFromURL("https://github.com/example/skill")],
  ["zip", () => uploadSkillPackage(new File(["zip bytes"], "skill.zip"))],
] as const) {
  test.each(["skill", "plugin", "tree"] as const)(
    `${source} import accepts a complete %s result`,
    async (shape) => {
      const receipt = { ...RECEIPT, shape };
      if (shape !== "plugin") delete receipt.plugin;
      if (shape === "skill") receipt.skills = [{ ...SKILL_RECEIPT, path: "" }];
      const fetch = vi.fn(() => Promise.resolve(Response.json(receipt, { status: 201 })));
      vi.stubGlobal("fetch", fetch);
      await expect(invoke()).resolves.toEqual(receipt);
      expect(fetch).toHaveBeenCalledOnce();
      expect(fetch).toHaveBeenCalledWith(
        source === "URL" ? "/skills/import/url" : "/skills/import/upload",
        expect.objectContaining({ method: "POST" }),
      );
    },
  );

  test.each([
    ["null", null],
    ["array", []],
    ["missing fields", {}],
    ["missing shape", { ...RECEIPT, shape: undefined }],
    ["unknown shape", { ...RECEIPT, shape: "folder" }],
    ["missing skills", { ...RECEIPT, skills: undefined }],
    ["no imported skills", { ...RECEIPT, skills: [] }],
    ["null skill", { ...RECEIPT, skills: [null] }],
    ["incomplete skill", { ...RECEIPT, skills: [{}] }],
    ["missing skill path", { ...RECEIPT, skills: [{ ...SKILL_RECEIPT, path: undefined }] }],
    ["non-string skill path", { ...RECEIPT, skills: [{ ...SKILL_RECEIPT, path: 3 }] }],
    ["invalid skill identity", { ...RECEIPT, skills: [{ ...SKILL_RECEIPT, skill_id: "invalid" }] }],
    [
      "invalid version identity",
      { ...RECEIPT, skills: [{ ...SKILL_RECEIPT, version_id: "invalid" }] },
    ],
    ["invalid version number", { ...RECEIPT, skills: [{ ...SKILL_RECEIPT, version_number: 0 }] }],
    ["missing refusal list", { ...RECEIPT, refused: undefined }],
    ["non-array refusal list", { ...RECEIPT, refused: {} }],
    ["null refusal", { ...RECEIPT, refused: [null] }],
    [
      "missing refusal path",
      { ...RECEIPT, refused: [{ findings: { errors: [FINDING], warnings: [], infos: [] } }] },
    ],
    ["missing refusal findings", { ...RECEIPT, refused: [{ path: "broken" }] }],
    [
      "malformed refusal finding",
      {
        ...RECEIPT,
        refused: [{ path: "broken", findings: { errors: [null], warnings: [], infos: [] } }],
      },
    ],
    ["missing excluded components", { ...RECEIPT, excluded_components: undefined }],
    ["non-array excluded components", { ...RECEIPT, excluded_components: {} }],
    ["null excluded component", { ...RECEIPT, excluded_components: [null] }],
    [
      "invalid component details",
      { ...RECEIPT, excluded_components: [{ ...FINDING, details: [null] }] },
    ],
    ["null plugin", { ...RECEIPT, plugin: null }],
    ["non-object plugin", { ...RECEIPT, plugin: "desk-tools" }],
    ["missing plugin name", { ...RECEIPT, plugin: {} }],
    ["non-string plugin name", { ...RECEIPT, plugin: { name: 3 } }],
    ["non-string plugin version", { ...RECEIPT, plugin: { name: "desk-tools", version: 3 } }],
    [
      "non-string plugin repository",
      { ...RECEIPT, plugin: { name: "desk-tools", repository: {} } },
    ],
  ])(`${source} import does not confirm a reply with %s`, async (_name, receipt) => {
    const fetch = vi.fn(() => Promise.resolve(Response.json(receipt, { status: 201 })));
    vi.stubGlobal("fetch", fetch);
    await expect(invoke()).rejects.toThrow("Invalid import response");
    expect(fetch).toHaveBeenCalledOnce();
  });
}

test.each([
  [
    "a refused candidate",
    [{ path: "broken", findings: { errors: [FINDING], warnings: [], infos: [] } }],
  ],
  ["no candidate", []],
])("a complete refusal can report %s without inventing imported skills", (_name, refused) => {
  expect(isImportResult({ ...RECEIPT, skills: [], refused })).toBe(true);
});

test("a complete import preserves optional finding details and plugin metadata", async () => {
  const receipt = {
    ...RECEIPT,
    excluded_components: [
      { ...FINDING, severity: "info", path: "mcp.json", details: ["MCP server 宣告"] },
    ],
    refused: [{ path: "broken", findings: { errors: [FINDING], warnings: [], infos: [] } }],
  };
  vi.stubGlobal("fetch", () => Promise.resolve(Response.json(receipt, { status: 201 })));
  await expect(importSkillFromURL("https://github.com/example/skill")).resolves.toEqual(receipt);
});
