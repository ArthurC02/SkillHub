import { afterEach, expect, test, vi } from "vitest";
import { setSkillCategory } from "./skills.service";

const SKILL = "abcdef01-2345-4789-abcd-0123456789ab";
const RECEIPT = { skill_id: SKILL, name: "", summary: "", redistribution: "allowed" };

afterEach(() => vi.unstubAllGlobals());

for (const redistribution of ["allowed", "blocked", "unknown", "self_supplied", "generated"]) {
  test(`category storage accepts a matching receipt with ${redistribution} redistribution and empty descriptions`, async () => {
    const receipt = { ...RECEIPT, skill_id: SKILL.toUpperCase(), redistribution };
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(receipt)));
    vi.stubGlobal("fetch", fetch);
    await expect(setSkillCategory(SKILL, "unassigned")).resolves.toEqual(receipt);
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining(`/skills/${SKILL}/category`),
      expect.objectContaining({ method: "PUT", body: '{"category":"unassigned"}' }),
    );
  });
}

for (const [name, body] of [
  ["null", null],
  ["array", []],
  ["text", "stored"],
  ["missing id", { ...RECEIPT, skill_id: undefined }],
  ["numeric id", { ...RECEIPT, skill_id: 1 }],
  ["another skill", { ...RECEIPT, skill_id: "11111111-1111-1111-1111-111111111111" }],
  ["missing name", { ...RECEIPT, name: undefined }],
  ["missing summary", { ...RECEIPT, summary: undefined }],
  ["missing redistribution", { ...RECEIPT, redistribution: undefined }],
  ["unknown redistribution", { ...RECEIPT, redistribution: "future" }],
] as const) {
  test(`category storage does not confirm a ${name} success response`, async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(body))));
    await expect(setSkillCategory(SKILL, "writing")).rejects.toThrow("Invalid category response");
  });
}

test("category storage does not confirm an empty success response", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
  await expect(setSkillCategory(SKILL, "writing")).rejects.toThrow("Invalid category response");
});
