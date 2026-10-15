import { afterEach, expect, test, vi } from "vitest";
import { forkSkill } from "./skills.service";

const SOURCE = "11111111-1111-1111-1111-111111111111";
const RECEIPT = {
  skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
  version_id: "abcdef02-2345-4789-abcd-0123456789ab",
  version_number: 1,
  name: "PDF Summariser 副本",
  summary: "",
  redistribution: "self_supplied",
};

afterEach(() => vi.unstubAllGlobals());

for (const version of [1, Number.MAX_SAFE_INTEGER]) {
  test(`fork storage accepts version ${version} and canonicalizes receipt identifiers`, async () => {
    const result = {
      ...RECEIPT,
      version_number: version,
      skill_id: RECEIPT.skill_id.toUpperCase(),
      version_id: RECEIPT.version_id.toUpperCase(),
    };
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(result), { status: 201 }));
    vi.stubGlobal("fetch", fetch);
    await expect(forkSkill(SOURCE)).resolves.toEqual({ ...RECEIPT, version_number: version });
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining(`/skills/${SOURCE}/fork`),
      expect.objectContaining({ method: "POST" }),
    );
  });
}

for (const [name, result] of [
  ["null", null],
  ["array", []],
  ["text", "created"],
  ["missing skill identity", { ...RECEIPT, skill_id: undefined }],
  ["malformed skill identity", { ...RECEIPT, skill_id: "skill" }],
  ["missing version identity", { ...RECEIPT, version_id: undefined }],
  ["malformed version identity", { ...RECEIPT, version_id: "version" }],
  ["missing version number", { ...RECEIPT, version_number: undefined }],
  ["text version number", { ...RECEIPT, version_number: "1" }],
  ["zero version", { ...RECEIPT, version_number: 0 }],
  ["negative version", { ...RECEIPT, version_number: -1 }],
  ["fractional version", { ...RECEIPT, version_number: 1.5 }],
  ["unsafe version", { ...RECEIPT, version_number: Number.MAX_SAFE_INTEGER + 1 }],
  ["missing name", { ...RECEIPT, name: undefined }],
  ["numeric name", { ...RECEIPT, name: 1 }],
  ["missing summary", { ...RECEIPT, summary: undefined }],
  ["missing redistribution", { ...RECEIPT, redistribution: undefined }],
  ["unknown redistribution", { ...RECEIPT, redistribution: "future" }],
] as const) {
  test(`fork storage does not confirm a ${name} success response`, async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify(result), { status: 201 })),
    );
    await expect(forkSkill(SOURCE)).rejects.toThrow("Invalid fork response");
  });
}

test("fork storage does not confirm an empty success response", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
  await expect(forkSkill(SOURCE)).rejects.toThrow("Invalid fork response");
});
